package ytmusic

import (
	"context"
	"fmt"
	"strings"
)

// Charts trae el ranking de YouTube Music.
//
// La página `FEmusic_charts` devuelve, según país y cliente, estanterías de
// canciones y/o tarjetas de playlist ("Top 100 …", "Trending 20 …"). Se publica
// lo que haya: primero las canciones sueltas de la página; si no hay, se abre la
// tarjeta cuyo título dice "Top" (o la primera de rankings) y se listan sus
// canciones. Si no hay ni una cosa ni la otra, es error: un chart vacío no es un
// chart.
func Charts(ctx context.Context, c *Client, country string) (Feed, error) {
	params := ""
	if country != "" && country != "ZZ" {
		// El país se elige por el formulario de la página; se manda como params
		// sólo cuando el usuario lo pide explícito.
		c.GL = strings.ToUpper(country)
	}
	payload, err := c.Browse(ctx, "FEmusic_charts", params)
	if err != nil {
		return Feed{}, fmt.Errorf("charts: %w", err)
	}

	songs := songDedup(songsFromRenderers(payload))
	if len(songs) > 0 {
		feed := Feed{
			Source:       "charts",
			Title:        chartsTitle(payload),
			Items:        songs,
			Continuation: continuationOf(payload),
			FetchedAt:    nowUnix(),
		}
		rankSongs(feed.Items)
		return feed, nil
	}

	cards := playlistCards(payload)
	if len(cards) == 0 {
		return Feed{}, fmt.Errorf("charts: la página no trajo canciones ni tarjetas de ranking (¿cambió la forma del JSON?)")
	}
	// Preferencia por el ranking más completo: "Top 100" > "Top 50" > "Top" >
	// "Trending". Las estanterías de videos vienen antes en la página, así que
	// quedarse con la primera tarjeta devuelve el chart equivocado.
	best, bestScore := -1, -1
	for index, card := range cards {
		lower := strings.ToLower(card.Title)
		score := 0
		switch {
		case strings.Contains(lower, "top 100"):
			score = 100
		case strings.Contains(lower, "top 50"):
			score = 80
		case strings.Contains(lower, "top"):
			score = 40
		case strings.Contains(lower, "trending"):
			score = 20
		}
		if strings.Contains(lower, "song") {
			score += 10
		}
		if strings.Contains(lower, "global") {
			score += 5
		}
		if score > bestScore {
			best, bestScore = index, score
		}
	}
	card := cards[best]
	page, err := c.Browse(ctx, card.BrowseID, card.Params)
	if err != nil {
		return Feed{}, fmt.Errorf("charts: abriendo %q: %w", card.Title, err)
	}
	songs = songDedup(songsFromRenderers(page))
	if len(songs) == 0 {
		return Feed{}, fmt.Errorf("charts: el ranking %q no trajo canciones", card.Title)
	}
	feed := Feed{
		Source:       "charts:" + card.BrowseID,
		Title:        card.Title,
		Items:        songs,
		Continuation: continuationOf(page),
		FetchedAt:    nowUnix(),
	}
	rankSongs(feed.Items)
	return feed, nil
}

// chartsTitle arma "Charts · Panama" con el país que la propia página declara.
func chartsTitle(payload map[string]any) string {
	title := "Charts"
	if header := findRenderer(payload, "musicHeaderRenderer"); header != nil {
		if text := strings.TrimSpace(textOf(header["title"])); text != "" {
			title = text
		}
	}
	if filter := findRenderer(payload, "musicSortFilterButtonRenderer"); filter != nil {
		if country := strings.TrimSpace(textOf(filter["title"])); country != "" {
			title += " · " + country
		}
	}
	return title
}

// rankSongs numera la lista: el panel muestra el puesto sin recalcularlo.
func rankSongs(songs []Song) {
	for index := range songs {
		songs[index].ChartRank = index + 1
	}
}
