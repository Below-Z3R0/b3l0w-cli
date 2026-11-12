package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// --force sobre una config que nunca estuvo gestionada no puede perder
// el trabajo local en silencio: deja un .bak al lado.
func TestBug_ForceSobreConfigNoGestionadaDejaBackup(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		// Sin base en el estado: el usuario editó un archivo que el
		// CLI nunca gestionó.
		"noctalia/config.toml": {local: "mi config", repo: "config del repo"},
	})

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("esperaba 1 actualizado: %+v", res)
	}

	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "config del repo" {
		t.Fatalf("no aplico el repo: %q", got)
	}

	backup := dest + ".bak"
	old, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal("no dejo backup del local:", err)
	}
	if string(old) != "mi config" {
		t.Fatalf("el backup no tiene el local: %q", old)
	}
}

// Sin --force no hay backup: el conflicto deja el .new, no el .bak.
func TestBug_SinForceNoHayBackup(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "mi config", repo: "config del repo"},
	})

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(home, ".config", "noctalia", "config.toml") + ".bak"
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("sin --force no deberia haber backup")
	}
}
