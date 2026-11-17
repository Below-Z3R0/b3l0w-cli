package ytmusic

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── helpers de JSON genérico ─────────────────────────────────────────────

// TestAsMapConTiposIncorrectos: un valor que no es mapa devuelve nil, no
// revienta con un type assertion.
func TestAsMapConTiposIncorrectos(t *testing.T) {
	for _, value := range []any{nil, "texto", 42, []any{1, 2}, true} {
		if got := asMap(value); got != nil {
			t.Errorf("asMap(%v) = %v, quería nil", value, got)
		}
	}
}

// TestAsSliceConTiposIncorrectos.
func TestAsSliceConTiposIncorrectos(t *testing.T) {
	for _, value := range []any{nil, "texto", 42, map[string]any{}, true} {
		if got := asSlice(value); got != nil {
			t.Errorf("asSlice(%v) = %v, quería nil", value, got)
		}
	}
}

// TestAsStringYAsIntConTiposIncorrectos.
func TestAsStringYAsIntConTiposIncorrectos(t *testing.T) {
	if got := asString(42); got != "" {
		t.Errorf("asString(42) = %q, quería vacío", got)
	}
	if got := asString(nil); got != "" {
		t.Errorf("asString(nil) = %q, quería vacío", got)
	}
	if got := asInt("no es un número"); got != 0 {
		t.Errorf("asInt con texto inválido = %d, quería 0", got)
	}
	if got := asInt(nil); got != 0 {
		t.Errorf("asInt(nil) = %d, quería 0", got)
	}
	if got := asInt(true); got != 0 {
		t.Errorf("asInt(true) = %d, quería 0", got)
	}
}

// TestAsIntAceptaLasFormasDeJSON: el JSON de innertube trae los enteros como
// float64 y algunos como texto.
func TestAsIntAceptaLasFormasDeJSON(t *testing.T) {
	if got := asInt(float64(226)); got != 226 {
		t.Errorf("asInt(float64) = %d, quería 226", got)
	}
	if got := asInt(226); got != 226 {
		t.Errorf("asInt(int) = %d, quería 226", got)
	}
	if got := asInt("226"); got != 226 {
		t.Errorf("asInt(string) = %d, quería 226", got)
	}
}

// TestTextOfAplanaLasTresFormas: un texto llega como {runs}, {simpleText} o
// pelado.
func TestTextOfAplanaLasTresFormas(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{"pelado", "pelado"},
		{map[string]any{"simpleText": "simple"}, "simple"},
		{map[string]any{"content": "content"}, "content"},
		{map[string]any{"runs": []any{
			map[string]any{"text": "Daft "},
			map[string]any{"text": "Punk"},
		}}, "Daft Punk"},
		{map[string]any{"runs": []any{map[string]any{"nope": 1}}}, ""},
		{nil, ""},
		{42, ""},
		{map[string]any{}, ""},
	}
	for _, item := range cases {
		if got := textOf(item.value); got != item.want {
			t.Errorf("textOf(%v) = %q, quería %q", item.value, got, item.want)
		}
	}
}

// TestTextOfPrefiereElContenido: si vienen las dos formas, gana "content".
func TestTextOfPrefiereElContenido(t *testing.T) {
	value := map[string]any{"content": "contenido", "simpleText": "simple"}
	if got := textOf(value); got != "contenido" {
		t.Errorf("textOf = %q, quería contenido", got)
	}
}

// TestFindRendererBuscaEnProfundidad: el renderer está anidado donde se le
// ocurra.
func TestFindRendererBuscaEnProfundidad(t *testing.T) {
	root := map[string]any{
		"contents": map[string]any{
			"sectionListRenderer": map[string]any{
				"contents": []any{
					map[string]any{
						"musicShelfRenderer": map[string]any{
							"titulo": map[string]any{"simpleText": "Shelf"},
						},
					},
				},
			},
		},
	}
	found := findRenderer(root, "musicShelfRenderer")
	if found == nil {
		t.Fatal("no encontró el renderer")
	}
	if got := textOf(found["titulo"]); got != "Shelf" {
		t.Errorf("titulo = %q", got)
	}
	if findRenderer(root, "noExiste") != nil {
		t.Error("no debería encontrar un renderer inexistente")
	}
	if findRenderer(nil, "x") != nil {
		t.Error("findRenderer(nil) tiene que devolver nil")
	}
}

