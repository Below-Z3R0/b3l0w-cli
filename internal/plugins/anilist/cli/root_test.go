// Tests de la capa de parseo y salida de `b3l0w anilist`.
//
// Esta capa es la que consume el plugin de Noctalia: `b3l0w anilist <cmd> --json`
// tiene que devolver UN documento JSON en stdout. Los tests no tocan la red:
// un RoundTripper falso intercepta el cliente GraphQL y las descargas de
// carátulas, así que se puede verificar el armado del pedido, la traducción de
// la respuesta y la forma exacta de la salida.
package anilistcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist"
)

// ---------------------------------------------------------------------------
// Captura de stdout/stderr
// ---------------------------------------------------------------------------

// capturar corre fn con stdout y stderr apuntando a tuberías y devuelve el
// código de salida junto con lo que se escribió en cada uno.
func capturar(t *testing.T, fn func() int) (int, string, string) {
	t.Helper()
	originalOut, originalErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = originalOut, originalErr }()

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(&outBuf, rOut) }()
	go func() { defer wg.Done(); io.Copy(&errBuf, rErr) }()

	codigo := fn()
	wOut.Close()
	wErr.Close()
	wg.Wait()
	rOut.Close()
	rErr.Close()
	return codigo, outBuf.String(), errBuf.String()
}

// afirmarUnSoloJSON comprueba que stdout sea UN documento JSON y nada más.
// Se valida decodificando (no con strings.Contains) porque un println de
// progreso antes del JSON rompe al consumidor y un substring no lo ve.
func afirmarUnSoloJSON(t *testing.T, salida string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(salida))
	var valor map[string]any
	if err := dec.Decode(&valor); err != nil {
		t.Fatalf("stdout no es un documento JSON: %v\nstdout: %q", err, salida)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout tiene más de un documento JSON (err=%v): %q", err, salida)
	}
	return valor
}

// ---------------------------------------------------------------------------
// AniList falso: intercepta el cliente GraphQL y las descargas de carátulas
// ---------------------------------------------------------------------------

type consultaAniList struct {
	query     string
	variables map[string]any
}

type anilistFalso struct {
	mu        sync.Mutex
	registro  []consultaAniList
	responder func(query string, variables map[string]any) (cuerpo string, status int)
}

func (f *anilistFalso) RoundTrip(req *http.Request) (*http.Response, error) {
	// Las carátulas se piden por GET al CDN, no a la API.
	if req.Method == http.MethodGet {
		return respuestaHTTP(http.StatusOK, "image/jpeg", []byte("JPEG-de-prueba"), req), nil
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()
	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.registro = append(f.registro, consultaAniList{query: payload.Query, variables: payload.Variables})
	f.mu.Unlock()
	cuerpo, status := f.responder(payload.Query, payload.Variables)
	if status == 0 {
		status = http.StatusOK
	}
	return respuestaHTTP(status, "application/json", []byte(cuerpo), req), nil
}

func respuestaHTTP(status int, contentType string, cuerpo []byte, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(cuerpo)),
		Request:    req,
	}
}

// instalarAniListFalso reemplaza el transporte por defecto de net/http: el
// cliente del dominio no permite inyectar uno, así que se cambia el global y
// se restaura al terminar el test.
func instalarAniListFalso(t *testing.T) *anilistFalso {
	t.Helper()
	falso := &anilistFalso{responder: responderPorDefecto}
	original := http.DefaultTransport
	http.DefaultTransport = falso
	t.Cleanup(func() { http.DefaultTransport = original })
	return falso
}

// consultasHechas devuelve las consultas que llegaron a la API.
func (f *anilistFalso) consultasHechas() []consultaAniList {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]consultaAniList(nil), f.registro...)
}

// variablesDe devuelve las variables de la última consulta cuya query contiene
// la subcadena dada.
func (f *anilistFalso) variablesDe(t *testing.T, subcadena string) map[string]any {
	t.Helper()
	var encontrada map[string]any
	for _, c := range f.consultasHechas() {
		if strings.Contains(c.query, subcadena) {
			encontrada = c.variables
		}
	}
	if encontrada == nil {
		t.Fatalf("no llegó ninguna consulta con %q; llegaron %d", subcadena, len(f.consultasHechas()))
	}
	return encontrada
}

const cuerpoViewer = `{"data":{"Viewer":{"id":123,"name":"b3low"}}}`

const cuerpoListaAnime = `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
{"mediaId":1,"status":"CURRENT","progress":5,"progressVolumes":0,"score":80,"updatedAt":1700000000,
 "media":{"id":1,"type":"ANIME","episodes":12,"chapters":null,"volumes":null,
  "coverImage":{"color":"#112233","extraLarge":"https://cdn.test/1.jpg","large":"","medium":""},
  "title":{"userPreferred":"Serie de Prueba"},"genres":["Action","Comedy"],
  "nextAiringEpisode":{"airingAt":1700500000,"episode":6}}}]}]}}}`

const cuerpoListaManga = `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[{"entries":[
{"mediaId":2,"status":"PLANNING","progress":0,"progressVolumes":1,"score":0,"updatedAt":1700000001,
 "media":{"id":2,"type":"MANGA","episodes":null,"chapters":20,"volumes":4,
  "coverImage":{"color":"#445566","extraLarge":"https://cdn.test/2.jpg","large":"","medium":""},
  "title":{"userPreferred":"Manga de Prueba"},"genres":["Drama"],"nextAiringEpisode":null}}]}]}}}`

const cuerpoDescubrimiento = `{"data":{"Page":{"pageInfo":{"hasNextPage":true,"total":42},"media":[
{"id":9,"title":{"userPreferred":"Tendencia del Mes"},"coverImage":{"extraLarge":"https://cdn.test/9.jpg","large":"","color":"#abcdef"},
 "averageScore":85,"episodes":24,"status":"RELEASING","format":"TV","season":"SUMMER","seasonYear":2024,
 "genres":["Action","Comedy"],"description":"<p>Sipnosis con <br> HTML pegado</p>"}]}}}`

