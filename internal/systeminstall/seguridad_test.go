package systeminstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El destino de un dotfile viene del perfil, que puede ser una
// definicion remota. Si el destino fuera "~" o "/", el RemoveAll
// borraba el home entero.
func TestBug_DestinoPeligrosoSeRechaza(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sin home")
	}

	casos := []struct {
		name string
		dest string
	}{
		{"home pelado", home},
		{"raiz", "/"},
		{"vacio", ""},
		{"fuera del home", "/etc/passwd"},
		{"home con barra", home + "/"},
	}

	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			if err := destinoSeguro(c.dest); err == nil {
				t.Fatalf("destinoSeguro(%q) = nil, quería error", c.dest)
			}
		})
	}
}

// Un destino legitimo dentro del home tiene que pasar.
func TestBug_DestinoDentroDelHomeSeAcepta(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sin home")
	}
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	if err := destinoSeguro(dest); err != nil {
		t.Fatalf("destinoSeguro(%q) = %v, quería nil", dest, err)
	}
}

// Apply no tiene que pisar un dotfile que ya esta: el plan solo trae
// lo que falta, y antes se pisaba igual (con backup y todo).
func TestBug_ApplyNoPisaLoQueYaEsta(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.conf")
	dest := filepath.Join(dir, "dest.conf")
	if err := os.WriteFile(src, []byte("del repo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("del usuario"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := newFake()
	plan := &Plan{Files: []FileStatus{{Src: src, Dest: dest, SrcExists: true, DestExists: true}}}

	if err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}}); err != nil {
		t.Fatal(err)
	}

	if len(fake.calls) != 0 {
		t.Errorf("no se deberia copiar nada: %v", fake.calls)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "del usuario" {
		t.Errorf("el destino quedo %q, quería el del usuario intacto", got)
	}
	// Y no hay backups acumulados.
	backups, _ := filepath.Glob(dest + ".bak*")
	if len(backups) != 0 {
		t.Errorf("backups = %v, quería 0", backups)
	}
}

// El error de destino peligroso tiene que nombrar el motivo.
func TestBug_ElErrorDeDestinoPeligrosoLoDice(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sin home")
	}
	err = destinoSeguro(home)
	if err == nil {
		t.Fatal("destinoSeguro deberia fallar")
	}
	if !strings.Contains(err.Error(), "peligroso") && !strings.Contains(err.Error(), "fuera del home") {
		t.Errorf("el error no explica el motivo: %v", err)
	}
}
