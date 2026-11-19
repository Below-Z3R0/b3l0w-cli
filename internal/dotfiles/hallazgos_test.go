package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests canónicos de los hallazgos CRÍTICO y ALTO de internal/dotfiles.
//
// Cada TestHallazgo_* cubre UN hallazgo y falla si se revierte el
// arreglo: no basta con que el código "parezca" correcto, el test
// comprueba la consecuencia exacta que el bug producía.
//
// | Hallazgo | Qué rompe si se revierte |
// |---|---|
// | C1 | un repo vacío (o sin dotfiles) borra el estado y el pull siguiente pisa el trabajo local |
// | C2 | el `.git/` (u otro VCS) del repo se despliega en ~/.config/ |
// | A1 | `Adopt` pisa la base y el pull siguiente pierde la edición local en silencio |
// | A2 | `--force --dry-run` no dice qué se escribiría |
// | A3 | un estado con JSON `null` tumba el CLI con un panic |
// | A4 | el estado se escribe in-place: un corte lo deja corrupto |
// | A5 | un archivo que no se puede aplicar aborta el pull entero |
// | A6 | `--force` pisa una config nunca gestionada sin dejar backup |
//
// REGLA DURA: nada de esto toca el home real. Todo vive en t.TempDir().

// ---------------------------------------------------------------------------
// C1 — un repo vacío borra el estado entero
// ---------------------------------------------------------------------------

// TestHallazgo_C1_ElRepoEmptyNoBorraElEstado: un srcDir vacío (clone
// incompleto, --dotfiles-dir apuntando al lado equivocado) no puede
// podar el estado.
//
// El daño real no es el estado vacío: es su consecuencia. Sin base, el
// pull siguiente trata cada config como "nunca gestionada" y una
// actualización normal se vuelve un conflicto (o, con --force, la
// pérdida del trabajo local).
func TestHallazgo_C1_ElRepoEmptyNoBorraElEstado(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "base")
	bugEscribir(t, local, "base")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	base := leerEstado(t, statePath)["noctalia/config.toml"]
	if base == "" {
		t.Fatal("la base no se registró")
	}

	// El repo se vacía: un clon a medias.
	if err := os.Remove(repo); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull con el repo vacío: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("un repo vacío reportó huérfanos: %v", res.Orphaned)
	}
	leido := leerEstado(t, statePath)
	if leido["noctalia/config.toml"] != base {
		t.Fatalf("un repo vacío borró la base: %q (quería %q)", leido["noctalia/config.toml"], base)
	}

	// El repo vuelve con otra versión y el usuario NO tocó el local:
	// tiene que ser una actualización normal, no un conflicto. Si el
	// estado se hubiera podado, esto sería un conflicto.
	bugEscribir(t, repo, "repo-v2")
	res, err = Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("el estado perdido convirtió una actualización en conflicto: %v", res.Conflicts)
	}
	if !mismoConjunto(res.Updated, "noctalia/config.toml") {
		t.Fatalf("Updated = %v, quería la actualización normal", res.Updated)
	}
}

// ---------------------------------------------------------------------------
// C2 — el VCS del repo se despliega en ~/.config/
// ---------------------------------------------------------------------------

