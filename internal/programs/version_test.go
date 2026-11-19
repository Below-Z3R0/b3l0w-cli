package programs

import "testing"

// TestParseVersionNoConfundeFechasConVersiones es el bug M1: la regex
// aceptaba cualquier terna de enteros, así que una fecha de un tag
// ("desktop-2026.11.06-r1") o un copyright se leían como versión.
//
// El contrato del módulo es "prog vX.Y.Z": una versión real, no una
// fecha.
func TestParseVersionNoConfundeFechasConVersiones(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		{"kitty-shapes 2026.11.06-r1", ""},
		{"desktop 2026.11.06", ""},
		{"Copyright 2024.01.15", ""},
		{"tool v1.2.3", "v1.2.3"},
		{"tool 1.2.3", "v1.2.3"},
		{"kitty-shapes v1.0.0", "v1.0.0"},
		{"b3l0w-cli v0.5.0-dev", "v0.5.0"},
		{"prog 10.20.30 extra", "v10.20.30"},
		{"sin version", ""},
		{"", ""},
	}
	for _, c := range casos {
		if got := ParseVersion(c.in); got != c.want {
			t.Errorf("ParseVersion(%q) = %q, quiero %q", c.in, got, c.want)
		}
	}
}
