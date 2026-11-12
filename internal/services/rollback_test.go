package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dockerStub es un binario "docker" falso que se pone delante del PATH.
//
// El DockerAdapter no tiene Runner inyectable: llama a "docker" por el
// PATH. Este stub intercepta esas llamadas, asi que el test nunca
// toca el Docker real: cada comando queda registrado en un log y el
// stub decide que responder.
type dockerStub struct {
	logPath string
}

func newDockerStub(t *testing.T) *dockerStub {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
printf 'args=%s image=%s\n' "$*" "${IMAGE:-}" >> "$STUB_LOG"
if [ -n "$STUB_FAIL" ]; then
  case "$*" in
    *"$STUB_FAIL"*) echo "stub: fallo simulado" >&2; exit 1 ;;
  esac
fi
case "$1" in
  ps)
    if [ -n "$STUB_PS_OUT" ]; then printf '%s\n' "$STUB_PS_OUT"; fi
    ;;
  image)
    if [ "$2" = "inspect" ]; then
      if [ -n "$STUB_INSPECT_OUT" ]; then printf '%s\n' "$STUB_INSPECT_OUT"; fi
    fi
    ;;
esac
exit 0
`
	stubPath := filepath.Join(dir, "docker")
	if err := os.WriteFile(stubPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("STUB_LOG", logPath)
	return &dockerStub{logPath: logPath}
}

// calls devuelve las invocaciones registradas, en orden.
func (s *dockerStub) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(s.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// callsContaining devuelve las invocaciones que matchean el prefijo.
func (s *dockerStub) callsContaining(t *testing.T, want string) []string {
	t.Helper()
	var out []string
	for _, c := range s.calls(t) {
		if strings.Contains(c, want) {
			out = append(out, c)
		}
	}
	return out
}

// TestNormalizeDigest deja solo el sha256 del digest.
//
// Del lado local el digest viene como RepoDigest
// ("ghcr.io/org/img@sha256:...") y del remoto como header
// ("sha256:..."). Sin normalizar, los dos nunca son iguales y el
// update ve un digest nuevo en cada corrida.
func TestNormalizeDigest(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/org/img@sha256:abc":     "sha256:abc",
		"sha256:abc":                     "sha256:abc",
		"  sha256:abc  ":                 "sha256:abc",
		"sha256:abc\n":                   "sha256:abc",
		"ghcr.io/org/img:tag@sha256:abc": "sha256:abc",
		"@sha256:abc":                    "sha256:abc",
		"":                               "",
	}
	for in, want := range cases {
		if got := NormalizeDigest(in); got != want {
			t.Errorf("NormalizeDigest(%q) = %q, quería %q", in, got, want)
		}
	}
}

// TestNormalizeDigestComparaLocalYRemoto es la propiedad que importa:
// el mismo digest, escrito de las dos formas, da el mismo string.
func TestNormalizeDigestComparaLocalYRemoto(t *testing.T) {
	local := "ghcr.io/below-z3r0/argos@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	remote := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	if NormalizeDigest(local) != NormalizeDigest(remote) {
		t.Fatalf("local %q y remoto %q no normalizan igual", NormalizeDigest(local), NormalizeDigest(remote))
	}
	// Un digest distinto tiene que seguir siendo distinto.
	otro := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	if NormalizeDigest(local) == NormalizeDigest(otro) {
		t.Fatal("dos digests distintos normalizan igual")
	}
}

// TestRollbackSinEstadoNoHaceNada: sin estado no hay a que volver.
func TestRollbackSinEstadoNoHaceNada(t *testing.T) {
	stub := newDockerStub(t)
	in := &Installer{Docker: NewDocker(), StateDir: t.TempDir()}
	_, err := in.Rollback(Service{ID: "argos"}, nil)
	if err == nil {
		t.Fatal("quiso rollback sin estado")
	}
	if !strings.Contains(err.Error(), "no está instalado") {
		t.Errorf("mensaje inesperado: %v", err)
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("llamó a docker sin estado: %v", calls)
	}
}

// TestRollbackSinImagenAnteriorNoHaceNada: sin tag previo guardado no
// hay imagen a la que volver.
func TestRollbackSinImagenAnteriorNoHaceNada(t *testing.T) {
	stub := newDockerStub(t)
	in := &Installer{Docker: NewDocker(), StateDir: t.TempDir()}
	state := &State{ID: "argos", Image: "ghcr.io/org/img:new", Tag: "new"}
	_, err := in.Rollback(Service{ID: "argos"}, state)
	if err == nil {
		t.Fatal("quiso rollback sin imagen anterior")
	}
	if !strings.Contains(err.Error(), "no tiene imagen anterior") {
		t.Errorf("mensaje inesperado: %v", err)
	}
	if calls := stub.calls(t); len(calls) != 0 {
		t.Errorf("llamó a docker sin imagen anterior: %v", calls)
	}
}

// TestRollbackVuelveALaImagenAnterior es el ciclo completo del
// rollback: pull del tag anterior, recreate con ese tag, estado
// guardado y la imagen que se dejo de usar borrada.
func TestRollbackVuelveALaImagenAnterior(t *testing.T) {
	stub := newDockerStub(t)
	t.Setenv("STUB_INSPECT_OUT", "ghcr.io/org/img@sha256:old")

	stateDir := t.TempDir()
	repoDir := t.TempDir()
	in := &Installer{
		Docker:   NewDocker(),
		StateDir: stateDir,
		RepoDir:  func(Service) string { return repoDir },
	}
	svc := Service{ID: "argos", Image: "ghcr.io/org/img"}
	state := &State{
		ID:          "argos",
		Variant:     "cuda",
		Image:       "ghcr.io/org/img:new",
		Tag:         "new",
		Digest:      "ghcr.io/org/img@sha256:new",
		Previous:    "ghcr.io/org/img:old",
		PreviousTag: "old",
	}

	out, err := in.Rollback(svc, state)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// La imagen activa pasa a ser la anterior.
	if out.Image != "ghcr.io/org/img:old" {
		t.Errorf("Image = %q, quería la anterior", out.Image)
	}
	if out.Tag != "old" {
		t.Errorf("Tag = %q, quería old", out.Tag)
	}
	// Y la que se dejo de usar queda guardada para volver adelante.
	if out.Previous != "ghcr.io/org/img:new" {
		t.Errorf("Previous = %q, quería la nueva", out.Previous)
	}
	if out.PreviousTag != "new" {
		t.Errorf("PreviousTag = %q, quería new", out.PreviousTag)
	}
	if out.Digest != "ghcr.io/org/img@sha256:old" {
		t.Errorf("Digest = %q, quería el de la imagen anterior", out.Digest)
	}
	if out.UpdatedAt == "" {
		t.Error("no se marco UpdatedAt")
	}

	// El estado queda en disco.
	saved, err := LoadState(stateDir, "argos")
	if err != nil {
		t.Fatalf("no guardó el estado: %v", err)
	}
	if saved.Image != "ghcr.io/org/img:old" || saved.Tag != "old" {
		t.Errorf("estado en disco = %+v", saved)
	}
	if saved.PreviousTag != "new" {
		t.Errorf("estado en disco perdio el tag anterior: %+v", saved)
	}

	// Y el pull fue del tag anterior, no del actual.
	pulls := stub.callsContaining(t, "pull")
	if len(pulls) != 1 {
		t.Fatalf("pulls = %v, quería 1", pulls)
	}
	if !strings.Contains(pulls[0], "ghcr.io/org/img:old") {
		t.Errorf("pull de la imagen equivocada: %s", pulls[0])
	}

	// El compose se levanta con el tag anterior en el env.
	ups := stub.callsContaining(t, "up -d")
	if len(ups) != 1 {
		t.Fatalf("ups = %v, quería 1", ups)
	}
	if !strings.Contains(ups[0], "image=old") {
		t.Errorf("el compose no recibio el tag anterior: %s", ups[0])
	}

	// La imagen que quedo atras se borra.
	if rms := stub.callsContaining(t, "image rm"); len(rms) != 1 {
		t.Errorf("no borro la imagen anterior: %v", rms)
	} else if !strings.Contains(rms[0], "ghcr.io/org/img:new") {
		t.Errorf("boro la imagen equivocada: %s", rms[0])
	}
}

// TestRollbackFallaSiElPullFalla: sin la imagen anterior no hay
// rollback, y el estado queda como estaba.
func TestRollbackFallaSiElPullFalla(t *testing.T) {
	stub := newDockerStub(t)
	t.Setenv("STUB_FAIL", "pull")

	stateDir := t.TempDir()
	in := &Installer{
		Docker:   NewDocker(),
		StateDir: stateDir,
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: "ghcr.io/org/img"}
	state := &State{
		ID: "argos", Image: "ghcr.io/org/img:new", Tag: "new",
		Previous: "ghcr.io/org/img:old", PreviousTag: "old",
	}

	_, err := in.Rollback(svc, state)
	if err == nil {
		t.Fatal("rollback sin pull deberia fallar")
	}
	if !strings.Contains(err.Error(), "pull:") {
		t.Errorf("mensaje inesperado: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "argos.json")); err == nil {
		t.Error("guardo el estado de un rollback fallido")
	}
	if ups := stub.callsContaining(t, "up -d"); len(ups) != 0 {
		t.Errorf("levanto el stack sin imagen: %v", ups)
	}
}

// TestRollbackFallaSiElUpFalla: si el stack no levanta con la imagen
// anterior, el rollback avisa y no guarda el estado.
func TestRollbackFallaSiElUpFalla(t *testing.T) {
	newDockerStub(t)
	t.Setenv("STUB_FAIL", "up -d")

	stateDir := t.TempDir()
	in := &Installer{
		Docker:   NewDocker(),
		StateDir: stateDir,
		RepoDir:  func(Service) string { return t.TempDir() },
	}
	svc := Service{ID: "argos", Image: "ghcr.io/org/img"}
	state := &State{
		ID: "argos", Image: "ghcr.io/org/img:new", Tag: "new",
		Previous: "ghcr.io/org/img:old", PreviousTag: "old",
	}

	_, err := in.Rollback(svc, state)
	if err == nil {
		t.Fatal("rollback sin up deberia fallar")
	}
	if !strings.Contains(err.Error(), "up:") {
		t.Errorf("mensaje inesperado: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "argos.json")); err == nil {
		t.Error("guardo el estado de un rollback fallido")
	}
}

// TestImageInUseMiroLosContainers: la imagen que usa un container no
// se puede borrar.
func TestImageInUseMiroLosContainers(t *testing.T) {
	stub := newDockerStub(t)
	t.Setenv("STUB_PS_OUT", "ghcr.io/org/img:new")

	if !ImageInUse("ghcr.io/org/img:new") {
		t.Error("no vio la imagen en uso")
	}
	if ImageInUse("ghcr.io/org/img:otra") {
		t.Error("vio una imagen que no esta en uso")
	}
	if calls := stub.callsContaining(t, "ps"); len(calls) != 2 {
		t.Errorf("llamó a docker ps %d veces, quería 2", len(calls))
	}
}