const cuerpoMedia = `{"data":{
"Media":{"id":9,"title":{"userPreferred":"Serie de Prueba"},"description":"<p>Texto</p>","genres":["Drama"],
 "episodes":24,"chapters":null,"format":"TV","status":"RELEASING","averageScore":85,
 "coverImage":{"extraLarge":"https://cdn.test/9.jpg","large":"","color":"#abcdef"},
 "nextAiringEpisode":{"episode":7,"airingAt":1700500000},"airingSchedule":{"nodes":[{"episode":7,"airingAt":1700500000}]}},
"MediaList":{"status":"CURRENT","progress":3,"score":70,"updatedAt":1700000000}}}`

const cuerpoMutacion = `{"data":{"SaveMediaListEntry":{"id":55,"mediaId":9,"progress":4,"status":"CURRENT"}}}`

const cuerpoStats = `{"data":{"User":{"name":"b3low","statistics":{"anime":{"count":10,"episodesWatched":100,
"minutesWatched":6000,"meanScore":78.4,"genres":[{"genre":"Action","count":5}],"scores":[{"score":8,"count":2}]},
"manga":{"count":2,"chaptersRead":20,"volumesRead":3,"meanScore":70}}}}}`

const cuerpoListaActual = `{"data":{"Page":{"mediaList":[{"mediaId":9,"status":"CURRENT","media":{"id":9,
"title":{"userPreferred":"Serie de Prueba"},"coverImage":{"extraLarge":"https://cdn.test/9.jpg","color":"#abcdef"},
"nextAiringEpisode":{"episode":7,"airingAt":1700500000}}}]}}}`

const cuerpoAgenda = `{"data":{"Page":{"airingSchedules":[{"mediaId":9,"episode":7,"airingAt":1700500000,
"media":{"id":9,"title":{"userPreferred":"Serie de Prueba"},"coverImage":{"extraLarge":"https://cdn.test/9.jpg"}}}]}}}`

// responderPorDefecto reparte las consultas de la API por su contenido.
func responderPorDefecto(query string, variables map[string]any) (string, int) {
	switch {
	case strings.Contains(query, "Viewer { id name }"):
		return cuerpoViewer, 0
	case strings.Contains(query, "SaveMediaListEntry"):
		return cuerpoMutacion, 0
	case strings.Contains(query, "Media(id: $id)"):
		return cuerpoMedia, 0
	case strings.Contains(query, "statistics {"):
		return cuerpoStats, 0
	case strings.Contains(query, "MediaListCollection(userName:"):
		return `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[]}}}`, 0
	case strings.Contains(query, "MediaListCollection(userId:"):
		if variables["type"] == "MANGA" {
			return cuerpoListaManga, 0
		}
		return cuerpoListaAnime, 0
	case strings.Contains(query, "mediaList(userId:"):
		return cuerpoListaActual, 0
	case strings.Contains(query, "airingSchedules("):
		return cuerpoAgenda, 0
	case strings.Contains(query, "media(sort:"):
		return cuerpoDescubrimiento, 0
	}
	return `{"errors":[{"message":"consulta no simulada"}]}`, 0
}

// ---------------------------------------------------------------------------
// Utilidades
// ---------------------------------------------------------------------------

// escribir deja un archivo con el contenido dado.
func escribir(t *testing.T, ruta, contenido string) {
	t.Helper()
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
}

// escribirToken deja un token.json en el data dir.
func escribirToken(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	crudo, err := json.Marshal(map[string]string{"access_token": "token-de-prueba"})
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(dir, "token.json"), string(crudo))
}

// sinTokenDeEntorno vacía ANILIST_TOKEN: el token tiene que venir del flag o
// de token.json, nunca del entorno del que corra el test.
func sinTokenDeEntorno(t *testing.T) {
	t.Helper()
	t.Setenv("ANILIST_TOKEN", "")
}

// ---------------------------------------------------------------------------
// Despacho y ayuda
// ---------------------------------------------------------------------------

func TestSinArgumentosImprimeElUsoYDevuelveDos(t *testing.T) {
	codigo, salida, errores := capturar(t, func() int { return Run(nil) })
	if codigo != 2 {
		t.Errorf("código = %d, quería 2 (uso sin comando)", codigo)
	}
	if !strings.Contains(salida, "b3l0w anilist") || !strings.Contains(salida, "sync") {
		t.Errorf("no imprimió el uso:\n%s", salida)
	}
	if errores != "" {
		t.Errorf("stderr = %q, quería vacío", errores)
	}
}

func TestComandoDesconocidoVaAStderrYDevuelveDos(t *testing.T) {
	codigo, salida, errores := capturar(t, func() int { return Run([]string{"inventado"}) })
	if codigo != 2 {
		t.Errorf("código = %d, quería 2", codigo)
	}
	if !strings.Contains(errores, "comando desconocido: inventado") {
		t.Errorf("stderr = %q, quería el nombre del comando", errores)
	}
	if !strings.Contains(salida, "b3l0w anilist") {
		t.Errorf("no imprimió el uso en stdout:\n%s", salida)
	}
}

