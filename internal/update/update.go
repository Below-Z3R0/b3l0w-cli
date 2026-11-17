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
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/dotfiles"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/registry"
	"github.com/Below-Z3R0/b3l0w-cli/internal/releases"
	"github.com/Below-Z3R0/b3l0w-cli/internal/selfupdate"
	"github.com/Below-Z3R0/b3l0w-cli/internal/services"
	"github.com/Below-Z3R0/b3l0w-cli/internal/snapshot"
	"github.com/Below-Z3R0/b3l0w-cli/internal/snapshots"
	"github.com/Below-Z3R0/b3l0w-cli/internal/system"
	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
	"github.com/Below-Z3R0/b3l0w-cli/internal/versioncheck"

	"github.com/Below-Z3R0/b3l0w-cli/kit/confirm"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
	kitsystem "github.com/Below-Z3R0/b3l0w-cli/kit/system"
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
	Keep        int
	Plugins     bool
	Services    bool
	Adopt       bool
}

// Run ejecuta el update y devuelve el código de salida.
func Run(m *config.Manifest, opts Options) int {
	if opts.Check {
		return check(m)
	}

	// Dotfiles: no pisa cambios locales, así que no pide confirmación.
	if opts.Dotfiles || opts.Adopt {
		return updateDotfiles(opts)
	}

	// Escritorio: por ahora solo verifica y sugiere; no ejecuta pacman.
	if opts.Desktop {
		return desktopUpdate()
	}

	// Pase lo que pase, la terminal tiene que quedar usable: cachy-update
	// usa alternate screen y deja el cursor oculto.
	defer kitsystem.RestoreTTY()

	// --dry-run tiene que ser un dry-run de verdad.
	//
	// Antes solo lo respetaba la capa de dotfiles: selfUpdate(),
	// updateSystem() y updatePrograms() lo ignoraban y ejecutaban. Un
	// "b3l0w update --dry-run" reemplazaba el binario en ejecucion y
	// dejaba el sistema a medio actualizar.
	//
	// Con el flag se muestra el plan y no se toca nada.
	if opts.DryRun {
		return dryRun(m, opts)
	}

	// Sin banderas se hacen las tres capas y el CLI; con una sola, esa.
	anySelector := opts.System || opts.Programs || opts.Plugins || opts.Services
	selfWanted := opts.Self || !anySelector
	systemWanted := opts.System || !anySelector
	programsWanted := opts.Programs || !anySelector
	pluginsWanted := opts.Plugins || !anySelector
	servicesWanted := opts.Services || !anySelector
	// Los dotfiles usan merge 3-way: no pisan cambios locales, así
	// que van en el update general sin pedir nada extra.
	dotfilesWanted := !anySelector

	if !opts.Yes {
		if !confirm.Confirm(confirmPrompt(selfWanted, systemWanted, programsWanted, pluginsWanted, servicesWanted, dotfilesWanted), false, true) {
			output.Warn("cancelled")
			return 0
		}
	}

	// Refrescar las DB propias antes de comparar: sin esto se
	// actualiza contra un catálogo viejo. Es el -Sy que faltaba.
	syncRegistry(systemWanted, servicesWanted, pluginsWanted)

	// El CLI primero: así el resto corre con la versión nueva.
	if selfWanted {
		if code := selfUpdate(); code != 0 {
			return code
		}
		// `update --self` a secas es solo eso.
		if opts.Self && !anySelector {
			return 0
		}
	}

	if systemWanted {
		if code := updateSystem(opts); code != 0 {
			return code
		}
	}
	if programsWanted {
		if code := updatePrograms(m); code != 0 {
			return code
		}
	}
	if pluginsWanted {
		if code := updatePlugins(); code != 0 {
			return code
		}
	}
	if servicesWanted {
		updateServices(opts.Yes)
	}
	if dotfilesWanted {
		if code := updateDotfiles(opts); code != 0 {
			return code
		}
	}
	code := snapshots.Report(opts.Keep, opts.Yes)
	if code == 0 {
		output.Step("Done")
		output.Ok("update complete")
	}
	return code
}

