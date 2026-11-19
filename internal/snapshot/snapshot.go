// Package snapshot pide una snapshot antes de tocar configuración.
//
// No hace copias de archivos: si snapper tiene config para home, crea
// una snapshot; si no, el llamador decide (warning, no bloqueo).
package snapshot

import (
	"fmt"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Create intenta una snapshot del subvolume home.
//
// El label es la descripción de la snapshot: es lo que después permite
// saber cuál era ("b3l0w pre-dotfiles"). Vacío no dice nada, así que se
// rechaza antes de tocar el disco.
func Create(label string) error {
	if strings.TrimSpace(label) == "" {
		return fmt.Errorf("el label de la snapshot está vacío")
	}
	if system.CommandPath("snapper") == "" {
		return fmt.Errorf("snapper no está instalado")
	}
	// La config `home` no existe en toda instalación (solo la de root).
	// Sin este chequeo, el error de snapper no distingue "falta la
	// config" de "snapper se rompió", que son decisiones distintas:
	// la primera es un aviso esperable, la segunda un error.
	if out, err := system.RunCombined("snapper", "-c", "home", "list"); err != nil {
		return fmt.Errorf("no hay config `home` en snapper: %s", strings.TrimSpace(out))
	}
	out, err := system.RunCombined("snapper", "-c", "home", "create", "-d", label)
	if err != nil {
		return fmt.Errorf("snapper -c home: %s", out)
	}
	return nil
}
