// Package channel guarda si el escritorio sigue stable o edge.
package channel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
)

const (
	Stable = "stable"
	Edge   = "edge"
)

// Path es <config>/b3l0w/channel.
//
// Usa el mismo resolver que el resto del CLI: con XDG_CONFIG_HOME puesto,
// el canal tiene que vivir donde el resto de la configuración.
func Path() string {
	dir := paths.ConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "channel")
}

// Current lee el canal guardado; si no hay, lo detecta por pacman.
func Current() string {
	if data, err := os.ReadFile(Path()); err == nil {
		// Solo los canales conocidos: un archivo editado a mano con
		// "nightly" se devolvía tal cual y update lo trataba como edge.
		// Un valor desconocido se ignora y se detecta.
		if value := strings.TrimSpace(string(data)); value == Stable || value == Edge {
			return value
		}
	}
	return Detect()
}

// Detect mira qué paquete está instalado.
//
// Una sola invocación de pacman para los dos paquetes: dos procesos
// separados suman 100-200 ms al arranque de cada comando.
func Detect() string {
	edge, err := algunoInstalado("umbriel-git", "noctalia-git")
	if err != nil || !edge {
		return Stable
	}
	return Edge
}

// Set escribe el canal pedido.
func Set(name string) error {
	if name != Stable && name != Edge {
		return fmt.Errorf("canal inválido: %s (usá stable o edge)", name)
	}
	path := Path()
	if path == "" {
		return fmt.Errorf("no se pudo resolver el directorio de configuración")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Escribir a un temporal y renombrar: un corte deja el archivo viejo,
	// no uno a medias ("st", "e") que Current() devolvería tal cual.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// algunoInstalado dice si alguno de los paquetes está instalado.
//
// pacman -Q con varios nombres sale 1 si falta alguno y la salida lista
// solo los que están: se mira la salida, no el exit code. La salida del
// comando no se hereda (antes se imprimía en el stdout del CLI).
func algunoInstalado(pkgs ...string) (bool, error) {
	args := append([]string{"-Q"}, pkgs...)
	out, err := exec.Command("pacman", args...).Output()
	if err != nil {
		// Exit 1 con salida vacía es "no está instalado", no un fallo.
		var exit *exec.ExitError
		if ok := errors.As(err, &exit); ok && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}
