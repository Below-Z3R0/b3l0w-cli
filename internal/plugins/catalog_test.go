package plugins

import "testing"

const catalogFixture = `[[plugin]]
id = "a/uno"
name = "Uno"
version = "1.0.0"
updated_at = 1790000000
added_at = 1780000000
author = "b3l0w"
license = "MIT"
icon = "puzzle"
description = "test"
plugin_api = 43
tags = ["demo"]

[[plugin.release]]
plugin_api = 36
version = "0.9.0"
updated_at = 1789000000
rev = "27abc83cdb31dc468e4d8751f5c192d7cd17d654"

[[plugin.release]]
plugin_api = 26
version = "0.8.0"
updated_at = 1788000000
rev = "cf84bca0131189c9ae4057136340418b335bf969"

[[plugin]]
id = "a/dos"
name = "Dos"
version = "2.0.0"
plugin_api = 51
`

func TestParseCatalog(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	if len(c.Entries) != 2 {
		t.Fatalf("esperaba 2 entradas, obtuve %d", len(c.Entries))
	}
	uno, ok := c.Entries["a/uno"]
	if !ok {
		t.Fatal("falta a/uno")
	}
	if uno.Version != "1.0.0" || uno.PluginAPI != 43 {
		t.Fatalf("a/uno: %+v", uno)
	}
	if len(uno.Releases) != 2 {
		t.Fatalf("esperaba 2 releases, obtuve %d", len(uno.Releases))
	}
	if uno.Releases[0].Version != "0.9.0" || uno.Releases[0].PluginAPI != 36 {
		t.Fatalf("release 0: %+v", uno.Releases[0])
	}
	if uno.Releases[0].Rev != "27abc83cdb31dc468e4d8751f5c192d7cd17d654" {
		t.Fatalf("rev: %+v", uno.Releases[0])
	}
}

func TestResolveTipWhenCompatible(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	res := c.Resolve("a/uno", 43)
	if !res.Installable {
		t.Fatal("debería ser instalable")
	}
	if res.HeldBack {
		t.Fatal("no debería estar held back con API 43")
	}
	if res.Version != "1.0.0" {
		t.Fatalf("versión: %q", res.Version)
	}
}

func TestResolveHeldBack(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	// API 30: la tip (43) no entra, pero la release de 26 sí.
	res := c.Resolve("a/uno", 30)
	if !res.Installable {
		t.Fatal("debería ser instalable")
	}
	if !res.HeldBack {
		t.Fatal("debería estar held back")
	}
	if res.Version != "0.8.0" {
		t.Fatalf("esperaba la release 0.8.0, obtuve %q", res.Version)
	}
	if res.TipVersion != "1.0.0" || res.TipAPI != 43 {
		t.Fatalf("la tip debería reportarse: %+v", res)
	}
}

func TestResolveNotInstallable(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	// API 10: no hay ninguna revisión compatible de a/uno.
	res := c.Resolve("a/uno", 10)
	if res.Installable {
		t.Fatal("no debería ser instalable")
	}
	if res.Version != "" {
		t.Fatalf("no debería proponer versión: %q", res.Version)
	}
}

func TestResolveUnknownPlugin(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	res := c.Resolve("no/existe", 43)
	if res.Installable {
		t.Fatal("un plugin fuera del catálogo no es instalable")
	}
}

func TestResolvePicksHighestCompatibleRelease(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	// API 40: la release de 36 es la más alta compatible.
	res := c.Resolve("a/uno", 40)
	if !res.HeldBack || res.Version != "0.9.0" {
		t.Fatalf("esperaba 0.9.0 held back: %+v", res)
	}
}

func TestResolveNewestPluginWithNoRelease(t *testing.T) {
	c := ParseCatalog(catalogFixture)
	// a/dos pide 51 y no tiene releases.
	res := c.Resolve("a/dos", 45)
	if res.Installable {
		t.Fatal("sin release compatible no es instalable")
	}
}