// dryRun muestra que haria el update y no toca nada.
//
// El plan tiene que ser util: si solo dice "dry-run" el usuario no
// sabe que habria cambiado. Cada capa dice que haria.
func dryRun(m *config.Manifest, opts Options) int {
	anySelector := opts.System || opts.Programs || opts.Plugins || opts.Services

	output.Step("Dry-run: no se toca nada")

	if opts.Self || !anySelector {
		if tag, err := selfupdate.Check(); err == nil && tag != "" && tag != version.Version {
			output.Step(fmt.Sprintf("  b3l0w: %s -> %s", version.Version, tag))
		} else {
			output.Step(fmt.Sprintf("  b3l0w: %s (sin cambios)", version.Version))
		}
	}
	if opts.System || !anySelector {
		output.Step("  sistema: se actualizaria con cachy-update")
	}
	if opts.Programs || !anySelector {
		output.Step(fmt.Sprintf("  programas: %d se revisarian", len(m.Programs)))
	}
	if opts.Plugins || !anySelector {
		output.Step("  plugins: los sources se refrescarian")
	}
	if opts.Services || !anySelector {
		output.Step("  servicios: las imagenes se revisarian")
	}
	if !anySelector {
		output.Step("  dotfiles: se aplicaria el merge 3-way")
	}

	output.Ok("dry-run completo")
	return 0
}

// updatePlugins refresca los sources con preflight de plugin_api. Si
// un plugin habilitado no es compatible con el shell, se saltea el
// source entero y se conserva la versión instalada.
func updatePlugins() int {
	output.Step("Actualizando plugins")

	if kitsystem.HaveCommand("noctalia") {
		if sources, err := plugins.Sources(); err == nil && len(sources) > 0 {
			if err := plugins.UpdateSources(); err != nil {
				output.Warn("plugins: " + err.Error())
				return 1
			}
			return 0
		}
	}

	m, err := plugins.LoadDefault()
	if err != nil {
		output.Warn("plugins: " + err.Error())
		return 1
	}
	if err := plugins.Update(m, "", plugins.DefaultPluginsDir(), false); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

// updateServices pregunta por cada servicio instalado si hay digest
// nuevo y lo actualiza. Los servicios son opcionales: sin estado
// guardado, no hay nada que hacer.
func updateServices(yes bool) int {
	states, err := services.ListStates(services.DefaultStateDir())
	if err != nil || len(states) == 0 {
		return 0
	}
	output.Step("Actualizando servicios")
	m, err := services.LoadDefault()
	if err != nil {
		output.Warn("services: " + err.Error())
		return 0
	}
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: services.DefaultStateDir(),
	}
	for _, state := range states {
		svc := m.Find(state.ID)
		if svc == nil {
			continue
		}
		newState, err := in.Update(*svc, state, true, yes)
		if err != nil {
			output.Fail(fmt.Sprintf("%s: %v", state.ID, err))
			continue
		}
		if newState.UpdatedAt != state.UpdatedAt {
			output.Ok(fmt.Sprintf("%s actualizado", state.ID))
		}
	}
	return 0
}

// syncRegistry baja las DB del registry antes de aplicar cambios.
//
// Sin embed: si no hay red se usa el caché; si tampoco hay, se avisa y
// se sigue con lo que haya. Un sync fallido no debe abortar el update.
func syncRegistry(systemWanted, servicesWanted, pluginsWanted bool) {
	var kinds []registry.Kind
	if systemWanted {
		kinds = append(kinds, registry.System)
	}
	if servicesWanted {
		kinds = append(kinds, registry.Services)
	}
	if pluginsWanted {
		kinds = append(kinds, registry.Plugins)
	}
	for _, kind := range kinds {
		if _, err := registry.Sync(kind, nil); err != nil {
			output.Warn(err.Error())
		}
	}
}

func selfUpdate() int {
	output.Step("Actualizando b3l0w")
	result, err := selfupdate.Run()
	if err != nil {
		// Si el binario es de un paquete, pacman manda: no es fatal.
		if strings.Contains(err.Error(), "es del paquete") {
			output.Warn(err.Error())
			return 0
		}
		output.Fail(err.Error())
		return 1
	}
	switch {
	case result.Changed:
		output.Ok(fmt.Sprintf("b3l0w %s -> %s", result.Before, result.After))
	case result.Note != "":
		output.Warn(result.Note)
	default:
		output.Ok(fmt.Sprintf("b3l0w ya está al día (%s)", result.After))
	}
	return 0
}

func updateSystem(opts Options) int {
	if !system.Available() {
		output.Warn("no hay cachy-update ni arch-update: se omite el sistema")
		// Si pidió solo el sistema, es un error.
		if opts.System && !opts.Programs {
			return 1
		}
		return 0
	}

	// El actualizador pregunta cosas; sin terminal no se puede.
	if !kitsystem.IsTTY() {
		if opts.System && !opts.Programs {
			output.Fail("sin terminal: la actualización del sistema es interactiva")
			output.Fail("corré b3l0w update en una terminal, o usá --programs")
			return 1
		}
		output.Warn("sin terminal: se omite el sistema (es interactivo); usá --programs")
		return 0
	}

	output.Step("Actualizando el sistema")
	if err := system.UpdateInteractive(); err != nil {
		output.Fail(fmt.Sprintf("el actualizador del sistema falló: %v", err))
		return 1
	}
	// cachy-update deja la terminal en alternate screen: restaurarla acá
	// permite leer el resto del update sin arrastrar el estado.
	kitsystem.RestoreTTY()
	return 0
}

