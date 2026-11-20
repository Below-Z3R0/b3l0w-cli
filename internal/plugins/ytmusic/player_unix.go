//go:build unix

package ytmusic

import (
	"os/exec"
	"syscall"
)

// El reproductor es un proceso POSIX: `setsid`, grupo de procesos y señales
// de Unix. Estas piezas no existen fuera de Unix, así que viven acá y no en
// player.go (Y20): el resto del paquete compila en cualquier plataforma y lo
// que no funciona lo dice, en vez de reventar con "undefined: syscall.Kill".

const (
	signalTerm = syscall.SIGTERM
	signalKill = syscall.SIGKILL
)

// setSessionAttr deja al reproductor en su propia sesión: si el service se
// recarga, la música no se corta.
func setSessionAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// alive dice si el proceso existe.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// killGroup manda una señal al grupo de procesos del reproductor (el `sh` del
// pipeline y sus hijos).
func killGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

// killProcess manda una señal a un proceso puntual.
func killProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
