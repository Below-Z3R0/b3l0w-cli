package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// stdinReader se comparte para no perder líneas entre preguntas.
var stdinReader = bufio.NewReader(os.Stdin)

// Confirm pregunta como pacman: [Y/n] o [y/N], en inglés.
//
// Con --yes no pregunta. Sin terminal devuelve false: abortar es más
// seguro que actualizar a ciegas.
func Confirm(prompt string, yes bool, def bool) bool {
	if yes {
		return true
	}
	if !IsTTY() {
		return false
	}

	reader := stdinReader
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return def
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
	if !IsTTY() {
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
