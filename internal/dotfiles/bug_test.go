package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Tests adversariales del módulo internal/dotfiles.
//
// Cada TestBug_* reproduce un bug real verificado contra el código. Los
// nombres y el orden siguen el orden de aparición en TESTS.md.
//
// REGLA DURA de este archivo: nunca tocar ~/.config real. Todo vive en
// t.TempDir().

// ---------------------------------------------------------------------------
// helpers (prefijo "bug" para no chocar con los de los tests existentes)
// ---------------------------------------------------------------------------

// bugSetup arma repo + home + estado en un tmpdir.
func bugSetup(t *testing.T) (src, home, statePath string) {
	t.Helper()
	root := t.TempDir()
	src = filepath.Join(root, "repo")
	home = filepath.Join(root, "home")
	statePath = filepath.Join(root, "estado.json")
	if err := os.MkdirAll(filepath.Join(src, "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

// bugEscribir escribe un archivo creando los directorios intermedios.
func bugEscribir(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bugEstadoEscribe deja un estado en disco con el contenido dado.
func bugEstadoEscribe(t *testing.T, path, contenido string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bugEstadoBase deja un estado con la base que se le pida.
func bugEstadoBase(t *testing.T, path string, base map[string]string) {
	t.Helper()
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	bugEstadoEscribe(t, path, string(data))
}

// bugNoPanica corre fn y convierte un panic en un fallo de test legible.
func bugNoPanica(t *testing.T, nombre string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s paniqueó: %v", nombre, r)
		}
	}()
	fn()
}

// ---------------------------------------------------------------------------
// A3 — el estado JSON "null" deja el mapa en nil y Pull/Adopt panican
// ---------------------------------------------------------------------------

// TestBug_EstadoNullPanicaEnPull: un estado con el JSON "null" (válido)
// deja el mapa de estado en nil. La primera escritura al mapa revienta
// con "assignment to entry in nil map".
func TestBug_EstadoNullPanicaEnPull(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEstadoEscribe(t, statePath, "null")

	bugNoPanica(t, "Pull", func() {
		if _, err := Pull(src, home, statePath, false, false); err != nil {
			t.Fatalf("Pull: %v", err)
		}
	})
}

// TestBug_EstadoNullPanicaEnAdopt: mismo problema por el lado de Adopt.
func TestBug_EstadoNullPanicaEnAdopt(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")
	bugEstadoEscribe(t, statePath, "null")

	bugNoPanica(t, "Adopt", func() {
		if _, err := Adopt(src, home, statePath); err != nil {
			t.Fatalf("Adopt: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Conflicto falso: local y repo llegan al mismo contenido
// ---------------------------------------------------------------------------

// TestBug_ConflictoFalsoCuandoLocalYRepoCoinciden: la base es vieja, el
// local y el repo llegaron al MISMO contenido. En un merge 3-way eso no
// es conflicto: ambas partes convergen. Acá se reporta como conflicto y
// se escribe un .new con el mismo contenido que ya tiene el archivo.
func TestBug_ConflictoFalsoCuandoLocalYRepoCoinciden(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "mismo")
	bugEscribir(t, local, "mismo")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("base-viejo")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("conflicto falso: local y repo son idénticos. Conflicts = %v", res.Conflicts)
	}
	if _, err := os.Stat(local + ".new"); !os.IsNotExist(err) {
		t.Fatal("escribió un .new redundante con el mismo contenido")
	}
	// La base debe avanzar al contenido actual para no repetir el falso
	// conflicto en el próximo pull.
	leido := leerEstado(t, statePath)
	if leido["noctalia/config.toml"] != hash([]byte("mismo")) {
		t.Fatalf("la base no avanzó: %v", leido)
	}
}

// ---------------------------------------------------------------------------
// A2 — --force --dry-run no reporta lo que se escribiría
// ---------------------------------------------------------------------------

// TestBug_ForceDryRunNoReportaUpdated: con --force --dry-run, los
// archivos que se pisarían por conflicto real no aparecen en Updated
// (sí en ForceOverwrites). El camino no-conflicto sí los reporta. La
// vista previa miente sobre lo que va a pasar.
func TestBug_ForceDryRunNoReportaUpdated(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "repo-nuevo")
	bugEscribir(t, local, "mio")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("base")),
	})

	res, err := Pull(src, home, statePath, true, true)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("dry-run+force no reportó el archivo a actualizar: %+v", res)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("con --force no debería haber conflictos: %+v", res)
	}
}

// TestBug_ForceDryRunReportaIgualQueLaCorridaReal: la vista previa y la
// corrida real tienen que coincidir en lo que reportan.
func TestBug_ForceDryRunReportaIgualQueLaCorridaReal(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "repo-nuevo")
	bugEscribir(t, local, "mio")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("base")),
	})

	preview, err := Pull(src, home, statePath, true, true)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	real, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatalf("real: %v", err)
	}
	if len(preview.Updated) != len(real.Updated) {
		t.Fatalf("la vista previa y la corrida real no coinciden: preview=%v real=%v",
			preview.Updated, real.Updated)
	}
}

