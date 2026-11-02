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
	if limit <= 0 || limit > len(songs) {
		limit = len(songs)
	}
	// Las portadas ya cacheadas se resuelven sin red y sin goroutine.
	pending := make([]int, 0, limit)
	for index := 0; index < limit; index++ {
		path := filepath.Join(dir, songs[index].VideoID+".jpg")
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
			path := filepath.Join(dir, songs[index].VideoID+".jpg")
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
