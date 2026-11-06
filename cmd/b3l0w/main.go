// b3l0w es el CLI de control del sistema: versiones, estado, updates y
// extensiones de los programas propios.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/channel"
	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/doctor"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/services"
	"github.com/Below-Z3R0/b3l0w-cli/internal/snapshots"
	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
	"github.com/Below-Z3R0/b3l0w-cli/internal/systeminstall"
	"github.com/Below-Z3R0/b3l0w-cli/internal/update"
	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 0
	}

	command, rest := args[0], args[1:]

	switch command {
	case "help", "-h", "--help":
		usage()
		return 0
	case "version", "--version", "-V":
		fmt.Printf("b3l0w %s\n", version.Version)
		return 0
	case "list":
		return list()
	case "status":
		return withManifest(func(m *config.Manifest) int {
			return update.Run(m, update.Options{Check: true})
		})
	case "doctor":
		return withManifest(doctor.Run)
	case "update":
		return updateCommand(rest)
	case "install":
		return installCommand(rest)
	case "uninstall":
		return uninstallCommand(rest)
	case "program":
		return programCommand(rest)
	case "plugins":
		return pluginsCommand(rest)
	case "services":
		return servicesCommand(rest)
	case "plugin-exec":
		return pluginExecCommand(rest)
	case "channel":
		return channelCommand(rest)
	case "check":
		return checkCommand(rest)
	case "system":
		return systemCommand(rest)
	case "setup":
		return setupCommand()
	}

	// Extensión: b3l0w <nombre> -> b3l0w-<nombre> del directorio privado.
	if plugins.Find(command) != "" {
		if err := plugins.Run(command, rest); err != nil {
			cli.Fail(err.Error())
			return 1
		}
		return 0
	}

	cli.Fail(fmt.Sprintf("comando desconocido: %s", command))
	fmt.Println()
	usage()
	return 1
}

// withManifest carga el manifiesto una vez y se lo pasa al comando.
func withManifest(fn func(*config.Manifest) int) int {
	manifest, err := config.Load(cli.ManifestPath())
	if err != nil {
		cli.Fail(fmt.Sprintf("no se pudo leer %s: %v", cli.ManifestPath(), err))
		return 1
	}
	return fn(manifest)
}

func updateCommand(args []string) int {
	opts := update.Options{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--dotfiles-dir=") {
			opts.DotfilesDir = strings.TrimPrefix(arg, "--dotfiles-dir=")
			continue
		}
		if strings.HasPrefix(arg, "--keep=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--keep=")); err == nil && n > 0 {
				opts.Keep = n
			}
			continue
		}
		switch arg {
		case "--system":
			opts.System = true
		case "--programs":
			opts.Programs = true
		case "--check":
			opts.Check = true
		case "--yes", "-y":
			opts.Yes = true
		case "--dotfiles":
			opts.Dotfiles = true
		case "--desktop":
			opts.Desktop = true
		case "--plugins":
			opts.Plugins = true
		case "--services":
			opts.Services = true
		case "--dry-run":
			opts.DryRun = true
		case "--force":
			opts.Force = true
		case "--adopt":
			opts.Adopt = true
		case "--self":
			opts.Self = true
		case "-h", "--help":
			fmt.Println("uso: b3l0w update [--self] [--system|--programs|--plugins|--services|--dotfiles] [--check] [--yes] [--dry-run] [--force]")
			return 0
		default:
			cli.Fail(fmt.Sprintf("opción desconocida: %s", arg))
			return 1
		}
	}
	if opts.System && opts.Programs {
		cli.Fail("--system y --programs son excluyentes; sin banderas se hacen las dos")
		return 1
	}
	return withManifest(func(m *config.Manifest) int { return update.Run(m, opts) })
}

func list() int {
	code := withManifest(func(m *config.Manifest) int {
		cli.Step("Programas registrados")
		if len(m.Programs) == 0 {
			fmt.Println("  (ninguno)")
			return 0
		}
		for _, status := range programs.Statuses(m) {
			switch {
			case !status.Installed:
				fmt.Printf("  %-14s %s\n", status.Name, cli.Dim("no instalado"))
			case status.Err != "":
				fmt.Printf("  %-14s %s\n", status.Name, cli.Dim("error: "+status.Err))
			default:
				fmt.Printf("  %-14s %-8s %s\n", status.Name, status.Version, cli.Dim(status.Bin))
			}
		}
		return 0
	})
	if code != 0 {
		return code
	}
	pluginsList(nil)
	return 0
}

func uninstallCommand(args []string) int {
	if len(args) == 0 {
		cli.Fail("falta el programa: b3l0w uninstall <nombre>")
		return 1
	}
	name := args[0]
	dryRun := false
	for _, arg := range args[1:] {
		if arg == "--dry-run" {
			dryRun = true
		}
	}
	return withManifest(func(m *config.Manifest) int {
		if dryRun {
			path, _ := programs.InstallPath(name)
			fmt.Printf("  → borrar %s y quitarlo del manifiesto\n", path)
			return 0
		}
		path, err := programs.Uninstall(name, m)
		if err != nil {
			cli.Fail(err.Error())
			return 1
		}
		cli.Ok(fmt.Sprintf("%s desinstalado (%s)", name, path))
		return 0
	})
}

