package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// fakeSnapper pone un `snapper` falso primero en el PATH que registra
// cada invocación en un log. El snapper real no se ejecuta nunca.
func fakeSnapper(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	log := filepath.Join(dir, "snapper.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$SNAPPER_LOG"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "snapper"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SNAPPER_LOG", log)
	return log
}

// TestDryRunNoCreaSnapshot es el bug C1: `--dotfiles --dry-run` creaba
// una snapshot real porque updateDotfiles llamaba a snapshot.Create sin
// mirar el flag.
func TestDryRunNoCreaSnapshot(t *testing.T) {
	log := fakeSnapper(t)
	t.Setenv("HOME", t.TempDir())
	dotfilesDir := t.TempDir()

	Run(&config.Manifest{}, Options{
		Dotfiles:    true,
		DryRun:      true,
		DotfilesDir: dotfilesDir,
	})

	if data, err := os.ReadFile(log); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		t.Errorf("el dry-run llamó a snapper:\n%s", data)
	}
}

// TestSinDryRunSiCreaSnapshot es el contraste: sin el flag, la snapshot
// se pide. Si este test falla, el anterior pasaría por la razón
// equivocada (por ejemplo, porque snapshot.Create ya no se llama nunca).
func TestSinDryRunSiCreaSnapshot(t *testing.T) {
	log := fakeSnapper(t)
	t.Setenv("HOME", t.TempDir())
	dotfilesDir := t.TempDir()

	Run(&config.Manifest{}, Options{
		Dotfiles:    true,
		DotfilesDir: dotfilesDir,
	})

	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("sin dry-run no se llamó a snapper: %v", err)
	}
	if !strings.Contains(string(data), "create") {
		t.Errorf("se llamó a snapper pero no para crear: %s", data)
	}
}

// TestDryRunNoAdopta: --adopt escribe el estado de cada archivo, así
// que en dry-run no puede ejecutarse.
func TestDryRunNoAdopta(t *testing.T) {
	fakeSnapper(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dotfilesDir := t.TempDir()

	Run(&config.Manifest{}, Options{
		Adopt:       true,
		DryRun:      true,
		DotfilesDir: dotfilesDir,
	})

	// Adopt escribiría este archivo; en dry-run no tiene que existir.
	state := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	if _, err := os.Stat(state); err == nil {
		t.Errorf("--adopt --dry-run escribió el estado en %s", state)
	}
}
