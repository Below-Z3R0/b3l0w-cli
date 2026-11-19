package selfupdate

import (
	"strings"
	"testing"
)

// TestAPIUsaPerPage100 fija que la consulta pide 100 releases, no 30: el
// repo publica también los desktop-*, y cada uno ocupa una posición que el
// tag v* del CLI necesita.
func TestAPIUsaPerPage100(t *testing.T) {
	if !strings.Contains(api, "per_page=100") {
		t.Errorf("api = %q; con menos de 100, los desktop-* pueden dejar el tag v* fuera", api)
	}
}
