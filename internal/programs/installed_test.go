package programs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// fakePacman instala un `pacman` falso que responde distinto según la
// consulta:
//
//	pacman -Qq <pkg>   → 0 si el pkg está en $FAKE_INSTALADOS, 1 si no
//	pacman -Si <pkg>   → 0 si el pkg está en $FAKE_DISPONIBLES, 1 si no
//
// El fake va primero en el PATH, así que el pacman real no se ejecuta.
func fakePacman(t *testing.T, instalados, disponibles []string) {
	t.Helper()

	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  -Qq)
    for p in $FAKE_INSTALADOS; do [ "$p" = "$2" ] && exit 0; done
    exit 1
    ;;
  -Si)
    for p in $FAKE_DISPONIBLES; do [ "$p" = "$2" ] && exit 0; done
    exit 1
    ;;
esac
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "pacman"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_INSTALADOS", strings.Join(instalados, " "))
	t.Setenv("FAKE_DISPONIBLES", strings.Join(disponibles, " "))
}

// TestInstalledNoConfundeDisponibleConInstalado es el bug A1: con
// `pacman -Si` cualquier paquete de cualquier repo daba true, así que
// Uninstall podía disparar pacman -Rns sobre un paquete de la distro
// que solo estaba disponible.
func TestInstalledNoConfundeDisponibleConInstalado(t *testing.T) {
	// bash está instalado; firefox está en un repo pero no instalado.
	fakePacman(t, []string{"bash"}, []string{"bash", "firefox"})

	if !Installed("bash") {
		t.Error("Installed(bash) = false, quería true: está instalado")
	}
	if Installed("firefox") {
		t.Error("Installed(firefox) = true, quería false: no está instalado")
	}

	// La otra pregunta sigue siendo la de los disponibles.
	if !Packaged("firefox") {
		t.Error("Packaged(firefox) = false, quería true: existe en un repo")
	}
	if Packaged("no-existe-xyz") {
		t.Error("Packaged(no-existe-xyz) = true, quería false")
	}
}

// TestUninstallNoBorraSiElManifiestoNoSeGuarda es el bug A2: el binario
// se borraba antes de guardar, así que un save fallido dejaba el
// programa listado y el archivo inexistente.
func TestUninstallNoBorraSiElManifiestoNoSeGuarda(t *testing.T) {
	// Un DIRECTORIO en la ruta del manifiesto: Save hace MkdirAll del
	// padre (que ya existe) y después WriteFile falla con "is a
	// directory". Es la forma de forzar el error sin depender de los
	// permisos del usuario que corre el test.
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if err := os.MkdirAll(filepath.Join(cfg, "b3l0w", "programs.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	fakePacman(t, nil, nil) // sin pacman: el camino del binario suelto

	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin", "prog")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := &config.Manifest{
		Version:  1,
		Programs: []config.Program{{Name: "prog", Source: "github:test/prog"}},
	}
	if _, err := Uninstall("prog", m); err == nil {
		t.Fatal("Uninstall devolvió nil con un manifiesto que no se puede guardar")
	}

	// Lo que importa: el binario sigue en su lugar.
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("el binario se borró aunque el manifiesto no se guardó: %v", err)
	}
	// Y la entrada se restauró en memoria.
	if m.Get("prog") == nil {
		t.Error("la entrada del manifiesto no se restauró tras el fallo")
	}
}
