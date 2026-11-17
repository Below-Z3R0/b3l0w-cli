package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests del estado y de las formas raras del filesystem.
//
// El estado es la única memoria del merge 3-way. Si se pierde o se poda
// de más, el pull siguiente deja de distinguir "lo editó el usuario" de
// "lo cambió el repo" y todo cae en la rama 4 (conflicto) o, con
// --force, se pisa en masa.

// ---------------------------------------------------------------------------
// El guard de "repo vacío" (C1) es demasiado angosto
// ---------------------------------------------------------------------------

// TestEdge_ElRepoTotalmenteVacioNoBorraElEstado: un repo vacío no puede
// borrar el estado. Si lo hiciera, el pull siguiente trataría cada
// config como "nunca gestionada" y, con --force, las pisaría todas.
//
// Este es el guard de C1 (`len(seen) == 0`): funciona, pero solo cuando
// el repo está vacío de verdad. Ver TestBug_ElRepoSoloConArchivosDescartados.
func TestEdge_ElRepoTotalmenteVacioNoBorraElEstado(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("base")),
		"fish/config.fish":     hash([]byte("base")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("un repo vacío reportó huérfanos: %v", res.Orphaned)
	}
	leido := leerEstado(t, statePath)
	if len(leido) != 2 {
		t.Fatalf("un repo vacío borró el estado: %v", leido)
	}
}

// TestBug_ElRepoSoloConArchivosDescartadosBorraElEstado: el guard de C1
// mira `seen`, que cuenta TAMBIÉN los archivos que `skip()` descarta. Un
// repo con un solo README.md (un clone a medias, o el --dotfiles-dir
// apuntando a otra cosa) deja `seen` con una entrada y por lo tanto PASA
// el guard: el estado entero se poda y todas las bases se pierden.
//
// Es el mismo daño que C1 pero por la puerta de atrás: un repo que no
// tiene los dotfiles (un clone incompleto, un repo equivocado) borra el
// estado. El pull siguiente ya no sabe qué base tenía cada config.
func TestBug_ElRepoSoloConArchivosDescartadosBorraElEstado(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un repo con SOLO un README: nada que sea un dotfile.
	bugEscribir(t, filepath.Join(src, "README.md"), "# dotfiles\n")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("a")),
		"fish/config.fish":     hash([]byte("b")),
		"umbriel/config.toml":  hash([]byte("c")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("un repo sin dotfiles reportó %d huérfanos y podó el estado: %v",
			len(res.Orphaned), res.Orphaned)
	}
	leido := leerEstado(t, statePath)
	if len(leido) != 3 {
		t.Fatalf("el estado se vació: %v", leido)
	}
}

// TestBug_ElRepoSoloConGitBorraElEstado: el mismo caso con un repo que
// solo tiene el ".git/" (un clon sin working tree). El ".git" lo descarta
// `skip()`, pero igual cuenta como visto.
func TestBug_ElRepoSoloConGitBorraElEstado(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, ".git", "HEAD"), "ref: refs/heads/main")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("a")),
		"fish/config.fish":     hash([]byte("b")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("un repo con solo .git/ podó el estado: %v", res.Orphaned)
	}
}

// TestBug_ElEstadoPerdidoProvocaConflictosFalsos: la consecuencia
// concreta de perder el estado. Un pull con el repo completo, después de
// que otro pull haya vaciado el estado, reporta TODOS los archivos como
// conflicto: el 3-way ya no sabe qué base tenía cada uno.
//
// Con --force, en vez de conflicto, es pérdida de trabajo local en masa.
func TestBug_ElEstadoPerdidoProvocaConflictosFalsos(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// El repo "equivocado": solo un README.
	bugEscribir(t, filepath.Join(src, "README.md"), "# dotfiles\n")
	// El estado con las bases reales.
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("contenido-local")),
	})
	// El home con el archivo ya aplicado.
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "contenido-local")

	// Pull 1: el repo equivocado vacía el estado.
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}

	// Ahora el repo correcto aparece.
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "contenido-nuevo-del-repo")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("conflictos falsos: el archivo estaba gestionado y no cambió: %v", res.Conflicts)
	}
	if !mismoConjunto(res.Updated, "noctalia/config.toml") {
		t.Fatalf("Updated = %v, quería el archivo actualizado", res.Updated)
	}
}

// ---------------------------------------------------------------------------
// El estado como archivo raro
// ---------------------------------------------------------------------------

