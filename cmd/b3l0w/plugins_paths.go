package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// Las rutas y el parseo de argumentos de los plugins.
// pluginPaths resuelve las rutas y separa las banderas de los argumentos.
func pluginPaths(args []string) (src, registry, manifest, noctalia string, rest []string, dryRun bool) {
	home, _ := os.UserHomeDir()
	src = filepath.Join(home, "Documents/Github")
	registry = filepath.Join(src, "b3l0w-plugins")
	// Sin --manifest, el catálogo sale del registry (caché -> red -> embed).
	manifest = ""
	noctalia = filepath.Join(home, ".local", "share", "noctalia", "plugins")

	f := parsearFlags(args)
	src = f.valorO("--src", src)
	registry = f.valorO("--registry", registry)
	manifest = f.valorO("--manifest", manifest)
	noctalia = f.valorO("--plugins-dir", noctalia)
	dryRun = f.presente("--dry-run")
	rest = f.libres
	for nombre, valor := range f.valores {
		switch nombre {
		case "--src", "--registry", "--manifest", "--plugins-dir":
			continue
		}
		rest = append(rest, nombre+"="+valor)
	}
	for _, d := range f.desconocidos {
		rest = append(rest, d)
	}
	return
}

func loadPluginManifest(path string) (*plugins.Manifest, int) {
	m, err := plugins.LoadManifest(path)
	if err != nil {
		origen := path
		if origen == "" {
			origen = "el registry"
		}
		output.Fail(fmt.Sprintf("no se pudo leer %s: %v", origen, err))
		return nil, 1
	}
	return m, 0
}
