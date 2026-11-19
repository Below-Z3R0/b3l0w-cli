// Package programs consulta y actualiza los programas del manifiesto.
//
// El contrato con cada programa es chico y no se negocia:
//
//	prog version   -> una línea: prog vX.Y.Z
//	prog update    -> se actualiza solo, sin preguntar, exit 0/1
//	prog doctor    -> chequeos, exit != 0 si algo falta
//
// El core no sabe de GitHub ni de assets: ejecuta y reporta.
package programs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// versionRe acepta "v1.2.3" y "1.2.3". Es la única forma de leer una
// versión en todo el CLI.
//
// Los límites de palabra evitan que una terna pegada a otra palabra
// cuente como versión ("abc1.2.3"), y el sufijo opcional acepta las
// versiones con pre-release ("v1.2.3-rc.1").
var versionRe = regexp.MustCompile(`\bv?(\d+\.\d+\.\d+)(?:[-+][0-9A-Za-z][0-9A-Za-z.+]*)?\b`)

// Status es el estado de un programa en esta máquina.
type Status struct {
	Name      string
	Bin       string
	Version   string
	Installed bool
	Err       string
}

// ParseVersion saca la versión de la salida de `prog version`.
//
// Se queda con la primera terna que parece una versión. Las que tienen
// forma de fecha (YYYY.MM.DD) se descartan: los tags de este proyecto
// usan fechas ("desktop-2026.11.06-r1") y un copyright también las
// tiene.
func ParseVersion(out string) string {
	for _, match := range versionRe.FindAllStringSubmatch(out, -1) {
		if len(match) < 2 || looksLikeDate(match[1]) {
			continue
		}
		return "v" + match[1]
	}
	return ""
}

// looksLikeDate dice si una terna "N.N.N" es una fecha y no una versión.
//
// El año tiene cuatro dígitos, el mes va de 1 a 12 y el día de 1 a 31:
// con eso "2026.11.06" es una fecha y "1.2.3" es una versión.
func looksLikeDate(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	if len(parts[0]) != 4 {
		return false
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 1000 || year > 9999 {
		return false
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		return false
	}
	day, err := strconv.Atoi(parts[2])
	if err != nil || day < 1 || day > 31 {
		return false
	}
	return true
}

// Current pregunta la versión a un binario.
//
// Un binario viejo que no conoce `version` imprime su ayuda y sale con
// error: mostrar "exit status 1" no dice nada, así que se usa su primera
// línea, que sí explica qué pasó.
func Current(bin string) (string, error) {
	out, err := system.RunTimeout(10*time.Second, bin, "version")
	if err != nil {
		message := strings.TrimSpace(out)
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s version falló: %s", bin, firstLine(message))
	}
	version := ParseVersion(out)
	if version == "" {
		return "", fmt.Errorf("%s version no devolvió una versión: %q", bin, firstLine(out))
	}
	return version, nil
}

// firstLine devuelve la primera línea no vacía.
func firstLine(text string) string {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "sin salida"
}

// Statuses arma el estado de todos los programas del manifiesto.
func Statuses(m *config.Manifest) []Status {
	list := make([]Status, 0, len(m.Programs))
	for _, program := range m.Programs {
		status := Status{Name: program.Name}
		bin := system.CommandPath(program.Name)
		if bin == "" {
			list = append(list, status)
			continue
		}
		status.Bin = bin
		status.Installed = true

		version, err := Current(bin)
		if err != nil {
			status.Err = err.Error()
		} else {
			status.Version = version
		}
		list = append(list, status)
	}
	return list
}

// Update ejecuta el update de un programa.
//
// Devuelve su salida y si salió bien. No aborta: el llamador decide si
// sigue con el resto.
func Update(name string) (string, bool) {
	bin := system.CommandPath(name)
	if bin == "" {
		return "no está instalado", false
	}

	// Si es de un paquete, no se auto-actualiza: eso lo hace el sistema.
	if owner := system.PackageOwner(bin); owner != "" {
		return fmt.Sprintf("es del paquete %s: se actualiza con b3l0w update", owner), true
	}

	out, err := system.RunTimeout(10*time.Minute, bin, "update")
	if err != nil {
		message := strings.TrimSpace(out)
		if message == "" {
			message = err.Error()
		}
		return lastLine(message), false
	}
	return lastLine(out), true
}

// lastLine deja el mensaje más útil de la salida de un update.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return "sin salida"
}
