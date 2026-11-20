package anilist

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// errInvalidMediaID es el error de una media sin id: la API responde con un
// mensaje genérico y el panel necesita saber que el pedido estaba mal.
var errInvalidMediaID = errors.New("media-id inválido")

// AiringItem es un capítulo que se va a airing. La agenda del panel es una
// lista de estos, ordenada por fecha.
type AiringItem struct {
	MediaID   int    `json:"mediaId"`
	Title     string `json:"title"`
	Episode   int    `json:"episode"`
	AiringAt  int64  `json:"airingAt"`
	CoverURL  string `json:"coverUrl"`
	CoverClr  string `json:"coverColor"`
	MediaType string `json:"mediaType"`
}

// Schedule es la agenda de emisión: los capítulos que todavía no se emitieron
// de las series que el usuario está siguiendo.
type Schedule struct {
	Items     []AiringItem `json:"items"`
	FetchedAt int64        `json:"fetchedAt"`
}

// currentListQuery trae las series que el usuario está viendo con su próximo
// capítulo. `Page.mediaList` es la lista (a diferencia del `MediaList` de
// raíz, que devuelve un solo objeto). El `pageInfo` es lo que permite traer
// TODAS: sin paginar, un usuario con más de 50 series en curso veía sólo 50.
const currentListQuery = `query ($userId: Int, $page: Int) {
  Page(page: $page, perPage: 50) {
    pageInfo { hasNextPage }
    mediaList(userId: $userId, status: CURRENT, type: ANIME) {
      mediaId
      status
      media {
        id
        type
        title { userPreferred }
        coverImage { extraLarge color }
        nextAiringEpisode { episode airingAt }
      }
    }
  }
}`

// scheduleQuery trae los capítulos futuros de un conjunto de medias.
// `airingSchedules` acepta mediaId_in y ordena por fecha; se pagina igual que
// la lista, porque la primera página también se cortaba en 50.
const scheduleQuery = `query ($ids: [Int], $page: Int) {
  Page(page: $page, perPage: 50) {
    pageInfo { hasNextPage }
    airingSchedules(mediaId_in: $ids, notYetAired: true, sort: [TIME]) {
      mediaId
      episode
      airingAt
      media {
        id
        type
        title { userPreferred }
        coverImage { extraLarge color }
      }
    }
  }
}`

type currentListResponse struct {
	Page struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		MediaList []struct {
			MediaID int    `json:"mediaId"`
			Status  string `json:"status"`
			Media   struct {
				ID    int    `json:"id"`
				Type  string `json:"type"`
				Title struct {
					Preferred string `json:"userPreferred"`
				} `json:"title"`
				Cover struct {
					ExtraLarge string `json:"extraLarge"`
					Color      string `json:"color"`
				} `json:"coverImage"`
				NextAiring *struct {
					Episode  int   `json:"episode"`
					AiringAt int64 `json:"airingAt"`
				} `json:"nextAiringEpisode"`
			} `json:"media"`
		} `json:"mediaList"`
	} `json:"Page"`
}

type scheduleResponse struct {
	Page struct {
		PageInfo struct {
			HasNextPage bool `json:"hasNextPage"`
		} `json:"pageInfo"`
		Schedules []struct {
			MediaID  int   `json:"mediaId"`
			Episode  int   `json:"episode"`
			AiringAt int64 `json:"airingAt"`
			Media    struct {
				ID    int    `json:"id"`
				Type  string `json:"type"`
				Title struct {
					Preferred string `json:"userPreferred"`
				} `json:"title"`
				Cover struct {
					ExtraLarge string `json:"extraLarge"`
					Color      string `json:"color"`
				} `json:"coverImage"`
			} `json:"media"`
		} `json:"airingSchedules"`
	} `json:"Page"`
}

// FetchSchedule trae la agenda de emisión de las series que el usuario está
// viendo. Son dos consultas: la lista de las series en curso y después los
// capítulos futuros de esas series (bulk por mediaId_in: una consulta para
// todas, no una por serie).
func FetchSchedule(ctx context.Context, c *Client, userID int) (Schedule, error) {
	out := Schedule{Items: []AiringItem{}, FetchedAt: time.Now().Unix()}
	if userID <= 0 {
		viewer, err := FetchViewer(ctx, c)
		if err != nil {
			return out, err
		}
		userID = viewer.ID
	}
	var list currentListResponse
	titulos := map[int]AiringItem{}
	ids := make([]int, 0, 50)
	// Se pagina hasta agotar: con más de 50 series en curso, las que sobraban
	// no aparecían en la agenda (A3). El corte por lista vacía es la red de
	// seguridad contra un `hasNextPage` que mienta.
	for page := 1; ; page++ {
		if err := c.Query(ctx, currentListQuery, map[string]any{"userId": userID, "page": page}, &list); err != nil {
			return out, err
		}
		for _, entry := range list.Page.MediaList {
			if _, seen := titulos[entry.MediaID]; seen {
				continue
			}
			item := AiringItem{
				MediaID:   entry.MediaID,
				Title:     entry.Media.Title.Preferred,
				CoverURL:  entry.Media.Cover.ExtraLarge,
				CoverClr:  entry.Media.Cover.Color,
				MediaType: mediaType(entry.Media.Type),
			}
			titulos[entry.MediaID] = item
			ids = append(ids, entry.MediaID)
		}
		if !list.Page.PageInfo.HasNextPage || len(list.Page.MediaList) == 0 {
			break
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	// La query de capítulos también se pagina: la primera página traía 50 y el
	// resto se perdía con `hasNextPage: true` (A4).
	for page := 1; ; page++ {
		var sched scheduleResponse
		if err := c.Query(ctx, scheduleQuery, map[string]any{"ids": ids, "page": page}, &sched); err != nil {
			return out, fmt.Errorf("agenda de emisión: %w", err)
		}
		for _, node := range sched.Page.Schedules {
			item, ok := titulos[node.MediaID]
			if !ok {
				item = AiringItem{
					MediaID:   node.MediaID,
					Title:     node.Media.Title.Preferred,
					CoverURL:  node.Media.Cover.ExtraLarge,
					CoverClr:  node.Media.Cover.Color,
					MediaType: mediaType(node.Media.Type),
				}
			}
			item.Episode = node.Episode
			item.AiringAt = node.AiringAt
			out.Items = append(out.Items, item)
		}
		if !sched.Page.PageInfo.HasNextPage || len(sched.Page.Schedules) == 0 {
			break
		}
	}
	return out, nil
}

// mediaType devuelve el tipo de la media que trajo la API. La agenda es de
// anime, así que ANIME es el default: una respuesta vieja sin `type` no puede
// dejar la etiqueta vacía (M1).
func mediaType(value string) string {
	if value == "" {
		return "ANIME"
	}
	return value
}
