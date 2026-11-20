package ytmusiccli

// Tests de los hallazgos MEDIO y BAJO del CLI (Y14, Y15, Y16 y Y17).
// Cada uno tiene que FALLAR si se revierte su arreglo:
//
//   - Y14: `--limit` no pasa un valor negativo como si fuera un límite.
//   - Y15: el reloj del CLI lleva horas ("1:02:03", no "62:03").
//   - Y16: `seek abc` falla en vez de saltar a 0.
//   - Y17: el comando `play` no tiene la rama muerta del título vacío.

import (
	"os"
	"strings"
	"testing"
)

// ── Y14: el límite ────────────────────────────────────────────────────────

// TestElLimiteNegativoCaeAlDefault fija Y14: `--limit -1` no puede llegar a
// Thumbs y a EnrichDurations con meanings distintos. Con el fix del CLI, un
// valor no positivo cae al default y las dos funciones reciben lo mismo.
func TestElLimiteNegativoCaeAlDefault(t *testing.T) {
	casos := []struct {
		arg      string
		fallback int
	}{
		{"--limit=-1", 24},
		{"--limit=0", 24},
		{"--limit=-99", 18},
		{"--limit=abc", 24},
		{"--limit=7", 7},
	}
	for _, caso := range casos {
		f := parseArgs([]string{caso.arg}, "json")
		got := f.number("limit", caso.fallback)
		if got <= 0 {
			t.Errorf("%s = %d: el límite tiene que ser positivo (Y14)", caso.arg, got)
		}
	}
	if got := parseArgs([]string{"--limit=-1"}, "json").number("limit", 24); got != 24 {
		t.Errorf("--limit=-1 = %d, quería 24 (el default)", got)
	}
	if got := parseArgs([]string{"--limit=5"}, "json").number("limit", 24); got != 5 {
		t.Errorf("--limit=5 = %d, quería 5", got)
	}
}

// ── Y15: el reloj ─────────────────────────────────────────────────────────

// TestElRelojDelCLILlevaHoras fija Y15: 3.723 s se mostraban como "62:03".
// Ahora el reloj del CLI es el del paquete, con horas.
func TestElRelojDelCLILlevaHoras(t *testing.T) {
	casos := map[int]string{
		0:     "0:00",
		59:    "0:59",
		60:    "1:00",
		225:   "3:45",
		3599:  "59:59",
		3600:  "1:00:00",
		3723:  "1:02:03",
		7323:  "2:02:03",
		-1:    "0:00",
		-3600: "0:00",
	}
	for seconds, want := range casos {
		if got := clock(seconds); got != want {
			t.Errorf("clock(%d) = %q, quería %q (Y15)", seconds, got, want)
		}
	}
}

// ── Y16: seek ─────────────────────────────────────────────────────────────

// TestSeekConUnArgumentoQueNoEsNumeroFalla fija Y16: "seek abc" ignoraba el
// error de Sscanf y mandaba "seek 0" a mpv: la pista volvía al principio en
// vez de decir que el argumento no es un número.
func TestSeekConUnArgumentoQueNoEsNumeroFalla(t *testing.T) {
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if code := Run([]string{"seek", "abc"}); code != 1 {
				t.Errorf("código = %d, quería 1", code)
			}
		})
	})
	if !strings.Contains(stderr, "número") {
		t.Errorf("stderr = %q, tenía que decir que el argumento no es un número (Y16)", stderr)
	}
	_ = stdout
}

// TestSeekConUnArgumentoParcialmenteNumeroTambiénFalla: "12abc" se
// aceptaba como 12 sin quejarse. Ahora es un error.
func TestSeekConUnArgumentoParcialmenteNumeroTambiénFalla(t *testing.T) {
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if code := Run([]string{"seek", "12abc"}); code != 1 {
				t.Errorf("código = %d, quería 1", code)
			}
		})
	})
	if !strings.Contains(stderr, "número") {
		t.Errorf("stderr = %q, tenía que decir que el argumento no es un número (Y16)", stderr)
	}
}

// TestSeekSinArgumentoNoFallaPorElNumero: sin argumento el comando sigue
// siendo un error de "no hay player", no un error de parseo.
func TestSeekSinArgumentoNoFallaPorElNumero(t *testing.T) {
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if code := Run([]string{"seek"}); code != 1 {
				t.Errorf("código = %d, quería 1", code)
			}
		})
	})
	if strings.Contains(stderr, "no es un número") {
		t.Errorf("stderr = %q, sin argumento no hay nada que parsear", stderr)
	}
}

// ── Y17: código muerto ───────────────────────────────────────────────────

// TestElComandoPlayNoTieneCodigoMuerto fija Y17: el `if song.Title == "" {
// song.Title = "" }` no hacía nada — la rama entera era un esqueleto de una
// versión anterior. Se borró.
//
// El arreglo no cambia el comportamiento (por eso no hay un test de
// comportamiento que pueda fallar), así que el test mira el código: la rama
// muerta no puede volver.
func TestElComandoPlayNoTieneCodigoMuerto(t *testing.T) {
	raw, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatalf("leyendo root.go: %v", err)
	}
	fuente := string(raw)
	if strings.Contains(fuente, `song.Title = ""`) {
		t.Error("el comando play volvió a tener la rama muerta `song.Title = \"\"` (Y17)")
	}
	if strings.Contains(fuente, `if song.Title == "" {`) {
		t.Error("el comando play volvió a tener la rama `if song.Title == \"\"` (Y17)")
	}
}

// TestElComandoPlayNoRellenaElTituloConElVideoId: el comportamiento que la
// rama muerta NO tenía que cambiar: `play` sin --title deja el título vacío
// (lo resuelve `Play` con yt-dlp), no el videoId.
func TestElComandoPlayNoRellenaElTituloConElVideoId(t *testing.T) {
	f := parseArgs([]string{"play", "Z85lxckrtzg"}, "json")
	if got := f.value("title", ""); got != "" {
		t.Errorf("el título quedó en %q: `play` no rellena el título con el videoId (Y17)", got)
	}
	if got := f.value("video", ""); got != "" {
		t.Errorf("video = %q, quería vacío", got)
	}
}
