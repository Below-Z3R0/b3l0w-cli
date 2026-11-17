package ytmusiccli

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestParseArgsAceptaBanderasEnCualquierPosicion: el paquete `flag` de Go
// corta en el primer posicional y `search "daft punk" --json` perdía el --json
// en silencio.
func TestParseArgsAceptaBanderasEnCualquierPosicion(t *testing.T) {
	f := parseArgs([]string{"search", "daft punk", "--json"}, "json")
	if !f.bools["json"] {
		t.Error("perdió el --json que venía después del posicional")
	}
	if len(f.pos) != 2 || f.pos[0] != "search" || f.pos[1] != "daft punk" {
		t.Errorf("posicionales = %v, quería [search daft punk]", f.pos)
	}
}

// TestParseArgsConIgual: "--limit=10" es una forma válida.
func TestParseArgsConIgual(t *testing.T) {
	f := parseArgs([]string{"--limit=10", "--country=PA"}, "json")
	if f.value("limit", "") != "10" {
		t.Errorf("limit = %q, quería 10", f.value("limit", ""))
	}
	if f.value("country", "") != "PA" {
		t.Errorf("country = %q, quería PA", f.value("country", ""))
	}
}

// TestParseArgsTomaElValorSiguiente: "--limit 10" también vale.
func TestParseArgsTomaElValorSiguiente(t *testing.T) {
	f := parseArgs([]string{"--limit", "10"}, "json")
	if f.value("limit", "") != "10" {
		t.Errorf("limit = %q, quería 10", f.value("limit", ""))
	}
}

// TestParseArgsNoSeComeElSiguientePosicional: "--session --json" deja --json
// como bandera, no como valor.
func TestParseArgsNoSeComeElSiguientePosicional(t *testing.T) {
	f := parseArgs([]string{"--session", "--json"}, "json")
	if f.value("session", "") != "" {
		t.Errorf("session = %q, quería vacío", f.value("session", ""))
	}
	if !f.bools["json"] {
		t.Error("no registró --json como bandera")
	}
}

// TestFlagsValueUsaElFallback: un valor vacío cae al default.
func TestFlagsValueUsaElFallback(t *testing.T) {
	f := parseArgs([]string{"--country="}, "json")
	if got := f.value("country", "US"); got != "US" {
		t.Errorf("value = %q, quería US", got)
	}
	if got := f.value("inexistente", "defecto"); got != "defecto" {
		t.Errorf("value = %q, quería defecto", got)
	}
}

// TestFlagsNumberUsaElFallback: un número no parseable o cero cae al default.
func TestFlagsNumberUsaElFallback(t *testing.T) {
	f := parseArgs([]string{"--limit=abc"}, "json")
	if got := f.number("limit", 24); got != 24 {
		t.Errorf("number = %d, quería 24", got)
	}
	f = parseArgs([]string{"--limit=7"}, "json")
	if got := f.number("limit", 24); got != 7 {
		t.Errorf("number = %d, quería 7", got)
	}
	if got := f.number("inexistente", 18); got != 18 {
		t.Errorf("number = %d, quería 18", got)
	}
}

// TestClockFormateaLaPosición: el panel muestra "1:01".
func TestClockFormateaLaPosición(t *testing.T) {
	cases := map[int]string{
		0:    "0:00",
		5:    "0:05",
		61:   "1:01",
		3599: "59:59",
		3600: "60:00",
		-1:   "0:00",
	}
	for seconds, want := range cases {
		if got := clock(seconds); got != want {
			t.Errorf("clock(%d) = %q, quería %q", seconds, got, want)
		}
	}
}

// TestRunSinArgsMuestraElUso: un comando vacío imprime la ayuda y sale con 2.
func TestRunSinArgsMuestraElUso(t *testing.T) {
	stdout := captureStdout(t, func() {
		if code := Run(nil); code != 2 {
			t.Errorf("código = %d, quería 2", code)
		}
	})
	if !strings.Contains(stdout, "uso:") {
		t.Errorf("stdout = %q, quería el texto de uso", stdout)
	}
}

// TestRunComandoDesconocidoSaleConDos: un typo no se confunde con un comando
// válido.
func TestRunComandoDesconocidoSaleConDos(t *testing.T) {
	captureStdout(t, func() {
		if code := Run([]string{"inventado"}); code != 2 {
			t.Errorf("código = %d, quería 2", code)
		}
	})
}

// TestRunHelpSaleConCero.
func TestRunHelpSaleConCero(t *testing.T) {
	captureStdout(t, func() {
		if code := Run([]string{"help"}); code != 0 {
			t.Errorf("código = %d, quería 0", code)
		}
	})
}

// TestRunSearchSinConsultaFalla: el error sale por stderr y con código 1.
func TestRunSearchSinConsultaFalla(t *testing.T) {
	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			if code := Run([]string{"search"}); code != 1 {
				t.Errorf("código = %d, quería 1", code)
			}
		})
	})
	if !strings.Contains(stderr, "consulta") {
		t.Errorf("stderr = %q, quería que mencione la consulta", stderr)
	}
}

// captureStderr corre la función y devuelve lo que escribió en stderr.
func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = original }()
	run()
	writer.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("leyendo stderr: %v", err)
	}
	return string(raw)
}

// captureStdout corre la función y devuelve lo que escribió en stdout.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	run()
	writer.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("leyendo stdout: %v", err)
	}
	return string(raw)
}
