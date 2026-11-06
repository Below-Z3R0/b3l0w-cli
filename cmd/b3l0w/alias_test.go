package main

import "testing"

// TestAliasVerb comprueba el mapeo de banderas cortas a verbos.
func TestAliasVerb(t *testing.T) {
	casos := []struct {
		categoria string
		bandera   string
		quiere    string
	}{
		// Comunes a todas las categorías.
		{"services", "-S", "install"},
		{"services", "-R", "remove"},
		{"services", "-Rns", "remove"},
		{"services", "-Q", "list"},
		{"services", "-Qe", "list"},
		{"services", "-U", "update"},
		{"services", "-Syu", "update"},
		{"plugins", "-S", "install"},
		{"plugins", "-R", "remove"},
		{"plugins", "-Q", "list"},
		{"plugins", "-U", "update"},

		// Buscar solo existe donde hay catálogo.
		{"plugins", "-Ss", "search"},
		{"plugins", "-Qs", "search"},
		{"services", "-Ss", ""},
		{"services", "-Qs", ""},

		// Verbos largos: la forma canónica no se toca.
		{"services", "install", ""},
		{"services", "remove", ""},
		{"plugins", "list", ""},

		// Basura.
		{"services", "-X", ""},
		{"plugins", "", ""},
	}

	for _, c := range casos {
		if got := aliasVerb(c.categoria, c.bandera); got != c.quiere {
			t.Errorf("aliasVerb(%q, %q) = %q, quería %q",
				c.categoria, c.bandera, got, c.quiere)
		}
	}
}

// TestAliasHelpMencionaSearchSoloEnPlugins evita que el help prometa
// algo que la categoría no hace.
func TestAliasHelpMencionaSearchSoloEnPlugins(t *testing.T) {
	if h := aliasHelp("plugins"); !contains(h, "-Ss search") {
		t.Errorf("plugins: %q, quería que mencione -Ss search", h)
	}
	if h := aliasHelp("services"); contains(h, "-Ss") {
		t.Errorf("services: %q, no debería mencionar -Ss", h)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
