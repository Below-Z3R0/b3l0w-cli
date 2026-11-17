package systemdef

import "testing"

// Este test pega a GitHub: no puede correr en una suite offline.
// Con -short se saltea; la suite completa lo corre a proposito.
func TestRemoteBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere red: GitHub")
	}
	data, err := RemoteBytes()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("remote len=%d", len(data))
	local := BuiltinBytes()
	t.Logf("builtin len=%d", len(local))
	if len(data) == 0 {
		t.Fatal("remoto vacío")
	}
}