// TestHallazgo_C2_ElVCSDelRepoNoSeDespliega: un `.git/` (u otro VCS)
// dentro del repo no es configuración del usuario: no puede terminar en
// ~/.config/.git/.
//
// Se prueba también el VCS ANIDADO (un submódulo, un repo clonado
// dentro del repo de dotfiles): filtrar por prefijo de path no alcanza,
// hay que comparar por segmento.
func TestHallazgo_C2_ElVCSDelRepoNoSeDespliega(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")

	// El .git de la raíz.
	bugEscribir(t, filepath.Join(src, ".git", "HEAD"), "ref: refs/heads/main")
	bugEscribir(t, filepath.Join(src, ".git", "objects", "ab", "cd"), "binario")
	// Un VCS anidado: otro repo clonado dentro del repo de dotfiles.
	bugEscribir(t, filepath.Join(src, "subrepo", ".git", "config"), "[core]\n")
	bugEscribir(t, filepath.Join(src, "subrepo", ".hg", "dreq"), "hg")
	bugEscribir(t, filepath.Join(src, "subrepo", ".svn", "entries"), "svn")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	for _, rel := range []string{".git", "subrepo/.git", "subrepo/.hg", "subrepo/.svn"} {
		ruta := filepath.Join(home, ".config", filepath.FromSlash(rel))
		if _, err := os.Stat(ruta); !os.IsNotExist(err) {
			t.Fatalf("el %s del repo se desplegó en ~/.config", rel)
		}
	}
	// El dotfile real sí se instala: el filtro no puede comerse todo.
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("el dotfile real no se instaló: %v", err)
	}
	// Y el estado no gestiona ninguna ruta de VCS.
	for k := range leerEstado(t, statePath) {
		if strings.Contains(k, ".git") || strings.Contains(k, ".hg") || strings.Contains(k, ".svn") {
			t.Fatalf("el estado tiene una entrada de VCS: %q", k)
		}
	}
}

// ---------------------------------------------------------------------------
// A1 — Adopt pisa la base y borra la protección del merge
// ---------------------------------------------------------------------------

// TestHallazgo_A1_AdoptNoPisaUnaBaseExistente: Adopt "solo agrega". Si
// pisa una base que ya existe, una edición local pasa a ser "no
// editada" y el pull siguiente la pisa en silencio: es la única forma
// de perder trabajo local sin --force.
func TestHallazgo_A1_AdoptNoPisaUnaBaseExistente(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "v1")
	bugEscribir(t, local, "v1")

	if _, err := Adopt(src, home, statePath); err != nil {
		t.Fatal(err)
	}
	base := leerEstado(t, statePath)["noctalia/config.toml"]
	if base != hash([]byte("v1")) {
		t.Fatalf("la base no es el hash del local: %q", base)
	}

	// El usuario edita el local y el repo publica otra versión.
	bugEscribir(t, local, "editado-por-mi")
	bugEscribir(t, repo, "v2")

	if _, err := Adopt(src, home, statePath); err != nil {
		t.Fatal(err)
	}
	despues := leerEstado(t, statePath)["noctalia/config.toml"]
	if despues != base {
		t.Fatalf("Adopt pisó la base: %q → %q", base, despues)
	}

	// Consecuencia: el pull siguiente tiene que ver el conflicto real.
	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Conflicts, "noctalia/config.toml") {
		t.Fatalf("la edición local no quedó en conflicto: %+v", res)
	}
	got, _ := os.ReadFile(local)
	if string(got) != "editado-por-mi" {
		t.Fatalf("el trabajo local se perdió: %q", got)
	}
}

// ---------------------------------------------------------------------------
// A2 — --force --dry-run no dice qué se escribiría
// ---------------------------------------------------------------------------

