package confirm

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// stdinReader se comparte para no perder líneas entre preguntas.
var stdinReader = bufio.NewReader(os.Stdin)

// ttyForTest permite a los tests simular una terminal. En producción es nil.
var ttyForTest func() bool

// SetTTYForTest reemplaza la detección de terminal y devuelve cómo restaurarla.
//
// IsTTY usa un ioctl real (TCGETS): un pipe o /dev/null ya no pasan por
// terminal. Los tests necesitan decir "esto es una terminal" sin un PTY.
func SetTTYForTest(f func() bool) (restore func()) {
	prev := ttyForTest
	ttyForTest = f
	return func() { ttyForTest = prev }
}

func isTTY() bool {
	if ttyForTest != nil {
		return ttyForTest()
	}
	return system.IsTTY()
}

// Confirm pregunta como pacman: [Y/n] o [y/N], en inglés.
//
// Con --yes no pregunta. Sin terminal devuelve false: abortar es más
// seguro que actualizar a ciegas.
func Confirm(prompt string, yes bool, def bool) bool {
	if yes {
		return true
	}
	if !isTTY() {
		return false
	}

	reader := stdinReader
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			// EOF sin respuesta: nadie contesto. Abortar es mas
			// seguro que asumir el default, aunque sea "si".
			return false
		}
		if answer, ok := parseAnswer(line, def); ok {
			return answer
		}
		fmt.Println("Please answer 'y' or 'n'.")
	}
}

// parseAnswer interpreta la respuesta. Vacío usa el default del prompt,
// igual que pacman con [Y/n] o [y/N].
func parseAnswer(s string, def bool) (answer, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return def, true
	case "y", "yes":
		return true, true
	case "n", "no":
		return false, true
	case "c", "cancel", "cancelar":
		return false, true
	}
	return false, false
}

// AskInt pide un número; vacío o inválido usa el default.
func AskInt(prompt string, def int) int {
	if !isTTY() {
		return def
	}
	fmt.Print(prompt)
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	switch strings.ToLower(line) {
	case "c", "cancel", "cancelar":
		return -1
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 {
		return def
	}
	return n
}
