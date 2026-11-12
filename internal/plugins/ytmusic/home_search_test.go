package ytmusic

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestHomeTraeLasCancionesDeLaPagina.
func TestHomeTraeLasCancionesDeLaPagina(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, shelfPayload([]any{
			songRenderer("v1", "One", "Artista Uno • 3:45"),
			songRenderer("v2", "Two", "Artista Dos • 4:00"),
		}, "Quick picks", "TOKEN-HOME"))
	})
	feed, err := Home(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if feed.Source != "home" {
		t.Errorf("source = %q", feed.Source)
	}
	if len(feed.Items) != 2 {
		t.Errorf("items = %d, quería 2", len(feed.Items))
	}
	if feed.Continuation != "TOKEN-HOME" {
		t.Errorf("continuation = %q", feed.Continuation)
	}
	if asString(fake.body(0)["browseId"]) != "FEmusic_home" {
		t.Errorf("browseId = %q, quería FEmusic_home", asString(fake.body(0)["browseId"]))
	}
}

// TestHomeConContinuacionSigueLaPaginaSiguiente.
func TestHomeConContinuacionSigueLaPaginaSiguiente(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, shelfPayload([]any{
			songRenderer("v2", "Two", "Artista • 4:00"),
		}, "Más", ""))
	})
	feed, err := Home(context.Background(), fake.client, "TOKEN-HOME")
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if len(feed.Items) != 1 {
		t.Errorf("items = %d, quería 1", len(feed.Items))
	}
	if asString(fake.body(0)["continuation"]) != "TOKEN-HOME" {
		t.Errorf("continuation = %q", asString(fake.body(0)["continuation"]))
	}
}

