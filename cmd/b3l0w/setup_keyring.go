package main

import (
	"os"
	"strings"

	"github.com/Below-Z3R0/b3l0w-kit/output"
	kitsystem "github.com/Below-Z3R0/b3l0w-kit/system"
	"github.com/Below-Z3R0/b3l0w-kit/verify"
)

// ensureKeyring deja la clave del repo [b3l0w] en el keyring de pacman.
//
// En una instalación nueva, `SigLevel = Required` rechaza todo paquete
// del repo hasta que la clave esté importada y firmada localmente. El
// paquete b3l0w-keyring hace esto solo; acá queda el camino para
// quien instaló el CLI por asset.
//
// No pide sudo si la clave ya está.
func ensureKeyring() {
	if !kitsystem.HaveCommand("pacman-key") {
		return
	}
	if keyringHasKey() {
		output.Ok("keyring: clave de [b3l0w] presente")
		return
	}

	tmp, err := os.CreateTemp("", "b3l0w-key-*.asc")
	if err != nil {
		output.Warn("keyring: " + err.Error())
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(verify.PublicKey()); err != nil {
		tmp.Close()
		output.Warn("keyring: " + err.Error())
		return
	}
	tmp.Close()

	output.Step("Importando la clave de [b3l0w]")
	if err := kitsystem.Interactive("sudo", "pacman-key", "--add", tmp.Name()); err != nil {
		output.Warn("keyring: no se pudo importar la clave")
		return
	}
	if err := kitsystem.Interactive("sudo", "pacman-key", "--lsign-key", verify.Fingerprint); err != nil {
		output.Warn("keyring: la clave se importó pero no se firmó localmente")
		return
	}
	output.Ok("keyring: clave importada y firmada")
}

// keyringHasKey consulta sin root: --list-keys lee el keyring público.
func keyringHasKey() bool {
	out, err := kitsystem.RunCombined("pacman-key", "--list-keys", verify.Fingerprint)
	if err != nil {
		return false
	}
	return strings.Contains(out, "pub")
}
