package apps

import (
	"testing"
	"time"
)

// TestHumanizarNoDaCeroHoras fija que una antigüedad de 30 minutos no se
// anuncie como "0 h": redondear al piso da un valor que no dice nada.
// Al alza, 30 minutos son "1 h"; cero es "menos de 1 h".
func TestHumanizarNoDaCeroHoras(t *testing.T) {
	casos := []struct {
		d      time.Duration
		quiere string
	}{
		{0, "menos de 1 h"},
		{30 * time.Minute, "1 h"},
		{90 * time.Minute, "2 h"},
		{26 * time.Hour, "1 día"},
		{50 * time.Hour, "2 días"},
	}
	for _, c := range casos {
		if got := humanizar(c.d); got != c.quiere {
			t.Errorf("humanizar(%v) = %q, quería %q", c.d, got, c.quiere)
		}
	}
}
