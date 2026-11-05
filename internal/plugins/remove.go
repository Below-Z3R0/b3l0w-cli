package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// Remove borra un plugin local. Si viene del source de Noctalia, no se
// puede borrar (se re-materializa): se deshabilita.
func Remove(manifestPath, id, pluginsDir string, dryRun bool) error {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	var entry *Entry
	for i := range m.Plugins {
		if m.Plugins[i].ID == id {
			entry = &m.Plugins[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("plugin %s no está en el manifiesto", id)
	}

	fromSource := HasSource(SourceName)
	removed := false
	for _, dir := range []string{slug(*entry), entry.Path} {
		target := filepath.Join(pluginsDir, dir)
		if _, err := os.Stat(target); err != nil {
			continue
		}
		if dryRun {
			fmt.Printf("  → borrar %s\n", target)
			removed = true
			continue
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		removed = true
	}

	if fromSource {
		if dryRun {
			fmt.Printf("  → deshabilitar %s en Noctalia\n", id)
			return nil
		}
		out, err := cli.RunCombined("noctalia", "msg", "plugins", "disable", id)
		if err != nil {
			return fmt.Errorf("noctalia disable: %v: %s", err, out)
		}
		fmt.Printf("  · %s: deshabilitado en el source (sigue en el registry)\n", id)
		return nil
	}
	if !removed {
		fmt.Printf("  · %s: no estaba instalado\n", id)
	}
	return nil
}
