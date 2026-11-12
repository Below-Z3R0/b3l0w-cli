package version

import "testing"

// La versión la pisa el build con -ldflags. Si fuera una función, el
// linker no la tocaría y el binario diría "dev" para siempre.
func TestVersionEsUnaVariableYNoEstaVacia(t *testing.T) {
	if Version == "" {
		t.Error("Version no debería estar vacía")
	}
	// En una compilación local no se inyecta nada: queda "dev".
	if Version != "dev" {
		t.Logf("Version = %q (inyectada por el build)", Version)
	}
}
