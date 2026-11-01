package we

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Salida real de ffprobe para un mp4 con un stream de video y otro de audio.
const probeFixture = `{
  "streams": [
    {
      "index": 0,
      "codec_name": "h264",
      "codec_type": "video",
      "width": 3840,
      "height": 2160,
      "duration": "35.200000",
      "r_frame_rate": "60/1"
    },
    {
      "index": 1,
      "codec_name": "aac",
      "codec_type": "audio",
      "duration": "35.180000"
    }
  ],
  "format": {
    "duration": "35.200000",
    "size": "12345678"
  }
}`

func TestParseProbeFixture(t *testing.T) {
	m, err := parseProbe([]byte(probeFixture))
	if err != nil {
		t.Fatalf("parseProbe: %v", err)
	}
	if m.Width != 3840 || m.Height != 2160 {
		t.Errorf("dimensiones = %dx%d, quería 3840x2160", m.Width, m.Height)
	}
	if m.Codec != "h264" {
		t.Errorf("Codec = %q, quería h264", m.Codec)
	}
	if m.Duration != 35.2 {
		t.Errorf("Duration = %v, quería 35.2", m.Duration)
	}
}

func TestParseProbeDuracionDesdeElStream(t *testing.T) {
	// Sin duration en format: cae al del stream de video.
	const sinFormatDur = `{"streams":[{"codec_name":"vp9","codec_type":"video","width":1920,"height":1080,"duration":12.5}],"format":{}}`
	m, err := parseProbe([]byte(sinFormatDur))
	if err != nil {
		t.Fatalf("parseProbe: %v", err)
	}
	if m.Duration != 12.5 || m.Codec != "vp9" || m.Width != 1920 {
		t.Errorf("probe inesperado: %+v", m)
	}
}

func TestParseProbeIgnoraStreamsQueNoSonVideo(t *testing.T) {
	const soloAudio = `{"streams":[{"codec_name":"aac","codec_type":"audio","duration":"3.0"}],"format":{"duration":"3.0"}}`
	if _, err := parseProbe([]byte(soloAudio)); err == nil {
		t.Fatal("un archivo sin stream de video debería fallar")
	}
}

func TestParseProbeJSONRoto(t *testing.T) {
	if _, err := parseProbe([]byte(`{"streams":`)); err == nil {
		t.Fatal("un JSON truncado debería fallar")
	}
	if _, err := parseProbe([]byte(``)); err == nil {
		t.Fatal("una salida vacía debería fallar")
	}
}

func TestParseProbeDuracionBasura(t *testing.T) {
	// ffprobe a veces manda "N/A" o un número suelto en vez de string.
	const raro = `{"streams":[{"codec_name":"h264","codec_type":"video","width":640,"height":480,"duration":"N/A"}],"format":{"duration":"10.5"}}`
	m, err := parseProbe([]byte(raro))
	if err != nil {
		t.Fatalf("parseProbe: %v", err)
	}
	if m.Duration != 10.5 {
		t.Errorf("Duration = %v, quería 10.5", m.Duration)
	}

	const numerico = `{"streams":[{"codec_name":"h264","codec_type":"video","width":640,"height":480}],"format":{"duration":7}}`
	m, err = parseProbe([]byte(numerico))
	if err != nil {
		t.Fatalf("parseProbe con duration numérica: %v", err)
	}
	if m.Duration != 7 {
		t.Errorf("Duration = %v, quería 7", m.Duration)
	}
}

func TestFfmpegTailLines(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("linea ")
		b.WriteByte(byte('a' + i%26))
		b.WriteByte('\n')
	}
	got := tailLines(b.String(), 20)
	lines := strings.Split(got, "\n")
	if len(lines) != 20 {
		t.Fatalf("esperaba 20 líneas, obtuve %d", len(lines))
	}
	if !strings.HasPrefix(lines[len(lines)-1], "linea ") {
		t.Errorf("última línea inesperada: %q", lines[len(lines)-1])
	}

	// El progreso de ffmpeg usa \r: se normaliza y se toma la última.
	got = tailLines("frame=1\rframe=2\rframe=3\r", 5)
	if got != "frame=1\nframe=2\nframe=3" {
		t.Errorf("tailLines con \\r = %q", got)
	}
	if got := tailLines("", 5); got != "" {
		t.Errorf("tailLines vacío = %q", got)
	}
	if got := tailLines("   \n\n", 5); got != "" {
		t.Errorf("tailLines de solo espacios = %q", got)
	}
}

