// Package systeminstall compara la definición del sistema contra la
// máquina y aplica lo que falta.
package systeminstall

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
	"github.com/Below-Z3R0/b3l0w-kit/cli"
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
func Build(profile systemdef.Profile, dotfiles string) *Plan {
	plan := &Plan{}
	installed := installedPackages()

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

	flatpaks := installedFlatpaks()
	for _, app := range profile.Flatpak {
		if !flatpaks[app] {
			plan.Flatpak = append(plan.Flatpak, app)
		}
	}

	for _, name := range profile.Own {
		if cli.CommandPath(name) == "" {
			plan.Own = append(plan.Own, name)
		}
	}

	for _, service := range profile.Services {
		if !serviceEnabled(service) {
			plan.Services = append(plan.Services, service)
		}
	}

	groups := userGroups()
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
		dest := cli.ExpandHome(file.Dest)
		plan.Files = append(plan.Files, FileStatus{
			Src:        src,
			Dest:       dest,
			SrcExists:  pathExists(src),
			DestExists: pathExists(dest),
		})
	}
	return plan
}

func installedPackages() map[string]bool {
	set := map[string]bool{}
	out, err := cli.Run("pacman", "-Qq")
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

func installedFlatpaks() map[string]bool {
	set := map[string]bool{}
	out, err := cli.Run("flatpak", "list", "--app", "--columns=application")
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

func serviceEnabled(name string) bool {
	// is-enabled devuelve 0 para enabled/enabled-runtime/static.
	_, err := cli.Run("systemctl", "is-enabled", name)
	return err == nil
}

func userGroups() map[string]bool {
	set := map[string]bool{}
	out, err := cli.Run("id", "-nG")
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
