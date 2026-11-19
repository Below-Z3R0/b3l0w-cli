package systemdef

import "testing"

// TestNamesOrdenado fija que Names devuelve los perfiles en orden: el
// recorrido de un mapa no es determinista.
func TestNamesOrdenado(t *testing.T) {
	d := &Definition{Profiles: map[string]Profile{
		"zeta": {}, "alfa": {}, "medio": {},
	}}
	got := d.Names()
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("Names sin ordenar: %v", got)
		}
	}
}