// ---------------------------------------------------------------------------
// M3 — un symlink del repo se instala como archivo 0777
// ---------------------------------------------------------------------------

// TestBug_SymlinkEnRepoInstalaConModo0777: un symlink del repo apunta a
// un archivo. Walk lo ve con el modo del symlink (0777), así que el
// archivo instalado en ~/.config queda world-writable y ejecutable.
func TestBug_SymlinkEnRepoInstalaConModo0777(t *testing.T) {
	src, home, statePath := bugSetup(t)
	real := filepath.Join(src, "noctalia", "real.toml")
	bugEscribir(t, real, "contenido")
	link := filepath.Join(src, "noctalia", "enlace.toml")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	dest := filepath.Join(home, ".config", "noctalia", "enlace.toml")
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("no se instaló: %v", err)
	}
	// Ni ejecutable ni escribible por otros: el 0777 del symlink de
	// origen no puede viajar al archivo instalado.
	if fi.Mode().Perm()&0o111 != 0 || fi.Mode().Perm()&0o002 != 0 {
		t.Fatalf("instalado con permisos abiertos: %v", fi.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// Un symlink a un directorio o roto en el repo aborta todo el Pull
// ---------------------------------------------------------------------------

// TestBug_SymlinkADirectorioEnRepoAbortaElPull: un symlink del repo que
// apunta a un directorio abortaba el Pull entero. Un archivo raro en el
// repo no puede dejar la config a medio aplicar: se ignora y el resto
// del pull sigue.
func TestBug_SymlinkADirectorioEnRepoAbortaElPull(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	otro := filepath.Join(src, "noctalia", "otrodir")
	if err := os.MkdirAll(otro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otro, filepath.Join(src, "noctalia", "enlace")); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("un symlink a directorio abortó el pull entero: %v", err)
	}
	// El archivo sano del mismo repo se tiene que haber aplicado.
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se aplicó el archivo sano: %v (res=%+v)", err, res)
	}
}

// TestBug_SymlinkRotoEnRepoAbortaElPull: un symlink del repo que no
// apunta a nada abortaba el Pull entero.
func TestBug_SymlinkRotoEnRepoAbortaElPull(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	roto := filepath.Join(src, "noctalia", "roto.toml")
	if err := os.Symlink(filepath.Join(src, "no-existe"), roto); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("un symlink roto abortó el pull entero: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se aplicó el archivo sano: %v (res=%+v)", err, res)
	}
}

// ---------------------------------------------------------------------------
// Un FIFO en el repo cuelga el Pull para siempre
// ---------------------------------------------------------------------------

