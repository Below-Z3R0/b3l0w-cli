// Package apps es el wrapper de pacman.
//
// No reimplementa el gestor: traduce banderas y agrega una sola cosa
// que pacman no hace — impedir un partial upgrade.
//
//	-Syu        sync + upgrade (el correcto para actualizar)
//	-S <pkg>    instala, solo si la DB está fresca
//	-Sy/-Syy    rechazado: deja el sistema a medio actualizar
//	-R          se expande a -Rns
//	-Ss/-Si/-Q  consultas, sin root
package apps

import (
	"errors"
	"fmt"
	"math"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// MaxAge es cuánto puede tener la DB de pacman antes de negarse a
// instalar. Arch no fija un número; una semana es el punto donde un
// partial upgrade empieza a romper librerías.
const MaxAge = 7 * 24 * time.Hour

// Run ejecuta pacman con las banderas traducidas.
func Run(args []string) int {
	allowStale := false
	pacmanArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--allow-stale" {
			allowStale = true
			continue
		}
		pacmanArgs = append(pacmanArgs, arg)
	}

	if len(pacmanArgs) == 0 {
		return exec(false, "-Q")
	}

	// El freno mira TODOS los argumentos, no el primero: con
	// `--noconfirm -Sy firefox` el -Sy queda en la posición 1 y el
	// freno no lo veía.
	for _, arg := range pacmanArgs {
		if partialUpgrade(arg) {
			output.Fail("eso es un partial upgrade: sincroniza la DB sin actualizar y deja el sistema a medias")
			fmt.Println("  usá: b3l0w apps -Syu")
			return 1
		}
	}

	flag := pacmanArgs[0]

	// El freno de DB vieja aplica a las instalaciones, no a -Syu.
	//
	// -Syu sincroniza y actualiza: el chequeo de "¿la DB es vieja?"
	// se resuelve solo con el sync. Antes frenaba -Syu y el mensaje de
	// escape decia "usá: b3l0w apps -Syu": un deadlock, porque el
	// comando que el usuario necesitaba era el que se rechazaba.
	if instala(flag) && !allowStale && !actualiza(flag) {
		if edad, db, err := OldestSyncDB(); err == nil && edad > MaxAge {
			output.Fail(fmt.Sprintf("la base de datos de pacman tiene %s (%s)", humanizar(edad), db))
			fmt.Println("  actualizá antes de instalar: b3l0w apps -Syu")
			fmt.Println("  o forzá con --allow-stale")
			return 1
		}
	}

	// -R quita el paquete y nada más. -Rns agrega dependencias
	// huérfanas y configuración: es lo que la gente espera.
	if flag == "-R" {
		pacmanArgs[0] = "-Rns"
	}

	return exec(needsRoot(pacmanArgs[0]), pacmanArgs...)
}

func exec(root bool, args ...string) int {
	nombre := "pacman"
	full := args
	if root {
		nombre = "sudo"
		full = append([]string{"pacman"}, args...)
	}
	if err := system.Interactive(nombre, full...); err != nil {
		// El exit code de pacman, no un 1 genérico: un 127 (el binario
		// no existe) no le dice lo mismo a un script que un 1 (la
		// transacción falló).
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

// banderas descompone una bandera corta de pacman en sus letras.
//
// pacman combina las banderas: -Syu es -S + -y + -u. Mirar substrings
// confunde "Sy" dentro de otra palabra y no distingue -Syyu (que sí
// actualiza) de -Syy (que no).
func banderas(flag string) map[rune]bool {
	out := map[rune]bool{}
	if !strings.HasPrefix(flag, "-") || strings.HasPrefix(flag, "--") {
		return out
	}
	for _, c := range strings.TrimPrefix(flag, "-") {
		out[c] = true
	}
	return out
}

// partialUpgrade detecta -Sy y -Syy sin -u.
func partialUpgrade(flag string) bool {
	b := banderas(flag)
	return b['S'] && b['y'] && !b['u']
}

// instala dice si el comando baja paquetes (no busca ni consulta).
func instala(flag string) bool {
	if !strings.HasPrefix(flag, "-S") {
		return false
	}
	return !strings.Contains(flag, "s") && !strings.Contains(flag, "i")
}

// actualiza dice si el flag sincroniza y actualiza todo (-Syu).
//
// -Syu refresca las DB antes de actualizar: la freshness no es un
// problema ahi, es justamente lo que arregla.
func actualiza(flag string) bool {
	b := banderas(flag)
	return b['S'] && b['y'] && b['u']
}

// needsRoot: consultar (-Ss, -Si, -Q) no pide sudo; instalar y quitar
// sí. Distinguirlos evita un password para un simple `-Ss`.
func needsRoot(flag string) bool {
	if strings.HasPrefix(flag, "-R") || strings.HasPrefix(flag, "-U") {
		return true
	}
	return instala(flag)
}

// OldestSyncDB devuelve la edad de la DB más vieja de pacman.
//
// La más vieja manda: si una sección quedó atrás, instalar desde ella
// es igual de riesgoso.
func OldestSyncDB() (time.Duration, string, error) {
	return oldestSyncDBIn("/var/lib/pacman/sync")
}

// oldestSyncDBIn es OldestSyncDB con el directorio explícito: un test no
// necesita /var/lib/pacman.
func oldestSyncDBIn(dir string) (time.Duration, string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.db"))
	if err != nil || len(paths) == 0 {
		return 0, "", fmt.Errorf("no hay bases de datos de pacman en %s", dir)
	}
	var vieja time.Time
	var nombre string
	for _, path := range paths {
		fi, err := os.Stat(path)
		if err != nil {
			// Una DB que no se puede leer no se salta en silencio: si
			// era la más vieja, el resultado miente.
			return 0, "", fmt.Errorf("no pude leer %s: %w", filepath.Base(path), err)
		}
		if vieja.IsZero() || fi.ModTime().Before(vieja) {
			vieja = fi.ModTime()
			nombre = filepath.Base(path)
		}
	}
	return time.Since(vieja), nombre, nil
}

func humanizar(d time.Duration) string {
	dias := int(d.Hours() / 24)
	switch {
	case dias >= 2:
		return fmt.Sprintf("%d días", dias)
	case dias == 1:
		return "1 día"
	}
	// Redondear al alza: 30 minutos son "1 h", no "0 h"; y 26 horas
	// ya son "1 día", no "1 día" por el piso de la división.
	horas := int(math.Ceil(d.Hours()))
	if horas < 1 {
		return "menos de 1 h"
	}
	return fmt.Sprintf("%d h", horas)
}
