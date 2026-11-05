// Package snapshot pide una snapshot antes de tocar configuración.
//
// No hace copias de archivos: si snapper tiene config para home, crea
// una snapshot; si no, el llamador decide (warning, no bloqueo).
package snapshot

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// Create intenta una snapshot del subvolume home.
func Create(label string) error {
	if cli.CommandPath("snapper") == "" {
		return fmt.Errorf("snapper no está instalado")
	}
	out, err := cli.RunCombined("snapper", "-c", "home", "create", "-d", label)
	if err != nil {
		return fmt.Errorf("snapper -c home: %s", out)
	}
	return nil
}
