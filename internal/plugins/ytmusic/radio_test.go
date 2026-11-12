package ytmusic

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionFixture arma una sesión en disco sin tocar la red.
func sessionFixture(t *testing.T, dir string, session Session) {
	t.Helper()
	if err := SaveSession(dir, session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
}

// loadRawSession lee el JSON de la sesión tal como está en disco, sin pasar
// por LoadSession (que valida el cursor).
func loadRawSession(t *testing.T, dir, id string) Session {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "sessions", id+".json"))
	if err != nil {
		t.Fatalf("leyendo la sesión: %v", err)
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		t.Fatalf("sesión ilegible: %v", err)
	}
	return session
}

// ── creación ─────────────────────────────────────────────────────────────

// TestRadioCreaLaSesionYLaGuarda: la cola del watch-next queda persistida y
// el puntero a la sesión activa apunta a ella.
func TestRadioCreaLaSesionYLaGuarda(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("seed01", "Thriller", "Michael Jackson", "5:58"),
			panelSong("song02", "Billie Jean", "Michael Jackson", "4:54"),
		}, "RADIO-NEXT"))
	})
	dir := t.TempDir()
	session, err := Radio(context.Background(), fake.client, dir, "seed01", "")
	if err != nil {
		t.Fatalf("Radio: %v", err)
	}
	if session.ID == "" {
		t.Error("la sesión no tiene id")
	}
	if session.Seed != "seed01" {
		t.Errorf("seed = %q", session.Seed)
	}
	if session.PlaylistID != "RDAMVMseed01" {
		t.Errorf("playlistId = %q, quería RDAMVMseed01", session.PlaylistID)
	}
	if session.Cursor != 0 {
		t.Errorf("cursor = %d, quería 0", session.Cursor)
	}
	if len(session.Queue) != 2 {
		t.Fatalf("cola de %d canciones, quería 2", len(session.Queue))
	}
	if session.Continuation != "RADIO-NEXT" {
		t.Errorf("continuation = %q", session.Continuation)
	}
	// El puntero a la sesión activa tiene que existir.
	pointer := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatalf("no se escribió current.json: %v", err)
	}
	if err := json.Unmarshal(raw, &pointer); err != nil {
		t.Fatalf("current.json ilegible: %v", err)
	}
	if pointer["session"] != session.ID {
		t.Errorf("current.json apunta a %q, quería %q", pointer["session"], session.ID)
	}
}

// TestRadioDeduplicaLaCola: el seed vuelve a aparecer más abajo.
func TestRadioDeduplicaLaCola(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("seed01", "Thriller", "Michael Jackson", "5:58"),
			panelSong("otro", "Otra", "Artista", "3:00"),
			panelSong("seed01", "Thriller", "Michael Jackson", "5:58"),
		}, ""))
	})
	session, err := Radio(context.Background(), fake.client, t.TempDir(), "seed01", "")
	if err != nil {
		t.Fatalf("Radio: %v", err)
	}
	if len(session.Queue) != 2 {
		t.Errorf("cola de %d canciones, quería 2 (el seed repetido se va)", len(session.Queue))
	}
}

// TestRadioSinVideoIDEsError: sin semilla no hay radio.
func TestRadioSinVideoIDEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar la red sin videoId")
	})
	if _, err := Radio(context.Background(), fake.client, t.TempDir(), "   ", ""); err == nil {
		t.Fatal("un videoId vacío tiene que ser error")
	}
}

// TestRadioSinCancionesEsError: un watch-next vacío es un error, no una cola
// vacía.
func TestRadioSinCancionesEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload(nil, ""))
	})
	dir := t.TempDir()
	if _, err := Radio(context.Background(), fake.client, dir, "seed01", ""); err == nil {
		t.Fatal("una radio sin canciones tiene que ser error")
	}
	// No debe quedar una sesión a medias en disco.
	if _, err := os.Stat(filepath.Join(dir, "current.json")); !os.IsNotExist(err) {
		t.Errorf("dejó current.json sin sesión: %v", err)
	}
}

// TestRadioRespetaElIDDeSesion: el panel puede fijar el id para reemplazar la
// cola anterior.
func TestRadioRespetaElIDDeSesion(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("seed01", "Thriller", "Michael Jackson", "5:58"),
		}, ""))
	})
	session, err := Radio(context.Background(), fake.client, t.TempDir(), "seed01", "s-fijo")
	if err != nil {
		t.Fatalf("Radio: %v", err)
	}
	if session.ID != "s-fijo" {
		t.Errorf("id = %q, quería s-fijo", session.ID)
	}
}

// ── carga y validación del cursor ────────────────────────────────────────

