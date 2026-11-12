package ytmusic

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestRankSongsNumeraDesdeUno: el panel muestra el puesto sin recalcularlo.
func TestRankSongsNumeraDesdeUno(t *testing.T) {
	songs := []Song{{VideoID: "a"}, {VideoID: "b"}, {VideoID: "c"}}
	rankSongs(songs)
	for index, song := range songs {
		if song.ChartRank != index+1 {
			t.Errorf("p[%d].ChartRank = %d, quería %d", index, song.ChartRank, index+1)
		}
	}
}

// TestRankSongsConListaVacia: no hay puestos que dar.
func TestRankSongsConListaVacia(t *testing.T) {
	songs := []Song{}
	rankSongs(songs)
	if len(songs) != 0 {
		t.Errorf("la lista cambió: %+v", songs)
	}
}

// TestChartsTitleUsaElPaisDeLaPagina: "Charts · Panama".
func TestChartsTitleUsaElPaisDeLaPagina(t *testing.T) {
	payload := map[string]any{
		"header": map[string]any{
			"musicHeaderRenderer": map[string]any{
				"title": map[string]any{"runs": []any{map[string]any{"text": "Charts"}}},
			},
		},
		"filter": map[string]any{
			"musicSortFilterButtonRenderer": map[string]any{
				"title": map[string]any{"runs": []any{map[string]any{"text": "Panama"}}},
			},
		},
	}
	if got := chartsTitle(payload); got != "Charts · Panama" {
		t.Errorf("chartsTitle = %q, quería %q", got, "Charts · Panama")
	}
}

// TestChartsTitleSinPais: sin filtro de país el título queda corto.
func TestChartsTitleSinPais(t *testing.T) {
	payload := map[string]any{
		"header": map[string]any{
			"musicHeaderRenderer": map[string]any{
				"title": map[string]any{"simpleText": "Charts"},
			},
		},
	}
	if got := chartsTitle(payload); got != "Charts" {
		t.Errorf("chartsTitle = %q, quería %q", got, "Charts")
	}
}

// TestChartsTitleSinHeaderDevuelveCharts: nunca devolver vacío, el panel
// muestra el título tal cual.
func TestChartsTitleSinHeaderDevuelveCharts(t *testing.T) {
	if got := chartsTitle(map[string]any{}); got != "Charts" {
		t.Errorf("chartsTitle = %q, quería %q", got, "Charts")
	}
}

// TestChartsUsaLasCancionesDeLaPagina: si la página ya trae canciones, se
// publican y no se abre ninguna tarjeta.
func TestChartsUsaLasCancionesDeLaPagina(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"contents": map[string]any{
				"sectionListRenderer": map[string]any{
					"contents": []any{
						map[string]any{
							"musicShelfRenderer": map[string]any{
								"title": map[string]any{"runs": []any{map[string]any{"text": "Trending"}}},
								"contents": []any{
									songRenderer("v1", "One", "Artista Uno • 3:45"),
									songRenderer("v2", "Two", "Artista Dos • 4:00"),
								},
							},
						},
					},
				},
			},
		})
	})
	feed, err := Charts(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Charts: %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("trajo %d canciones, quería 2", len(feed.Items))
	}
	if feed.Source != "charts" {
		t.Errorf("source = %q, quería %q", feed.Source, "charts")
	}
	if feed.Items[0].ChartRank != 1 || feed.Items[1].ChartRank != 2 {
		t.Errorf("los puestos no se numeraron: %+v", feed.Items)
	}
	if feed.Items[0].Title != "One" || feed.Items[0].Seconds != 225 {
		t.Errorf("primera canción = %+v", feed.Items[0])
	}
	if len(fake.calls()) != 1 {
		t.Errorf("hizo %d peticiones, quería 1 (no debía abrir tarjetas)", len(fake.calls()))
	}
}