// TestHomeExpandeLaPrimeraPlaylist: una página de inicio anónima trae pocas
// estanterías de canciones y muchas de playlists.
func TestHomeExpandeLaPrimeraPlaylist(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		browseID, _ := body["browseId"].(string)
		if browseID == "FEmusic_home" {
			jsonResponse(w, map[string]any{
				"contents": map[string]any{
					"sectionListRenderer": map[string]any{
						"contents": []any{
							map[string]any{
								"musicCarouselShelfRenderer": map[string]any{
									"contents": []any{cardPayload("Mi Mix", "VLmix")},
								},
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, shelfPayload([]any{
			songRenderer("v1", "One", "Artista • 3:45"),
		}, "Mi Mix", ""))
	})
	feed, err := Home(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if feed.Source != "playlist:VLmix" {
		t.Errorf("source = %q, quería playlist:VLmix", feed.Source)
	}
	if feed.Title != "Mi Mix" {
		t.Errorf("title = %q", feed.Title)
	}
	if len(feed.Items) != 1 {
		t.Errorf("items = %d, quería 1", len(feed.Items))
	}
}

// TestHomeSaltaLaTarjetaQueFalla: si la primera playlist no abre, se prueba la
// siguiente antes de rendirse.
func TestHomeSaltaLaTarjetaQueFalla(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		browseID, _ := body["browseId"].(string)
		switch browseID {
		case "FEmusic_home":
			jsonResponse(w, map[string]any{
				"contents": map[string]any{
					"sectionListRenderer": map[string]any{
						"contents": []any{
							map[string]any{
								"musicCarouselShelfRenderer": map[string]any{
									"contents": []any{
										cardPayload("Rota", "VLrota"),
										cardPayload("Buena", "VLbuena"),
									},
								},
							},
						},
					},
				},
			})
		case "VLrota":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no existe"}`))
		default:
			jsonResponse(w, shelfPayload([]any{
				songRenderer("v1", "One", "Artista • 3:45"),
			}, "Buena", ""))
		}
	})
	feed, err := Home(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if feed.Source != "playlist:VLbuena" {
		t.Errorf("source = %q, quería playlist:VLbuena", feed.Source)
	}
}

// TestHomeSinCancionesNiTarjetasEsError: un "0 canciones" se lee como plugin
// roto.
func TestHomeSinCancionesNiTarjetasEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	if _, err := Home(context.Background(), fake.client, ""); err == nil {
		t.Fatal("una página vacía tiene que ser error")
	}
}

// TestHomeConContinuacionVaciaEsError: la continuación se informa, no se
// muestra entera.
func TestHomeConContinuacionVaciaEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	_, err := Home(context.Background(), fake.client, "TOKEN-MUY-LARGO-PARA-EL-PANEL")
	if err == nil {
		t.Fatal("una continuación vacía tiene que ser error")
	}
	if !strings.Contains(err.Error(), "TOKEN-MU") {
		t.Errorf("el error no muestra un fragmento del token: %v", err)
	}
}

// TestShelfTitlesJuntaHastaTres: el subtítulo del panel no puede ser un muro.
func TestShelfTitlesJuntaHastaTres(t *testing.T) {
	shelves := []any{}
	for _, name := range []string{"Estante 1", "Estante 2", "Estante 3", "Estante 4"} {
		shelves = append(shelves, map[string]any{
			"musicCarouselShelfRenderer": map[string]any{
				"header": map[string]any{
					"musicCarouselShelfBasicHeaderRenderer": map[string]any{
						"title": map[string]any{"runs": []any{map[string]any{"text": name}}},
					},
				},
			},
		})
	}
	payload := map[string]any{
		"contents": map[string]any{
			"sectionListRenderer": map[string]any{
				"contents": shelves,
			},
		},
	}
	got := shelfTitles(payload)
	if got != "Estante 1 · Estante 2 · Estante 3" {
		t.Errorf("shelfTitles = %q, quería los tres primeros", got)
	}
}

// TestShelfTitlesIgnoraLasEstanteriasSinTitulo: una estantería sin header no
// aporta nada al subtítulo.
func TestShelfTitlesIgnoraLasEstanteriasSinTitulo(t *testing.T) {
	payload := map[string]any{
		"contents": map[string]any{
			"sectionListRenderer": map[string]any{
				"contents": []any{
					map[string]any{"musicCarouselShelfRenderer": map[string]any{}},
					map[string]any{
						"musicCarouselShelfRenderer": map[string]any{
							"header": map[string]any{
								"musicCarouselShelfBasicHeaderRenderer": map[string]any{
									"title": map[string]any{"runs": []any{map[string]any{"text": "Con título"}}},
								},
							},
						},
					},
				},
			},
		},
	}
	if got := shelfTitles(payload); got != "Con título" {
		t.Errorf("shelfTitles = %q, quería sólo la que tiene título", got)
	}
}

// TestShelfTitlesSinEstanterias.
func TestShelfTitlesSinEstanterias(t *testing.T) {
	if got := shelfTitles(map[string]any{}); got != "" {
		t.Errorf("shelfTitles = %q, quería vacío", got)
	}
}

// TestShortTokenRecortaElToken.
func TestShortTokenRecortaElToken(t *testing.T) {
	if got := shortToken("corto"); got != "corto" {
		t.Errorf("shortToken = %q, quería corto", got)
	}
	got := shortToken("TOKEN-MUY-LARGO-DE-VERDAD")
	if got != "TOKEN-MUY-LA…" {
		t.Errorf("shortToken = %q, quería el token cortado", got)
	}
	if len([]rune(got)) != 13 {
		t.Errorf("shortToken quedó en %d caracteres, quería 13", len([]rune(got)))
	}
}

// ── búsqueda ─────────────────────────────────────────────────────────────

// TestSearchTraeLosResultados.
func TestSearchTraeLosResultados(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, shelfPayload([]any{
			songRenderer("v1", "One", "Artista • 3:45"),
		}, "Resultados", "TOKEN-SEARCH"))
	})
	feed, err := Search(context.Background(), fake.client, "daft punk", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if feed.Source != "search" {
		t.Errorf("source = %q", feed.Source)
	}
	if feed.Title != "daft punk" {
		t.Errorf("title = %q, quería la consulta", feed.Title)
	}
	if len(feed.Items) != 1 {
		t.Errorf("items = %d, quería 1", len(feed.Items))
	}
	if feed.Continuation != "TOKEN-SEARCH" {
		t.Errorf("continuation = %q", feed.Continuation)
	}
}

// TestSearchSinConsultaEsError.
func TestSearchSinConsultaEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar la red sin consulta")
	})
	if _, err := Search(context.Background(), fake.client, "   ", ""); err == nil {
		t.Fatal("una consulta vacía tiene que ser error")
	}
}

// TestSearchSinResultadosEsError: una búsqueda sin hits es un error, no una
// pantalla vacía.
func TestSearchSinResultadosEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	_, err := Search(context.Background(), fake.client, "xyz", "")
	if err == nil {
		t.Fatal("una búsqueda sin resultados tiene que ser error")
	}
	if !strings.Contains(err.Error(), "xyz") {
		t.Errorf("el error no nombra la consulta: %v", err)
	}
}

// TestSearchEnLaPaginaSiguienteVaciaEsError: el mensaje distingue "no hay
// resultados" de "la página siguiente vino vacía".
func TestSearchEnLaPaginaSiguienteVaciaEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	_, err := Search(context.Background(), fake.client, "xyz", "TOKEN-2")
	if err == nil {
		t.Fatal("una página siguiente vacía tiene que ser error")
	}
	if !strings.Contains(err.Error(), "página siguiente") {
		t.Errorf("el error no distingue la página siguiente: %v", err)
	}
}