// TestBug_FifoEnElRepoCuelgaElPull: un FIFO (o cualquier archivo que no
// es regular) en el repo hace que os.ReadFile bloquee para siempre. El
// Pull queda colgado sin timeout: la CLI se cuelga sin explicación.
func TestBug_FifoEnElRepoCuelgaElPull(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin FIFOs en windows")
	}
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	fifo := filepath.Join(src, "noctalia", "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no se puede crear fifo: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := Pull(src, home, statePath, false, false)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Pull falló con un FIFO en el repo: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pull se colgó leyendo un FIFO del repo (read bloquea para siempre)")
	}
}

// ---------------------------------------------------------------------------
// A4 — saveState no es atómico: un corte deja el módulo inutilizable
// ---------------------------------------------------------------------------

// TestBug_ElEstadoNoSeEscribeDeFormaAtomica: saveState trunca el
// archivo y escribe encima. Si el proceso muere a mitad, el estado
// queda corrupto y Pull falla para siempre. Con escritura atómica
// (tmp + rename) el inode del estado cambia en cada guardado.
func TestBug_ElEstadoNoSeEscribeDeFormaAtomica(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	fi1, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}

	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo-2")
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	fi2, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if os.SameFile(fi1, fi2) {
		t.Fatal("el estado se reescribió in-place: un corte de luz deja el JSON a medias")
	}
}

// TestBug_ElEstadoQuedaParseableDespuesDeCadaPull: el estado que queda
// en disco siempre tiene que ser JSON válido. Es la invariante que
// depende de que el guardado sea atómico.
func TestBug_ElEstadoQuedaParseableDespuesDeCadaPull(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")

	for i := 0; i < 5; i++ {
		bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo-"+string(rune('a'+i)))
		if _, err := Pull(src, home, statePath, false, false); err != nil {
			t.Fatalf("Pull %d: %v", i, err)
		}
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		var st map[string]string
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatalf("el estado quedó corrupto en la corrida %d: %v", i, err)
		}
	}
}

// TestBug_EstadoCorruptoNoDiceComoRecuperarse: un estado truncado hace
// fallar Pull, pero el error no dice qué archivo hay que tocar ni cómo
// recuperarse: el módulo queda inutilizable sin instrucción.
func TestBug_EstadoCorruptoNoDiceComoRecuperarse(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEstadoEscribe(t, statePath, `{"noctalia/config.toml": "abc`)

	_, err := Pull(src, home, statePath, false, false)
	if err == nil {
		t.Fatal("Pull debería fallar con un estado corrupto")
	}
	// El error tiene que señalar el archivo: sin eso el usuario no
	// sabe qué borrar para desbloquear el módulo.
	if !strings.Contains(err.Error(), statePath) {
		t.Fatalf("el error no menciona el archivo del estado: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Un archivo local ilegible se trata como ausente y se pisa
// ---------------------------------------------------------------------------

// TestBug_ArchivoLocalIlegibleSeTrataComoAusente: si el archivo del
// home no se puede leer (permisos), Pull cree que no existe y lo pisa
// sin avisar. El usuario lo protegió y el tool lo destruye.
func TestBug_ArchivoLocalIlegibleSeTrataComoAusente(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "repo-nuevo")
	bugEscribir(t, local, "mio-secreto")
	if err := os.Chmod(local, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(local, 0o644) })

	res, err := Pull(src, home, statePath, false, false)
	if err == nil {
		if contiene(res.Updated, "noctalia/config.toml") || contiene(res.Missing, "noctalia/config.toml") {
			t.Fatalf("pisó un archivo que no pudo leer: %+v", res)
		}
	}
}

// ---------------------------------------------------------------------------
// M2 — el destino es un directorio: error confuso + .tmp huérfano
// ---------------------------------------------------------------------------

// TestBug_DestinoEsDirectorioDejaTmpHuerfano: si en ~/.config hay un
// directorio donde debería ir el archivo, el Pull fallaba con un error
// de rename críptico y dejaba el .tmp tirado en ~/.config. Ahora se
// reporta como conflicto, no se toca el directorio y no queda basura.
func TestBug_DestinoEsDirectorioDejaTmpHuerfano(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(src, home, statePath, false, false)

	// El directorio no se puede reemplazar por un archivo: nunca se pisa.
	if fi, serr := os.Stat(dest); serr != nil || !fi.IsDir() {
		t.Fatalf("el directorio del usuario fue reemplazado: %v", serr)
	}
	// O falla, o lo reporta: lo que no puede pasar es pisarlo en silencio.
	if err == nil && !contiene(res.Conflicts, "noctalia/config.toml") {
		t.Fatalf("el destino directorio no se reportó: %+v", res)
	}

	// No debe quedar basura .tmp en ~/.config.
	var basura []string
	_ = filepath.Walk(home, func(path string, info os.FileInfo, e error) error {
		if e == nil && info != nil && !info.IsDir() && filepath.Ext(path) == ".tmp" {
			basura = append(basura, path)
		}
		return nil
	})
	if len(basura) != 0 {
		t.Fatalf("quedó un .tmp huérfano: %v", basura)
	}
}

// ---------------------------------------------------------------------------
// A1 — Adopt sobrescribe bases existentes y destruye el merge
// ---------------------------------------------------------------------------

// TestBug_AdoptSobrescribeBaseExistente: Adopt registra el hash local
// aunque el archivo ya tenga una base distinta. El README dice que
// Adopt "solo agrega". Correr Adopt dos veces convierte una edición
// local en "no editada" y el pull siguiente la pisa en silencio.
func TestBug_AdoptSobrescribeBaseExistente(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "v1")
	bugEscribir(t, local, "v1")

	if _, err := Adopt(src, home, statePath); err != nil {
		t.Fatal(err)
	}

	// El usuario edita y el repo publica otra versión.
	bugEscribir(t, local, "editado-por-mi")
	bugEscribir(t, repo, "v2")

	if _, err := Adopt(src, home, statePath); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("la edición local debía quedar en conflicto: %+v", res)
	}
	got, _ := os.ReadFile(local)
	if string(got) != "editado-por-mi" {
		t.Fatalf("el trabajo local se perdió: %q", got)
	}
}