// installCommand baja el programa del último release y lo deja en
// ~/.local/bin. No toca su configuración: eso lo hace el propio
// programa (por ejemplo `kitty-shapes install`).
func installCommand(args []string) int {
	if len(args) == 0 {
		cli.Fail("falta el programa: b3l0w install <nombre>  (o `b3l0w install system`)")
		return 1
	}
	if args[0] == "system" {
		return installSystemCommand(args[1:])
	}
	name := args[0]
	source, asset := "", ""
	for _, arg := range args[1:] {
		switch {
		case strings.HasPrefix(arg, "--source="):
			source = strings.TrimPrefix(arg, "--source=")
		case strings.HasPrefix(arg, "--asset="):
			asset = strings.TrimPrefix(arg, "--asset=")
		}
	}
	return withManifest(func(m *config.Manifest) int {
		program := m.Get(name)
		if program == nil {
			if source == "" {
				cli.Fail(fmt.Sprintf("%s no está registrado (b3l0w install %s --source=github:owner/repo --asset=archivo)", name, name))
				return 1
			}
			m.AddProgram(config.Program{Name: name, Source: source, Asset: asset})
			if err := m.Save(cli.ManifestPath()); err != nil {
				cli.Fail(fmt.Sprintf("no se pudo guardar el manifiesto: %v", err))
				return 1
			}
			program = m.Get(name)
			cli.Ok(fmt.Sprintf("%s registrado", name))
		}
		cli.Step(fmt.Sprintf("Instalando %s", name))
		version, err := programs.Install(*program)
		if err != nil {
			cli.Fail(err.Error())
			return 1
		}
		dest, _ := programs.InstallPath(name)
		cli.Ok(fmt.Sprintf("%s %s en %s", name, version, dest))
		return 0
	})
}

// installSystemCommand compara la definición del sistema contra esta
// máquina y aplica lo que falta. Sin --apply no toca nada.
func installSystemCommand(args []string) int {
	profiles := []string{"desktop"}
	file := ""
	dotfiles := ""
	apply := false
	yes := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Acepta --clave=valor y --clave valor: escribir cualquiera de
		// las dos formas es lo natural, y fallar en una es una trampa.
		value := func() (string, bool) {
			if _, after, found := strings.Cut(arg, "="); found {
				return after, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}

		switch {
		case arg == "--apply":
			apply = true
		case arg == "--yes":
			yes = true
		case arg == "-h" || arg == "--help":
			fmt.Println("uso: b3l0w install system [--profile=desktop,dev] [--file=ruta] [--dotfiles=dir] [--apply] [--yes]")
			return 0
		case arg == "--profile" || strings.HasPrefix(arg, "--profile="):
			v, ok := value()
			if !ok {
				cli.Fail("--profile necesita un valor")
				return 1
			}
			profiles = strings.Split(v, ",")
		case arg == "--file" || strings.HasPrefix(arg, "--file="):
			v, ok := value()
			if !ok {
				cli.Fail("--file necesita un valor")
				return 1
			}
			file = v
		case arg == "--dotfiles" || strings.HasPrefix(arg, "--dotfiles="):
			v, ok := value()
			if !ok {
				cli.Fail("--dotfiles necesita un valor")
				return 1
			}
			dotfiles = v
		default:
			cli.Fail(fmt.Sprintf("opción desconocida: %s", arg))
			return 1
		}
	}

	var def *systemdef.Definition
	var err error
	if file != "" {
		def, err = systemdef.Load(file)
	} else {
		def, err = systemdef.LoadRemote()
	}
	if err != nil {
		cli.Fail(fmt.Sprintf("no se pudo leer la definición: %v", err))
		return 1
	}

	profile, err := def.Merge(profiles)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}

	if dotfiles == "" {
		if dir, err := os.Getwd(); err == nil {
			dotfiles = dir
		}
	}

	plan := systeminstall.Build(profile, dotfiles)
	printPlan(profiles, plan)

	if !apply {
		fmt.Printf("\n  %s\n", cli.Dim("simulación: no se cambió nada. Para aplicarlo: b3l0w install system --apply"))
		return 0
	}

	cli.Step("Aplicando")
	if err := systeminstall.Apply(plan, systeminstall.Options{Yes: yes}); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok("sistema al día")
	return 0
}

