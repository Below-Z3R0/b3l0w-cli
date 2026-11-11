package main

import (
	"fmt"
	"os"

	"github.com/Below-Z3R0/b3l0w-cli/internal/apps"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/doctor"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/update"
	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
	"github.com/Below-Z3R0/b3l0w-kit/output"
	"github.com/Below-Z3R0/b3l0w-kit/paths"
)

// b3l0w es el CLI de control del sistema: versiones, estado, updates y
// extensiones de los programas propios.

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
	case "extensions":
		return pluginsExtensions()
	case "sync":
		return syncCommand(rest)
	case "apps":
		return apps.Run(rest)
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
			output.Fail(err.Error())
			return 1
		}
		return 0
	}

	output.Fail(fmt.Sprintf("comando desconocido: %s", command))
	fmt.Println()
	usage()
	return 1
}

// firstPositional devuelve el primer argumento que no es un flag. Los
// flags pueden ir antes o despues del nombre del servicio.
//
// Delega en el parser comun: la regla de "que es un posicional" esta
// escrita una sola vez.
func firstPositional(args []string) string {
	return parsearFlags(args).libre()
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func usage() {
	fmt.Printf(`b3l0w %s — control del sistema

uso: b3l0w <comando> [opciones]

Cada categoría lista sin subcomando: b3l0w plugins, b3l0w services,
b3l0w program, b3l0w system, b3l0w extensions.

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
  extensions             lista las extensiones binarias del CLI
  sync [cat] [--check]   refresca las DB locales del registry
  apps [-S|-Syu|-R|-Q]   wrapper de pacman con freno de partial upgrade
  plugins                lista los plugins de Noctalia del registry
  plugins list           lo mismo (alias explícito)
  plugins build         regenera catalog.toml y plugins.json
  plugins release       copia producción de dev a main (sin tests/)
  plugins release --api=N  exige que los plugins no superen ese plugin_api
  plugins enable <id>   habilita un plugin en Noctalia
  plugins disable <id>  deshabilita un plugin en Noctalia
  plugins install|update [id]  instala/actualiza plugins
  plugins remove <id>    borra local o deshabilita si viene del source
  plugins info <id>      muestra versión, API, repo y si está instalado
  plugins search <texto> busca en el registry
  services               lista los servicios Docker del sistema
  services list          lo mismo (alias explícito)
  plugin-exec <script>   ejecuta un backend embebido
  setup                  source + plugins + backends + doctor
  version                imprime la versión

Las extensiones son b3l0w-<nombre> en %s
(no se ejecutan desde el PATH).
`, version.Version, paths.PluginsDir())
}
