package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoConfigurado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pacman.conf")
	viejo := pacmanConf
	pacmanConf = path
	t.Cleanup(func() { pacmanConf = viejo })

	// Sin archivo.
	if repoConfigurado() {
		t.Error("sin archivo no debería estar configurado")
	}

	// Con otros repos pero sin b3l0w.
	os.WriteFile(path, []byte("[core]\nServer = x\n"), 0o644)
	if repoConfigurado() {
		t.Error("no debería detectar b3l0w donde no está")
	}

	// Con la sección.
	os.WriteFile(path, []byte("[core]\n[b3l0w]\nSigLevel = Required\n"), 0o644)
	if !repoConfigurado() {
		t.Error("debería detectar la sección")
	}
}
