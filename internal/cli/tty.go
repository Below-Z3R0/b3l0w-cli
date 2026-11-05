package cli

import (
	"fmt"
	"os"
	"os/exec"
)

// RestoreTTY devuelve la terminal a un estado usable después de un
// programa interactivo.
//
// cachy-update/arch-update usan alternate screen, ocultan el cursor y
// a veces dejan el modo raw. Si el CLI termina sin restaurar, el prompt
// del shell queda sin eco o invisible: parece que la terminal se colgó.
// `stty sane` cubre el modo; los escapes cubren cursor y atributos.
func RestoreTTY() {
	if !IsTTY() {
		return
	}
	if path, err := exec.LookPath("stty"); err == nil {
		cmd := exec.Command(path, "sane")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
	// Mostrar cursor y resetear atributos. No se limpia la pantalla: el
	// usuario tiene que poder leer el resultado del update.
	fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[0m")
	fmt.Fprintln(os.Stdout)
}
