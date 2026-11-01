package anilist

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// statsBody es una respuesta real de AniList recortada: seis géneros para
// comprobar el corte en cinco y notas desordenadas.
const statsBody = `{"data":{"User":{"name":"b3low","statistics":{"anime":{"count":334,"episodesWatched":4210,"minutesWatched":101040,"meanScore":78.44,"genres":[{"genre":"Action","count":88},{"genre":"Drama","count":42},{"genre":"Comedy","count":61},{"genre":"Fantasy","count":55},{"genre":"Sci-Fi","count":55},{"genre":"Romance","count":30}],"scores":[{"score":10,"count":12},{"score":7,"count":40},{"score":8,"count":120}]},"manga":{"count":12,"chaptersRead":300,"volumesRead":30,"meanScore":72}}}}}`

const viewerBody = `{"data":{"Viewer":{"id":123,"name":"b3low"}}}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// fakeClient responde las queries con `respond` y no toca la red: el test corre
// sin token real y sin depender del uptime de AniList.
func fakeClient(t *testing.T, respond func(query string, variables map[string]any) (string, int)) *Client {
	t.Helper()
	c := NewClient("token-de-prueba")
	c.Retry = time.Millisecond
	c.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("el cuerpo enviado no es GraphQL: %v", err)
			return nil, err
		}
		body, status := respond(payload.Query, payload.Variables)
		if status == 0 {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	return c
}

// offlineClient falla el test si algo sale a la API: sirve para probar que el
// caché fresco se sirve de verdad desde disco.
func offlineClient(t *testing.T) *Client {
	t.Helper()
	return fakeClient(t, func(query string, _ map[string]any) (string, int) {
		t.Errorf("no debía consultar la API:\\n%s", query)
		return `{"errors":[{"message":"no esperado"}]}`, http.StatusOK
	})
}

func writeCache(t *testing.T, dir string, stats Stats) {
	t.Helper()
	if err := writeStatsCache(dir, stats); err != nil {
		t.Fatalf("escribir caché: %v", err)
	}
}

func TestFetchStatsTraduceYOrdena(t *testing.T) {
	c := fakeClient(t, func(string, map[string]any) (string, int) { return statsBody, 0 })
	stats, err := FetchStats(context.Background(), c, "b3low")
	if err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	if stats.User != "b3low" {
		t.Errorf("user = %q, quería b3low", stats.User)
	}
	if stats.Anime.Count != 334 || stats.Anime.EpisodesWatched != 4210 || stats.Anime.MinutesWatched != 101040 {
		t.Errorf("totales de anime mal traducidos: %+v", stats.Anime)
	}
	if stats.Anime.DaysWatched != 70.2 {
		t.Errorf("daysWatched = %v, quería 70.2 (un decimal)", stats.Anime.DaysWatched)
	}
	if stats.Anime.MeanScore != 78.4 {
		t.Errorf("meanScore = %v, quería 78.4", stats.Anime.MeanScore)
	}
	wantGenres := []GenreStat{
		{"Action", 88}, {"Comedy", 61}, {"Fantasy", 55}, {"Sci-Fi", 55}, {"Drama", 42},
	}
	if len(stats.Anime.Genres) != len(wantGenres) {
		t.Fatalf("genres = %+v, quería top 5 (sin Romance)", stats.Anime.Genres)
	}
	for i, want := range wantGenres {
		if stats.Anime.Genres[i] != want {
			t.Errorf("genres[%d] = %+v, quería %+v", i, stats.Anime.Genres[i], want)
		}
	}
	wantScores := []ScoreStat{{7, 40}, {8, 120}, {10, 12}}
	if len(stats.Anime.Scores) != len(wantScores) {
		t.Fatalf("scores = %+v, quería %+v", stats.Anime.Scores, wantScores)
	}
	for i, want := range wantScores {
		if stats.Anime.Scores[i] != want {
			t.Errorf("scores[%d] = %+v, quería %+v", i, stats.Anime.Scores[i], want)
		}
	}
	if stats.Manga.Count != 12 || stats.Manga.ChaptersRead != 300 || stats.Manga.VolumesRead != 30 || stats.Manga.MeanScore != 72 {
		t.Errorf("totales de manga mal traducidos: %+v", stats.Manga)
	}
	if stats.FetchedAt <= 0 {
		t.Errorf("fetchedAt sin setear: %d", stats.FetchedAt)
	}
}

const statsVacioBody = `{"data":{"User":{"name":"b3low","statistics":{"anime":{"count":3,"episodesWatched":30,"minutesWatched":720,"meanScore":90,"genres":[],"scores":[]},"manga":{"count":0,"chaptersRead":0,"volumesRead":0,"meanScore":0}}}}}`

func TestStatsFallbackDesdeLaLista(t *testing.T) {
	// La API devuelve count pero los agregados vacíos (le pasa a muchas
	// cuentas): hay que recalcular desde la lista.
	c := fakeClient(t, func(query string, variables map[string]any) (string, int) {
		if strings.Contains(query, "statistics") {
			return statsVacioBody, 0
		}
		switch variables["chunk"] {
		case float64(0):
			return `{"data":{"MediaListCollection":{"hasNextChunk":true,"lists":[{"entries":[
				{"mediaId":1,"status":"COMPLETED","score":8,"media":{"genres":["Action","Comedy"]}},
				{"mediaId":2,"status":"PLANNING","score":0,"media":{"genres":["Drama"]}},
				{"mediaId":3,"status":"CURRENT","score":0,"media":{"genres":["Action"]}}
			]}]}}}`, 0
		case float64(1):
			// media 3 repetida como PLANNING y media 4 nueva: primero gana CURRENT.
			return `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
				{"mediaId":3,"status":"PLANNING","score":0,"media":{"genres":["Action"]}},
				{"mediaId":4,"status":"COMPLETED","score":10,"media":{"genres":["Action"]}}
			]}]}}}`, 0
		}
		t.Errorf("chunk inesperado: %v", variables["chunk"])
		return `{"errors":[{"message":"chunk inesperado"}]}`, 0
	})
	stats, err := FetchStats(context.Background(), c, "b3low")
	if err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	wantGenres := []GenreStat{{"Action", 3}, {"Comedy", 1}}
	if len(stats.Anime.Genres) != len(wantGenres) {
		t.Fatalf("genres = %+v, quería %+v (PLANNING afuera, media 3 una vez)", stats.Anime.Genres, wantGenres)
	}
	for i, want := range wantGenres {
		if stats.Anime.Genres[i] != want {
			t.Errorf("genres[%d] = %+v, quería %+v", i, stats.Anime.Genres[i], want)
		}
	}
	wantScores := []ScoreStat{{8, 1}, {10, 1}}
	if len(stats.Anime.Scores) != len(wantScores) {
		t.Fatalf("scores = %+v, quería %+v (sin el 0)", stats.Anime.Scores, wantScores)
	}
	for i, want := range wantScores {
		if stats.Anime.Scores[i] != want {
			t.Errorf("scores[%d] = %+v, quería %+v", i, stats.Anime.Scores[i], want)
		}
	}
	// Los totales siguen viniendo de statistics, no de la lista.
	if stats.Anime.Count != 3 || stats.Anime.MeanScore != 90 {
		t.Errorf("los totales se pisaron: %+v", stats.Anime)
	}
}

func TestStatsNoPideListaSiLaApiTraeAgregados(t *testing.T) {
	calls := 0
	c := fakeClient(t, func(query string, _ map[string]any) (string, int) {
		calls++
		if !strings.Contains(query, "statistics") {
			t.Errorf("no debía pedir la lista: %s", query)
		}
		return statsBody, 0
	})
	if _, err := FetchStats(context.Background(), c, "b3low"); err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	if calls != 1 {
		t.Errorf("consultas = %d, quería 1", calls)
	}
}

func TestStatsFallbackFallidoNoPierdeLosTotales(t *testing.T) {
	c := fakeClient(t, func(query string, _ map[string]any) (string, int) {
		if strings.Contains(query, "statistics") {
			return statsVacioBody, 0
		}
		return `{"errors":[{"message":"rate limited"}]}`, 0
	})
	stats, err := FetchStats(context.Background(), c, "b3low")
	if err != nil {
		t.Fatalf("el fallback no debe tirar abajo los totales: %v", err)
	}
	if stats.Anime.Count != 3 || stats.Anime.Genres == nil || stats.Anime.Scores == nil {
		t.Errorf("totales o slices mal: %+v", stats.Anime)
	}
}

func TestFetchStatsUsuarioInexistente(t *testing.T) {
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		return `{"data":{"User":null}}`, 0
	})
	if _, err := FetchStats(context.Background(), c, "fantasma"); err == nil {
		t.Fatal("User null debería ser error, no estadísticas en cero")
	} else if !strings.Contains(err.Error(), "fantasma") {
		t.Errorf("el error no nombra al usuario: %v", err)
	}
}

func TestFetchStatsExigeNombre(t *testing.T) {
	if _, err := FetchStats(context.Background(), offlineClient(t), "  "); err == nil {
		t.Fatal("sin nombre debería fallar antes de consultar")
	}
}

func TestStatsSinDatosDevuelveArraysVacios(t *testing.T) {
	// Un usuario sin géneros ni notas: AniList manda null y el plugin Luau
	// hace #payload.genres, que explota con null.
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		return `{"data":{"User":{"name":"vacio","statistics":{"anime":{"count":0,"episodesWatched":0,"minutesWatched":0,"meanScore":null,"genres":null,"scores":null},"manga":{"count":0,"chaptersRead":0,"volumesRead":0,"meanScore":0}}}}}`, 0
	})
	stats, err := FetchStats(context.Background(), c, "vacio")
	if err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	if stats.Anime.Genres == nil || stats.Anime.Scores == nil {
		t.Fatalf("slices nil: genres=%v scores=%v", stats.Anime.Genres, stats.Anime.Scores)
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"genres":[]`, `"scores":[]`, `"daysWatched":0`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("falta %s en %s", want, raw)
		}
	}
}

