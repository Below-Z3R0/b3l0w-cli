package systemdef

import "testing"

func TestRemoteBytes(t *testing.T) {
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
