package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// fakeRunner es un Runner de mentira: devuelve la salida preparada para cada
// comando y anota los que le pidieron.
//
// Se pasa como f.run (el valor del método), porque un Runner es un tipo
// func: una struct no se convierte sola.
//
// Es seguro para uso concurrente: Collect lanza tres goroutines (imágenes,
// volúmenes y redes) contra el mismo runner, y un mapa sin candado explotaría
// en `go test -race`.
type fakeRunner struct {
	mu       sync.Mutex
	salidas  map[string]string
	errores  map[string]error
	llamadas [][]string
}

// nuevoFake devuelve un runner sin respuestas: cualquier comando no
// registrado es un error, así el test falla si el código llama algo que no
// esperaba.
func nuevoFake() *fakeRunner {
	return &fakeRunner{
		salidas: map[string]string{},
		errores: map[string]error{},
	}
}

// con define la salida de un comando. La clave son los argumentos unidos por
// espacio, en el orden en que el código los pasa.
func (f *fakeRunner) con(clave string, salida string) *fakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.salidas[clave] = salida
	return f
}

// falla hace que un comando devuelva error (con esa salida a medias).
func (f *fakeRunner) falla(clave string, salida string, err error) *fakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errores[clave] = err
	f.salidas[clave] = salida
	return f
}

// llamado dice si se pidió ese comando exacto.
func (f *fakeRunner) llamado(clave string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, args := range f.llamadas {
		if strings.Join(args, " ") == clave {
			return true
		}
	}
	return false
}

// pedidos devuelve los comandos ejecutados, en orden.
func (f *fakeRunner) pedidos() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.llamadas))
	for _, args := range f.llamadas {
		out = append(out, strings.Join(args, " "))
	}
	return out
}

// run es el Runner de mentira.
func (f *fakeRunner) run(ctx context.Context, args ...string) (string, error) {
	clave := strings.Join(args, " ")
	f.mu.Lock()
	f.llamadas = append(f.llamadas, append([]string(nil), args...))
	salida, ok := f.salidas[clave]
	err := f.errores[clave]
	f.mu.Unlock()
	if err != nil {
		return salida, err
	}
	if !ok {
		return "", fmt.Errorf("el runner falso no tiene respuesta para %q", clave)
	}
	return salida, nil
}
