package ytmusic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// safeThumbName devuelve el nombre de archivo de la portada de un videoId.
//
// El id viene de la red y no está validado: sin sanear, un "../../escaped"
// sacaba la escritura fuera del directorio de portadas (Y9). Un id que no sea
// un nombre plano se descarta: la canción queda sin portada en vez de
// escribir donde le digan.
func safeThumbName(videoID string) (string, bool) {
	if videoID == "" || len(videoID) > 64 {
		return "", false
	}
	for _, r := range videoID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return "", false
		}
	}
	return videoID, true
}

// Thumbs baja las portadas a disco: ui.image sólo acepta rutas locales, y una
// URL remota en el panel se dibuja como un rectángulo vacío sin ningún error.
// Se cachea por videoId: la segunda visita a Charts no baja nada.
func Thumbs(ctx context.Context, dir string, songs []Song, limit int) []Song {
	if len(songs) == 0 || dir == "" {
		return songs
	}
	// `dir` es el destino tal cual lo pasa el plugin: agregarle "thumbs" otra vez
	// dejaba las portadas en <data>/thumbs/thumbs y el panel sin imágenes.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return songs
	}
	// Un límite inválido (0 o negativo) no significa "todas": no se baja
	// ninguna. Es lo mismo que hace EnrichDurations (Y14).
	if limit <= 0 {
		return songs
	}
	if limit > len(songs) {
		limit = len(songs)
	}
	// Las portadas ya cacheadas se resuelven sin red y sin goroutine.
	pending := make([]int, 0, limit)
	for index := 0; index < limit; index++ {
		name, ok := safeThumbName(songs[index].VideoID)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name+".jpg")
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			songs[index].Thumb = path
			continue
		}
		pending = append(pending, index)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	sem := make(chan struct{}, 5)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	for _, index := range pending {
		wait.Add(1)
		sem <- struct{}{}
		go func(index int) {
			defer wait.Done()
			defer func() { <-sem }()
			url := songs[index].ThumbURL
			if url == "" && songs[index].VideoID != "" {
				url = "https://i.ytimg.com/vi/" + songs[index].VideoID + "/mqdefault.jpg"
			}
			if url == "" {
				return
			}
			name, ok := safeThumbName(songs[index].VideoID)
			if !ok {
				return
			}
			path := filepath.Join(dir, name+".jpg")
			if err := downloadThumb(ctx, client, url, path); err != nil {
				return
			}
			mutex.Lock()
			songs[index].Thumb = path
			mutex.Unlock()
		}(index)
	}
	wait.Wait()
	return songs
}

func downloadThumb(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Chrome/131.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("portada %s: HTTP %d", filepath.Base(path), resp.StatusCode)
	}
	tmp := path + ".part"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 4<<20)); err != nil {
		file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if info, err := os.Stat(tmp); err != nil || info.Size() < 512 {
		_ = os.Remove(tmp)
		return fmt.Errorf("portada %s: respuesta demasiado chica", filepath.Base(path))
	}
	return os.Rename(tmp, path)
}
