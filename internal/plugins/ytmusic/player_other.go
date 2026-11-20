//go:build !unix

package ytmusic

import (
	"fmt"
	"os/exec"
	"syscall"
)

// Fuera de Unix no hay `setsid`, ni grupos de procesos, ni señales POSIX. El
// reproductor no puede funcionar, pero el paquete sí compila y lo dice
// (Y20): antes el error de build era "undefined: syscall.Kill", sin
// explicación.

const (
	signalTerm = syscall.Signal(0xf)
	signalKill = syscall.Signal(0x9)
)

// setSessionAttr no hace nada: no hay sesiones POSIX.
func setSessionAttr(cmd *exec.Cmd) {}

// alive no puede preguntarle al kernel: se informa que no vive.
func alive(pid int) bool { return false }

// killGroup no puede señalar al grupo.
func killGroup(pid int, sig syscall.Signal) error {
	return fmt.Errorf("el reproductor sólo funciona en Unix")
}

// killProcess no puede señalar al proceso.
func killProcess(pid int, sig syscall.Signal) error {
	return fmt.Errorf("el reproductor sólo funciona en Unix")
}
