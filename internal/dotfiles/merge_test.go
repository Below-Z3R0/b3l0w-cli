package dotfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escenario arma un repo, un home y un estado con los contenidos dados.
func escenario(t *testing.T, base, local, remoto string) (src, home, state string) {
	t.Helper()
	dir := t.TempDir()
	src = filepath.Join(dir, "repo")
	home = filepath.Join(dir, "home")
	state = filepath.Join(dir, "estado.json")

	os.MkdirAll(filepath.Join(src, "noctalia"), 0o755)
	os.MkdirAll(filepath.Join(home, ".config", "noctalia"), 0o755)

	os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte(remoto), 0o644)
	os.WriteFile(filepath.Join(home, ".config", "noctalia", "config.toml"), []byte(local), 0o644)

	sum := sha256.Sum256([]byte(base))
	data, _ := json.Marshal(stateFile{"noctalia/config.toml": hex.EncodeToString(sum[:])})
	os.WriteFile(state, data, 0o644)
	return
}

// stateFile replica el formato del estado en disco.
type stateFile map[string]string

func leer(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	return string(data)
}

// TestPullActualizaCuandoNoTocaste: el usuario no editó, el repo
// renombró una variable. Se aplica el nuevo sin preguntar.
func TestPullActualizaCuandoNoTocaste(t *testing.T) {
	base := "[bar]\nactivo = true\n"
	remoto := "[baz]\nactivo = true\n" // Noctalia renombró bar -> baz

	src, home, state := escenario(t, base, base, remoto)

	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Updated) != 1 {
		t.Errorf("Updated = %v, quería 1 archivo", res.Updated)
	}
	got := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml"))
	if !strings.Contains(got, "baz") {
		t.Errorf("no se aplicó el renombre:\n%s", got)
	}
}

// TestPullConflictoDejaNew: el usuario editó y el repo también. No se
// pisa nada; se escribe .new para que el usuario mergee.
func TestPullConflictoDejaNew(t *testing.T) {
	base := "[bar]\nactivo = true\n"
	local := "[bar]\nactivo = false\n" // el usuario cambió el valor
	remoto := "[baz]\nactivo = true\n"

	src, home, state := escenario(t, base, local, remoto)

	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("Conflicts = %v, quería 1", res.Conflicts)
	}

	// El local se conserva intacto.
	got := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml"))
	if got != local {
		t.Errorf("el local se pisó:\n%s", got)
	}
	// Y queda el .new con la versión del repo.
	nuevo := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml.new"))
	if !strings.Contains(nuevo, "baz") {
		t.Errorf(".new no tiene la versión nueva:\n%s", nuevo)
	}
}

// TestPullConservaCuandoSoloCambioLocal: el repo no cambió, el usuario
// sí. No se toca.
func TestPullConservaCuandoSoloCambioLocal(t *testing.T) {
	base := "[bar]\nactivo = true\n"
	local := "[bar]\nactivo = false\n"

	src, home, state := escenario(t, base, local, base)

	res, err := Pull(src, home, state, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Kept) != 1 {
		t.Errorf("Kept = %v, quería 1", res.Kept)
	}
	if got := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml")); got != local {
		t.Errorf("se pisó el cambio local:\n%s", got)
	}
}

// TestPullDryRunNoEscribe: --dry-run informa pero no toca nada.
func TestPullDryRunNoEscribe(t *testing.T) {
	base := "[bar]\nactivo = true\n"
	remoto := "[baz]\nactivo = true\n"

	src, home, state := escenario(t, base, base, remoto)

	if _, err := Pull(src, home, state, false, true); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml")); got != base {
		t.Errorf("dry-run escribió:\n%s", got)
	}
}

// TestPullForcePisaTodo: --force no hace 3-way.
func TestPullForcePisaTodo(t *testing.T) {
	base := "[bar]\nactivo = true\n"
	local := "[bar]\nactivo = false\n"
	remoto := "[baz]\nactivo = true\n"

	src, home, state := escenario(t, base, local, remoto)

	if _, err := Pull(src, home, state, true, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got := leer(t, filepath.Join(home, ".config", "noctalia", "config.toml")); !strings.Contains(got, "baz") {
		t.Errorf("--force no aplicó el remoto:\n%s", got)
	}
}
