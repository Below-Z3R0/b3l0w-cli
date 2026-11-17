package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// manifestJSON arma un manifiesto valido en JSON.
func manifestJSON(version int, services string) string {
	return `{"version":` + strconv.Itoa(version) + `,"services":[` + services + `]}`
}

// TestParseManifestLeeElManifiesto: el manifiesto tiene version,
// servicios y los campos de cada uno.
func TestParseManifestLeeElManifiesto(t *testing.T) {
	data := []byte(manifestJSON(1, `{
		"id": "argos",
		"name": "Argos",
		"repo": "Below-Z3R0/argos",
		"image": "ghcr.io/below-z3r0/argos",
		"variants": ["cpu", "cuda"],
		"available": ["cpu"],
		"gpu": true,
		"ep": {"cpu": "CPU", "cuda": "CUDA"},
		"ports": [6333, 7711],
		"health": "http://127.0.0.1:7712/readyz",
		"state": "ready",
		"size_gb": 8.7,
		"overrides": {"cuda": "compose.gpu.yml"},
		"description": "Búsqueda semántica",
		"components": ["api", "worker"],
		"models": "argosctl models install",
		"compose": "compose.yaml"
	}`))
	m, err := parseManifest(data)
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, quería 1", m.Version)
	}
	if len(m.Services) != 1 {
		t.Fatalf("%d servicios, quería 1", len(m.Services))
	}
	svc := m.Services[0]
	if svc.ID != "argos" || svc.Name != "Argos" {
		t.Errorf("servicio = %+v", svc)
	}
	if svc.Image != "ghcr.io/below-z3r0/argos" {
		t.Errorf("Image = %q", svc.Image)
	}
	if len(svc.Variants) != 2 || svc.Variants[1] != "cuda" {
		t.Errorf("Variants = %v", svc.Variants)
	}
	if !svc.GPU {
		t.Error("GPU = false, quería true")
	}
	if svc.EP["cuda"] != "CUDA" {
		t.Errorf("EP = %v", svc.EP)
	}
	if len(svc.Ports) != 2 || svc.Ports[0] != 6333 {
		t.Errorf("Ports = %v", svc.Ports)
	}
	if svc.Health != "http://127.0.0.1:7712/readyz" {
		t.Errorf("Health = %q", svc.Health)
	}
	if svc.State != "ready" {
		t.Errorf("State = %q", svc.State)
	}
	if svc.SizeGB != 8.7 {
		t.Errorf("SizeGB = %v, quería 8.7", svc.SizeGB)
	}
	if svc.Overrides["cuda"] != "compose.gpu.yml" {
		t.Errorf("Overrides = %v", svc.Overrides)
	}
	if len(svc.Components) != 2 {
		t.Errorf("Components = %v", svc.Components)
	}
	if svc.Models == "" || svc.Description == "" {
		t.Errorf("faltan campos: %+v", svc)
	}
}

// TestParseManifestRechazaLoInvalido: un manifiesto roto tiene que
// fallar, no devolver un manifiesto vacio que el CLI despiese.
func TestParseManifestRechazaLoInvalido(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"json roto", `{"version":1,`, "unexpected end"},
		{"sin version", `{"services":[{"id":"argos"}]}`, "vacío o sin versión"},
		{"sin servicios", `{"version":1,"services":[]}`, "vacío o sin versión"},
		{"id con traversal", manifestJSON(1, `{"id":"../escape"}`), "id inválido"},
		{"id con barra", manifestJSON(1, `{"id":"a/b"}`), "id inválido"},
		{"id vacio", manifestJSON(1, `{"id":""}`), "id inválido"},
	}
	for _, c := range cases {
		_, err := parseManifest([]byte(c.data))
		if err == nil {
			t.Errorf("%s: no falló", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q, quería que contenga %q", c.name, err, c.want)
		}
	}
}

// TestParseManifestAceptaElManifiestoEmbebido: el manifiesto que va
// dentro del binario tiene que ser valido.
func TestParseManifestAceptaElManifiestoEmbebido(t *testing.T) {
	m, err := parseManifest(Embedded())
	if err != nil {
		t.Fatalf("el manifiesto embebido no parsea: %v", err)
	}
	if m.Version == 0 {
		t.Error("el manifiesto embebido no tiene version")
	}
	if len(m.Services) == 0 {
		t.Fatal("el manifiesto embebido no tiene servicios")
	}
	for _, svc := range m.Services {
		if svc.ID == "" {
			t.Errorf("servicio sin id: %+v", svc)
		}
		if svc.State == "" {
			t.Errorf("servicio sin state: %+v", svc)
		}
	}
}

// TestLoadManifestLeeElArchivo: el manifiesto se lee del path que le
// pasan.
func TestLoadManifestLeeElArchivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.json")
	content := manifestJSON(2, `{"id":"sonus","name":"Sonus","state":"ready"}`)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Version != 2 {
		t.Errorf("Version = %d, quería 2", m.Version)
	}
	if m.Find("sonus") == nil {
		t.Error("no encontró sonus")
	}
}

