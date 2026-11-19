package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/registry"
	"github.com/Below-Z3R0/b3l0w-cli/internal/services"
	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
	"github.com/Below-Z3R0/b3l0w-cli/internal/versioncheck"
)

// Regresiones de los hallazgos ALTO (A1-A4) de internal/update.
//
// Cada test falla si se revierte el arreglo: no mira que el código
// "no se rompa", mira que el síntoma que se reportó vuelva a pasar.
//
// Los fakes de red y de Docker entran por las variables inyectables
// (selfCheck, aurLookup, registrySync, updateService): la máquina del
// runner no se toca.

func repoConNoctalia(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "noctalia", "config.toml"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta es el hallazgo A1:
// sin el binario de noctalia instalado, el update de dotfiles abortaba
// con código 1 y decía "repo TOML does not validate:", que era falso:
// el TOML podía estar perfecto, lo que faltaba era el validador.
func TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta(t *testing.T) {
	repo := repoConNoctalia(t)
	t.Setenv("HOME", t.TempDir())
	// PATH sin noctalia: el validador no está instalado.
	t.Setenv("PATH", t.TempDir())

	salida, errOut := capturarSalida(t, func() {
		if code := updateDotfiles(Options{DryRun: true, DotfilesDir: repo}); code != 0 {
			t.Errorf("updateDotfiles() sin noctalia = %d, quería 0", code)
		}
	})
	if strings.Contains(errOut, "does not validate") {
		t.Errorf("abortó por una validación que no se pudo hacer: %q", errOut)
	}
	if !strings.Contains(salida, "noctalia no está instalado") {
		t.Errorf("no avisó que el TOML no se validó: %q", salida)
	}
}

// TestBug_A1_ConNoctaliaElTOMLInvalidoSigueAbortando es el contraste
// del A1: si el validador está y el TOML no valida, el update tiene
// que abortar con 1. El arreglo del A1 no puede ser "no validar nunca".
func TestBug_A1_ConNoctaliaElTOMLInvalidoSigueAbortando(t *testing.T) {
	repo := repoConNoctalia(t)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	fakeBin(t, dir, "noctalia", "#!/bin/sh\nprintf 'toml invalido\\n' >&2\nexit 1\n")
	t.Setenv("PATH", dir)

	_, errOut := capturarSalida(t, func() {
		if code := updateDotfiles(Options{DryRun: true, DotfilesDir: repo}); code != 1 {
			t.Errorf("updateDotfiles() con TOML inválido = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "repo TOML does not validate") {
		t.Errorf("no reportó el TOML inválido: %q", errOut)
	}
}

// TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida es el hallazgo
// A2: updateServices imprimía el error de cada servicio y devolvía 0
// siempre, así que un update donde los tres servicios fallan salía
// con "update complete" y código 0.
func TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	// El manifiesto de servicios, del caché local del registry: el
	// test no baja nada de la red.
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	cache := registry.CachePath(registry.Services)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, services.Embedded(), 0o644); err != nil {
		t.Fatal(err)
	}

	// El estado de un servicio instalado (argos está en el manifiesto).
	dir := filepath.Join(home, ".local", "state", "b3l0w", "services")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	estado := `{"id":"argos","image":"ghcr.io/below-z3r0/argos","tag":"latest"}`
	if err := os.WriteFile(filepath.Join(dir, "argos.json"), []byte(estado), 0o644); err != nil {
		t.Fatal(err)
	}

	// El update del servicio falla (el registry de imágenes no está).
	orig := updateService
	t.Cleanup(func() { updateService = orig })
	updateService = func(in *services.Installer, svc services.Service, state *services.State, rebuild, yes bool) (*services.State, error) {
		return nil, errors.New("no pude leer el registry")
	}

	_, errOut := capturarSalida(t, func() {
		if code := updateServices(true); code != 1 {
			t.Errorf("updateServices() con un servicio que falla = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "argos") {
		t.Errorf("no nombró el servicio que falló: %q", errOut)
	}
}

// TestBug_A2_RunPropagaElFalloDeServicios es la otra mitad del A2: Run
// descartaba el código de updateServices, así que el comando entero
// salía con 0 aunque los servicios fallaran.
func TestBug_A2_RunPropagaElFalloDeServicios(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	cache := registry.CachePath(registry.Services)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, services.Embedded(), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(home, ".local", "state", "b3l0w", "services")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	estado := `{"id":"argos","image":"ghcr.io/below-z3r0/argos","tag":"latest"}`
	if err := os.WriteFile(filepath.Join(dir, "argos.json"), []byte(estado), 0o644); err != nil {
		t.Fatal(err)
	}

	origUpdate := updateService
	t.Cleanup(func() { updateService = origUpdate })
	updateService = func(in *services.Installer, svc services.Service, state *services.State, rebuild, yes bool) (*services.State, error) {
		return nil, errors.New("no pude leer el registry")
	}
	// El sync previo tampoco toca la red en el test.
	origSync := registrySync
	t.Cleanup(func() { registrySync = origSync })
	registrySync = func(kind registry.Kind, embedded []byte) (registry.Status, error) {
		return registry.Status{}, nil
	}

	capturarSalida(t, func() {
		if code := Run(&config.Manifest{}, Options{Yes: true, Services: true}); code != 1 {
			t.Errorf("Run(--yes --services) con un servicio que falla = %d, quería 1", code)
		}
	})
}

// TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal es el
// hallazgo A3: checkDesktop preguntaba siempre por los paquetes -git,
// aunque el canal fuera stable, y decía que los programas del
// escritorio no estaban instalados.
func TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal(t *testing.T) {
	cases := []struct {
		canal string
		want  []string
	}{
		{"stable", []string{"umbriel", "noctalia"}},
		{"edge", []string{"umbriel-git", "noctalia-git"}},
	}
	for _, c := range cases {
		home := t.TempDir()
		t.Setenv("HOME", home)
		canalDir := filepath.Join(home, ".config", "b3l0w")
		if err := os.MkdirAll(canalDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(canalDir, "channel"), []byte(c.canal+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Se captura qué paquetes se le piden al AUR.
		var pedidos []string
		orig := aurLookup
		t.Cleanup(func() { aurLookup = orig })
		aurLookup = func(names []string) ([]versioncheck.Package, error) {
			pedidos = append(pedidos, names...)
			out := make([]versioncheck.Package, 0, len(names))
			for _, n := range names {
				out = append(out, versioncheck.Package{Name: n, Version: "1.0.0-1"})
			}
			return out, nil
		}

		// pacman y vercmp falsos: el check no sale a la red ni al sistema.
		dir := t.TempDir()
		fakeBin(t, dir, "pacman", "#!/bin/sh\nprintf 'x 1.0.0-1\\n'\n")
		fakeBin(t, dir, "vercmp", "#!/bin/sh\nprintf '0'\n")
		t.Setenv("PATH", dir)

		capturarSalida(t, func() { checkDesktop() })

		if strings.Join(pedidos, ",") != strings.Join(c.want, ",") {
			t.Errorf("canal %s: consultó %v, quería %v", c.canal, pedidos, c.want)
		}
	}
}

// TestBug_A4_ElDryRunComparaLasVersionesComoNumeros es el hallazgo
// A4: el dry-run comparaba las versiones como strings, así que
// "v0.5.0" y "0.5.0" parecían distintas y el plan anunciaba una
// actualización a la versión que ya se tenía.
func TestBug_A4_ElDryRunComparaLasVersionesComoNumeros(t *testing.T) {
	cases := []struct {
		local, remota string
		wantActualiza bool
	}{
		{"0.5.0", "v0.5.0", false},     // misma versión con y sin "v"
		{"v0.5.0", "0.5.0", false},     // al revés
		{"0.4.0", "v0.5.0", true},      // hay release más nueva
		{"v0.4.0", "0.5.0", true},      // hay release más nueva
		{"0.5.0", "v0.4.0", false},     // el release es más viejo: no hay downgrade
		{"0.9.0", "v0.10.0", true},     // 0.10.0 > 0.9.0 (como string no)
		{"0.5.0", "v0.5.0-dev", false}, // el sufijo no cuenta
	}
	for _, c := range cases {
		origVersion := version.Version
		t.Cleanup(func() { version.Version = origVersion })
		version.Version = c.local

		origCheck := selfCheck
		t.Cleanup(func() { selfCheck = origCheck })
		selfCheck = func() (string, error) { return c.remota, nil }

		salida, _ := capturarSalida(t, func() {
			if code := dryRun(&config.Manifest{}, Options{Self: true}); code != 0 {
				t.Errorf("dryRun() = %d, quería 0", code)
			}
		})
		actualiza := strings.Contains(salida, "->")
		if actualiza != c.wantActualiza {
			t.Errorf("local %q remota %q: anuncia actualización = %t, quería %t (%q)",
				c.local, c.remota, actualiza, c.wantActualiza, salida)
		}
	}
}

// TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes es la otra mitad
// del A4: check comparaba igual, como strings, y avisaba de una
// actualización que no existía.
func TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes(t *testing.T) {
	origVersion := version.Version
	t.Cleanup(func() { version.Version = origVersion })
	version.Version = "0.5.0"

	origCheck := selfCheck
	t.Cleanup(func() { selfCheck = origCheck })
	selfCheck = func() (string, error) { return "v0.5.0", nil }

	salida, _ := capturarSalida(t, func() { avisoVersionCLI() })
	if strings.Contains(salida, "->") {
		t.Errorf("avisó de una actualización que ya se tenía: %q", salida)
	}

	// Con un release más nuevo, el aviso sí tiene que aparecer.
	selfCheck = func() (string, error) { return "v0.6.0", nil }
	salida, _ = capturarSalida(t, func() { avisoVersionCLI() })
	if !strings.Contains(salida, "0.5.0 -> v0.6.0") {
		t.Errorf("no avisó del release más nuevo: %q", salida)
	}
}
