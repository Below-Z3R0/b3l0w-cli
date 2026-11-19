package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// Los caminos de error del paquete: los que se necesitan para que el comando
// no se caiga en silencio cuando el disco, el manifiesto o el motor fallan.

func TestConvertFallaSiElCatalogoNoExiste(t *testing.T) {
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--root", filepath.Join(t.TempDir(), "no-existe"), "--id", "111")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "leer") {
		t.Errorf("stderr = %q, quería que diga que no pudo leer el catálogo", errSalida)
	}
}

func TestConvertFallaSiElManifiestoEsUnDirectorio(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()
	manifiesto := filepath.Join(out, "manifiesto")
	if err := os.MkdirAll(manifiesto, 0o755); err != nil {
		t.Fatal(err)
	}

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0", "--manifest", manifiesto)
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "guardar manifiesto") {
		t.Errorf("stderr = %q, quería que diga que no pudo guardar el manifiesto", errSalida)
	}
}

func TestConvertFallaSiNoPuedeCrearElDirectorioDeSalida(t *testing.T) {
	// Un archivo en el medio del camino: el mkdir no puede hacer su trabajo.
	base := t.TempDir()
	bloqueo := filepath.Join(base, "bloqueo")
	escribirArchivo(t, bloqueo, "bloqueo")
	out := filepath.Join(bloqueo, "salida")

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", t.TempDir(), "--id", "111")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "crear") {
		t.Errorf("stderr = %q, quería que diga que no pudo crear el directorio", errSalida)
	}
}

func TestPosterFallaSiNoPuedeCrearElDirectorioDeSalida(t *testing.T) {
	base := t.TempDir()
	bloqueo := filepath.Join(base, "bloqueo")
	escribirArchivo(t, bloqueo, "bloqueo")
	out := filepath.Join(bloqueo, "salida")

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", t.TempDir(), "--id", "111")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "crear") {
		t.Errorf("stderr = %q, quería que diga que no pudo crear el directorio", errSalida)
	}
}

func TestPosterRehaceElManifiestoIlegible(t *testing.T) {
	origen := t.TempDir()
	fixture := filepath.Join(origen, "base.png")
	escribirPNG(t, fixture)
	t.Setenv("WE_TEST_PNG", fixture)
	motorFalso(t, `
while [ $# -gt 0 ]; do
  case "$1" in
    --screenshot) shift; salida="$1" ;;
  esac
  shift
done
cp "$WE_TEST_PNG" "$salida"
`)

	root := raizWorkshop(t, map[string]string{"111": `{"title":"Alfa","type":"video"}`})
	out := t.TempDir()
	escribirArchivo(t, filepath.Join(out, "manifest.json"), "{ roto")

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("el manifiesto ilegible no se rehizo: %v (%q)", err, datos)
	}
	if len(m.Items) != 1 || m.Items[0].ID != "111" {
		t.Errorf("manifiesto = %+v, quería el item 111", m.Items)
	}
}

func TestPosterFallaSiElManifiestoEsUnDirectorio(t *testing.T) {
	origen := t.TempDir()
	fixture := filepath.Join(origen, "base.png")
	escribirPNG(t, fixture)
	t.Setenv("WE_TEST_PNG", fixture)
	motorFalso(t, `
while [ $# -gt 0 ]; do
  case "$1" in
    --screenshot) shift; salida="$1" ;;
  esac
  shift
done
cp "$WE_TEST_PNG" "$salida"
`)

	root := raizWorkshop(t, map[string]string{"111": `{"title":"Alfa","type":"video"}`})
	out := t.TempDir()
	manifiesto := filepath.Join(out, "manifiesto")
	if err := os.MkdirAll(manifiesto, 0o755); err != nil {
		t.Fatal(err)
	}

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111", "--manifest", manifiesto)
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "guardar manifiesto") {
		t.Errorf("stderr = %q, quería que diga que no pudo guardar el manifiesto", errSalida)
	}
}

