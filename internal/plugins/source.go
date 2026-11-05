package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// SourceName es el nombre del source de Noctalia que publica el registry.
const SourceName = "b3l0w"

// RegistryURL es el repo que Noctalia clona como source.
const RegistryURL = "https://github.com/Below-Z3R0/b3l0w-plugins"

// EnsureSource agrega el source a Noctalia si falta.
func EnsureSource() error {
	if HasSource(SourceName) {
		return nil
	}
	if !cli.HaveCommand("noctalia") {
		return fmt.Errorf("noctalia no está en PATH")
	}
	out, err := cli.RunCombined("noctalia", "msg", "plugins", "source", "add", SourceName, "git", RegistryURL)
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// HasSource dice si Noctalia tiene el source agregado.
func HasSource(name string) bool {
	if !cli.HaveCommand("noctalia") {
		return false
	}
	out, err := cli.RunCombined("noctalia", "msg", "plugins", "source", "list")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), name+" ") {
			return true
		}
	}
	return false
}

// UpdateSource repara el clon si quedó con archivos borrados y pide a
// Noctalia que refresque el source.
func UpdateSource(name string) error {
	repairSourceRepo(name)
	out, err := cli.RunCombined("noctalia", "msg", "plugins", "update", name)
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// repairSourceRepo detecta un working tree vacío (nos pasó: todo el
// índice como "D") y lo restaura desde HEAD.
func repairSourceRepo(name string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	repo := filepath.Join(home, ".local", "state", "noctalia", "plugins", "sources", name, "repo")
	if _, err := os.Stat(repo); err != nil {
		return
	}
	out, err := cli.RunCombined("git", "-C", repo, "status", "--porcelain")
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "D ") || strings.HasPrefix(line, " D ") {
			cli.RunCombined("git", "-C", repo, "reset", "--hard", "HEAD")
			return
		}
	}
}
