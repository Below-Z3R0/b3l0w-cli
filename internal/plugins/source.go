package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// reAutoUpdate matchea la clave exacta, no un prefijo: `auto_update_mode`
// y `auto_update_interval` también empiezan igual.
var reAutoUpdate = regexp.MustCompile(`(?m)^\s*auto_update\s*=\s*(true|false)\s*(?:#.*)?$`)

// SourceName es el nombre del source de Noctalia que publica el registry.
const SourceName = "b3l0w"

// RegistryURL es el repo que Noctalia clona como source.
const RegistryURL = "https://github.com/Below-Z3R0/b3l0w-plugins"

// EnsureSource agrega el source a Noctalia si falta.
func EnsureSource() error {
	if HasSource(SourceName) {
		return nil
	}
	if !system.HaveCommand("noctalia") {
		return fmt.Errorf("noctalia no está en PATH")
	}
	out, err := system.RunCombined("noctalia", "msg", "plugins", "source", "add", SourceName, "git", RegistryURL)
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

// HasSource dice si Noctalia tiene el source agregado.
func HasSource(name string) bool {
	if !system.HaveCommand("noctalia") {
		return false
	}
	out, err := system.RunCombined("noctalia", "msg", "plugins", "source", "list")
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

// AutoUpdate lee plugins.auto_update del estado (settings.toml) y, si
// no está, del config. Noctalia tiene que estar en "none" para que las
// actualizaciones las controle b3l0w.
func AutoUpdate() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	paths := []string{
		filepath.Join(home, ".local", "state", "noctalia", "settings.toml"),
		filepath.Join(home, ".config", "noctalia", "config.toml"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Match exacto de la clave: con HasPrefix, `auto_update_interval`
		// y `auto_update_mode` también matcheaban.
		if mm := reAutoUpdate.FindStringSubmatch(string(data)); mm != nil {
			return mm[1]
		}
	}
	return ""
}

// Sources devuelve los sources configurados en Noctalia.
func Sources() ([]string, error) {
	if !system.HaveCommand("noctalia") {
		return nil, nil
	}
	out, err := system.RunCombined("noctalia", "msg", "plugins", "source", "list")
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, out)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

// InstalledBySource agrupa los plugins habilitados por source.
func InstalledBySource() (map[string][]string, error) {
	if !system.HaveCommand("noctalia") {
		return nil, nil
	}
	out, err := system.RunCombined("noctalia", "msg", "plugins", "list")
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, out)
	}
	bySource := make(map[string][]string)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[len(fields)-1] != "enabled" {
			continue
		}
		source := strings.Trim(fields[1], "[]")
		if source == "" || source == "local" {
			continue
		}
		bySource[source] = append(bySource[source], fields[0])
	}
	return bySource, nil
}

// UpdateSources refresca cada source. Noctalia ya sabe resolver qué
// revisión instalar según su rango de API, así que el CLI no bloquea
// el update: solo informa qué plugins quedan en una versión anterior
// ("held back") porque la más nueva pide más API del que el shell
// soporta.
func UpdateSources() error {
	sources, err := Sources()
	if err != nil {
		return err
	}
	failed := 0
	for _, name := range sources {
		if err := UpdateSource(name); err != nil {
			fmt.Printf("  ! %s: %v\n", name, err)
			failed++
			continue
		}
		fmt.Printf("  ✓ %s actualizado\n", name)
	}
	if failed > 0 {
		return fmt.Errorf("%d sources fallaron", failed)
	}
	return nil
}

// UpdateSource repara el clon si quedó con archivos borrados y pide a
// Noctalia que refresque el source.
func UpdateSource(name string) error {
	// El docstring prometía reparar el clon y no lo hacía: un working
	// tree con todo el índice como "D" se quedaba así para siempre.
	repairSourceRepo(name)
	out, err := system.RunCombined("noctalia", "msg", "plugins", "update", name)
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
	out, err := system.RunCombined("git", "-C", repo, "status", "--porcelain")
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "D ") || strings.HasPrefix(line, " D ") {
			system.RunCombined("git", "-C", repo, "reset", "--hard", "HEAD")
			return
		}
	}
}
