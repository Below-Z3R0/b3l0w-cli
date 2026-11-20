package anilist

// Tests de los hallazgos MEDIO y BAJO de la revisión de integridad.
//
// Cada test tiene que FALLAR si se revierte su arreglo: el mensaje dice qué
// pasa, no sólo que algo cambió.
//
//   - M1: la agenda filtra por tipo y la etiqueta sale de la media.
//   - M2: las notas del fallback están en la escala de statistics.scores.
//   - M3: la biblioteca pide de a 500 entradas.
//   - M4: el chunk arranca en 1 (la página 0 es la misma que la 1).
//   - M5: con `updatedAt` empatado gana la entrada más completa.
//   - M6: `reused` cuenta carátulas, no archivos.
//   - M7: una carátula más grande que el tope se rechaza, no se trunca.
//   - M8: sin dueño verificable el caché de stats no se sirve.
//   - M9: DataDir respeta NOCTALIA_STATE_HOME y XDG_STATE_HOME.
//   - B1: el meta se escribe después del rename del JPG y de forma atómica.
//   - B2: stripHTML decodifica entidades y no se come el texto tras un '<'.
//   - B3: `MediaListCollection: null` es un error, no una biblioteca vacía.
//   - B5: el README no afirma que el CLI exija token para todo.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// escribirLibraryJSON deja el library.json que escribe el plugin de Noctalia:
// es la pista de a quién pertenece el caché de stats.
func escribirLibraryJSON(t *testing.T, dir, nombre string) {
	t.Helper()
	raw, err := json.Marshal(Library{Viewer: Viewer{ID: 1, Name: nombre}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "library.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// M1 — la agenda mete manga y etiqueta todo como ANIME
// ---------------------------------------------------------------------------

func TestLaAgendaFiltraPorTipoYEtiquetaConElTipoDeLaMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, vars := requestBody(t, r)
		switch {
		case strings.Contains(query, "mediaList"):
			// La API real filtra por tipo: si la query no lo pide, la lista
			// trae manga también y la manga ocupa los slots de la agenda.
			if !strings.Contains(query, "type: ANIME") {
				w.Write([]byte(`{"data":{"Page":{"pageInfo":{"hasNextPage":false},"mediaList":[
					{"mediaId":1,"status":"CURRENT","media":{"id":1,"type":"ANIME","title":{"userPreferred":"Anime"},"coverImage":{"extraLarge":"http://c/1.jpg","color":"#abc"},"nextAiringEpisode":{"episode":1,"airingAt":1000}}},
					{"mediaId":2,"status":"CURRENT","media":{"id":2,"type":"MANGA","title":{"userPreferred":"Manga"},"coverImage":{"extraLarge":"http://c/2.jpg","color":"#def"},"nextAiringEpisode":{"episode":1,"airingAt":1000}}}
				]}}}`))
				return
			}
			w.Write([]byte(`{"data":{"Page":{"pageInfo":{"hasNextPage":false},"mediaList":[
				{"mediaId":1,"status":"CURRENT","media":{"id":1,"type":"ANIME","title":{"userPreferred":"Anime"},"coverImage":{"extraLarge":"http://c/1.jpg","color":"#abc"},"nextAiringEpisode":{"episode":1,"airingAt":1000}}}
			]}}}`))
			return
		case strings.Contains(query, "airingSchedules"):
			// Un capítulo por cada media pedida, más una media que no está en
			// la lista: la etiqueta tiene que salir de la respuesta, no de un
			// "ANIME" fijo.
			tipos := map[int]string{1: "ANIME", 2: "MANGA", 9: "MANGA"}
			nodes := []string{}
			for _, id := range vars["ids"].([]any) {
				mediaID := int(id.(float64))
				nodes = append(nodes, `{"mediaId":`+strconv.Itoa(mediaID)+`,"episode":2,"airingAt":2000,"media":{"id":`+strconv.Itoa(mediaID)+`,"type":"`+
					tipos[mediaID]+`","title":{"userPreferred":"Serie"},"coverImage":{"extraLarge":"http://c/`+strconv.Itoa(mediaID)+`.jpg","color":"#123"}}}`)
			}
			nodes = append(nodes, `{"mediaId":9,"episode":3,"airingAt":2000,"media":{"id":9,"type":"MANGA","title":{"userPreferred":"Manga oculto"},"coverImage":{"extraLarge":"http://c/9.jpg","color":"#123"}}}`)
			fmt.Fprintf(w, `{"data":{"Page":{"pageInfo":{"hasNextPage":false},"airingSchedules":[%s]}}}`, strings.Join(nodes, ","))
			return
		}
		w.Write([]byte(`{"errors":[{"message":"consulta inesperada"}]}`))
	}))
	defer srv.Close()

	sched, err := FetchSchedule(context.Background(), serverClient(t, srv), 42)
	if err != nil {
		t.Fatalf("FetchSchedule: %v", err)
	}
	porID := map[int]AiringItem{}
	for _, item := range sched.Items {
		porID[item.MediaID] = item
	}
	if _, ok := porID[2]; ok {
		t.Fatalf("la manga de la lista entró en la agenda: %+v", sched.Items)
	}
	if item, ok := porID[1]; !ok {
		t.Fatalf("falta la serie de anime en la agenda: %+v", sched.Items)
	} else if item.MediaType != "ANIME" {
		t.Errorf("mediaType = %q; quería ANIME", item.MediaType)
	}
	if item, ok := porID[9]; !ok {
		t.Fatalf("falta el capítulo de la media 9: %+v", sched.Items)
	} else if item.MediaType != "MANGA" {
		t.Errorf("mediaType = %q; quería MANGA: la etiqueta no puede ser fija (M1)", item.MediaType)
	}
}

