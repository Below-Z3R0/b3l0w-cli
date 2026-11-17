package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// El .git/ del repo de dotfiles no debe terminar en ~/.config/.git/.
func TestGit_ElGitDelRepoNoSeDespliega(t *testing.T) {
	src, home, state := setup(t)

	gitDir := filepath.Join(src, ".git", "objects")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "pack"), []byte("pack"), 0o644); err != nil {
		t.Fatal(err)
	}
	// un dotfile real, para verificar que el pull no se跳过 por completo
	if err := os.MkdirAll(filepath.Join(src, "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("repo"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(src, home, state, false, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(home, ".config", ".git")); !os.IsNotExist(err) {
		t.Fatal("el .git/ del repo se desplego en ~/.config/.git/")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatal("el dotfile real no se instalo:", err)
	}
}