func TestFfmpegLocateReportaRutasProbadas(t *testing.T) {
	// En esta máquina ffmpeg existe (PATH o ~/.hermes/tools), así que el caso
	// interesante es el error: se prueba con un nombre que seguro no existe.
	if _, err := locate("binario-que-no-existe-b3l0w"); err == nil {
		t.Fatal("locate de un binario inexistente debería fallar")
	} else if !strings.Contains(err.Error(), "binario-que-no-existe-b3l0w") {
		t.Errorf("el error debería nombrar el binario: %v", err)
	}

	tool, err := Locate()
	if err != nil {
		t.Skipf("ffmpeg no disponible en esta máquina: %v", err)
	}
	if !tool.Ok || !filepath.IsAbs(tool.Path) {
		t.Errorf("Locate devolvió %+v, quería ruta absoluta y Ok", tool)
	}
}

func TestFfmpegProbeReal(t *testing.T) {
	if _, err := LocateProbe(); err != nil {
		t.Skipf("ffprobe no disponible: %v", err)
	}
	ff, err := Locate()
	if err != nil {
		t.Skipf("ffmpeg no disponible: %v", err)
	}

	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	// Clip sintético mínimo: 1 s de negro a 64x48, sin depender de fixtures.
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x48:d=1:r=10",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-an", clip}
	if err := Run(context.Background(), ff.Path, args...); err != nil {
		t.Fatalf("generar clip de prueba: %v", err)
	}

	m, err := Probe(context.Background(), clip)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if m.Width != 64 || m.Height != 48 {
		t.Errorf("dimensiones = %dx%d, quería 64x48", m.Width, m.Height)
	}
	if m.Codec != "h264" {
		t.Errorf("Codec = %q, quería h264", m.Codec)
	}
	if m.Duration < 0.5 || m.Duration > 2 {
		t.Errorf("Duration = %v, fuera de rango para un clip de 1 s", m.Duration)
	}
}

func TestProbeArchivoInexistente(t *testing.T) {
	if _, err := LocateProbe(); err != nil {
		t.Skipf("ffprobe no disponible: %v", err)
	}
	if _, err := Probe(context.Background(), filepath.Join(t.TempDir(), "nada.mp4")); err == nil {
		t.Fatal("Probe de un archivo inexistente debería fallar")
	}
}

func TestRunPropagaStderr(t *testing.T) {
	// sh siempre está; nos sirve para verificar que el error arrastra la cola.
	sh, err := locate("sh")
	if err != nil {
		t.Skipf("sh no disponible: %v", err)
	}
	err = Run(context.Background(), sh.Path, "-c", "echo boom-en-stderr >&2; exit 3")
	if err == nil {
		t.Fatal("un exit code distinto de 0 debería devolver error")
	}
	if !strings.Contains(err.Error(), "boom-en-stderr") {
		t.Errorf("el error debería incluir el stderr: %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("el error debería incluir el exit code: %v", err)
	}

	// Y no debe dejar basura ni fallar con exit 0.
	if err := Run(context.Background(), sh.Path, "-c", "true"); err != nil {
		t.Errorf("un comando exitoso no debería devolver error: %v", err)
	}
}

func TestProbeSinFfprobe(t *testing.T) {
	// El fallback por env apunta a un directorio vacío: con PATH vacío también,
	// la resolución tiene que fallar con un mensaje útil y no con un panic.
	t.Setenv(toolDirEnv, t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if _, err := LocateProbe(); err == nil {
		t.Skip("hay un ffprobe resoluble fuera de PATH/~/.hermes/tools")
	}
	if _, err := Probe(context.Background(), "x.mp4"); err == nil {
		t.Fatal("Probe sin ffprobe debería fallar")
	}
	if _, err := os.Stat("x.mp4"); err == nil {
		t.Fatal("no debería crear archivos")
	}
}