// ---------------------------------------------------------------------------
// M2 — el fallback de stats cuenta las notas en otra escala
// ---------------------------------------------------------------------------

func TestElFallbackDeStatsUsaLaEscalaDeStatistics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, _ := requestBody(t, r)
		switch {
		case strings.Contains(query, "statistics"):
			// La API devuelve count pero sin géneros ni notas: entra el
			// fallback, que recalcula desde la lista.
			w.Write([]byte(`{"data":{"User":{"name":"b3low","statistics":{"anime":{"count":10,"episodesWatched":100,"minutesWatched":6000,"meanScore":78.4,"genres":[],"scores":[]},"manga":{"count":2}}}}}`))
			return
		case strings.Contains(query, "MediaListCollection"):
			// La escala del score depende del `format` que pida la query:
			// sin format, AniList contesta en la escala del usuario.
			if strings.Contains(query, "POINT_100") {
				w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
					{"mediaId":1,"status":"COMPLETED","score":85,"media":{"genres":["Action"]}}
				]}]}}}`))
				return
			}
			w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
				{"mediaId":1,"status":"COMPLETED","score":8.5,"media":{"genres":["Action"]}}
			]}]}}}`))
			return
		}
		w.Write([]byte(`{"errors":[{"message":"consulta inesperada"}]}`))
	}))
	defer srv.Close()

	stats, err := FetchStats(context.Background(), serverClient(t, srv), "b3low")
	if err != nil {
		t.Fatalf("FetchStats: %v", err)
	}
	if len(stats.Anime.Scores) != 1 {
		t.Fatalf("scores = %+v; quería una nota", stats.Anime.Scores)
	}
	if stats.Anime.Scores[0].Score != 85 {
		t.Errorf("score = %d; quería 85: la distribución tiene que estar en la escala de statistics.scores (M2)", stats.Anime.Scores[0].Score)
	}
}

// ---------------------------------------------------------------------------
// M3 — perChunk: 50 cuando la API acepta 500
// ---------------------------------------------------------------------------

