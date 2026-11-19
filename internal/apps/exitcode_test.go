package apps

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecPropagaElExitCode fija que el exit code de pacman llega al
// llamador. Antes cualquier fallo devolvía 1: un 127 (binario ausente) y
// un 1 (transacción fallida) son cosas distintas para un script.
func TestExecPropagaElExitCode(t *testing.T) {
	dir := t.TempDir()
	// pacman falso que sale con 3.
	script := "#!/bin/sh\nexit 3\n"
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := exec(false, "-Syu"); got != 3 {
		t.Fatalf("exit code = %d, quería 3 (el de pacman, no un 1 genérico)", got)
	}
}

// TestExecDevuelveCeroCuandoTodoVaBien es el contraste.
func TestExecDevuelveCeroCuandoTodoVaBien(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := exec(false, "-Syu"); got != 0 {
		t.Fatalf("exit code = %d, quería 0", got)
	}
}
