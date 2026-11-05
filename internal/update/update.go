// Package update orquesta la actualización: primero el sistema, después
// los programas propios.
package update

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w/internal/cli"
	"github.com/Below-Z3R0/b3l0w/internal/config"
	"github.com/Below-Z3R0/b3l0w/internal/programs"
	"github.com/Below-Z3R0/b3l0w/internal/selfupdate"
	"github.com/Below-Z3R0/b3l0w/internal/system"
)

// Options son las banderas de `b3l0w update`.
type Options struct {
	System   bool
	Programs bool
	Check    bool
	Self     bool
	Yes      bool
}

// Run ejecuta el update y devuelve el código de salida.
func Run(m *config.Manifest, opts Options) int {
	if opts.Check {
		return check(m)
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
	return 0
}
