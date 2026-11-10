// Package plugins descubre y ejecuta extensiones.
//
// Una extensión es un binario `b3l0w-<nombre>` en el directorio privado
// del CLI, nunca en el PATH: cualquier cosa que pueda escribir en un
// directorio del PATH podría poner un b3l0w-update y correrlo con este
// usuario. git lo permite y es un riesgo aceptado; acá no.
package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-kit/paths"
)

// Prefix es el prefijo de los binarios de extensión.
const Prefix = "b3l0w-"

// Discover devuelve los nombres de las extensiones instaladas.
func Discover() []string {
	entries, err := os.ReadDir(paths.PluginsDir())
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, Prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		names = append(names, strings.TrimPrefix(name, Prefix))
	}
	return names
}

// Find devuelve la ruta de una extensión, o "" si no existe.
func Find(name string) string {
	path := filepath.Join(paths.PluginsDir(), Prefix+name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return path
}

// Run ejecuta la extensión heredando la terminal.
func Run(name string, args []string) error {
	path := Find(name)
	if path == "" {
		return fmt.Errorf("no existe la extensión %s%s", Prefix, name)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
