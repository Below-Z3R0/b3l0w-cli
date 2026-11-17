package anilist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// rewriteTransport manda las consultas del cliente al servidor de prueba:
// el Endpoint del cliente es una constante, así que hay que redirigirla.
type rewriteTransport struct{ target *url.URL }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

// serverClient devuelve un cliente que habla con el servidor de prueba.
func serverClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsear la URL del servidor: %v", err)
	}
	c := NewClient("token-de-prueba")
	c.Retry = time.Millisecond
	c.HTTP = &http.Client{Transport: rewriteTransport{target: target}}
	return c
}

// requestBody lee la query y las variables que mandó el cliente.
func requestBody(t *testing.T, r *http.Request) (string, map[string]any) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("leer el cuerpo: %v", err)
	}
	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("el cuerpo no es GraphQL: %v", err)
	}
	return payload.Query, payload.Variables
}

func TestFetchListPaginaYMergeaPorMediaID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, vars := requestBody(t, r)
		if vars["chunk"] == float64(0) {
			w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":true,"lists":[{"entries":[
				{"mediaId":1,"status":"CURRENT","progress":5,"score":80,"updatedAt":100,"media":{"episodes":12,"coverImage":{"color":"#abc","extraLarge":"http://c/1a.jpg"},"title":{"userPreferred":"Zeta"},"genres":["Action"]}},
				{"mediaId":2,"status":"COMPLETED","progress":24,"score":90,"updatedAt":200,"media":{"episodes":24,"coverImage":{"large":"http://c/2.jpg"},"title":{"userPreferred":"Alfa"},"genres":["Drama"]}}
			]}]}}}`))
			return
		}
		w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
			{"mediaId":1,"status":"COMPLETED","progress":12,"score":95,"updatedAt":300,"media":{"episodes":12,"coverImage":{"extraLarge":"http://c/1b.jpg"},"title":{"userPreferred":"Zeta"},"genres":["Action"]}},
			{"mediaId":3,"status":"CURRENT","progress":1,"score":0,"updatedAt":50,"media":{"episodes":13,"coverImage":{"extraLarge":"http://c/3.jpg"},"title":{"userPreferred":"Mid"},"genres":[]}}
		]}]}}}`))
	}))
	defer srv.Close()

	entries, err := FetchList(context.Background(), serverClient(t, srv), 123, "ANIME")
	if err != nil {
		t.Fatalf("FetchList: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entradas = %d; quería 3 (la repetida se mergea)", len(entries))
	}
	want := []string{"Alfa", "Mid", "Zeta"}
	for i, title := range want {
		if entries[i].Title != title {
			t.Errorf("entradas[%d] = %q; quería %q", i, entries[i].Title, title)
		}
	}
	// La misma media en dos chunks: gana la más reciente.
	if entries[2].Progress != 12 || entries[2].Score != 95 {
		t.Errorf("no se quedó con la entrada más reciente: %+v", entries[2])
	}
	if entries[2].CoverURL != "http://c/1b.jpg" {
		t.Errorf("coverUrl = %q; quería la de la entrada más reciente", entries[2].CoverURL)
	}
	if entries[0].Total != 24 {
		t.Errorf("total = %d; quería los 24 episodios", entries[0].Total)
	}
	if entries[2].Genres == nil {
		t.Error("géneros nil: el plugin Luau cuenta sobre el array")
	}
}

func TestSyncMarcaParcialSiUnaListaFalla(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, vars := requestBody(t, r)
		switch {
		case strings.Contains(query, "Viewer"):
			w.Write([]byte(`{"data":{"Viewer":{"id":42,"name":"b3low"}}}`))
		case vars["type"] == "ANIME":
			w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
				{"mediaId":1,"status":"CURRENT","progress":1,"updatedAt":10,"media":{"episodes":12,"title":{"userPreferred":"Alfa"},"genres":[]}}
			]}]}}}`))
		default:
			w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
		}
	}))
	defer srv.Close()

	lib, err := Sync(context.Background(), serverClient(t, srv), 0)
	if err != nil {
		t.Fatalf("Sync no debe tirar abajo la lista que sí vino: %v", err)
	}
	if !lib.Partial {
		t.Error("partial = false; quería true (la lista de manga falló)")
	}
	if len(lib.Anime) != 1 {
		t.Errorf("anime = %d; quería 1", len(lib.Anime))
	}
	if len(lib.Manga) != 0 {
		t.Errorf("manga = %d; quería 0", len(lib.Manga))
	}
	if lib.Viewer.Name != "b3low" || lib.Viewer.ID != 42 {
		t.Errorf("viewer = %+v; quería b3low #42", lib.Viewer)
	}
}