func TestLaBibliotecaPideDeA500Entradas(t *testing.T) {
	var pedidos []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, vars := requestBody(t, r)
		pedidos = append(pedidos, vars["perChunk"].(float64))
		w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
			{"mediaId":1,"status":"CURRENT","progress":1,"updatedAt":10,"media":{"episodes":12,"title":{"userPreferred":"Alfa"},"genres":[]}}
		]}]}}}`))
	}))
	defer srv.Close()

	if _, err := FetchList(context.Background(), serverClient(t, srv), 123, "ANIME"); err != nil {
		t.Fatalf("FetchList: %v", err)
	}
	if len(pedidos) == 0 {
		t.Fatal("no se hizo ninguna consulta")
	}
	if pedidos[0] != 500 {
		t.Errorf("perChunk = %v; quería 500: pedir de a 50 multiplica por diez las consultas (M3)", pedidos[0])
	}
}

// ---------------------------------------------------------------------------
// M4 — chunk: 0 y chunk: 1 devuelven la misma página
// ---------------------------------------------------------------------------

func TestElChunkArrancaEnUno(t *testing.T) {
	var pedidos []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, vars := requestBody(t, r)
		chunk := vars["chunk"].(float64)
		pedidos = append(pedidos, chunk)
		// La API real responde la misma página para el chunk 0 y el 1.
		if chunk == 0 || chunk == 1 {
			w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":true,"lists":[{"entries":[
				{"mediaId":1,"status":"CURRENT","progress":1,"updatedAt":10,"media":{"episodes":12,"title":{"userPreferred":"Alfa"},"genres":[]}}
			]}]}}}`))
			return
		}
		w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
			{"mediaId":2,"status":"CURRENT","progress":1,"updatedAt":20,"media":{"episodes":12,"title":{"userPreferred":"Beta"},"genres":[]}}
		]}]}}}`))
	}))
	defer srv.Close()

	entries, err := FetchList(context.Background(), serverClient(t, srv), 123, "ANIME")
	if err != nil {
		t.Fatalf("FetchList: %v", err)
	}
	if len(pedidos) == 0 || pedidos[0] != 1 {
		t.Fatalf("chunks pedidos = %v; el primero tiene que ser el 1 (M4)", pedidos)
	}
	for _, chunk := range pedidos {
		if chunk == 0 {
			t.Fatalf("se pidió el chunk 0: la API devuelve la misma página que el 1 (M4)")
		}
	}
	if len(entries) != 2 {
		t.Errorf("entradas = %d; quería 2 (las dos páginas)", len(entries))
	}
}

// ---------------------------------------------------------------------------
// M5 — empate de updatedAt en el merge
// ---------------------------------------------------------------------------

func TestElMergeDesempataPorProgreso(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, vars := requestBody(t, r)
		if vars["chunk"] == float64(1) {
			// La misma obra en dos listas, tocadas en el mismo segundo.
			w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":true,"lists":[{"entries":[
				{"mediaId":7,"status":"CURRENT","progress":5,"score":80,"updatedAt":100,"media":{"episodes":24,"title":{"userPreferred":"Zeta"},"genres":[]}}
			]}]}}}`))
			return
		}
		w.Write([]byte(`{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
			{"mediaId":7,"status":"COMPLETED","progress":12,"score":80,"updatedAt":100,"media":{"episodes":24,"title":{"userPreferred":"Zeta"},"genres":[]}}
		]}]}}}`))
	}))
	defer srv.Close()

	entries, err := FetchList(context.Background(), serverClient(t, srv), 123, "ANIME")
	if err != nil {
		t.Fatalf("FetchList: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entradas = %d; quería 1", len(entries))
	}
	if entries[0].Progress != 12 {
		t.Errorf("progress = %d; quería 12: con updatedAt empatado gana la más completa (M5)", entries[0].Progress)
	}
}

// ---------------------------------------------------------------------------
// M6 — adopt() cuenta archivos, no carátulas
// ---------------------------------------------------------------------------

func TestReusedCuentaCaratulasYNoArchivos(t *testing.T) {
	reuse := t.TempDir()
	dir := t.TempDir()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("JPEG"))
	}))
	defer cdn.Close()

	// Una carátula del caché viejo: el .jpg y su .meta.json.
	if err := os.WriteFile(filepath.Join(reuse, "7.jpg"), []byte("JPEG"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := `{"url":"` + cdn.URL + `/7.jpg","version":2}`
	if err := os.WriteFile(filepath.Join(reuse, "7.jpg.meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	stats, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{
		Workers: 1, ReuseDir: reuse,
	})
	if err != nil {
		t.Fatalf("SyncCovers: %v", err)
	}
	if stats.Reused != 1 {
		t.Errorf("reused = %d; quería 1: es UNA carátula, no dos archivos (M6)", stats.Reused)
	}
	if len(stats.Covers) != 1 {
		t.Errorf("covers = %+v; quería la carátula adoptada", stats.Covers)
	}
}

// ---------------------------------------------------------------------------
// M7 — el tope de 16 MB trunca la imagen en silencio
// ---------------------------------------------------------------------------

func TestUnaCaratulaMasGrandeQueElTopeSeRechaza(t *testing.T) {
	bloque := bytes.Repeat([]byte("A"), 1<<20)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 17 MB: más que el tope de 16 MB.
		for i := 0; i < 17; i++ {
			if _, err := w.Write(bloque); err != nil {
				return
			}
		}
	}))
	defer cdn.Close()

	dir := t.TempDir()
	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	stats, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{Workers: 1})
	if err != nil {
		t.Fatalf("SyncCovers: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d; quería 1: una imagen que excede el tope es un fallo, no un éxito (M7)", stats.Failed)
	}
	if stats.Downloaded != 0 {
		t.Errorf("downloaded = %d; quería 0", stats.Downloaded)
	}
	if _, err := os.Stat(filepath.Join(dir, "7.jpg")); !os.IsNotExist(err) {
		t.Errorf("quedó una carátula truncada en disco (err = %v)", err)
	}
}

