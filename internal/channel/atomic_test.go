package channel

import (
	"os"
	"strings"
	"testing"
)

// TestSetNoDejaTemporales fija que Set escribe con rename: no queda un
// .tmp ni un archivo a medias.
func TestSetNoDejaTemporales(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := Set(Edge); err != nil {
		t.Fatalf("Set: %v", err)
	}
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("el canal no se escribió: %v", err)
	}
	if strings.TrimSpace(string(data)) != Edge {
		t.Errorf("contenido = %q, quería %q", data, Edge)
	}
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("quedó el archivo temporal")
	}
}
