package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadCacheRechazaUnJSONRoto fija que una copia truncada no se sirve
// como fresca.
func TestReadCacheRechazaUnJSONRoto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(path, []byte(`{"version":1,"apps":[`), 0o644)
	if _, ok := readCache(path); ok {
		t.Fatal("readCache aceptó un JSON truncado")
	}
}

// TestReadCacheAceptaUnJSONValido es el contraste.
func TestReadCacheAceptaUnJSONValido(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(path, []byte(`{"version":1}`), 0o644)
	if _, ok := readCache(path); !ok {
		t.Fatal("readCache rechazó un JSON válido")
	}
}

// TestWriteCacheNoDejaTemporales fija que el guardado es atómico.
func TestWriteCacheNoDejaTemporales(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	writeCache(path, []byte(`{"version":1}`))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no escribió el archivo: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "x.json" {
			t.Errorf("quedó %s", e.Name())
		}
	}
}
