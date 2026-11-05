package plugins

import "testing"

func TestExecScriptMissing(t *testing.T) {
	if err := ExecScript("no-existe-xyz", nil); err == nil {
		t.Fatal("esperaba error con un script inexistente")
	}
}
