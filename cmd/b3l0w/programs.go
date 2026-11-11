package main

import (
	"fmt"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
)

// Los comandos de programas: instalar, desinstalar y registrar los
// programas propios.
// installCommand baja el programa del último release y lo deja en
// ~/.local/bin. No toca su configuración: eso lo hace el propio
// programa (por ejemplo `kitty-shapes install`).
func installCommand(args []string) int {
	if len(args) == 0 {
		output.Fail("falta el programa: b3l0w install <nombre>  (o `b3l0w install system`)")
		return 1
	}
	if args[0] == "system" {
		return installSystemCommand(args[1:])
	}
	name := args[0]
	source, asset := "", ""
	insecure := false
	dryRun := false
	for _, arg := range args[1:] {
		switch {
		case strings.HasPrefix(arg, "--source="):
			source = strings.TrimPrefix(arg, "--source=")
		case strings.HasPrefix(arg, "--asset="):
			asset = strings.TrimPrefix(arg, "--asset=")
		case arg == "--insecure":
			insecure = true
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "--"):
			// Un flag que no existe se ignoraba en silencio: el
			// usuario creia que --dry-run hacia algo y no hacia nada.
			output.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	return withManifest(func(m *config.Manifest) int {
		program := m.Get(name)
		if program == nil {
			if source == "" {
				output.Fail(fmt.Sprintf("%s no está registrado (b3l0w install %s --source=github:owner/repo --asset=archivo)", name, name))
				return 1
			}
			m.AddProgram(config.Program{Name: name, Source: source, Asset: asset})
			if err := m.Save(paths.ManifestPath()); err != nil {
				output.Fail(fmt.Sprintf("no se pudo guardar el manifiesto: %v", err))
				return 1
			}
			program = m.Get(name)
			output.Ok(fmt.Sprintf("%s registrado", name))
		}
		if dryRun {
			dest, _ := programs.InstallPath(name)
			output.Step(fmt.Sprintf("%s: se instalaria en %s", name, dest))
			if source != "" {
				output.Step(fmt.Sprintf("  origen: %s", source))
			}
			output.Ok("dry-run: no se instaló nada")
			return 0
		}
		output.Step(fmt.Sprintf("Instalando %s", name))
		if insecure {
			output.Warn("--insecure: se salta la verificación de firma")
		}
		version, err := programs.InstallVerified(*program, !insecure)
		if err != nil {
			output.Fail(err.Error())
			return 1
		}
		dest, _ := programs.InstallPath(name)
		output.Ok(fmt.Sprintf("%s %s en %s", name, version, dest))
		return 0
	})
}

func uninstallCommand(args []string) int {
	if len(args) == 0 {
		output.Fail("falta el programa: b3l0w uninstall <nombre>")
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
			output.Fail(err.Error())
			return 1
		}
		output.Ok(fmt.Sprintf("%s desinstalado (%s)", name, path))
		return 0
	})
}

func programCommand(args []string) int {
	if len(args) == 0 {
		return list()
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
		if strings.HasPrefix(arg, "-") {
			// Un flag no es un nombre: antes "--unknown-flag myprog"
			// registraba un programa llamado "--unknown-flag".
			output.Fail("flag desconocido: " + arg)
			return 1
		}
		if name == "" {
			name = arg
		}
	}
	if name == "" {
		output.Fail("falta el nombre del programa")
		return 1
	}

	return withManifest(func(m *config.Manifest) int {
		switch action {
		case "add":
			if !m.AddProgram(config.Program{Name: name, Source: source, Asset: asset}) {
				output.Warn(fmt.Sprintf("%s ya estaba registrado", name))
				return 0
			}
		case "remove":
			if !m.Remove(name) {
				output.Fail(fmt.Sprintf("%s no está registrado", name))
				return 1
			}
		default:
			output.Fail(fmt.Sprintf("acción desconocida: %s", action))
			return 1
		}

		if err := m.Save(paths.ManifestPath()); err != nil {
			output.Fail(fmt.Sprintf("no se pudo guardar el manifiesto: %v", err))
			return 1
		}
		output.Ok(fmt.Sprintf("%s: %s", action, name))
		return 0
	})
}

// withManifest carga el manifiesto una vez y se lo pasa al comando.
func withManifest(fn func(*config.Manifest) int) int {
	manifest, err := config.Load(paths.ManifestPath())
	if err != nil {
		output.Fail(fmt.Sprintf("no se pudo leer %s: %v", paths.ManifestPath(), err))
		return 1
	}
	return fn(manifest)
}
