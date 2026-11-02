package ytmusic

import (
	"context"
	"fmt"
	"strconv"
	"sync"
)

const playerURL = "https://music.youtube.com/youtubei/v1/player?prettyPrint=false"

// EnrichDurations completa la duración de las canciones que no la traen.
//
// Las estanterías de la página de inicio ("Quick picks") publican título,
// artista y portada, pero NO la duración: sin este paso la fila de Home se ve
// con el tiempo vacío aunque Charts y Search sí lo muestren. Se pide el
// metadata de cada video en paralelo y se corta en `limit` para no hacer 50
// llamadas por pantalla.
func EnrichDurations(ctx context.Context, c *Client, songs []Song, limit int) []Song {
	if len(songs) == 0 || limit <= 0 {
		return songs
	}
	if limit > len(songs) {
		limit = len(songs)
	}
	sem := make(chan struct{}, 6)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	for index := 0; index < limit; index++ {
		if songs[index].Seconds > 0 {
			continue
		}
		wait.Add(1)
		sem <- struct{}{}
		go func(index int) {
			defer wait.Done()
			defer func() { <-sem }()
			seconds, err := videoSeconds(ctx, c, songs[index].VideoID)
			if err != nil || seconds <= 0 {
				return
			}
			mutex.Lock()
			songs[index].Seconds = seconds
			songs[index].Duration = clockOf(seconds)
			mutex.Unlock()
		}(index)
	}
	wait.Wait()
	return songs
}

func videoSeconds(ctx context.Context, c *Client, videoID string) (int, error) {
	if videoID == "" {
		return 0, fmt.Errorf("player: falta el videoId")
	}
	payload, err := c.PostRaw(ctx, playerURL, c.Body(map[string]any{
		"videoId":     videoID,
		"isAudioOnly": true,
	}))
	if err != nil {
		return 0, err
	}
	details := asMap(payload["videoDetails"])
	raw := asString(details["lengthSeconds"])
	if raw == "" {
		return 0, fmt.Errorf("player %s: sin lengthSeconds", videoID)
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("player %s: lengthSeconds inválido (%q)", videoID, raw)
	}
	return seconds, nil
}

func clockOf(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds%60)
}