func printPlan(profiles []string, plan *systeminstall.Plan) {
	cli.Step(fmt.Sprintf("Sistema · perfil %s", strings.Join(profiles, "+")))

	list := func(title string, items []string) {
		if len(items) == 0 {
			fmt.Printf("  %s %s: nada\n", "✓", title)
			return
		}
		fmt.Printf("  %s %s (%d): %s\n", "!", title, len(items), strings.Join(items, ", "))
	}

	list("paquetes", plan.Packages)
	list("AUR", plan.AUR)
	list("flatpak", plan.Flatpak)
	list("propios", plan.Own)
	list("servicios", plan.Services)
	list("grupos", plan.Groups)

	missing, sources := 0, 0
	for _, f := range plan.Files {
		switch {
		case !f.SrcExists:
			fmt.Printf("  %s dotfile sin origen: %s\n", "✗", f.Src)
			sources++
		case !f.DestExists:
			fmt.Printf("  %s dotfile a instalar: %s\n", "!", f.Dest)
			missing++
		}
	}
	if sources == 0 && missing == 0 && len(plan.Files) > 0 {
		fmt.Printf("  %s dotfiles: todos en su lugar\n", "✓")
	}
}

func programCommand(args []string) int {
	if len(args) == 0 {
		fmt.Println("uso: b3l0w program <add|remove> <nombre> [--source github:owner/repo]")
		return 1
	}

	action := args[0]
	name := ""
	source := ""
	asset := ""
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "--source=") {
			source = strings.TrimPrefix(arg, "--source=")
			continue
		}
		if strings.HasPrefix(arg, "--asset=") {
			asset = strings.TrimPrefix(arg, "--asset=")
			continue
		}
		if name == "" {
			name = arg
		}
	}
	if name == "" {
		cli.Fail("falta el nombre del programa")
		return 1
	}

	return withManifest(func(m *config.Manifest) int {
		switch action {
		case "add":
			if !m.AddProgram(config.Program{Name: name, Source: source, Asset: asset}) {
				cli.Warn(fmt.Sprintf("%s ya estaba registrado", name))
				return 0
			}
		case "remove":
			if !m.Remove(name) {
				cli.Fail(fmt.Sprintf("%s no está registrado", name))
				return 1
			}
		default:
			cli.Fail(fmt.Sprintf("acción desconocida: %s", action))
			return 1
		}

		if err := m.Save(cli.ManifestPath()); err != nil {
			cli.Fail(fmt.Sprintf("no se pudo guardar el manifiesto: %v", err))
			return 1
		}
		cli.Ok(fmt.Sprintf("%s: %s", action, name))
		return 0
	})
}

func systemCommand(args []string) int {
	if len(args) == 0 {
		fmt.Println("uso: b3l0w system diff|pull")
		return 0
	}
	switch args[0] {
	case "diff":
		remote, err := systemdef.RemoteBytes()
		if err != nil {
			cli.Fail(err.Error())
			return 1
		}
		local := systemdef.BuiltinBytes()
		if sha256.Sum256(remote) == sha256.Sum256(local) {
			cli.Ok("system.json: embed y remoto iguales")
			return 0
		}
		cli.Warn(fmt.Sprintf("system.json: embed %d bytes, remoto %d bytes (distintos)", len(local), len(remote)))
		cli.Warn("corré `b3l0w system pull` en el repo y reconstruí el binario")
		return 1
	case "pull":
		remote, err := systemdef.RemoteBytes()
		if err != nil {
			cli.Fail(err.Error())
			return 1
		}
		path := filepath.Join("internal", "systemdef", "system.json")
		if _, err := os.Stat(filepath.Dir(path)); err != nil {
			cli.Fail("system pull solo funciona dentro del repo b3l0w-cli")
			return 1
		}
		if err := os.WriteFile(path, remote, 0o644); err != nil {
			cli.Fail(err.Error())
			return 1
		}
		cli.Ok("system.json actualizado; reconstruí el binario")
		return 0
	default:
		cli.Fail("uso: b3l0w system diff|pull")
		return 1
	}
}

func checkCommand(args []string) int {
	keep := 20
	yes := false
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--keep="):
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--keep=")); err == nil && n > 0 {
				keep = n
			}
		case arg == "--yes" || arg == "-y":
			yes = true
		}
	}
	code := withManifest(func(m *config.Manifest) int {
		return update.Run(m, update.Options{Check: true})
	})
	if code != 0 {
		return code
	}
	return snapshots.Report(keep, yes)
}

func channelCommand(args []string) int {
	if len(args) == 0 {
		fmt.Printf("canal: %s\n", channel.Current())
		return 0
	}
	if err := channel.Set(args[0]); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok("canal: " + args[0])
	return 0
}

