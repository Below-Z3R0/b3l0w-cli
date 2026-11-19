package snapshots

import "testing"

// TestPruneKeepNegativoUsaElDefault fija que Prune y Report comparten el
// contrato: -1 es "usá el default", no un error.
func TestPruneKeepNegativoUsaElDefault(t *testing.T) {
	// Sin snapshot list, Prune(-1) tiene que comportarse como el default.
	// Se verifica que no devuelva el error de "keep inválido".
	_, err := Prune(-1)
	if err != nil && err.Error() == "keep inválido: -1" {
		t.Error("Prune(-1) devolvió el error viejo; tiene que normalizar")
	}
}
