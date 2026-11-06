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
		{Entry{ID: "b3l0w/wallpaper", Path: "wallpaper"}, "wallpaper"},
		{Entry{ID: "user/hover-zones", Path: "hover-zones"}, "hover-zones"},
		{Entry{ID: "sinbarra", Path: "x"}, "x"},
	}
	for _, c := range cases {
		if got := slug(c.entry); got != c.want {
			t.Errorf("slug(%+v) = %q, quería %q", c.entry, got, c.want)
		}
	}
}

func writePlugin(t *testing.T, registry, dir, id, version string, api int) {
	t.Helper()
	path := filepath.Join(registry, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `id = "` + id + `"
name = "` + dir + `"
version = "` + version + `"
plugin_api = ` + itoa(api) + `
author = "b3l0w"
license = "MIT"
icon = "movie"
description = "test"
tags = ["wallpaper", "desktop"]
dependencies = ["mpvpaper"]
`
	if err := os.WriteFile(filepath.Join(path, "plugin.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestCollectAndBuild(t *testing.T) {
	registry := t.TempDir()
	writePlugin(t, registry, "wallpaper", "b3l0w/wallpaper", "0.4.1", 43)
	writePlugin(t, registry, "audio", "b3l0w/audio", "1.0.0", 43)

	entries, err := Collect(registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(entries))
	}
	if entries[0].ID != "b3l0w/audio" || entries[0].Path != "audio" {
		t.Fatalf("no ordenó o no puso el path: %+v", entries[0])
	}
	for _, entry := range entries {
		if entry.Repo != DistributionRepo {
			t.Fatalf("%s: repo = %q", entry.ID, entry.Repo)
		}
	}

	manifestPath := filepath.Join(t.TempDir(), "plugins.json")
	if err := Build(registry, manifestPath, entries); err != nil {
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

	m, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Plugins) != 2 || m.Plugins[0].Repo != DistributionRepo || m.Plugins[0].Path != "audio" {
		t.Fatalf("manifest regenerado incorrecto: %+v", m.Plugins)
	}
}

func TestCollectRejectsSlugMismatch(t *testing.T) {
	registry := t.TempDir()
	writePlugin(t, registry, "wallpaper", "b3l0w/otro", "1.0.0", 43)
	if _, err := Collect(registry); err == nil {
		t.Fatal("esperaba error por id y carpeta distintos")
	}
}

func TestCollectAcceptsNewAPI(t *testing.T) {
	registry := t.TempDir()
	writePlugin(t, registry, "wallpaper", "b3l0w/wallpaper", "1.0.0", SupportedPluginAPI+1)
	if _, err := Collect(registry); err != nil {
		t.Fatalf("build no debe rechazar una API nueva: %v", err)
	}
}

func TestCheckCatalog(t *testing.T) {
	registry := t.TempDir()
	writePlugin(t, registry, "wallpaper", "b3l0w/wallpaper", "1.0.0", 43)
	entries, err := Collect(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := Build(registry, "", entries); err != nil {
		t.Fatal(err)
	}
	ok, err := CheckCatalog(registry, entries)
	if err != nil || !ok {
		t.Fatalf("catálogo recién generado debería estar al día: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(registry, "catalog.toml"), []byte("viejo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = CheckCatalog(registry, entries)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("un catálogo viejo no debería pasar el check")
	}
}

func TestSelectRelease(t *testing.T) {
	entries := []Entry{
		{ID: "b3l0w/audio", Path: "audio"},
		{ID: "b3l0w/wallpaper", Path: "wallpaper"},
	}
	if got, err := selectRelease(entries, nil); err != nil || len(got) != 2 {
		t.Fatalf("sin filtro debería devolver todo: %v %v", got, err)
	}
	if got, err := selectRelease(entries, []string{"wallpaper"}); err != nil || len(got) != 1 || got[0].Path != "wallpaper" {
		t.Fatalf("filtro por slug: %v %v", got, err)
	}
	if _, err := selectRelease(entries, []string{"no-existe"}); err == nil {
		t.Fatal("esperaba error con un slug inexistente")
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