func TestHelpImprimeElUsoYDevuelveCero(t *testing.T) {
	for _, variante := range []string{"help", "-h", "--help"} {
		t.Run(variante, func(t *testing.T) {
			codigo, salida, errores := capturar(t, func() int { return Run([]string{variante}) })
			if codigo != 0 {
				t.Errorf("código = %d, quería 0", codigo)
			}
			if !strings.Contains(salida, "uso: b3l0w anilist") {
				t.Errorf("no imprimió el uso:\n%s", salida)
			}
			if errores != "" {
				t.Errorf("stderr = %q, quería vacío", errores)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sync
// ---------------------------------------------------------------------------

func TestSyncDevuelveBibliotecaYCubiertasEnUnSoloJSON(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	viewer, ok := doc["viewer"].(map[string]any)
	if !ok || viewer["name"] != "b3low" {
		t.Errorf("viewer = %v, quería el usuario del token", doc["viewer"])
	}
	anime, ok := doc["anime"].([]any)
	if !ok || len(anime) != 1 {
		t.Fatalf("anime = %v, quería una entrada", doc["anime"])
	}
	cubiertas, ok := doc["covers"].(map[string]any)
	if !ok {
		t.Fatalf("covers = %v, quería el mapa de carátulas", doc["covers"])
	}
	if _, ok := cubiertas["1"]; !ok {
		t.Errorf("covers = %v, quería la entrada 1", cubiertas)
	}
	if doc["coversDir"] != filepath.Join(dir, "covers") {
		t.Errorf("coversDir = %v, quería %q", doc["coversDir"], filepath.Join(dir, "covers"))
	}
	// Las carátulas bajan al disco.
	if _, err := os.Stat(filepath.Join(dir, "covers", "1.jpg")); err != nil {
		t.Errorf("no se descargó la carátula: %v", err)
	}
	// Y la biblioteca se pidió con el usuario del token.
	vars := falso.variablesDe(t, "MediaListCollection(userId:")
	if vars["userId"] != float64(123) {
		t.Errorf("userId = %v, quería 123 (el del Viewer)", vars["userId"])
	}
}

func TestSyncEnTextoMuestraUsuarioYCubiertas(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"sync", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	if !strings.Contains(salida, "usuario: b3low (#123)") {
		t.Errorf("no muestra el usuario:\n%s", salida)
	}
	if !strings.Contains(salida, "carátulas:") {
		t.Errorf("no muestra el resumen de carátulas:\n%s", salida)
	}
}

func TestSyncRespetaElDirectorioDeCubiertas(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)
	cubiertas := filepath.Join(t.TempDir(), "mis-cubiertas")

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir, "--covers-dir", cubiertas})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["coversDir"] != cubiertas {
		t.Errorf("coversDir = %v, quería %q", doc["coversDir"], cubiertas)
	}
	if _, err := os.Stat(filepath.Join(cubiertas, "1.jpg")); err != nil {
		t.Errorf("no escribió la carátula en el directorio pedido: %v", err)
	}
}

func TestSyncSinTokenFallaConCodigoUno(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, _, errores := capturar(t, func() int {
		return Run([]string{"sync", "--data-dir", dir})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errores, "no hay token") {
		t.Errorf("stderr = %q, quería el motivo de la falta de token", errores)
	}
}

func TestSyncEnJSONElErrorEsUnDocumentoJSON(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["error"] == nil || !strings.Contains(doc["error"].(string), "no hay token") {
		t.Errorf("error = %v, quería el motivo en el JSON", doc["error"])
	}
}

// ---------------------------------------------------------------------------
// library
// ---------------------------------------------------------------------------

func TestLibraryDevuelveLaBibliotecaEnUnSoloJSON(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"library", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	anime, ok := doc["anime"].([]any)
	if !ok || len(anime) != 1 {
		t.Fatalf("anime = %v, quería una entrada", doc["anime"])
	}
	manga, ok := doc["manga"].([]any)
	if !ok || len(manga) != 1 {
		t.Fatalf("manga = %v, quería una entrada", doc["manga"])
	}
	if doc["partial"] != false {
		t.Errorf("partial = %v, quería false (las dos listas vinieron)", doc["partial"])
	}
	vars := falso.variablesDe(t, "MediaListCollection(userId:")
	if vars["type"] != "ANIME" && vars["type"] != "MANGA" {
		t.Errorf("type = %v, quería ANIME o MANGA", vars["type"])
	}
}

func TestLibraryConUnaListaRotaMarcaParcial(t *testing.T) {
	falso := instalarAniListFalso(t)
	falso.responder = func(query string, variables map[string]any) (string, int) {
		if strings.Contains(query, "MediaListCollection(userId:") && variables["type"] == "MANGA" {
			return `{"errors":[{"message":"Not Found."}]}`, 0
		}
		return responderPorDefecto(query, variables)
	}
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"library", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0: la mitad de la biblioteca vale (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["partial"] != true {
		t.Errorf("partial = %v, quería true (una lista falló)", doc["partial"])
	}
}

func TestLibraryEnTextoAvisaSiUnaListaFallo(t *testing.T) {
	falso := instalarAniListFalso(t)
	falso.responder = func(query string, variables map[string]any) (string, int) {
		if strings.Contains(query, "MediaListCollection(userId:") && variables["type"] == "MANGA" {
			return `{"errors":[{"message":"Not Found."}]}`, 0
		}
		return responderPorDefecto(query, variables)
	}
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"library", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "aviso: una de las listas no se pudo traer") {
		t.Errorf("no avisa que una lista falló:\n%s", salida)
	}
}

func TestLibraryEnTextoMuestraLosTotales(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"library", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "usuario: b3low (#123)") {
		t.Errorf("no muestra el usuario:\n%s", salida)
	}
	if !strings.Contains(salida, "anime:   1") || !strings.Contains(salida, "manga:   1") {
		t.Errorf("no muestra los totales:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// covers
// ---------------------------------------------------------------------------

func TestCoversDesdeElArchivoDeBibliotecaNoPideToken(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	archivo := filepath.Join(t.TempDir(), "library.json")
	crudo := `{"viewer":{"id":7,"name":"b3low"},"anime":[{"mediaId":11,"title":"Serie","coverUrl":"https://cdn.test/11.jpg"}],"manga":[]}`
	escribir(t, archivo, crudo)
	cubiertas := filepath.Join(t.TempDir(), "cubiertas")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"covers", "--json", "--data-dir", dir, "--library", archivo, "--covers-dir", cubiertas})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	mapa, ok := doc["covers"].(map[string]any)
	if !ok || mapa["11"] == nil {
		t.Fatalf("covers = %v, quería la entrada 11", doc["covers"])
	}
	if _, err := os.Stat(filepath.Join(cubiertas, "11.jpg")); err != nil {
		t.Errorf("no descargó la carátula: %v", err)
	}
}

func TestCoversTomaElArchivoDeBibliotecaDeLua(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	archivo := filepath.Join(t.TempDir(), "library.json")
	// Una tabla Lua vacía se serializa como {} y no como [].
	escribir(t, archivo, `{}`)
	cubiertas := filepath.Join(t.TempDir(), "cubiertas")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"covers", "--json", "--library", archivo, "--covers-dir", cubiertas})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	mapa, ok := doc["covers"].(map[string]any)
	if !ok || len(mapa) != 0 {
		t.Errorf("covers = %v, quería un mapa vacío", doc["covers"])
	}
}

func TestCoversConArchivoInexistenteFalla(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	faltante := filepath.Join(t.TempDir(), "no-existe.json")

	codigo, _, errores := capturar(t, func() int {
		return Run([]string{"covers", "--data-dir", t.TempDir(), "--library", faltante})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errores, "error:") {
		t.Errorf("stderr = %q, quería el motivo del error", errores)
	}
}

func TestCoversEnTextoMuestraElResumen(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	archivo := filepath.Join(t.TempDir(), "library.json")
	crudo := `{"anime":[{"mediaId":11,"title":"Serie","coverUrl":"https://cdn.test/11.jpg"}],"manga":[]}`
	escribir(t, archivo, crudo)
	cubiertas := filepath.Join(t.TempDir(), "cubiertas")

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"covers", "--library", archivo, "--covers-dir", cubiertas})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "carátulas:") || !strings.Contains(salida, "1 válidas") {
		t.Errorf("no muestra el resumen de carátulas:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// mutate
// ---------------------------------------------------------------------------

func TestMutateExigeMediaIDYProgreso(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"mutate", "--json", "--data-dir", dir})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["error"] == nil || !strings.Contains(doc["error"].(string), "--media-id") {
		t.Errorf("error = %v, quería el motivo en el JSON", doc["error"])
	}
}