// TestLoadManifestFallaSiNoEstaElArchivo.
func TestLoadManifestFallaSiNoEstaElArchivo(t *testing.T) {
	_, err := LoadManifest(filepath.Join(t.TempDir(), "no-existe.json"))
	if err == nil {
		t.Fatal("cargó un manifiesto que no existe")
	}
}

// TestManifestFindDevuelveElServicioYNilSiNoEsta.
func TestManifestFindDevuelveElServicioYNilSiNoEsta(t *testing.T) {
	m := &Manifest{Version: 1, Services: []Service{{ID: "argos"}, {ID: "sonus"}}}
	svc := m.Find("sonus")
	if svc == nil {
		t.Fatal("no encontró sonus")
	}
	if svc.ID != "sonus" {
		t.Errorf("ID = %q", svc.ID)
	}
	if m.Find("noexiste") != nil {
		t.Error("devolvió un servicio que no existe")
	}
}

// TestSaveManifestOrdenaPorID: el manifiesto se guarda ordenado por id
// para que el diff no sea ruido.
func TestSaveManifestOrdenaPorID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.json")
	m := &Manifest{Version: 1, Services: []Service{
		{ID: "sonus", State: "ready"},
		{ID: "argos", State: "ready"},
		{ID: "argos2", State: "ready"},
	}}
	if err := SaveManifest(path, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reread Manifest
	if err := json.Unmarshal(data, &reread); err != nil {
		t.Fatalf("el manifiesto guardado no parsea: %v", err)
	}
	if len(reread.Services) != 3 {
		t.Fatalf("%d servicios, quería 3", len(reread.Services))
	}
	want := []string{"argos", "argos2", "sonus"}
	for i, id := range want {
		if reread.Services[i].ID != id {
			t.Errorf("servicio %d = %q, quería %q", i, reread.Services[i].ID, id)
		}
	}
	// Y termina en newline: el archivo es texto, no un blob json.
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("el manifiesto no termina en newline")
	}
}

// TestSaveManifestCreaElDirectorio: guardar en un path nuevo crea el
// directorio.
func TestSaveManifestCreaElDirectorio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "services.json")
	m := &Manifest{Version: 1, Services: []Service{{ID: "argos", State: "ready"}}}
	if err := SaveManifest(path, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("no wrote el manifiesto: %v", err)
	}
}

// TestSaveManifestYLoadManifestEsUnRoundTrip: lo que se guarda se
// vuelve a leer igual.
func TestSaveManifestYLoadManifestEsUnRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	original := &Manifest{Version: 1, Services: []Service{{
		ID: "argos", Name: "Argos", Repo: "Below-Z3R0/argos",
		Image:    "ghcr.io/below-z3r0/argos",
		Variants: []string{"cpu", "cuda"}, Available: []string{"cpu"},
		GPU: true, EP: map[string]string{"cpu": "CPU"},
		Ports: []int{6333, 7711}, Health: "http://127.0.0.1:7712/readyz",
		State: "ready", SizeGB: 8.7,
		Overrides:  map[string]string{"cuda": "compose.gpu.yml"},
		Components: []string{"api"}, Description: "Busqueda", Models: "ctl models",
	}}}
	if err := SaveManifest(path, original); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(loaded.Services) != 1 {
		t.Fatalf("%d servicios, quería 1", len(loaded.Services))
	}
	got := loaded.Services[0]
	if got.ID != original.Services[0].ID ||
		got.Image != original.Services[0].Image ||
		got.Health != original.Services[0].Health ||
		got.SizeGB != original.Services[0].SizeGB ||
		got.EP["cpu"] != "CPU" ||
		got.Overrides["cuda"] != "compose.gpu.yml" ||
		len(got.Variants) != 2 || len(got.Ports) != 2 ||
		len(got.Components) != 1 {
		t.Errorf("el round trip perdio datos: %+v", got)
	}
}

