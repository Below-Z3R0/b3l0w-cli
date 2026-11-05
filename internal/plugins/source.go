package plugins

import (
	"fmt"
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

// UpdateSource pide a Noctalia que refresque el source.
func UpdateSource(name string) error {
	out, err := cli.RunCombined("noctalia", "msg", "plugins", "update", name)
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