func TestPosterMarcaElWallpaperQueFallo(t *testing.T) {
	// El motor falla para un wallpaper y funciona para el otro: el lote
	// sigue, el fallido queda marcado en el manifiesto y el resumen cuenta.
	origen := t.TempDir()
	fixture := filepath.Join(origen, "base.png")
	escribirPNG(t, fixture)
	t.Setenv("WE_TEST_PNG", fixture)
	motorFalso(t, `
case "$*" in
  *"--bg 222"*) exit 1 ;;
esac
while [ $# -gt 0 ]; do
  case "$1" in
    --screenshot) shift; salida="$1" ;;
  esac
  shift
done
cp "$WE_TEST_PNG" "$salida"
`)

	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"video"}`,
	})
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--all")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0: un fallo parcial no es fatal (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(errSalida, "1 fallidos") {
		t.Errorf("stderr = %q, quería el conteo de fallidos", errSalida)
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatal(err)
	}
	estados := map[string]string{}
	for _, e := range m.Items {
		estados[e.ID] = e.Status
	}
	if estados["111"] != "ok" {
		t.Errorf("el item 111 quedó %q, quería ok", estados["111"])
	}
	if estados["222"] != "failed" {
		t.Errorf("el item 222 quedó %q, quería failed", estados["222"])
	}
}

func TestInfoMuestraElErrorDeUnItemRoto(t *testing.T) {
	homeConWorkshop(t, map[string]string{"333": `{ roto`})

	codigo, salida, errSalida := correr(t, "info", "333")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "error:") {
		t.Errorf("stdout = %q, quería que muestre el error del item roto", salida)
	}
}

func TestInfoFallaSiNoHayWorkshop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	codigo, _, errSalida := correr(t, "info", "111")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no encontré el workshop") {
		t.Errorf("stderr = %q, quería que diga que no encontró el workshop", errSalida)
	}
}

func TestStillFallaSiNoHayFfmpeg(t *testing.T) {
	pathSinBinarios(t)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")

	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"))
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "ffmpeg") {
		t.Errorf("stderr = %q, quería que nombre el ffmpeg que falta", errSalida)
	}
}

func TestSolidFallaSiNoPuedeEscribirElArchivo(t *testing.T) {
	// El destino es una carpeta: no se puede abrir como archivo.
	dir := t.TempDir()
	out := filepath.Join(dir, "carpeta")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", out, "--size", "2x2")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no se pudo escribir") {
		t.Errorf("stderr = %q, quería que diga que no pudo escribir el archivo", errSalida)
	}
}

func TestSolidFallaSiNoPuedeCrearElDirectorio(t *testing.T) {
	base := t.TempDir()
	bloqueo := filepath.Join(base, "bloqueo")
	escribirArchivo(t, bloqueo, "bloqueo")
	out := filepath.Join(bloqueo, "sub", "fondo.png")

	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", out, "--size", "2x2")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no se pudo crear") {
		t.Errorf("stderr = %q, quería que diga que no pudo crear el directorio", errSalida)
	}
}

func TestScanIgnoraLosArchivosOcultos(t *testing.T) {
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "visible.png"), "x")
	escribirArchivo(t, filepath.Join(dir, ".oculto.png"), "x")

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	lineas := strings.Fields(salida)
	if len(lineas) != 1 {
		t.Fatalf("listó %d archivos (%v), quería 1", len(lineas), lineas)
	}
	if !strings.HasSuffix(lineas[0], "visible.png") {
		t.Errorf("listó %q, quería el archivo visible", lineas[0])
	}
}

func TestScanNoRepiteElMismoArchivo(t *testing.T) {
	// La misma carpeta dos veces: el segundo recorrido no duplica nada.
	dir := arbolDePrueba(t)

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir, "--dir", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if lineas := strings.Fields(salida); len(lineas) != 4 {
		t.Errorf("listó %d archivos (%v), quería 4 sin repeticiones", len(lineas), lineas)
	}
}

func TestScanSinFfmpegNoGeneraMiniaturas(t *testing.T) {
	pathSinBinarios(t)
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "fondo.png"), "x")
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
	if items[0].Thumb != "" {
		t.Errorf("thumb = %q, quería vacío: sin ffmpeg no hay miniatura", items[0].Thumb)
	}
}

func TestScanDejaLaMiniaturaVaciaSiFfmpegFalla(t *testing.T) {
	stubFFmpeg(t, `exit 1`)
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "fondo.png"), "x")
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
	if items[0].Thumb != "" {
		t.Errorf("thumb = %q, quería vacío: el ffmpeg falló", items[0].Thumb)
	}
}
