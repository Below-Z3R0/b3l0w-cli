package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-kit/confirm"
	"github.com/Below-Z3R0/b3l0w-kit/output"
	"github.com/Below-Z3R0/b3l0w-kit/paths"
	kitsystem "github.com/Below-Z3R0/b3l0w-kit/system"
)

// Los subcomandos de `b3l0w plugins`: el catalogo de extensiones de
// Noctalia, su build y su release.

func pluginsCommand(args []string) int {
	if len(args) == 0 {
		return pluginsList(args)
	}
	if verb := aliasVerb("plugins", args[0]); verb != "" {
		args = append([]string{verb}, args[1:]...)
	}
	switch args[0] {
	case "sync":
		output.Fail("plugins sync fue eliminado; usá b3l0w plugins build")
		return 1
	case "build":
		return pluginsBuild(args[1:])
	case "release":
		return pluginsRelease(args[1:])
	case "enable":
		return pluginsEnable(args[1:])
	case "disable":
		return pluginsDisable(args[1:])
	case "install":
		return pluginsInstall(args[1:])
	case "update":
		return pluginsUpdate(args[1:])
	case "list":
		return pluginsList(args[1:])
	case "remove":
		return pluginsRemove(args[1:])
	case "info":
		return pluginsInfo(args[1:])
	case "search":
		return pluginsSearch(args[1:])
	}
	output.Fail("uso: b3l0w plugins [build|release|enable|disable|install|update|list|remove|info|search]\n" +
		aliasHelp("plugins"))
	return 1
}

// pluginsExtensions lista los binarios b3l0w-* del directorio privado.
//
// Son extensiones del CLI, no plugins de Noctalia: viven aparte
// porque `b3l0w-anilist` (el binario) y `b3l0w/anilist` (el plugin
// de Noctalia) son cosas distintas. Por eso tienen su propio comando.
func pluginsExtensions() int {
	names := plugins.Discover()
	output.Step("Extensiones del CLI")
	if len(names) == 0 {
		fmt.Printf("  (ninguna en %s)\n", paths.PluginsDir())
		return 0
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("  b3l0w-%s\n", name)
	}
	fmt.Println()
	fmt.Println("  Se ejecutan con: b3l0w <nombre> <args>")
	return 0
}

func pluginsBuild(args []string) int {
	_, registry, manifest, _, rest, dryRun := pluginPaths(args)
	check := false
	for _, arg := range rest {
		if arg == "--check" {
			check = true
		}
	}
	entries, err := plugins.Collect(registry)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if check {
		ok, err := plugins.CheckCatalog(registry, entries)
		if err != nil {
			output.Fail(err.Error())
			return 1
		}
		if !ok {
			output.Fail("catalog.toml desactualizado; corré b3l0w plugins build")
			return 1
		}
		output.Ok("catalog.toml al día")
		return 0
	}
	if dryRun {
		output.Step(fmt.Sprintf("%d plugins: se escribirian catalog.toml y plugins.json", len(entries)))
		output.Ok("dry-run: no se escribió nada")
		return 0
	}
	if err := plugins.Build(registry, manifest, entries); err != nil {
		output.Fail(err.Error())
		return 1
	}
	output.Ok(fmt.Sprintf("%d plugins: catalog.toml y plugins.json actualizados", len(entries)))
	return 0
}

