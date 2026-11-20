package ytmusic

// Tests de los hallazgos MEDIO y BAJO (Y6–Y20). Cada uno tiene que FALLAR si
// se revierte su arreglo:
//
//   - Y6: `Play` tiene timeout: un yt-dlp colgado no cuelga el comando.
//   - Y7: `Play` mata el reproductor si no puede guardar el estado.
//   - Y8: `EnsureDir` ajusta los permisos de un directorio que ya existía.
//   - Y9: `Thumbs` no escribe fuera del directorio de portadas.
//   - Y10: `sessionPath` no sale de `sessions/`.
//   - Y11: `bestThumb` tiene red de seguridad si todas las portadas son grandes.
//   - Y12: `durationToSeconds` no desborda.
//   - Y13: un `player.json` corrupto es un error, no "parado" para siempre.
//   - Y14: `limit <= 0` significa lo mismo en Thumbs y EnrichDurations.
//   - Y18: el reintento no espera después del último intento.
//   - Y20: el player compila fuera de Unix.
//
// Y15, Y16 y Y17 son del CLI: van en `cli/resto_test.go`.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ── Y6: Play tiene timeout ────────────────────────────────────────────────

// fakeYTDLP arma un `yt-dlp` falso en el PATH que se queda esperando. El
// `Play` lo lanza con `--print` para resolver la metadata: si no hay timeout,
// ese comando no vuelve nunca.
func fakeYTDLP(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("escribiendo el yt-dlp falso: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestPlayCortaElYTDLPColgado fija Y6: un yt-dlp que se queda esperando por
// la red no puede dejar el comando colgado para siempre.
func TestPlayCortaElYTDLPColgado(t *testing.T) {
	// El yt-dlp falso duerme 30 s: sin timeout, Play tardaría 30 s.
	fakeYTDLP(t, "#!/bin/sh\nsleep 30\n")
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := Play(ctx, dir, Song{VideoID: "abc123"}, "")
	tardanza := time.Since(inicio)
	if err == nil {
		t.Fatal("con el yt-dlp colgado, Play tiene que fallar")
	}
	if !strings.Contains(err.Error(), "tiempo") {
		t.Errorf("el error no dice que se agotó el tiempo: %v", err)
	}
	if tardanza > 10*time.Second {
		t.Fatalf("Play tardó %s: el yt-dlp colgado no tiene timeout (Y6)", tardanza)
	}
}

// TestResolveMetadataRespetaElContextoVencido fija Y6 por el lado de la
// metadata: con el contexto ya vencido, no se puede quedar esperando.
func TestResolveMetadataRespetaElContextoVencido(t *testing.T) {
	fakeYTDLP(t, "#!/bin/sh\nsleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inicio := time.Now()
	if _, err := resolveMetadata(ctx, "abc123"); err == nil {
		t.Fatal("con el contexto cancelado, resolveMetadata tiene que fallar")
	}
	if tardanza := time.Since(inicio); tardanza > 5*time.Second {
		t.Fatalf("resolveMetadata tardó %s con el contexto cancelado (Y6)", tardanza)
	}
}

// ── Y7: Play no deja el reproductor huérfano ──────────────────────────────

// TestPlayMataElProcesoSiNoGuardaElEstado fija Y7: si el estado no se puede
// guardar, el reproductor que ya arrancó se mata. Sin esto, la música sigue
// sonando sin control: no hay player.json y `stop` no lo encuentra.
func TestPlayMataElProcesoSiNoGuardaElEstado(t *testing.T) {
	// Los dos binarios falsos se quedan vivos: el proceso del pipeline tiene
	// que morir cuando el guardado falla.
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"yt-dlp", "mpv"} {
		script := "#!/bin/sh\nsleep 60\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	dataDir := t.TempDir()
	// El guardado de player.json no puede completarse: player.json es un
	// directorio NO VACÍO, así que ni el rename ni la limpieza lo pueden
	// quitar.
	if err := os.Mkdir(playerPath(dataDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playerPath(dataDir), "bloqueo"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// La canción trae título: no hace falta resolver metadata con yt-dlp.
	_, err := Play(context.Background(), dataDir, Song{VideoID: "abc123", Title: "Canción"}, "")
	if err == nil {
		t.Fatal("con el estado que no se puede guardar, Play tiene que fallar")
	}
	// El script del pipeline lleva la ruta del socket, que es única de este
	// test: con eso se encuentra al `sh` (y a sus hijos) sin depender de un
	// proceso externo.
	socket := socketPath(dataDir)
	// Primero se espera a que el pipeline forkee: si el guardado falló y el
	// proceso no se mató, aparece con este socket en la cmdline.
	vivo := false
	for i := 0; i < 30; i++ {
		if procesoConRuta(socket) || procesoConRuta(bin) {
			vivo = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !vivo {
		// Puede que el pipeline no llegara a forkear. Se espera un poco más
		// y se vuelve a mirar, para no dar el arreglo por bueno porque el
		// proceso todavía no había nacido.
		time.Sleep(500 * time.Millisecond)
		vivo = procesoConRuta(socket) || procesoConRuta(bin)
	}
	if vivo {
		t.Fatal("quedó un proceso del reproductor vivo: sin player.json nadie lo puede parar (Y7)")
	}
}

// procesoConRuta dice si algún proceso tiene la ruta en su línea de comandos.
// Los zombies tienen la cmdline vacía, así que no cuentan.
func procesoConRuta(ruta string) bool {
	entradas, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entrada := range entradas {
		if !entrada.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entrada.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		if strings.Contains(string(raw), ruta) {
			return true
		}
	}
	return false
}

// ── Y8: EnsureDir ajusta los permisos ─────────────────────────────────────

// TestEnsureDirAjustaLosPermisosDeUnDirectorioExistente fija Y8: el dataDir
// puede haber quedado en 0755 de una corrida anterior y el socket de mpv
// quedaría legible por otros.
func TestEnsureDirAjustaLosPermisosDeUnDirectorioExistente(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	antes, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if antes.Mode().Perm() != 0o755 {
		t.Fatalf("el directorio de prueba quedó en %o, quería 0755", antes.Mode().Perm())
	}
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	despues, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if despues.Mode().Perm() != 0o700 {
		t.Fatalf("los permisos quedaron en %o, quería 0700: EnsureDir no ajusta un directorio que ya existía (Y8)",
			despues.Mode().Perm())
	}
}

// ── Y9: Thumbs no escribe fuera del directorio ─────────────────────────────

// TestThumbsNoEscribeFueraDelDirectorio fija Y9: el videoId viene de la red.
// Sin sanear, un "../../escaped" sacaba la escritura del directorio de
// portadas.
func TestThumbsNoEscribeFueraDelDirectorio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("JPEG-DATA-", 100)))
	}))
	defer server.Close()

	dir := t.TempDir()
	// El id pretende salirse dos niveles.
	songs := []Song{{VideoID: "../../escaped", ThumbURL: server.URL + "/escaped.jpg"}}
	got := Thumbs(context.Background(), dir, songs, 10)
	if got[0].Thumb != "" {
		t.Errorf("apuntó a una portada fuera del directorio: %q", got[0].Thumb)
	}
	// Nada se escribió fuera de dir.
	entradas, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entrada := range entradas {
		if entrada.Name() == filepath.Base(dir) {
			continue
		}
		if strings.Contains(entrada.Name(), "escaped") {
			t.Errorf("Thumbs escribió %q fuera del directorio de portadas (Y9)", entrada.Name())
		}
	}
}

// TestThumbsConVideoIDHostilNoPideLaPortada: el id también se usa en la URL
// de i.ytimg.com: un id con "/" no puede volcar una URL rara.
func TestThumbsConVideoIDHostilNoPideLaPortada(t *testing.T) {
	var pedidas int
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		pedidas++
		mutex.Unlock()
		_, _ = w.Write([]byte(strings.Repeat("JPEG-DATA-", 100)))
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{{VideoID: "../otro/abc"}}
	Thumbs(context.Background(), dir, songs, 10)
	mutex.Lock()
	defer mutex.Unlock()
	if pedidas != 0 {
		t.Errorf("pidió %d portadas con un videoId hostil, quería 0 (Y9)", pedidas)
	}
}

// ── Y10: sessionPath no sale de sessions/ ─────────────────────────────────

// TestSessionPathNoSaleDeSessions fija Y10: el id viene del flag --session o
// de current.json. Con "../" la lectura o la escritura se salían de
// sessions/.
func TestSessionPathNoSaleDeSessions(t *testing.T) {
	dir := t.TempDir()
	casos := []string{"../../escaped", "../escaped", "a/b", "/absoluto"}
	for _, id := range casos {
		path := sessionPath(dir, id)
		base := sessionsDir(dir)
		if path != base && !strings.HasPrefix(path, base+string(os.PathSeparator)) {
			t.Errorf("sessionPath(%q) = %q, salió de %q (Y10)", id, path, base)
		}
	}
}

// TestLoadSessionRechazaUnIDConTraversal fija Y10 por el lado de la carga: un
// id con "../" no puede leer un JSON cualquiera del disco.
func TestLoadSessionRechazaUnIDConTraversal(t *testing.T) {
	dir := t.TempDir()
	// Un archivo malicioso fuera de sessions/.
	secreto := filepath.Join(dir, "..", "secreto.json")
	if err := os.WriteFile(secreto, []byte(`{"id":"x","queue":[{"videoId":"ajeno"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secreto) })
	session, err := LoadSession(dir, "../secreto")
	if err == nil {
		t.Fatalf("un id con traversal tiene que ser error, no la sesión %+v (Y10)", session)
	}
	if len(session.Queue) != 0 {
		t.Errorf("leó una sesión de fuera: %+v", session)
	}
}

// TestSaveSessionRechazaUnIDConTraversal: escribir tampoco puede salirse.
func TestSaveSessionRechazaUnIDConTraversal(t *testing.T) {
	dir := t.TempDir()
	err := SaveSession(dir, Session{ID: "../escaped", Queue: []Song{{VideoID: "a"}}})
	if err == nil {
		t.Fatal("guardar una sesión con un id de traversal tiene que ser error (Y10)")
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escaped.json")); err == nil {
		t.Error("escribió la sesión fuera de sessions/ (Y10)")
	}
}

// ── Y11: bestThumb tiene red de seguridad ─────────────────────────────────

// TestBestThumbUsaUnaPortadaGrandeSiNoHayChica fija Y11: si la página sólo
// trae portadas de más de 400 px, la fila se quedaba sin imagen.
func TestBestThumbUsaUnaPortadaGrandeSiNoHayChica(t *testing.T) {
	renderer := map[string]any{
		"thumbnail": map[string]any{
			"thumbnails": []any{
				map[string]any{"url": "https://example.test/544.jpg", "width": 544},
			},
		},
	}
	got := bestThumb(renderer)
	if got == "" {
		t.Fatal("bestThumb se quedó sin portada: la página sólo traía una de 544 px (Y11)")
	}
	if got != "https://example.test/544.jpg" {
		t.Errorf("bestThumb = %q, quería la de 544 px", got)
	}
}

// TestBestThumbPrefiereLaChicaSiHayUnaChica: la red de seguridad no puede
// replacing la preferencia por la portada chica.
func TestBestThumbPrefiereLaChicaSiHayUnaChica(t *testing.T) {
	renderer := map[string]any{
		"thumbnail": map[string]any{
			"thumbnails": []any{
				map[string]any{"url": "https://example.test/544.jpg", "width": 544},
				map[string]any{"url": "https://example.test/226.jpg", "width": 226},
			},
		},
	}
	if got := bestThumb(renderer); got != "https://example.test/226.jpg" {
		t.Errorf("bestThumb = %q, quería la de 226 px", got)
	}
}

// ── Y12: durationToSeconds no desborda ────────────────────────────────────

// TestDurationToSecondsNoDesborda fija Y12: un lengthText enorme (viene de
// la red) devolvía segundos negativos o absurdos.
func TestDurationToSecondsNoDesborda(t *testing.T) {
	casos := []string{
		"99999999999999999999",
		"9223372036854775808",
		"1:99999999999999999999",
		"9999999999:9999999999",
		"-9223372036854775808",
	}
	for _, entrada := range casos {
		got := durationToSeconds(entrada)
		if got < 0 {
			t.Errorf("durationToSeconds(%q) = %d: un segundo negativo (Y12)", entrada, got)
		}
		if got > maxDurationSeconds {
			t.Errorf("durationToSeconds(%q) = %d: una duración absurda (Y12)", entrada, got)
		}
	}
}

// TestDurationToSecondsSigueParseandoLoNormal: el techo no rompe los casos
// buenos.
func TestDurationToSecondsSigueParseandoLoNormal(t *testing.T) {
	casos := map[string]int{
		"3:45":    225,
		"1:02:03": 3723,
		"0:59":    59,
		"5":       5,
		"":        0,
		"abc":     0,
	}
	for entrada, want := range casos {
		if got := durationToSeconds(entrada); got != want {
			t.Errorf("durationToSeconds(%q) = %d, quería %d", entrada, got, want)
		}
	}
}

// ── Y13: un player.json corrupto ──────────────────────────────────────────

// TestStatusConEstadoCorruptoEsErrorY LimpiaElArchivo fija Y13: un
// player.json roto se traducía en "parado" con exit 0 para siempre, y el
// archivo roto nunca se limpiaba.
func TestStatusConEstadoCorruptoEsErrorYLimpiaElArchivo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(playerPath(dir), []byte(`{"pid": "no es un número`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Status(dir)
	if err == nil {
		t.Fatal("un player.json corrupto tiene que ser error, no \"parado\" (Y13)")
	}
	if !strings.Contains(err.Error(), "roto") {
		t.Errorf("el error no dice que el estado está roto: %v", err)
	}
	if state.Playing {
		t.Error("un estado corrupto no puede estar reproduciendo")
	}
	// El archivo roto se limpia: el status siguiente no lo vuelve a leer.
	if _, err := os.Stat(playerPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no limpió el estado roto: %v", err)
	}
	// Y el status siguiente ya dice "parado" sin error.
	if _, err := Status(dir); err != nil {
		t.Errorf("con el archivo ya limpio, Status no tiene que fallar: %v", err)
	}
}

// TestStatusSinPlayerDevuelveParadoSinError: la ausencia de player.json
// sigue siendo "parado", no un error.
func TestStatusSinPlayerDevuelveParadoSinError(t *testing.T) {
	state, err := Status(t.TempDir())
	if err != nil {
		t.Fatalf("sin player.json, Status no tiene que fallar: %v", err)
	}
	if state.Playing {
		t.Error("sin player.json no puede estar reproduciendo")
	}
}

// ── Y14: limit <= 0 significa lo mismo en todas partes ───────────────────

// TestThumbsConLimiteNoPositivoNoBajaNinguna fija Y14: `limit <= 0` no
// puede significar "todas" en Thumbs y "ninguna" en EnrichDurations.
func TestThumbsConLimiteNoPositivoNoBajaNinguna(t *testing.T) {
	var pedidas int
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		pedidas++
		mutex.Unlock()
		_, _ = w.Write([]byte(strings.Repeat("JPEG-DATA-", 100)))
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{
		{VideoID: "a", ThumbURL: server.URL + "/a.jpg"},
		{VideoID: "b", ThumbURL: server.URL + "/b.jpg"},
	}
	for _, limit := range []int{0, -1} {
		Thumbs(context.Background(), dir, songs, limit)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if pedidas != 0 {
		t.Errorf("Thumbs bajó %d portadas con un límite no positivo, quería 0 (Y14)", pedidas)
	}
}

// TestEnrichDurationsConLimiteNoPositivoNoConsultaNinguno: la otra mitad del
// contrato de Y14.
func TestEnrichDurationsConLimiteNoPositivoNoConsultaNinguno(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar el player")
	})
	songs := []Song{{VideoID: "a"}}
	for _, limit := range []int{0, -1} {
		got := EnrichDurations(context.Background(), fake.client, songs, limit)
		if len(got) != 1 || got[0].Seconds != 0 {
			t.Errorf("cambió la lista con límite %d: %+v", limit, got)
		}
	}
	if len(fake.calls()) != 0 {
		t.Errorf("hizo %d peticiones, quería 0", len(fake.calls()))
	}
}

// ── Y18: el reintento no espera después del último intento ────────────────

// TestElReintentoNoEsperaDespuesDelUltimoIntento fija Y18: la espera de
// 400 ms caía también después del último intento, cuyo resultado ya no se
// usa. Con un 5xx permanente el comando tardaba 800 ms de más.
func TestElReintentoNoEsperaDespuesDelUltimoIntento(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	// Se cuenta la espera en vez de dormir de verdad.
	var esperas int
	var mutex sync.Mutex
	original := retrySleep
	retrySleep = func(d time.Duration) {
		mutex.Lock()
		esperas++
		mutex.Unlock()
	}
	t.Cleanup(func() { retrySleep = original })

	inicio := time.Now()
	_, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/browse", fake.client.Body(nil))
	tardanza := time.Since(inicio)
	if err == nil {
		t.Fatal("un 500 permanente tiene que ser error")
	}
	mutex.Lock()
	defer mutex.Unlock()
	// 2 intentos => 1 espera entre ellos, no 2.
	if esperas != 1 {
		t.Errorf("esperó %d veces, quería 1: la espera después del último intento no sirve (Y18)", esperas)
	}
	if tardanza > 300*time.Millisecond {
		t.Errorf("el comando tardó %s: la espera del último intento se cuelga en el panel (Y18)", tardanza)
	}
}

// ── Y20: el player compila fuera de Unix ──────────────────────────────────

// TestElPlayerCompilaFueraDeUnix fija Y20: `syscall.Kill` y
// `SysProcAttr.Setsid` no existen fuera de Unix. El paquete tiene que
// compilar igual (con el player marcado como no soportado), no reventar con
// "undefined: syscall.Kill".
func TestElPlayerCompilaFueraDeUnix(t *testing.T) {
	if testing.Short() {
		t.Skip("en modo corto no se compila para otra plataforma")
	}
	for _, goos := range []string{"windows", "darwin"} {
		cmd := exec.Command("go", "build", ".")
		cmd.Env = append(os.Environ(),
			"GOOS="+goos,
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		)
		salida, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("el paquete no compila para %s: %v\n%s", goos, err, salida)
		}
	}
}

// TestElEstadoDelPlayerSePuedeLeerYEscribir: el JSON del player sigue siendo
// el mismo después del arreglo de Y20 (el campo startTime de Y1).
func TestElEstadoDelPlayerSePuedeLeerYEscribir(t *testing.T) {
	dir := t.TempDir()
	estado := playerFile{PID: 42, VideoID: "abc", Title: "Canción", StartTime: 99}
	if err := writeJSON(playerPath(dir), estado); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(playerPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var leido map[string]any
	if err := json.Unmarshal(raw, &leido); err != nil {
		t.Fatal(err)
	}
	if leido["videoId"] != "abc" {
		t.Errorf("videoId = %v", leido["videoId"])
	}
	if leido["startTime"] != float64(99) {
		t.Errorf("startTime = %v, quería 99", leido["startTime"])
	}
}

// TestElSocketSeDerivaDelDataDir: el contrato de Y2 sigue intacto después de
// los cambios de Y20.
func TestElSocketSeDerivaDelDataDir(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "mpv.sock")
	if got := socketPath(dir); got != want {
		t.Errorf("socketPath = %q, quería %q", got, want)
	}
}

// TestElGrupoDelPlayerSeMataEntero: el mecanismo de grupo (Y20 lo movió de
// lugar) sigue matando el grupo entero.
func TestElGrupoDelPlayerSeMataEntero(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sleep", "120")
	setSessionAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("arrancando el proceso de prueba: %v", err)
	}
	pid := cmd.Process.Pid
	terminado := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(terminado)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		select {
		case <-terminado:
		case <-time.After(2 * time.Second):
		}
	})
	if !alive(pid) {
		t.Fatal("el proceso de prueba no arrancó")
	}
	started, ok := processStartTime(pid)
	if !ok {
		t.Fatal("no se pudo leer /proc/<pid>/stat")
	}
	if err := writeJSON(playerPath(dir), playerFile{
		PID:       pid,
		VideoID:   "abc",
		Socket:    socketPath(dir),
		StartTime: started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Stop(dir); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	limite := time.Now().Add(3 * time.Second)
	for time.Now().Before(limite) && alive(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if alive(pid) {
		t.Error("el grupo sigue vivo después de Stop")
	}
}
