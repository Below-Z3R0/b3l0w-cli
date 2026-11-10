// Package system envuelve el actualizador del sistema.
//
// En CachyOS el binario es `cachy-update`, un symlink a `arch-update`
// (el upstream). Es el mismo programa y pertenece a un paquete: no se
// renombra ni se reescribe, se llama por nombre con fallback al otro.
package system

import (
	"fmt"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-kit/cli"
)

const (
	cachyName    = "cachy-update"
	upstreamName = "arch-update"
)

// Binary devuelve la ruta del actualizador, o "" si no hay ninguno.
func Binary() string {
	if path := cli.CommandPath(cachyName); path != "" {
		return path
	}
	return cli.CommandPath(upstreamName)
}

// Available dice si hay actualizador del sistema.
func Available() bool { return Binary() != "" }

// List devuelve la salida cruda de `-l`, que es la opción no interactiva.
func List() (string, error) {
	bin := Binary()
	if bin == "" {
		return "", fmt.Errorf("no hay %s ni %s en el PATH", cachyName, upstreamName)
	}
	out, err := cli.RunTimeout(5*time.Minute, bin, "-l")
	if err != nil {
		return out, err
	}
	return out, nil
}

// Pending cuenta los paquetes pendientes separando repo y AUR.
//
// El formato de -l es una sección por encabezado (==> Packages:,
// ==> AUR packages:) y una línea por paquete con "->" o, si el paquete
// no está en el repo, solo su versión. No se parsea el nombre: alcanza
// con contar líneas de la sección.
func Pending(list string) (repo, aur int) {
	section := ""
	for _, line := range strings.Split(list, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "==>") {
			section = strings.ToLower(trimmed)
			continue
		}
		if strings.Contains(section, "aur") {
			aur++
		} else if strings.Contains(section, "package") {
			repo++
		}
	}
	return repo, aur
}

// UpdateInteractive corre el actualizador heredando la terminal.
//
// No se automatiza: el programa muestra las news, pide confirmación y
// después ofrece huérfanos, pacnew, kernel pendiente y servicios a
// reiniciar. Una bandera --yes no existe, y fingir que sí sería mentir.
func UpdateInteractive() error {
	bin := Binary()
	if bin == "" {
		return fmt.Errorf("no hay actualizador del sistema")
	}
	return cli.Interactive(bin)
}