// ---------------------------------------------------------------------------
// M8 — sin library.json se sirve el caché de otra cuenta
// ---------------------------------------------------------------------------

func TestElCacheDeStatsNoSeSirveSinDueñoVerificable(t *testing.T) {
	t.Run("sin library.json", func(t *testing.T) {
		dir := t.TempDir()
		writeCache(t, dir, Stats{
			User:      "cuentaAjena",
			Anime:     AnimeStats{Count: 999},
			FetchedAt: time.Now().Unix(),
		})
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
		if stats.User == "cuentaAjena" || stats.Anime.Count == 999 {
			t.Errorf("sirvió el caché de otra cuenta: %+v", stats)
		}
	})

	t.Run("con viewer mal formado", func(t *testing.T) {
		dir := t.TempDir()
		writeCache(t, dir, Stats{
			User:      "cuentaAjena",
			Anime:     AnimeStats{Count: 999},
			FetchedAt: time.Now().Unix(),
		})
		if err := os.WriteFile(filepath.Join(dir, "library.json"), []byte(`{"viewer":"no-es-un-objeto"}`), 0o600); err != nil {
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
		if stats.User == "cuentaAjena" || stats.Anime.Count == 999 {
			t.Errorf("sirvió el caché de otra cuenta: %+v", stats)
		}
	})

	t.Run("con dueño verificable se sirve", func(t *testing.T) {
		dir := t.TempDir()
		escribirLibraryJSON(t, dir, "b3low")
		writeCache(t, dir, Stats{
			User:      "b3low",
			Anime:     AnimeStats{Count: 334},
			FetchedAt: time.Now().Unix(),
		})
		stats, err := LoadStats(context.Background(), offlineClient(t), dir, "", false)
		if err != nil {
			t.Fatalf("LoadStats: %v", err)
		}
		if stats.Anime.Count != 334 {
			t.Errorf("no sirvió el caché propio: %+v", stats)
		}
	})
}

// ---------------------------------------------------------------------------
// M9 — DataDir ignora XDG_STATE_HOME y NOCTALIA_STATE_HOME
// ---------------------------------------------------------------------------

func TestDataDirRespetaLasVariablesDeEstado(t *testing.T) {
	t.Setenv("NOCTALIA_STATE_HOME", "/estado/noctalia")
	t.Setenv("XDG_STATE_HOME", "/estado/xdg")

	got := DataDir("", "b3l0w/anilist")
	want := filepath.Join("/estado/noctalia", "noctalia", "plugins", "data", "b3l0w", "anilist")
	if got != want {
		t.Errorf("DataDir = %q; quería %q (NOCTALIA_STATE_HOME, M9)", got, want)
	}

	// Sin NOCTALIA_STATE_HOME manda el de XDG.
	t.Setenv("NOCTALIA_STATE_HOME", "")
	got = DataDir("", "b3l0w/anilist")
	want = filepath.Join("/estado/xdg", "noctalia", "plugins", "data", "b3l0w", "anilist")
	if got != want {
		t.Errorf("DataDir = %q; quería %q (XDG_STATE_HOME, M9)", got, want)
	}

	// El flag explícito gana sobre todo.
	got = DataDir("/estado/explicito", "b3l0w/anilist")
	want = filepath.Join("/estado/explicito", "noctalia", "plugins", "data", "b3l0w", "anilist")
	if got != want {
		t.Errorf("DataDir = %q; quería %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// B1 — el meta se escribe in situ y antes del rename del .part
// ---------------------------------------------------------------------------

func TestElMetaSeEscribeDespuesDelRenameDelJpg(t *testing.T) {
	dir := t.TempDir()
	// "7.jpg" es un directorio: el rename del JPG falla. Si el meta se
	// escribiera antes, quedaría un meta nuevo con la imagen vieja y un
	// lector concurrente la daría por válida.
	if err := os.Mkdir(filepath.Join(dir, "7.jpg"), 0o700); err != nil {
		t.Fatal(err)
	}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("JPEG"))
	}))
	defer cdn.Close()

	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	err := downloadCover(context.Background(), &http.Client{}, entry, dir)
	if err == nil {
		t.Fatal("downloadCover debía fallar: el rename del JPG no puede prosperar")
	}
	if _, err := os.Stat(filepath.Join(dir, "7.jpg.meta.json")); !os.IsNotExist(err) {
		t.Errorf("el meta quedó escrito aunque el JPG no se renombró (B1)")
	}
	restos, _ := os.ReadDir(dir)
	for _, item := range restos {
		if strings.HasSuffix(item.Name(), ".part") {
			t.Errorf("quedó un temporal: %s", item.Name())
		}
	}
}

