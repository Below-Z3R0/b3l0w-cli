package pacman

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pacman.conf")

	if err := os.WriteFile(path, []byte("[options]\nColor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Configurado(path) {
		t.Fatal("un archivo sin el repo no debería estar configurado")
	}

	if err := os.WriteFile(path, []byte("[options]\nColor\n"+Section), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Configurado(path) {
		t.Fatal("con la sección debería estar configurado")
	}
}

func TestConfiguradoArchivoInexistente(t *testing.T) {
	if Configurado(filepath.Join(t.TempDir(), "no-existe.conf")) {
		t.Fatal("un archivo que no se puede leer no está configurado")
	}
}