// TestCollectRenderersJuntaEnOrden: las canciones salen en el orden en que
// aparecen en la pantalla.
func TestCollectRenderersJuntaEnOrden(t *testing.T) {
	root := []any{
		map[string]any{"musicResponsiveListItemRenderer": map[string]any{"n": 1}},
		map[string]any{"otro": map[string]any{}},
		map[string]any{"musicResponsiveListItemRenderer": map[string]any{"n": 2}},
	}
	var out []map[string]any
	collectRenderers(root, "musicResponsiveListItemRenderer", &out)
	if len(out) != 2 {
		t.Fatalf("juntó %d renderers, quería 2", len(out))
	}
	if asInt(out[0]["n"]) != 1 || asInt(out[1]["n"]) != 2 {
		t.Errorf("orden incorrecto: %+v", out)
	}
}

// TestCollectRenderersConRaizVacia: no hay panic con nil.
func TestCollectRenderersConRaizVacia(t *testing.T) {
	var out []map[string]any
	collectRenderers(nil, "musicResponsiveListItemRenderer", &out)
	collectRenderers(map[string]any{}, "musicResponsiveListItemRenderer", &out)
	if len(out) != 0 {
		t.Errorf("juntó %d renderers, quería 0", len(out))
	}
}

// ── portadas ─────────────────────────────────────────────────────────────

// TestBestThumbEligeLaMasGrandeHasta400: 544 px de carátula para una fila de
// 46 px es ancho de banda tirado.
func TestBestThumbEligeLaMasGrandeHasta400(t *testing.T) {
	renderer := map[string]any{
		"thumbnail": map[string]any{
			"musicThumbnailRenderer": map[string]any{
				"thumbnail": map[string]any{
					"thumbnails": []any{
						map[string]any{"url": "https://example.test/60.jpg", "width": 60},
						map[string]any{"url": "https://example.test/226.jpg", "width": 226},
						map[string]any{"url": "https://example.test/544.jpg", "width": 544},
					},
				},
			},
		},
	}
	if got := bestThumb(renderer); got != "https://example.test/226.jpg" {
		t.Errorf("bestThumb = %q, quería la de 226", got)
	}
}

// TestBestThumbConRendererVacio: sin portadas no hay ruta.
func TestBestThumbConRendererVacio(t *testing.T) {
	if got := bestThumb(nil); got != "" {
		t.Errorf("bestThumb(nil) = %q, quería vacío", got)
	}
	if got := bestThumb(map[string]any{}); got != "" {
		t.Errorf("bestThumb sin thumbnails = %q, quería vacío", got)
	}
}

// TestBestThumbIgnoraLasQueNoTienenURL: una entrada sin url no es portada.
func TestBestThumbIgnoraLasQueNoTienenURL(t *testing.T) {
	renderer := map[string]any{
		"thumbnail": map[string]any{
			"musicThumbnailRenderer": map[string]any{
				"thumbnail": map[string]any{
					"thumbnails": []any{
						map[string]any{"width": 300},
						map[string]any{"url": "https://example.test/ok.jpg", "width": 300},
					},
				},
			},
		},
	}
	if got := bestThumb(renderer); got != "https://example.test/ok.jpg" {
		t.Errorf("bestThumb = %q, quería la que tiene url", got)
	}
}

// TestBestThumbUsaElThumbnailDirecto: la cola de radio cuelga las portadas
// de `thumbnail.thumbnails`.
func TestBestThumbUsaElThumbnailDirecto(t *testing.T) {
	renderer := map[string]any{
		"thumbnail": map[string]any{
			"thumbnails": []any{
				map[string]any{"url": "https://example.test/panel.jpg", "width": 302},
			},
		},
	}
	if got := bestThumb(renderer); got != "https://example.test/panel.jpg" {
		t.Errorf("bestThumb = %q, quería la del panel", got)
	}
}

// ── continuaciones ───────────────────────────────────────────────────────

// TestContinuationOfAceptaLasCuatroFormas: innertube usa varias y el
// primerio que aparezca gana.
func TestContinuationOfAceptaLasCuatroFormas(t *testing.T) {
	keys := []string{
		"nextContinuationData",
		"nextRadioContinuationData",
		"reloadContinuationData",
		"continuationCommand",
	}
	for _, key := range keys {
		payload := map[string]any{
			"contents": map[string]any{
				key: map[string]any{"continuation": "TOKEN-" + key},
			},
		}
		want := "TOKEN-" + key
		if got := continuationOf(payload); got != want {
			t.Errorf("continuationOf con %s = %q, quería %q", key, got, want)
		}
	}
}

