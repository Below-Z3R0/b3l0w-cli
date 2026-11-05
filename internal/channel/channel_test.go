package channel

import "testing"

func TestSetAndCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Set(Edge); err != nil {
		t.Fatal(err)
	}
	if got := Current(); got != Edge {
		t.Fatalf("Current() = %q, quería %q", got, Edge)
	}
	if err := Set(Stable); err != nil {
		t.Fatal(err)
	}
	if got := Current(); got != Stable {
		t.Fatalf("Current() = %q, quería %q", got, Stable)
	}
}

func TestSetInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Set("nightly"); err == nil {
		t.Fatal("esperaba error con un canal inválido")
	}
}
