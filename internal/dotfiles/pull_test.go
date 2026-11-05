package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) (src, home, state string) {
	t.Helper()
	root := t.TempDir()
	src = filepath.Join(root, "repo")
	home = filepath.Join(root, "home")
	state = filepath.Join(root, "state.json")
	if err := os.MkdirAll(filepath.Join(src, "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPullUpdatesWhenLocalUntouched(t *testing.T) {
	src, home, state := setup(t)
	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("nuevo"), 0o644)
	os.WriteFile(filepath.Join(home, ".config", "noctalia", "config.toml"), []byte("viejo"), 0o644)

	// Primera corrida sin estado: conflicto, no pisa.
	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("esperaba conflicto, obtuve %+v", res)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".config", "noctalia", "config.toml")); string(got) != "viejo" {
		t.Fatalf("no debía pisar: %q", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml.new")); err != nil {
		t.Fatal("esperaba el .new")
	}
}

func TestPullKeepsLocalChange(t *testing.T) {
	src, home, state := setup(t)
	path := filepath.Join(home, ".config", "noctalia", "config.toml")
	os.WriteFile(path, []byte("base"), 0o644)
	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("base"), 0o644)

	// Adopta el estado.
	if _, err := Pull(src, home, state, false, false); err != nil {
		t.Fatal(err)
	}
	// El usuario edita y el repo no cambia.
	os.WriteFile(path, []byte("local"), 0o644)
	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kept) != 1 {
		t.Fatalf("esperaba conservar, obtuve %+v", res)
	}
	if got, _ := os.ReadFile(path); string(got) != "local" {
		t.Fatalf("pisó el cambio local: %q", got)
	}
}

func TestPullUpdatesWhenRepoChangesAndLocalUntouched(t *testing.T) {
	src, home, state := setup(t)
	path := filepath.Join(home, ".config", "noctalia", "config.toml")
	os.WriteFile(path, []byte("base"), 0o644)
	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("base"), 0o644)
	if _, err := Pull(src, home, state, false, false); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("nuevo"), 0o644)
	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("esperaba update, obtuve %+v", res)
	}
	if got, _ := os.ReadFile(path); string(got) != "nuevo" {
		t.Fatalf("no actualizó: %q", got)
	}
}

func TestPullDryRunDoesNotWrite(t *testing.T) {
	src, home, state := setup(t)
	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("nuevo"), 0o644)

	_, err := Pull(src, home, state, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("dry-run escribió")
	}
}
