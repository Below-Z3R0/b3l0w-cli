package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadRechazaUnaVersionFutura fija que un manifiesto de una versión
// más nueva no se carga: guardarlo perdería campos que no entiende.
func TestLoadRechazaUnaVersionFutura(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(path, []byte(`{"version":99,"programs":[]}`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("Load aceptó un manifiesto de versión 99")
	} else if !strings.Contains(err.Error(), "más nueva") {
		t.Errorf("error = %q", err)
	}
}

// TestLoadNormalizaUnJSONVacio fija que null o {} no dejan Programs nil.
func TestLoadNormalizaUnJSONVacio(t *testing.T) {
	for _, contenido := range []string{"null", "{}", `{"programs":null}`} {
		path := filepath.Join(t.TempDir(), "p.json")
		os.WriteFile(path, []byte(contenido), 0o644)
		m, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", contenido, err)
		}
		if m.Programs == nil {
			t.Errorf("%s: Programs = nil", contenido)
		}
	}
}

// TestGetDevuelveUnaCopia fija que Remove no deja el puntero colgando.
func TestGetDevuelveUnaCopia(t *testing.T) {
	m := Default()
	p := m.Get("kitty-shapes")
	if p == nil {
		t.Fatal("no encontró kitty-shapes")
	}
	p.Name = "pisado"
	if m.Get("kitty-shapes") == nil {
		t.Fatal("Get devolvió un puntero al slice: el manifiesto cambió")
	}
}

// TestRemoveNoDejaBasura fija que Remove limpia la cola del array.
//
// Mirar solo len(m.Programs) no prueba nada: append da el len correcto
// y deja el último elemento copiado en el array subyacente.
func TestRemoveNoDejaBasura(t *testing.T) {
	m := &Manifest{Version: 1, Programs: []Program{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}}
	backing := m.Programs
	m.Remove("a")
	if len(m.Programs) != 2 {
		t.Fatalf("quedaron %d programas, quería 2", len(m.Programs))
	}
	// backing[2] es la cola que append deja con "c" duplicado.
	if backing[2].Name == "c" {
		t.Error("el array subyacente conserva una copia del último elemento")
	}
}

// TestSaveEsAtomico fija que el guardado no trunca el archivo original.
func TestSaveEsAtomico(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	m := Default()
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	// No quedan temporales.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".programs-") {
			t.Errorf("quedó el temporal %s", e.Name())
		}
	}
}
