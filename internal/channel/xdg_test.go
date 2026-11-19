package channel

import (
	"path/filepath"
	"testing"
)

// TestPathRespetaXDG fija que el canal vive donde el resto de la config.
func TestPathRespetaXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := filepath.Join(dir, "b3l0w", "channel")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, quería %q", got, want)
	}
}
