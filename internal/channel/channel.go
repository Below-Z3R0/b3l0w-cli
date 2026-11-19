// Package channel guarda si el escritorio sigue stable o edge.
package channel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	Stable = "stable"
	Edge   = "edge"
)

// Path es ~/.config/b3l0w/channel.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "b3l0w", "channel")
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
func Detect() string {
	if installed("umbriel-git") || installed("noctalia-git") {
		return Edge
	}
	return Stable
}

// Set escribe el canal pedido.
func Set(name string) error {
	if name != Stable && name != Edge {
		return fmt.Errorf("canal inválido: %s (usá stable o edge)", name)
	}
	path := Path()
	if path == "" {
		return fmt.Errorf("no se pudo resolver el home")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Escribir a un temporal y renombrar: un corte deja el archivo viejo,
	// no uno a medias ("st", "e") que Current() devolveria tal cual.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func installed(pkg string) bool {
	return exec.Command("pacman", "-Q", pkg).Run() == nil
}
