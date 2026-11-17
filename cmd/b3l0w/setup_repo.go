package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/pacman"
	"github.com/Below-Z3R0/b3l0w-cli/kit/confirm"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
)

// pacmanConf es variable para los tests.
var pacmanConf = "/etc/pacman.conf"

// ensureRepo deja el repo [b3l0w] configurado en pacman.conf.
//
// Pide confirmación y muestra el bloque exacto antes de tocar un
// archivo del sistema. Si no hay TTY, se omite.
func ensureRepo() {
	if pacman.Configurado(pacmanConf) {
		output.Ok("repo: [b3l0w] ya está en pacman.conf")
		return
	}

	output.Step("Configurando el repo [b3l0w]")
	fmt.Printf("  Se agrega a %s:\n\n", pacmanConf)
	for _, line := range strings.Split(strings.TrimSpace(pacman.Section), "\n") {
		fmt.Printf("    %s\n", line)
	}
	fmt.Println()

	if !confirm.Confirm("¿Agregar el repo?", true, false) {
		output.Warn("repo: se omite")
		return
	}

	if err := pacman.AgregarRepo(pacmanConf, pacman.Section); err != nil {
		output.Warn("repo: " + err.Error())
		return
	}

	if pacman.Configurado(pacmanConf) {
		output.Ok("repo: [b3l0w] agregado")
		fmt.Println("  Para sincronizar: sudo pacman -Sy")
		return
	}
	output.Warn("repo: no se pudo verificar el cambio")
}

// avisarBinarioViejo detecta el caso que confunde a todos: el paquete
// instala /usr/bin/b3l0w, pero ~/.local/bin/b3l0w gana en PATH y se
// sigue ejecutando el viejo.
func avisarBinarioViejo() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	viejo := filepath.Join(home, ".local", "bin", "b3l0w")
	if _, err := os.Stat(viejo); err != nil {
		return
	}
	if _, err := os.Stat("/usr/bin/b3l0w"); err != nil {
		return
	}
	output.Warn("hay dos b3l0w: el paquete y " + viejo)
	fmt.Printf("  borrá el viejo: rm %s\n", viejo)
}
