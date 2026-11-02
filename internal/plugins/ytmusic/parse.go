package ytmusic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ── helpers de JSON genérico ─────────────────────────────────────────────
//
// innertube no tiene esquema: la forma cambia entre shelves, y un struct fijo
// se rompe en silencio (campos en cero). Se camina el árbol y se extraen los
// renderers que conocemos.

func asMap(value any) map[string]any {
	if table, ok := value.(map[string]any); ok {
		return table
	}
	return nil
}

func asSlice(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return nil
}

func asString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func asInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		n, _ := strconv.Atoi(typed)
		return n
	}
	return 0
}

// textOf aplana cualquiera de las tres formas en que llega un texto:
// {runs:[{text}]}, {simpleText} o un string pelado.
func textOf(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if content := asString(typed["content"]); content != "" {
			return content
		}
		if simple := asString(typed["simpleText"]); simple != "" {
			return simple
		}
		if runs := asSlice(typed["runs"]); runs != nil {
			var out strings.Builder
			for _, run := range runs {
				out.WriteString(asString(asMap(run)["text"]))
			}
			return out.String()
		}
	}
	return ""
}

// findRenderer devuelve el primer sub-mapa que tenga la clave pedida.
func findRenderer(root any, key string) map[string]any {
	switch typed := root.(type) {
	case map[string]any:
		if inner, ok := typed[key].(map[string]any); ok {
			return inner
		}
		for _, value := range typed {
			if found := findRenderer(value, key); found != nil {
				return found
			}
		}
	case []any:
		for _, value := range typed {
			if found := findRenderer(value, key); found != nil {
				return found
			}
		}
	}
	return nil
}

// collectRenderers junta TODOS los sub-mapas con la clave pedida, en orden de
// aparición (que es el orden de la lista en pantalla).
func collectRenderers(root any, key string, out *[]map[string]any) {
	switch typed := root.(type) {
	case map[string]any:
		if inner, ok := typed[key].(map[string]any); ok {
			*out = append(*out, inner)
		}
		// Se recorren las claves ordenadas para que la extracción sea estable
		// entre llamadas: el orden de un map de Go no lo es.
		keys := make([]string, 0, len(typed))
		for name := range typed {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			collectRenderers(typed[name], key, out)
		}
	case []any:
		for _, value := range typed {
			collectRenderers(value, key, out)
		}
	}
}

// bestThumb elige la portada más grande que no sea gigante: 544 px de carátula
// para una fila de 46 px es ancho de banda tirado.
func bestThumb(renderer map[string]any) string {
	if renderer == nil {
		return ""
	}
	best, bestWidth := "", 0
	if thumb := findRenderer(renderer, "musicThumbnailRenderer"); thumb != nil {
		best, bestWidth = pickThumb(asSlice(asMap(thumb["thumbnail"])["thumbnails"]), 0)
	}
	if thumb := findRenderer(renderer, "thumbnailRenderer"); thumb != nil {
		url, width := pickThumb(asSlice(asMap(thumb["thumbnail"])["thumbnails"]), 0)
		if url != "" && width > bestWidth {
			best, bestWidth = url, width
		}
	}
	if url, width := pickThumb(asSlice(renderer["thumbnails"]), 0); url != "" && width > bestWidth {
		best = url
	}
	// playlistPanelVideoRenderer cuelga las suyas de `thumbnail.thumbnails`.
	if thumb := asMap(renderer["thumbnail"]); thumb != nil {
		if url, width := pickThumb(asSlice(thumb["thumbnails"]), 0); url != "" && width > bestWidth {
			best = url
		}
	}
	return best
}

func pickThumb(thumbs []any, _ int) (string, int) {
	chosen, chosenWidth := "", 0
	for _, entry := range thumbs {
		thumb := asMap(entry)
		url := asString(thumb["url"])
		width := asInt(thumb["width"])
		if url == "" {
			continue
		}
		if width == 0 && chosen == "" {
			chosen = url
		}
		if width <= 400 && width >= chosenWidth {
			chosen, chosenWidth = url, width
		}
	}
	return chosen, chosenWidth
}

// continuationOf busca el primer token de continuación en un subárbol,
// aceptando las cuatro formas que usa innertube.
func continuationOf(root map[string]any) string {
	if root == nil {
		return ""
	}
	for _, key := range []string{
		"nextContinuationData", "nextRadioContinuationData",
		"reloadContinuationData", "continuationCommand",
	} {
		if found := findRenderer(root, key); found != nil {
			if token := asString(found["continuation"]); token != "" {
				return token
			}
			if token := asString(found["token"]); token != "" {
				return token
			}
		}
	}
	return ""
}

// ── canciones ────────────────────────────────────────────────────────────