// TestBug_ElEstadoQueEsUnDirectorioDaUnErrorSinSalida: si el archivo del
// estado es un directorio, loadState devuelve el error crudo de
// os.ReadFile ("is a directory"). El usuario no tiene forma de saber qué
// hacer. El error de estado corrupto (JSON inválido) sí dice "borralo
// para regenerarlo"; este no dice nada.
func TestBug_ElEstadoQueEsUnDirectorioDaUnErrorSinSalida(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Pull(src, home, statePath, false, false)
	if err == nil {
		t.Fatal("Pull debería fallar con el estado como directorio")
	}
	// El error tiene que señalar el archivo y cómo recuperar.
	if !strings.Contains(err.Error(), statePath) {
		t.Fatalf("el error no menciona el archivo del estado: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "borr") {
		t.Fatalf("el error no dice cómo recuperar el estado: %v", err)
	}
}

// TestEdge_ElEstadoQueEsUnDirectorioNoEscribeNada: el fallo tiene que ser
// limpio: no se toca el home ni se crea el estado.
func TestEdge_ElEstadoQueEsUnDirectorioNoEscribeNada(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(src, home, statePath, false, false); err == nil {
		t.Fatal("Pull debería fallar")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("el pull fallido igual escribió en el home")
	}
}

// TestEdge_AdoptConElEstadoQueEsUnDirectorioFallaSinTocarNada: Adopt
// tiene el mismo comportamiento: error, cero adopciones, home intacto.
func TestEdge_AdoptConElEstadoQueEsUnDirectorioFallaSinTocarNada(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	n, err := Adopt(src, home, statePath)
	if err == nil {
		t.Fatal("Adopt debería fallar con el estado como directorio")
	}
	if n != 0 {
		t.Fatalf("Adopt contó %d adopciones en un pull fallido", n)
	}
}

// ---------------------------------------------------------------------------
// El srcDir raro
// ---------------------------------------------------------------------------

// TestEdge_ElSrcDirQueNoExisteFallaEnVozAlta: un --dotfiles-dir que no
// existe tiene que fallar con un error claro, no podar el estado ni
// tocar el home.
func TestEdge_ElSrcDirQueNoExisteFallaEnVozAlta(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "no-existe")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("a")),
	})

	_, err := Pull(src, home, statePath, false, false)
	if err == nil {
		t.Fatal("Pull debería fallar si el repo no existe")
	}
	if !strings.Contains(err.Error(), "no-existe") {
		t.Fatalf("el error no menciona el repo: %v", err)
	}
	// El estado no se toca: un repo mal pasado no puede borrar el estado.
	leido := leerEstado(t, statePath)
	if len(leido) != 1 {
		t.Fatalf("el estado se modificó con un repo inexistente: %v", leido)
	}
}

// TestEdge_ConRutasRelativasFunciona: srcDir, homeDir y statePath
// relativos (con ".." y sin él) funcionan igual. El 3-way no depende de
// que las rutas sean absolutas.
func TestEdge_ConRutasRelativasFunciona(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	bugEscribir(t, filepath.Join(root, "repo", "noctalia", "config.toml"), "repo")

	res, err := Pull("repo", "home", "estado.json", false, false)
	if err != nil {
		t.Fatalf("Pull con rutas relativas: %v", err)
	}
	if !mismoConjunto(res.Missing, "noctalia/config.toml") {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if _, err := os.Stat(filepath.Join(root, "home", ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se instaló con rutas relativas: %v", err)
	}
	// La clave del estado es la ruta RELATIVA al repo, no un camino
	// absoluto ni con "..".
	data, err := os.ReadFile(filepath.Join(root, "estado.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]string
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if _, ok := st["noctalia/config.toml"]; !ok {
		t.Fatalf("la clave del estado no es relativa: %v", st)
	}
}

// TestEdge_ElHomeQueEsUnSymlinkSeAplicaEnElTarget: si el home es un
// symlink (un home en otro disco, un home montado), el pull escribe en
// el home real y no reemplaza el symlink.
func TestEdge_ElHomeQueEsUnSymlinkSeAplicaEnElTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	realHome := filepath.Join(root, "home-real")
	home := filepath.Join(root, "home-link")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	if err := os.MkdirAll(realHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHome, home); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Missing, "noctalia/config.toml") {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se escribió en el home real: %v", err)
	}
	// El symlink sigue siendo symlink.
	fi, err := os.Lstat(home)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("el symlink del home fue reemplazado por un directorio")
	}
}

// TestEdge_ElConfigQueEsUnArchivoFallaSinTocarNada: si ~/.config es un
// archivo (no un directorio), el pull tiene que fallar sin escribir
// nada. No puede crear el archivo ni pisar el que está.
func TestEdge_ElConfigQueEsUnArchivoFallaSinTocarNada(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	archivoConfig := filepath.Join(home, ".config")
	if err := os.WriteFile(archivoConfig, []byte("soy un archivo"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Pull(src, home, statePath, false, false)
	if err == nil {
		t.Fatal("Pull debería fallar si ~/.config es un archivo")
	}
	got, rerr := os.ReadFile(archivoConfig)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != "soy un archivo" {
		t.Fatalf("el archivo ~/.config fue pisado: %q", got)
	}
	// Y el estado no se crea: no se gestionó nada.
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("el pull fallido dejó el estado escrito")
	}
}

// TestEdge_UnaFallaDeEscrituraDejaElEstadoConsistente: si la escritura
// de un archivo falla, los archivos ya escritos quedan con su base
// Advanceada y el que falló no queda con base. Es el contrato de
// "persistir por archivo".
func TestEdge_UnaFallaDeEscrituraDejaElEstadoConsistente(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"a-ok.toml":       {repo: "ok", sinLocal: true},
		"ro/b-falla.toml": {repo: "falla", sinLocal: true},
	})
	// ~/.config/ro no tiene permiso de escritura: el temporal de
	// "ro/b-falla.toml" no se puede crear y la escritura falla.
	roDir := filepath.Join(home, ".config", "ro")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	_, err := Pull(src, home, statePath, false, false)
	if err == nil {
		t.Fatal("Pull debería fallar al escribir")
	}
	// El archivo que se aplicó antes del fallo queda instalado y con base.
	if _, err := os.Stat(filepath.Join(home, ".config", "a-ok.toml")); err != nil {
		t.Fatalf("el archivo sano no se aplicó: %v", err)
	}
	leido := leerEstado(t, statePath)
	if _, ok := leido["a-ok.toml"]; !ok {
		t.Fatalf("el archivo aplicado quedó sin base: %v", leido)
	}
	// El que falló no quedó con base: no se gestionó.
	if _, ok := leido["ro/b-falla.toml"]; ok {
		t.Fatalf("el archivo que falló quedó con base: %v", leido)
	}
}
