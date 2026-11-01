package we

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// png1x1 es un PNG válido mínimo. El engine real escribe PNGs, así que el falso
// tiene que hacer lo mismo para pasar la validación de pngComplete.
const png1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// fakePNG deja el PNG de fixture en disco y devuelve su ruta; los engines
// falsos lo copian con cat al destino de --screenshot.
func fakePNG(t *testing.T) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(png1x1)
	if err != nil {
		t.Fatalf("fixture PNG inválido: %v", err)
	}
	p := filepath.Join(t.TempDir(), "fixture.png")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("escribir fixture PNG: %v", err)
	}
	return p
}

// pngEscribe es el cuerpo de un engine que captura bien.
const pngEscribe = "cat \"$B3L0W_TEST_PNG\" > \"$out\"\n"

// fakeEngine escribe un engine de mentira: no toca GPU ni display. El cuerpo
// recibe los mismos argumentos que el binario real y deja el PNG en $out.
func fakeEngine(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "linux-wallpaperengine-fake")
	script := "#!/bin/sh\n" +
		"out=\"\"\nprev=\"\"\nbg=\"\"\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$prev\" in\n" +
		"    --screenshot) out=\"$a\" ;;\n" +
		"    --bg) bg=\"$a\" ;;\n" +
		"  esac\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		body
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("escribir engine falso: %v", err)
	}
	return p
}

// writePNG arma un archivo falso en dir para simular un poster ya generado.
func writePNG(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPosterOptionsNormalize(t *testing.T) {
	o := PosterOptions{}.normalize()
	if o.Size != DefaultPosterSize {
		t.Errorf("Size por defecto = %v, quería %v", o.Size, DefaultPosterSize)
	}
	if o.Delay != defaultScreenshotDelay {
		t.Errorf("Delay por defecto = %d, quería %d", o.Delay, defaultScreenshotDelay)
	}
	want := runtime.NumCPU() / 2
	if want > maxDefaultJobs {
		want = maxDefaultJobs
	}
	if want < 1 {
		want = 1
	}
	if o.Jobs != want {
		t.Errorf("Jobs por defecto = %d, quería %d", o.Jobs, want)
	}

	// Un tamaño a medio setear no sirve: cae al default completo.
	if got := (PosterOptions{Size: [2]int{800, 0}}).normalize().Size; got != DefaultPosterSize {
		t.Errorf("Size parcial = %v, quería %v", got, DefaultPosterSize)
	}
	if got := (PosterOptions{Size: [2]int{-1, 720}}).normalize().Size; got != DefaultPosterSize {
		t.Errorf("Size negativo = %v, quería %v", got, DefaultPosterSize)
	}

	// Lo explícito se respeta y no se recorta.
	in := PosterOptions{Size: [2]int{1920, 1080}, Delay: 5, Jobs: 3, OutDir: "/tmp/x"}
	got := in.normalize()
	if got.Size != in.Size || got.Delay != in.Delay || got.Jobs != in.Jobs || got.OutDir != in.OutDir {
		t.Errorf("normalize pisó valores explícitos: %+v", got)
	}
}

func TestPosterSalteaSiElPNGYaExiste(t *testing.T) {
	out := t.TempDir()
	previo := writePNG(t, out, "424242.png", "png-viejo")
	// Engine inexistente a propósito: si se invocara, Poster fallaría.
	opt := PosterOptions{OutDir: out, EnginePath: filepath.Join(t.TempDir(), "no-existe")}

	path, err := Poster(context.Background(), Item{ID: "424242", Title: "Campfire PC"}, opt)
	if err != nil {
		t.Fatalf("Poster con PNG existente no debe fallar: %v", err)
	}
	if path != previo {
		t.Errorf("path = %q, quería %q", path, previo)
	}
	b, err := os.ReadFile(previo)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "png-viejo" {
		t.Errorf("el PNG existente se tocó: %q", string(b))
	}
}

func TestPosterOverwriteSiPideRender(t *testing.T) {
	out := t.TempDir()
	writePNG(t, out, "7.png", "png-viejo")
	opt := PosterOptions{OutDir: out, Overwrite: true, EnginePath: filepath.Join(t.TempDir(), "no-existe")}
	if _, err := Poster(context.Background(), Item{ID: "7"}, opt); err == nil {
		t.Fatal("con Overwrite y engine inexistente debería fallar")
	} else if !strings.Contains(err.Error(), "engine") {
		t.Errorf("el error no menciona el engine: %v", err)
	}
}

func TestPosterRenderizaConEngineFalso(t *testing.T) {
	out := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("B3L0W_TEST_PNG", fakePNG(t))
	t.Setenv("B3L0W_TEST_LOG", logPath)
	engine := fakeEngine(t, "if [ -n \"$B3L0W_TEST_LOG\" ]; then printf '%s\\n' \"$@\" > \"$B3L0W_TEST_LOG\"; fi\n"+pngEscribe)

	path, err := Poster(context.Background(), Item{ID: "3582809467", Title: "Campfire"}, PosterOptions{
		OutDir:     out,
		EnginePath: engine,
	})
	if err != nil {
		t.Fatalf("Poster: %v", err)
	}
	want := filepath.Join(out, "3582809467.png")
	if path != want {
		t.Errorf("path = %q, quería %q", path, want)
	}
	if !pngComplete(path) {
		t.Error("el poster generado no es un PNG completo")
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("el engine no registró argumentos: %v", err)
	}
	linea := string(args)
	for _, token := range []string{"0x0x1280x720", "--bg", "3582809467", "--screenshot-delay", "30"} {
		if !strings.Contains(linea, token) {
			t.Errorf("faltó %q en los argumentos:\n%s", token, linea)
		}
	}
	// El temporal se limpia: en el directorio queda sólo el PNG final.
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("quedaron archivos de más en el out: %v", entries)
	}
}

