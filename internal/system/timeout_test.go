package system

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// TestRunTimeoutEnvuelveElCuelgue fija que un timeout se distingue de un
// fallo propio del comando.
func TestRunTimeoutEnvuelveElCuelgue(t *testing.T) {
	dir := t.TempDir()
	// un comando que duerme más que el timeout
	script := "#!/bin/sh\nsleep 5\n"
	if err := os.WriteFile(dir+"/lento", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := system.RunTimeout(50*time.Millisecond, dir+"/lento")
	if err == nil {
		t.Fatal("RunTimeout no devolvió error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, tenía que envolver DeadlineExceeded", err)
	}
}