// TestLoadSessionSinSesionActivaEsError: no hay puntero, no hay sesión.
func TestLoadSessionSinSesionActivaEsError(t *testing.T) {
	if _, err := LoadSession(t.TempDir(), ""); err == nil {
		t.Fatal("sin current.json tiene que ser error")
	}
}

// TestLoadSessionDesconocidaEsError: el id no existe en disco.
func TestLoadSessionDesconocidaEsError(t *testing.T) {
	_, err := LoadSession(t.TempDir(), "no-existe")
	if err == nil {
		t.Fatal("una sesión desconocida tiene que ser error")
	}
	if !strings.Contains(err.Error(), "no-existe") {
		t.Errorf("el error no nombra la sesión: %v", err)
	}
}

// TestLoadSessionRechazaCursorFueraDeRango: el cursor viene de un JSON en
// disco. Si quedó fuera de la cola, indexar revienta con un panic. Tiene que
// ser un error limpio.
func TestLoadSessionRechazaCursorFueraDeRango(t *testing.T) {
	cases := []struct {
		name   string
		cursor int
		queue  int
	}{
		{"cursor pasado", 5, 2},
		{"cursor en el límite", 2, 2},
		{"cursor negativo", -1, 3},
		{"cola vacía", 0, 0},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			dir := t.TempDir()
			queue := make([]Song, item.queue)
			for index := range queue {
				queue[index] = Song{VideoID: string(rune('a' + index))}
			}
			sessionFixture(t, dir, Session{
				ID:     "s1",
				Seed:   "seed01",
				Queue:  queue,
				Cursor: item.cursor,
			})
			_, err := LoadSession(dir, "s1")
			if err == nil {
				t.Fatal("un cursor fuera de rango tiene que ser error, no un panic")
			}
			if !strings.Contains(err.Error(), "cursor") {
				t.Errorf("el error no menciona el cursor: %v", err)
			}
		})
	}
}

// TestLoadSessionAceptaElCursorValido: el último índice es válido.
func TestLoadSessionAceptaElCursorValido(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:     "s1",
		Seed:   "seed01",
		Queue:  []Song{{VideoID: "a"}, {VideoID: "b"}},
		Cursor: 1,
	})
	session, err := LoadSession(dir, "s1")
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if session.Cursor != 1 {
		t.Errorf("cursor = %d, quería 1", session.Cursor)
	}
}

// ── next ──────────────────────────────────────────────────────────────────

// TestNextAvanzaElCursor: la operación de la máquina de estados más usada.
func TestNextAvanzaElCursor(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a", Title: "A"}, {VideoID: "b", Title: "B"}, {VideoID: "c", Title: "C"}},
	})
	result, err := Next(context.Background(), nil, dir, "s1")
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if result.Cursor != 1 {
		t.Errorf("cursor = %d, quería 1", result.Cursor)
	}
	if result.Song.VideoID != "b" {
		t.Errorf("canción = %q, quería b", result.Song.VideoID)
	}
	if result.QueueLength != 3 {
		t.Errorf("queueLength = %d, quería 3", result.QueueLength)
	}
	if result.FetchedMore {
		t.Error("no había que traer nada más")
	}
	// El cursor quedó persistido: `next` corre en otro proceso.
	if disk := loadRawSession(t, dir, "s1"); disk.Cursor != 1 {
		t.Errorf("el cursor en disco = %d, quería 1", disk.Cursor)
	}
}

// TestNextPideMasCuandoSeAcabaLaCola: al llegar al final pide la siguiente
// tanda del automix y la agrega.
func TestNextPideMasCuandoSeAcabaLaCola(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("b", "B", "Artista", "3:00"),
			panelSong("c", "C", "Artista", "4:00"),
		}, "RADIO-MAS"))
	})
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:           "s1",
		Seed:         "seed01",
		Queue:        []Song{{VideoID: "a", Title: "A"}},
		Continuation: "RADIO-NEXT",
	})
	result, err := Next(context.Background(), fake.client, dir, "s1")
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !result.FetchedMore {
		t.Error("fetchedMore = false, quería true")
	}
	if result.QueueLength != 3 {
		t.Errorf("queueLength = %d, quería 3", result.QueueLength)
	}
	if result.Cursor != 1 {
		t.Errorf("cursor = %d, quería 1", result.Cursor)
	}
	if result.Song.VideoID != "b" {
		t.Errorf("canción = %q, quería b", result.Song.VideoID)
	}
	if len(fake.calls()) != 1 {
		t.Errorf("hizo %d peticiones, quería 1", len(fake.calls()))
	}
	// La continuación nueva quedó guardada.
	if disk := loadRawSession(t, dir, "s1"); disk.Continuation != "RADIO-MAS" {
		t.Errorf("continuation en disco = %q, quería RADIO-MAS", disk.Continuation)
	}
}

