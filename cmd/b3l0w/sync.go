package main

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/registry"
	"github.com/Below-Z3R0/b3l0w-cli/internal/services"
	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// syncCommand es el "pacman -Sy" del CLI: refresca las DB locales
// desde el repo de distribución.
//
// No toca pacman. La DB de pacman se sincroniza con `b3l0w apps -Syu`,
// porque dejarla sincronizada sin actualizar es un partial upgrade.
func syncCommand(args []string) int {
	check := false
	var kinds []registry.Kind

	for _, arg := range args {
		switch arg {
		case "--check":
			check = true
		case "all":
			kinds = nil
		case string(registry.System):
			kinds = append(kinds, registry.System)
		case string(registry.Services):
			kinds = append(kinds, registry.Services)
		case string(registry.Plugins):
			kinds = append(kinds, registry.Plugins)
		case "-h", "--help":
			syncUsage()
			return 0
		default:
			output.Fail(fmt.Sprintf("categoría desconocida: %s", arg))
			syncUsage()
			return 1
		}
	}
	if len(kinds) == 0 {
		kinds = registry.All()
	}

	if check {
		return syncCheck(kinds)
	}

	errores := 0
	output.Step("Sincronizando el registry")
	for _, kind := range kinds {
		st, err := registry.Sync(kind, embeddedFor(kind))
		if err != nil {
			output.Warn(err.Error())
			errores++
			continue
		}
		output.Ok(fmt.Sprintf("%s: %d bytes (%s)", kind, st.Size, st.Source))
	}
	if errores > 0 {
		return 1
	}
	return 0
}

// syncCheck muestra el estado local sin tocar la red.
func syncCheck(kinds []registry.Kind) int {
	output.Step("Registry")
	for _, kind := range kinds {
		st := registry.Stat(kind)
		switch st.Source {
		case registry.FromEmbed:
			output.Warn(fmt.Sprintf("%s: sin sincronizar (se usa la copia embebida)", kind))
		default:
			output.Ok(fmt.Sprintf("%s: %s, %d bytes (sincronizado %s)",
				kind, st.Source, st.Size, st.Updated.Format("2006-01-02 15:04")))
		}
	}
	return 0
}

func syncUsage() {
	fmt.Println("uso: b3l0w sync [system|services|plugins] [--check]")
	fmt.Println("  sin categoría sincroniza todas")
	fmt.Println("  --check solo muestra el estado local")
}

// embeddedFor devuelve la copia compilada de cada DB, que el registry
// usa como último recurso.
func embeddedFor(kind registry.Kind) []byte {
	switch kind {
	case registry.Services:
		return services.Embedded()
	case registry.Plugins:
		return plugins.Embedded()
	case registry.System:
		return systemdef.BuiltinBytes()
	}
	return nil
}