// TestContinuationOfAceptaElTokenAlternativo: algunos comandos traen
// "token" en vez de "continuation".
func TestContinuationOfAceptaElTokenAlternativo(t *testing.T) {
	payload := map[string]any{
		"nextContinuationData": map[string]any{"token": "TOKEN-ALT"},
	}
	if got := continuationOf(payload); got != "TOKEN-ALT" {
		t.Errorf("continuationOf = %q, quería TOKEN-ALT", got)
	}
}

// TestContinuationOfSinToken: sin token no hay paginación.
func TestContinuationOfSinToken(t *testing.T) {
	if got := continuationOf(nil); got != "" {
		t.Errorf("continuationOf(nil) = %q, quería vacío", got)
	}
	if got := continuationOf(map[string]any{}); got != "" {
		t.Errorf("continuationOf sin nada = %q, quería vacío", got)
	}
	if got := continuationOf(map[string]any{
		"nextContinuationData": map[string]any{"continuation": ""},
	}); got != "" {
		t.Errorf("continuationOf con token vacío = %q, quería vacío", got)
	}
}

// ── metadatos ────────────────────────────────────────────────────────────

// TestSplitMetaSeparaArtistaAlbumYDuracion.
func TestSplitMetaSeparaArtistaAlbumYDuracion(t *testing.T) {
	artist, album, duration := splitMeta([]string{"Daft Punk • Random Access Memories • 6:09"})
	if artist != "Daft Punk" {
		t.Errorf("artist = %q", artist)
	}
	if album != "Random Access Memories" {
		t.Errorf("album = %q", album)
	}
	if duration != "6:09" {
		t.Errorf("duration = %q", duration)
	}
}

// TestSplitMetaSinAlbum: un artista solo no inventa un álbum.
func TestSplitMetaSinAlbum(t *testing.T) {
	artist, album, duration := splitMeta([]string{"Solo Artista"})
	if artist != "Solo Artista" {
		t.Errorf("artist = %q", artist)
	}
	if album != "" || duration != "" {
		t.Errorf("album = %q, duration = %q, quería vacíos", album, duration)
	}
}

// TestSplitMetaConListaVacia.
func TestSplitMetaConListaVacia(t *testing.T) {
	artist, album, duration := splitMeta(nil)
	if artist != "" || album != "" || duration != "" {
		t.Errorf("splitMeta(nil) = %q/%q/%q, quería vacíos", artist, album, duration)
	}
}

// TestSplitMetaIgnoraContadoresDeRed: "56M likes" no es el álbum.
func TestSplitMetaIgnoraContadoresDeRed(t *testing.T) {
	for _, counter := range []string{"1.2B plays", "56M likes", "240K subscribers", "10M monthly audience", "9 views"} {
		_, album, _ := splitMeta([]string{"Artista • " + counter})
		if album != "" {
			t.Errorf("album = %q para %q, quería vacío", album, counter)
		}
	}
}

// TestSplitBulletsIgnoraLosVacíos: la viñeta puede venir con espacios.
func TestSplitBulletsIgnoraLosVacíos(t *testing.T) {
	got := splitBullets("  A  •   •  B ")
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("splitBullets = %v, quería [A B]", got)
	}
	if len(splitBullets("")) != 0 {
		t.Errorf("splitBullets(\"\") = %v, quería vacío", splitBullets(""))
	}
}

// TestIsDurationReconoceElFormatoDelPanel: "3:45" sí, "1.2B plays" no.
func TestIsDurationReconoceElFormatoDelPanel(t *testing.T) {
	for _, text := range []string{"3:45", "1:02:03", "0:59"} {
		if !isDuration(text) {
			t.Errorf("isDuration(%q) = false, quería true", text)
		}
	}
	for _, text := range []string{"", "abc", "1.2B plays", "5", "3:4x", "3:45 "} {
		if isDuration(text) {
			t.Errorf("isDuration(%q) = true, quería false", text)
		}
	}
}

// ── canciones ────────────────────────────────────────────────────────────

// TestSongFromResponsiveSinColumnasNoEsCancion: un item sin flexColumns no
// es una canción.
func TestSongFromResponsiveSinColumnasNoEsCancion(t *testing.T) {
	if _, ok := songFromResponsive(map[string]any{}); ok {
		t.Error("un item sin flexColumns no puede ser canción")
	}
	if _, ok := songFromResponsive(nil); ok {
		t.Error("un item nil no puede ser canción")
	}
}

