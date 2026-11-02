package ytmusic

import (
	"context"
	"fmt"
	"strings"
)

// Home trae la página de inicio de YTM ("Quick picks", mixes, novedades) y
// devuelve las canciones que encuentre, en el orden en que YouTube las puso.
//
// Una página de inicio anónima trae pocas estanterías de canciones y muchas de
// playlists: si no hay ni una canción, se sigue la primera tarjeta de playlist
// con browseEndpoint y se expande. Eso mantiene la pantalla con contenido real
// en vez de un "0 canciones" que el usuario lee como plugin roto.
func Home(ctx context.Context, c *Client, continuation string) (Feed, error) {
	var payload map[string]any
	var err error
	if continuation == "" {
		payload, err = c.Browse(ctx, "FEmusic_home", "")
	} else {
		payload, err = c.BrowseContinuation(ctx, continuation)
	}
	if err != nil {
		return Feed{}, fmt.Errorf("home: %w", err)
	}
	songs := songDedup(songsFromRenderers(payload))
	if len(songs) == 0 && continuation == "" {
		expanded, expandErr := expandFirstPlaylist(ctx, c, payload)
		if expandErr != nil {
			return Feed{}, fmt.Errorf("home: la página de inicio no trajo canciones y no se pudo expandir ninguna playlist: %w", expandErr)
		}
		return expanded, nil
	}
	if len(songs) == 0 {
		return Feed{}, fmt.Errorf("home: la continuación %q no trajo canciones", shortToken(continuation))
	}
	return Feed{
		Source:       "home",
		Title:        shelfTitles(payload),
		Items:        songs,
		Continuation: continuationOf(payload),
		FetchedAt:    nowUnix(),
	}, nil
}

// expandFirstPlaylist abre la primera tarjeta que sea una playlist y devuelve
// sus canciones. Es el fallback de home y el mecanismo de charts.
func expandFirstPlaylist(ctx context.Context, c *Client, payload map[string]any) (Feed, error) {
	cards := playlistCards(payload)
	if len(cards) == 0 {
		return Feed{}, fmt.Errorf("la página no tiene tarjetas de playlist")
	}
	var lastErr error
	for _, card := range cards {
		page, err := c.Browse(ctx, card.BrowseID, card.Params)
		if err != nil {
			lastErr = err
			continue
		}
		songs := songDedup(songsFromRenderers(page))
		if len(songs) == 0 {
			continue
		}
		return Feed{
			Source:       "playlist:" + card.BrowseID,
			Title:        card.Title,
			Items:        songs,
			Continuation: continuationOf(page),
			FetchedAt:    nowUnix(),
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%d tarjetas, ninguna con canciones", len(cards))
	}
	return Feed{}, lastErr
}

// shelfTitles resume las estanterías de una página para el subtítulo del panel.
func shelfTitles(payload map[string]any) string {
	var shelves []map[string]any
	collectRenderers(payload, "musicCarouselShelfRenderer", &shelves)
	titles := make([]string, 0, 3)
	for _, shelf := range shelves {
		header := findRenderer(shelf, "musicCarouselShelfBasicHeaderRenderer")
		if header == nil {
			continue
		}
		title := strings.TrimSpace(textOf(header["title"]))
		if title == "" {
			continue
		}
		titles = append(titles, title)
		if len(titles) == 3 {
			break
		}
	}
	return strings.Join(titles, " · ")
}

func shortToken(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12] + "…"
}
