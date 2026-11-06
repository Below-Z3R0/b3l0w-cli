package programs

import "testing"

func TestPackageName(t *testing.T) {
	casos := map[string]string{
		"b3l0w":        "b3l0w-cli",
		"b3l0w-cli":    "b3l0w-cli",
		"boot-preview": "boot-preview",
		"kitty-shapes": "kitty-shapes",
	}
	for entrada, quiere := range casos {
		if got := PackageName(entrada); got != quiere {
			t.Errorf("PackageName(%q) = %q, quería %q", entrada, got, quiere)
		}
	}
}

// TestPackagedNoConfundeNombres: un programa que no está en [b3l0w] no
// puede tomarse por paquete.
func TestPackagedNoConfundeNombres(t *testing.T) {
	if Packaged("programa-que-no-existe-xyz") {
		t.Error("Packaged devolvió true para un paquete inexistente")
	}
}

func TestOwnedByB3l0w(t *testing.T) {
	casos := map[string]bool{
		"b3l0w-cli":     true,
		"b3l0w-keyring": true,
		"boot-preview":  true,
		"kitty-shapes":  true,
		"firefox":       false,
		"pacman":        false,
	}
	for pkg, quiere := range casos {
		if got := ownedByB3l0w(pkg); got != quiere {
			t.Errorf("ownedByB3l0w(%q) = %v, quería %v", pkg, got, quiere)
		}
	}
}