// songFromResponsive lee un musicResponsiveListItemRenderer: es la forma de
// charts, search y las estanterías de home.
func songFromResponsive(item map[string]any) (Song, bool) {
	song := Song{}
	if data := asMap(item["playlistItemData"]); data != nil {
		song.VideoID = asString(data["videoId"])
	}
	columns := asSlice(item["flexColumns"])
	if len(columns) == 0 {
		return Song{}, false
	}
	title := ""
	var meta []string
	for index, column := range columns {
		holder := asMap(asMap(column)["musicResponsiveListItemFlexColumnRenderer"])
		text := textOf(holder["text"])
		if index == 0 {
			title = strings.TrimSpace(text)
			if song.VideoID == "" {
				// El videoId también viaja en el watchEndpoint del título.
				if endpoint := findRenderer(holder, "watchEndpoint"); endpoint != nil {
					song.VideoID = asString(endpoint["videoId"])
				}
			}
			continue
		}
		if text != "" {
			meta = append(meta, text)
		}
	}
	if title == "" || song.VideoID == "" {
		return Song{}, false
	}
	song.Title = title
	song.Artist, song.Album, song.Duration = splitMeta(meta)

	// La duración a veces sólo está en la columna fija (alineada a la derecha).
	if song.Duration == "" {
		if fixed := asSlice(item["fixedColumns"]); len(fixed) > 0 {
			holder := asMap(asMap(fixed[0])["musicResponsiveListItemFixedColumnRenderer"])
			song.Duration = strings.TrimSpace(textOf(holder["text"]))
		}
	}
	song.Seconds = durationToSeconds(song.Duration)
	song.ThumbURL = bestThumb(item)
	return song, true
}

// songFromPanelItem lee un playlistPanelVideoRenderer: la cola de la radio.
func songFromPanelItem(item map[string]any) (Song, bool) {
	song := Song{}
	song.VideoID = asString(item["videoId"])
	song.Title = strings.TrimSpace(textOf(item["title"]))
	if song.VideoID == "" || song.Title == "" {
		return Song{}, false
	}
	byline := textOf(item["longBylineText"])
	if byline == "" {
		byline = textOf(item["shortBylineText"])
	}
	parts := splitBullets(byline)
	if len(parts) > 0 {
		song.Artist = parts[0]
	}
	song.Duration = strings.TrimSpace(textOf(item["lengthText"]))
	if song.Duration == "" {
		song.Duration = strings.TrimSpace(textOf(item["lengthTextSimple"]))
	}
	song.Seconds = durationToSeconds(song.Duration)
	song.ThumbURL = bestThumb(item)
	return song, true
}

// splitMeta separa "Artista • Álbum • 3:45" (o sólo "Artista").
func splitMeta(columns []string) (artist, album, duration string) {
	var parts []string
	for _, column := range columns {
		parts = append(parts, splitBullets(column)...)
	}
	if len(parts) == 0 {
		return "", "", ""
	}
	artist = parts[0]
	for _, part := range parts[1:] {
		if isDuration(part) {
			duration = part
			continue
		}
		// "1.2B plays", "56M likes", "240K subscribers" no son el álbum.
		lower := strings.ToLower(part)
		if strings.Contains(lower, "play") || strings.Contains(lower, "view") ||
			strings.Contains(lower, "like") || strings.Contains(lower, "subscriber") ||
			strings.Contains(lower, "monthly audience") {
			continue
		}
		if album == "" {
			album = part
		}
	}
	return artist, album, duration
}

func splitBullets(text string) []string {
	fields := strings.Split(text, "•")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func isDuration(text string) bool {
	if !strings.Contains(text, ":") {
		return false
	}
	for _, r := range text {
		if (r < '0' || r > '9') && r != ':' {
			return false
		}
	}
	return true
}

// songsFromRenderers extrae canciones de cualquier subárbol, en orden.
func songsFromRenderers(root any) []Song {
	var responsive []map[string]any
	collectRenderers(root, "musicResponsiveListItemRenderer", &responsive)
	var panel []map[string]any
	collectRenderers(root, "playlistPanelVideoRenderer", &panel)

	songs := make([]Song, 0, len(responsive)+len(panel))
	for _, item := range responsive {
		if song, ok := songFromResponsive(item); ok {
			songs = append(songs, song)
		}
	}
	for _, item := range panel {
		if song, ok := songFromPanelItem(item); ok {
			songs = append(songs, song)
		}
	}
	return songs
}

// playlistCards lista las tarjetas de playlist (charts) con su título.
type playlistCard struct {
	Title    string
	BrowseID string
	Params   string
}

func playlistCards(root any) []playlistCard {
	var items []map[string]any
	collectRenderers(root, "musicTwoRowItemRenderer", &items)
	cards := make([]playlistCard, 0, len(items))
	for _, item := range items {
		endpoint := asMap(asMap(item["navigationEndpoint"])["browseEndpoint"])
		if endpoint == nil {
			continue
		}
		browseID := asString(endpoint["browseId"])
		if !strings.HasPrefix(browseID, "VL") {
			continue
		}
		cards = append(cards, playlistCard{
			Title:    strings.TrimSpace(textOf(item["title"])),
			BrowseID: browseID,
			Params:   asString(endpoint["params"]),
		})
	}
	return cards
}

// feedFrom arma el Feed con lo que haya en la respuesta y falla claro si no hay
// canciones: un feed vacío con exit 0 es el bug que hay que evitar.
func feedFrom(source string, payload map[string]any) (Feed, error) {
	songs := songDedup(songsFromRenderers(payload))
	if len(songs) == 0 {
		return Feed{}, fmt.Errorf("%s: la respuesta no trajo canciones", source)
	}
	return Feed{
		Source:       source,
		Items:        songs,
		Continuation: continuationOf(payload),
		FetchedAt:    nowUnix(),
	}, nil
}
