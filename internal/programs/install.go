package programs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/fetch"
)

// AssetURL arma la URL del asset del último release a partir de la
// fuente "github:owner/repo".
func AssetURL(p config.Program) (string, error) {
	if p.Asset == "" {
		return "", fmt.Errorf("%s no tiene asset en el manifiesto (b3l0w program add --asset=…)", p.Name)
	}
	if !strings.HasPrefix(p.Source, "github:") {
		return "", fmt.Errorf("fuente no soportada para %s: %q (usá github:owner/repo)", p.Name, p.Source)
	}
	owner, repo, ok := strings.Cut(strings.TrimPrefix(p.Source, "github:"), "/")
	if !ok || owner == "" || repo == "" {
		return "", fmt.Errorf("fuente no soportada para %s: %q (usá github:owner/repo)", p.Name, p.Source)
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", owner, repo, p.Asset), nil
}

// InstallPath es donde se deja el binario de un programa.
func InstallPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", name), nil
}

// Install baja el asset del último release y lo deja en ~/.local/bin.
//
// Valida el binario con `version` antes de pisar el que está: un asset
// corrupto o de otra arquitectura no reemplaza algo que funciona.
func Install(p config.Program) (string, error) {
	url, err := AssetURL(p)
	if err != nil {
		return "", err
	}

	tmp, err := fetch.ToTemp(url)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)

	version, err := Current(tmp)
	if err != nil {
		return "", fmt.Errorf("el binario bajado no responde: %w", err)
	}

	dest, err := InstallPath(p.Name)
	if err != nil {
		return "", err
	}
	if err := cli.ReplaceFile(tmp, dest); err != nil {
		return "", fmt.Errorf("no se pudo escribir %s: %w", dest, err)
	}
	return version, nil
}