// TestNextIgnoraLasCancionesRepetidas: la radio devuelve el seed otra vez.
func TestNextIgnoraLasCancionesRepetidas(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("a", "A", "Artista", "3:00"),
			panelSong("b", "B", "Artista", "3:00"),
		}, ""))
	})
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:           "s1",
		Seed:         "seed01",
		Queue:        []Song{{VideoID: "a", Title: "A"}},
		Continuation: "RADIO-NEXT",
	})
	result, err := Next(context.Background(), fake.client, dir, "s1")
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if result.QueueLength != 2 {
		t.Errorf("queueLength = %d, quería 2 (la repetida no cuenta)", result.QueueLength)
	}
	if result.Song.VideoID != "b" {
		t.Errorf("canción = %q, quería b", result.Song.VideoID)
	}
}

// TestNextSiembraUnaRadioNuevaCuandoLaOriginalSeAgota: rendirse deja el
// botón "siguiente" muerto a los 50 temas.
func TestNextSiembraUnaRadioNuevaCuandoLaOriginalSeAgota(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		videoID, _ := body["videoId"].(string)
		if videoID == "ultima" {
			jsonResponse(w, radioPayload([]any{
				panelSong("nueva1", "Nueva 1", "Artista", "3:00"),
				panelSong("nueva2", "Nueva 2", "Artista", "3:00"),
			}, "OTRO-TOKEN"))
			return
		}
		jsonResponse(w, radioPayload(nil, ""))
	})
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "ultima", Title: "Última"}},
	})
	result, err := Next(context.Background(), fake.client, dir, "s1")
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !result.FetchedMore {
		t.Error("fetchedMore = false, quería true")
	}
	if result.QueueLength != 3 {
		t.Errorf("queueLength = %d, quería 3", result.QueueLength)
	}
	if result.Song.VideoID != "nueva1" {
		t.Errorf("canción = %q, quería nueva1", result.Song.VideoID)
	}
	// La sesión se re-siembra desde la última canción.
	if disk := loadRawSession(t, dir, "s1"); disk.Seed != "ultima" {
		t.Errorf("seed en disco = %q, quería ultima", disk.Seed)
	}
}

// TestNextFallaSiNoTraeCancionesNuevas: ni la continuación ni la radio nueva
// traen nada: es un error explícito, no un cursor que se queda pegado.
func TestNextFallaSiNoTraeCancionesNuevas(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload(nil, ""))
	})
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a", Title: "A"}},
	})
	if _, err := Next(context.Background(), fake.client, dir, "s1"); err == nil {
		t.Fatal("una radio que no trae nada tiene que ser error")
	}
}

// TestNextSinCancionesEsError: la cola vacía no se puede avanzar.
func TestNextSinCancionesEsError(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{ID: "s1", Seed: "seed01", Queue: []Song{}})
	if _, err := Next(context.Background(), nil, dir, "s1"); err == nil {
		t.Fatal("una cola vacía tiene que ser error")
	}
}

// TestNextConSesionDesconocidaEsError.
func TestNextConSesionDesconocidaEsError(t *testing.T) {
	if _, err := Next(context.Background(), nil, t.TempDir(), "no-existe"); err == nil {
		t.Fatal("una sesión desconocida tiene que ser error")
	}
}

// ── previous ──────────────────────────────────────────────────────────────

// TestPreviousRetrocedeElCursor.
func TestPreviousRetrocedeElCursor(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:     "s1",
		Seed:   "seed01",
		Queue:  []Song{{VideoID: "a", Title: "A"}, {VideoID: "b", Title: "B"}},
		Cursor: 1,
	})
	result, err := Previous(dir, "s1")
	if err != nil {
		t.Fatalf("Previous: %v", err)
	}
	if result.Cursor != 0 {
		t.Errorf("cursor = %d, quería 0", result.Cursor)
	}
	if result.Song.VideoID != "a" {
		t.Errorf("canción = %q, quería a", result.Song.VideoID)
	}
	if disk := loadRawSession(t, dir, "s1"); disk.Cursor != 0 {
		t.Errorf("el cursor en disco = %d, quería 0", disk.Cursor)
	}
}

// TestPreviousEnLaPrimeraEsError: no hay canción anterior.
func TestPreviousEnLaPrimeraEsError(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a", Title: "A"}},
	})
	if _, err := Previous(dir, "s1"); err == nil {
		t.Fatal("volver desde la primera tiene que ser error")
	}
}

// ── shuffle ───────────────────────────────────────────────────────────────

