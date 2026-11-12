package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDryRunNoEscribeElEstadoNiElNew: --dry-run informa pero no toca
// nada: ni el archivo, ni el .new del conflicto, ni el estado en disco.
func TestDryRunNoEscribeElEstadoNiElNew(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "mio", repo: "nuevo"},
	})
	path := filepath.Join(home, ".config", "noctalia", "config.toml")

	res, err := Pull(src, home, statePath, false, true)
	if err != nil {
		t.Fatal(err)
	}

	// El conflicto se reporta igual: dry-run es una vista previa.
	if !mismoConjunto(res.Conflicts, "noctalia/config.toml") {
		t.Fatalf("Conflicts = %v, quería el archivo", res.Conflicts)
	}
	if !mismoConjunto(res.ForceOverwrites, "noctalia/config.toml") {
		t.Fatalf("ForceOverwrites = %v, quería el archivo", res.ForceOverwrites)
	}
	// Pero no se escribe nada.
	if got, _ := os.ReadFile(path); string(got) != "mio" {
		t.Fatalf("dry-run pisó el local: %q", got)
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Fatal("dry-run escribió el .new")
	}
	if res.Changed {
		t.Fatal("dry-run no debería marcar Changed")
	}
}

// TestDryRunNoModificaElEstadoExistente: si el estado ya estaba, dry-run
// lo deja tal cual (mismos bytes).
func TestDryRunNoModificaElEstadoExistente(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	})
	antes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(src, home, statePath, false, true); err != nil {
		t.Fatal(err)
	}

	despues, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(antes) != string(despues) {
		t.Fatalf("dry-run modificó el estado:\n%s\n---\n%s", antes, despues)
	}
}

// TestDryRunNoCreaElEstadoSiNoExiste: sin estado previo, dry-run no deja
// el archivo de estado en disco.
func TestDryRunNoCreaElEstadoSiNoExiste(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "mio", repo: "nuevo"},
	})
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(src, home, statePath, false, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("dry-run creó el estado")
	}
}

// TestDryRunNoPodaHuerfanosEnElEstado: los huérfanos se reportan pero
// dry-run no los saca del estado.
func TestDryRunNoPodaHuerfanosEnElEstado(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	}, "noctalia/old.toml")

	res, err := Pull(src, home, statePath, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !mismoConjunto(res.Orphaned, "noctalia/old.toml") {
		t.Fatalf("Orphaned = %v, quería el huérfano", res.Orphaned)
	}

	// El estado sigue con el huérfano: dry-run no poda.
	st := leerEstado(t, statePath)
	if _, ok := st["noctalia/old.toml"]; !ok {
		t.Fatalf("dry-run podó el huérfano del estado: %v", st)
	}
}
