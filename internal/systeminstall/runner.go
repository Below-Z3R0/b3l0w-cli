package systeminstall

import (
	kitsystem "github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Runner es la costura entre el módulo y la máquina.
//
// Todo lo que el módulo le pide al sistema pasa por acá: consultar
// paquetes, instalar, habilitar servicios. Se inyecta para que los
// tests puedan correr sin pacman, flatpak ni systemctl de verdad: este
// es el módulo que más riesgo tiene del CLI (muta el sistema con root)
// y sin esta costura no se puede probar sin root.
type Runner interface {
	// Run ejecuta un comando y devuelve su stdout.
	Run(name string, args ...string) (string, error)
	// Interactive ejecuta un comando heredando la terminal: sudo
	// necesita preguntar la contraseña ahí mismo.
	Interactive(name string, args ...string) error
	// CommandPath devuelve la ruta de un binario o "" si no está.
	CommandPath(name string) string
}

// systemRunner es el Runner real: delega en kit/system.
type systemRunner struct{}

func (systemRunner) Run(name string, args ...string) (string, error) {
	return kitsystem.Run(name, args...)
}

func (systemRunner) Interactive(name string, args ...string) error {
	return kitsystem.Interactive(name, args...)
}

func (systemRunner) CommandPath(name string) string {
	return kitsystem.CommandPath(name)
}

// SystemRunner devuelve el runner que habla con la máquina real.
func SystemRunner() Runner { return systemRunner{} }

// orReal evita el panic de un Runner nil: un runner faltante se
// reemplaza por el real, no por un crash.
func orReal(r Runner) Runner {
	if r == nil {
		return systemRunner{}
	}
	return r
}
