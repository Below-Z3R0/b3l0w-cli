package programs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// PackageName es el nombre del programa en el repo [b3l0w].
//
// Los paquetes propios no llevan prefijo (boot-preview, kitty-shapes);
// el CLI sí, porque el binario se llama b3l0w.
func PackageName(name string) string {
	if name == "b3l0w" || name == "b3l0w-cli" {
		return "b3l0w-cli"
	}
	return name
}

// Packaged dice si el programa ya se distribuye como paquete de
// [b3l0w]. Si es así, pacman manda: el asset suelto queda como
// camino viejo para programas que todavía no están empaquetados.
func Packaged(name string) bool {
	if !system.HaveCommand("pacman") {
		return false
	}
	_, err := system.RunTimeout(5*time.Second, "pacman", "-Si", PackageName(name))
	return err == nil
}

// InstallPackage instala con pacman heredando la terminal: sudo pide
// la contraseña ahí mismo, no en un pipe.
func InstallPackage(name string) (string, error) {
	if err := system.Interactive("sudo", "pacman", "-S", "--needed", PackageName(name)); err != nil {
		return "", fmt.Errorf("pacman no pudo instalar %s: %w", PackageName(name), err)
	}
	// El binario viejo de ~/.local/bin gana en PATH: si queda, se
	// ejecuta la versión suelta en vez de la del paquete.
	if viejo, err := InstallPath(name); err == nil {
		if owner := system.PackageOwner(viejo); owner == "" {
			_ = os.Remove(viejo)
		}
	}
	bin := system.CommandPath(name)
	if bin == "" {
		return "", fmt.Errorf("%s se instaló pero no está en PATH", name)
	}
	return Current(bin)
}

// RemovePackage quita el paquete, sus dependencias huérfanas y su
// configuración.
func RemovePackage(name string) error {
	if err := system.Interactive("sudo", "pacman", "-Rns", "--noconfirm", PackageName(name)); err != nil {
		return fmt.Errorf("pacman no pudo quitar %s: %w", PackageName(name), err)
	}
	return nil
}

// ownedByB3l0w dice si un paquete pertenece al repo propio.
func ownedByB3l0w(pkg string) bool {
	return pkg == "b3l0w-cli" || strings.HasPrefix(pkg, "b3l0w-") ||
		pkg == "boot-preview" || pkg == "kitty-shapes"
}
