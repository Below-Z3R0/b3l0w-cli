// Package update orquesta la actualización: primero el sistema, después
// los programas propios.
package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/channel"
	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/dotfiles"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/selfupdate"
	"github.com/Below-Z3R0/b3l0w-cli/internal/snapshot"
	"github.com/Below-Z3R0/b3l0w-cli/internal/system"
	"github.com/Below-Z3R0/b3l0w-cli/internal/versioncheck"
)

// Options son las banderas de `b3l0w update`.
type Options struct {
	System      bool
	Programs    bool
	Check       bool
	Self        bool
	Yes         bool
	Dotfiles    bool
	DryRun      bool
	Force       bool
	DotfilesDir string
	Desktop     bool
}

// Run ejecuta el update y devuelve el código de salida.
func Run(m *config.Manifest, opts Options) int {
	if opts.Check {
		return check(m)
	}

	// Dotfiles: no pisa cambios locales, así que no pide confirmación.
	if opts.Dotfiles {
		return updateDotfiles(opts)
	}

	// Escritorio: por ahora solo verifica y sugiere; no ejecuta pacman.
	if opts.Desktop {
		return desktopUpdate()
	}

	// Pase lo que pase, la terminal tiene que quedar usable: cachy-update
	// usa alternate screen y deja el cursor oculto.
	defer cli.RestoreTTY()

	// Sin banderas se hacen las dos mitades; con una sola, esa.
	systemWanted := opts.System || !opts.Programs
	programsWanted := opts.Programs || !opts.System

	if !opts.Yes {
		if !cli.Confirm(confirmPrompt(systemWanted, programsWanted), false, true) {
			cli.Warn("cancelled")
			return 0
		}
	}

	// El CLI primero: así el resto corre con la versión nueva.
	if opts.Self {
		if code := selfUpdate(); code != 0 {
			return code
		}
		// `update --self` a secas es solo eso.
		if !opts.System && !opts.Programs {
			return 0
		}
	}

	if systemWanted {
		if code := updateSystem(opts); code != 0 {
			return code
		}
	}
	if programsWanted {
		return updatePrograms(m)
	}
	return 0
}

func selfUpdate() int {
	cli.Step("Actualizando b3l0w")
	result, err := selfupdate.Run()
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if result.Changed {
		cli.Ok(fmt.Sprintf("b3l0w %s -> %s", result.Before, result.After))
	} else {
		cli.Ok(fmt.Sprintf("b3l0w ya está al día (%s)", result.After))
	}
	return 0
}

func updateSystem(opts Options) int {
	if !system.Available() {
		cli.Warn("no hay cachy-update ni arch-update: se omite el sistema")
		// Si pidió solo el sistema, es un error.
		if opts.System && !opts.Programs {
			return 1
		}
		return 0
	}

	// El actualizador pregunta cosas; sin terminal no se puede.
	if !cli.IsTTY() {
		if opts.System && !opts.Programs {
			cli.Fail("sin terminal: la actualización del sistema es interactiva")
			cli.Fail("corré b3l0w update en una terminal, o usá --programs")
			return 1
		}
		cli.Warn("sin terminal: se omite el sistema (es interactivo); usá --programs")
		return 0
	}

	cli.Step("Actualizando el sistema")
	if err := system.UpdateInteractive(); err != nil {
		cli.Fail(fmt.Sprintf("el actualizador del sistema falló: %v", err))
		return 1
	}
	// cachy-update deja la terminal en alternate screen: restaurarla acá
	// permite leer el resto del update sin arrastrar el estado.
	cli.RestoreTTY()
	return 0
}

// updateDotfiles aplica los TOML del repo sin pisar cambios locales.
func updateDotfiles(opts Options) int {
	dir := opts.DotfilesDir
	if dir == "" {
		dir = defaultDotfilesDir()
	}
	if dir == "" {
		cli.Fail("no encontré dotfiles/; pasá --dotfiles-dir=<ruta>")
		return 1
	}

	// Validar el TOML de Noctalia antes de tocar nada.
	if path := filepath.Join(dir, "noctalia", "config.toml"); fileExists(path) {
		if out, err := cli.RunCombined("noctalia", "config", "validate", path); err != nil {
			cli.Fail("el TOML del repo no valida: " + out)
			return 1
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	state := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")

	// Snapshot antes de tocar el home. Si snapper no tiene config `home`,
	// el 3-way sigue protegiendo: no se pisa ningún cambio local.
	if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
		cli.Warn("sin snapshot de home: " + err.Error())
	}

	cli.Step("Aplicando dotfiles")
	res, err := dotfiles.Pull(dir, home, state, opts.Force, opts.DryRun)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}

	for _, f := range res.Updated {
		cli.Ok(fmt.Sprintf("%s: actualizado", f))
	}
	for _, f := range res.Kept {
		fmt.Printf("  %s %s: conservado (cambio local)\n", cli.Dim("·"), f)
	}
	for _, f := range res.Missing {
		cli.Ok(fmt.Sprintf("%s: instalado", f))
	}
	for _, f := range res.Conflicts {
		cli.Warn(fmt.Sprintf("%s: conflicto, revisá %s.new", f, f))
	}

	if opts.DryRun {
		fmt.Printf("\n  %s\n", cli.Dim("dry-run: no se escribió nada"))
	}
	if len(res.Conflicts) > 0 {
		fmt.Printf("\n  %s\n", cli.Dim("los conflictos no se pisaron; fusioná a mano y volvé a correr"))
	}
	return 0
}

func defaultDotfilesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, "Documents", "Github", "b3l0w", "dotfiles"),
		filepath.Join(home, ".local", "share", "b3l0w", "dotfiles"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// confirmPrompt arma la pregunta según lo que se va a tocar. Formato
// pacman: [Y/n] con Y por defecto.
func confirmPrompt(system, programs bool) string {
	switch {
	case system && programs:
		return "Update system and programs? [Y/n] "
	case system:
		return "Update system? [Y/n] "
	default:
		return "Update programs? [Y/n] "
	}
}

// updatePrograms corre el update de cada programa y NO se detiene en el
// primero que falla: abortar esconde el resto y deja el sistema a medio
// actualizar sin saberlo.
func updatePrograms(m *config.Manifest) int {
	if len(m.Programs) == 0 {
		cli.Warn("no hay programas registrados")
		return 0
	}

	cli.Step("Actualizando los programas propios")
	failed := 0
	for _, program := range m.Programs {
		message, ok := programs.Update(program.Name)
		if ok {
			cli.Ok(fmt.Sprintf("%s: %s", program.Name, message))
		} else {
			cli.Fail(fmt.Sprintf("%s: %s", program.Name, message))
			failed++
		}
	}

	if failed > 0 {
		cli.Fail(fmt.Sprintf("%d de %d programas fallaron", failed, len(m.Programs)))
		return 1
	}
	return 0
}

// check reporta qué hay pendiente sin tocar nada.
func check(m *config.Manifest) int {
	cli.Step("Estado de las actualizaciones")

	if system.Available() {
		list, err := system.List()
		if err != nil && list == "" {
			cli.Fail(fmt.Sprintf("no se pudo consultar el sistema: %v", err))
			return 1
		}
		repo, aur := system.Pending(list)
		if repo+aur == 0 {
			cli.Ok("sistema base: al día")
		} else {
			cli.Ok(fmt.Sprintf("sistema base: %d del repo, %d de AUR", repo, aur))
		}
	} else {
		cli.Warn("sistema base: no hay cachy-update ni arch-update")
	}

	for _, status := range programs.Statuses(m) {
		switch {
		case !status.Installed:
			cli.Warn(fmt.Sprintf("%s: no instalado", status.Name))
		case status.Err != "":
			cli.Fail(fmt.Sprintf("%s: %s", status.Name, status.Err))
		default:
			cli.Ok(fmt.Sprintf("%s: %s", status.Name, status.Version))
		}
	}

	checkDesktop()
	return 0
}

// checkDesktop compara lo instalado con AUR. Sin red es un warning.
func checkDesktop() {
	ch := channel.Current()
	remote, err := versioncheck.AUR([]string{"umbriel-git", "noctalia-git"})
	if err != nil {
		cli.Warn("escritorio: no se pudo consultar AUR")
		return
	}
	for _, pkg := range remote {
		local := localVersion(pkg.Name)
		switch {
		case local == "":
			cli.Warn(fmt.Sprintf("%s: no instalado (%s)", pkg.Name, ch))
		case compareVersions(local, pkg.Version) == 0:
			cli.Ok(fmt.Sprintf("%s: %s (%s)", pkg.Name, local, ch))
		case compareVersions(local, pkg.Version) > 0:
			cli.Ok(fmt.Sprintf("%s: %s (local más nuevo que AUR; %s)", pkg.Name, local, ch))
		default:
			cli.Warn(fmt.Sprintf("%s: %s -> %s (%s)", pkg.Name, local, pkg.Version, ch))
		}
	}
}

// desktopUpdate no ejecuta pacman: muestra el comando y el canal.
func desktopUpdate() int {
	ch := channel.Current()
	pkgs := []string{"umbriel-git", "noctalia-git"}
	if ch == channel.Stable {
		pkgs = []string{"umbriel", "noctalia"}
	}
	cli.Step("Escritorio")
	fmt.Printf("  canal: %s\n", ch)
	fmt.Printf("  b3l0w todavía no ejecuta pacman.\n\n")
	fmt.Printf("  para actualizar:\n    sudo pacman -S %s\n", strings.Join(pkgs, " "))
	return 0
}

func localVersion(pkg string) string {
	out, err := cli.Run("pacman", "-Q", pkg)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// compareVersions usa vercmp (pacman): -1 local < remoto, 0 igual,
// 1 local > remoto. Sin vercmp asume igual para no inventar updates.
func compareVersions(local, remote string) int {
	out, err := cli.Run("vercmp", local, remote)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return n
}
