package pacman

import "testing"

// TestAgregarRepoRechazaUnaSeccionQueNoEsUnBloque fija que la firma
// exportada no acepta cualquier texto: escribir basura en
// /etc/pacman.conf no se puede deshacer.
func TestAgregarRepoRechazaUnaSeccionQueNoEsUnBloque(t *testing.T) {
	casos := []string{"", "texto suelto", "no es un bloque"}
	for _, section := range casos {
		if err := AgregarRepo("/tmp/pacman.conf", section); err == nil {
			t.Errorf("AgregarRepo aceptó %q", section)
		}
	}
}
