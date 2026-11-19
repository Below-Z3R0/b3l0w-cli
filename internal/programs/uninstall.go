package programs

import (
	"fmt"
	"os"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"

	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Uninstall borra el binario de ~/.local/bin y lo saca del manifiesto.
// Si el binario pertenece a un paquete pacman, no lo toca.
//
// El manifiesto se guarda en paths.ManifestPath(): es el que usa el CLI.
// Para guardar en otro lado (un test, otro XDG_CONFIG_HOME) está
// UninstallTo.
func Uninstall(name string, m *config.Manifest) (string, error) {
	return UninstallTo(name, m, paths.ManifestPath())
}

// UninstallTo es Uninstall con la ruta del manifiesto explícita.
//
// El manifiesto que se recibe puede venir de otro lado: guardarlo
// siempre en paths.ManifestPath() descarta lo que el llamador quiso
// guardar y deja el manifiesto real sin tocar.
//
// El manifiesto se guarda ANTES de borrar el binario. Al revés, un save
// fallido dejaba el binario ya borrado y el programa todavía listado:
// el próximo update lo buscaba, no lo encontraba y fallaba sin decir por
// qué. Si el borrado falla después, el manifiesto ya no lo lista y queda
// un binario huérfano, que es el estado menos malo de los dos: el
// manifiesto es la fuente de verdad y un archivo suelto se borra a mano.
func UninstallTo(name string, m *config.Manifest, manifestPath string) (string, error) {
	p := m.Get(name)
	if p == nil {
		return "", fmt.Errorf("%s no está registrado", name)
	}
	path, err := InstallPath(name)
	if err != nil {
		return "", err
	}
	// Si el programa está instalado como paquete, pacman manda.
	if Installed(name) {
		if err := RemovePackage(name); err != nil {
			return "", err
		}
		_ = os.Remove(path) // el binario suelto, si quedó
		m.Remove(name)
		if err := m.Save(manifestPath); err != nil {
			return "", fmt.Errorf("no se pudo guardar el manifiesto: %w", err)
		}
		return path, nil
	}
	if owner := system.PackageOwner(path); owner != "" {
		if ownedByB3l0w(owner) {
			if err := RemovePackage(name); err != nil {
				return "", err
			}
		}
		return "", fmt.Errorf("%s es del paquete %s: se quita con pacman", name, owner)
	}
	// El manifiesto primero: si el save falla, el binario sigue en su
	// lugar y la entrada se restaura en memoria.
	m.Remove(name)
	if err := m.Save(manifestPath); err != nil {
		m.AddProgram(*p)
		return "", fmt.Errorf("no se pudo guardar el manifiesto: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("no se pudo borrar %s: %w", path, err)
		}
	}
	return path, nil
}
