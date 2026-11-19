// Package pacman maneja la configuración del repo [b3l0w] en pacman.conf.
//
// Escribir en /etc es acceso al sistema, no presentación: vive acá y no
// en cmd/, que solo parsea, confirma y muestra.
package pacman

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Section es el bloque que se agrega a pacman.conf.
const Section = `
[b3l0w]
SigLevel = Required DatabaseOptional
Server = https://raw.githubusercontent.com/Below-Z3R0/b3l0w-packages/main/x86_64
`

// Configurado dice si la sección [b3l0w] ya está en el archivo.
//
// Busca la línea exacta: "[b3l0w-extra]" es otra sección y
// "# [b3l0w]" está comentada (el usuario desactivó el repo a mano).
// Con un substring, las dos contaban como configuradas y el diagnóstico
// mentía.
//
// Un archivo que no se puede leer cuenta como no configurado: el
// llamador decide si eso es un error o una razón para intentar.
func Configurado(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "[b3l0w]" {
			return true
		}
	}
	return false
}

// AgregarRepo añade la sección al final del archivo.
//
// Usa `sudo tee -a` y no `os.OpenFile`: la redirección la haría el shell
// como usuario, así que el append necesita privilegios. Si sudo falla,
// devuelve el error para que el llamador avise; no deja el archivo a
// medio escribir porque tee escribe de una sola vez.
func AgregarRepo(path, section string) error {
	// La sección tiene que ser un bloque de pacman.conf, no cualquier
	// texto: la firma es exportada y un llamador equivocado escribiría
	// basura en /etc/pacman.conf.
	if !strings.HasPrefix(strings.TrimSpace(section), "[") {
		return fmt.Errorf("la sección no es un bloque de pacman.conf: %q", section)
	}
	cmd := exec.Command("sudo", "tee", "-a", path)
	cmd.Stdin = strings.NewReader(section)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", path, err)
	}
	return nil
}