// updateDotfiles aplica los TOML del repo sin pisar cambios locales.
func updateDotfiles(opts Options) int {
	dir := opts.DotfilesDir
	if dir == "" {
		dir = defaultDotfilesDir()
	}
	if dir == "" {
		output.Fail("dotfiles/ not found; pass --dotfiles-dir=<path>")
		return 1
	}

	// Validar el TOML de Noctalia antes de tocar nada.
	if path := filepath.Join(dir, "noctalia", "config.toml"); fileExists(path) {
		if out, err := kitsystem.RunCombined("noctalia", "config", "validate", path); err != nil {
			output.Fail("repo TOML does not validate: " + out)
			return 1
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	state := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")

	if opts.Adopt {
		n, err := dotfiles.Adopt(dir, home, state)
		if err != nil {
			output.Fail(err.Error())
			return 1
		}
		output.Ok(fmt.Sprintf("%d files adopted (untouched)", n))
		return 0
	}

	// Snapshot antes de tocar el home. Si snapper no tiene config `home`,
	// el 3-way sigue protegiendo: no se pisa ningún cambio local.
	if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
		output.Warn("no home snapshot: " + err.Error())
	}

	// Detectar conflictos antes de escribir: si los hay, preguntar si
	// el usuario quiere que el repo gane, con un segundo aviso.
	// El preview se hace siempre (tambien en dry-run): el usuario tiene
	// que ver los conflictos antes de confirmar. Con el if de antes
	// (que excluia el dry-run) el dry-run no mostraba ninguno, y un
	// dry-run que no dice lo que pasaria no sirve de nada.
	if !opts.Force {
		if preview, err := dotfiles.Pull(dir, home, state, false, true); err == nil && len(preview.Conflicts) > 0 {
			fmt.Printf("  ! %d conflict(s):\n", len(preview.Conflicts))
			for _, f := range preview.Conflicts {
				fmt.Printf("    %s\n", f)
			}
			if confirm.Confirm("  Merge automatically? The repo version overwrites your local changes [y/N] ", false, false) {
				if confirm.Confirm(fmt.Sprintf("  This will overwrite %d file(s). Continue? [y/N] ", len(preview.Conflicts)), false, false) {
					opts.Force = true
				}
			}
			fmt.Println()
		}
	}

	output.Step("Applying dotfiles")
	res, err := dotfiles.Pull(dir, home, state, opts.Force, opts.DryRun)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}

	for _, f := range res.Updated {
		output.Ok(fmt.Sprintf("%s: updated", f))
	}
	for _, f := range res.Kept {
		fmt.Printf("  %s %s: kept (local change)\n", output.Dim("·"), f)
	}
	for _, f := range res.Missing {
		output.Ok(fmt.Sprintf("%s: installed", f))
	}
	for _, f := range res.Conflicts {
		output.Warn(fmt.Sprintf("%s: conflict, see %s.new", f, f))
	}
	// Lo que quedo en el repo pero no en el estado: se avisa, pero el
	// archivo local no se borra.
	for _, f := range res.Orphaned {
		output.Warn(fmt.Sprintf("%s: orphaned (in the state, not in the repo)", f))
	}

	if opts.DryRun {
		fmt.Printf("\n  %s\n", output.Dim("dry-run: nothing written"))
	}
	if len(res.Conflicts) > 0 {
		fmt.Printf("\n  %s\n", output.Dim("conflicts were not overwritten; merge by hand and run again"))
		fmt.Printf("  %s\n", output.Dim("validate with: noctalia config validate  |  umbriel config validate"))
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
func confirmPrompt(self, system, programs, pluginsWanted, servicesWanted, dotfilesWanted bool) string {
	var parts []string
	if self {
		parts = append(parts, "b3l0w")
	}
	if system {
		parts = append(parts, "system")
	}
	if programs {
		parts = append(parts, "programs")
	}
	if pluginsWanted {
		parts = append(parts, "plugins")
	}
	if servicesWanted {
		parts = append(parts, "services")
	}
	if dotfilesWanted {
		parts = append(parts, "dotfiles")
	}
	if len(parts) == 0 {
		return "Update? [Y/n] "
	}
	return "Update " + strings.Join(parts, ", ") + "? [Y/n] "
}

// updatePrograms corre el update de cada programa y NO se detiene en el
// primero que falla: abortar esconde el resto y deja el sistema a medio
// actualizar sin saberlo.
func updatePrograms(m *config.Manifest) int {
	if len(m.Programs) == 0 {
		output.Warn("no hay programas registrados")
		return 0
	}

	output.Step("Actualizando los programas propios")
	failed := 0
	for _, program := range m.Programs {
		message, ok := programs.Update(program.Name)
		if ok {
			output.Ok(fmt.Sprintf("%s: %s", program.Name, message))
		} else {
			output.Fail(fmt.Sprintf("%s: %s", program.Name, message))
			failed++
		}
	}

	if failed > 0 {
		output.Fail(fmt.Sprintf("%d de %d programas fallaron", failed, len(m.Programs)))
		return 1
	}
	return 0
}

// check reporta qué hay pendiente sin tocar nada.
func check(m *config.Manifest) int {
	output.Step("b3l0w")
	output.Ok(fmt.Sprintf("versión: %s", version.Version))
	if !strings.Contains(version.Version, "dev") {
		if latest, err := selfupdate.Check(); err == nil && latest != "" && latest != version.Version {
			output.Warn(fmt.Sprintf("b3l0w: %s -> %s", version.Version, latest))
		}
	}

	output.Step("Estado de las actualizaciones")

	if system.Available() {
		list, err := system.List()
		if err != nil && list == "" {
			output.Fail(fmt.Sprintf("no se pudo consultar el sistema: %v", err))
			return 1
		}
		repo, aur := system.Pending(list)
		if repo+aur == 0 {
			output.Ok("sistema base: al día")
		} else {
			output.Ok(fmt.Sprintf("sistema base: %d del repo, %d de AUR", repo, aur))
		}
	} else {
		output.Warn("sistema base: no hay cachy-update ni arch-update")
	}

	for _, status := range programs.Statuses(m) {
		switch {
		case !status.Installed:
			output.Warn(fmt.Sprintf("%s: no instalado", status.Name))
		case status.Err != "":
			output.Fail(fmt.Sprintf("%s: %s", status.Name, status.Err))
		default:
			output.Ok(fmt.Sprintf("%s: %s", status.Name, status.Version))
		}
	}

	checkDesktop()
	checkPlugins()

	// Un release sin firma rompe install y update --self. Este es el
	// chequeo previo, así que se avisa acá y no al fallar la descarga.
	if sinFirma := releases.Render(releases.Check(m)); sinFirma > 0 {
		output.Warn(fmt.Sprintf("%d release(s) sin firma", sinFirma))
	}
	return 0
}

// checkPlugins compara el registry con lo instalado y avisa si falta
// un script de backend.
func checkPlugins() {
	m, err := plugins.LoadDefault()
	if err != nil {
		output.Warn("plugins: " + err.Error())
		return
	}
	for _, entry := range m.Plugins {
		local := plugins.InstalledVersion(plugins.DefaultPluginsDir(), entry)
		switch {
		case local == "":
			output.Warn(fmt.Sprintf("%s: no instalado", entry.ID))
		case local == entry.Version:
			output.Ok(fmt.Sprintf("%s: %s", entry.ID, local))
		default:
			output.Warn(fmt.Sprintf("%s: %s -> %s", entry.ID, local, entry.Version))
		}
	}
	if names, err := plugins.Scripts(); err == nil {
		for _, name := range names {
			_ = name // los backends viven en el binario
		}
	}
}

// checkDesktop compara lo instalado con AUR. Sin red es un warning.
func checkDesktop() {
	ch := channel.Current()
	remote, err := versioncheck.AUR([]string{"umbriel-git", "noctalia-git"})
	if err != nil {
		output.Warn("escritorio: no se pudo consultar AUR")
		return
	}
	for _, pkg := range remote {
		local := localVersion(pkg.Name)
		switch {
		case local == "":
			output.Warn(fmt.Sprintf("%s: no instalado (%s)", pkg.Name, ch))
		case compareVersions(local, pkg.Version) == 0:
			output.Ok(fmt.Sprintf("%s: %s (%s)", pkg.Name, local, ch))
		case compareVersions(local, pkg.Version) > 0:
			output.Ok(fmt.Sprintf("%s: %s (local más nuevo que AUR; %s)", pkg.Name, local, ch))
		default:
			output.Warn(fmt.Sprintf("%s: %s -> %s (%s)", pkg.Name, local, pkg.Version, ch))
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
	output.Step("Escritorio")
	fmt.Printf("  canal: %s\n", ch)
	fmt.Printf("  b3l0w todavía no ejecuta pacman.\n\n")
	fmt.Printf("  para actualizar:\n    sudo pacman -S %s\n", strings.Join(pkgs, " "))
	return 0
}

func localVersion(pkg string) string {
	out, err := kitsystem.Run("pacman", "-Q", pkg)
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
	out, err := kitsystem.Run("vercmp", local, remote)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return n
}
