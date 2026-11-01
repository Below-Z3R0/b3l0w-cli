package anilist

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DiscoverItem es una entrada de descubrimiento: no está en la lista del
// usuario, sale del catálogo de AniList.
type DiscoverItem struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	CoverURL string   `json:"coverUrl"`
	CoverClr string   `json:"coverColor"`
	Score    int      `json:"score"`
	Episodes int      `json:"episodes"`
	Status   string   `json:"status"`
	Format   string   `json:"format"`
	Season   string   `json:"season"`
	SeasonYr int      `json:"seasonYear"`
	Genres   []string `json:"genres"`
	Summary  string   `json:"summary"`
}

// DiscoverPage es una página del catálogo.
type DiscoverPage struct {
	Kind      string         `json:"kind"`
	Page      int            `json:"page"`
	HasNext   bool           `json:"hasNext"`
	Total     int            `json:"total"`
	Items     []DiscoverItem `json:"items"`
	FetchedAt int64          `json:"fetchedAt"`
}

const discoverQuery = `query ($page: Int, $perPage: Int, $sort: [MediaSort], $season: MediaSeason, $seasonYear: Int) {
  Page(page: $page, perPage: $perPage) {
    pageInfo { hasNextPage total }
    media(sort: $sort, type: ANIME, season: $season, seasonYear: $seasonYear, isAdult: false) {
      id
      title { userPreferred }
      coverImage { extraLarge large color }
      averageScore episodes status format season seasonYear genres
      description(asHtml: false)
    }
  }
}`

// DiscoverSorts son los únicos valores aceptados por el CLI. Un sort inventado
// hace que AniList devuelva la página por popularidad sin avisar.
var DiscoverSorts = map[string][]string{
	"trending": {"TRENDING_DESC"},
	"popular":  {"POPULARITY_DESC"},
	"top":      {"SCORE_DESC"},
	"new":      {"START_DATE_DESC"},
}

// currentSeason devuelve la temporada de AniList para una fecha.
func currentSeason(t time.Time) (string, int) {
	month := t.Month()
	year := t.Year()
	switch {
	case month <= 3:
		return "WINTER", year
	case month <= 6:
		return "SPRING", year
	case month <= 9:
		return "SUMMER", year
	default:
		return "FALL", year
	}
}

// Discover trae una página del catálogo. `seasonal` no es un sort: es popular
// filtrado por la temporada en curso.
func Discover(ctx context.Context, c *Client, kind string, page, perPage int) (DiscoverPage, error) {
	if perPage <= 0 || perPage > 50 {
		perPage = 24
	}
	if page < 1 {
		page = 1
	}
	// `seasonal` no es un sort: es popularidad filtrada por temporada. Hay que
	// resolverlo ANTES de buscar en el mapa, o el fallback lo convierte en
	// trending sin ningún aviso.
	kind = strings.ToLower(strings.TrimSpace(kind))
	variables := map[string]any{"page": page, "perPage": perPage}
	if kind == "seasonal" {
		season, year := currentSeason(time.Now())
		variables["season"] = season
		variables["seasonYear"] = year
		variables["sort"] = DiscoverSorts["popular"]
	} else if sort, ok := DiscoverSorts[kind]; ok {
		variables["sort"] = sort
	} else {
		return DiscoverPage{}, fmt.Errorf("kind inválido %q: usá trending, popular, top, new o seasonal", kind)
	}
	var data struct {
		Page struct {
			PageInfo struct {
				HasNext bool `json:"hasNextPage"`
				Total   int  `json:"total"`
			} `json:"pageInfo"`
			Media []struct {
				ID    int `json:"id"`
				Title struct {
					Preferred string `json:"userPreferred"`
				} `json:"title"`
				Cover struct {
					ExtraLarge string `json:"extraLarge"`
					Large      string `json:"large"`
					Color      string `json:"color"`
				} `json:"coverImage"`
				Score    int      `json:"averageScore"`
				Episodes int      `json:"episodes"`
				Status   string   `json:"status"`
				Format   string   `json:"format"`
				Season   string   `json:"season"`
				SeasonYr int      `json:"seasonYear"`
				Genres   []string `json:"genres"`
				Summary  string   `json:"description"`
			} `json:"media"`
		} `json:"Page"`
	}
	if err := c.Query(ctx, discoverQuery, variables, &data); err != nil {
		return DiscoverPage{}, err
	}
	out := DiscoverPage{
		Kind:      kind,
		Page:      page,
		HasNext:   data.Page.PageInfo.HasNext,
		Total:     data.Page.PageInfo.Total,
		FetchedAt: time.Now().Unix(),
		Items:     make([]DiscoverItem, 0, len(data.Page.Media)),
	}
	for _, media := range data.Page.Media {
		out.Items = append(out.Items, DiscoverItem{
			ID:       media.ID,
			Title:    media.Title.Preferred,
			CoverURL: firstNonEmpty(media.Cover.ExtraLarge, media.Cover.Large),
			CoverClr: media.Cover.Color,
			Score:    media.Score,
			Episodes: media.Episodes,
			Status:   media.Status,
			Format:   media.Format,
			Season:   media.Season,
			SeasonYr: media.SeasonYr,
			Genres:   media.Genres,
			Summary:  stripHTML(media.Summary),
		})
	}
	return out, nil
}

// stripHTML quita las etiquetas que trae la sinopsis (viene en HTML aunque se
// pida asHtml: false en algunos campos).
func stripHTML(text string) string {
	out := make([]rune, 0, len(text))
	depth := 0
	for _, r := range text {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out = append(out, r)
			}
		}
	}
	return strings.TrimSpace(string(out))
}
