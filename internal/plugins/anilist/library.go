package anilist

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ChunkSize es cuántas entradas pide por página. La API acepta hasta 500
// entradas por MediaListCollection: pedir de a 50 multiplica por diez las
// consultas (y con ellas los chances de un 429) sin ganar nada (M3).
const ChunkSize = 500

// Entry es una entrada de la lista ya normalizada. Los nombres cortos son a
// propósito: el panel la consume como JSON y cada byte viaja al runtime de Luau.
type Entry struct {
	MediaID   int      `json:"mediaId"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Progress  int      `json:"progress"`
	Volumes   int      `json:"volumes,omitempty"`
	Score     float64  `json:"score"`
	Total     int      `json:"total"`
	CoverURL  string   `json:"coverUrl"`
	CoverClr  string   `json:"coverColor"`
	UpdatedAt int64    `json:"updatedAt"`
	AiringAt  int64    `json:"airingAt,omitempty"`
	AiringEp  int      `json:"airingEpisode,omitempty"`
	Genres    []string `json:"genres,omitempty"`
}

// Library es el resultado completo de una sincronización.
type Library struct {
	Viewer    Viewer  `json:"viewer"`
	Anime     []Entry `json:"anime"`
	Manga     []Entry `json:"manga"`
	FetchedAt int64   `json:"fetchedAt"`
	Partial   bool    `json:"partial"` // true si algún tipo falló a mitad
}

// Viewer es el usuario autenticado.
type Viewer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

const viewerQuery = `query { Viewer { id name } }`

const listQuery = `query ($userId: Int, $type: MediaType, $chunk: Int, $perChunk: Int) {
  MediaListCollection(userId: $userId, type: $type, forceSingleCompletedList: true, chunk: $chunk, perChunk: $perChunk) {
    hasNextChunk
    lists {
      entries {
        id mediaId status progress progressVolumes score(format: POINT_100) updatedAt
        media {
          id type episodes chapters volumes
          coverImage { color extraLarge large medium }
          title { userPreferred }
          genres
          nextAiringEpisode { airingAt episode }
        }
      }
    }
  }
}`

// FetchViewer devuelve el usuario dueño del token.
func FetchViewer(ctx context.Context, c *Client) (Viewer, error) {
	var data struct {
		Viewer Viewer `json:"Viewer"`
	}
	err := c.Query(ctx, viewerQuery, nil, &data)
	return data.Viewer, err
}

// listResponse usa un puntero para la colección: `MediaListCollection: null`
// es una respuesta que no se puede leer, no una biblioteca vacía (B3).
type listResponse struct {
	MediaListCollection *struct {
		HasNextChunk bool `json:"hasNextChunk"`
		Lists        []struct {
			Entries []rawEntry `json:"entries"`
		} `json:"lists"`
	} `json:"MediaListCollection"`
}

type rawEntry struct {
	MediaID  int     `json:"mediaId"`
	Status   string  `json:"status"`
	Progress int     `json:"progress"`
	Volumes  int     `json:"progressVolumes"`
	Score    float64 `json:"score"`
	Updated  int64   `json:"updatedAt"`
	Media    struct {
		Episodes int `json:"episodes"`
		Chapters int `json:"chapters"`
		Volumes  int `json:"volumes"`
		Cover    struct {
			Color      string `json:"color"`
			ExtraLarge string `json:"extraLarge"`
			Large      string `json:"large"`
			Medium     string `json:"medium"`
		} `json:"coverImage"`
		Title struct {
			Preferred string `json:"userPreferred"`
		} `json:"title"`
		Genres []string `json:"genres"`
		Airing *struct {
			AiringAt int64 `json:"airingAt"`
			Episode  int   `json:"episode"`
		} `json:"nextAiringEpisode"`
	} `json:"media"`
}

func (r rawEntry) normalize(kind string) Entry {
	entry := Entry{
		MediaID:   r.MediaID,
		Title:     r.Media.Title.Preferred,
		Status:    r.Status,
		Progress:  r.Progress,
		Volumes:   r.Volumes,
		Score:     r.Score,
		UpdatedAt: r.Updated,
		CoverURL:  firstNonEmpty(r.Media.Cover.ExtraLarge, r.Media.Cover.Large, r.Media.Cover.Medium),
		CoverClr:  r.Media.Cover.Color,
		Genres:    r.Media.Genres,
	}
	if entry.Genres == nil {
		entry.Genres = []string{}
	}
	if kind == "MANGA" {
		entry.Total = r.Media.Chapters
	} else {
		entry.Total = r.Media.Episodes
	}
	if r.Media.Airing != nil {
		entry.AiringAt = r.Media.Airing.AiringAt
		entry.AiringEp = r.Media.Airing.Episode
	}
	return entry
}

// FetchList pagina una lista completa y la mergea por mediaId: la misma entrada
// puede aparecer en dos listas (en curso y repetida) y gana la más reciente.
func FetchList(ctx context.Context, c *Client, userID int, kind string) ([]Entry, error) {
	byID := map[int]Entry{}
	// El chunk arranca en 1: la API devuelve la MISMA página para `chunk: 0` y
	// `chunk: 1`, así que pedir el 0 es una consulta de más por biblioteca (M4).
	for chunk := 1; ; chunk++ {
		variables := map[string]any{
			"userId":   userID,
			"type":     kind,
			"chunk":    chunk,
			"perChunk": ChunkSize,
		}
		var page listResponse
		if err := c.Query(ctx, listQuery, variables, &page); err != nil {
			return nil, err
		}
		// `MediaListCollection: null` no es "el usuario no tiene nada": es una
		// respuesta que no se puede leer, y sin este error el panel muestra
		// "anime: 0" como si la biblioteca estuviera vacía (B3).
		if page.MediaListCollection == nil {
			return nil, fmt.Errorf("la API devolvió la colección nula de %s del usuario %d", kind, userID)
		}
		for _, list := range page.MediaListCollection.Lists {
			for _, raw := range list.Entries {
				entry := raw.normalize(kind)
				if prev, ok := byID[entry.MediaID]; ok && !betterEntry(entry, prev) {
					continue
				}
				byID[entry.MediaID] = entry
			}
		}
		if !page.MediaListCollection.HasNextChunk {
			break
		}
	}
	out := make([]Entry, 0, len(byID))
	for _, entry := range byID {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out, nil
}

// Sync trae viewer + ambas listas. Si una lista falla, devuelve la otra con
// Partial=true en vez de tirar todo el trabajo a la basura. Si fallan las DOS,
// devuelve error: "no pude leer la biblioteca" no es lo mismo que "la
// biblioteca está vacía", y confundirlos hacía que el GC borrara las carátulas
// del usuario (C1).
func Sync(ctx context.Context, c *Client, userID int) (Library, error) {
	lib := Library{FetchedAt: time.Now().Unix()}
	if userID == 0 {
		viewer, err := FetchViewer(ctx, c)
		if err != nil {
			return lib, err
		}
		lib.Viewer = viewer
		userID = viewer.ID
	} else {
		lib.Viewer.ID = userID
	}
	anime, animeErr := FetchList(ctx, c, userID, "ANIME")
	manga, mangaErr := FetchList(ctx, c, userID, "MANGA")
	if animeErr != nil && mangaErr != nil {
		return lib, fmt.Errorf("biblioteca: %w", errors.Join(animeErr, mangaErr))
	}
	if animeErr != nil || mangaErr != nil {
		lib.Partial = true
	}
	lib.Anime = anime
	lib.Manga = manga
	if lib.Anime == nil {
		lib.Anime = []Entry{}
	}
	if lib.Manga == nil {
		lib.Manga = []Entry{}
	}
	return lib, nil
}

// betterEntry dice si `next` describe mejor la misma obra que `prev`.
// Primero la más reciente por `updatedAt`; con empate —el reloj de AniList
// tiene resolución de segundos y dos listas pueden tocarse en el mismo
// segundo— gana la más completa: más progreso, después nota, después la que
// no está en PLANNING. Sin esto el merge se queda con la primera que vio,
// que puede ser la de menos progreso (M5).
func betterEntry(next, prev Entry) bool {
	if next.UpdatedAt != prev.UpdatedAt {
		return next.UpdatedAt > prev.UpdatedAt
	}
	if next.Progress != prev.Progress {
		return next.Progress > prev.Progress
	}
	if next.Score != prev.Score {
		return next.Score > prev.Score
	}
	if (next.Status != "PLANNING") != (prev.Status != "PLANNING") {
		return next.Status != "PLANNING"
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
