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

// TestConfiguradoSoloCuentaLaSeccionExacta: la búsqueda es por línea
// exacta. "[b3l0w-extra]" es otra sección y "# [b3l0w]" está
// comentada (el usuario desactivó el repo a mano): con un substring,
// las dos contaban como configuradas y el diagnóstico mentía.
func TestConfiguradoSoloCuentaLaSeccionExacta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pacman.conf")

	casos := []struct {
		nombre    string
		contenido string
		quiere    bool
	}{
		{"otra sección", "[b3l0w-extra]\nServer = https://ejemplo\n", false},
		{"sección comentada", "# [b3l0w]\n# Server = https://ejemplo\n", false},
		{"la sección exacta", "[b3l0w]\nServer = https://ejemplo\n", true},
		{"la sección exacta con espacios", "   [b3l0w]   \n", true},
		{"la sección dentro de otra", "[otro]\n[b3l0w]\n", true},
	}
	for _, c := range casos {
		if err := os.WriteFile(path, []byte(c.contenido), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := Configurado(path); got != c.quiere {
			t.Errorf("%s: Configurado = %v, quiero %v", c.nombre, got, c.quiere)
		}
	}
}