// TestHallazgo_A2_ForceDryRunReportaLoQueSeEscribiria: el Result
// describe lo que PASARÍA; solo las escrituras se suprimen en dry-run.
// Con --force, el preview tiene que listar en Updated lo que se
// escribiría.
func TestHallazgo_A2_ForceDryRunReportaLoQueSeEscribiria(t *testing.T) {
	// Rama 7: hay base, el local cambió y el repo también. Conflicto real.
	t.Run("conflicto real", func(t *testing.T) {
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
		if !mismoConjunto(preview.Updated, "noctalia/config.toml") {
			t.Fatalf("el preview con --force no dijo qué se escribiría: %+v", preview)
		}
		if len(preview.Conflicts) != 0 {
			t.Fatalf("con --force no hay conflictos: %+v", preview)
		}
		// El dry-run no escribió nada.
		if got, _ := os.ReadFile(local); string(got) != "mio" {
			t.Fatalf("el dry-run pisó el local: %q", got)
		}

		// Y el preview coincide con la corrida real.
		real, err := Pull(src, home, statePath, true, false)
		if err != nil {
			t.Fatalf("corrida real: %v", err)
		}
		if !mismoConjunto(real.Updated, preview.Updated...) {
			t.Fatalf("preview y corrida real difieren: %v vs %v", preview.Updated, real.Updated)
		}
	})

	// Rama 4: no hay base, el local existe y difiere del repo.
	t.Run("sin base", func(t *testing.T) {
		src, home, statePath := bugSetup(t)
		repo := filepath.Join(src, "noctalia", "config.toml")
		local := filepath.Join(home, ".config", "noctalia", "config.toml")
		bugEscribir(t, repo, "repo-nuevo")
		bugEscribir(t, local, "mio")

		preview, err := Pull(src, home, statePath, true, true)
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		if !mismoConjunto(preview.Updated, "noctalia/config.toml") {
			t.Fatalf("el preview con --force no dijo qué se escribiría: %+v", preview)
		}
		if got, _ := os.ReadFile(local); string(got) != "mio" {
			t.Fatalf("el dry-run pisó el local: %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// A3 — un estado con JSON "null" tumba el CLI
// ---------------------------------------------------------------------------

// TestHallazgo_A3_ElEstadoNullNoPanica: "null" es JSON válido y deja el
// mapa de estado en nil. La primera asignación revienta con "assignment
// to entry in nil map".
func TestHallazgo_A3_ElEstadoNullNoPanica(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")
	bugEstadoEscribe(t, statePath, "null\n")

	bugNoPanica(t, "Pull", func() {
		if _, err := Pull(src, home, statePath, false, false); err != nil {
			t.Fatalf("Pull: %v", err)
		}
	})
	// El estado que queda escrito es un objeto, no null.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]string
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("estado inválido: %v", err)
	}
	if st == nil {
		t.Fatal("el estado quedó en null")
	}

	// Adopt tampoco puede panicar.
	bugEstadoEscribe(t, statePath, "null\n")
	bugNoPanica(t, "Adopt", func() {
		if _, err := Adopt(src, home, statePath); err != nil {
			t.Fatalf("Adopt: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// A4 — el estado se escribe in-place
// ---------------------------------------------------------------------------

// TestHallazgo_A4_ElEstadoSeGuardaDeFormaAtomica: saveState tiene que
// escribir a un temporal y renombrar. Si se trunca el archivo, un corte
// de luz deja el JSON a medias y el módulo queda inutilizable para
// siempre (no hay recuperación).
func TestHallazgo_A4_ElEstadoSeGuardaDeFormaAtomica(t *testing.T) {
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
		t.Fatalf("Pull 2: %v", err)
	}
	fi2, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(fi1, fi2) {
		t.Fatal("el estado se reescribió in-place: un corte deja el JSON truncado")
	}

	// Invariante: lo que queda siempre es JSON parseable.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]string
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("el estado quedó corrupto: %v (%s)", err, data)
	}
}

// ---------------------------------------------------------------------------
// A5 — un archivo que falla aborta el pull entero
// ---------------------------------------------------------------------------

// TestHallazgo_A5_UnArchivoQueFallaNoAbregaElPull: un archivo que no se
// puede escribir (permisos, disco lleno, el destino es un directorio) no
// puede dejar el resto de la config sin aplicar. El pull sigue con los
// demás archivos y reporta el fallo.
func TestHallazgo_A5_UnArchivoQueFallaNoAbregaElPull(t *testing.T) {
	// Caso 1: la escritura falla porque el destino está ocupado por un
	// archivo. Determinista y sin depender de los permisos: escribir
	// ~/.config/b-falla/x.toml con ~/.config/b-falla siendo un archivo
	// falla con ENOTDIR.
	t.Run("destino ocupado", func(t *testing.T) {
		src, home, statePath := bugSetup(t)
		bugEscribir(t, filepath.Join(src, "a-ok.toml"), "ok")
		bugEscribir(t, filepath.Join(src, "b-falla", "x.toml"), "falla")
		bugEscribir(t, filepath.Join(src, "z-ok.toml"), "ok-z")
		bugEscribir(t, filepath.Join(home, ".config", "b-falla"), "soy un archivo")

		res, err := Pull(src, home, statePath, false, false)
		if err == nil {
			t.Fatal("Pull debería reportar el archivo que no pudo aplicar")
		}

		// El archivo sano que viene DESPUÉS del que falló se aplicó.
		got, rerr := os.ReadFile(filepath.Join(home, ".config", "z-ok.toml"))
		if rerr != nil {
			t.Fatalf("un archivo que falló abortó el pull entero: %v", rerr)
		}
		if string(got) != "ok-z" {
			t.Fatalf("contenido = %q", got)
		}
		// El que falló no se reporta como instalado ni como actualizado.
		if contiene(res.Missing, "b-falla/x.toml") {
			t.Fatalf("reportó como instalado un archivo que no se instaló: %v", res.Missing)
		}
		if contiene(res.Updated, "b-falla/x.toml") {
			t.Fatalf("reportó como actualizado un archivo que no se actualizó: %v", res.Updated)
		}
		// Y el estado no lo gestiona.
		if _, ok := leerEstado(t, statePath)["b-falla/x.toml"]; ok {
			t.Fatal("el estado registró un archivo que no se aplicó")
		}
	})

	// Caso 2: la escritura falla por permisos. El archivo se puede leer
	// (el directorio es legible) pero no escribir.
	t.Run("sin permiso de escritura", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignora los permisos: el caso 1 ya cubre el fallo")
		}
		src, home, statePath := bugSetup(t)
		bugEscribir(t, filepath.Join(src, "a-ok.toml"), "ok")
		bugEscribir(t, filepath.Join(src, "ro", "b-falla.toml"), "falla")
		bugEscribir(t, filepath.Join(src, "z-ok.toml"), "ok-z")
		roDir := filepath.Join(home, ".config", "ro")
		if err := os.MkdirAll(roDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(roDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(roDir, 0o755) })

		res, err := Pull(src, home, statePath, false, false)
		if err == nil {
			t.Fatal("Pull debería reportar el archivo que no pudo aplicar")
		}

		got, rerr := os.ReadFile(filepath.Join(home, ".config", "z-ok.toml"))
		if rerr != nil {
			t.Fatalf("un archivo que falló abortó el pull entero: %v", rerr)
		}
		if string(got) != "ok-z" {
			t.Fatalf("contenido = %q", got)
		}
		if contiene(res.Missing, "ro/b-falla.toml") {
			t.Fatalf("reportó como instalado un archivo que no se instaló: %v", res.Missing)
		}
		if _, ok := leerEstado(t, statePath)["ro/b-falla.toml"]; ok {
			t.Fatal("el estado registró un archivo que no se aplicó")
		}
	})
}

// ---------------------------------------------------------------------------
// A6 — --force pisa una config nunca gestionada sin backup
// ---------------------------------------------------------------------------

// TestHallazgo_A6_ForceSobreUnaConfigNoGestionadaDejaBackup: la rama 4
// ("no hay base, el local existe y difiere") no es un conflicto real:
// puede ser config preexisting del usuario. Con --force, el contenido
// anterior tiene que quedar en un .bak: sin él, la pérdida es
// irreversible.
func TestHallazgo_A6_ForceSobreUnaConfigNoGestionadaDejaBackup(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "config del repo")
	bugEscribir(t, local, "mi config")
	// Sin entrada en el estado: esta config nunca fue gestionada.

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Updated, "noctalia/config.toml") {
		t.Fatalf("Updated = %v", res.Updated)
	}
	got, _ := os.ReadFile(local)
	if string(got) != "config del repo" {
		t.Fatalf("no aplicó el repo: %q", got)
	}

	// El trabajo local tiene que estar en el backup.
	backup, berr := os.ReadFile(local + ".bak")
	if berr != nil {
		t.Fatalf("--force no dejó backup del local: %v", berr)
	}
	if string(backup) != "mi config" {
		t.Fatalf("el backup no tiene el contenido local: %q", backup)
	}
}
