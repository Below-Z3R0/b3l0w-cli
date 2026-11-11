package main

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/doctor"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// Los comandos sueltos: el listado, la ejecucion de un plugin y el
// setup inicial.
func list() int {
	code := withManifest(func(m *config.Manifest) int {
		output.Step("Programas registrados")
		if len(m.Programs) == 0 {
			fmt.Println("  (ninguno)")
			return 0
		}
		for _, status := range programs.Statuses(m) {
			switch {
			case !status.Installed:
				fmt.Printf("  %-14s %s\n", status.Name, output.Dim("no instalado"))
			case status.Err != "":
				fmt.Printf("  %-14s %s\n", status.Name, output.Dim("error: "+status.Err))
			default:
				fmt.Printf("  %-14s %-8s %s\n", status.Name, status.Version, output.Dim(status.Bin))
			}
		}
		return 0
	})
	if code != 0 {
		return code
	}
	// El error del manifiesto de plugins tiene que verse en el exit
	// code: si no, un script no se entera de que algo esta roto.
	return pluginsList(nil)
}

func pluginExecCommand(args []string) int {
	if len(args) == 0 {
		output.Fail("falta el script: b3l0w plugin-exec <script> [args...]")
		return 1
	}
	if err := plugins.ExecScript(args[0], args[1:]); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

func setupCommand() int {
	output.Step("Setup de b3l0w")
	ensureRepo()
	ensureKeyring()
	avisarBinarioViejo()
	ensureLauncher()
	if err := plugins.EnsureSource(); err != nil {
		output.Warn("source: " + err.Error())
	} else {
		output.Ok("source " + plugins.SourceName)
	}
	if plugins.HasSource(plugins.SourceName) {
		if err := plugins.UpdateSource(plugins.SourceName); err != nil {
			output.Warn("plugins: " + err.Error())
		} else {
			output.Ok("plugins actualizados")
		}
	}
	if names, err := plugins.Scripts(); err != nil {
		output.Warn("backends: " + err.Error())
	} else {
		output.Ok(fmt.Sprintf("%d backends embebidos", len(names)))
	}
	return withManifest(doctor.Run)
}
