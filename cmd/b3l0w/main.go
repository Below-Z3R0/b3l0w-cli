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

	"github.com/Below-Z3R0/b3l0w-cli/internal/channel"
	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/doctor"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
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
	case "program":
		return programCommand(rest)
	case "plugins":
		return pluginsCommand(rest)
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
		case "--dry-run":
			opts.DryRun = true
		case "--force":
			opts.Force = true
		case "--adopt":
			opts.Adopt = true
		case "--self":
			opts.Self = true
		case "-h", "--help":
			fmt.Println("uso: b3l0w update [--self] [--system|--programs|--plugins|--dotfiles] [--check] [--yes] [--dry-run] [--force]")
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

func pluginsCommand(args []string) int {
	if len(args) == 0 {
		return pluginsExtensions()
	}
	switch args[0] {
	case "sync":
		return pluginsSync(args[1:])
	case "build":
		return pluginsBuild(args[1:])
	case "install":
		return pluginsInstall(args[1:])
	case "update":
		return pluginsUpdate(args[1:])
	case "list":
		return pluginsList(args[1:])
	case "scripts":
		return pluginsScripts(args[1:])
	case "remove":
		return pluginsRemove(args[1:])
	case "info":
		return pluginsInfo(args[1:])
	case "search":
		return pluginsSearch(args[1:])
	}
	cli.Fail("uso: b3l0w plugins [sync|build|install|update|list]")
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

func pluginsSync(args []string) int {
	src, registry, manifest, _, _, _ := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	if err := plugins.Sync(src, registry, manifest, m); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%d plugins sincronizados en %s", len(m.Plugins), registry))
	return 0
}

func pluginsBuild(args []string) int {
	src, registry, manifest, _, _, _ := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	if err := plugins.Build(registry, src, m); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("catalog.toml generado en %s", registry))
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
	if _, err := plugins.InstallScripts(""); err != nil {
		cli.Warn("backends: " + err.Error())
	} else {
		cli.Ok("backends instalados")
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
			fmt.Printf("  instalado:  %s\n", local)
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
	_, _, _, noctalia, rest, dryRun := pluginPaths(args)
	if len(rest) == 0 {
		cli.Fail("falta el id: b3l0w plugins remove <autor/nombre>")
		return 1
	}
	if err := plugins.Remove(plugins.DefaultManifestPath(), rest[0], noctalia, dryRun); err != nil {
		cli.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsScripts(args []string) int {
	dest := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--dest=") {
			dest = strings.TrimPrefix(arg, "--dest=")
		}
	}
	names, err := plugins.InstallScripts(dest)
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	for _, name := range names {
		cli.Ok(name)
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
		switch {
		case local == "":
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, cli.Dim("no instalado"))
		case local != entry.Version:
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, cli.Dim("instalado "+local))
		default:
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, cli.Dim("ok"))
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
  install system         compara la definición y aplica lo que falta
                         (--profile=… --file=… --dotfiles=… --apply --yes)
  system diff            compara el system.json embebido con el remoto
  system pull            actualiza el system.json del repo (dev)
  doctor                 dependencias, programas, scripts y plugins
  program add <nombre>   registra un programa (--source=… --asset=…)
  program remove <nom>   lo saca del manifiesto
  plugins                lista las extensiones binarias instaladas
  plugins list           lista los plugins de Noctalia vs registry
  plugins sync|build     herramientas de publicación del registry
  plugins install|update [id]  instala/actualiza plugins
  plugins remove <id>    borra local o deshabilita si viene del source
  plugins info <id>      muestra versión, API, repo y si está instalado
  plugins search <texto> busca en el registry
  plugins scripts        instala backends en /usr/local/bin (legacy)
  plugin-exec <script>   ejecuta un backend embebido
  setup                  source + plugins + backends + doctor
  version                imprime la versión

Las extensiones son b3l0w-<nombre> en %s
(no se ejecutan desde el PATH).
`, version.Version, cli.PluginsDir())
}
