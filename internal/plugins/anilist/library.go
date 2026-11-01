package anilist

import (
	"context"
	"sort"
	"strings"
	"time"
)

// ChunkSize es cuántas entradas pide por página. 50 es el máximo que acepta la
// API para MediaListCollection.
const ChunkSize = 50

// Entry es una entrada de la lista ya normalizada. Los nombres cortos son a
// propósito: el panel la consume como JSON y cada byte viaja al runtime de Luau.
type Entry struct {
	MediaID   int     `json:"mediaId"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	Progress  int     `json:"progress"`
	Volumes   int     `json:"volumes,omitempty"`
	Score     float64 `json:"score"`
	Total     int     `json:"total"`
	CoverURL  string  `json:"coverUrl"`
	CoverClr  string  `json:"coverColor"`
	UpdatedAt int64   `json:"updatedAt"`
	AiringAt  int64   `json:"airingAt,omitempty"`
	AiringEp  int     `json:"airingEpisode,omitempty"`
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

type listResponse struct {
	MediaListCollection struct {
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
	for chunk := 0; ; chunk++ {
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
		for _, list := range page.MediaListCollection.Lists {
			for _, raw := range list.Entries {
				entry := raw.normalize(kind)
				if prev, ok := byID[entry.MediaID]; ok && prev.UpdatedAt >= entry.UpdatedAt {
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
// Partial=true en vez de tirar todo el trabajo a la basura.
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
	if entries, err := FetchList(ctx, c, userID, "ANIME"); err == nil {
		lib.Anime = entries
	} else if lib.Anime == nil {
		lib.Partial = true
	}
	if entries, err := FetchList(ctx, c, userID, "MANGA"); err == nil {
		lib.Manga = entries
	} else {
		lib.Partial = true
	}
	if lib.Anime == nil {
		lib.Anime = []Entry{}
	}
	if lib.Manga == nil {
		lib.Manga = []Entry{}
	}
	return lib, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
