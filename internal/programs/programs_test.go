package programs

import (
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

func TestParseVersion(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		{"kitty-shapes v1.0.0", "v1.0.0"},
		{"boot-preview v0.2.2", "v0.2.2"},
		{"v1.2.3", "v1.2.3"},
		{"prog 10.20.30 extra", "v10.20.30"},
		{"sin version", ""},
		{"", ""},
	}
	for _, caso := range casos {
		if got := ParseVersion(caso.in); got != caso.want {
			t.Errorf("ParseVersion(%q) = %q, quiero %q", caso.in, got, caso.want)
		}
	}
}

func TestLastLine(t *testing.T) {
	if got := lastLine("uno\ndos\n\n"); got != "dos" {
		t.Fatalf("lastLine = %q, quiero %q", got, "dos")
	}
	if got := lastLine("   "); got != "sin salida" {
		t.Fatalf("lastLine vacío = %q", got)
	}
}

func TestAssetURL(t *testing.T) {
	p := config.Program{Name: "kitty-shapes", Source: "github:Below-Z3R0/kitty-shapes", Asset: "kitty-shapes-linux-x64"}
	got, err := AssetURL(p)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	want := "https://github.com/Below-Z3R0/kitty-shapes/releases/latest/download/kitty-shapes-linux-x64"
	if got != want {
		t.Fatalf("AssetURL = %q, quiero %q", got, want)
	}
}

func TestAssetURLSinAsset(t *testing.T) {
	if _, err := AssetURL(config.Program{Name: "x", Source: "github:a/b"}); err == nil {
		t.Fatal("esperaba error sin asset")
	}
}

func TestAssetURLFuenteRara(t *testing.T) {
	if _, err := AssetURL(config.Program{Name: "x", Source: "gitlab:a/b", Asset: "a"}); err == nil {
		t.Fatal("esperaba error con una fuente que no es github:")
	}
}

func TestInstallPath(t *testing.T) {
	path, err := InstallPath("kitty-shapes")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if path == "" || !strings.HasSuffix(path, "/.local/bin/kitty-shapes") {
		t.Fatalf("InstallPath = %q", path)
	}
}
