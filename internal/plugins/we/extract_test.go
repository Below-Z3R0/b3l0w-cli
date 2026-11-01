package we

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// payloadFalso no es un mp4 válido a propósito: si Extract intentara llamar a
// ffmpeg con esto, el test fallaría. Que pase prueba que tomó el camino de copia.
func payloadFalso() []byte {
	b := make([]byte, 64*1024)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func escribirFuente(t *testing.T, dir, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	// os.WriteFile respeta el umask; forzamos el modo para el assert.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractCopiaExactaSinFfmpeg(t *testing.T) {
	dir := t.TempDir()
	data := payloadFalso()
	src := escribirFuente(t, dir, "Campfire PC.mp4", data, 0o640)
	outDir := filepath.Join(dir, "out")

	it := Item{ID: "3582809467", Title: "Campfire PC", Type: "video", File: src}
	res, err := Extract(context.Background(), it, ExtractOptions{OutDir: outDir, Format: "mp4", CRF: 0})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Method != MethodCopy {
		t.Errorf("Method = %q, quería %q", res.Method, MethodCopy)
	}
	if res.Skipped {
		t.Error("no debería estar saltado en la primera pasada")
	}
	want := filepath.Join(outDir, "3582809467-campfire-pc.mp4")
	if res.Path != want {
		t.Errorf("Path = %q, quería %q", res.Path, want)
	}

	got, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("leer salida: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("la copia no es idéntica: %d vs %d bytes", len(got), len(data))
	}
	if res.Bytes != int64(len(data)) {
		t.Errorf("Bytes = %d, quería %d", res.Bytes, len(data))
	}
	st, err := os.Stat(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o640 {
		t.Errorf("permisos = %v, quería 0640 (los del origen)", perm)
	}
}

func TestExtractSkipSiElDestinoExiste(t *testing.T) {
	dir := t.TempDir()
	src := escribirFuente(t, dir, "rain.mp4", payloadFalso(), 0o644)
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	previo := []byte("contenido viejo que no se toca")
	dest := filepath.Join(outDir, "7-rain.mp4")
	if err := os.WriteFile(dest, previo, 0o644); err != nil {
		t.Fatal(err)
	}

	it := Item{ID: "7", Title: "rain", Type: "video", File: src}
	res, err := Extract(context.Background(), it, ExtractOptions{OutDir: outDir, Format: "mp4", CRF: 0})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !res.Skipped {
		t.Error("debería devolver Skipped sin Overwrite")
	}
	if res.Bytes != int64(len(previo)) {
		t.Errorf("Bytes = %d, quería el tamaño del existente %d", res.Bytes, len(previo))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, previo) {
		t.Error("pisó el destino aunque no se pidió Overwrite")
	}

	// Con Overwrite sí reemplaza y toma el camino de copia.
	res, err = Extract(context.Background(), it, ExtractOptions{OutDir: outDir, Format: "mp4", CRF: 0, Overwrite: true})
	if err != nil {
		t.Fatalf("Extract con Overwrite: %v", err)
	}
	if res.Skipped || res.Method != MethodCopy {
		t.Errorf("con Overwrite: %+v", res)
	}
	got, err = os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payloadFalso()) {
		t.Error("no reemplazó el contenido")
	}
}

func TestExtractRechazaTipoNoVideo(t *testing.T) {
	dir := t.TempDir()
	src := escribirFuente(t, dir, "escena.pkg", []byte("x"), 0o644)
	for _, typ := range []string{"scene", "web", "unknown"} {
		it := Item{ID: "9", Title: "Escena", Type: typ, File: src}
		_, err := Extract(context.Background(), it, ExtractOptions{OutDir: filepath.Join(dir, "out"), Format: "mp4"})
		if err == nil {
			t.Fatalf("tipo %q debería fallar", typ)
		}
		if !strings.Contains(err.Error(), "record") {
			t.Errorf("el error de %q debería sugerir el método record: %v", typ, err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "out")); statErr == nil {
			t.Errorf("tipo %q: no debería haber creado el directorio de salida", typ)
		}
	}
}

func TestExtractValidaciones(t *testing.T) {
	dir := t.TempDir()
	src := escribirFuente(t, dir, "a.mp4", payloadFalso(), 0o644)
	it := Item{ID: "1", Title: "a", Type: "video", File: src}

	cases := []struct {
		name  string
		item  Item
		opt   ExtractOptions
		chunk string
	}{
		{"sin archivo", Item{ID: "1", Type: "video"}, ExtractOptions{OutDir: dir}, "project.json"},
		{"sin out dir", it, ExtractOptions{}, "directorio de salida"},
		{"formato inválido", it, ExtractOptions{OutDir: dir, Format: "avi"}, "mp4 o webm"},
		{"crf negativo", it, ExtractOptions{OutDir: dir, CRF: -1}, "CRF"},
		{"fuente inexistente", Item{ID: "1", Type: "video", File: filepath.Join(dir, "nope.mp4")}, ExtractOptions{OutDir: dir}, "stat"},
		{"fuente es directorio", Item{ID: "1", Type: "video", File: dir}, ExtractOptions{OutDir: dir}, "directorio"},
	}
	for _, c := range cases {
		if _, err := Extract(context.Background(), c.item, c.opt); err == nil || !strings.Contains(err.Error(), c.chunk) {
			t.Errorf("%s: error = %v, quería que mencione %q", c.name, err, c.chunk)
		}
	}
}

func TestExtractCopyable(t *testing.T) {
	cases := []struct {
		src, format string
		crf         int
		want        bool
	}{
		{"/x/a.mp4", "mp4", 0, true},
		{"/x/a.MP4", "mp4", 0, true},
		{"/x/a.mp4", "mp4", 18, false},
		{"/x/a.mp4", "webm", 0, false},
		{"/x/a.webm", "mp4", 0, false},
		{"/x/a.mkv", "mp4", 0, false},
	}
	for _, c := range cases {
		if got := copyable(c.src, c.format, c.crf); got != c.want {
			t.Errorf("copyable(%q, %q, %d) = %v, quería %v", c.src, c.format, c.crf, got, c.want)
		}
	}
}

func TestExtractNormalizaFormato(t *testing.T) {
	cases := map[string]string{"": "mp4", "MP4": "mp4", ".webm": "webm", " WebM ": "webm"}
	for in, want := range cases {
		got, err := normalizeFormat(in)
		if err != nil || got != want {
			t.Errorf("normalizeFormat(%q) = %q, %v; quería %q", in, got, err, want)
		}
	}
	if _, err := normalizeFormat("gif"); err == nil {
		t.Error("gif debería ser rechazado")
	}
}

// TestExtractTranscodificaReal ejercita el camino ffmpeg de punta a punta. Se
// salta si no hay ffmpeg: los tests de arriba cubren la lógica sin depender del
// binario.
func TestExtractTranscodificaReal(t *testing.T) {
	ff, err := Locate()
	if err != nil {
		t.Skipf("ffmpeg no disponible: %v", err)
	}
	if _, err := LocateProbe(); err != nil {
		t.Skipf("ffprobe no disponible: %v", err)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "fuente.mkv")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=96x64:d=1:r=10",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-an", src}
	if err := Run(context.Background(), ff.Path, args...); err != nil {
		t.Fatalf("generar fuente: %v", err)
	}

	cases := []struct{ format, wantCodec string }{
		{"mp4", "h264"},
		{"webm", "vp9"},
	}
	for _, c := range cases {
		outDir := filepath.Join(dir, "out-"+c.format)
		it := Item{ID: "42", Title: "Test Clip", Type: "video", File: src}
		res, err := Extract(context.Background(), it, ExtractOptions{OutDir: outDir, Format: c.format, CRF: 40})
		if err != nil {
			t.Fatalf("%s: Extract: %v", c.format, err)
		}
		if res.Method != MethodTranscode {
			t.Errorf("%s: Method = %q, quería %q", c.format, res.Method, MethodTranscode)
		}
		if res.Bytes <= 0 {
			t.Errorf("%s: Bytes = %d", c.format, res.Bytes)
		}
		if filepath.Ext(res.Path) != "."+c.format {
			t.Errorf("%s: Path = %q", c.format, res.Path)
		}
		m, err := Probe(context.Background(), res.Path)
		if err != nil {
			t.Fatalf("%s: Probe: %v", c.format, err)
		}
		if m.Codec != c.wantCodec {
			t.Errorf("%s: Codec = %q, quería %q", c.format, m.Codec, c.wantCodec)
		}
		if m.Width != 96 || m.Height != 64 {
			t.Errorf("%s: dimensiones = %dx%d, quería 96x64", c.format, m.Width, m.Height)
		}

		// No deben quedar temporales .b3l0w-* en el directorio de salida.
		entries, err := os.ReadDir(outDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".b3l0w-") {
				t.Errorf("%s: quedó el temporal %s", c.format, e.Name())
			}
		}
	}
}
