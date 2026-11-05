package plugins

import "testing"

func TestDependencies(t *testing.T) {
	deps, err := Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) == 0 {
		t.Fatal("no leyó ningún plugin")
	}
	for id, list := range deps {
		t.Logf("%s: %v", id, list)
	}
}
