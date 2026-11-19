// Package system envuelve el actualizador del sistema.
//
// En CachyOS el binario es `cachy-update`, un symlink a `arch-update`
// (el upstream). Es el mismo programa y pertenece a un paquete: no se
// renombra ni se reescribe, se llama por nombre con fallback al otro.
package system

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

const (
	cachyName    = "cachy-update"
	upstreamName = "arch-update"
)

// Binary devuelve la ruta del actualizador, o "" si no hay ninguno.
func Binary() string {
	if path := system.CommandPath(cachyName); path != "" {
		return path
	}
	return system.CommandPath(upstreamName)
}

// Available dice si hay actualizador del sistema.
func Available() bool { return Binary() != "" }

// List devuelve la salida cruda de `-l`, que es la opción no interactiva.
func List() (string, error) {
	bin := Binary()
	if bin == "" {
		return "", fmt.Errorf("no hay %s ni %s en el PATH", cachyName, upstreamName)
	}
	out, err := system.RunTimeout(5*time.Minute, bin, "-l")
	if err != nil {
		// Un cuelgue de 5 minutos se reintenta; un actualizador que
		// falla hay que mirarlo. Antes los dos salían igual.
		if errors.Is(err, context.DeadlineExceeded) {
			return out, fmt.Errorf("%s -l se colgó: %w", bin, err)
		}
		return out, fmt.Errorf("%s -l: %w", bin, err)
	}
	return out, nil
}

// Pending cuenta los paquetes pendientes separando repo y AUR.
//
// El formato de `arch-update -l` (v4.4.6) es:
//
//	==> Looking for updates...
//	==> Packages:
//	<paquete> <versión>  ->  <versión>
//	==> AUR Packages:
//	...
//	==> Flatpak Packages:
//	...
//
// Dos cosas del script que hay que respetar:
//
//   - La salida va con color aunque no sea un tty: la cabecera real es
//     "\x1b[1m\x1b[34m==>\x1b[0m\x1b[1m Packages:\x1b[0m". Sin quitar los
//     códigos, ninguna cabecera se reconoce y el conteo queda en cero.
//   - "Flatpak Packages:" no son paquetes de pacman: el doctor informa
//     de los del repo y de los de AUR, así que no cuentan.
//
// Una línea cuenta como paquete si tiene la forma de una entrada:
// "<nombre> <versión>", con o sin "-> <versión>". Un texto suelto
// dentro de la sección no cuenta.
func Pending(list string) (repo, aur int) {
	section := ""
	for _, line := range strings.Split(list, "\n") {
		trimmed := strings.TrimSpace(sinANSI(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "==>") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "==>")))
			continue
		}
		if !esPaquete(trimmed) {
			continue
		}
		switch {
		case strings.Contains(section, "flatpak"):
			// No son paquetes de pacman.
		case strings.Contains(section, "aur"):
			aur++
		case strings.Contains(section, "package"):
			repo++
		}
	}
	return repo, aur
}

// sinANSI quita los códigos de color de la salida del actualizador.
func sinANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// ansiRegexp matchea los códigos SGR que usa el actualizador.
var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;]*m")

// esPaquete dice si la línea es una entrada de paquete del listado.
//
// El formato es "<nombre> <versión>" o "<nombre> <versión> -> <versión>":
// el primer campo es un nombre de paquete y el segundo, una versión.
func esPaquete(line string) bool {
	campos := strings.Fields(line)
	if len(campos) < 2 {
		return false
	}
	return esNombreDePaquete(campos[0]) && esVersion(campos[1])
}

// esNombreDePaquete dice si el token parece un nombre de paquete:
// letras, dígitos y los signos que pacman admite (- _ . + @).
func esNombreDePaquete(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '+', r == '@':
		default:
			return false
		}
	}
	return true
}

// esVersion dice si el token parece una versión de pacman: arranca con
// un número, o con "r" en los paquetes de VCS (r1234.abc-1).
func esVersion(s string) bool {
	if s == "" {
		return false
	}
	if s[0] >= '0' && s[0] <= '9' {
		return true
	}
	return s[0] == 'r' && len(s) > 1 && s[1] >= '0' && s[1] <= '9'
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
	return system.Interactive(bin)
}
