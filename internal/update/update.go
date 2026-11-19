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
	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
	kitsystem "github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Inyectables para los tests.
//
// El módulo llama a la red (GitHub, AUR) y a Docker. Los tests no
// pueden tocar ninguno de los dos, así que estas variables se
// sustituyen por fakes. Es el mismo patrón que ya usa internal/selfupdate
// con apiURL y replaceFile.
var (
	selfCheck     = selfupdate.Check
	aurLookup     = versioncheck.AUR
	registrySync  = registry.Sync
	releaseCheck  = releases.Check
	pluginSources = plugins.Sources
	updateService = func(in *services.Installer, svc services.Service, state *services.State, rebuild, yes bool) (*services.State, error) {
		return in.Update(svc, state, rebuild, yes)
	}
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
	//
	// `--self` cuenta como selector (M1): antes no contaba, así que el
	// prompt prometía el sistema, los programas, los plugins, los
	// servicios y los dotfiles cuando en realidad solo se actualizaba
	// el CLI.
	c := capasPedidas(opts)
	selfWanted := c.self
	systemWanted := c.system
	programsWanted := c.programs
	pluginsWanted := c.plugins
	servicesWanted := c.services
	// Los dotfiles usan merge 3-way: no pisan cambios locales, así
	// que van en el update general sin pedir nada extra.
	dotfilesWanted := c.dotfiles

	if !opts.Yes {
		if !confirm.Confirm(confirmPrompt(selfWanted, systemWanted, programsWanted, pluginsWanted, servicesWanted, dotfilesWanted), false, true) {
			output.Warn("cancelled")
			// Cancelar no es "se actualizó todo": un script tiene que
			// poder distinguirlo.
			return 1
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
		if soloSelf(opts) {
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
		if code := updateServices(opts.Yes); code != 0 {
			return code
		}
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

// capas son las capas que un update va a tocar.
type capas struct {
	self, system, programs, plugins, services, dotfiles bool
}

// capasPedidas decide qué capas cubre un update.
//
// Un flag de selección (`--system`, `--programs`, `--plugins`,
// `--services`, `--self`) cuenta como selector: si el usuario pide una
// sola capa, solo esa se actualiza.
//
// `--self` también cuenta (M1): antes no contaba, así que el prompt
// prometía las seis capas cuando en realidad solo se actualizaba el
// CLI. Sin ningún flag se hacen todas.
func capasPedidas(opts Options) capas {
	anySelector := opts.System || opts.Programs || opts.Plugins ||
		opts.Services || opts.Self
	return capas{
		self:     opts.Self || !anySelector,
		system:   opts.System || !anySelector,
		programs: opts.Programs || !anySelector,
		plugins:  opts.Plugins || !anySelector,
		services: opts.Services || !anySelector,
		dotfiles: !anySelector,
	}
}

// soloSelf dice que el único flag de selección es `--self`: en ese
// caso el update corta después del CLI y no toca el resto.
func soloSelf(opts Options) bool {
	return opts.Self && !opts.System && !opts.Programs && !opts.Plugins &&
		!opts.Services
}

// dryRun muestra que haria el update y no toca nada.
//
// El plan tiene que ser util: si solo dice "dry-run" el usuario no
// sabe que habria cambiado. Cada capa dice que haria.
func dryRun(m *config.Manifest, opts Options) int {
	c := capasPedidas(opts)

	output.Step("Dry-run: no se toca nada")

	if c.self {
		if tag, err := selfCheck(); err == nil && tag != "" && versionMasNueva(tag, version.Version) {
			output.Step(fmt.Sprintf("  b3l0w: %s -> %s", version.Version, tag))
		} else {
			output.Step(fmt.Sprintf("  b3l0w: %s (sin cambios)", version.Version))
		}
	}
	if c.system {
		output.Step("  sistema: se actualizaria con cachy-update")
	}
	if c.programs {
		output.Step(fmt.Sprintf("  programas: %d se revisarian", len(m.Programs)))
	}
	if c.plugins {
		output.Step("  plugins: los sources se refrescarian")
	}
	if c.services {
		output.Step("  servicios: las imagenes se revisarian")
	}
	if c.dotfiles {
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

	// Si Noctalia está, sus sources son la fuente de verdad: un source
	// que no se puede leer es un ERROR, no una invitación al camino de
	// abajo (M5).
	//
	// Un binario ausente y un source roto son casos distintos: antes
	// los dos caían al mismo camino, y un error transitorio de
	// `Sources()` cambiaba en silencio QUÉ se actualizaba.
	if kitsystem.HaveCommand("noctalia") {
		sources, err := pluginSources()
		if err != nil {
			output.Fail("no se pudieron leer los sources: " + err.Error())
			return 1
		}
		if len(sources) > 0 {
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
	// Un servicio que no se actualiza se sigue contando como fallo:
	// se sigue con el resto (para ver todos los errores), pero el
	// resultado tiene que llegar al codigo de salida.
	fallos := 0
	for _, state := range states {
		svc := m.Find(state.ID)
		if svc == nil {
			continue
		}
		newState, err := updateService(in, *svc, state, true, yes)
		if err != nil {
			output.Fail(fmt.Sprintf("%s: %v", state.ID, err))
			fallos++
			continue
		}
		if newState.UpdatedAt != state.UpdatedAt {
			output.Ok(fmt.Sprintf("%s actualizado", state.ID))
		}
	}
	if fallos > 0 {
		return 1
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
	failed := 0
	for _, kind := range kinds {
		if _, err := registrySync(kind, nil); err != nil {
			output.Warn(fmt.Sprintf("%s: %v", kind, err))
			failed++
		}
	}
	if failed > 0 {
		output.Warn(fmt.Sprintf("%d de %d catálogos sin refrescar; se usa la copia local", failed, len(kinds)))
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

	// Validar el TOML de Noctalia Y de Umbriel antes de tocar nada.
	//
	// Los dos: el final del update le dice al usuario que valide
	// `noctalia config validate | umbriel config validate`, así que un
	// TOML de Umbriel roto tiene que abortar igual que uno de
	// Noctalia (M3).
	//
	// Solo si el validador está instalado: si Noctalia no está, el
	// TOML puede estar perfectamente y lo que falta es el programa que
	// lo valida. Antes esto abortaba con 1 y decía "repo TOML does not
	// validate", que era falso.
	for _, cfg := range []struct{ dir, cmd string }{
		{"noctalia", "noctalia"},
		{"umbriel", "umbriel"},
	} {
		path := filepath.Join(dir, cfg.dir, "config.toml")
		if !fileExists(path) {
			continue
		}
		if !kitsystem.HaveCommand(cfg.cmd) {
			output.Warn(cfg.cmd + " no está instalado: no se valida el TOML del repo")
			continue
		}
		if out, err := kitsystem.RunCombined(cfg.cmd, "config", "validate", path); err != nil {
			output.Fail("repo TOML does not validate (" + cfg.dir + "): " + out)
			return 1
		}
	}

	// El home del usuario: el mismo de todo el módulo, y explícito
	// cuando no se puede resolver (M4).
	home := paths.Home()
	if home == "" {
		output.Fail("no se pudo resolver el home del usuario: revisá $HOME")
		return 1
	}
	state := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")

	if opts.Adopt {
		if opts.DryRun {
			// Adopt registra el estado de cada archivo: no hay forma de
			// previsualizarlo sin escribir. Se dice y no se ejecuta.
			output.Warn("--adopt no tiene preview: registra el estado de cada archivo")
			return 0
		}
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
	//
	// En dry-run no: el flag promete no tocar nada y una snapshot es
	// escribir en el sistema. Antes se creaba igual, así que
	// `--dotfiles --dry-run` dejaba una snapshot real por corrida.
	if !opts.DryRun {
		if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
			output.Warn("no home snapshot: " + err.Error())
		}
	}

	// Detectar conflictos antes de escribir: si los hay, preguntar si
	// el usuario quiere que el repo gane, con un segundo aviso.
	// El preview se hace siempre (tambien en dry-run): el usuario tiene
	// que ver los conflictos antes de confirmar. Con el if de antes
	// (que excluia el dry-run) el dry-run no mostraba ninguno, y un
	// dry-run que no dice lo que pasaria no sirve de nada.
	if !opts.Force {
		// Si el preview falla, no se aplica nada (M2).
		//
		// Antes el error se descartaba (`err == nil &&`): el módulo
		// seguía al pull real, que fallaba con el mismo error, y el
		// usuario veía el fallo dos veces sin haber visto nunca los
		// conflictos. Peor: si el preview fallaba por un motivo que el
		// pull real no reproduce, el pull real se ejecutaba SIN la
		// confirmación que el preview iba a pedir.
		preview, err := dotfiles.Pull(dir, home, state, false, true)
		if err != nil {
			output.Fail("no se pudo previsualizar dotfiles: " + err.Error())
			return 1
		}
		if len(preview.Conflicts) > 0 {
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
	home := paths.Home()
	if home == "" {
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
	avisoVersionCLI()

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

	// El escritorio cuenta: si no se pudo consultar el AUR, "check" no
	// puede salir con 0 diciendo que todo está al día (M6).
	fallos := 0
	if !checkDesktop() {
		fallos++
	}
	checkPlugins()

	// Un release sin firma rompe install y update --self. Este es el
	// chequeo previo, así que se avisa acá y no al fallar la descarga.
	if sinFirma := releases.Render(releaseCheck(m)); sinFirma > 0 {
		output.Warn(fmt.Sprintf("%d release(s) sin firma", sinFirma))
	}
	if fallos > 0 {
		return 1
	}
	return 0
}

// avisoVersionCLI avisa si hay una versión del CLI más nueva que la
// local.
//
// La comparación es numérica: "v0.5.0" y "0.5.0" son la misma
// versión, así que no se avisa de una actualización que ya se tiene.
func avisoVersionCLI() {
	if strings.Contains(version.Version, "dev") {
		return
	}
	latest, err := selfCheck()
	if err != nil || latest == "" {
		return
	}
	if versionMasNueva(latest, version.Version) {
		output.Warn(fmt.Sprintf("b3l0w: %s -> %s", version.Version, latest))
	}
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

// desktopPackages da los paquetes del escritorio según el canal.
//
// Las dos mitades del módulo (el check y el comando a mano) tienen que
// preguntar por los mismos paquetes: si una pregunta por los -git y la
// otra por los estables, el check dice que no está instalado lo que
// sí lo está.
func desktopPackages(ch string) []string {
	if ch == channel.Stable {
		return []string{"umbriel", "noctalia"}
	}
	return []string{"umbriel-git", "noctalia-git"}
}

// checkDesktop compara lo instalado con AUR. Sin red es un warning,
// pero es un fallo: el que llama lo suma a su contador.
//
// El mensaje no cambia: lo que mentía era el código de salida (M6).
func checkDesktop() bool {
	ch := channel.Current()
	remote, err := aurLookup(desktopPackages(ch))
	if err != nil {
		output.Warn("escritorio: no se pudo consultar AUR")
		return false
	}
	for _, pkg := range remote {
		local, err := localVersion(pkg.Name)
		if err != nil {
			output.Warn(fmt.Sprintf("%s: no se pudo leer la versión: %v", pkg.Name, err))
			continue
		}
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
	return true
}

// desktopUpdate no ejecuta pacman: muestra el comando y el canal.
func desktopUpdate() int {
	ch := channel.Current()
	pkgs := desktopPackages(ch)
	output.Step("Escritorio")
	fmt.Printf("  canal: %s\n", ch)
	fmt.Printf("  b3l0w todavía no ejecuta pacman.\n\n")
	fmt.Printf("  para actualizar:\n    sudo pacman -S %s\n", strings.Join(pkgs, " "))
	return 0
}

// localVersion devuelve la versión instalada de un paquete.
//
// "" con nil es "no instalado"; el error es un fallo de pacman (DB
// corrupta, sin permisos, binario ausente). Antes los dos casos eran ""
// y el consumidor los leía igual.
func localVersion(pkg string) (string, error) {
	out, err := kitsystem.Run("pacman", "-Q", pkg)
	if err != nil {
		// pacman -Q sale con 1 cuando el paquete no está instalado.
		if strings.Contains(err.Error(), "exit status 1") {
			return "", nil
		}
		return "", fmt.Errorf("pacman -Q %s: %w", pkg, err)
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return "", nil
	}
	return fields[1], nil
}

// versionMasNueva dice si la versión remota es más nueva que la local.
//
// Numérica, no de strings: "v0.5.0" y "0.5.0" son la misma versión
// (el ldflags puede traerla con o sin la "v") y "0.10.0" es más
// nueva que "0.9.0" aunque como string parezca al revés. Los
// sufijos (-dev, +build) se ignoran: "v0.5.0-dev" es la misma
// versión que "v0.5.0".
//
// Es la misma regla con la que internal/selfupdate decide si baja un
// release, replicada acá para no tocar otro paquete.
func versionMasNueva(remota, local string) bool {
	r := numerosVersion(remota)
	l := numerosVersion(local)
	for i := 0; i < 3; i++ {
		if r[i] != l[i] {
			return r[i] > l[i]
		}
	}
	return false
}

// numerosVersion saca los tres primeros números de una versión:
// "v0.5.0-dev" -> {0, 5, 0}. Lo que no es número cuenta como 0.
func numerosVersion(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, parte := range strings.Split(v, ".") {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(parte)
	}
	return out
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
