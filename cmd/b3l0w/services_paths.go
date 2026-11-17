package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Las rutas y el parseo de argumentos de los servicios.
// servicePaths resuelve las rutas del manifiesto y del estado, y separa
// los flags de los argumentos. Los flags con valor se pueden escribir
// como "--flag=valor" o "--flag valor".
func servicePaths(args []string) (manifest, stateDir string, rest []string, dryRun bool) {
	home, _ := os.UserHomeDir()
	// Sin --manifest, la fuente es el registry (caché -> red -> embed).
	manifest = ""
	stateDir = filepath.Join(home, ".local/state/b3l0w/services")
	f := parsearFlags(args)
	manifest = f.valorO("--manifest", manifest)
	stateDir = f.valorO("--state-dir", stateDir)
	dryRun = f.presente("--dry-run")
	// Los flags que no son de services se pasan tal cual: los usan
	// otros subcomandos.
	rest = f.libres
	for nombre, valor := range f.valores {
		switch nombre {
		case "--manifest", "--state-dir":
			continue
		}
		rest = append(rest, nombre+"="+valor)
	}
	for _, d := range f.desconocidos {
		rest = append(rest, d)
	}
	return
}

// serviceDir devuelve el directorio del repo de un servicio.
func repoName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}
