package plugins

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

//go:embed scripts/*.sh
var scriptsFS embed.FS

// DefaultScriptsDest es donde los plugins esperan sus backends.
const DefaultScriptsDest = "/usr/local/bin"

// Scripts lista los scripts embebidos.
func Scripts() ([]string, error) {
	entries, err := scriptsFS.ReadDir("scripts")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// InstallScripts copia los scripts embebidos a dest.
//
// Intenta escribir directo; si no hay permiso, usa `sudo install`.
func InstallScripts(dest string) ([]string, error) {
	if dest == "" {
		dest = DefaultScriptsDest
	}
	names, err := Scripts()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}

	for _, name := range names {
		data, err := scriptsFS.ReadFile(filepath.Join("scripts", name))
		if err != nil {
			return nil, err
		}
		target := filepath.Join(dest, name)
		if err := os.WriteFile(target, data, 0o755); err == nil {
			continue
		}

		tmp, err := os.CreateTemp("", "b3l0w-script-*")
		if err != nil {
			return nil, err
		}
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return nil, err
		}
		tmp.Close()
		os.Chmod(tmp.Name(), 0o755)

		out, err := cli.RunCombined("sudo", "install", "-m755", tmp.Name(), target)
		os.Remove(tmp.Name())
		if err != nil {
			return nil, fmt.Errorf("%s: %v: %s", name, err, out)
		}
	}
	return names, nil
}
