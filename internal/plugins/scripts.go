package plugins

import (
	"embed"
	"sort"
)

//go:embed scripts/*.sh
var scriptsFS embed.FS

// Scripts lista los backends embebidos. Se ejecutan con `b3l0w
// plugin-exec`; no se copian al filesystem.
func Scripts() ([]string, error) {
	entries, err := scriptsFS.ReadDir("scripts")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}