func TestLoadStatsSirveCacheFresco(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, Stats{
		User: "b3low",
		Anime: AnimeStats{Count: 334, MinutesWatched: 101040, DaysWatched: 70.2, MeanScore: 78.4,
			Genres: []GenreStat{{"Action", 88}}, Scores: []ScoreStat{{10, 12}}},
		Manga:     MangaStats{Count: 0},
		FetchedAt: time.Now().Unix() - 60,
	})
	stats, err := LoadStats(context.Background(), offlineClient(t), dir, "", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if stats.User != "b3low" || stats.Anime.Count != 334 || stats.Anime.DaysWatched != 70.2 {
		t.Errorf("no vino del caché: %+v", stats)
	}
}

func TestLoadStatsCacheViejoConsulta(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, Stats{
		User:      "b3low",
		Anime:     AnimeStats{Count: 1, Genres: []GenreStat{}, Scores: []ScoreStat{}},
		FetchedAt: time.Now().Add(-StatsCacheTTL - time.Minute).Unix(),
	})
	calls := 0
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		calls++
		return statsBody, 0
	})
	stats, err := LoadStats(context.Background(), c, dir, "b3low", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if calls != 1 {
		t.Errorf("consultas = %d, quería 1 (caché vencido)", calls)
	}
	if stats.Anime.Count != 334 {
		t.Errorf("no refrescó: %+v", stats.Anime)
	}
}