func TestMutateGuardaElProgreso(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"mutate", "--json", "--data-dir", dir, "--media-id", "9", "--progress", "4", "--status", "CURRENT"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["mediaId"] != float64(9) || doc["progress"] != float64(4) {
		t.Errorf("mutación = %v, quería mediaId 9 y progress 4", doc)
	}
	if doc["status"] != "CURRENT" {
		t.Errorf("status = %v, quería CURRENT", doc["status"])
	}
	vars := falso.variablesDe(t, "SaveMediaListEntry")
	if vars["mediaId"] != float64(9) || vars["progress"] != float64(4) || vars["status"] != "CURRENT" {
		t.Errorf("variables de la mutación = %v, quería mediaId 9, progress 4 y status CURRENT", vars)
	}
}

func TestMutateEnTextoMuestraLoGuardado(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"mutate", "--data-dir", dir, "--media-id", "9", "--progress", "4"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "guardado: media 9 → 4") {
		t.Errorf("no muestra lo guardado:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// discover
// ---------------------------------------------------------------------------

func TestDiscoverUsaElKindPedido(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"discover", "--json", "--data-dir", dir, "--kind", "top", "--page", "2", "--per-page", "10"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["kind"] != "top" {
		t.Errorf("kind = %v, quería top", doc["kind"])
	}
	items, ok := doc["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v, quería un item", doc["items"])
	}
	vars := falso.variablesDe(t, "media(sort:")
	sorts, ok := vars["sort"].([]any)
	if !ok || len(sorts) != 1 || sorts[0] != "SCORE_DESC" {
		t.Errorf("sort = %v, quería SCORE_DESC (el de top)", vars["sort"])
	}
	if vars["page"] != float64(2) || vars["perPage"] != float64(10) {
		t.Errorf("variables = %v, quería page 2 y perPage 10", vars)
	}
}

func TestDiscoverSeasonalPideLaTemporada(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"discover", "--json", "--data-dir", dir, "--kind", "seasonal"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["kind"] != "seasonal" {
		t.Errorf("kind = %v, quería seasonal", doc["kind"])
	}
	vars := falso.variablesDe(t, "media(sort:")
	if vars["season"] == nil || vars["seasonYear"] == nil {
		t.Errorf("variables = %v, quería season y seasonYear", vars)
	}
	// La temporada es la del momento: el año tiene que ser el actual.
	if vars["seasonYear"] != float64(time.Now().Year()) {
		t.Errorf("seasonYear = %v, quería %d", vars["seasonYear"], time.Now().Year())
	}
	sorts, _ := vars["sort"].([]any)
	if len(sorts) != 1 || sorts[0] != "POPULARITY_DESC" {
		t.Errorf("sort = %v, quería POPULARITY_DESC (seasonal es popular filtrado)", vars["sort"])
	}
}

func TestDiscoverConCoversDirDescargaLasCaratulas(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)
	cubiertas := filepath.Join(t.TempDir(), "cubiertas")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"discover", "--json", "--data-dir", dir, "--covers-dir", cubiertas})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	mapa, ok := doc["covers"].(map[string]any)
	if !ok || mapa["9"] == nil {
		t.Fatalf("covers = %v, quería la entrada 9", doc["covers"])
	}
	if _, err := os.Stat(filepath.Join(cubiertas, "9.jpg")); err != nil {
		t.Errorf("no descargó la carátula del catálogo: %v", err)
	}
}

func TestDiscoverEnTextoMuestraComoMaximoDoceItems(t *testing.T) {
	falso := instalarAniListFalso(t)
	falso.responder = func(query string, variables map[string]any) (string, int) {
		if !strings.Contains(query, "media(sort:") {
			return responderPorDefecto(query, variables)
		}
		var items []string
		for i := 1; i <= 20; i++ {
			n := strconv.Itoa(i)
			items = append(items, `{"id":`+n+`,"title":{"userPreferred":"Item `+n+`"},`+
				`"coverImage":{"extraLarge":"https://cdn.test/`+n+`.jpg","large":"","color":"#000"},`+
				`"averageScore":80,"episodes":12,"status":"RELEASING","format":"TV","season":"SUMMER",`+
				`"seasonYear":2024,"genres":[],"description":""}`)
		}
		return `{"data":{"Page":{"pageInfo":{"hasNextPage":true,"total":20},"media":[` +
			strings.Join(items, ",") + `]}}}`, 0
	}
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"discover", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "Item 1") || !strings.Contains(salida, "Item 12") {
		t.Errorf("no muestra los primeros doce items:\n%s", salida)
	}
	if strings.Contains(salida, "Item 13") {
		t.Errorf("mostró más de doce items:\n%s", salida)
	}
}

