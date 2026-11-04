package anilist

import (
	"context"
	"time"
)

// AiringEpisode es un capítulo futuro de una serie. Es lo que devuelve
// `nextAiringEpisode` (el próximo) y `airingSchedule` (la lista de los que
// vienen). La cuenta regresiva del panel sale de airingAt.
type AiringEpisode struct {
	Episode  int   `json:"episode"`
	AiringAt int64 `json:"airingAt"`
}

// MediaDetail es la ficha completa de una serie: lo que la vista de detalle
// necesita y la biblioteca NO trae (sinopsis, formato, nota, próximos
// capítulos). La biblioteca es una lista de 300 entradas y meterle la
// sinopsis de cada una al estado del panel sería un payload imposible; por
// eso la ficha se pide sólo para la serie que se está mirando.
type MediaDetail struct {
	ID          int             `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Genres      []string        `json:"genres"`
	Episodes    int             `json:"episodes"`
	Chapters    int             `json:"chapters"`
	Format      string          `json:"format"`
	Status      string          `json:"status"`
	Score       int             `json:"averageScore"`
	CoverURL    string          `json:"coverUrl"`
	CoverClr    string          `json:"coverColor"`
	NextAiring  *AiringEpisode  `json:"nextAiring,omitempty"`
	Schedule    []AiringEpisode `json:"schedule"`
	ListStatus  string          `json:"listStatus"`
	Progress    int             `json:"progress"`
	ListScore   float64         `json:"listScore"`
	UpdatedAt   int64           `json:"updatedAt"`
	FetchedAt   int64           `json:"fetchedAt"`
}

// mediaQuery trae la ficha de una media y la entrada del usuario en su lista.
//
// `MediaList(mediaId: $id)` sin userId es la del usuario AUTENTICADO: es
// exactamente la que hay que mutar con SaveProgress, así que la ficha y el
// progreso siempre hablan de la misma entrada.
const mediaQuery = `query ($id: Int) {
  Media(id: $id) {
    id
    title { userPreferred }
    description(asHtml: false)
    genres
    episodes
    chapters
    format
    status
    averageScore
    coverImage { extraLarge large color }
    nextAiringEpisode { episode airingAt }
    airingSchedule(notYetAired: true, perPage: 8) { nodes { episode airingAt } }
  }
  MediaList(mediaId: $id) { status progress score updatedAt }
}`

type mediaResponse struct {
	Media struct {
		ID          int    `json:"id"`
		Title       struct {
			Preferred string `json:"userPreferred"`
		} `json:"title"`
		Description string   `json:"description"`
		Genres      []string `json:"genres"`
		Episodes    int      `json:"episodes"`
		Chapters    int      `json:"chapters"`
		Format      string   `json:"format"`
		Status      string   `json:"status"`
		Score       int      `json:"averageScore"`
		Cover       struct {
			ExtraLarge string `json:"extraLarge"`
			Large      string `json:"large"`
			Color      string `json:"color"`
		} `json:"coverImage"`
		NextAiring *struct {
			Episode  int   `json:"episode"`
			AiringAt int64 `json:"airingAt"`
		} `json:"nextAiringEpisode"`
		Schedule struct {
			Nodes []struct {
				Episode  int   `json:"episode"`
				AiringAt int64 `json:"airingAt"`
			} `json:"nodes"`
		} `json:"airingSchedule"`
	} `json:"Media"`
	MediaList *struct {
		Status    string  `json:"status"`
		Progress  int     `json:"progress"`
		Score     float64 `json:"score"`
		UpdatedAt int64   `json:"updatedAt"`
	} `json:"MediaList"`
}

// FetchMedia trae la ficha de una media con la entrada del usuario en su
// lista. La sinopsis viene con HTML pegado (aunque se pida asHtml: false:
// el texto original de AniList trae <br>), así que pasa por stripHTML.
func FetchMedia(ctx context.Context, c *Client, mediaID int) (MediaDetail, error) {
	out := MediaDetail{ID: mediaID, FetchedAt: time.Now().Unix()}
	if mediaID <= 0 {
		return out, errInvalidMediaID
	}
	variables := map[string]any{"id": mediaID}
	var data mediaResponse
	if err := c.Query(ctx, mediaQuery, variables, &data); err != nil {
		return out, err
	}
	media := data.Media
	out.Title = media.Title.Preferred
	out.Description = stripHTML(media.Description)
	out.Genres = media.Genres
	if out.Genres == nil {
		out.Genres = []string{}
	}
	out.Episodes = media.Episodes
	out.Chapters = media.Chapters
	out.Format = media.Format
	out.Status = media.Status
	out.Score = media.Score
	out.CoverURL = firstNonEmpty(media.Cover.ExtraLarge, media.Cover.Large)
	out.CoverClr = media.Cover.Color
	if media.NextAiring != nil {
		out.NextAiring = &AiringEpisode{Episode: media.NextAiring.Episode, AiringAt: media.NextAiring.AiringAt}
	}
	out.Schedule = make([]AiringEpisode, 0, len(media.Schedule.Nodes))
	for _, node := range media.Schedule.Nodes {
		out.Schedule = append(out.Schedule, AiringEpisode{Episode: node.Episode, AiringAt: node.AiringAt})
	}
	if data.MediaList != nil {
		out.ListStatus = data.MediaList.Status
		out.Progress = data.MediaList.Progress
		out.ListScore = data.MediaList.Score
		out.UpdatedAt = data.MediaList.UpdatedAt
	}
	return out, nil
}
