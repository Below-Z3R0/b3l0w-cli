package channel

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCurrentIgnoraUnCanalDesconocido es el bug A1: el archivo se
// devolvía sin validar, así que "nightly" pasaba tal cual y update lo
// trataba como edge.
func TestCurrentIgnoraUnCanalDesconocido(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte("nightly\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Current()
	if got == "nightly" {
		t.Fatalf("Current devolvió %q: un canal desconocido no se acepta", got)
	}
	if got != Stable && got != Edge {
		t.Errorf("Current devolvió %q, quería %q o %q", got, Stable, Edge)
	}
}

// TestCurrentLeeLosConocidos: los dos valores válidos sí se devuelven.
func TestCurrentLeeLosConocidos(t *testing.T) {
	for _, canal := range []string{Stable, Edge} {
		t.Run(canal, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(Path(), []byte(canal+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := Current(); got != canal {
				t.Errorf("Current = %q, quería %q", got, canal)
			}
		})
	}
}
