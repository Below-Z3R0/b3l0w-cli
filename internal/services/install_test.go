package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistry levanta un registry falso y devuelve el Registry que
// habla con el.
//
// El Registry del paquete arma las URLs a mano, asi que el host de la
// imagen tiene que ser el del server de prueba. El token y el digest
// salen de headers: el digest remoto viene en "Docker-Content-Digest",
// como en un registry de verdad.
func fakeRegistry(t *testing.T, tags []string, digest string) (*Registry, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"fake"}`))
	})
	mux.HandleFunc("/v2/repo/tags/list", func(w http.ResponseWriter, r *http.Request) {
		body := `{"tags":[`
		for i, tag := range tags {
			if i > 0 {
				body += ","
			}
			body += `"` + tag + `"`
		}
		body += `]}`
		w.Write([]byte(body))
	})
	mux.HandleFunc("/v2/repo/manifests/", func(w http.ResponseWriter, r *http.Request) {
		if digest == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest)
		w.Write([]byte(`{}`))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "https://")
	reg := &Registry{BaseURL: srv.URL, Client: srv.Client()}
	return reg, host
}

// TestUpdateNoActualizaSiElDigestEsElMismo compara el digest local con
// el remoto.
//
// El local viene de "docker image inspect" como RepoDigest
// ("ghcr.io/org/img@sha256:...") y el remoto del header
// "Docker-Content-Digest" ("sha256:..."). Si no se normalizan, los dos
// nunca dan igual y cada corrida del update rehace el stack.
func TestUpdateNoActualizaSiElDigestEsElMismo(t *testing.T) {
	stub := newDockerStub(t)
	digest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, digest)

	in := &Installer{
		Docker:   NewDocker(),
		Registry: reg,
		StateDir: t.TempDir(),
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}}
	state := &State{
		ID: "argos", Variant: "cpu",
		Image:  host + "/repo:cpu-latest",
		Tag:    "cpu-latest",
		Digest: host + "/repo@" + digest, // forma local, con repo delante
	}

	out, err := in.Update(svc, state, true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.UpdatedAt != "" {
		t.Errorf("marcó UpdatedAt sin cambio: %q", out.UpdatedAt)
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("tocó docker sin cambio de digest: %v", calls)
	}
}

// TestUpdateActualizaSiElDigestCambia: si el digest remoto es otro,
// hay pull, recreate y estado nuevo.
func TestUpdateActualizaSiElDigestCambia(t *testing.T) {
	stub := newDockerStub(t)
	t.Setenv("STUB_INSPECT_OUT", "host/repo@sha256:nuevo")
	remoto := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, remoto)

	stateDir := t.TempDir()
	in := &Installer{
		Docker:   NewDocker(),
		Registry: reg,
		StateDir: stateDir,
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}}
	state := &State{
		ID: "argos", Variant: "cpu",
		Image:  host + "/repo:cpu-latest",
		Tag:    "cpu-latest",
		Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}

	out, err := in.Update(svc, state, true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.UpdatedAt == "" {
		t.Error("no marcó UpdatedAt")
	}
	if pulls := stub.callsContaining(t, "pull"); len(pulls) != 1 {
		t.Errorf("pulls = %v, quería 1", pulls)
	}
	if ups := stub.callsContaining(t, "up -d"); len(ups) != 1 {
		t.Errorf("ups = %v, quería 1", ups)
	}
	// El estado se guarda con el digest nuevo.
	saved, err := LoadState(stateDir, "argos")
	if err != nil {
		t.Fatalf("no guardó el estado: %v", err)
	}
	if saved.Digest != "host/repo@sha256:nuevo" {
		t.Errorf("Digest = %q", saved.Digest)
	}
}

// TestUpdateFallaSiElRegistryNoDevuelveDigest: un registry que no
// manda el header "Docker-Content-Digest" no se puede comparar, asi
// que el update avisa y no toca docker.
func TestUpdateFallaSiElRegistryNoDevuelveDigest(t *testing.T) {
	stub := newDockerStub(t)
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, "")

	in := &Installer{
		Docker:   NewDocker(),
		Registry: reg,
		StateDir: t.TempDir(),
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}}
	state := &State{ID: "argos", Image: host + "/repo:cpu-latest", Tag: "cpu-latest"}

	if _, err := in.Update(svc, state, true, true); err == nil {
		t.Fatal("actualizó sin digest remoto")
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("tocó docker sin digest remoto: %v", calls)
	}
}

// TestUpdateNoActualizaSiElDigestRemotoQuedaVacio: un digest remoto
// que al normalizar queda vacio no se puede comparar, asi que el
// servicio se deja como estaba.
func TestUpdateNoActualizaSiElDigestRemotoQuedaVacio(t *testing.T) {
	stub := newDockerStub(t)
	// "@" se normaliza a cadena vacia: no hay digest que comparar.
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, "@")

	in := &Installer{
		Docker:   NewDocker(),
		Registry: reg,
		StateDir: t.TempDir(),
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}}
	state := &State{ID: "argos", Image: host + "/repo:cpu-latest", Tag: "cpu-latest"}

	out, err := in.Update(svc, state, true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.UpdatedAt != "" {
		t.Errorf("actualizó con digest remoto vacío: %q", out.UpdatedAt)
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("tocó docker con digest remoto vacío: %v", calls)
	}
}

// TestUpdateFallaSiNoEstaInstalado.
func TestUpdateFallaSiNoEstaInstalado(t *testing.T) {
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, "sha256:abc")
	in := &Installer{
		Docker:   NewDocker(),
		Registry: reg,
		StateDir: t.TempDir(),
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}}
	if _, err := in.Update(svc, nil, true, true); err == nil {
		t.Fatal("actualizó un servicio que no está instalado")
	}
	// Tampoco con un estado sin imagen.
	if _, err := in.Update(svc, &State{ID: "argos"}, true, true); err == nil {
		t.Fatal("actualizó un estado sin imagen")
	}
}

// TestResolveVariantEligeLaDelUsuario: si el usuario pide una
// variante, esa gana aunque no sea la del hardware.
func TestResolveVariantEligeLaDelUsuario(t *testing.T) {
	svc := Service{ID: "argos", Variants: []string{"cpu", "cuda"}}
	published := []string{"cpu", "cuda"}
	got, reason, err := ResolveVariant(svc, Hardware{}, published, "cuda")
	if err != nil {
		t.Fatalf("ResolveVariant: %v", err)
	}
	if got != "cuda" {
		t.Errorf("eligió %q, quería cuda", got)
	}
	if reason == "" {
		t.Error("sin motivo")
	}
}

// TestResolveVariantFallaConUnaVarianteNoPublicada: pedir una
// variante que no existe es un error, no un fallback silencioso.
func TestResolveVariantFallaConUnaVarianteNoPublicada(t *testing.T) {
	svc := Service{ID: "argos", Variants: []string{"cpu", "cuda"}}
	_, _, err := ResolveVariant(svc, Hardware{}, []string{"cpu"}, "cuda")
	if err == nil {
		t.Fatal("eligió una variante no publicada")
	}
	if !strings.Contains(err.Error(), "no está publicada") {
		t.Errorf("mensaje inesperado: %v", err)
	}
}

// TestResolveVariantFallaSinVariantesPublicadas.
func TestResolveVariantFallaSinVariantesPublicadas(t *testing.T) {
	svc := Service{ID: "argos", Variants: []string{"cpu"}}
	_, _, err := ResolveVariant(svc, Hardware{}, nil, "")
	if err == nil {
		t.Fatal("eligió sin variantes publicadas")
	}
	if !strings.Contains(err.Error(), "no tiene imagen publicada") {
		t.Errorf("mensaje inesperado: %v", err)
	}
}

// TestTagForEligeElTagDeLaVariante: el tag sale del catalogo, no de lo
// que dice el manifiesto.
func TestTagForEligeElTagDeLaVariante(t *testing.T) {
	svc := Service{ID: "argos", Variants: []string{"cpu", "cuda"}}
	tags := []string{"buildcache-cpu", "cpu-latest", "cuda-latest", "cpu-abc1234"}
	if got := TagFor(svc, "cuda", tags); got != "cuda-latest" {
		t.Errorf("TagFor(cuda) = %q, quería cuda-latest", got)
	}
	if got := TagFor(svc, "cpu", tags); got != "cpu-latest" {
		t.Errorf("TagFor(cpu) = %q, quería cpu-latest", got)
	}
}

// TestTagPorDefectoTomaElTagDeVersion: la variante "default" es la
// imagen sin sufijo de variante. Se salta los tags de arm64, los de
// commit (sha-) y el "latest" sin version.
//
// Ojo: esta rama no excluye los tags "buildcache-*", a diferencia de
// AvailableVariants. No muerde hoy porque ningun servicio con
// variante "default" publica buildcache (seanime solo tiene tags de
// version), pero queda anotado en el README.
func TestTagPorDefectoTomaElTagDeVersion(t *testing.T) {
	svc := Service{ID: "seanime", Variants: []string{"default"}}
	tags := []string{"3.10.4", "arm64-3.10.4", "arm64-sha-d693", "latest", "sha-d558"}
	if got := TagFor(svc, "default", tags); got != "3.10.4" {
		t.Errorf("TagFor(default) = %q, quería 3.10.4", got)
	}
	// Sin tag de version util, cae en "latest".
	if got := TagFor(svc, "default", []string{"arm64-3.10.4", "latest", "sha-d558"}); got != "latest" {
		t.Errorf("TagFor(default) = %q, quería latest", got)
	}
}
