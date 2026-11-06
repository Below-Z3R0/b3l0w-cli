package plugins

import (
	"os"
	"path/filepath"
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