func TestPosterRespetaSizeYDelay(t *testing.T) {
	out := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("B3L0W_TEST_PNG", fakePNG(t))
	t.Setenv("B3L0W_TEST_LOG", logPath)
	engine := fakeEngine(t, "if [ -n \"$B3L0W_TEST_LOG\" ]; then printf '%s\\n' \"$@\" > \"$B3L0W_TEST_LOG\"; fi\n"+pngEscribe)

	_, err := Poster(context.Background(), Item{ID: "9"}, PosterOptions{
		OutDir:     out,
		Size:       [2]int{1920, 1080},
		Delay:      5,
		EnginePath: engine,
	})
	if err != nil {
		t.Fatalf("Poster: %v", err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "0x0x1920x1080") || !strings.Contains(string(b), "\n5\n") {
		t.Errorf("no se respetaron size/delay:\n%s", string(b))
	}
}

func TestPosterValidaEntrada(t *testing.T) {
	if _, err := Poster(context.Background(), Item{ID: "1"}, PosterOptions{}); err == nil {
		t.Error("sin OutDir debería fallar")
	}
	if _, err := Poster(context.Background(), Item{}, PosterOptions{OutDir: t.TempDir()}); err == nil {
		t.Error("sin ID debería fallar")
	}
}

func TestPosterContextCancelado(t *testing.T) {
	t.Setenv("B3L0W_TEST_PNG", fakePNG(t))
	engine := fakeEngine(t, pngEscribe)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Poster(ctx, Item{ID: "1"}, PosterOptions{OutDir: t.TempDir(), EnginePath: engine})
	if err == nil {
		t.Fatal("con el context cancelado debería fallar")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("el error no envuelve context.Canceled: %v", err)
	}
}

func TestPosterCancelaYNoDejaElEngineVivo(t *testing.T) {
	// El engine falso no escribe nada y se duerme: el corte tiene que venir de
	// la cancelación, no del timeout largo.
	engine := fakeEngine(t, "sleep 30\n")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Poster(ctx, Item{ID: "1"}, PosterOptions{OutDir: t.TempDir(), EnginePath: engine})
	if err == nil {
		t.Fatal("debería fallar por cancelación")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("el error no envuelve context.Canceled: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("no cortó el render a tiempo: %s", elapsed)
	}
}

func TestPosterTimeoutSinPNG(t *testing.T) {
	oldTimeout, oldPoll := renderTimeout, pollInterval
	renderTimeout, pollInterval = 300*time.Millisecond, 20*time.Millisecond
	defer func() { renderTimeout, pollInterval = oldTimeout, oldPoll }()

	engine := fakeEngine(t, "sleep 30\n")
	start := time.Now()
	_, err := Poster(context.Background(), Item{ID: "1"}, PosterOptions{OutDir: t.TempDir(), EnginePath: engine})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("esperaba error de timeout, obtuve %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("el timeout no cortó a tiempo: %s", elapsed)
	}
}

func TestPostersFallaParcialNoAborta(t *testing.T) {
	out := t.TempDir()
	t.Setenv("B3L0W_TEST_PNG", fakePNG(t))
	engine := fakeEngine(t, "if [ \"$bg\" = \"222\" ]; then echo 'engine roto' >&2; exit 3; fi\n"+pngEscribe)
	items := []Item{{ID: "111"}, {ID: "222"}, {ID: "333"}}

	res, err := Posters(context.Background(), items, PosterOptions{OutDir: out, Jobs: 2, EnginePath: engine})
	if err != nil {
		t.Fatalf("un solo fallo no debe abortar el lote: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("esperaba 3 resultados, obtuve %d", len(res))
	}
	if res[1].ID != "222" || res[1].Err == "" {
		t.Errorf("el item 222 debería venir con Err: %+v", res[1])
	}
	if !strings.Contains(res[1].Err, "engine roto") {
		t.Errorf("el error no trae la salida del engine: %q", res[1].Err)
	}
	for _, i := range []int{0, 2} {
		if res[i].Err != "" {
			t.Errorf("%s no debería fallar: %q", res[i].ID, res[i].Err)
		}
		if _, err := os.Stat(res[i].Path); err != nil {
			t.Errorf("no quedó el PNG de %s: %v", res[i].ID, err)
		}
	}
}

func TestPostersSkipYOverwrite(t *testing.T) {
	out := t.TempDir()
	t.Setenv("B3L0W_TEST_PNG", fakePNG(t))
	previo := writePNG(t, out, "111.png", "png-viejo")
	engine := fakeEngine(t, pngEscribe)
	items := []Item{{ID: "111"}, {ID: "222"}}

	res, err := Posters(context.Background(), items, PosterOptions{OutDir: out, Jobs: 2, EnginePath: engine})
	if err != nil {
		t.Fatalf("Posters: %v", err)
	}
	if !res[0].Skipped || res[0].Path != previo || res[0].Err != "" {
		t.Errorf("111 debería venir salteado: %+v", res[0])
	}
	if res[1].Skipped || res[1].Err != "" {
		t.Errorf("222 debería renderizarse: %+v", res[1])
	}
	if b, _ := os.ReadFile(previo); string(b) != "png-viejo" {
		t.Errorf("el PNG salteado se tocó: %q", string(b))
	}

	res, err = Posters(context.Background(), items, PosterOptions{OutDir: out, Jobs: 1, Overwrite: true, EnginePath: engine})
	if err != nil {
		t.Fatalf("Posters con Overwrite: %v", err)
	}
	if res[0].Skipped {
		t.Errorf("con Overwrite no debería saltear: %+v", res[0])
	}
	if !pngComplete(previo) {
		t.Error("Overwrite no regeneró un PNG válido")
	}
}

func TestPostersVacioYTodoFallido(t *testing.T) {
	res, err := Posters(context.Background(), nil, PosterOptions{OutDir: t.TempDir()})
	if err != nil || len(res) != 0 {
		t.Fatalf("lote vacío: %v %+v", err, res)
	}
	if _, err := Posters(context.Background(), []Item{{ID: "1"}}, PosterOptions{}); err == nil {
		t.Error("sin OutDir debería fallar antes de arrancar workers")
	}

	engine := fakeEngine(t, "echo 'no renderiza' >&2\nexit 1\n")
	res, err = Posters(context.Background(), []Item{{ID: "1"}, {ID: "2"}}, PosterOptions{OutDir: t.TempDir(), Jobs: 2, EnginePath: engine})
	if err == nil {
		t.Fatal("si fallan todos debería devolver error agregado")
	}
	if len(res) != 2 || res[0].Err == "" || res[1].Err == "" {
		t.Errorf("los resultados parciales deberían viajar igual: %+v", res)
	}
}
