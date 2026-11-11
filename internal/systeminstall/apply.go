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
)

// Options controla cómo se aplica el plan.
type Options struct {
	// Yes pasa --noconfirm a pacman y -y a flatpak (instalador
	// desatendido). Sin esto, pacman pregunta lo que tenga que preguntar.
	Yes bool

	// DryRun no ejecuta ningún comando: solo informa qué se haría.
	// Sirve para que un llamador pueda simular sin tocar la máquina.
	DryRun bool

	// Runner ejecuta los comandos contra la máquina. Nil usa la
	// máquina real (kit/system).
	Runner Runner

	// SyncPlugins deja la source de plugins de Noctalia lista. Los
	// avisos que tira no cortan la instalación, así que no devuelve
	// error. Nil usa la implementación real, que habla con noctalia.
	SyncPlugins func()
}

// runner devuelve el runner con el que se habla con la máquina.
func (o Options) runner() Runner { return orReal(o.Runner) }

// syncPlugins devuelve el paso de plugins a ejecutar.
func (o Options) syncPlugins() func() {
	if o.SyncPlugins != nil {
		return o.SyncPlugins
	}
	return preparePlugins
}

// preparePlugins deja la source de plugins de Noctalia lista: la agrega
// si falta y la actualiza si ya está.
//
// Los dos fallos son avisos, no errores: los plugins son un extra del
// sistema, no el sistema.
func preparePlugins() {
	if err := plugins.EnsureSource(); err != nil {
		output.Warn("plugins: " + err.Error())
	} else if plugins.HasSource(plugins.SourceName) {
		if err := plugins.UpdateSource(plugins.SourceName); err != nil {
			output.Warn("plugins: " + err.Error())
		} else {
			output.Ok("source de plugins actualizado")
		}
	}
}

// Apply ejecuta el plan en orden: paquetes, AUR, flatpak, propios,
// servicios, grupos, dotfiles y plugins.
//
// El orden importa: lo que habilita a lo que viene después se instala
// primero (paquetes antes que servicios, servicios antes que grupos).
// Un paso que falla corta el resto: la instalación queda a medias y el
// error dice qué falló.
func Apply(plan *Plan, opts Options) error {
	if opts.DryRun {
		dryRun(plan)
		return nil
	}

	run := opts.runner()

	if len(plan.Packages) > 0 {
		args := []string{"pacman", "-S", "--needed"}
		if opts.Yes {
			args = append(args, "--noconfirm")
		}
		args = append(args, plan.Packages...)
		if err := run.Interactive("sudo", args...); err != nil {
			return fmt.Errorf("pacman falló: %w", err)
		}
	}

	if len(plan.AUR) > 0 {
		helper := ""
		for _, h := range []string{"paru", "yay"} {
			if run.CommandPath(h) != "" {
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
		if err := run.Interactive(helper, args...); err != nil {
			return fmt.Errorf("%s falló: %w", helper, err)
		}
	}

	if len(plan.Flatpak) > 0 {
		// flathub tiene que existir; agregarlo es idempotente.
		_ = run.Interactive("flatpak", "remote-add", "--if-not-exists", "flathub",
			"https://flathub.org/repo/flathub.flatpakrepo")
		args := append([]string{"install", "-y", "flathub"}, plan.Flatpak...)
		if err := run.Interactive("flatpak", args...); err != nil {
			return fmt.Errorf("flatpak falló: %w", err)
		}
	}

	if len(plan.Own) > 0 {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		for _, name := range plan.Own {
			if err := run.Interactive(self, "install", name); err != nil {
				return fmt.Errorf("no se pudo instalar %s: %w", name, err)
			}
		}
	}

	if len(plan.Services) > 0 {
		if err := run.Interactive("sudo", append([]string{"systemctl", "enable"}, plan.Services...)...); err != nil {
			return fmt.Errorf("no se pudieron habilitar los servicios: %w", err)
		}
	}

	if len(plan.Groups) > 0 {
		current, err := user.Current()
		if err != nil {
			return err
		}
		if err := run.Interactive("sudo", "usermod", "-aG", strings.Join(plan.Groups, ","), current.Username); err != nil {
			return fmt.Errorf("no se pudieron agregar los grupos: %w", err)
		}
	}

	for _, file := range plan.Files {
		if !file.SrcExists {
			return fmt.Errorf("falta el origen del dotfile: %s", file.Src)
		}
		// Si el destino ya esta, no se toca: el plan se armó con lo que
		// FALTA (Empty() lo dice), y apply tiene que hacer lo mismo.
		// Antes se pisaba igual, dejando backups y perdiendo las
		// ediciones locales del usuario en cada corrida.
		if file.DestExists {
			continue
		}
		if err := installDotfile(run, file); err != nil {
			return err
		}
	}

	// Dejar los plugins de Noctalia listos: source y backends.
	opts.syncPlugins()()

	return nil
}

// dryRun informa qué haría Apply, sin ejecutar nada.
func dryRun(plan *Plan) {
	seccion := func(titulo string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Printf("  → %s (%d): %s\n", titulo, len(items), strings.Join(items, ", "))
	}

	seccion("paquetes", plan.Packages)
	seccion("AUR", plan.AUR)
	seccion("flatpak", plan.Flatpak)
	seccion("propios", plan.Own)
	seccion("servicios", plan.Services)
	seccion("grupos", plan.Groups)

	for _, f := range plan.Files {
		if f.SrcExists && !f.DestExists {
			fmt.Printf("  → dotfile: %s\n", f.Dest)
		}
	}
}

// destinoSeguro valida que el destino de un dotfile no pueda borrar el
// home ni el sistema de archivos.
//
// El destino viene del perfil (que puede ser una definicion remota), y
// ExpandHome("~") devuelve el home entero. Con "~" como destino, el
// RemoveAll de abajo borraba el home del usuario.
func destinoSeguro(dest string) error {
	if dest == "" {
		return fmt.Errorf("destino de dotfile vacio")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return fmt.Errorf("no se pudo determinar el home: %w", err)
	}
	limpio := filepath.Clean(dest)
	if limpio == "/" {
		return fmt.Errorf("destino de dotfile peligroso: %s", dest)
	}
	if limpio == filepath.Clean(home) {
		return fmt.Errorf("destino de dotfile peligroso: %s", dest)
	}
	if !strings.HasPrefix(limpio, filepath.Clean(home)+string(os.PathSeparator)) {
		return fmt.Errorf("el destino esta fuera del home: %s", dest)
	}
	return nil
}

// installDotfile copia un archivo o directorio con backup del destino.
func installDotfile(run Runner, file FileStatus) error {
	if err := destinoSeguro(file.Dest); err != nil {
		return err
	}
	if pathExists(file.Dest) {
		stamp := time.Now().Format("20060102-150405")
		backup := fmt.Sprintf("%s.bak-%s", file.Dest, stamp)
		// rename en vez de cp + RemoveAll: es atomico y no deja una
		// ventana en la que el destino no existe. Con cp, un corte
		// dejaba el destino ya borrado y el backup sin hacer.
		if err := os.Rename(file.Dest, backup); err != nil {
			return fmt.Errorf("backup de %s: %w", file.Dest, err)
		}
		output.Ok(fmt.Sprintf("backup: %s", backup))
	}

	if err := os.MkdirAll(filepath.Dir(file.Dest), 0o755); err != nil {
		return err
	}
	if err := run.Interactive("cp", "-a", file.Src, file.Dest); err != nil {
		return fmt.Errorf("copiar %s: %w", file.Src, err)
	}
	output.Ok(fmt.Sprintf("dotfile: %s", file.Dest))
	return nil
}
