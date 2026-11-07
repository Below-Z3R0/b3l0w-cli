package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// repoSection es la sección que se agrega a pacman.conf.
const repoSection = `
[b3l0w]
SigLevel = Required DatabaseOptional
Server = https://raw.githubusercontent.com/Below-Z3R0/b3l0w-packages/main/x86_64
`

// pacmanConf es variable para los tests.
var pacmanConf = "/etc/pacman.conf"

// ensureRepo deja el repo [b3l0w] configurado en pacman.conf.
//
// Pide confirmación y muestra el bloque exacto antes de tocar un
// archivo del sistema. Si no hay TTY, se omite.
func ensureRepo() {
	if repoConfigurado() {
		cli.Ok("repo: [b3l0w] ya está en pacman.conf")
		return
	}

	cli.Step("Configurando el repo [b3l0w]")
	fmt.Printf("  Se agrega a %s:\n\n", pacmanConf)
	for _, line := range strings.Split(strings.TrimSpace(repoSection), "\n") {
		fmt.Printf("    %s\n", line)
	}
	fmt.Println()

	if !cli.Confirm("¿Agregar el repo?", true, false) {
		cli.Warn("repo: se omite")
		return
	}

	// sudo tee -a: la redirección la haría el shell como usuario, así
	// que el append tiene que hacerlo tee con privilegios.
	cmd := exec.Command("sudo", "tee", "-a", pacmanConf)
	cmd.Stdin = strings.NewReader(repoSection)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cli.Warn("repo: no se pudo escribir " + pacmanConf)
		return
	}

	if repoConfigurado() {
		cli.Ok("repo: [b3l0w] agregado")
		fmt.Println("  Para sincronizar: sudo pacman -Sy")
		return
	}
	cli.Warn("repo: no se pudo verificar el cambio")
}

// repoConfigurado dice si la sección ya está.
func repoConfigurado() bool {
	data, err := os.ReadFile(pacmanConf)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "[b3l0w]")
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
	cli.Warn("hay dos b3l0w: el paquete y " + viejo)
	fmt.Printf("  borrá el viejo: rm %s\n", viejo)
}
