package plugins

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestParseAceptaUnaListaVacia fija que un manifiesto con version:1 y
// plugins:[] es válido: es el manifiesto con todos los plugins dados de
// baja, no un archivo corrupto.
func TestParseAceptaUnaListaVacia(t *testing.T) {
	m, err := parse([]byte(`{"version":1,"plugins":[]}`))
	if err != nil {
		t.Fatalf("parse rechazó una lista vacía: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("version = %d, quería 1", m.Version)
	}
}

// TestParseRechazaSinVersion fija el otro caso: sin version no se puede
// saber el formato.
func TestParseRechazaSinVersion(t *testing.T) {
	if _, err := parse([]byte(`{"plugins":[]}`)); err == nil {
		t.Fatal("parse aceptó un manifiesto sin versión")
	}
}

// TestExtractPluginNoDependeDelNombreDelRepo fija que el tarball se busca
// por el nombre del plugin: la carpeta raíz cambia si el repo se renombra
// o la rama por defecto cambia.
func TestExtractPluginNoDependeDelNombreDelRepo(t *testing.T) {
	// Un tarball con una raíz distinta a "b3l0w-plugins-main".
	tarball := filepath.Join(t.TempDir(), "p.tar.gz")
	f, err := os.Create(tarball)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	contenido := []byte("id = \"we\"\n")
	base := "otro-repo-otra-rama/we/"
	tw.WriteHeader(&tar.Header{Name: base + "plugin.toml", Mode: 0o644, Size: int64(len(contenido))})
	tw.Write(contenido)
	tw.Close()
	gz.Close()
	f.Close()

	dest := t.TempDir()
	if err := extractPlugin(tarball, "we", dest); err != nil {
		t.Fatalf("extractPlugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "plugin.toml")); err != nil {
		t.Fatalf("el plugin no se extrajo: %v", err)
	}
}

// TestExtractPluginIgnoraOtrosPlugins fija que no se cuela lo de otro
// plugin del mismo tarball.
func TestExtractPluginIgnoraOtrosPlugins(t *testing.T) {
	tarball := filepath.Join(t.TempDir(), "p.tar.gz")
	f, err := os.Create(tarball)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	datos := []byte("x")
	for _, nombre := range []string{"repo-main/we/plugin.toml", "repo-main/docker/plugin.toml"} {
		tw.WriteHeader(&tar.Header{Name: nombre, Mode: 0o644, Size: int64(len(datos))})
		tw.Write(datos)
	}
	tw.Close()
	gz.Close()
	f.Close()

	dest := t.TempDir()
	if err := extractPlugin(tarball, "we", dest); err != nil {
		t.Fatalf("extractPlugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "docker")); err == nil {
		t.Error("se extrajo el plugin de otro id")
	}
}
