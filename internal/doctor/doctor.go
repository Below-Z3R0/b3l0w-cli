// Package doctor chequea que la máquina pueda hacer su trabajo.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/system"
)

// Run devuelve 0 si no hay errores (los avisos no cuentan).
func Run(m *config.Manifest) int {
	cli.Step("Chequeos")

	errors := 0

	// El actualizador del sistema: sin él, media herramienta no sirve.
	if bin := system.Binary(); bin != "" {
		cli.Ok(fmt.Sprintf("actualizador del sistema: %s", bin))
	} else {
		cli.Fail("no hay cachy-update ni arch-update")
		errors++
	}

	// pacman: lo necesita el chequeo de "es de un paquete".
	if path := cli.CommandPath("pacman"); path != "" {
		cli.Ok(fmt.Sprintf("pacman: %s", path))
	} else {
		cli.Fail("no hay pacman: esto no es un sistema Arch")
		errors++
	}

	// Terminal: el update del sistema la necesita.
	if cli.IsTTY() {
		cli.Ok("terminal interactiva")
	} else {
		cli.Warn("sin terminal: b3l0w update solo podrá actualizar programas")
	}

	// Programas del manifiesto.
	statuses := programs.Statuses(m)
	if len(statuses) == 0 {
		cli.Warn("el manifiesto no tiene programas")
	}
	for _, status := range statuses {
		switch {
		case !status.Installed:
			cli.Warn(fmt.Sprintf("%s: no instalado", status.Name))
		case status.Err != "":
			cli.Fail(fmt.Sprintf("%s: %s", status.Name, status.Err))
			errors++
		default:
			cli.Ok(fmt.Sprintf("%s: %s", status.Name, status.Version))
		}
	}

	// Directorio de extensiones: si existe, tiene que ser privado.
	dir := cli.PluginsDir()
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		cli.Warn(fmt.Sprintf("plugins: %s todavía no existe", dir))
	case err != nil:
		cli.Fail(fmt.Sprintf("plugins: %v", err))
		errors++
	case info.Mode().Perm()&0o077 != 0:
		cli.Fail(fmt.Sprintf("plugins: %s es accesible por otros (modo %o)", dir, info.Mode().Perm()))
		errors++
	default:
		cli.Ok(fmt.Sprintf("plugins: %s (privado)", dir))
	}

	// Scripts/backends de plugins: si falta uno, el plugin no funciona.
	if names, err := plugins.Scripts(); err == nil {
		for _, name := range names {
			path := filepath.Join(plugins.DefaultScriptsDest, name)
			if _, err := os.Stat(path); err != nil {
				cli.Warn(fmt.Sprintf("script de plugin: falta %s (b3l0w plugins scripts)", path))
			} else {
				cli.Ok(fmt.Sprintf("script de plugin: %s", path))
			}
		}
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
					cli.Warn(fmt.Sprintf("%s: falta dependencia %s", id, dep))
				}
			}
		}
	}

	if errors > 0 {
		cli.Fail(fmt.Sprintf("%d chequeos fallaron", errors))
		return 1
	}
	return 0
}

// hasDependency busca un comando en PATH o un paquete instalado.
func hasDependency(name string) bool {
	if cli.CommandPath(name) != "" {
		return true
	}
	return exec.Command("pacman", "-Q", name).Run() == nil
}
