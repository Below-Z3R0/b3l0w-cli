// Package cli tiene la salida y los helpers que comparten los comandos.
package output

import (
	"fmt"
	"os"
)

// color dice si se pueden emitir secuencias ANSI: solo en una terminal,
// y nunca si NO_COLOR está definido.
var color = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func paint(code, text string) string {
	if !color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Bold marca el texto si la salida es una terminal.
func Bold(text string) string { return paint("1", text) }

// Dim marca el texto como secundario.
func Dim(text string) string { return paint("2", text) }

// Step imprime el encabezado de un paso.
func Step(text string) { fmt.Printf("%s %s\n", Bold("▸"), text) }

// Ok imprime un chequeo correcto.
func Ok(text string) { fmt.Printf("  %s %s\n", paint("32", "✓"), text) }

// Warn imprime un aviso: no impide seguir.
func Warn(text string) { fmt.Printf("  %s %s\n", paint("33", "!"), text) }

// Fail imprime un error por stderr.
func Fail(text string) { fmt.Fprintf(os.Stderr, "  %s %s\n", paint("31", "✗"), text) }
