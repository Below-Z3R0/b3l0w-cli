package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHallazgo_B11_ElNewVaJuntoAlTargetDelSymlink fija que un conflicto
// sobre un symlink deja el .new al lado del archivo real, no del link:
// el usuario que mira el target no ve el .new si queda en ~/.config.
func TestHallazgo_B11_ElNewVaJuntoAlTargetDelSymlink(t *testing.T) {
	src, home, statePath := bugSetup(t)
	target := t.TempDir()
	real := filepath.Join(target, "config.toml")
	if err := os.WriteFile(real, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	if err := os.Symlink(real, dest); err != nil {
		t.Skipf("no se pudo crear el symlink: %v", err)
	}
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "remoto")
	bugEstadoBase(t, statePath, map[string]string{"noctalia/config.toml": hash([]byte("base"))})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !contiene(res.Conflicts, "noctalia/config.toml") {
		t.Fatalf("no hubo conflicto: %+v", res)
	}

	if _, err := os.Stat(real + ".new"); err != nil {
		t.Errorf("el .new no quedó junto al target: %v", err)
	}
	if _, err := os.Stat(dest + ".new"); err == nil {
		t.Error("el .new quedó junto al symlink, no junto al target")
	}
}
