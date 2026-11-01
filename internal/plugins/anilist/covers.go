package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CoverVersion invalida el caché entero cuando cambia el esquema del meta.
const CoverVersion = 2

// CoverStats es el contrato que consume el plugin: mapa mediaId → ruta local.
type CoverStats struct {
	Dir        string            `json:"dir"`
	Covers     map[string]string `json:"covers"`
	Downloaded int               `json:"downloaded"`
	Reused     int               `json:"reused"`
	Failed     int               `json:"failed"`
	Removed    int               `json:"removed"`
	Bytes      int64             `json:"bytes"`
}

// CoverOptions controla la sincronización.
type CoverOptions struct {
	Workers  int    // descargas en paralelo (default 6)
	ReuseDir string // caché viejo a adoptar si el nuevo está vacío
	GC       bool   // borrar archivos de entradas que ya no están en la lista
}

type coverMeta struct {
	URL     string `json:"url"`
	Version int    `json:"version"`
}

func coverPath(dir string, mediaID int) string {
	return filepath.Join(dir, strconv.Itoa(mediaID)+".jpg")
}

func metaPath(dir string, mediaID int) string {
	return coverPath(dir, mediaID) + ".meta.json"
}

// validCover comprueba archivo + meta + misma URL. Sin meta coincidente el JPG
// puede ser de otra portada (AniList las cambia al anunciar temporada).
func validCover(dir string, entry Entry) bool {
	if entry.CoverURL == "" {
		return false
	}
	if info, err := os.Stat(coverPath(dir, entry.MediaID)); err != nil || info.Size() == 0 {
		return false
	}
	raw, err := os.ReadFile(metaPath(dir, entry.MediaID))
	if err != nil {
		return false
	}
	var meta coverMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	return meta.Version == CoverVersion && meta.URL == entry.CoverURL
}

// adopt reutiliza el caché del plugin viejo con hardlinks: 68 MB de carátulas
// ya descargadas no se vuelven a bajar y no ocupan el doble.
func adopt(dir, reuse string) int {
	if reuse == "" || reuse == dir {
		return 0
	}
	entries, err := os.ReadDir(reuse)
	if err != nil {
		return 0
	}
	adopted := 0
	for _, item := range entries {
		name := item.Name()
		if !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := os.Link(filepath.Join(reuse, name), target); err == nil {
			adopted++
			continue
		}
		// Hardlink puede fallar entre sistemas de archivos: copiar es el fallback.
		if err := copyFile(filepath.Join(reuse, name), target); err == nil {
			adopted++
		}
	}
	return adopted
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func downloadCover(ctx context.Context, client *http.Client, entry Entry, dir string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, entry.CoverURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "b3l0w-anilist")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	final := coverPath(dir, entry.MediaID)
	// .part + rename: la UI nunca ve un JPG a medio escribir.
	tmp := final + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(resp.Body, 16<<20)); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	meta, _ := json.Marshal(coverMeta{URL: entry.CoverURL, Version: CoverVersion})
	if err := os.WriteFile(metaPath(dir, entry.MediaID), meta, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, final)
}

// SyncCovers garantiza que cada entrada con carátula tenga un archivo local y
// devuelve el mapa completo. El mapa es la única fuente de verdad para la UI:
// el modelo de la lista no lleva rutas, así un refresh no puede borrarlas.
func SyncCovers(ctx context.Context, entries []Entry, dir string, opts CoverOptions) (CoverStats, error) {
	stats := CoverStats{Dir: dir, Covers: map[string]string{}}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return stats, err
	}
	stats.Reused = adopt(dir, opts.ReuseDir)

	// Lo que ya está válido entra al mapa sin tocar la red.
	pending := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if validCover(dir, entry) {
			stats.Covers[strconv.Itoa(entry.MediaID)] = coverPath(dir, entry.MediaID)
			continue
		}
		pending = append(pending, entry)
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 6
	}
	client := &http.Client{Timeout: 30 * time.Second}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for _, entry := range pending {
		wg.Add(1)
		go func(e Entry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := downloadCover(ctx, client, e, dir)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				stats.Failed++
				return
			}
			stats.Downloaded++
			stats.Covers[strconv.Itoa(e.MediaID)] = coverPath(dir, e.MediaID)
		}(entry)
	}
	wg.Wait()

	if opts.GC {
		stats.Removed = gcOrphans(dir, stats.Covers)
	}
	for _, path := range stats.Covers {
		if info, err := os.Stat(path); err == nil {
			stats.Bytes += info.Size()
		}
	}
	return stats, nil
}

// gcOrphans borra JPG y meta que ya no corresponden a ninguna entrada.
func gcOrphans(dir string, covers map[string]string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, item := range entries {
		name := item.Name()
		if !strings.HasSuffix(name, ".jpg") {
			continue
		}
		id := strings.TrimSuffix(name, ".jpg")
		if _, ok := covers[id]; ok {
			continue
		}
		if os.Remove(filepath.Join(dir, name)) == nil {
			removed++
		}
		os.Remove(filepath.Join(dir, name+".meta.json"))
	}
	return removed
}
