package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThumbNameUsaElHashAcordadoConElPlugin(t *testing.T) {
	// El nombre de la miniatura es contrato con el plugin de Noctalia:
	// si cambia la fórmula, el plugin deja de encontrar las miniaturas.
	// Los valores salen de djb2 de 32 bits (hash*33 + byte) sobre la ruta.
	casos := map[string]string{
		"/tmp/x/a.jpg":        "v-5c3f9fab.jpg",
		"/tmp/dd/img_001.png": "v-6133dc4b.jpg",
	}
	for ruta, want := range casos {
		if got := thumbName(ruta); got != want {
			t.Errorf("thumbName(%q) = %q, quería %q", ruta, got, want)
		}
	}
}

func TestThumbNameCambiaConLaRuta(t *testing.T) {
	if thumbName("/a.jpg") == thumbName("/b.jpg") {
		t.Error("dos rutas distintas dieron el mismo nombre de miniatura")
	}
}

func TestFreshSoloDaPorFrescoSiLaMiniaturaEsMasNueva(t *testing.T) {
	dir := t.TempDir()
	origen := filepath.Join(dir, "a.jpg")
	miniatura := filepath.Join(dir, "v.jpg")
	escribirArchivo(t, origen, "x")
	escribirArchivo(t, miniatura, "x")

	// La miniatura es más vieja que el original: hay que rehacerla.
	pasado := time.Now().Add(-time.Hour)
	if err := os.Chtimes(miniatura, pasado, pasado); err != nil {
		t.Fatal(err)
	}
	if fresh(miniatura, origen) {
		t.Error("una miniatura más vieja no debería dar por fresca")
	}

	// Más nueva y con contenido: sí.
	escribirArchivo(t, miniatura, "contenido")
	futuro := time.Now().Add(time.Hour)
	if err := os.Chtimes(miniatura, futuro, futuro); err != nil {
		t.Fatal(err)
	}
	if !fresh(miniatura, origen) {
		t.Error("una miniatura más nueva debería dar por fresca")
	}

	// Vacía no sirve: el plugin mostraría un archivo de 0 bytes.
	if err := os.Truncate(miniatura, 0); err != nil {
		t.Fatal(err)
	}
	if fresh(miniatura, origen) {
		t.Error("una miniatura vacía no debería dar por fresca")
	}

	// Si falta alguno de los dos archivos, no hay nada que reusar.
	if fresh(filepath.Join(dir, "no-existe"), origen) {
		t.Error("una miniatura inexistente no debería dar por fresca")
	}
	if fresh(miniatura, filepath.Join(dir, "no-existe")) {
		t.Error("sin origen no hay miniatura fresca")
	}
}

func TestScanGeneraLasMiniaturas(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	imagen := filepath.Join(dir, "fondo.png")
	escribirArchivo(t, imagen, "x")
	minis := filepath.Join(t.TempDir(), "minis")

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir, "--thumbs", minis, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, quería 1", len(items))
	}
	esperado := filepath.Join(minis, thumbName(imagen))
	if items[0].Thumb != esperado {
		t.Errorf("thumb = %q, quería %q", items[0].Thumb, esperado)
	}
	if _, err := os.Stat(esperado); err != nil {
		t.Errorf("no escribió la miniatura: %v", err)
	}
	args := strings.Join(argumentosDeFfmpeg(t), " ")
	if !strings.Contains(args, "-vf scale=320:-2") {
		t.Errorf("args de ffmpeg = %q, quería el escalado a 320", args)
	}
	if !strings.Contains(args, "-threads 1") {
		t.Errorf("args de ffmpeg = %q, quería -threads 1 (una imagen por proceso)", args)
	}
}

func TestScanNoGeneraMiniaturasDeVideos(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "clip.mp4"), "x")
	minis := filepath.Join(t.TempDir(), "minis")

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir, "--kind", "video", "--thumbs", minis, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 || items[0].Thumb != "" {
		t.Errorf("items = %+v, quería el video sin miniatura", items)
	}
	if len(argumentosDeFfmpeg(t)) != 0 {
		t.Errorf("se llamó a ffmpeg (%v), no debía", argumentosDeFfmpeg(t))
	}
}

func TestScanNoDejaUnaMiniaturaACortarEnDisco(t *testing.T) {
	// Un ffmpeg cortado escribe un JPEG a medias. La miniatura rota no puede
	// quedar en disco: el caché la daría por fresca en la corrida siguiente.
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'JPEG-PARCIAL' > "$out"
exit 1
`)
	dir := t.TempDir()
	imagen := filepath.Join(dir, "fondo.png")
	escribirArchivo(t, imagen, "x")
	minis := filepath.Join(t.TempDir(), "minis")

	if codigo, _, errSalida := correr(t, "scan", "--dir", dir, "--thumbs", minis, "--json"); codigo != 0 {
		t.Fatalf("primera pasada: código = %d (stderr: %s)", codigo, errSalida)
	}
	esperado := filepath.Join(minis, thumbName(imagen))
	if _, err := os.Stat(esperado); err == nil {
		contenido, _ := os.ReadFile(esperado)
		t.Errorf("quedó una miniatura a medias en disco: %s (%q)", esperado, contenido)
	}
	// La segunda pasada tiene que volver a llamar a ffmpeg: la miniatura rota
	// no puede quedar cacheada como válida.
	llamadasAntes := len(argumentosDeFfmpeg(t))
	if codigo, _, errSalida := correr(t, "scan", "--dir", dir, "--thumbs", minis, "--json"); codigo != 0 {
		t.Fatalf("segunda pasada: código = %d (stderr: %s)", codigo, errSalida)
	}
	if len(argumentosDeFfmpeg(t)) == llamadasAntes {
		t.Error("la segunda pasada no volvió a llamar a ffmpeg: la miniatura rota quedó cacheada")
	}
}

func TestScanReusaLaMiniaturaExistente(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	imagen := filepath.Join(dir, "fondo.png")
	escribirArchivo(t, imagen, "x")
	minis := filepath.Join(t.TempDir(), "minis")

	if codigo, _, errSalida := correr(t, "scan", "--dir", dir, "--thumbs", minis, "--json"); codigo != 0 {
		t.Fatalf("primera pasada: código = %d (stderr: %s)", codigo, errSalida)
	}
	llamadas := len(argumentosDeFfmpeg(t))
	if llamadas != 1 {
		t.Fatalf("ffmpeg se llamó %d veces, quería 1", llamadas)
	}

	// La segunda pasada tiene que reusar la miniatura: el original no cambió.
	if codigo, _, errSalida := correr(t, "scan", "--dir", dir, "--thumbs", minis, "--json"); codigo != 0 {
		t.Fatalf("segunda pasada: código = %d (stderr: %s)", codigo, errSalida)
	}
	if llamadas := len(argumentosDeFfmpeg(t)); llamadas != 1 {
		t.Errorf("ffmpeg se llamó %d veces en total, quería 1 (la segunda pasada reusa)", llamadas)
	}
}