// TestStateSaveYLoadEsUnRoundTrip: el estado guardado se vuelve a leer
// igual, campo por campo.
func TestStateSaveYLoadEsUnRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &State{
		ID: "argos", Variant: "cuda",
		Image:       "ghcr.io/below-z3r0/argos:cuda-latest",
		Tag:         "cuda-latest",
		Digest:      "ghcr.io/below-z3r0/argos@sha256:abc",
		InstalledAt: "2026-01-02T03:04:05-03:00",
		UpdatedAt:   "2026-02-03T04:05:06-03:00",
		Health:      "healthy",
		Provider:    "CUDA",
		Ports:       []int{6333, 7711},
		Previous:    "ghcr.io/below-z3r0/argos:cpu-latest",
		PreviousTag: "cpu-latest",
		Services:    []string{"argos-api", "argos-worker"},
		Warnings:    []string{"sin modelos cargados"},
	}
	if err := original.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadState(dir, "argos")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.ID != original.ID ||
		loaded.Variant != original.Variant ||
		loaded.Image != original.Image ||
		loaded.Tag != original.Tag ||
		loaded.Digest != original.Digest ||
		loaded.InstalledAt != original.InstalledAt ||
		loaded.UpdatedAt != original.UpdatedAt ||
		loaded.Health != original.Health ||
		loaded.Provider != original.Provider ||
		loaded.Previous != original.Previous ||
		loaded.PreviousTag != original.PreviousTag {
		t.Errorf("el round trip perdio datos: %+v", loaded)
	}
	if len(loaded.Ports) != 2 || loaded.Ports[0] != 6333 {
		t.Errorf("Ports = %v", loaded.Ports)
	}
	if len(loaded.Services) != 2 {
		t.Errorf("Services = %v", loaded.Services)
	}
	if len(loaded.Warnings) != 1 {
		t.Errorf("Warnings = %v", loaded.Warnings)
	}
}

// TestStateSaveEscribeElArchivoDelID: el nombre del archivo es el id
// del servicio.
func TestStateSaveEscribeElArchivoDelID(t *testing.T) {
	dir := t.TempDir()
	s := &State{ID: "argos", Image: "ghcr.io/org/img:tag"}
	if err := s.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "argos.json")); err != nil {
		t.Errorf("no wrote argos.json: %v", err)
	}
}

// TestStateSaveCreaElDirectorio: el directorio de estado se crea si no
// existe.
func TestStateSaveCreaElDirectorio(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "services")
	s := &State{ID: "argos"}
	if err := s.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "argos.json")); err != nil {
		t.Errorf("no wrote el estado: %v", err)
	}
}

// TestLoadStateFallaSiNoEsta.
func TestLoadStateFallaSiNoEsta(t *testing.T) {
	_, err := LoadState(t.TempDir(), "argos")
	if err == nil {
		t.Fatal("cargó un estado que no existe")
	}
}

// TestListStatesLeeTodosYOrdena: lista los estados del directorio,
// ordenados por id.
func TestListStatesLeeTodosYOrdena(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"sonus", "argos"} {
		s := &State{ID: id, Image: "ghcr.io/org/" + id + ":tag"}
		if err := s.Save(dir); err != nil {
			t.Fatal(err)
		}
	}
	states, err := ListStates(dir)
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("%d estados, quería 2", len(states))
	}
	if states[0].ID != "argos" || states[1].ID != "sonus" {
		t.Errorf("orden = %s, %s", states[0].ID, states[1].ID)
	}
}

// TestListStatesIgnoraLoQueNoEsEstado: un directorio sin estados no es
// un error, y los archivos que no son estados se saltan.
func TestListStatesIgnoraLoQueNoEsEstado(t *testing.T) {
	dir := t.TempDir()
	// Sin directorio: no hay estados y no es error.
	states, err := ListStates(filepath.Join(dir, "no-existe"))
	if err != nil {
		t.Errorf("ListStates sin directorio: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("estado sin directorio: %v", states)
	}

	// Con un estado valido y ruido alrededor.
	s := &State{ID: "argos"}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notas.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roto.json"), []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	states, err = ListStates(dir)
	if err != nil {
		t.Fatalf("ListStates: %v", err)
	}
	if len(states) != 1 || states[0].ID != "argos" {
		t.Errorf("estados = %v, quería solo argos", states)
	}
}

// TestStateSaveConDirectorioSinPermisosFalla: si no puede escribir,
// el error se propaga (no se pierde en silencio).
func TestStateSaveConDirectorioSinPermisosFalla(t *testing.T) {
	// Un path donde hay un archivo no puede ser un directorio.
	dir := t.TempDir()
	bloqueado := filepath.Join(dir, "bloqueado")
	if err := os.WriteFile(bloqueado, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &State{ID: "argos"}
	if err := s.Save(bloqueado); err == nil {
		t.Fatal("Save deberia fallar si no puede escribir")
	}
}

// TestDefaultStateDirEsElEsperado: el estado vive en el directorio de
// estado de la distro.
func TestDefaultStateDirEsElEsperado(t *testing.T) {
	dir := DefaultStateDir()
	if !strings.HasSuffix(dir, filepath.Join(".local", "state", "b3l0w", "services")) {
		t.Errorf("DefaultStateDir = %q", dir)
	}
}

// TestDefaultManifestPathEsElEsperado: el manifiesto por defecto vive
// en el repo de distro.
func TestDefaultManifestPathEsElEsperado(t *testing.T) {
	path := DefaultManifestPath()
	if !strings.HasSuffix(path, filepath.Join("services", "services.json")) {
		t.Errorf("DefaultManifestPath = %q", path)
	}
}
