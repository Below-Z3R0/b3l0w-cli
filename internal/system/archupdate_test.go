package system

import "testing"

// Fixture del formato real de `cachy-update -l`.
const listado = `==> Looking for updates...

==> Packages:
7zip                                   26.04-1.1                   ->  26.05-1.1
alsa-card-profiles                     1:1.6.9-1                   ->  1:1.6.10-1
breeze                                 6.8.0-1.10

==> AUR packages:
visual-studio-code-bin                 1.99.0-1                    ->  1.100.0-1
`

func TestPendingSeparaRepoYAUR(t *testing.T) {
	repo, aur := Pending(listado)
	if repo != 3 {
		t.Fatalf("repo = %d, quiero 3", repo)
	}
	if aur != 1 {
		t.Fatalf("aur = %d, quiero 1", aur)
	}
}

func TestPendingSinUpdates(t *testing.T) {
	repo, aur := Pending("==> Looking for updates...\n\n==> Packages:\n")
	if repo != 0 || aur != 0 {
		t.Fatalf("esperaba 0 y 0, obtuve %d y %d", repo, aur)
	}
}

func TestPendingIgnoraEncabezados(t *testing.T) {
	repo, aur := Pending("==> Packages:\n==> AUR packages:\n")
	if repo != 0 || aur != 0 {
		t.Fatalf("contó encabezados: %d y %d", repo, aur)
	}
}

// Fixture de la salida REAL de `arch-update -l` (v4.4.6) en este
// equipo: el script colorea la salida aunque no sea un tty, así que la
// cabecera es "\x1b[1m\x1b[34m==>\x1b[0m\x1b[1m Packages:\x1b[0m" y las
// líneas de paquete traen los colores dentro del nombre y las
// versiones. Es un recorte: dos paquetes del repo, uno de AUR y uno de
// Flatpak (que NO es un paquete de pacman).
const salidaReal = "\x1b[1m\x1b[32m==>\x1b[0m\x1b[1m Looking for updates...\n\x1b[0m\n" +
	"\x1b[1m\x1b[34m==>\x1b[0m\x1b[1m Packages:\x1b[0m\n" +
	"\x1b[1m7zip\x1b[0m   26.\x1b[1m\x1b[31m\x1b[1m05-1.1\x1b[0m  ->  26.\x1b[1m\x1b[32m\x1b[1m06-1.1\x1b[0m\n" +
	"\x1b[1mbreeze\x1b[0m   6.8.0-1.10\n" +
	"\x1b[1m\x1b[34m==>\x1b[0m\x1b[1m AUR Packages:\x1b[0m\n" +
	"\x1b[1mvisual-studio-code-bin\x1b[0m   1.99.0-1  ->  1.100.0-1\n" +
	"\x1b[1m\x1b[34m==>\x1b[0m\x1b[1m Flatpak Packages:\x1b[0m\n" +
	"org.mozilla.firefox   141.0\n"

// TestPendingParseaLaSalidaRealDeArchUpdate: sin quitar los códigos de
// color, ninguna cabecera se reconoce y el conteo queda en cero: el
// doctor decía "sistema base: al día" con la máquina llena de
// actualizaciones.
func TestPendingParseaLaSalidaRealDeArchUpdate(t *testing.T) {
	repo, aur := Pending(salidaReal)
	if repo != 2 {
		t.Fatalf("repo = %d, quiero 2", repo)
	}
	if aur != 1 {
		t.Fatalf("aur = %d, quiero 1", aur)
	}
}

// TestPendingNoCuentaLosPaquetesDeFlatpak: la sección "Flatpak
// Packages:" no son paquetes de pacman. El doctor informa de los del
// repo y de los de AUR.
func TestPendingNoCuentaLosPaquetesDeFlatpak(t *testing.T) {
	list := "==> Packages:\n" +
		"7zip   26.04-1.1 -> 26.05-1.1\n" +
		"==> Flatpak Packages:\n" +
		"org.mozilla.firefox   141.0\n"
	repo, aur := Pending(list)
	if repo != 1 {
		t.Fatalf("repo = %d, quiero 1 (el de Flatpak no cuenta)", repo)
	}
	if aur != 0 {
		t.Fatalf("aur = %d, quiero 0", aur)
	}
}

// TestPendingNoCuentaLasLineasQueNoSonPaquetes: dentro de la sección,
// una nota de texto libre no es un paquete.
func TestPendingNoCuentaLasLineasQueNoSonPaquetes(t *testing.T) {
	list := "==> Packages:\n" +
		"7zip   26.04-1.1 -> 26.05-1.1\n" +
		"Total: 3 packages\n" +
		"some free text note\n"
	repo, aur := Pending(list)
	if repo != 1 {
		t.Fatalf("repo = %d, quiero 1", repo)
	}
	if aur != 0 {
		t.Fatalf("aur = %d, quiero 0", aur)
	}
}
