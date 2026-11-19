package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// registryFalso devuelve un registry que emite token y lista los tags
// pedidos. Va por TLS porque Registry.token arma la URL con https://
// fijo: es el precio de que la producción no acepte un registry en
// claro.
func registryFalso(t *testing.T, tags []string) *Registry {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"token":"test"}`)
		case strings.HasSuffix(r.URL.Path, "/tags/list"):
			fmt.Fprintf(w, `{"tags":["%s"]}`, strings.Join(tags, `","`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return &Registry{BaseURL: srv.URL, Client: srv.Client()}
}

// servidorDePrueba escribe un manifiesto con un servicio y su estado
// instalado, y devuelve las rutas.
func servidorDePrueba(t *testing.T, host string) (manifestPath, stateDir string) {
	t.Helper()

	dir := t.TempDir()
	manifestPath = filepath.Join(dir, "services.json")
	stateDir = filepath.Join(dir, "state")

	manifest := manifestJSON(1, fmt.Sprintf(`{"id":"app","name":"App","repo":"Below-Z3R0/app","image":"%s/app","variants":["cpu","rocm"]}`, host))
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	viejo := &State{ID: "app", Variant: "cpu", Image: host + "/app:cpu", Tag: "cpu"}
	if err := viejo.Save(stateDir); err != nil {
		t.Fatal(err)
	}
	return manifestPath, stateDir
}

func TestSwitchVariantAplicaYPersiste(t *testing.T) {
	stub := newDockerStub(t)
	reg := registryFalso(t, []string{"cpu", "rocm"})
	host := strings.TrimPrefix(reg.BaseURL, "https://")
	manifestPath, stateDir := servidorDePrueba(t, host)
	t.Setenv("HOME", t.TempDir())

	res, err := SwitchVariant(VariantOptions{
		ManifestPath: manifestPath,
		StateDir:     stateDir,
		ID:           "app",
		Variant:      "rocm",
		Registry:     reg,
	})
	if err != nil {
		t.Fatalf("SwitchVariant: %v", err)
	}
	if res.Variant != "rocm" {
		t.Errorf("variante: esperaba rocm, obtuve %q", res.Variant)
	}

	// El tag de la variante es el de la variante, no el primero que devuelva
	// el registry: con ["cpu","rocm"] LatestTag tiene que elegir rocm.
	if res.Tag != "rocm" {
		t.Errorf("tag: esperaba rocm, obtuve %q", res.Tag)
	}

	// El estado tiene que reflejar lo que quedó corriendo, no lo que se
	// intentó: variante nueva y la anterior guardada para el rollback.
	nuevo, err := LoadState(stateDir, "app")
	if err != nil {
		t.Fatal(err)
	}
	if nuevo.Variant != "rocm" {
		t.Errorf("estado.variant: esperaba rocm, obtuve %q", nuevo.Variant)
	}
	if nuevo.Previous != host+"/app:cpu" {
		t.Errorf("estado.previous: esperaba la imagen cpu, obtuve %q", nuevo.Previous)
	}
	if nuevo.PreviousTag != "cpu" {
		t.Errorf("estado.previous_tag: esperaba cpu, obtuve %q", nuevo.PreviousTag)
	}

	// Recrear el stack es lo que aplica la variante: sin el up, el
	// contenedor sigue con la imagen vieja.
	var huboPull, huboUp bool
	for _, c := range stub.calls(t) {
		if strings.Contains(c, "pull") {
			huboPull = true
		}
		if strings.Contains(c, "up") {
			huboUp = true
			if !strings.Contains(c, "image=rocm") {
				t.Errorf("el up tiene que llevar IMAGE=rocm: %s", c)
			}
		}
	}
	if !huboPull {
		t.Error("no se hizo pull")
	}
	if !huboUp {
		t.Error("no se recreó el stack")
	}
}

func TestSwitchVariantRechazaLoQueNoCorresponde(t *testing.T) {
	stub := newDockerStub(t)
	reg := registryFalso(t, []string{"cpu"})
	host := strings.TrimPrefix(reg.BaseURL, "https://")
	manifestPath, stateDir := servidorDePrueba(t, host)
	t.Setenv("HOME", t.TempDir())

	casos := []struct {
		nombre  string
		id      string
		variant string
		frag    string
	}{
		{"servicio desconocido", "no-existe", "cpu", "servicio desconocido"},
		{"variante no publicada", "app", "cuda", "no está publicada"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := SwitchVariant(VariantOptions{
				ManifestPath: manifestPath,
				StateDir:     stateDir,
				ID:           c.id,
				Variant:      c.variant,
				Registry:     reg,
			})
			if err == nil {
				t.Fatal("esperaba un error")
			}
			if !strings.Contains(err.Error(), c.frag) {
				t.Errorf("esperaba %q en el error, obtuve %q", c.frag, err)
			}
		})
	}

	// Un caso rechazado no toca el estado ni llama a docker.
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("un rechazo no debe llamar a docker, hubo %d llamadas: %v", len(calls), calls)
	}
}

func TestSwitchVariantSinInstalarFalla(t *testing.T) {
	stub := newDockerStub(t)
	reg := registryFalso(t, []string{"cpu", "rocm"})
	host := strings.TrimPrefix(reg.BaseURL, "https://")
	manifestPath, _ := servidorDePrueba(t, host)
	t.Setenv("HOME", t.TempDir())

	_, err := SwitchVariant(VariantOptions{
		ManifestPath: manifestPath,
		StateDir:     t.TempDir(), // sin estado
		ID:           "app",
		Variant:      "rocm",
		Registry:     reg,
	})
	if err == nil || !strings.Contains(err.Error(), "no está instalado") {
		t.Fatalf("esperaba 'no está instalado', obtuve %v", err)
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("sin instalar no debe llamar a docker: %v", calls)
	}
}

// TestSwitchVariantVerificaElHealthYElProvider: al cambiar de variante
// se verifica el health y el provider de la variante nueva.
//
// Antes quedaban los de la variante anterior: el estado decía "healthy"
// y "CUDA" con el stack de rocm corriendo.
func TestSwitchVariantVerificaElHealthYElProvider(t *testing.T) {
	newDockerStub(t)
	reg := registryFalso(t, []string{"cpu", "rocm"})
	host := strings.TrimPrefix(reg.BaseURL, "https://")

	// El health del servicio nuevo responde 500: no arrancó.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","provider":"CUDA"}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "services.json")
	stateDir := filepath.Join(dir, "state")
	manifest := manifestJSON(1, fmt.Sprintf(
		`{"id":"app","name":"App","repo":"Below-Z3R0/app","image":"%s/app","variants":["cpu","rocm"],"health":"%s"}`,
		host, srv.URL))
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := &State{ID: "app", Variant: "cuda", Image: host + "/app:cuda", Tag: "cuda",
		Health: "healthy", Provider: "CUDA"}
	if err := prev.Save(stateDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())

	_, err := SwitchVariant(VariantOptions{
		ManifestPath:  manifestPath,
		StateDir:      stateDir,
		ID:            "app",
		Variant:       "rocm",
		Registry:      reg,
		HealthTimeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("cambiar a una variante que no arranca no puede ser un éxito")
	}

	saved, err := LoadState(stateDir, "app")
	if err != nil {
		t.Fatalf("no guardó el estado: %v", err)
	}
	if saved.Variant != "rocm" {
		t.Errorf("Variant = %q, quería rocm", saved.Variant)
	}
	if saved.Health == "healthy" {
		t.Errorf("Health = %q, quedó el de la variante anterior", saved.Health)
	}
	if saved.Provider == "CUDA" {
		t.Errorf("Provider = %q, quedó el de la variante anterior", saved.Provider)
	}
}
