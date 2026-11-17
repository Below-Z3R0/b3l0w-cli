// Package systeminstall compara la definición del sistema contra la
// máquina y aplica lo que falta.
package systeminstall

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"

	kitsystem "github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// FileStatus es un dotfile: si existe el origen (en el repo) y el destino.
type FileStatus struct {
	Src        string
	Dest       string
	SrcExists  bool
	DestExists bool
}

// Plan es lo que falta para que la máquina cumpla la definición.
type Plan struct {
	Packages []string
	AUR      []string
	Flatpak  []string
	Own      []string
	Services []string
	Groups   []string
	Files    []FileStatus
}

// Empty dice si no hay nada para hacer.
func (p *Plan) Empty() bool {
	return len(p.Packages) == 0 && len(p.AUR) == 0 && len(p.Flatpak) == 0 &&
		len(p.Own) == 0 && len(p.Services) == 0 && len(p.Groups) == 0 &&
		!p.filesMissing()
}

func (p *Plan) filesMissing() bool {
	for _, f := range p.Files {
		if f.SrcExists && !f.DestExists {
			return true
		}
	}
	return false
}

// Build arma el plan. dotfiles es el directorio con los archivos del
// perfil (por ejemplo el `dotfiles/` del repo).
//
// opts.Runner decide contra qué máquina se consulta el estado: nil usa
// la máquina real. Yes y DryRun no aplican acá: Build solo lee.
func Build(profile systemdef.Profile, dotfiles string, opts Options) *Plan {
	run := opts.runner()
	plan := &Plan{}
	installed := installedPackages(run)

	for _, pkg := range profile.Repo {
		if !installed[pkg] {
			plan.Packages = append(plan.Packages, pkg)
		}
	}

	for _, pkg := range profile.AUR {
		if !installed[pkg] {
			plan.AUR = append(plan.AUR, pkg)
		}
	}

	flatpaks := installedFlatpaks(run)
	for _, app := range profile.Flatpak {
		if !flatpaks[app] {
			plan.Flatpak = append(plan.Flatpak, app)
		}
	}

	for _, name := range profile.Own {
		if run.CommandPath(name) == "" {
			plan.Own = append(plan.Own, name)
		}
	}

	for _, service := range profile.Services {
		if !serviceEnabled(run, service) {
			plan.Services = append(plan.Services, service)
		}
	}

	groups := userGroups(run)
	for _, group := range profile.Groups {
		if !groups[group] {
			plan.Groups = append(plan.Groups, group)
		}
	}

	for _, file := range profile.Files {
		src := file.Src
		if dotfiles != "" && !filepath.IsAbs(src) {
			src = filepath.Join(dotfiles, file.Src)
		}
		dest := kitsystem.ExpandHome(file.Dest)
		plan.Files = append(plan.Files, FileStatus{
			Src:        src,
			Dest:       dest,
			SrcExists:  pathExists(src),
			DestExists: pathExists(dest),
		})
	}
	return plan
}

func installedPackages(run Runner) map[string]bool {
	set := map[string]bool{}
	out, err := run.Run("pacman", "-Qq")
	if err != nil {
		return set
	}
	for _, name := range strings.Split(out, "\n") {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = true
		}
	}
	return set
}

func installedFlatpaks(run Runner) map[string]bool {
	set := map[string]bool{}
	out, err := run.Run("flatpak", "list", "--app", "--columns=application")
	if err != nil {
		return set
	}
	for _, name := range strings.Split(out, "\n") {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = true
		}
	}
	return set
}

func serviceEnabled(run Runner, name string) bool {
	// is-enabled devuelve 0 para enabled/enabled-runtime/static.
	_, err := run.Run("systemctl", "is-enabled", name)
	return err == nil
}

func userGroups(run Runner) map[string]bool {
	set := map[string]bool{}
	out, err := run.Run("id", "-nG")
	if err != nil {
		return set
	}
	for _, group := range strings.Fields(out) {
		set[group] = true
	}
	return set
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
