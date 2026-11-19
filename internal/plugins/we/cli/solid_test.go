package wecli

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHexColorAceptaLasFormasDocumentadas(t *testing.T) {
	casos := map[string]struct {
		r, g, b uint8
	}{
		"#1e1e2e": {0x1e, 0x1e, 0x2e},
		"1e1e2e":  {0x1e, 0x1e, 0x2e},
		"#f0a":    {0xff, 0x00, 0xaa},
		"f0a":     {0xff, 0x00, 0xaa},
		"#FFFFFF": {0xff, 0xff, 0xff},
	}
	for entrada, want := range casos {
		rgba, err := parseHexColor(entrada)
		if err != nil {
			t.Errorf("parseHexColor(%q) falló: %v", entrada, err)
			continue
		}
		if rgba.R != want.r || rgba.G != want.g || rgba.B != want.b {
			t.Errorf("parseHexColor(%q) = %+v, quería %v", entrada, rgba, want)
		}
		if rgba.A != 255 {
			t.Errorf("parseHexColor(%q) alpha = %d, quería 255 (opaco)", entrada, rgba.A)
		}
	}
}

func TestParseHexColorRechazaLoInvalido(t *testing.T) {
	for _, entrada := range []string{"", "#", "#12", "#12345", "#gggggg", "#1234567"} {
		if _, err := parseHexColor(entrada); err == nil {
			t.Errorf("parseHexColor(%q) debería fallar", entrada)
		}
	}
}

func TestSolidGeneraElPNGDelColor(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "fondo.png")

	codigo, salida, errSalida := correr(t, "solid", "--color", "#1e1e2e", "--out", out, "--size", "8x4")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, out) {
		t.Errorf("stdout = %q, quería la ruta del PNG", salida)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("no escribió el PNG: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("el PNG no se pudo decodificar: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 4 {
		t.Errorf("tamaño = %v, quería 8x4", b)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 0x1e || g>>8 != 0x1e || b>>8 != 0x2e {
		t.Errorf("color = %d,%d,%d, quería 30,30,46", r>>8, g>>8, b>>8)
	}
	if a>>8 != 255 {
		t.Errorf("alpha = %d, quería 255 (opaco)", a>>8)
	}
}

func TestSolidCreaElDirectorioDeSalida(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sub", "fondo.png")
	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", out, "--size", "2x2")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("no escribió el PNG: %v", err)
	}
}

func TestSolidSinColorNiOutFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "solid")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--color") || !strings.Contains(errSalida, "--out") {
		t.Errorf("stderr = %q, quería que pida los dos flags", errSalida)
	}
}

func TestSolidColorInvalidoFalla(t *testing.T) {
	dir := t.TempDir()
	codigo, _, errSalida := correr(t, "solid", "--color", "no-es-color", "--out", filepath.Join(dir, "f.png"))
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "color inválido") {
		t.Errorf("stderr = %q, quería que diga que el color es inválido", errSalida)
	}
}

func TestSolidSizeInvalidoFalla(t *testing.T) {
	dir := t.TempDir()
	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", filepath.Join(dir, "f.png"), "--size", "grande")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--size") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestSolidSizeDemasiadoGrandeFalla(t *testing.T) {
	// RGBA son 4 bytes por píxel: 100000x100000 son 40 GB y el kernel mata
	// el proceso. El comando tiene que cortar antes.
	dir := t.TempDir()
	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", filepath.Join(dir, "f.png"), "--size", "100000x100000")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "demasiado grande") {
		t.Errorf("stderr = %q, quería que diga que es demasiado grande", errSalida)
	}
}

func TestSolidOpcionDesconocidaFalla(t *testing.T) {
	// La ruta de salida es temporal: si el flag desconocido se ignorara, el
	// comando escribiría el PNG y la carpeta del repo se llenaría de basura.
	out := filepath.Join(t.TempDir(), "x.png")
	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", out, "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestSolidAceptaElValorConIgual(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "fondo.png")
	codigo, _, errSalida := correr(t, "solid", "--color=#1e1e2e", "--out="+out, "--size=4x4")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("el PNG no se pudo decodificar: %v", err)
	}
	if r, _, _, _ := img.At(0, 0).RGBA(); r>>8 != 0x1e {
		t.Errorf("color = %d, quería 30", r>>8)
	}
}
