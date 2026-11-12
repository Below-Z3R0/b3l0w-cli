package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPruneProduction(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"plugin.toml",
		"main.luau",
		"assets/icon.png",
		"LICENSE",
		"tests/main_spec.luau",
		"docs/guide.md",
		"README.md",
		"package.json",
		"tool.py",
		"fixtures/cards.json",
		"private/secret.txt",
		".releaseignore",
		".gitignore",
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "x"
		if name == ".releaseignore" {
			content = "# dev only\nprivate/\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := pruneProduction(dir); err != nil {
		t.Fatal(err)
	}

	keep := []string{"plugin.toml", "main.luau", "assets/icon.png", "LICENSE"}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("debería quedar %s: %v", name, err)
		}
	}
	gone := []string{
		"tests", "docs", "README.md", "package.json", "tool.py",
		"fixtures", "private", ".releaseignore", ".gitignore",
	}
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("no debería quedar %s", name)
		}
	}
}

// mustGit corre git contra el repo y falla el test si sale mal.
func mustGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := git(repo, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// newPluginRepo arma un repo git de plugins con una sola rama (main),
// como quedó el repo real después de borrar dev.
func newPluginRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustGit(t, repo, "init")
	mustGit(t, repo, "symbolic-ref", "HEAD", "refs/heads/main")
	mustGit(t, repo, "config", "user.email", "test@b3l0w.invalid")
	mustGit(t, repo, "config", "user.name", "b3l0w test")
	mustGit(t, repo, "config", "commit.gpgsign", "false")
	return repo
}

// commitRepo deja el repo commiteado y limpio.
func commitRepo(t *testing.T, repo, msg string) {
	t.Helper()
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", msg)
}

// TestReleaseFromMain cubre el caso real: el repo tiene una sola rama
// (main) y el release tiene que publicar desde ahí, sin rama dev.
func TestReleaseFromMain(t *testing.T) {
	repo := newPluginRepo(t)
	writePlugin(t, repo, "wallpaper", "b3l0w/wallpaper", "1.0.0", 43)
	writePlugin(t, repo, "audio", "b3l0w/audio", "1.0.0", 43)
	for _, dir := range []string{"wallpaper", "audio"} {
		if err := os.MkdirAll(filepath.Join(repo, dir, "tests"), 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			filepath.Join(dir, "tests", "spec.luau"): "x",
			filepath.Join(dir, "README.md"):          "x",
			filepath.Join(dir, "package.json"):       "{}",
		}
		for rel, content := range files {
			if err := os.WriteFile(filepath.Join(repo, rel), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	commitRepo(t, repo, "init")

	err := Release(ReleaseOptions{RepoDir: repo, Slugs: []string{"wallpaper"}})
	if err != nil {
		t.Fatalf("release desde main: %v", err)
	}

	// El plugin elegido quedó en producción.
	for _, gone := range []string{"tests", "README.md", "package.json"} {
		if _, err := os.Stat(filepath.Join(repo, "wallpaper", gone)); err == nil {
			t.Errorf("wallpaper/%s no debería quedar", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "wallpaper", "plugin.toml")); err != nil {
		t.Errorf("wallpaper/plugin.toml no debería desaparecer: %v", err)
	}
	// El que no se releaseó quedó intacto.
	if _, err := os.Stat(filepath.Join(repo, "audio", "tests")); err != nil {
		t.Errorf("audio/tests no debería tocarse: %v", err)
	}

	// El catálogo se regeneró con lo que quedó en main.
	catalog, err := os.ReadFile(filepath.Join(repo, "catalog.toml"))
	if err != nil {
		t.Fatalf("no se escribió catalog.toml: %v", err)
	}
	for _, want := range []string{`id = "b3l0w/wallpaper"`, `id = "b3l0w/audio"`, `plugin_api = 43`} {
		if !strings.Contains(string(catalog), want) {
			t.Errorf("catalog.toml no contiene %q:\n%s", want, catalog)
		}
	}

	// El commit quedó en main y el repo limpio.
	log := mustGit(t, repo, "log", "--oneline")
	if !strings.Contains(log, "release: wallpaper") {
		t.Errorf("no se commiteó el release:\n%s", log)
	}
	if got := strings.TrimSpace(mustGit(t, repo, "branch", "--show-current")); got != "main" {
		t.Errorf("el release cambió de rama: %q", got)
	}
	if status := strings.TrimSpace(mustGit(t, repo, "status", "--porcelain")); status != "" {
		t.Errorf("el repo quedó sucio:\n%s", status)
	}
}

// TestReleaseRejectsOtherBranch verifica que el release no corre desde
// una rama que no publica (la dev vieja tenía que seguir rechazada).
func TestReleaseRejectsOtherBranch(t *testing.T) {
	repo := newPluginRepo(t)
	writePlugin(t, repo, "wallpaper", "b3l0w/wallpaper", "1.0.0", 43)
	commitRepo(t, repo, "init")
	mustGit(t, repo, "checkout", "-b", "dev")

	err := Release(ReleaseOptions{RepoDir: repo})
	if err == nil {
		t.Fatal("el release debería rechazar una rama que no es main")
	}
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("el error debería decir que va desde main: %v", err)
	}
}

// TestReleaseRejectsDirtyTree: con cambios sin commitear no se publica,
// así el commit del release solo lleva lo que el release cambió.
func TestReleaseRejectsDirtyTree(t *testing.T) {
	repo := newPluginRepo(t)
	writePlugin(t, repo, "wallpaper", "b3l0w/wallpaper", "1.0.0", 43)
	commitRepo(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "wallpaper", "main.luau"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Release(ReleaseOptions{RepoDir: repo})
	if err == nil {
		t.Fatal("esperaba error de working tree sucio")
	}
	if !strings.Contains(err.Error(), "sucio") {
		t.Fatalf("el error debería avisar del working tree sucio: %v", err)
	}
}

// TestReleaseDryRunKeepsRepo: el dry-run no toca nada.
func TestReleaseDryRunKeepsRepo(t *testing.T) {
	repo := newPluginRepo(t)
	writePlugin(t, repo, "wallpaper", "b3l0w/wallpaper", "1.0.0", 43)
	if err := os.MkdirAll(filepath.Join(repo, "wallpaper", "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, repo, "init")

	if err := Release(ReleaseOptions{RepoDir: repo, DryRun: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "wallpaper", "tests")); err != nil {
		t.Errorf("el dry-run no debería pruneear: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "catalog.toml")); !os.IsNotExist(err) {
		t.Error("el dry-run no debería escribir catalog.toml")
	}
}
