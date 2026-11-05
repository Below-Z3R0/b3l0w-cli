package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdoptThenPullAppliesRepo(t *testing.T) {
	src, home, state := setup(t)
	local := filepath.Join(home, ".config", "noctalia", "config.toml")
	os.WriteFile(local, []byte("local"), 0o644)
	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("repo"), 0o644)

	n, err := Adopt(src, home, state)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("adoptó %d, quería 1", n)
	}
	if got, _ := os.ReadFile(local); string(got) != "local" {
		t.Fatalf("adopt tocó el archivo: %q", got)
	}

	// Después de adoptar, el repo manda: el archivo no se tocó desde la
	// adopción, así que se actualiza.
	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("después de adopt no debía haber conflictos: %+v", res)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("esperaba actualizar desde el repo: %+v", res)
	}
	if got, _ := os.ReadFile(local); string(got) != "repo" {
		t.Fatalf("no aplicó el repo: %q", got)
	}
}
