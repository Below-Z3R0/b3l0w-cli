package ytmusic

import (
	"context"
	"fmt"
	"strings"
)

// Search busca canciones (filtro "songs" de YTM) y pagina con continuation.
func Search(ctx context.Context, c *Client, query, continuation string) (Feed, error) {
	query = strings.TrimSpace(query)
	if query == "" && continuation == "" {
		return Feed{}, fmt.Errorf("search: falta la consulta")
	}
	payload, err := c.Search(ctx, query, continuation)
	if err != nil {
		return Feed{}, fmt.Errorf("search: %w", err)
	}
	songs := songDedup(songsFromRenderers(payload))
	if len(songs) == 0 {
		if continuation != "" {
			return Feed{}, fmt.Errorf("search: la página siguiente de %q no trajo canciones", query)
		}
		return Feed{}, fmt.Errorf("search: sin resultados para %q", query)
	}
	return Feed{
		Source:       "search",
		Title:        query,
		Items:        songs,
		Continuation: continuationOf(payload),
		FetchedAt:    nowUnix(),
	}, nil
}
