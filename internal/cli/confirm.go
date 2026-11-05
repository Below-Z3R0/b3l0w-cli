package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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

	reader := bufio.NewReader(os.Stdin)
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
	}
	return false, false
}
