package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSinArchivoDevuelveDefault(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "no-existe.json"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(m.Programs) == 0 {
		t.Fatal("el default no trae programas")
	}
	if m.Get("boot-preview") == nil || m.Get("kitty-shapes") == nil {
		t.Fatalf("faltan los programas por defecto: %+v", m.Programs)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "programs.json")
	m := Default()
	if _, err := m.Add("otro", "github:algo/otro"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no se creó el archivo: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Programs) != len(m.Programs) {
		t.Fatalf("programas = %d, quiero %d", len(loaded.Programs), len(m.Programs))
	}
	if loaded.Get("otro") == nil {
		t.Fatal("se perdió el programa agregado")
	}
}

func TestAddEsIdempotente(t *testing.T) {
	m := Default()
	if ok, _ := m.Add("boot-preview", "github:algo/boot-preview"); ok {
		t.Fatal("agregar uno existente devolvió true")
	}
	if ok, err := m.Add("nuevo", "github:algo/nuevo"); !ok || err != nil {
		t.Fatalf("agregar uno nuevo devolvió false: %v", err)
	}
	if ok, _ := m.Add("nuevo", "github:algo/nuevo"); ok {
		t.Fatal("agregar dos veces devolvió true la segunda")
	}
}

// TestAddValidaAlRegistrar fija que un programa mal cargado falla al
// registrarse, no tres comandos después al instalarlo.
func TestAddValidaAlRegistrar(t *testing.T) {
	casos := []struct {
		nombre string
		name   string
		source string
	}{
		{"nombre vacío", "", "github:algo/x"},
		{"nombre con /", "a/b", "github:algo/x"},
		{"nombre ..", "..", "github:algo/x"},
		{"fuente sin github:", "x", "gitlab:algo/x"},
		{"fuente con ruta extra", "x", "github:algo/x/extra"},
		{"fuente sin repo", "x", "github:algo"},
	}
	for _, c := range casos {
		m := Default()
		if ok, err := m.Add(c.name, c.source); ok || err == nil {
			t.Errorf("%s: Add(%q, %q) = (%v, %v), quería false y error", c.nombre, c.name, c.source, ok, err)
		}
		if m.Get(c.name) != nil {
			t.Errorf("%s: el programa inválido quedó en el manifiesto", c.nombre)
		}
	}
}

func TestRemove(t *testing.T) {
	m := Default()
	if !m.Remove("kitty-shapes") {
		t.Fatal("no borró un programa existente")
	}
	if m.Get("kitty-shapes") != nil {
		t.Fatal("el programa sigue ahí")
	}
	if m.Remove("kitty-shapes") {
		t.Fatal("borrar de nuevo devolvió true")
	}
}
