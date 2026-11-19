package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDependencies fija la lectura de dependencias sin red ni home real:
// manifiesto y directorio explícitos.
func TestDependencies(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "we")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "id = \"we\"\nname = \"We\"\nversion = \"1.0.0\"\ndependencies = [\"mpv\", \"yt-dlp\"]\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{Version: 1, Plugins: []Entry{{ID: "we", Path: "we"}}}
	deps := DependenciesFrom(m, dir)
	got, ok := deps["we"]
	if !ok {
		t.Fatalf("no leyó el plugin: %v", deps)
	}
	if len(got) != 2 || got[0] != "mpv" || got[1] != "yt-dlp" {
		t.Errorf("deps = %v, quería [mpv yt-dlp]", got)
	}
}

// TestDependenciesSinPluginToml es el contraste: un plugin sin metadatos
// no aparece, y no es un error.
func TestDependenciesSinPluginToml(t *testing.T) {
	m := &Manifest{Version: 1, Plugins: []Entry{{ID: "x", Path: "x"}}}
	if deps := DependenciesFrom(m, t.TempDir()); len(deps) != 0 {
		t.Errorf("deps = %v, quería vacío", deps)
	}
}
