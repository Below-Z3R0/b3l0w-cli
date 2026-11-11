package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
	"github.com/Below-Z3R0/b3l0w-cli/internal/systeminstall"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
)

// Los comandos de sistema: el perfil de la distro, su diff contra la
// maquina y la aplicacion de los cambios.
func systemCommand(args []string) int {
	if len(args) == 0 {
		return systemList()
	}
	switch args[0] {
	case "diff":
		remote, err := systemdef.RemoteBytes()
		if err != nil {
			output.Fail(err.Error())
			return 1
		}
		local := systemdef.BuiltinBytes()
		if sha256.Sum256(remote) == sha256.Sum256(local) {
			output.Ok("system.json: embed y remoto iguales")
			return 0
		}
		output.Warn(fmt.Sprintf("system.json: embed %d bytes, remoto %d bytes (distintos)", len(local), len(remote)))
		output.Warn("corré `b3l0w system pull` en el repo y reconstruí el binario")
		return 1
	case "pull":
		remote, err := systemdef.RemoteBytes()
		if err != nil {
			output.Fail(err.Error())
			return 1
		}
		path := filepath.Join("internal", "systemdef", "system.json")
		if _, err := os.Stat(filepath.Dir(path)); err != nil {
			output.Fail("system pull solo funciona dentro del repo b3l0w-cli")
			return 1
		}
		if err := os.WriteFile(path, remote, 0o644); err != nil {
			output.Fail(err.Error())
			return 1
		}
		output.Ok("system.json actualizado; reconstruí el binario")
		return 0
	default:
		output.Fail("uso: b3l0w system diff|pull")
		return 1
	}
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
				output.Fail("--profile necesita un valor")
				return 1
			}
			profiles = strings.Split(v, ",")
		case arg == "--file" || strings.HasPrefix(arg, "--file="):
			v, ok := value()
			if !ok {
				output.Fail("--file necesita un valor")
				return 1
			}
			file = v
		case arg == "--dotfiles" || strings.HasPrefix(arg, "--dotfiles="):
			v, ok := value()
			if !ok {
				output.Fail("--dotfiles necesita un valor")
				return 1
			}
			dotfiles = v
		default:
			output.Fail(fmt.Sprintf("opción desconocida: %s", arg))
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
		output.Fail(fmt.Sprintf("no se pudo leer la definición: %v", err))
		return 1
	}

	profile, err := def.Merge(profiles)
	if err != nil {
		output.Fail(err.Error())
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
		fmt.Printf("\n  %s\n", output.Dim("simulación: no se cambió nada. Para aplicarlo: b3l0w install system --apply"))
		return 0
	}

	output.Step("Aplicando")
	if err := systeminstall.Apply(plan, systeminstall.Options{Yes: yes}); err != nil {
		output.Fail(err.Error())
		return 1
	}
	output.Ok("sistema al día")
	return 0
}

func printPlan(profiles []string, plan *systeminstall.Plan) {
	output.Step(fmt.Sprintf("Sistema · perfil %s", strings.Join(profiles, "+")))

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
