package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// TestVerifyHealthRechazaUn500: un status que no es 2xx no es un
// servicio sano.
//
// Antes el health check miraba cualquier respuesta como buena: un 500
// quedaba "healthy" y hasta se copiaba el provider del cuerpo del
// error al estado.
func TestVerifyHealthRechazaUn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","provider":"CUDA"}`))
	}))
	t.Cleanup(srv.Close)

	if _, err := HTTPGet(srv.URL, 2*time.Second); err == nil {
		t.Error("HTTPGet aceptó un 500 como respuesta buena")
	}

	state := &State{ID: "argos"}
	svc := Service{ID: "argos", Health: srv.URL}
	if err := verifyHealth(state, svc, 100*time.Millisecond); err == nil {
		t.Fatal("un 500 tiene que fallar el health check")
	}
	if state.Health == "healthy" {
		t.Errorf("Health = %q, un 500 no es healthy", state.Health)
	}
	if state.Provider != "" {
		t.Errorf("el provider salió del cuerpo de un error: %q", state.Provider)
	}
}

// TestUpdateGuardaElDigestAnteriorParaElRollback: el update tiene que
// dejar guardado el digest anterior.
//
// Los tags son mutables: sin el digest previo, el rollback vuelve a
// la imagen que se acaba de instalar y no vuelve atrás.
func TestUpdateGuardaElDigestAnteriorParaElRollback(t *testing.T) {
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
		Digest: host + "/repo@sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}

	out, err := in.Update(svc, state, true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if out.PreviousDigest == "" {
		t.Fatal("el update no guardó el digest anterior")
	}
	if out.PreviousDigest == NormalizeDigest(out.Digest) {
		t.Errorf("PreviousDigest = %q es el nuevo: no sirve para volver atrás", out.PreviousDigest)
	}

	// Y el rollback vuelve a esa imagen, no a la que se acaba de instalar.
	// El digest anterior se captura ANTES: Rollback muta el estado que
	// recibe (PreviousDigest pasa a ser el que se deja atrás).
	anterior := out.PreviousDigest
	rb, err := in.Rollback(svc, out)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if NormalizeDigest(rb.Digest) != anterior {
		t.Errorf("Digest = %q, quería el anterior %q", rb.Digest, anterior)
	}
	var porDigest bool
	for _, c := range stub.calls(t) {
		if strings.Contains(c, "pull") && strings.Contains(c, "@"+anterior) {
			porDigest = true
		}
	}
	if !porDigest {
		t.Errorf("el rollback no bajó la imagen por digest: %v", stub.calls(t))
	}
}

// TestInstallFallidoGuardaElEstadoEnDisco: si el health no responde,
// el stack queda arriba y el estado tiene que estar en disco.
//
// Sin estado, el servicio corre y el CLI no lo sabe: no aparece en el
// listado y el install siguiente choca con los puertos ocupados.
func TestInstallFallidoGuardaElEstadoEnDisco(t *testing.T) {
	stub := newDockerStub(t)
	reg, host := fakeRegistry(t, []string{"cpu-latest"}, "sha256:1111111111111111111111111111111111111111111111111111111111111111")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	stateDir := t.TempDir()
	in := &Installer{
		Docker:        NewDocker(),
		Registry:      reg,
		StateDir:      stateDir,
		RepoDir:       func(Service) string { return t.TempDir() },
		HealthTimeout: 200 * time.Millisecond,
	}
	svc := Service{ID: "argos", Image: host + "/repo", Variants: []string{"cpu"}, Health: srv.URL}

	if _, err := in.Install(svc, "cpu", false); err == nil {
		t.Fatal("un install sin health sano tiene que fallar")
	}
	if len(stub.callsContaining(t, "up -d")) == 0 {
		t.Error("no levantó el stack")
	}
	saved, err := LoadState(stateDir, "argos")
	if err != nil {
		t.Fatalf("no guardó el estado del servicio que quedó arriba: %v", err)
	}
	if saved.Health == "healthy" {
		t.Errorf("Health = %q, el health no respondió", saved.Health)
	}
}

// TestUpdatePasaElTagResueltoAlCompose: el env del compose lleva el tag
// resuelto, no el del estado.
//
// Con el estado sin tag, el compose recibía un env sin IMAGE y levantaba
// su default: la imagen correcta estaba descargada y corría otra.
func TestUpdatePasaElTagResueltoAlCompose(t *testing.T) {
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
	// Un estado viejo: sin tag.
	state := &State{
		ID: "argos", Variant: "cpu",
		Image:  host + "/repo:cpu-latest",
		Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}

	_, err := in.Update(svc, state, true, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	ups := stub.callsContaining(t, "up -d")
	if len(ups) != 1 {
		t.Fatalf("ups = %v, quería 1", ups)
	}
	if !strings.Contains(ups[0], "image=cpu-latest") {
		t.Errorf("el compose no recibió el tag resuelto: %s", ups[0])
	}
	saved, err := LoadState(stateDir, "argos")
	if err != nil {
		t.Fatalf("no guardó el estado: %v", err)
	}
	if saved.Tag != "cpu-latest" {
		t.Errorf("el estado no guardó el tag resuelto: %q", saved.Tag)
	}
	if saved.Image != host+"/repo:cpu-latest" {
		t.Errorf("el estado no guardó la imagen resuelta: %q", saved.Image)
	}
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
