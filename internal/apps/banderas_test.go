package apps

import "testing"

// TestBanderasDescompone fija el parseo letra por letra: -Syu es -S+-y+-u.
func TestBanderasDescompone(t *testing.T) {
	casos := []struct {
		flag      string
		parcial   bool
		actualiza bool
	}{
		{"-Sy", true, false},
		{"-Syy", true, false},
		{"-Syu", false, true},
		{"-Syyu", false, true},
		{"-S", false, false},
		{"-Xy", false, false},
		{"-Ss", false, false},
	}
	for _, c := range casos {
		if got := partialUpgrade(c.flag); got != c.parcial {
			t.Errorf("partialUpgrade(%q) = %v, quería %v", c.flag, got, c.parcial)
		}
		if got := actualiza(c.flag); got != c.actualiza {
			t.Errorf("actualiza(%q) = %v, quería %v", c.flag, got, c.actualiza)
		}
	}
}

// TestBanderasIgnoraLasLargas fija que --sync no se parsea como -S+-y+n+c.
func TestBanderasIgnoraLasLargas(t *testing.T) {
	if partialUpgrade("--sync") {
		t.Error("--sync no es un partial upgrade")
	}
}
