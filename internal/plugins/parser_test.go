package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParsePluginMetaArrayMultilinea fija que un array TOML de varias
// líneas con comentarios adentro no mete el comentario como ítem.
func TestParsePluginMetaArrayMultilinea(t *testing.T) {
	toml := `id = "we/wallpaper"
name = "Wallpaper"
version = "1.0.0"
plugin_api = 43
dependencies = [
  "mpv",   # el player
  "yt-dlp", # el downloader
]
`
	meta, err := parsePluginMeta(toml)
	if err != nil {
		t.Fatalf("parsePluginMeta: %v", err)
	}
	if len(meta.Dependencies) != 2 {
		t.Fatalf("deps = %v, quería 2", meta.Dependencies)
	}
	if meta.Dependencies[0] != "mpv" || meta.Dependencies[1] != "yt-dlp" {
		t.Errorf("deps = %v", meta.Dependencies)
	}
}

// TestParsePluginMetaArrayConComaEnUnComentario fija que una coma dentro
// de un comentario no parte el array.
func TestParsePluginMetaArrayConComaEnUnComentario(t *testing.T) {
	toml := `id = "we/x"
name = "X"
version = "1.0.0"
plugin_api = 43
tags = ["a", "b"] # uno, dos
`
	meta, err := parsePluginMeta(toml)
	if err != nil {
		t.Fatalf("parsePluginMeta: %v", err)
	}
	if len(meta.Tags) != 2 {
		t.Errorf("tags = %v, quería 2", meta.Tags)
	}
}

// TestAutoUpdateNoConfundeClaves fija que auto_update_mode no gana por
// prefijo. Se prueba AutoUpdate() con un HOME temporal, no la regex: el
// bug estaba en el HasPrefix del escaneo.
func TestAutoUpdateNoConfundeClaves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "noctalia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// auto_update_mode va PRIMERO: con HasPrefix ganaba y devolvía "true".
	toml := "auto_update_mode = \"automatic\"\nauto_update = false\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := AutoUpdate(); got != "false" {
		t.Errorf("AutoUpdate() = %q, quería false (auto_update_mode no es auto_update)", got)
	}
}
