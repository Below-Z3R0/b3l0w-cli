package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlugNuncaDevuelveUnPath es el bug C2: un ID que termina en ".."
// hacía que slug() devolviera "..", y installOne lo usaba en un
// os.RemoveAll sobre el directorio padre del de plugins.
func TestSlugNuncaDevuelveUnPath(t *testing.T) {
	casos := []string{
		"x/..",
		"x/.",
		"x/../../etc",
		"x/a/b",
		"..",
	}
	for _, id := range casos {
		got := slug(Entry{ID: id})
		if got == "." || got == ".." {
			t.Errorf("slug(%q) = %q, no puede ser un componente relativo", id, got)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("slug(%q) = %q, no puede llevar separadores", id, got)
		}
	}
}

// TestInstallOneNoBorraFueraDelDirectorio es el bug C1: entry.Path
// venía del manifiesto y se usaba crudo en un RemoveAll.
func TestInstallOneNoBorraFueraDelDirectorio(t *testing.T) {
	base := t.TempDir()
	pluginsDir := filepath.Join(base, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un archivo al lado del directorio de plugins: es lo que un "../"
	// se llevaría.
	dato := filepath.Join(base, "dato.txt")
	if err := os.WriteFile(dato, []byte("no me borres"), 0o644); err != nil {
		t.Fatal(err)
	}

	casos := []Entry{
		{ID: "x/..", Path: ".."},
		{ID: "x/../../etc", Path: "../../etc"},
	}
	for _, entry := range casos {
		if err := installOne(entry, pluginsDir, false); err == nil {
			t.Errorf("installOne(%+v) devolvió nil, esperaba un error de nombre", entry)
		}
		if _, err := os.Stat(dato); err != nil {
			t.Fatalf("installOne(%+v) borró fuera de pluginsDir: %v", entry, err)
		}
	}
}

// TestFindRechazaTraversal es el bug A1: Find() componía el path con el
// nombre crudo, así que "../../bin/sh" salía del directorio privado y
// el CLI ejecutaba un binario de cualquier parte.
func TestFindRechazaTraversal(t *testing.T) {
	casos := []string{
		"../../bin/sh",
		"..",
		".",
		"a/b",
		`a\b`,
	}
	for _, name := range casos {
		if got := Find(name); got != "" {
			t.Errorf("Find(%q) = %q, quería vacío", name, got)
		}
	}
}