func TestDiscoverConKindInvalidoFalla(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, _, errores := capturar(t, func() int {
		return Run([]string{"discover", "--data-dir", dir, "--kind", "inventado"})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errores, "kind inválido") {
		t.Errorf("stderr = %q, quería el motivo del kind inválido", errores)
	}
}

func TestDiscoverEnTextoMuestraElCatalogo(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"discover", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "trending · página 1 · 1 items (total 42)") {
		t.Errorf("no muestra el encabezado del catálogo:\n%s", salida)
	}
	if !strings.Contains(salida, "Tendencia del Mes") {
		t.Errorf("no muestra los items:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// stats
// ---------------------------------------------------------------------------

func TestStatsDevuelveLosTotalesEnUnSoloJSON(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["user"] != "b3low" {
		t.Errorf("user = %v, quería b3low", doc["user"])
	}
	anime, ok := doc["anime"].(map[string]any)
	if !ok || anime["count"] != float64(10) {
		t.Errorf("anime = %v, quería count 10", doc["anime"])
	}
	manga, ok := doc["manga"].(map[string]any)
	if !ok || manga["count"] != float64(2) {
		t.Errorf("manga = %v, quería count 2", doc["manga"])
	}
}

func TestStatsConsultaElUsuarioPedido(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, _, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir, "--user", "otro"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	vars := falso.variablesDe(t, "statistics {")
	if vars["name"] != "otro" {
		t.Errorf("name = %v, quería otro", vars["name"])
	}
}

func TestStatsConErrorDejaStdoutVacio(t *testing.T) {
	falso := instalarAniListFalso(t)
	falso.responder = func(string, map[string]any) (string, int) {
		return `{"errors":[{"message":"Not Found."}]}`, 0
	}
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	// El plugin lee stdout como JSON: un objeto de error lo rompe.
	if salida != "" {
		t.Errorf("stdout = %q, quería vacío (el error va a stderr)", salida)
	}
	if !strings.Contains(errores, "error:") {
		t.Errorf("stderr = %q, quería el motivo del error", errores)
	}
}

func TestStatsSirveElCacheSinPreguntarALaAPI(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)
	crudo, err := json.Marshal(anilist.Stats{
		User:      "b3low",
		Anime:     anilist.AnimeStats{Count: 42, MinutesWatched: 1440},
		Manga:     anilist.MangaStats{Count: 3},
		FetchedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(dir, "stats.json"), string(crudo))
	// El caché sólo se sirve si se puede verificar su dueño: library.json es
	// la pista de a quién pertenece (M8).
	lib, err := json.Marshal(anilist.Library{Viewer: anilist.Viewer{ID: 1, Name: "b3low"}})
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(dir, "library.json"), string(lib))

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	anime, _ := doc["anime"].(map[string]any)
	if anime["count"] != float64(42) {
		t.Errorf("anime = %v, quería el del caché (count 42)", doc["anime"])
	}
	if len(falso.consultasHechas()) != 0 {
		t.Errorf("hubo %d consultas a la API, quería 0 (el caché alcanza)", len(falso.consultasHechas()))
	}
}

func TestStatsEnTextoMuestraLosTotalesYLosGeneros(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"stats", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "usuario: b3low") {
		t.Errorf("no muestra el usuario:\n%s", salida)
	}
	if !strings.Contains(salida, "géneros: Action 5") {
		t.Errorf("no muestra los géneros:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// media
// ---------------------------------------------------------------------------

func TestMediaExigeElMediaID(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"media", "--json", "--data-dir", dir})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["error"] == nil || !strings.Contains(doc["error"].(string), "--media-id") {
		t.Errorf("error = %v, quería el motivo en el JSON", doc["error"])
	}
}

func TestMediaDevuelveLaFichaEnUnSoloJSON(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"media", "--json", "--data-dir", dir, "--media-id", "9"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["title"] != "Serie de Prueba" {
		t.Errorf("title = %v, quería Serie de Prueba", doc["title"])
	}
	generos, ok := doc["genres"].([]any)
	if !ok || len(generos) != 1 || generos[0] != "Drama" {
		t.Errorf("genres = %v, quería [Drama]", doc["genres"])
	}
	if doc["description"] != "Texto" {
		t.Errorf("description = %v, quería el texto sin HTML", doc["description"])
	}
	proximo, ok := doc["nextAiring"].(map[string]any)
	if !ok || proximo["episode"] != float64(7) {
		t.Errorf("nextAiring = %v, quería el capítulo 7", doc["nextAiring"])
	}
	vars := falso.variablesDe(t, "Media(id: $id)")
	if vars["id"] != float64(9) {
		t.Errorf("id = %v, quería 9", vars["id"])
	}
}

func TestMediaEnTextoMuestraLaFicha(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"media", "--data-dir", dir, "--media-id", "9"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "Serie de Prueba (9)") {
		t.Errorf("no muestra el título:\n%s", salida)
	}
	if !strings.Contains(salida, "géneros: Drama") {
		t.Errorf("no muestra los géneros:\n%s", salida)
	}
	if !strings.Contains(salida, "próximo: ep 7") {
		t.Errorf("no muestra el próximo capítulo:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// schedule
// ---------------------------------------------------------------------------

func TestScheduleDevuelveLaAgendaEnUnSoloJSON(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"schedule", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	items, ok := doc["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v, quería un capítulo", doc["items"])
	}
	item, _ := items[0].(map[string]any)
	if item["episode"] != float64(7) || item["mediaId"] != float64(9) {
		t.Errorf("item = %v, quería el capítulo 7 de la media 9", item)
	}
	// La agenda son dos consultas: la lista en curso y los capítulos.
	if len(falso.consultasHechas()) < 2 {
		t.Errorf("consultas = %d, quería 2 (la lista en curso y la agenda)", len(falso.consultasHechas()))
	}
}

func TestScheduleEnTextoMuestraLosCapitulos(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"schedule", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "agenda: 1 capítulos") {
		t.Errorf("no muestra el encabezado de la agenda:\n%s", salida)
	}
	if !strings.Contains(salida, "Serie de Prueba ep 7") {
		t.Errorf("no muestra los capítulos:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Fallas de la API: el comando tiene que salir con 1 y el motivo en stderr
// ---------------------------------------------------------------------------

func TestComandoQueConsultaLaApiFallaConCodigoUno(t *testing.T) {
	casos := []struct {
		comando []string
	}{
		{comando: []string{"sync", "--json"}},
		{comando: []string{"library", "--json"}},
		{comando: []string{"covers", "--json"}},
		{comando: []string{"mutate", "--json", "--media-id", "9", "--progress", "1"}},
		{comando: []string{"discover", "--json"}},
		{comando: []string{"media", "--json", "--media-id", "9"}},
		{comando: []string{"schedule", "--json"}},
	}
	for _, caso := range casos {
		t.Run(caso.comando[0], func(t *testing.T) {
			falso := instalarAniListFalso(t)
			falso.responder = func(string, map[string]any) (string, int) {
				return `{"errors":[{"message":"Not Found."}]}`, 0
			}
			sinTokenDeEntorno(t)
			dir := t.TempDir()
			escribirToken(t, dir)

			args := append([]string{caso.comando[0], "--data-dir", dir}, caso.comando[1:]...)
			codigo, salida, errores := capturar(t, func() int { return Run(args) })
			if codigo != 1 {
				t.Errorf("código = %d, quería 1", codigo)
			}
			// Con --json el error sale como documento JSON en stdout: el
			// plugin lo distingue por el código de salida, no por el cuerpo.
			doc := afirmarUnSoloJSON(t, salida)
			if doc["error"] == nil {
				t.Errorf("error = %v, quería el motivo en el JSON", doc["error"])
			}
			if errores != "" {
				t.Errorf("stderr = %q, wanted vacío", errores)
			}
		})
	}
}

// TestSinTokenTodosLosComandosFallanConCodigoUno cubre el camino de error
// de client(): sin token ningún comando que consulta la API puede correr.
func TestSinTokenTodosLosComandosFallanConCodigoUno(t *testing.T) {
	casos := [][]string{
		{"sync"},
		{"library"},
		{"covers"},
		{"mutate", "--media-id", "9", "--progress", "1"},
		{"discover"},
		{"stats"},
		{"media", "--media-id", "9"},
		{"schedule"},
	}
	for _, caso := range casos {
		t.Run(caso[0], func(t *testing.T) {
			instalarAniListFalso(t)
			sinTokenDeEntorno(t)
			dir := t.TempDir()

			args := append([]string{caso[0], "--data-dir", dir}, caso[1:]...)
			codigo, _, errores := capturar(t, func() int { return Run(args) })
			if codigo != 1 {
				t.Errorf("código = %d, quería 1", codigo)
			}
			if !strings.Contains(errores, "no hay token") {
				t.Errorf("stderr = %q, quería el motivo de la falta de token", errores)
			}
		})
	}
}

// TestEmitFallaSiElJSONNoSePuedeCodificar cubre el error del encoder: un
// valor no serializable tiene que salir con 1 y el motivo en stderr.
func TestEmitFallaSiElJSONNoSePuedeCodificar(t *testing.T) {
	codigo, salida, errores := capturar(t, func() int {
		return emit(map[string]any{"invalido": math.NaN()}, true, func() {
			t.Error("no debía imprimir la salida humana")
		})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if salida != "" {
		t.Errorf("stdout = %q, quería vacío (nada se pudo codificar)", salida)
	}
	if !strings.Contains(errores, "error de salida:") {
		t.Errorf("stderr = %q, quería el error de salida", errores)
	}
}

// TestStatusUsaElDataDirPorDefecto comprueba que sin --data-dir el comando
// usa el directorio del plugin, no el de datos vacío.
func TestStatusUsaElDataDirPorDefecto(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	codigo, salida, errores := capturar(t, func() int { return Run([]string{"status", "--json"}) })
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	esperado := filepath.Join(home, ".local", "state", "noctalia", "plugins", "data", "b3l0w", "anilist")
	if doc["dataDir"] != esperado {
		t.Errorf("dataDir = %v, quería %q", doc["dataDir"], esperado)
	}
	if doc["coversDir"] != filepath.Join(esperado, "covers") {
		t.Errorf("coversDir = %v, quería %q", doc["coversDir"], filepath.Join(esperado, "covers"))
	}
}

// TestCoversConArchivoDeBibliotecaInvalidoFalla cubre el error del parser del
// archivo de biblioteca.
func TestCoversConArchivoDeBibliotecaInvalidoFalla(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	archivo := filepath.Join(t.TempDir(), "library.json")
	escribir(t, archivo, `{esto no es json`)

	codigo, _, errores := capturar(t, func() int {
		return Run([]string{"covers", "--data-dir", t.TempDir(), "--library", archivo})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errores, "error:") {
		t.Errorf("stderr = %q, quería el motivo del error", errores)
	}
}

// TestFallaSiNoPuedeCrearElDirectorioDeCubiertas cubre el error de
// SyncCovers en los tres comandos que bajan carátulas: si el directorio no se
// puede crear (hay un archivo en su lugar), el comando sale con 1.
func TestFallaSiNoPuedeCrearElDirectorioDeCubiertas(t *testing.T) {
	casos := []struct {
		comando string
		// args arma la invocación; bloqueado es el path que se ocupa con un
		// archivo para que MkdirAll falle.
		args func(dir, bloqueado string) []string
	}{
		{"sync", func(dir, _ string) []string {
			return []string{"sync", "--data-dir", dir}
		}},
		{"covers", func(dir, bloqueado string) []string {
			archivo := filepath.Join(t.TempDir(), "library.json")
			escribir(t, archivo, `{"anime":[{"mediaId":11,"coverUrl":"https://cdn.test/11.jpg"}],"manga":[]}`)
			return []string{"covers", "--data-dir", dir, "--library", archivo, "--covers-dir", bloqueado}
		}},
		{"discover", func(dir, bloqueado string) []string {
			return []string{"discover", "--data-dir", dir, "--covers-dir", bloqueado}
		}},
	}
	for _, caso := range casos {
		t.Run(caso.comando, func(t *testing.T) {
			instalarAniListFalso(t)
			sinTokenDeEntorno(t)
			dir := t.TempDir()
			escribirToken(t, dir)
			// Un archivo donde debería ir el directorio de carátulas.
			bloqueado := filepath.Join(dir, "covers")
			escribir(t, bloqueado, "x")

			codigo, _, errores := capturar(t, func() int {
				return Run(caso.args(dir, bloqueado))
			})
			if codigo != 1 {
				t.Errorf("código = %d, quería 1", codigo)
			}
			if !strings.Contains(errores, "error:") {
				t.Errorf("stderr = %q, quería el motivo del error", errores)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func TestStatusReportaElTokenYElCache(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	cubiertas := filepath.Join(dir, "covers")
	if err := os.MkdirAll(cubiertas, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(cubiertas, "1.jpg"), "0123456789")
	escribir(t, filepath.Join(cubiertas, "notas.txt"), "x")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"status", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["hasToken"] != false {
		t.Errorf("hasToken = %v, quería false (no hay token.json)", doc["hasToken"])
	}
	if doc["covers"] != float64(1) {
		t.Errorf("covers = %v, quería 1 (sólo los .jpg cuentan)", doc["covers"])
	}
	if doc["coversBytes"] != float64(10) {
		t.Errorf("coversBytes = %v, quería 10", doc["coversBytes"])
	}
	if doc["coversDir"] != cubiertas {
		t.Errorf("coversDir = %v, quería %q", doc["coversDir"], cubiertas)
	}
}

func TestStatusDetectaElTokenEnElDisco(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"status", "--json", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["hasToken"] != true {
		t.Errorf("hasToken = %v, quería true (hay token.json)", doc["hasToken"])
	}
}

func TestStatusAceptaElTokenExplicito(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	// El flagset de status usa ExitOnError: un flag desconocido mata el
	// proceso. Por eso el comando corre en un subproceso.
	codigo, salida, errores := ejecutarEnSubproceso(t, "status", "--json", "--data-dir", dir, "--token", "token-de-prueba")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["hasToken"] != true {
		t.Errorf("hasToken = %v, quería true (el token vino por flag)", doc["hasToken"])
	}
}

func TestStatusEnTextoMuestraElEstado(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, salida, _ := capturar(t, func() int {
		return Run([]string{"status", "--data-dir", dir})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "datos:   "+dir) {
		t.Errorf("no muestra el data dir:\n%s", salida)
	}
	if !strings.Contains(salida, "token:   false") {
		t.Errorf("no muestra el estado del token:\n%s", salida)
	}
	if !strings.Contains(salida, "caché:") {
		t.Errorf("no muestra el caché:\n%s", salida)
	}
}

// ---------------------------------------------------------------------------
// Flags inválidos: los flagsets usan ExitOnError, así que un flag desconocido
// mata el proceso con código 2. Por eso el comando corre en un subproceso.
// ---------------------------------------------------------------------------

const envHelper = "B3L0W_ANILIST_CLI_HELPER"
const envArgs = "B3L0W_ANILIST_CLI_ARGS"

// TestHelperProceso sólo corre cuando lo invoca ejecutarEnSubproceso.
func TestHelperProceso(t *testing.T) {
	if os.Getenv(envHelper) != "1" {
		t.Skip("helper: sólo corre como subproceso")
	}
	var args []string
	if crudo := os.Getenv(envArgs); crudo != "" {
		if err := json.Unmarshal([]byte(crudo), &args); err != nil {
			os.Exit(99)
		}
	}
	os.Exit(Run(args))
}

// ejecutarEnSubproceso corre Run en un proceso aparte y devuelve su código de
// salida junto con stdout y stderr.
func ejecutarEnSubproceso(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	crudo, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProceso$")
	cmd.Env = append(os.Environ(), envHelper+"=1", envArgs+"="+string(crudo))
	var salida, errores bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &errores
	if err := cmd.Run(); err != nil {
		var salidaErr *exec.ExitError
		if !errors.As(err, &salidaErr) {
			t.Fatalf("no se pudo correr el subproceso: %v", err)
		}
		return salidaErr.ExitCode(), salida.String(), errores.String()
	}
	return 0, salida.String(), errores.String()
}

func TestFlagInvalidoSaleConCodigoDos(t *testing.T) {
	comandos := []string{"sync", "library", "covers", "mutate", "discover", "stats", "status", "media", "schedule"}
	for _, comando := range comandos {
		t.Run(comando, func(t *testing.T) {
			codigo, salida, errores := ejecutarEnSubproceso(t, comando, "--no-existe")
			if codigo != 2 {
				t.Errorf("código = %d, quería 2 (flag inválido)", codigo)
			}
			if !strings.Contains(errores, "flag provided but not defined") {
				t.Errorf("stderr = %q, quería el error del flag", errores)
			}
			if strings.Contains(errores, "comando desconocido") {
				t.Errorf("stderr = %q, quería el comando despacho", errores)
			}
			if salida != "" {
				t.Errorf("stdout = %q, quería vacío", salida)
			}
		})
	}
}

func TestAyudaDeUnSubcomandoSaleConCodigoCero(t *testing.T) {
	codigo, _, errores := ejecutarEnSubproceso(t, "sync", "--help")
	if codigo != 0 {
		t.Errorf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(errores, "Usage of sync") {
		t.Errorf("stderr = %q, quería el uso del subcomando", errores)
	}
}

// ---------------------------------------------------------------------------
// La tabla de despacho tiene que seguir al pie de la letra
// ---------------------------------------------------------------------------

// comandosDelUso saca los subcomandos que anuncia el texto de ayuda.
func comandosDelUso() []string {
	var comandos []string
	for _, linea := range strings.Split(usage, "\n") {
		if !strings.HasPrefix(linea, "  ") || strings.HasPrefix(strings.TrimSpace(linea), "-") {
			continue
		}
		partes := strings.Fields(linea)
		if len(partes) == 0 {
			continue
		}
		comandos = append(comandos, partes[0])
	}
	return comandos
}

// TestCadaSubcomandoDelUsoTieneUnTest falla si se agrega un subcomando al
// texto de ayuda sin escribirle un test.
func TestCadaSubcomandoDelUsoTieneUnTest(t *testing.T) {
	existentes := listarTests(t)
	porSubcomando := map[string]string{
		"sync":     "TestSyncDevuelveBibliotecaYCubiertasEnUnSoloJSON",
		"library":  "TestLibraryDevuelveLaBibliotecaEnUnSoloJSON",
		"covers":   "TestCoversDesdeElArchivoDeBibliotecaNoPideToken",
		"mutate":   "TestMutateGuardaElProgreso",
		"discover": "TestDiscoverUsaElKindPedido",
		"stats":    "TestStatsDevuelveLosTotalesEnUnSoloJSON",
		"status":   "TestStatusReportaElTokenYElCache",
		"media":    "TestMediaDevuelveLaFichaEnUnSoloJSON",
		"schedule": "TestScheduleDevuelveLaAgendaEnUnSoloJSON",
	}
	var faltan []string
	for _, comando := range comandosDelUso() {
		nombre, ok := porSubcomando[comando]
		if !ok {
			faltan = append(faltan, comando+" (sin test asignado)")
			continue
		}
		if !contiene(existentes, nombre) {
			faltan = append(faltan, comando+" (falta "+nombre+")")
		}
	}
	if len(faltan) > 0 {
		t.Errorf("subcomandos sin test: %v", faltan)
	}
}

// listarTests devuelve los nombres de los tests del paquete.
func listarTests(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.list=^Test")
	var salida bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &salida
	if err := cmd.Run(); err != nil {
		t.Fatalf("no se pudo listar los tests: %v", err)
	}
	var nombres []string
	for _, linea := range strings.Split(salida.String(), "\n") {
		linea = strings.TrimSpace(linea)
		if strings.HasPrefix(linea, "Test") {
			nombres = append(nombres, linea)
		}
	}
	return nombres
}

func contiene(lista []string, valor string) bool {
	for _, item := range lista {
		if item == valor {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Flags de carátulas: --reuse y --gc
// ---------------------------------------------------------------------------

func TestSyncAdoptaElCacheViejoConReuse(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	// Caché viejo: la carátula de la serie 1 con su meta válido.
	viejo := filepath.Join(t.TempDir(), "cache-viejo")
	if err := os.MkdirAll(viejo, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(viejo, "1.jpg"), "JPEG-viejo")
	escribir(t, filepath.Join(viejo, "1.jpg.meta.json"), `{"url":"https://cdn.test/1.jpg","version":2}`)
	nuevo := filepath.Join(t.TempDir(), "cache-nuevo")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir, "--covers-dir", nuevo, "--reuse", viejo})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	cubiertas, ok := doc["covers"].(map[string]any)
	if !ok || cubiertas["1"] == nil {
		t.Fatalf("covers = %v, quería la entrada 1 adoptada", doc["covers"])
	}
	if _, err := os.Stat(filepath.Join(nuevo, "1.jpg")); err != nil {
		t.Errorf("no adoptó la carátula del caché viejo: %v", err)
	}
	stats, ok := doc["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats = %v, quería el resumen de carátulas", doc["stats"])
	}
	if stats["downloaded"] != float64(1) {
		t.Errorf("downloaded = %v, quería 1 (sólo la serie 2 se descarga)", stats["downloaded"])
	}
	if stats["reused"] == float64(0) {
		t.Errorf("reused = %v, quería > 0 (adoptó el caché viejo)", stats["reused"])
	}
}

func TestSyncConGcBorraLasHuerfanas(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	// Una carátula que no está en la biblioteca: es huérfana.
	cubiertas := filepath.Join(dir, "covers")
	if err := os.MkdirAll(cubiertas, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(cubiertas, "999.jpg"), "JPEG-huerfano")

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir, "--gc"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	stats, ok := doc["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats = %v, quería el resumen de carátulas", doc["stats"])
	}
	if stats["removed"] != float64(1) {
		t.Errorf("removed = %v, quería 1 (borró la huérfana)", stats["removed"])
	}
	if _, err := os.Stat(filepath.Join(cubiertas, "999.jpg")); !os.IsNotExist(err) {
		t.Errorf("la carátula huérfana sigue en disco (err = %v)", err)
	}
}

// ---------------------------------------------------------------------------
// Flags de stats: --refresh y --user
// ---------------------------------------------------------------------------

func TestStatsConRefreshIgnoraElCache(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)
	crudo, err := json.Marshal(anilist.Stats{
		User:      "b3low",
		Anime:     anilist.AnimeStats{Count: 42},
		FetchedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(dir, "stats.json"), string(crudo))

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir, "--refresh"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	anime, _ := doc["anime"].(map[string]any)
	if anime["count"] != float64(10) {
		t.Errorf("anime = %v, quería el de la API (count 10), no el del caché (42)", doc["anime"])
	}
	if len(falso.consultasHechas()) == 0 {
		t.Errorf("no consultó la API: --refresh tiene que ignorar el caché")
	}
}

// ---------------------------------------------------------------------------
// Cada kind de discover manda su propio sort
// ---------------------------------------------------------------------------

func TestDiscoverCadaKindMandaSuSort(t *testing.T) {
	casos := []struct {
		kind string
		sort string
	}{
		{"trending", "TRENDING_DESC"},
		{"popular", "POPULARITY_DESC"},
		{"new", "START_DATE_DESC"},
	}
	for _, caso := range casos {
		t.Run(caso.kind, func(t *testing.T) {
			falso := instalarAniListFalso(t)
			sinTokenDeEntorno(t)
			dir := t.TempDir()
			escribirToken(t, dir)

			codigo, salida, errores := capturar(t, func() int {
				return Run([]string{"discover", "--json", "--data-dir", dir, "--kind", caso.kind})
			})
			if codigo != 0 {
				t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
			}
			doc := afirmarUnSoloJSON(t, salida)
			if doc["kind"] != caso.kind {
				t.Errorf("kind = %v, quería %s", doc["kind"], caso.kind)
			}
			vars := falso.variablesDe(t, "media(sort:")
			sorts, ok := vars["sort"].([]any)
			if !ok || len(sorts) != 1 || sorts[0] != caso.sort {
				t.Errorf("sort = %v, quería %s (el de %s)", vars["sort"], caso.sort, caso.kind)
			}
		})
	}
}