func TestElMetaSeEscribeAtomico(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "7.jpg.meta.json")
	if err := os.WriteFile(metaPath, []byte(`{"url":"http://viejo/7.jpg","version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	viejo, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("JPEG"))
	}))
	defer cdn.Close()

	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	if err := downloadCover(context.Background(), &http.Client{}, entry, dir); err != nil {
		t.Fatalf("downloadCover: %v", err)
	}
	nuevo, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(viejo, nuevo) {
		t.Errorf("el meta se truncó in situ: la escritura no es atómica (B1)")
	}
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta coverMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("el meta no es JSON: %v", err)
	}
	if meta.URL != entry.CoverURL || meta.Version != CoverVersion {
		t.Errorf("meta = %+v; quería la URL nueva y la versión %d", meta, CoverVersion)
	}
	restos, _ := os.ReadDir(dir)
	for _, item := range restos {
		if strings.HasSuffix(item.Name(), ".part") {
			t.Errorf("quedó un temporal: %s", item.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// B2 — stripHTML no decodifica entidades y se traga el texto tras un '<'
// ---------------------------------------------------------------------------

func TestStripHTMLDecodificaEntidadesYNoSeComeElTexto(t *testing.T) {
	if got := stripHTML("a &mdash; b"); got != "a — b" {
		t.Errorf("stripHTML(%q) = %q; quería %q (las entidades no se decodifican, B2)", "a &mdash; b", got, "a — b")
	}
	if got := stripHTML("5 < 6 y 7"); got != "5 < 6 y 7" {
		t.Errorf("stripHTML(%q) = %q; quería %q (el '<' suelto se come el resto, B2)", "5 < 6 y 7", got, "5 < 6 y 7")
	}
	if got := stripHTML("<b>Hola</b> <i>mundo</i>"); got != "Hola mundo" {
		t.Errorf("stripHTML de HTML bien formado = %q; quería %q", got, "Hola mundo")
	}
	if got := stripHTML("Caf&eacute; &amp; m&aacute;s"); got != "Café & más" {
		t.Errorf("stripHTML = %q; quería %q", got, "Café & más")
	}
}

// ---------------------------------------------------------------------------
// B3 — MediaListCollection: null → biblioteca vacía sin error
// ---------------------------------------------------------------------------

func TestUnaColeccionNulaEsUnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"MediaListCollection":null}}`))
	}))
	defer srv.Close()

	entries, err := FetchList(context.Background(), serverClient(t, srv), 123, "ANIME")
	if err == nil {
		t.Fatalf("FetchList devolvió err=nil con la colección nula: biblioteca vacía sin error (B3)")
	}
	if len(entries) != 0 {
		t.Errorf("entradas = %d; quería 0", len(entries))
	}
}

// ---------------------------------------------------------------------------
// B5 — el README dice que el CLI exige token
// ---------------------------------------------------------------------------

func TestElReadmeNoDiceQueElCliExijaTokenParaTodo(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("leer el README: %v", err)
	}
	texto := string(raw)
	if strings.Contains(texto, "antes de correr cualquier comando") {
		t.Errorf("el README afirma que el CLI exige token antes de correr cualquier comando: `covers --library` corre sin token (B5)")
	}
	if !strings.Contains(texto, "covers --library") {
		t.Errorf("el README no menciona la excepción de `covers --library` (B5)")
	}
}
