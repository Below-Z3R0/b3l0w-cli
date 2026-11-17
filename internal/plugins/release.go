package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// releaseBranch es la rama que publica producción. El repo de plugins
// tiene una sola rama: la dev se borró después de mergearla a main.
const releaseBranch = "main"

// ReleaseOptions describe un release selectivo de plugins a main.
type ReleaseOptions struct {
	RepoDir      string
	ManifestPath string
	Slugs        []string
	TargetAPI    int
	DryRun       bool
	Push         bool
}

// Release publica producción desde la rama actual a main. Solo toca las
// carpetas de los plugins elegidos, limpia tests/docs/dev deps y
// regenera el catálogo desde lo que quedó en main. Un plugin nuevo
// entra con su carpeta y su fila; uno que no se releasea no aparece.
//
// El repo de plugins ya no tiene rama dev (se borró el 2026-11-04,
// después de mergearla a main). Por eso el release corre sobre main:
// prunea los artefactos de desarrollo en el lugar y commitea ahí. No
// hay checkout de otra rama porque no hay rama de trabajo a la que
// volver, y no hay merge dev->main porque dev no existe.
func Release(opts ReleaseOptions) error {
	repo := opts.RepoDir
	if repo == "" {
		return fmt.Errorf("falta el repo del registry")
	}
	branch, err := git(repo, "branch", "--show-current")
	if err != nil {
		return err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return fmt.Errorf("release necesita una rama: HEAD está desligado")
	}
	if branch != releaseBranch {
		return fmt.Errorf("release se corre desde %s (estás en %s)", releaseBranch, branch)
	}
	dirty, err := git(repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	var others []string
	for _, line := range strings.Split(dirty, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "catalog.toml") {
			continue
		}
		others = append(others, line)
	}
	if len(others) > 0 {
		return fmt.Errorf("working tree sucio en %s; commiteá o stasheá antes del release: %s", branch, strings.Join(others, ", "))
	}

	entries, err := Collect(repo)
	if err != nil {
		return err
	}
	targets, err := selectRelease(entries, opts.Slugs)
	if err != nil {
		return err
	}
	// El gate de plugin_api es opcional y explícito: sin --api se
	// publica lo que el plugin declara. La resolución de compatibilidad
	// la hace Noctalia con las filas [[plugin.release]] del catálogo.
	if opts.TargetAPI > 0 {
		for _, entry := range targets {
			if entry.PluginAPI > opts.TargetAPI {
				return fmt.Errorf("%s: plugin_api %d > %d; no lo liberes a main todavía",
					entry.ID, entry.PluginAPI, opts.TargetAPI)
			}
		}
	}
	if opts.DryRun {
		if opts.TargetAPI > 0 {
			fmt.Printf("  → target plugin_api: %d\n", opts.TargetAPI)
		}
		fmt.Printf("  → publicar en %s: %d plugins (sin tests/docs/dev deps)\n", branch, len(targets))
		for _, entry := range targets {
			fmt.Printf("  → %s/\n", entry.Path)
		}
		fmt.Println("  → catalog.toml desde " + branch + manifestNote(opts.ManifestPath))
		fmt.Println("  → commit en " + branch)
		return nil
	}

	// El release prunea sobre la rama actual: no hay dev de la que
	// copiar ni una rama de trabajo a la que volver.
	for _, entry := range targets {
		if err := pruneProduction(filepath.Join(repo, entry.Path)); err != nil {
			return fmt.Errorf("%s: %w", entry.Path, err)
		}
	}
	if _, err := git(repo, "add", "-A"); err != nil {
		return err
	}

	// El catálogo se arma con lo que realmente quedó en main. Si el
	// release es selectivo, no lista plugins que no están en main.
	prodEntries, err := Collect(repo)
	if err != nil {
		return err
	}
	if err := Build(repo, opts.ManifestPath, prodEntries); err != nil {
		return err
	}
	if _, err := git(repo, "add", "catalog.toml"); err != nil {
		return err
	}

	staged, _ := git(repo, "diff", "--cached", "--name-only")
	if strings.TrimSpace(staged) != "" {
		names := make([]string, 0, len(targets))
		for _, entry := range targets {
			names = append(names, entry.Path)
		}
		msg := "release: " + strings.Join(names, ", ")
		if _, err := git(repo, "commit", "-m", msg); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s: %s\n", branch, msg)
	} else {
		fmt.Printf("  · %s ya estaba al día\n", branch)
	}

	if opts.Push {
		if _, err := git(repo, "push", "origin", branch); err != nil {
			return err
		}
		fmt.Printf("  ✓ push origin %s\n", branch)
	}
	return nil
}

// pruneProduction deja solo lo que necesita Noctalia: plugin.toml,
// Luau y assets. Saca tests, docs, fixtures y archivos de desarrollo.
// Un .releaseignore opcional agrega rutas relativas a borrar.
func pruneProduction(base string) error {
	ignorePath := filepath.Join(base, ".releaseignore")
	var extra []string
	if data, err := os.ReadFile(ignorePath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			extra = append(extra, line)
		}
	}

	devDirs := []string{"tests", "docs", "fixtures", "node_modules", ".github", ".vscode", ".zed"}
	for _, dir := range devDirs {
		if err := os.RemoveAll(filepath.Join(base, dir)); err != nil {
			return err
		}
	}
	devFiles := []string{"package.json", "bun.lock", "bun.lockb", "deno.lock", ".gitignore"}
	for _, file := range devFiles {
		if err := os.Remove(filepath.Join(base, file)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "tests", "docs", "fixtures", "node_modules", ".github", ".vscode", ".zed":
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".py") {
			return os.Remove(path)
		}
		if strings.HasSuffix(name, ".md") && !strings.HasPrefix(strings.ToUpper(name), "LICENSE") {
			return os.Remove(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, rel := range extra {
		if err := os.RemoveAll(filepath.Join(base, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	if err := os.Remove(ignorePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func selectRelease(entries []Entry, slugs []string) ([]Entry, error) {
	if len(slugs) == 0 {
		return entries, nil
	}
	want := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		want[s] = true
	}
	var out []Entry
	for _, entry := range entries {
		if want[entry.Path] || want[entry.ID] {
			out = append(out, entry)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ningún plugin coincide con %s", strings.Join(slugs, ", "))
	}
	return out, nil
}

func manifestNote(path string) string {
	if path == "" {
		return ""
	}
	return " + plugins.json"
}

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
