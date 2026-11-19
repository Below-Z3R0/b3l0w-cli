package programs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// TestAssetURLRechazaRutaExtra es el bug M3: `github:owner/repo/extra`
// pasaba la validación y la ruta extra se cuelaba en la URL del asset.
// El error tiene que ser de manifiesto, no un 404 de curl más tarde.
func TestAssetURLRechazaRutaExtra(t *testing.T) {
	casos := []struct {
		source string
		quiere string
	}{
		{"github:owner/repo/extra", ""},
		{"github:owner/repo/sub/extra", ""},
		{"github:owner/", ""},
		{"github:/repo", ""},
		{"github:owner", ""},
		{"gitlab:owner/repo", ""},
	}
	for _, c := range casos {
		got, err := AssetURL(config.Program{Name: "x", Source: c.source, Asset: "a"})
		if err == nil {
			t.Errorf("AssetURL(%q) = %q sin error: la ruta extra tiene que rechazarse", c.source, got)
		}
		if got != "" {
			t.Errorf("AssetURL(%q) devolvió %q además del error", c.source, got)
		}
	}

	// El camino feliz sigue igual.
	got, err := AssetURL(config.Program{Name: "x", Source: "github:owner/repo", Asset: "a"})
	if err != nil {
		t.Fatalf("AssetURL(github:owner/repo) falló: %v", err)
	}
	want := "https://github.com/owner/repo/releases/latest/download/a"
	if got != want {
		t.Fatalf("AssetURL = %q, quiero %q", got, want)
	}
}

// fakeCurl pone un `curl` falso en el PATH que anota que se lo llamó.
//
// Sirve para probar que una instalación inválida NO llega a descargar:
// el error tiene que ser de configuración y no de red.
func fakeCurl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	marca := filepath.Join(dir, "curl-corrio")
	script := "#!/bin/sh\ntouch \"$FAKE_CURL_MARKER\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CURL_MARKER", marca)
	return marca
}

// TestInstallNoDescargaConManifiestoInvalido es el bug M5: el
// manifiesto se validaba después de bajar el asset, así que un error de
// configuración llegaba tapado por el 404 de curl (o por un 404 que no
// era el problema real).
//
// Con el manifiesto inválido, el error tiene que ser de configuración y
// el curl no tiene que haber corrido.
func TestInstallNoDescargaConManifiestoInvalido(t *testing.T) {
	marca := fakeCurl(t)

	casos := []struct {
		nombre string
		prog   config.Program
		quiere string
	}{
		{
			nombre: "fuente con ruta extra",
			prog:   config.Program{Name: "x", Source: "github:owner/repo/extra", Asset: "a"},
			quiere: "fuente no soportada",
		},
		{
			nombre: "fuente que no es github",
			prog:   config.Program{Name: "x", Source: "gitlab:owner/repo", Asset: "a"},
			quiere: "fuente no soportada",
		},
		{
			nombre: "asset vacío",
			prog:   config.Program{Name: "x", Source: "github:owner/repo"},
			quiere: "no tiene asset",
		},
		{
			nombre: "asset que es una ruta",
			prog:   config.Program{Name: "x", Source: "github:owner/repo", Asset: "sub/asset"},
			quiere: "no es un nombre de archivo",
		},
	}

	for _, c := range casos {
		_, err := InstallVerified(c.prog, false)
		if err == nil {
			t.Errorf("%s: InstallVerified no dio error", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.quiere) {
			t.Errorf("%s: error = %q, quiero que diga %q", c.nombre, err, c.quiere)
		}
		if _, statErr := os.Stat(marca); statErr == nil {
			t.Errorf("%s: se intentó descargar con un manifiesto inválido", c.nombre)
		}
	}
}
