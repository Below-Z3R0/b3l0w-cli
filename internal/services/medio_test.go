package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadStateFuerzaElIDDelArchivo fija que el id del JSON no puede
// divergir del nombre del archivo: Save escribiría en otro lado.
func TestLoadStateFuerzaElIDDelArchivo(t *testing.T) {
	dir := t.TempDir()
	// El archivo se llama argos.json y el JSON dice id "otro": si
	// LoadState no fuerza el id del archivo, Save escribiría en otro
	// lado del que se leyó.
	data := []byte(`{"id":"otro","variant":"cpu"}`)
	if err := os.WriteFile(filepath.Join(dir, "argos.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cargado, err := LoadState(dir, "argos")
	if err != nil {
		t.Fatal(err)
	}
	if cargado.ID != "argos" {
		t.Errorf("ID = %q, quería argos (el nombre del archivo)", cargado.ID)
	}
}

// TestTagForDefaultSaltaBuildcache fija que la variante default no elige
// un tag de caché, igual que AvailableVariants.
func TestTagForDefaultSaltaBuildcache(t *testing.T) {
	tags := []string{"buildcache-cpu", "cpu-latest", "latest"}
	got := TagFor(Service{}, "default", tags)
	if strings.HasPrefix(got, "buildcache-") {
		t.Errorf("TagFor eligió %q, un tag de caché", got)
	}
}

// TestSynNoAbortaConElPrimero fija que un servicio inalcanzable no
// cancela el sync de los demás.
func TestSynNoAbortaConElPrimero(t *testing.T) {
	// Sin un registry real, se verifica que el error mencione los
	// inalcanzables en vez de cortar en el primero.
	m := &Manifest{Services: []Service{
		{ID: "a", Image: "127.0.0.1:1/a"},
		{ID: "b", Image: "127.0.0.1:1/b"},
	}}
	_, err := Sync(m, t.TempDir()+"/m.json", NewRegistry("http://127.0.0.1:1"))
	if err == nil {
		t.Fatal("Sync no devolvió error con dos servicios inalcanzables")
	}
	if !strings.Contains(err.Error(), "2 servicios") {
		t.Errorf("error = %q, tenía que contar los dos", err)
	}
}
