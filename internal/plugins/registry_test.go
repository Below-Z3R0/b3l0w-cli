package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := []struct {
		entry Entry
		want  string
	}{
		{Entry{ID: "b3l0w/wallpaper", Path: "b3l0w-wallpaper"}, "wallpaper"},
		{Entry{ID: "user/hover-zones", Path: "b3l0w-hover-zones"}, "hover-zones"},
		{Entry{ID: "sinbarra", Path: "x"}, "x"},
	}
	for _, c := range cases {
		if got := slug(c.entry); got != c.want {
			t.Errorf("slug(%+v) = %q, quería %q", c.entry, got, c.want)
		}
	}
}

func TestBuildCatalog(t *testing.T) {
	registry := t.TempDir()
	dir := filepath.Join(registry, "wallpaper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id = "b3l0w/wallpaper"
name = "b3l0w Wallpaper"
version = "0.4.1"
plugin_api = 43
author = "b3l0w"
license = "MIT"
icon = "movie"
description = "test"
tags = ["wallpaper", "desktop"]
dependencies = ["mpvpaper"]
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Manifest{Version: 1, Plugins: []Entry{{
		ID: "b3l0w/wallpaper", Name: "b3l0w Wallpaper",
		Repo: "Below-Z3R0/b3l0w-wallpaper", Path: "b3l0w-wallpaper",
		Version: "0.4.1", PluginAPI: 43,
	}}}
	if err := Build(registry, registry, m); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(registry, "catalog.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`id = "b3l0w/wallpaper"`,
		`version = "0.4.1"`,
		`plugin_api = 43`,
		`tags = ["wallpaper", "desktop"]`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("catalog.toml no contiene %q:\n%s", want, text)
		}
	}
}

func TestManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.json")
	m := &Manifest{Version: 1, Plugins: []Entry{
		{ID: "b3l0w/b", Path: "b", Version: "1.0.0", PluginAPI: 43},
		{ID: "b3l0w/a", Path: "a", Version: "1.0.0", PluginAPI: 43},
	}}
	if err := SaveManifest(path, m); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plugins) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(got.Plugins))
	}
	if got.Plugins[0].ID != "b3l0w/a" {
		t.Fatalf("no ordenó por id: %+v", got.Plugins)
	}
}
