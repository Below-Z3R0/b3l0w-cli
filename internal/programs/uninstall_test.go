package programs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
)

// TestUninstallGuardaEnLaRutaDelManifiesto es el bug M4: Uninstall
// guardaba siempre en paths.ManifestPath(), ignorando el manifiesto que
// había recibido. Con una ruta propia, tiene que guardar ahí y no
// tocar el manifiesto del CLI.
func TestUninstallGuardaEnLaRutaDelManifiesto(t *testing.T) {
	// El manifiesto del CLI va a un lado; el del test, a otro.
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	propio := filepath.Join(t.TempDir(), "mi-manifiesto.json")

	fakePacman(t, nil, nil) // sin pacman: el camino del binario suelto

	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin", "prog")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := &config.Manifest{
		Version:  1,
		Programs: []config.Program{{Name: "prog", Source: "github:test/prog"}},
	}
	if _, err := UninstallTo("prog", m, propio); err != nil {
		t.Fatalf("UninstallTo: %v", err)
	}

	// Se guardó en la ruta que le pasaron.
	if _, err := os.Stat(propio); err != nil {
		t.Fatalf("no se guardó el manifiesto en la ruta que se le pasó: %v", err)
	}
	// Y el manifiesto del CLI no se tocó.
	if _, err := os.Stat(paths.ManifestPath()); err == nil {
		t.Fatalf("Uninstall escribió en %s en vez de la ruta que recibió", paths.ManifestPath())
	}
}
