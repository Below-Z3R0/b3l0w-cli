package programs

import (
	"fmt"
	"os"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// Uninstall borra el binario de ~/.local/bin y lo saca del manifiesto.
// Si el binario pertenece a un paquete pacman, no lo toca.
func Uninstall(name string, m *config.Manifest) (string, error) {
	if m.Get(name) == nil {
		return "", fmt.Errorf("%s no está registrado", name)
	}
	path, err := InstallPath(name)
	if err != nil {
		return "", err
	}
	if owner := cli.PackageOwner(path); owner != "" {
		return "", fmt.Errorf("%s es del paquete %s: se quita con pacman", name, owner)
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("no se pudo borrar %s: %w", path, err)
		}
	}
	m.Remove(name)
	if err := m.Save(cli.ManifestPath()); err != nil {
		return "", fmt.Errorf("no se pudo guardar el manifiesto: %w", err)
	}
	return path, nil
}