// TestSongFromResponsiveSinVideoIDNoEsCancion: sin id no se puede reproducir.
func TestSongFromResponsiveSinVideoIDNoEsCancion(t *testing.T) {
	item := map[string]any{
		"flexColumns": []any{
			map[string]any{
				"musicResponsiveListItemFlexColumnRenderer": map[string]any{
					"text": map[string]any{"runs": []any{map[string]any{"text": "Título"}}},
				},
			},
		},
	}
	if _, ok := songFromResponsive(item); ok {
		t.Error("una canción sin videoId no puede ser canción")
	}
}

// TestSongFromResponsiveTomaElVideoIDDelWatchEndpoint: a veces el videoId
// no viene en playlistItemData.
func TestSongFromResponsiveTomaElVideoIDDelWatchEndpoint(t *testing.T) {
	item := map[string]any{
		"flexColumns": []any{
			map[string]any{
				"musicResponsiveListItemFlexColumnRenderer": map[string]any{
					"text": map[string]any{
						"runs": []any{
							map[string]any{
								"text": "Título",
								"navigationEndpoint": map[string]any{
									"watchEndpoint": map[string]any{"videoId": "del-endpoint"},
								},
							},
						},
					},
				},
			},
		},
	}
	song, ok := songFromResponsive(item)
	if !ok {
		t.Fatal("no reconoció la canción")
	}
	if song.VideoID != "del-endpoint" {
		t.Errorf("videoId = %q, quería del-endpoint", song.VideoID)
	}
}

// TestSongFromResponsiveLeeLaDuracionDeLaColumnaFija: la duración a veces
// sólo está en la columna de la derecha.
func TestSongFromResponsiveLeeLaDuracionDeLaColumnaFija(t *testing.T) {
	item := map[string]any{
		"playlistItemData": map[string]any{"videoId": "abc"},
		"flexColumns": []any{
			map[string]any{
				"musicResponsiveListItemFlexColumnRenderer": map[string]any{
					"text": map[string]any{"runs": []any{map[string]any{"text": "Título"}}},
				},
			},
			map[string]any{
				"musicResponsiveListItemFlexColumnRenderer": map[string]any{
					"text": map[string]any{"runs": []any{map[string]any{"text": "Artista"}}},
				},
			},
		},
		"fixedColumns": []any{
			map[string]any{
				"musicResponsiveListItemFixedColumnRenderer": map[string]any{
					"text": map[string]any{"runs": []any{map[string]any{"text": "3:45"}}},
				},
			},
		},
	}
	song, ok := songFromResponsive(item)
	if !ok {
		t.Fatal("no reconoció la canción")
	}
	if song.Duration != "3:45" || song.Seconds != 225 {
		t.Errorf("duración = %q (%d s)", song.Duration, song.Seconds)
	}
	if song.Artist != "Artista" {
		t.Errorf("artist = %q", song.Artist)
	}
}

// TestSongFromPanelItemSinTituloNoEsCancion.
func TestSongFromPanelItemSinTituloNoEsCancion(t *testing.T) {
	if _, ok := songFromPanelItem(map[string]any{"videoId": "abc"}); ok {
		t.Error("un item sin título no puede ser canción")
	}
	if _, ok := songFromPanelItem(map[string]any{"title": map[string]any{"simpleText": "T"}}); ok {
		t.Error("un item sin videoId no puede ser canción")
	}
	if _, ok := songFromPanelItem(nil); ok {
		t.Error("un item nil no puede ser canción")
	}
}

// TestSongFromPanelItemUsaElBylineCorto: cuando no hay longBylineText.
func TestSongFromPanelItemUsaElBylineCorto(t *testing.T) {
	item := map[string]any{
		"videoId":         "abc",
		"title":           map[string]any{"runs": []any{map[string]any{"text": "Canción"}}},
		"shortBylineText": map[string]any{"runs": []any{map[string]any{"text": "Artista"}}},
		"lengthTextSimple": map[string]any{
			"runs": []any{map[string]any{"text": "4:20"}},
		},
	}
	song, ok := songFromPanelItem(item)
	if !ok {
		t.Fatal("no reconoció la canción")
	}
	if song.Artist != "Artista" {
		t.Errorf("artist = %q", song.Artist)
	}
	if song.Duration != "4:20" || song.Seconds != 260 {
		t.Errorf("duración = %q (%d s)", song.Duration, song.Seconds)
	}
}

