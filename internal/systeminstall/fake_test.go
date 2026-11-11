package systeminstall

import (
	"errors"
	"strings"
)

// fakeRunner es un Runner de mentira: registra lo que se le pide y
// responde con lo que configuró cada test. Nunca toca el sistema.
type fakeRunner struct {
	// calls son los comandos interactivos, en orden.
	calls []string
	// queries son las consultas (Run), en orden.
	queries []string
	// lookups son los CommandPath pedidos, en orden.
	lookups []string

	// outputs es la respuesta de Run por comando ("pacman -Qq").
	outputs map[string]string
	// fail hace fallar un comando, por clave.
	fail map[string]error
	// failPrefix hace fallar un comando que empiece con el prefijo.
	failPrefix []string
	// paths responde CommandPath.
	paths map[string]string
}

func newFake() *fakeRunner {
	return &fakeRunner{
		outputs: map[string]string{},
		fail:    map[string]error{},
		paths:   map[string]string{},
	}
}

// cmdKey arma la clave estable de un comando: "nombre arg1 arg2".
func cmdKey(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := cmdKey(name, args...)
	f.queries = append(f.queries, key)
	if err := f.failure(key); err != nil {
		return "", err
	}
	return f.outputs[key], nil
}

func (f *fakeRunner) Interactive(name string, args ...string) error {
	key := cmdKey(name, args...)
	f.calls = append(f.calls, key)
	return f.failure(key)
}

func (f *fakeRunner) CommandPath(name string) string {
	f.lookups = append(f.lookups, name)
	return f.paths[name]
}

// failure devuelve el error configurado para un comando, por clave
// exacta o por prefijo.
func (f *fakeRunner) failure(key string) error {
	if err := f.fail[key]; err != nil {
		return err
	}
	for _, prefix := range f.failPrefix {
		if strings.HasPrefix(key, prefix) {
			return errors.New("falló: " + key)
		}
	}
	return nil
}

// contains dice si un comando quedó registrado.
func (f *fakeRunner) contains(cmd string) bool {
	for _, c := range f.calls {
		if c == cmd {
			return true
		}
	}
	return false
}
