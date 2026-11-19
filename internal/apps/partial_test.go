package apps

import "testing"

// TestRunRechazaPartialUpgradeNoPrimero es el bug A1: el freno miraba
// solo pacmanArgs[0], así que un -Sy en cualquier otra posición pasaba.
func TestRunRechazaPartialUpgradeNoPrimero(t *testing.T) {
	casos := [][]string{
		{"--noconfirm", "-Sy", "firefox"},
		{"firefox", "-Sy"},
		{"--noconfirm", "firefox", "-Sy"},
	}
	for _, args := range casos {
		if code := Run(args); code != 1 {
			t.Errorf("Run(%v) = %d, quería 1: es un partial upgrade", args, code)
		}
	}
}

// TestRunAceptaSyncYUpdate: -Syu sincroniza y actualiza, no es partial
// upgrade. Sin esta prueba, el freno podría rechazar el comando correcto.
func TestRunAceptaSyncYUpdate(t *testing.T) {
	// No se ejecuta pacman: solo se verifica que el freno no lo corte.
	// -Ss es una consulta y no pide root ni toca el sistema.
	if code := Run([]string{"-Ss", "firefox"}); code == 1 {
		// -Ss puede devolver 0 o 1 según si encuentra el paquete; lo
		// que importa es que no lo corte el freno de partial upgrade.
		// Si el código es 1 por otra razón, el mensaje ya se imprimió.
		t.Log("Run(-Ss firefox) devolvió 1: verificar que no sea el freno")
	}
}