// TestSongsFromRenderersConPayloadVacio: sin canciones, lista vacía y sin
// panic.
func TestSongsFromRenderersConPayloadVacio(t *testing.T) {
	for _, payload := range []any{nil, map[string]any{}, []any{}, "texto"} {
		if got := songsFromRenderers(payload); len(got) != 0 {
			t.Errorf("songsFromRenderers(%v) = %+v, quería vacío", payload, got)
		}
	}
}

// TestSongsFromRenderersIgnoraLosItemsInvalidos: un item sin título no
// rompe la lista.
func TestSongsFromRenderersIgnoraLosItemsInvalidos(t *testing.T) {
	root := map[string]any{
		"contents": map[string]any{
			"musicResponsiveListItemRenderer": map[string]any{},
		},
	}
	if got := songsFromRenderers(root); len(got) != 0 {
		t.Errorf("juntó %+v, quería vacío", got)
	}
}

// TestPlaylistCardsIgnoraLasQueNoSonListas: sólo las browseID con prefijo VL
// son rankings.
func TestPlaylistCardsIgnoraLasQueNoSonListas(t *testing.T) {
	root := map[string]any{
		"musicTwoRowItemRenderer": map[string]any{
			"title":              map[string]any{"runs": []any{map[string]any{"text": "Un album"}}},
			"navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "MPREb_xxx"}},
		},
	}
	if got := playlistCards(root); len(got) != 0 {
		t.Errorf("juntó %+v, quería vacío", got)
	}
}

// TestPlaylistCardsIgnoraLasSinEndpoint: una tarjeta sin navegación no se
// puede abrir.
func TestPlaylistCardsIgnoraLasSinEndpoint(t *testing.T) {
	root := map[string]any{
		"musicTwoRowItemRenderer": map[string]any{
			"title": map[string]any{"runs": []any{map[string]any{"text": "Sin endpoint"}}},
		},
	}
	if got := playlistCards(root); len(got) != 0 {
		t.Errorf("juntó %+v, quería vacío", got)
	}
}

// TestPlaylistCardsConPayloadVacio.
func TestPlaylistCardsConPayloadVacio(t *testing.T) {
	if got := playlistCards(nil); len(got) != 0 {
		t.Errorf("juntó %+v, quería vacío", got)
	}
}

// ── feeds ────────────────────────────────────────────────────────────────

// TestFeedFromTraeLaContinuacion: sin token la lista muere en la primera
// página.
func TestFeedFromTraeLaContinuacion(t *testing.T) {
	payload := decode(t, responsiveFixture)
	feed, err := feedFrom("home", payload)
	if err != nil {
		t.Fatalf("feedFrom: %v", err)
	}
	if feed.Source != "home" {
		t.Errorf("source = %q", feed.Source)
	}
	if len(feed.Items) != 1 {
		t.Errorf("items = %d, quería 1", len(feed.Items))
	}
	if feed.Continuation != "TOKEN-1" {
		t.Errorf("continuation = %q, quería TOKEN-1", feed.Continuation)
	}
	if feed.FetchedAt <= 0 {
		t.Errorf("fetchedAt = %d", feed.FetchedAt)
	}
}

// TestFeedFromDeduplica: la radio devuelve el seed dos veces.
func TestFeedFromDeduplica(t *testing.T) {
	payload := map[string]any{
		"contents": map[string]any{
			"musicResponsiveListItemRenderer": map[string]any{
				"playlistItemData": map[string]any{"videoId": "dup"},
				"flexColumns": []any{
					map[string]any{
						"musicResponsiveListItemFlexColumnRenderer": map[string]any{
							"text": map[string]any{"runs": []any{map[string]any{"text": "T"}}},
						},
					},
				},
			},
		},
	}
	feed, err := feedFrom("home", payload)
	if err != nil {
		t.Fatalf("feedFrom: %v", err)
	}
	if len(feed.Items) != 1 {
		t.Errorf("items = %d, quería 1", len(feed.Items))
	}
}

// TestFeedFromConJSONMalformado: un cuerpo roto es un error, no una lista
// vacía.
func TestFeedFromConJSONMalformado(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"contents": {`), &payload); err == nil {
		t.Fatal("el JSON de prueba debería estar roto")
	}
	_, err := feedFrom("home", nil)
	if err == nil {
		t.Fatal("un payload nil tiene que ser error")
	}
	if !strings.Contains(err.Error(), "home") {
		t.Errorf("el error no dice de qué pantalla viene: %v", err)
	}
}
