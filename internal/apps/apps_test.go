package apps

import (
	"testing"
	"time"
)

func TestPartialUpgrade(t *testing.T) {
	casos := map[string]bool{
		"-Sy":   true,
		"-Syy":  true,
		"-Syu":  false, // sync + upgrade: correcto
		"-Suy":  false,
		"-S":    false,
		"-Ss":   false,
		"-Q":    false,
		"-Syu ": false,
	}
	for flag, quiere := range casos {
		if got := partialUpgrade(flag); got != quiere {
			t.Errorf("partialUpgrade(%q) = %v, quería %v", flag, got, quiere)
		}
	}
}

func TestInstala(t *testing.T) {
	casos := map[string]bool{
		"-S":   true,
		"-Syu": true,
		"-Ss":  false, // busca
		"-Si":  false, // info
		"-Q":   false,
		"-R":   false,
	}
	for flag, quiere := range casos {
		if got := instala(flag); got != quiere {
			t.Errorf("instala(%q) = %v, quería %v", flag, got, quiere)
		}
	}
}

func TestNeedsRoot(t *testing.T) {
	casos := map[string]bool{
		"-S":   true,
		"-Syu": true,
		"-R":   true,
		"-Rns": true,
		"-U":   true,
		"-Ss":  false, // consulta
		"-Si":  false,
		"-Q":   false,
	}
	for flag, quiere := range casos {
		if got := needsRoot(flag); got != quiere {
			t.Errorf("needsRoot(%q) = %v, quería %v", flag, got, quiere)
		}
	}
}

func TestHumanizar(t *testing.T) {
	casos := []struct {
		d      time.Duration
		quiere string
	}{
		{3 * time.Hour, "3 h"},
		{26 * time.Hour, "1 día"},
		{50 * time.Hour, "2 días"},
		{10 * 24 * time.Hour, "10 días"},
	}
	for _, c := range casos {
		if got := humanizar(c.d); got != c.quiere {
			t.Errorf("humanizar(%v) = %q, quería %q", c.d, got, c.quiere)
		}
	}
}

// TestOldestSyncDB lee las DB reales de la máquina: si hay pacman, hay
// al menos una, y la edad no puede ser negativa.
func TestOldestSyncDB(t *testing.T) {
	edad, db, err := OldestSyncDB()
	if err != nil {
		t.Skipf("sin DB de pacman: %v", err)
	}
	if db == "" {
		t.Error("no devolvió el nombre de la DB más vieja")
	}
	if edad < 0 {
		t.Errorf("edad negativa: %v", edad)
	}
}
