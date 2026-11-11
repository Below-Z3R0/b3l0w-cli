package paths

import (
	"os"
	"path/filepath"
)

// ConfigDir es donde vive el manifiesto, respetando XDG_CONFIG_HOME.
func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "b3l0w")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".b3l0w"
	}
	return filepath.Join(home, ".config", "b3l0w")
}

// ManifestPath es el archivo con los programas registrados.
//
// JSON y no TOML: el core no tiene dependencias, y este archivo se
// escribe solo (install lo agrega) aunque quede editable a mano.
func ManifestPath() string { return filepath.Join(ConfigDir(), "programs.json") }

// PluginsDir es el único lugar del que se ejecutan extensiones.
//
// No se busca en PATH a propósito: cualquier cosa que pueda escribir un
// b3l0w-algo en un directorio del PATH lo haría correr con este usuario,
// y este CLI pide sudo.
func PluginsDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "b3l0w", "plugins")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".b3l0w-plugins"
	}
	return filepath.Join(home, ".local", "lib", "b3l0w", "plugins")
}