func pluginExecCommand(args []string) int {
	if len(args) == 0 {
		cli.Fail("falta el script: b3l0w plugin-exec <script> [args...]")
		return 1
	}
	if err := plugins.ExecScript(args[0], args[1:]); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

// serviceFlags son los flags de services que llevan valor.
var serviceFlags = map[string]bool{
	"--manifest=":  true,
	"--state-dir=": true,
	"--variant=":   true,
	"--service=":   true,
	"--tail=":      true,
}

// firstPositional devuelve el primer argumento que no es un flag. Los
// flags pueden ir antes o despues del nombre del servicio.
func firstPositional(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

// servicePaths resuelve las rutas del manifiesto y del estado, y separa
// los flags de los argumentos. Los flags con valor se pueden escribir
// como "--flag=valor" o "--flag valor".
func servicePaths(args []string) (manifest, stateDir string, rest []string, dryRun bool) {
	home, _ := os.UserHomeDir()
	manifest = filepath.Join(home, "Documents/Github/b3l0w/services/services.json")
	stateDir = filepath.Join(home, ".local/state/b3l0w/services")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "--manifest="):
			manifest = strings.TrimPrefix(arg, "--manifest=")
		case strings.HasPrefix(arg, "--state-dir="):
			stateDir = strings.TrimPrefix(arg, "--state-dir=")
		case strings.HasPrefix(arg, "--variant="):
			rest = append(rest, arg)
		case strings.HasPrefix(arg, "--service="), strings.HasPrefix(arg, "--tail="):
			rest = append(rest, arg)
		case strings.HasPrefix(arg, "--"):
			rest = append(rest, arg)
		default:
			rest = append(rest, arg)
		}
	}
	return
}

