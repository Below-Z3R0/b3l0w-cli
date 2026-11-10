package main

import (
	"os"
	"path/filepath"

	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// ensureLauncher oculta el lanzador de Cachy-Update y deja el de
// b3l0w.
//
// Se escribe en ~/.local/share/applications, que tiene prioridad sobre
// /usr/share/applications: no se toca el archivo del paquete
// arch-update (pacman no permite que un paquete pise el archivo de
// otro).
func ensureLauncher() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	apps := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		output.Warn("launcher: " + err.Error())
		return
	}

	target := filepath.Join(apps, "arch-update.desktop")
	if _, err := os.Stat(target); err == nil {
		output.Ok("launcher: Cachy-Update oculto")
		return
	}

	content := `[Desktop Entry]
Type=Application
Name=Cachy-Update
Hidden=true
`
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		output.Warn("launcher: " + err.Error())
		return
	}
	output.Ok("launcher: Cachy-Update oculto")
}
