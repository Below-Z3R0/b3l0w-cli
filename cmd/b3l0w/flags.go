package main

import "strings"

// flags es el parseo de los argumentos de un comando.
//
// Los flags con valor se escriben de dos formas y las dos tienen que
// funcionar: `--flag=valor` y `--flag valor`. Es lo natural y que una
// falle es una trampa.
//
// Un flag con valor se reconoce por el prefijo (con o sin "=") o por
// el nombre exacto. El resto de los flags son booleanos: aparecen
// solos y no consumen el argumento siguiente.
type flags struct {
	// valores es el valor de cada flag con valor, por nombre sin "=".
	valores map[string]string
	// presentes son los flags booleanos que aparecieron.
	presentes map[string]bool
	// libres son los argumentos que no son flags.
	libres []string
	// desconocidos son los flags que no se reconocieron.
	desconocidos []string
}

// banderaConValor son los flags que aceptan un valor.
var banderaConValor = map[string]bool{
	"--manifest":     true,
	"--state-dir":    true,
	"--variant":      true,
	"--service":      true,
	"--tail":         true,
	"--src":          true,
	"--registry":     true,
	"--plugins-dir":  true,
	"--profile":      true,
	"--file":         true,
	"--dotfiles":     true,
	"--source":       true,
	"--asset":        true,
	"--api":          true,
	"--plugin":       true,
	"--keep":         true,
	"--check":        true,
	"--channel":      true,
	"--registry-url": true,
}

// parsearFlags separa los argumentos en flags y posicionales.
//
// Se recorre todo el slice una vez: los flags con valor en forma
// `--flag valor` consumen el siguiente argumento, los demas no.
// Un flag desconocido se anota para que el comando pueda avisar, en
// vez de ignorarlo en silencio.
func parsearFlags(args []string) flags {
	f := flags{
		valores:   map[string]string{},
		presentes: map[string]bool{},
		libres:    []string{},
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			f.libres = append(f.libres, arg)
			continue
		}

		nombre, valor, conValor := strings.Cut(arg, "=")
		if banderaConValor[nombre] {
			if !conValor {
				// La forma `--flag valor`: el valor es el siguiente.
				if i+1 < len(args) {
					i++
					valor = args[i]
					conValor = true
				}
			}
			if conValor {
				f.valores[nombre] = valor
				continue
			}
			// El flag esta solo y no tiene valor: se ignora.
			f.presentes[nombre] = true
			continue
		}

		f.presentes[nombre] = true
		if !conValor {
			f.desconocidos = append(f.desconocidos, arg)
		}
	}
	return f
}

// valor devuelve el valor de un flag con valor.
func (f flags) valor(nombre string) string {
	return f.valores[nombre]
}

// valorO devuelve el valor de un flag o un default.
func (f flags) valorO(nombre, defecto string) string {
	if v, ok := f.valores[nombre]; ok {
		return v
	}
	return defecto
}

// presente dice si un flag booleano apareció.
func (f flags) presente(nombre string) bool {
	return f.presentes[nombre]
}

// libre devuelve el primer argumento que no es flag.
func (f flags) libre() string {
	if len(f.libres) == 0 {
		return ""
	}
	return f.libres[0]
}