// servicesInstall instala un servicio.
func servicesInstall(args []string) int {
	manifest, stateDir, rest, dryRun := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		cli.Fail("falta el servicio: b3l0w services install <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		cli.Fail("servicio desconocido: " + id)
		return 1
	}
	if svc.State == "planned" {
		cli.Fail(svc.ID + " todavía no está listo")
		return 1
	}
	variant := ""
	for _, arg := range rest {
		switch {
		case arg == "--yes" || arg == "-y":
			// el install no pregunta nada
		case strings.HasPrefix(arg, "--variant="):
			variant = strings.TrimPrefix(arg, "--variant=")
		case strings.HasPrefix(arg, "-"):
			cli.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	state, err := in.Install(*svc, variant, dryRun)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if state == nil || dryRun {
		return 0
	}
	cli.Ok(fmt.Sprintf("%s instalado (%s, %s)", state.ID, state.Variant, state.Health))
	if state.Provider != "" {
		fmt.Printf("  provider: %s\n", state.Provider)
	}
	if svc.Models != "" {
		fmt.Printf("  modelos: %s\n", cli.Dim("correr "+svc.Models))
	}
	return 0
}

// servicesUpdate actualiza los servicios instalados.
func servicesUpdate(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	yes := false
	rebuild := true
	var only string
	for _, arg := range rest {
		switch {
		case arg == "--yes" || arg == "-y":
			yes = true
		case arg == "--no-rebuild":
			rebuild = false
		case strings.HasPrefix(arg, "--service="):
			only = strings.TrimPrefix(arg, "--service=")
		default:
			if !strings.HasPrefix(arg, "-") {
				only = arg
				continue
			}
			cli.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	states, _ := services.ListStates(stateDir)
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	if len(states) == 0 {
		cli.Warn("no hay servicios instalados")
		return 0
	}
	cli.Step("Actualizando servicios")
	for _, state := range states {
		if only != "" && state.ID != only {
			continue
		}
		svc := m.Find(state.ID)
		if svc == nil {
			continue
		}
		newState, err := in.Update(*svc, state, rebuild, yes)
		if err != nil {
			cli.Fail(fmt.Sprintf("%s: %v", state.ID, err))
			continue
		}
		if newState.UpdatedAt != state.UpdatedAt {
			cli.Ok(fmt.Sprintf("%s actualizado", state.ID))
		}
	}
	return 0
}

// servicesStatus muestra el estado de los servicios instalados.
func servicesStatus(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	if len(states) == 0 {
		cli.Warn("no hay servicios instalados")
		return 0
	}
	docker := services.NewDocker()
	cli.Step("Estado de los servicios")
	for _, state := range states {
		svc := m.Find(state.ID)
		name := state.ID
		if svc != nil {
			name = svc.Name
		}
		fmt.Printf("  %-10s %-8s %-10s %s\n", state.ID, state.Variant, state.Health, cli.Dim(name))
		if state.Provider != "" {
			fmt.Printf("  %-10s provider: %s\n", "", state.Provider)
		}
		if svc == nil || svc.Compose == "" {
			continue
		}
		variant := ""
		if state != nil {
			variant = state.Variant
		}
		dir := serviceDir(*svc)
		ps, err := docker.PS(dir, services.ComposeFiles(*svc, variant))
		if err != nil {
			continue
		}
		for _, p := range ps {
			fmt.Printf("  %-10s %-12s %s\n", "", p.Name, cli.Dim(p.State+" "+p.Health))
		}
	}
	return 0
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func repoName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

func servicesCommand(args []string) int {
	if len(args) == 0 {
		return servicesList(args)
	}
	switch args[0] {
	case "list":
		return servicesList(args[1:])
	case "hardware":
		return servicesHardware(args[1:])
	case "images":
		return servicesImages(args[1:])
	case "sync":
		return servicesSync(args[1:])
	case "install":
		return servicesInstall(args[1:])
	case "update":
		return servicesUpdate(args[1:])
	case "status":
		return servicesStatus(args[1:])
	case "doctor":
		return servicesDoctor(args[1:])
	case "logs":
		return servicesLogs(args[1:])
	case "restart":
		return servicesSimple(args[1:], "restart")
	case "stop":
		return servicesSimple(args[1:], "stop")
	case "remove":
		return servicesRemove(args[1:])
	case "variant":
		return servicesVariant(args[1:])
	case "rollback":
		return servicesRollback(args[1:])
	case "prune":
		return servicesPrune(args[1:])
	}
	cli.Fail("uso: b3l0w services [list|hardware|images|sync|install|update|status|doctor|logs|restart|stop|remove|variant|rollback|prune]")
	return 1
}

// servicesRollback vuelve a la imagen anterior de un servicio.
func servicesRollback(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		cli.Fail("falta el servicio: b3l0w services rollback <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		cli.Fail("servicio desconocido: " + id)
		return 1
	}
	state, err := services.LoadState(stateDir, svc.ID)
	if err != nil {
		cli.Fail(svc.ID + " no está instalado")
		return 1
	}
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	newState, err := in.Rollback(*svc, state)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%s volvió a %s (%s)", newState.ID, newState.Tag, newState.Health))
	return 0
}

// servicesPrune borra las imágenes colgantes del registry local.
func servicesPrune(args []string) int {
	out, err := services.NewDocker().PruneImages()
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if out == "" {
		cli.Ok("no hay imágenes colgantes")
		return 0
	}
	fmt.Println(out)
	return 0
}

// servicesDoctor chequea que la máquina pueda correr servicios.
func servicesDoctor(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	docker := services.NewDocker()
	res := docker.Doctor(m, services.DetectHardware(), states)
	if res.Errors > 0 {
		return 1
	}
	return 0
}

// servicesLogs muestra las últimas líneas de un servicio.
func servicesLogs(args []string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		cli.Fail("falta el servicio: b3l0w services logs <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		cli.Fail("servicio desconocido: " + id)
		return 1
	}
	tail := 100
	service := ""
	for _, arg := range rest[1:] {
		switch {
		case strings.HasPrefix(arg, "--tail="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--tail="))
			if err != nil || n < 1 {
				cli.Fail("--tail debe ser un entero positivo")
				return 1
			}
			tail = n
		case strings.HasPrefix(arg, "--"):
			cli.Fail("flag desconocido: " + arg)
			return 1
		default:
			service = arg
		}
	}
	dir := serviceDir(*svc)
	out, err := services.NewDocker().Logs(dir, services.ComposeFiles(*svc, ""), service, tail)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	fmt.Print(out)
	return 0
}

// servicesSimple ejecuta restart o stop.
func servicesSimple(args []string, action string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if len(rest) == 0 {
		cli.Fail("falta el servicio: b3l0w services " + action + " <id>")
		return 1
	}
	svc := m.Find(rest[0])
	if svc == nil {
		cli.Fail("servicio desconocido: " + rest[0])
		return 1
	}
	dir := serviceDir(*svc)
	files := services.ComposeFiles(*svc, "")
	docker := services.NewDocker()
	var err2 error
	if action == "restart" {
		err2 = docker.Restart(dir, files)
	} else {
		err2 = docker.Stop(dir, files)
	}
	if err2 != nil {
		cli.Fail(err2.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%s: %s", svc.ID, action))
	return 0
}

// servicesRemove baja el stack. --purge borra los volúmenes.
func servicesRemove(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		cli.Fail("falta el servicio: b3l0w services remove <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		cli.Fail("servicio desconocido: " + id)
		return 1
	}
	purge := false
	for _, arg := range rest[1:] {
		switch {
		case arg == "--purge":
			purge = true
		case strings.HasPrefix(arg, "--"):
			cli.Fail("flag desconocido: " + arg)
			return 1
		default:
			cli.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	dir := serviceDir(*svc)
	files := services.ComposeFiles(*svc, "")
	if err := services.NewDocker().Remove(dir, files, purge); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	os.Remove(filepath.Join(stateDir, svc.ID+".json"))
	if purge {
		cli.Ok(fmt.Sprintf("%s removido con sus volúmenes", svc.ID))
	} else {
		cli.Ok(fmt.Sprintf("%s removido (los volúmenes quedan)", svc.ID))
	}
	return 0
}

// servicesVariant cambia la variante de un servicio instalado.
func servicesVariant(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if len(rest) < 2 {
		cli.Fail("uso: b3l0w services variant <id> <variante>")
		return 1
	}
	svc := m.Find(firstPositional(rest))
	if svc == nil {
		cli.Fail("servicio desconocido: " + firstPositional(rest))
		return 1
	}
	want := ""
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") && arg != firstPositional(rest) {
			want = arg
			break
		}
	}
	state, err := services.LoadState(stateDir, svc.ID)
	if err != nil {
		cli.Fail(svc.ID + " no está instalado")
		return 1
	}
	tags, err := services.NewRegistry("").Tags(svc.Image)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	published := services.AvailableVariants(tags, svc.Variants)
	found := false
	for _, v := range published {
		if v == want {
			found = true
			break
		}
	}
	if !found {
		cli.Fail(fmt.Sprintf("%s: la variante %q no está publicada (hay: %s)", svc.ID, want, strings.Join(published, ", ")))
		return 1
	}
	tag := services.TagFor(*svc, want, tags)
	image := svc.Image + ":" + tag
	cli.Step(fmt.Sprintf("%s → %s", svc.ID, want))
	fmt.Printf("  imagen: %s\n", image)
	fmt.Println("  → pull y recreate del stack")
	if err := services.NewDocker().Pull(svc.Image, tag); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	digest, _ := services.NewDocker().ImageDigest(image)
	state.Previous = state.Image
	state.PreviousTag = state.Tag
	state.Variant = want
	state.Image = image
	state.Tag = tag
	state.Digest = digest
	state.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := state.Save(stateDir); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%s ahora usa %s (%s)", svc.ID, want, tag))
	return 0
}

// serviceDir devuelve el directorio del repo de un servicio.
func serviceDir(svc services.Service) string {
	home := homeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Documents", "Github", repoName(svc.Repo))
}

func servicesSync(args []string) int {
	manifest, _, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	changed, err := services.Sync(m, manifest, services.NewRegistry(""))
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if changed == 0 {
		cli.Ok("manifiesto al día")
		return 0
	}
	cli.Ok(fmt.Sprintf("manifiesto actualizado (%d servicios)", changed))
	return 0
}

func servicesList(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	byID := make(map[string]*services.State, len(states))
	for _, s := range states {
		byID[s.ID] = s
	}
	reg := services.NewRegistry("")
	st := services.CheckRegistry(m, reg)
	services.RenderList(m, byID, st, services.DetectHardware())
	return 0
}

func servicesHardware(args []string) int {
	manifest, _, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	st := services.CheckRegistry(m, services.NewRegistry(""))
	services.RenderHardware(services.DetectHardware(), m, st)
	return 0
}

func servicesImages(args []string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	reg := services.NewRegistry("")
	for i := range m.Services {
		svc := &m.Services[i]
		if len(rest) > 0 && svc.ID != rest[0] {
			continue
		}
		if svc.Image == "" {
			continue
		}
		cli.Step(svc.ID + " · " + svc.Image)
		tags, err := reg.Tags(svc.Image)
		if err != nil {
			cli.Warn(err.Error())
			continue
		}
		for _, t := range tags {
			if strings.HasPrefix(t, "buildcache-") {
				continue
			}
			fmt.Printf("  %s\n", t)
		}
	}
	return 0
}

func pluginsCommand(args []string) int {
	if len(args) == 0 {
		return pluginsExtensions()
	}
	switch args[0] {
	case "sync":
		cli.Fail("plugins sync fue eliminado; usá b3l0w plugins build")
		return 1
	case "build":
		return pluginsBuild(args[1:])
	case "release":
		return pluginsRelease(args[1:])
	case "enable":
		return pluginsEnable(args[1:])
	case "disable":
		return pluginsDisable(args[1:])
	case "install":
		return pluginsInstall(args[1:])
	case "update":
		return pluginsUpdate(args[1:])
	case "list":
		return pluginsList(args[1:])
	case "remove":
		return pluginsRemove(args[1:])
	case "info":
		return pluginsInfo(args[1:])
	case "search":
		return pluginsSearch(args[1:])
	}
	cli.Fail("uso: b3l0w plugins [build|release|enable|disable|install|update|list|remove|info|search]")
	return 1
}

// pluginsExtensions lista los binarios b3l0w-* del directorio privado.
func pluginsExtensions() int {
	names := plugins.Discover()
	cli.Step("Extensiones")
	if len(names) == 0 {
		fmt.Printf("  (ninguna en %s)\n", cli.PluginsDir())
		return 0
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  b3l0w-%s\n", name)
	}
	return 0
}

// pluginPaths resuelve las rutas y separa las banderas de los argumentos.
func pluginPaths(args []string) (src, registry, manifest, noctalia string, rest []string, dryRun bool) {
	home, _ := os.UserHomeDir()
	src = filepath.Join(home, "Documents/Github")
	registry = filepath.Join(src, "b3l0w-plugins")
	manifest = filepath.Join(src, "b3l0w", "plugins", "plugins.json")
	noctalia = filepath.Join(home, ".local", "share", "noctalia", "plugins")

	for _, arg := range args {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "--src="):
			src = strings.TrimPrefix(arg, "--src=")
		case strings.HasPrefix(arg, "--registry="):
			registry = strings.TrimPrefix(arg, "--registry=")
		case strings.HasPrefix(arg, "--manifest="):
			manifest = strings.TrimPrefix(arg, "--manifest=")
		case strings.HasPrefix(arg, "--plugins-dir="):
			noctalia = strings.TrimPrefix(arg, "--plugins-dir=")
		default:
			rest = append(rest, arg)
		}
	}
	return
}

func loadPluginManifest(path string) (*plugins.Manifest, int) {
	m, err := plugins.LoadManifest(path)
	if err != nil {
		cli.Fail(fmt.Sprintf("no se pudo leer %s: %v", path, err))
		return nil, 1
	}
	return m, 0
}

func pluginsBuild(args []string) int {
	_, registry, manifest, _, rest, _ := pluginPaths(args)
	check := false
	for _, arg := range rest {
		if arg == "--check" {
			check = true
		}
	}
	entries, err := plugins.Collect(registry)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	if check {
		ok, err := plugins.CheckCatalog(registry, entries)
		if err != nil {
			cli.Fail(err.Error())
			return 1
		}
		if !ok {
			cli.Fail("catalog.toml desactualizado; corré b3l0w plugins build")
			return 1
		}
		cli.Ok("catalog.toml al día")
		return 0
	}
	if err := plugins.Build(registry, manifest, entries); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%d plugins: catalog.toml y plugins.json actualizados", len(entries)))
	return 0
}

func pluginsRelease(args []string) int {
	_, registry, manifest, _, rest, dryRun := pluginPaths(args)
	push := false
	yes := false
	targetAPI := 0
	var slugs []string
	for _, arg := range rest {
		switch {
		case arg == "--push":
			push = true
		case arg == "--yes":
			yes = true
		case strings.HasPrefix(arg, "--api="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--api="))
			if err != nil || n < 1 {
				cli.Fail("--api debe ser un entero positivo")
				return 1
			}
			targetAPI = n
		case strings.HasPrefix(arg, "--plugin="):
			slugs = append(slugs, strings.TrimPrefix(arg, "--plugin="))
		case strings.HasPrefix(arg, "--"):
			cli.Fail("flag desconocido: " + arg)
			return 1
		default:
			slugs = append(slugs, arg)
		}
	}
	if !yes && !dryRun && !cli.Confirm("Release plugins a main? [Y/n]", yes, true) {
		return 0
	}
	if err := plugins.Release(plugins.ReleaseOptions{
		RepoDir:      registry,
		ManifestPath: manifest,
		Slugs:        slugs,
		TargetAPI:    targetAPI,
		DryRun:       dryRun,
		Push:         push,
	}); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsEnable(args []string) int {
	return pluginsToggle(args, true)
}

func pluginsDisable(args []string) int {
	return pluginsToggle(args, false)
}

func pluginsToggle(args []string, enable bool) int {
	_, _, manifest, _, rest, dryRun := pluginPaths(args)
	if len(rest) == 0 {
		cli.Fail("falta el id: b3l0w plugins enable|disable <autor/nombre>")
		return 1
	}
	id := rest[0]
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	found := false
	for _, entry := range m.Plugins {
		if entry.ID == id {
			found = true
			break
		}
	}
	if !found {
		cli.Fail("plugin no encontrado: " + id)
		return 1
	}
	action := "disable"
	if enable {
		action = "enable"
	}
	if dryRun {
		fmt.Printf("  → %s %s\n", action, id)
		return 0
	}
	if !cli.HaveCommand("noctalia") {
		cli.Fail("noctalia no está en PATH")
		return 1
	}
	out, err := cli.RunCombined("noctalia", "msg", "plugins", action, id)
	if err != nil {
		cli.Fail(fmt.Sprintf("noctalia %s: %v: %s", action, err, out))
		return 1
	}
	cli.Ok(fmt.Sprintf("%s %s", action, id))
	return 0
}

func pluginsInstall(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	id := ""
	if len(rest) > 0 {
		id = rest[0]
	}
	if err := plugins.Install(m, id, noctalia, dryRun); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsUpdate(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	id := ""
	if len(rest) > 0 {
		id = rest[0]
	}
	if err := plugins.Update(m, id, noctalia, dryRun); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

func setupCommand() int {
	cli.Step("Setup de b3l0w")
	if err := plugins.EnsureSource(); err != nil {
		cli.Warn("source: " + err.Error())
	} else {
		cli.Ok("source " + plugins.SourceName)
	}
	if plugins.HasSource(plugins.SourceName) {
		if err := plugins.UpdateSource(plugins.SourceName); err != nil {
			cli.Warn("plugins: " + err.Error())
		} else {
			cli.Ok("plugins actualizados")
		}
	}
	if names, err := plugins.Scripts(); err != nil {
		cli.Warn("backends: " + err.Error())
	} else {
		cli.Ok(fmt.Sprintf("%d backends embebidos", len(names)))
	}
	return withManifest(doctor.Run)
}

func pluginsInfo(args []string) int {
	_, _, manifest, noctalia, rest, _ := pluginPaths(args)
	if len(rest) == 0 {
		cli.Fail("falta el id: b3l0w plugins info <autor/nombre>")
		return 1
	}
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	for _, entry := range m.Plugins {
		if entry.ID != rest[0] {
			continue
		}
		cli.Step(entry.ID)
		fmt.Printf("  nombre:     %s\n", entry.Name)
		fmt.Printf("  versión:    %s\n", entry.Version)
		fmt.Printf("  plugin_api: %d\n", entry.PluginAPI)
		fmt.Printf("  repo:       %s\n", entry.Repo)
		fmt.Printf("  path:       %s\n", entry.Path)
		local := plugins.InstalledVersion(noctalia, entry)
		if local == "" {
			fmt.Println("  instalado:  no")
		} else {
			state := "disabled"
			if plugins.Enabled(entry.ID) {
				state = "enabled"
			}
			fmt.Printf("  instalado:  %s (%s, %s)\n", local, plugins.Origin(noctalia, entry), state)
		}
		return 0
	}
	cli.Fail("plugin no encontrado: " + rest[0])
	return 1
}

func pluginsSearch(args []string) int {
	_, _, manifest, _, rest, _ := pluginPaths(args)
	q := strings.ToLower(strings.Join(rest, " "))
	if q == "" {
		cli.Fail("falta la búsqueda: b3l0w plugins search <texto>")
		return 1
	}
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	found := 0
	for _, entry := range m.Plugins {
		if strings.Contains(strings.ToLower(entry.ID+" "+entry.Name), q) {
			fmt.Printf("  %-24s %s\n", entry.ID, entry.Version)
			found++
		}
	}
	if found == 0 {
		cli.Warn("sin resultados")
	}
	return 0
}

func pluginsRemove(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	if len(rest) == 0 {
		cli.Fail("falta el id: b3l0w plugins remove <autor/nombre>")
		return 1
	}
	if err := plugins.Remove(manifest, rest[0], noctalia, dryRun); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsList(args []string) int {
	_, _, manifest, noctalia, _, _ := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	cli.Step("Plugins del registry")
	for _, entry := range m.Plugins {
		local := plugins.InstalledVersion(noctalia, entry)
		origin := plugins.Origin(noctalia, entry)
		state := "disabled"
		if plugins.Enabled(entry.ID) {
			state = "enabled"
		}
		switch {
		case local == "":
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, cli.Dim("no instalado"))
		case local != entry.Version:
			// La versión instalada difiere de la del catálogo: Noctalia
			// chose una revisión compatible con su rango de API.
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, local, cli.Dim(fmt.Sprintf("%s/%s (catálogo %s)", origin, state, entry.Version)))
		default:
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, cli.Dim(fmt.Sprintf("ok %s/%s", origin, state)))
		}
	}
	return 0
}

func usage() {
	fmt.Printf(`b3l0w %s — control del sistema

uso: b3l0w <comando> [opciones]

  status                 sistema base (novedades) + programas + plugins
  list                   programas registrados, versión y binario
  update                 CLI, sistema, programas, plugins y snapshots
  update --system        solo el sistema base (cachy-update / arch-update)
  update --programs      solo los programas propios
  update --plugins       solo plugins y backends
  update --dotfiles      aplica los TOML del repo sin pisar cambios locales
  update --adopt         registra los TOML actuales como base (primera vez)
  update --desktop       muestra el comando de pacman sugerido
  update --check         reporta, no toca nada
  update --yes           no pregunta
  update --keep=N        snapshots de Limine a conservar (default 20)
  update --self          actualiza el propio b3l0w
  check                  estado + snapshots de Limine (pregunta si limpiar)
  channel [stable|edge]  canal del escritorio
  install <nombre>       baja el programa del release a ~/.local/bin
                         (--source=… --asset=… lo registra en un paso)
  uninstall <nombre>     borra el binario y lo saca del manifiesto
  install system         compara la definición y aplica lo que falta
                         (--profile=… --file=… --dotfiles=… --apply --yes)
  system diff            compara el system.json embebido con el remoto
  system pull            actualiza el system.json del repo (dev)
  doctor                 dependencias, programas, scripts y plugins
  program add <nombre>   registra un programa (--source=… --asset=…)
  program remove <nom>   lo saca del manifiesto
  plugins                lista las extensiones binarias instaladas
  plugins list           lista los plugins de Noctalia vs registry
  plugins build         regenera catalog.toml y plugins.json
  plugins release       copia producción de dev a main (sin tests/)
  plugins release --api=N  exige que los plugins no superen ese plugin_api
  plugins enable <id>   habilita un plugin en Noctalia
  plugins disable <id>  deshabilita un plugin en Noctalia
  plugins install|update [id]  instala/actualiza plugins
  plugins remove <id>    borra local o deshabilita si viene del source
  plugins info <id>      muestra versión, API, repo y si está instalado
  plugins search <texto> busca en el registry
  plugin-exec <script>   ejecuta un backend embebido
  setup                  source + plugins + backends + doctor
  version                imprime la versión

Las extensiones son b3l0w-<nombre> en %s
(no se ejecutan desde el PATH).
`, version.Version, cli.PluginsDir())
}
