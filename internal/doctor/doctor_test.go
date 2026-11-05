package doctor

import "testing"

func TestHasDependency(t *testing.T) {
	if !hasDependency("sh") {
		t.Error("sh debería estar en PATH")
	}
	if hasDependency("binario-que-no-existe-xyz-123") {
		t.Error("un binario inexistente no debería existir")
	}
}
