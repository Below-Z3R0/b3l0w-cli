package ytmusic

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSecondsConvierteLaSalidaDeYTDLP: "358.07" son 358 segundos.
func TestParseSecondsConvierteLaSalidaDeYTDLP(t *testing.T) {
	cases := map[string]int{
		"358.07": 358,
		"358":    358,
		"0":      0,
		" 12.5 ": 12,
		"":       0,
		"abc":    0,
		"12,5":   0,
		"3:45":   0,
	}
	for input, want := range cases {
		if got := parseSeconds(input); got != want {
			t.Errorf("parseSeconds(%q) = %d, quería %d", input, got, want)
		}
	}
}

// TestShellQuoteEscapaLasComillasSimples: el comando va a /bin/sh, así que
// una comilla simple tiene que cerrarse, escaparse y reabrirse.
func TestShellQuoteEscapaLasComillasSimples(t *testing.T) {
	if got := shellQuote("simple"); got != "'simple'" {
		t.Errorf("shellQuote = %q", got)
	}
	if got := shellQuote("con espacio"); got != "'con espacio'" {
		t.Errorf("shellQuote = %q", got)
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("shellQuote = %q, quería 'it'\\''s'", got)
	}
}

// TestShellQuoteNoRompeUnVideoID: los ids de YouTube son alfanuméricos, pero
// el id tiene que quedar entrecomillado igual.
func TestShellQuoteNoRompeUnVideoID(t *testing.T) {
	got := shellQuote("dQw4w9WgXcQ")
	if got != "'dQw4w9WgXcQ'" {
		t.Errorf("shellQuote = %q", got)
	}
}

// TestStatusSinPlayerDevuelveParado: el panel tiene que poder preguntar
// "¿qué suena?" sin que haya nada sonando.
func TestStatusSinPlayerDevuelveParado(t *testing.T) {
	state, err := Status(t.TempDir())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if state.Playing {
		t.Error("no hay player y dice que está reproduciendo")
	}
	if state.PID != 0 {
		t.Errorf("pid = %d, quería 0", state.PID)
	}
}

// TestStopSinPlayerNoFalla: parar cuando no suena nada no es un error.
func TestStopSinPlayerNoFalla(t *testing.T) {
	if err := Stop(t.TempDir()); err != nil {
		t.Errorf("Stop sin player = %v, quería nil", err)
	}
}

// TestStatusConPlayerMuertoLimpiaElEstado: si el proceso de mpv ya no
// está, el estado no puede quedar diciendo "reproduciendo".
func TestStatusConPlayerMuertoLimpiaElEstado(t *testing.T) {
	dir := t.TempDir()
	// PID 0 no puede estar vivo: alive(0) es false.
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     0,
		VideoID: "abc",
		Title:   "Canción",
		Socket:  filepath.Join(dir, "mpv.sock"),
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if state.Playing {
		t.Error("un PID muerto no puede estar reproduciendo")
	}
	if state.Title != "Canción" {
		t.Errorf("title = %q, el título se conserva aunque esté parado", state.Title)
	}
	if _, err := os.Stat(playerPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no limpió el estado: %v", err)
	}
}

// TestAliveConPIDInvalido: sin PID no hay proceso vivo.
func TestAliveConPIDInvalido(t *testing.T) {
	if alive(0) {
		t.Error("alive(0) = true")
	}
	if alive(-1) {
		t.Error("alive(-1) = true")
	}
}