// TestChartsAbreLaTarjetaTop100: las estanterías de videos vienen primero en
// la página; quedarse con la primera devuelve el chart equivocado.
func TestChartsAbreLaTarjetaTop100(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		browseID, _ := body["browseId"].(string)
		if browseID != "FEmusic_charts" {
			jsonResponse(w, map[string]any{
				"contents": map[string]any{
					"sectionListRenderer": map[string]any{
						"contents": []any{
							map[string]any{
								"musicShelfRenderer": map[string]any{
									"contents": []any{
										songRenderer("chart1", "Chart One", "Artista • 3:00"),
									},
								},
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, map[string]any{
			"contents": map[string]any{
				"sectionListRenderer": map[string]any{
					"contents": []any{
						map[string]any{
							"musicCarouselShelfRenderer": map[string]any{
								"contents": []any{
									cardPayload("Trending 20 Panama", "VLtrending"),
									cardPayload("Top 100 Music Videos Panama", "VLtop100"),
									cardPayload("Top 50 Songs Panama", "VLtop50"),
								},
							},
						},
					},
				},
			},
		})
	})
	feed, err := Charts(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Charts: %v", err)
	}
	if feed.Source != "charts:VLtop100" {
		t.Errorf("source = %q, quería charts:VLtop100", feed.Source)
	}
	if feed.Title != "Top 100 Music Videos Panama" {
		t.Errorf("title = %q", feed.Title)
	}
	if len(feed.Items) != 1 || feed.Items[0].Title != "Chart One" {
		t.Errorf("items = %+v", feed.Items)
	}
	if len(fake.calls()) != 2 {
		t.Errorf("hizo %d peticiones, quería 2 (la página y la tarjeta)", len(fake.calls()))
	}
}

// TestChartsPrefiereTop50SobreTop100CuandoNoHayTop100: el score ordena por
// completitud del ranking.
func TestChartsPrefiereTop50SobreTop100CuandoNoHayTop100(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		browseID, _ := body["browseId"].(string)
		if browseID == "FEmusic_charts" {
			jsonResponse(w, map[string]any{
				"contents": map[string]any{
					"sectionListRenderer": map[string]any{
						"contents": []any{
							map[string]any{
								"musicCarouselShelfRenderer": map[string]any{
									"contents": []any{
										cardPayload("Trending 20 Panama", "VLtrending"),
										cardPayload("Top 50 Songs Panama", "VLtop50"),
									},
								},
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, map[string]any{
			"contents": map[string]any{
				"sectionListRenderer": map[string]any{
					"contents": []any{
						map[string]any{
							"musicShelfRenderer": map[string]any{
								"contents": []any{songRenderer("c1", "Canción", "Artista • 3:00")},
							},
						},
					},
				},
			},
		})
	})
	feed, err := Charts(context.Background(), fake.client, "")
	if err != nil {
		t.Fatalf("Charts: %v", err)
	}
	if feed.Source != "charts:VLtop50" {
		t.Errorf("source = %q, quería charts:VLtop50", feed.Source)
	}
}

// TestChartsSinCancionesNiTarjetasEsError: un chart vacío no es un chart.
func TestChartsSinCancionesNiTarjetasEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	if _, err := Charts(context.Background(), fake.client, ""); err == nil {
		t.Fatal("una página sin canciones ni tarjetas tiene que ser error")
	}
}

// TestChartsConTarjetaVaciaEsError: la tarjeta se abrió pero no trajo nada.
func TestChartsConTarjetaVaciaEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		browseID, _ := body["browseId"].(string)
		if browseID == "FEmusic_charts" {
			jsonResponse(w, map[string]any{
				"contents": map[string]any{
					"sectionListRenderer": map[string]any{
						"contents": []any{
							map[string]any{
								"musicCarouselShelfRenderer": map[string]any{
									"contents": []any{cardPayload("Top 100 Panama", "VLtop100")},
								},
							},
						},
					},
				},
			})
			return
		}
		jsonResponse(w, map[string]any{"contents": map[string]any{}})
	})
	_, err := Charts(context.Background(), fake.client, "")
	if err == nil {
		t.Fatal("una tarjeta sin canciones tiene que ser error")
	}
	if !strings.Contains(err.Error(), "Top 100 Panama") {
		t.Errorf("el error no nombra el ranking: %v", err)
	}
}

// TestChartsPropagaElErrorDeRed: un fallo del browse no se disfraza de chart
// vacío.
func TestChartsPropagaElErrorDeRed(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400}}`))
	})
	_, err := Charts(context.Background(), fake.client, "")
	if err == nil {
		t.Fatal("un HTTP 400 tiene que ser error")
	}
	if !strings.Contains(err.Error(), "charts") {
		t.Errorf("el error no dice de dónde viene: %v", err)
	}
}

// TestChartsIgnoraElPaisUniversal: "ZZ" es el valor global de YouTube; no
// cambia el GL del cliente.
func TestChartsIgnoraElPaisUniversal(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"contents": map[string]any{
				"sectionListRenderer": map[string]any{
					"contents": []any{
						map[string]any{
							"musicShelfRenderer": map[string]any{
								"contents": []any{songRenderer("v1", "One", "Artista • 3:45")},
							},
						},
					},
				},
			},
		})
	})
	client := fake.client
	client.GL = "US"
	if _, err := Charts(context.Background(), client, "ZZ"); err != nil {
		t.Fatalf("Charts: %v", err)
	}
	if client.GL != "US" {
		t.Errorf("GL = %q, quería US (el país universal no se manda)", client.GL)
	}
}

// TestChartsFijaElPaisEnElCliente: el país se manda en mayúsculas.
func TestChartsFijaElPaisEnElCliente(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"contents": map[string]any{
				"sectionListRenderer": map[string]any{
					"contents": []any{
						map[string]any{
							"musicShelfRenderer": map[string]any{
								"contents": []any{songRenderer("v1", "One", "Artista • 3:45")},
							},
						},
					},
				},
			},
		})
	})
	client := fake.client
	client.GL = "US"
	if _, err := Charts(context.Background(), client, "pa"); err != nil {
		t.Fatalf("Charts: %v", err)
	}
	if client.GL != "PA" {
		t.Errorf("GL = %q, quería PA", client.GL)
	}
}
