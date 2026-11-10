// Package doctor chequea que la máquina pueda hacer su trabajo.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/releases"
	"github.com/Below-Z3R0/b3l0w-cli/internal/system"

	"github.com/Below-Z3R0/b3l0w-kit/output"
	"github.com/Below-Z3R0/b3l0w-kit/paths"
	kitsystem "github.com/Below-Z3R0/b3l0w-kit/system"
)

// Run devuelve 0 si no hay errores (los avisos no cuentan).
func Run(m *config.Manifest) int {
	output.Step("Chequeos")

	errors := 0

	// El actualizador del sistema: sin él, media herramienta no sirve.
	if bin := system.Binary(); bin != "" {
		output.Ok(fmt.Sprintf("actualizador del sistema: %s", bin))
	} else {
		output.Fail("no hay cachy-update ni arch-update")
		errors++
	}

	// pacman: lo necesita el chequeo de "es de un paquete".
	if path := kitsystem.CommandPath("pacman"); path != "" {
		output.Ok(fmt.Sprintf("pacman: %s", path))
	} else {
		output.Fail("no hay pacman: esto no es un sistema Arch")
		errors++
	}

	// Terminal: el update del sistema la necesita.
	if kitsystem.IsTTY() {
		output.Ok("terminal interactiva")
	} else {
		output.Warn("sin terminal: b3l0w update solo podrá actualizar programas")
	}

	// Programas del manifiesto.
	statuses := programs.Statuses(m)
	if len(statuses) == 0 {
		output.Warn("el manifiesto no tiene programas")
	}
	for _, status := range statuses {
		switch {
		case !status.Installed:
			output.Warn(fmt.Sprintf("%s: no instalado", status.Name))
		case status.Err != "":
			output.Fail(fmt.Sprintf("%s: %s", status.Name, status.Err))
			errors++
		default:
			output.Ok(fmt.Sprintf("%s: %s", status.Name, status.Version))
		}
	}

	// Directorio de extensiones: si existe, tiene que ser privado.
	dir := paths.PluginsDir()
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		output.Warn(fmt.Sprintf("plugins: %s todavía no existe", dir))
	case err != nil:
		output.Fail(fmt.Sprintf("plugins: %v", err))
		errors++
	case info.Mode().Perm()&0o077 != 0:
		output.Fail(fmt.Sprintf("plugins: %s es accesible por otros (modo %o)", dir, info.Mode().Perm()))
		errors++
	default:
		output.Ok(fmt.Sprintf("plugins: %s (privado)", dir))
	}

	// Backends embebidos: viven en el binario, no hay copia que chequear.
	if names, err := plugins.Scripts(); err == nil {
		for _, name := range names {
			output.Ok(fmt.Sprintf("backend embebido: %s", name))
		}
	}

	// Noctalia y su source de plugins.
	if kitsystem.HaveCommand("noctalia") {
		if plugins.HasSource(plugins.SourceName) {
			output.Ok("noctalia: source " + plugins.SourceName + " presente")
		} else {
			output.Warn("noctalia: falta el source " + plugins.SourceName + " (b3l0w update --plugins)")
		}
		switch v := plugins.AutoUpdate(); v {
		case "none":
			output.Ok("plugins: auto-update off (lo controla b3l0w)")
		case "":
			output.Warn("plugins: no pude leer auto_update")
		default:
			output.Fail(fmt.Sprintf("plugins: auto_update=%q; poné \"none\" para que b3l0w controle las actualizaciones", v))
			errors++
		}
	} else {
		output.Warn("noctalia no está en PATH: los plugins no se actualizan")
	}

	// Dependencias declaradas por los plugins (gh, mpv, etc.).
	if deps, err := plugins.Dependencies(); err == nil {
		var ids []string
		for id := range deps {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			for _, dep := range deps[id] {
				if !hasDependency(dep) {
					output.Warn(fmt.Sprintf("%s: falta dependencia %s", id, dep))
				}
			}
		}
	}

	// Firmas de los releases: sin SHA256SUMS.asc, install y
	// update --self se niegan a instalar.
	errors += releases.Render(releases.Check(m))

	if errors > 0 {
		output.Fail(fmt.Sprintf("%d chequeos fallaron", errors))
		return 1
	}
	return 0
}

// hasDependency busca un comando en PATH o un paquete instalado.
func hasDependency(name string) bool {
	if kitsystem.CommandPath(name) != "" {
		return true
	}
	return exec.Command("pacman", "-Q", name).Run() == nil
}
