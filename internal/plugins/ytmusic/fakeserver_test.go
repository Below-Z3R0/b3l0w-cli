package ytmusic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// fakeInnertube reemplaza a music.youtube.com por un servidor local.
//
// Las URLs de innertube son constantes del paquete, así que no hay puntero de
// inyección: lo que se intercepta es el transporte HTTP. El cliente real
// (headers, reintentos, límite de 32 MB, playability) se ejercita tal cual,
// pero ningún test sale a la red.
type fakeInnertube struct {
	t      *testing.T
	server *httptest.Server
	client *Client

	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
}

// handler recibe el cuerpo ya decodificado: los tests deciden qué responder
// según el browseId, el continuation o el videoId que pidieron.
type handler func(w http.ResponseWriter, r *http.Request, body map[string]any)

func newFakeInnertube(t *testing.T, respond handler) *fakeInnertube {
	t.Helper()
	fake := &fakeInnertube{t: t}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("leyendo el cuerpo de la petición: %v", err)
			return
		}
		body := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("el cuerpo de la petición no es JSON: %v", err)
			}
		}
		fake.mu.Lock()
		fake.paths = append(fake.paths, r.URL.Path)
		fake.bodies = append(fake.bodies, body)
		fake.mu.Unlock()
		respond(w, r, body)
	}))
	t.Cleanup(fake.server.Close)

	base, err := url.Parse(fake.server.URL)
	if err != nil {
		t.Fatalf("URL del servidor de prueba: %v", err)
	}
	fake.client = &Client{
		HTTP: &http.Client{
			Timeout:   5 * time.Second,
			Transport: rewriteTransport{base: base},
		},
		HL: "en",
		GL: "US",
	}
	return fake
}

// rewriteTransport manda a otro host lo que el plugin le pide a innertube.
type rewriteTransport struct {
	base *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// calls devuelve cuántas peticiones se hicieron, en orden.
func (f *fakeInnertube) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.paths))
	copy(out, f.paths)
	return out
}

// body devuelve el cuerpo JSON de la petición número index (0-based).
func (f *fakeInnertube) body(index int) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if index < 0 || index >= len(f.bodies) {
		f.t.Fatalf("petición %d inexistente (hubo %d)", index, len(f.bodies))
	}
	return f.bodies[index]
}

// json responde con un cuerpo JSON y status 200.
func jsonResponse(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	raw, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(raw)
}

// ── constructores de respuestas ──────────────────────────────────────────

// songRenderer arma un musicResponsiveListItemRenderer: la forma de charts,
// search y las estanterías de home.
func songRenderer(id, title, meta string) map[string]any {
	return map[string]any{
		"musicResponsiveListItemRenderer": map[string]any{
			"playlistItemData": map[string]any{"videoId": id},
			"flexColumns": []any{
				map[string]any{
					"musicResponsiveListItemFlexColumnRenderer": map[string]any{
						"text": map[string]any{"runs": []any{map[string]any{"text": title}}},
					},
				},
				map[string]any{
					"musicResponsiveListItemFlexColumnRenderer": map[string]any{
						"text": map[string]any{"runs": []any{map[string]any{"text": meta}}},
					},
				},
			},
			"thumbnail": map[string]any{
				"musicThumbnailRenderer": map[string]any{
					"thumbnail": map[string]any{
						"thumbnails": []any{
							map[string]any{"url": "https://example.test/" + id + ".jpg", "width": 226},
						},
					},
				},
			},
		},
	}
}

// panelSong arma un playlistPanelVideoRenderer: la forma de la cola de radio.
func panelSong(id, title, byline, length string) map[string]any {
	return map[string]any{
		"playlistPanelVideoRenderer": map[string]any{
			"videoId":        id,
			"title":          map[string]any{"runs": []any{map[string]any{"text": title}}},
			"longBylineText": map[string]any{"runs": []any{map[string]any{"text": byline}}},
			"lengthText":     map[string]any{"runs": []any{map[string]any{"text": length}}},
		},
	}
}

// radioPayload arma el watch-next con una cola de canciones y, si se pasa,
// el token de continuación.
func radioPayload(songs []any, continuation string) map[string]any {
	contents := []any{}
	for _, song := range songs {
		contents = append(contents, song)
	}
	continuations := []any{}
	if continuation != "" {
		continuations = append(continuations, map[string]any{
			"nextRadioContinuationData": map[string]any{"continuation": continuation},
		})
	}
	return map[string]any{
		"contents": map[string]any{
			"singleColumnMusicWatchNextResultsRenderer": map[string]any{
				"tabbedRenderer": map[string]any{
					"watchNextTabbedResultsRenderer": map[string]any{
						"playlistPanelRenderer": map[string]any{
							"contents":      contents,
							"continuations": continuations,
						},
					},
				},
			},
		},
	}
}

// shelfPayload arma una estantería de canciones (charts, search, home).
func shelfPayload(songs []any, title string, continuation string) map[string]any {
	contents := []any{}
	for _, song := range songs {
		contents = append(contents, song)
	}
	shelf := map[string]any{
		"musicShelfRenderer": map[string]any{
			"title":    map[string]any{"runs": []any{map[string]any{"text": title}}},
			"contents": contents,
		},
	}
	if continuation != "" {
		shelf["musicShelfRenderer"].(map[string]any)["continuations"] = []any{
			map[string]any{"nextContinuationData": map[string]any{"continuation": continuation}},
		}
	}
	return map[string]any{
		"contents": map[string]any{
			"sectionListRenderer": map[string]any{
				"contents": []any{shelf},
			},
		},
	}
}

// cardPayload arma una tarjeta de playlist (charts y home).
func cardPayload(title, browseID string) map[string]any {
	return map[string]any{
		"musicTwoRowItemRenderer": map[string]any{
			"title": map[string]any{"runs": []any{map[string]any{"text": title}}},
			"navigationEndpoint": map[string]any{
				"browseEndpoint": map[string]any{"browseId": browseID},
			},
		},
	}
}
