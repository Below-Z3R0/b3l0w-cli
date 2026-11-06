package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPackageOwnerDetectaArchivoDePacman comprueba la regla que impide
// que el CLI pise un binario del sistema.
func TestPackageOwnerDetectaArchivoDePacman(t *testing.T) {
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman no está instalado")
	}
	if _, err := os.Stat("/usr/bin/pacman"); err != nil {
		t.Skip("/usr/bin/pacman no existe")
	}

	if owner := PackageOwner("/usr/bin/pacman"); owner != "pacman" {
		t.Errorf("PackageOwner(/usr/bin/pacman) = %q, quería pacman", owner)
	}
}

// TestPackageOwnerIgnoraArchivoPropio cubre el caso normal: un binario
// en ~/.local/bin no pertenece a ningún paquete.
func TestPackageOwnerIgnoraArchivoPropio(t *testing.T) {
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman no está instalado")
	}

	path := filepath.Join(t.TempDir(), "programa-propio")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho v1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if owner := PackageOwner(path); owner != "" {
		t.Errorf("PackageOwner(%s) = %q, quería vacío", path, owner)
	}
}

// TestPackageOwnerIgnoraInexistente no debe romper con rutas que no
// existen.
func TestPackageOwnerIgnoraInexistente(t *testing.T) {
	if owner := PackageOwner(filepath.Join(t.TempDir(), "no-existe")); owner != "" {
		t.Errorf("PackageOwner(inexistente) = %q, quería vacío", owner)
	}
}