func pluginsRelease(args []string) int {
	_, registry, manifest, _, rest, dryRun := pluginPaths(args)
	push := false
	yes := false
	targetAPI := 0
	var slugs []string
	for _, arg := range rest {
		switch {
		case arg == "--push":
			push = true
		case arg == "--yes":
			yes = true
		case strings.HasPrefix(arg, "--api="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--api="))
			if err != nil || n < 1 {
				output.Fail("--api debe ser un entero positivo")
				return 1
			}
			targetAPI = n
		case strings.HasPrefix(arg, "--plugin="):
			slugs = append(slugs, strings.TrimPrefix(arg, "--plugin="))
		case strings.HasPrefix(arg, "--"):
			output.Fail("flag desconocido: " + arg)
			return 1
		default:
			slugs = append(slugs, arg)
		}
	}
	if !yes && !dryRun && !confirm.Confirm("Release plugins a main? [Y/n]", yes, true) {
		return 0
	}
	if err := plugins.Release(plugins.ReleaseOptions{
		RepoDir:      registry,
		ManifestPath: manifest,
		Slugs:        slugs,
		TargetAPI:    targetAPI,
		DryRun:       dryRun,
		Push:         push,
	}); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsEnable(args []string) int {
	return pluginsToggle(args, true)
}

func pluginsDisable(args []string) int {
	return pluginsToggle(args, false)
}

func pluginsToggle(args []string, enable bool) int {
	_, _, manifest, _, rest, dryRun := pluginPaths(args)
	if len(rest) == 0 {
		output.Fail("falta el id: b3l0w plugins enable|disable <autor/nombre>")
		return 1
	}
	id := rest[0]
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	found := false
	for _, entry := range m.Plugins {
		if entry.ID == id {
			found = true
			break
		}
	}
	if !found {
		output.Fail("plugin no encontrado: " + id)
		return 1
	}
	action := "disable"
	if enable {
		action = "enable"
	}
	if dryRun {
		fmt.Printf("  → %s %s\n", action, id)
		return 0
	}
	if !kitsystem.HaveCommand("noctalia") {
		output.Fail("noctalia no está en PATH")
		return 1
	}
	out, err := kitsystem.RunCombined("noctalia", "msg", "plugins", action, id)
	if err != nil {
		output.Fail(fmt.Sprintf("noctalia %s: %v: %s", action, err, out))
		return 1
	}
	output.Ok(fmt.Sprintf("%s %s", action, id))
	return 0
}

func pluginsInstall(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	id := ""
	if len(rest) > 0 {
		id = rest[0]
	}
	if err := plugins.Install(m, id, noctalia, dryRun); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsUpdate(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	id := ""
	if len(rest) > 0 {
		id = rest[0]
	}
	if err := plugins.Update(m, id, noctalia, dryRun); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsInfo(args []string) int {
	_, _, manifest, noctalia, rest, _ := pluginPaths(args)
	if len(rest) == 0 {
		output.Fail("falta el id: b3l0w plugins info <autor/nombre>")
		return 1
	}
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	for _, entry := range m.Plugins {
		if entry.ID != rest[0] {
			continue
		}
		output.Step(entry.ID)
		fmt.Printf("  nombre:     %s\n", entry.Name)
		fmt.Printf("  versión:    %s\n", entry.Version)
		fmt.Printf("  plugin_api: %d\n", entry.PluginAPI)
		fmt.Printf("  repo:       %s\n", entry.Repo)
		fmt.Printf("  path:       %s\n", entry.Path)
		local := plugins.InstalledVersion(noctalia, entry)
		if local == "" {
			fmt.Println("  instalado:  no")
		} else {
			state := "disabled"
			if plugins.Enabled(entry.ID) {
				state = "enabled"
			}
			fmt.Printf("  instalado:  %s (%s, %s)\n", local, plugins.Origin(noctalia, entry), state)
		}
		return 0
	}
	output.Fail("plugin no encontrado: " + rest[0])
	return 1
}

func pluginsSearch(args []string) int {
	_, _, manifest, _, rest, _ := pluginPaths(args)
	q := strings.ToLower(strings.Join(rest, " "))
	if q == "" {
		output.Fail("falta la búsqueda: b3l0w plugins search <texto>")
		return 1
	}
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	found := 0
	for _, entry := range m.Plugins {
		if strings.Contains(strings.ToLower(entry.ID+" "+entry.Name), q) {
			fmt.Printf("  %-24s %s\n", entry.ID, entry.Version)
			found++
		}
	}
	if found == 0 {
		output.Warn("sin resultados")
	}
	return 0
}

func pluginsRemove(args []string) int {
	_, _, manifest, noctalia, rest, dryRun := pluginPaths(args)
	if len(rest) == 0 {
		output.Fail("falta el id: b3l0w plugins remove <autor/nombre>")
		return 1
	}
	if err := plugins.Remove(manifest, rest[0], noctalia, dryRun); err != nil {
		output.Fail(err.Error())
		return 1
	}
	return 0
}

func pluginsList(args []string) int {
	_, _, manifest, noctalia, _, _ := pluginPaths(args)
	m, code := loadPluginManifest(manifest)
	if m == nil {
		return code
	}
	output.Step("Plugins del registry")
	for _, entry := range m.Plugins {
		local := plugins.InstalledVersion(noctalia, entry)
		origin := plugins.Origin(noctalia, entry)
		state := "disabled"
		if plugins.Enabled(entry.ID) {
			state = "enabled"
		}
		switch {
		case local == "":
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, output.Dim("no instalado"))
		case local != entry.Version:
			// La versión instalada difiere de la del catálogo: Noctalia
			// chose una revisión compatible con su rango de API.
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, local, output.Dim(fmt.Sprintf("%s/%s (catálogo %s)", origin, state, entry.Version)))
		default:
			fmt.Printf("  %-24s %-8s %s\n", entry.ID, entry.Version, output.Dim(fmt.Sprintf("ok %s/%s", origin, state)))
		}
	}
	return 0
}