// TestShuffleDejaLaActualPrimero: un shuffle que cambia lo que está sonando
// es una patada, no un shuffle.
func TestShuffleDejaLaActualPrimero(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:     "s1",
		Seed:   "seed01",
		Queue:  []Song{{VideoID: "a"}, {VideoID: "b"}, {VideoID: "c"}, {VideoID: "d"}},
		Cursor: 2,
	})
	session, err := Shuffle(dir, "s1")
	if err != nil {
		t.Fatalf("Shuffle: %v", err)
	}
	if len(session.Queue) != 4 {
		t.Fatalf("cola de %d, quería 4", len(session.Queue))
	}
	if session.Queue[0].VideoID != "c" {
		t.Errorf("la primera canción = %q, quería c (la que sonaba)", session.Queue[0].VideoID)
	}
	if session.Cursor != 0 {
		t.Errorf("cursor = %d, quería 0", session.Cursor)
	}
	// Ninguna canción se perdió ni se duplicó.
	seen := map[string]int{}
	for _, song := range session.Queue {
		seen[song.VideoID]++
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		if seen[id] != 1 {
			t.Errorf("la canción %q aparece %d veces, quería 1", id, seen[id])
		}
	}
}

// TestShuffleConUnaSolaCancionNoCambiaNada.
func TestShuffleConUnaSolaCancionNoCambiaNada(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a"}},
	})
	session, err := Shuffle(dir, "s1")
	if err != nil {
		t.Fatalf("Shuffle: %v", err)
	}
	if len(session.Queue) != 1 || session.Queue[0].VideoID != "a" {
		t.Errorf("la cola cambió: %+v", session.Queue)
	}
	if session.Cursor != 0 {
		t.Errorf("cursor = %d, quería 0", session.Cursor)
	}
}

// TestShuffleConSesionDesconocidaEsError.
func TestShuffleConSesionDesconocidaEsError(t *testing.T) {
	if _, err := Shuffle(t.TempDir(), "no-existe"); err == nil {
		t.Fatal("una sesión desconocida tiene que ser error")
	}
}

// ── enqueue y queue ───────────────────────────────────────────────────────

// TestEnqueueAgregaLaCancion.
func TestEnqueueAgregaLaCancion(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a"}},
	})
	session, err := Enqueue(dir, "s1", Song{VideoID: "z", Title: "Z"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(session.Queue) != 2 {
		t.Fatalf("cola de %d, quería 2", len(session.Queue))
	}
	if session.Queue[1].VideoID != "z" {
		t.Errorf("la agregada no quedó al final: %+v", session.Queue)
	}
	if disk := loadRawSession(t, dir, "s1"); len(disk.Queue) != 2 {
		t.Errorf("en disco hay %d canciones, quería 2", len(disk.Queue))
	}
}

// TestEnqueueIgnoraLosRepetidos: la radio ya tiene esa canción.
func TestEnqueueIgnoraLosRepetidos(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:    "s1",
		Seed:  "seed01",
		Queue: []Song{{VideoID: "a"}, {VideoID: "b"}},
	})
	session, err := Enqueue(dir, "s1", Song{VideoID: "b", Title: "Otra B"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(session.Queue) != 2 {
		t.Errorf("cola de %d, quería 2 (no se duplica)", len(session.Queue))
	}
}

// TestQueueDevuelveLaColaYElCursor.
func TestQueueDevuelveLaColaYElCursor(t *testing.T) {
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:     "s1",
		Seed:   "seed01",
		Queue:  []Song{{VideoID: "a"}, {VideoID: "b"}},
		Cursor: 1,
	})
	view, err := Queue(dir, "s1")
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if view.Session != "s1" || view.Seed != "seed01" {
		t.Errorf("view = %+v", view)
	}
	if len(view.Items) != 2 {
		t.Errorf("items = %d, quería 2", len(view.Items))
	}
	if view.Cursor != 1 {
		t.Errorf("cursor = %d, quería 1", view.Cursor)
	}
}

// TestQueueSinSesionEsError.
func TestQueueSinSesionEsError(t *testing.T) {
	if _, err := Queue(t.TempDir(), "no-existe"); err == nil {
		t.Fatal("una sesión desconocida tiene que ser error")
	}
}

// TestSessionIDEsUnico: dos radios seguidas no pueden compartir id.
func TestSessionIDEsUnico(t *testing.T) {
	first := newSessionID()
	second := newSessionID()
	if first == "" || second == "" {
		t.Fatal("el id de sesión no puede estar vacío")
	}
	if first == second {
		t.Errorf("dos sesiones compartieron el id %q", first)
	}
	if !strings.HasPrefix(first, "s") {
		t.Errorf("id %q sin el prefijo s", first)
	}
}
