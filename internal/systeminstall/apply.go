package systeminstall

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"

	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Options controla cómo se aplica el plan.
type Options struct {
	// Yes pasa --noconfirm a pacman y -y a flatpak (instalador
	// desatendido). Sin esto, pacman pregunta lo que tenga que preguntar.
	Yes bool
}

// Apply ejecuta el plan en orden: paquetes, flatpak, propios, servicios,
// grupos y archivos.
func Apply(plan *Plan, opts Options) error {
	if len(plan.Packages) > 0 {
		args := []string{"pacman", "-S", "--needed"}
		if opts.Yes {
			args = append(args, "--noconfirm")
		}
		args = append(args, plan.Packages...)
		if err := system.Interactive("sudo", args...); err != nil {
			return fmt.Errorf("pacman falló: %w", err)
		}
	}

	if len(plan.AUR) > 0 {
		helper := ""
		for _, h := range []string{"paru", "yay"} {
			if system.CommandPath(h) != "" {
				helper = h
				break
			}
		}
		if helper == "" {
			return fmt.Errorf("hay %d paquetes AUR y no está paru ni yay; usá el repo [b3l0w]", len(plan.AUR))
		}
		args := []string{"-S", "--needed"}
		if opts.Yes {
			args = append(args, "--noconfirm")
		}
		args = append(args, plan.AUR...)
		if err := system.Interactive(helper, args...); err != nil {
			return fmt.Errorf("%s falló: %w", helper, err)
		}
	}

	if len(plan.Flatpak) > 0 {
		// flathub tiene que existir; agregarlo es idempotente.
		_ = system.Interactive("flatpak", "remote-add", "--if-not-exists", "flathub",
			"https://flathub.org/repo/flathub.flatpakrepo")
		args := append([]string{"install", "-y", "flathub"}, plan.Flatpak...)
		if err := system.Interactive("flatpak", args...); err != nil {
			return fmt.Errorf("flatpak falló: %w", err)
		}
	}

	if len(plan.Own) > 0 {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		for _, name := range plan.Own {
			if err := system.Interactive(self, "install", name); err != nil {
				return fmt.Errorf("no se pudo instalar %s: %w", name, err)
			}
		}
	}

	if len(plan.Services) > 0 {
		if err := system.Interactive("sudo", append([]string{"systemctl", "enable"}, plan.Services...)...); err != nil {
			return fmt.Errorf("no se pudieron habilitar los servicios: %w", err)
		}
	}

	if len(plan.Groups) > 0 {
		current, err := user.Current()
		if err != nil {
			return err
		}
		if err := system.Interactive("sudo", "usermod", "-aG", strings.Join(plan.Groups, ","), current.Username); err != nil {
			return fmt.Errorf("no se pudieron agregar los grupos: %w", err)
		}
	}

	for _, file := range plan.Files {
		if !file.SrcExists {
			return fmt.Errorf("falta el origen del dotfile: %s", file.Src)
		}
		if err := installDotfile(file); err != nil {
			return err
		}
	}

	// Dejar los plugins de Noctalia listos: source y backends.
	if err := plugins.EnsureSource(); err != nil {
		output.Warn("plugins: " + err.Error())
	} else if plugins.HasSource(plugins.SourceName) {
		if err := plugins.UpdateSource(plugins.SourceName); err != nil {
			output.Warn("plugins: " + err.Error())
		} else {
			output.Ok("source de plugins actualizado")
		}
	}
	return nil
}

// installDotfile copia un archivo o directorio con backup del destino.
func installDotfile(file FileStatus) error {
	if pathExists(file.Dest) {
		stamp := time.Now().Format("20060102-150405")
		backup := fmt.Sprintf("%s.bak-%s", file.Dest, stamp)
		if err := system.Interactive("cp", "-a", file.Dest, backup); err != nil {
			return fmt.Errorf("backup de %s: %w", file.Dest, err)
		}
		if err := os.RemoveAll(file.Dest); err != nil {
			return err
		}
		output.Ok(fmt.Sprintf("backup: %s", backup))
	}

	if err := os.MkdirAll(filepath.Dir(file.Dest), 0o755); err != nil {
		return err
	}
	if err := system.Interactive("cp", "-a", file.Src, file.Dest); err != nil {
		return fmt.Errorf("copiar %s: %w", file.Src, err)
	}
	output.Ok(fmt.Sprintf("dotfile: %s", file.Dest))
	return nil
}