// TestBug_AdoptAbortaPorUnArchivoIlegible: un solo archivo que no se
// puede leer cancelaba la adopción completa. Los archivos anteriores ya
// se habían contado pero no se persistía nada: el usuario veía 0 y no
// adoptó ninguno.
func TestBug_AdoptAbortaPorUnArchivoIlegible(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(src, "noctalia", "ilegible.toml"), "repo-ilegible")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")
	ilegible := filepath.Join(home, ".config", "noctalia", "ilegible.toml")
	bugEscribir(t, ilegible, "secreto")
	if err := os.Chmod(ilegible, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(ilegible, 0o644) })

	n, err := Adopt(src, home, statePath)
	if err != nil {
		t.Fatalf("Adopt abortó por un archivo ilegible: %v", err)
	}
	if n != 1 {
		t.Fatalf("adoptó %d, quería 1 (el legible)", n)
	}
	// El archivo legible quedó registrado.
	leido := leerEstado(t, statePath)
	if _, ok := leido["noctalia/config.toml"]; !ok {
		t.Fatalf("no se persistió el archivo legible: %v", leido)
	}
}

// TestBug_AdoptDevuelveCeroSiFallaElGuardado: si el guardado del
// estado falla, Adopt devolvía 0 aunque hubiera adoptado N archivos: el
// conteo real se perdía y el CLI informaba "0 files adopted".
func TestBug_AdoptDevuelveCeroSiFallaElGuardado(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")

	// Estado ya válido en una carpeta que después queda sin permiso de
	// escritura: loadState funciona, saveState falla.
	dir := filepath.Dir(statePath)
	bugEstadoEscribe(t, statePath, "{}")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	n, err := Adopt(src, home, statePath)
	if err == nil {
		t.Fatal("Adopt debería fallar al guardar el estado")
	}
	if n != 1 {
		t.Fatalf("Adopt perdió el conteo real: %d (quería 1)", n)
	}
}