func TestLoadStatsRefreshFuerzaConsulta(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, Stats{
		User:      "b3low",
		Anime:     AnimeStats{Count: 1, Genres: []GenreStat{}, Scores: []ScoreStat{}},
		FetchedAt: time.Now().Unix(),
	})
	calls := 0
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		calls++
		return statsBody, 0
	})
	if _, err := LoadStats(context.Background(), c, dir, "b3low", true); err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if calls != 1 {
		t.Errorf("--refresh no consultó la API (calls=%d)", calls)
	}
}

func TestLoadStatsCacheDeOtroUsuarioNoSeUsa(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, Stats{
		User:      "otro",
		Anime:     AnimeStats{Count: 1, Genres: []GenreStat{}, Scores: []ScoreStat{}},
		FetchedAt: time.Now().Unix(),
	})
	c := fakeClient(t, func(_ string, variables map[string]any) (string, int) {
		if variables["name"] != "b3low" {
			t.Errorf("name = %v, quería b3low", variables["name"])
		}
		return statsBody, 0
	})
	stats, err := LoadStats(context.Background(), c, dir, "b3low", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if stats.User != "b3low" {
		t.Errorf("sirvió el caché de otro usuario: %+v", stats)
	}
}

func TestLoadStatsSinUsuarioPreguntaAlViewer(t *testing.T) {
	dir := t.TempDir()
	var vistas []string
	c := fakeClient(t, func(query string, variables map[string]any) (string, int) {
		switch {
		case strings.Contains(query, "Viewer"):
			vistas = append(vistas, "viewer")
			return viewerBody, 0
		case strings.Contains(query, "statistics"):
			vistas = append(vistas, "stats")
			if variables["name"] != "b3low" {
				t.Errorf("name = %v, quería el del Viewer", variables["name"])
			}
			return statsBody, 0
		}
		t.Errorf("query inesperada: %s", query)
		return `{"errors":[{"message":"inesperada"}]}`, 0
	})
	stats, err := LoadStats(context.Background(), c, dir, "", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if stats.User != "b3low" {
		t.Errorf("user = %q, quería b3low", stats.User)
	}
	if len(vistas) != 2 || vistas[0] != "viewer" || vistas[1] != "stats" {
		t.Errorf("orden de consultas = %v, quería [viewer stats]", vistas)
	}
}

func TestLoadStatsCacheDeOtraCuentaNoSeSirve(t *testing.T) {
	dir := t.TempDir()
	writeCache(t, dir, Stats{
		User:      "ajeno",
		Anime:     AnimeStats{Count: 999, Genres: []GenreStat{}, Scores: []ScoreStat{}},
		FetchedAt: time.Now().Unix(),
	})
	// library.json es la pista de quién es el token cuando no hay --user.
	lib, err := json.Marshal(Library{Viewer: Viewer{ID: 1, Name: "b3low"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "library.json"), lib, 0o600); err != nil {
		t.Fatal(err)
	}
	c := fakeClient(t, func(query string, _ map[string]any) (string, int) {
		if strings.Contains(query, "Viewer") {
			return viewerBody, 0
		}
		return statsBody, 0
	})
	stats, err := LoadStats(context.Background(), c, dir, "", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if stats.User != "b3low" || stats.Anime.Count == 999 {
		t.Errorf("sirvió el caché de otra cuenta: %+v", stats)
	}
}

func TestLoadStatsEscribeCache(t *testing.T) {
	dir := t.TempDir()
	c := fakeClient(t, func(string, map[string]any) (string, int) { return statsBody, 0 })
	first, err := LoadStats(context.Background(), c, dir, "b3low", false)
	if err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	raw, err := os.ReadFile(StatsPath(dir))
	if err != nil {
		t.Fatalf("no quedó stats.json: %v", err)
	}
	var enDisco Stats
	if err := json.Unmarshal(raw, &enDisco); err != nil {
		t.Fatalf("stats.json ilegible: %v", err)
	}
	if enDisco.User != first.User || enDisco.FetchedAt != first.FetchedAt {
		t.Errorf("caché distinto de la respuesta: %+v vs %+v", enDisco, first)
	}
	// Y el segundo llamado no toca la red.
	if _, err := LoadStats(context.Background(), offlineClient(t), dir, "b3low", false); err != nil {
		t.Fatalf("segundo LoadStats: %v", err)
	}
}

func TestLoadStatsErrorNoDejaCache(t *testing.T) {
	dir := t.TempDir()
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		return `{"errors":[{"message":"Not Found."}]}`, 0
	})
	if _, err := LoadStats(context.Background(), c, dir, "b3low", false); err == nil {
		t.Fatal("una query fallida debería devolver error")
	} else if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("el error se perdió: %v", err)
	}
	if _, err := os.Stat(StatsPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no debería escribir caché con la query fallida: %v", err)
	}
}

func TestLoadStatsCacheCorruptoConsulta(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte("{roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := fakeClient(t, func(string, map[string]any) (string, int) {
		calls++
		return statsBody, 0
	})
	if _, err := LoadStats(context.Background(), c, dir, "b3low", false); err != nil {
		t.Fatalf("LoadStats: %v", err)
	}
	if calls != 1 {
		t.Errorf("caché corrupto debería forzar consulta (calls=%d)", calls)
	}
}
