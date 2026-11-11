// Package fetch baja un asset de un release de GitHub.
package fetch

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// ToTemp baja url a un temporal ejecutable y devuelve su ruta.
//
// El cache-buster evita la caché del CDN: justo después de publicar,
// /releases/latest/download sigue sirviendo el asset anterior. Y los
// timeouts evitan que una red muda deje el comando esperando.
func ToTemp(url string) (string, error) {
	tmp, err := os.CreateTemp("", "b3l0w-fetch-*")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	tmp.Close()

	full := fmt.Sprintf("%s?cb=%d", url, time.Now().Unix())
	out, err := system.RunTimeout(5*time.Minute,
		"curl", "-fsSL", "--connect-timeout", "15", "--max-time", "300", full, "-o", path)
	if err != nil {
		os.Remove(path)
		return "", fmt.Errorf("no se pudo bajar %s: %s", url, firstLine(out, err))
	}
	if err := os.Chmod(path, 0o755); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// firstLine devuelve la primera línea con contenido.
//
// Es la que sirve para diagnosticar: curl pone el motivo en la primera
// línea ("curl: (6) Could not resolve host") y deja el progreso abajo.
func firstLine(out string, err error) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	if err != nil {
		return err.Error()
	}
	return "sin salida"
}
