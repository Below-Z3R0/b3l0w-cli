package doctor

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ansiRegex saca las secuencias de color: el paquete output las agrega
// si stdout parece una terminal, y eso no debe depender de cómo se
// corran los tests.
var ansiRegex = regexp.MustCompile("\x1b\\[[0-9;]*m")

// capturarSalida corre fn con stdout y stderr atrapados y devuelve lo
// que se imprimió, sin los códigos de color.
func capturarSalida(t *testing.T, fn func()) (string, string) {
	t.Helper()

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	outDone := make(chan struct{})
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&outBuf, rOut)
		close(outDone)
	}()
	go func() {
		_, _ = io.Copy(&errBuf, rErr)
		close(errDone)
	}()

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	fn()
	os.Stdout, os.Stderr = origOut, origErr

	wOut.Close()
	wErr.Close()
	<-outDone
	<-errDone

	return ansiRegex.ReplaceAllString(outBuf.String(), ""),
		ansiRegex.ReplaceAllString(errBuf.String(), "")
}

// sinAcentos quita los acentos para comparar mensajes sin que el
// charset del terminal importa.
func contiene(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
