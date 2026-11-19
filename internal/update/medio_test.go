package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/registry"
	"github.com/Below-Z3R0/b3l0w-cli/internal/releases"
	"github.com/Below-Z3R0/b3l0w-cli/internal/versioncheck"
)

// Regresiones de los hallazgos MEDIO (M1-M6) de internal/update.
//
// Cada test falla si se revierte el arreglo: no mira que el código
// "no se rompa", mira que el síntoma que se reportó vuelva a pasar.
//
// Los fakes de red y de disco entran por las variables inyectables
// (aurLookup, releaseCheck, pluginSources) y por fakes en el PATH: la
// máquina del runner no se toca, y ningún test sale a la red.

// repoConUnArchivo arma un repo de dotfiles con un archivo y devuelve
// su path.
func repoConUnArchivo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app", "config.toml"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// cacheDePlugins deja una copia local del manifiesto de plugins: con
// ella el registry no baja nada de la red.
func cacheDePlugins(t *testing.T) {
	t.Helper()

	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	cache := registry.CachePath(registry.Plugins)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, plugins.Embedded(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestM1_CapasPedidasSelfNoArrastraLasDemas es el hallazgo M1: `--self`
// no contaba como selector, así que el prompt prometía el sistema, los
// programas, los plugins, los servicios y los dotfiles cuando en
// realidad solo se actualizaba el CLI.
//
// El prompt es lo que el usuario ve: si no lo arregla, el test falla.
func TestM1_CapasPedidasSelfNoArrastraLasDemas(t *testing.T) {
	c := capasPedidas(Options{Self: true})
	if !c.self {
		t.Error("con --self se quiere la capa del CLI")
	}
	if c.system || c.programs || c.plugins || c.services || c.dotfiles {
		t.Errorf("--self arrastró las demás capas: %+v", c)
	}

	prompt := confirmPrompt(c.self, c.system, c.programs, c.plugins, c.services, c.dotfiles)
	if want := "Update b3l0w? [Y/n] "; prompt != want {
		t.Errorf("prompt = %q, quería %q", prompt, want)
	}

	// `--self --system` quiere las dos, y solo las dos.
	dos := capasPedidas(Options{Self: true, System: true})
	if !dos.self || !dos.system {
		t.Errorf("--self --system = %+v, quería self y system", dos)
	}
	if dos.programs || dos.plugins || dos.services || dos.dotfiles {
		t.Errorf("--self --system arrastró otras capas: %+v", dos)
	}

	// Sin ningún flag se hacen todas.
	todas := capasPedidas(Options{})
	if !todas.self || !todas.system || !todas.programs || !todas.plugins ||
		!todas.services || !todas.dotfiles {
		t.Errorf("sin flags = %+v, quería todas las capas", todas)
	}

	// `--self` a secas corta después del CLI; con otro flag, no.
	if !soloSelf(Options{Self: true}) {
		t.Error("soloSelf(--self) = false, quería true")
	}
	if soloSelf(Options{Self: true, System: true}) {
		t.Error("soloSelf(--self --system) = true, quería false")
	}
	if soloSelf(Options{System: true}) {
		t.Error("soloSelf(--system) = true, quería false")
	}
}

// TestM2_ElPreviewQueFallaAbortaAntesDelPull es el hallazgo M2: el
// error del preview se descartaba y el módulo seguía al pull real, sin
// la confirmación que el preview iba a pedir.
//
// Con el estado corrupto el preview no se puede hacer: el update tiene
// que abortar diciendo que no se pudo previsualizar.
func TestM2_ElPreviewQueFallaAbortaAntesDelPull(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := repoConUnArchivo(t)

	// El estado está corrupto: el preview falla.
	estado := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	if err := os.MkdirAll(filepath.Dir(estado), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(estado, []byte("{esto no es json"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fakeBin(t, dir, "snapper", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	var code int
	_, errOut := capturarSalida(t, func() {
		code = updateDotfiles(Options{DryRun: true, DotfilesDir: repo})
	})
	if code != 1 {
		t.Errorf("updateDotfiles() con el estado corrupto = %d, quería 1", code)
	}
	if !strings.Contains(errOut, "previsualizar") {
		t.Errorf("no avisó que no se pudo previsualizar: %q", errOut)
	}
}

// TestM3_ElTOMLDeUmbrielSeValida es el hallazgo M3: solo se validaba el
// TOML de Noctalia, aunque el final del update le pide al usuario que
// valide los dos.
//
// Un TOML de Umbriel roto tiene que abortar antes de aplicar nada.
func TestM3_ElTOMLDeUmbrielSeValida(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "umbriel"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "umbriel", "config.toml"), []byte("roto = [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El validador de Umbriel está y dice que el TOML no valida.
	dir := t.TempDir()
	fakeBin(t, dir, "umbriel", "#!/bin/sh\nprintf 'toml invalido\\n' >&2\nexit 1\n")
	t.Setenv("PATH", dir)

	var code int
	_, errOut := capturarSalida(t, func() {
		code = updateDotfiles(Options{DryRun: true, DotfilesDir: repo})
	})
	if code != 1 {
		t.Errorf("updateDotfiles() con el TOML de umbriel inválido = %d, quería 1", code)
	}
	if !strings.Contains(errOut, "does not validate") {
		t.Errorf("no reportó el TOML inválido: %q", errOut)
	}
}

// TestM4_ElEstadoSeLeeDelHome es el hallazgo M4: el estado de dotfiles
// se deriva del home del usuario.
//
// El comportamiento ya era correcto en Linux (`os.UserHomeDir()` es
// `$HOME`): este test lo fija, para que nadie lo rompa al tocar el
// código.
func TestM4_ElEstadoSeLeeDelHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := repoConUnArchivo(t)
	local := filepath.Join(home, ".config", "app", "config.toml")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("local = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fakeBin(t, dir, "snapper", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	capturarSalida(t, func() {
		if code := updateDotfiles(Options{Adopt: true, DotfilesDir: repo}); code != 0 {
			t.Errorf("updateDotfiles(--adopt) = %d, quería 0", code)
		}
	})

	// El estado queda en el home nuevo, no en otro lado.
	estado := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	if _, err := os.Stat(estado); err != nil {
		t.Errorf("el estado no se escribió en $HOME: %v", err)
	}
}

// TestM4_SinHomeElErrorDiceQueFalta es el otro lado del M4: sin home no
// hay estado que leer, y el error tiene que decir qué variable falta.
//
// Antes el error era el crudo de Go ("neither $HOME nor the user
// database are defined"), que no dice qué hacer.
func TestM4_SinHomeElErrorDiceQueFalta(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("PATH", t.TempDir())

	repo := repoConUnArchivo(t)

	var code int
	_, errOut := capturarSalida(t, func() {
		code = updateDotfiles(Options{DryRun: true, DotfilesDir: repo})
	})
	if code != 1 {
		t.Errorf("updateDotfiles() sin HOME = %d, quería 1", code)
	}
	if !strings.Contains(errOut, "revisá $HOME") {
		t.Errorf("el error no dice qué variable falta: %q", errOut)
	}
}

// TestM5_UnSourceQueNoSeLeeEsUnError es el hallazgo M5: si Noctalia
// está pero los sources no se pueden leer, el módulo caía al camino
// alternativo y actualizaba OTRA lista de plugins, en silencio.
//
// Un source roto es un error: el update tiene que abortar.
func TestM5_UnSourceQueNoSeLeeEsUnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDePlugins(t)

	// Noctalia está, pero los sources no se pueden leer.
	dir := t.TempDir()
	fakeBin(t, dir, "noctalia", "#!/bin/sh\nprintf 'boom\\n' >&2\nexit 1\n")
	t.Setenv("PATH", dir)

	orig := pluginSources
	t.Cleanup(func() { pluginSources = orig })
	pluginSources = func() ([]string, error) {
		return nil, errors.New("no se pudo leer")
	}

	var code int
	salida, errOut := capturarSalida(t, func() {
		code = updatePlugins()
	})
	if code != 1 {
		t.Errorf("updatePlugins() con los sources rotos = %d, quería 1", code)
	}
	if !strings.Contains(salida+errOut, "sources") {
		t.Errorf("no reportó el error de los sources: %q %q", salida, errOut)
	}
}

// TestM6_SinAurElCheckFalla es el hallazgo M6: sin red, el check avisaba
// del escritorio y salía con 0, diciendo que todo estaba al día.
//
// El mensaje ya era correcto: lo que mentía era el código de salida.
func TestM6_SinAurElCheckFalla(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheDePlugins(t)

	// Las releases tampoco: el check no baja nada de la red.
	origCheck := releaseCheck
	t.Cleanup(func() { releaseCheck = origCheck })
	releaseCheck = func(m *config.Manifest) []releases.Status { return nil }

	// El AUR no contesta.
	origAUR := aurLookup
	t.Cleanup(func() { aurLookup = origAUR })
	aurLookup = func(names []string) ([]versioncheck.Package, error) {
		return nil, errors.New("sin red")
	}

	// pacman y vercmp falsos: el check no sale al sistema.
	dir := t.TempDir()
	fakeBin(t, dir, "pacman", "#!/bin/sh\nprintf 'x 1.0.0-1\\n'\n")
	fakeBin(t, dir, "vercmp", "#!/bin/sh\nprintf '0'\n")
	t.Setenv("PATH", dir)

	var code int
	capturarSalida(t, func() {
		code = check(&config.Manifest{})
	})
	if code != 1 {
		t.Errorf("check() sin AUR = %d, quería 1", code)
	}
}
