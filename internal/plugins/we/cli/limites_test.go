package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// Los bordes del contrato: flags que cambian el resultado, rutas imposibles
// y títulos que no tienen texto que limpiar.

func TestConvertConTodosProcesaTodoElCatalogo(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
		"222": `{"title":"Beta","type":"video","file":"b.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video uno")
	escribirArchivo(t, filepath.Join(root, "222", "b.mp4"), "video dos")
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--all", "--crf", "0")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	for _, nombre := range []string{"111-alfa.mp4", "222-beta.mp4"} {
		if _, err := os.Stat(filepath.Join(out, nombre)); err != nil {
			t.Errorf("no extrajo %s: %v", nombre, err)
		}
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != 2 {
		t.Errorf("manifiesto = %+v, quería los 2 items del catálogo", m.Items)
	}
}

func TestConvertSobreescribeLoQueYaExiste(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	if codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0"); codigo != 0 {
		t.Fatalf("primera corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0", "--overwrite")
	if codigo != 0 {
		t.Fatalf("segunda corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != 1 || m.Items[0].Status != "ok" {
		t.Errorf("manifiesto = %+v, quería el item 111 con status ok: con --overwrite se rehace", m.Items)
	}
}

func TestConvertAceptaElFormatoWebm(t *testing.T) {
	// Webm no se copia: se recodifica, así que el ffmpeg falso recibe los
	// argumentos del vp9 y el archivo sale con la extensión pedida.
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'webm falso' > "$out"
`)
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--format", "webm")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if _, err := os.Stat(filepath.Join(out, "111-alfa.webm")); err != nil {
		t.Errorf("no escribió el webm: %v", err)
	}
	args := strings.Join(argumentosDeFfmpeg(t), " ")
	if !strings.Contains(args, "libvpx-vp9") {
		t.Errorf("args de ffmpeg = %q, quería libvpx-vp9", args)
	}
}

func TestConvertAceptaElMetodoExtract(t *testing.T) {
	// El flag se acepta y no rompe la extracción. (Que el método "record"
	// no esté implementado en esta capa está anotado en bugs.md.)
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--method", "extract", "--crf", "0")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if _, err := os.Stat(filepath.Join(out, "111-alfa.mp4")); err != nil {
		t.Errorf("no extrajo el video: %v", err)
	}
}

func TestConvertFallaSiElFormatoNoExiste(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--format", "avi")
	if codigo != 2 {
		t.Errorf("código = %d, quería 2 (todo falló)", codigo)
	}
	if !strings.Contains(errSalida, "mp4 o webm") {
		t.Errorf("stderr = %q, quería que diga los formatos soportados", errSalida)
	}
}

func TestPosterFallaSiElCatalogoNoExiste(t *testing.T) {
	codigo, _, errSalida := correr(t, "poster", "--out", t.TempDir(), "--root", filepath.Join(t.TempDir(), "no-existe"), "--id", "111")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "leer") {
		t.Errorf("stderr = %q, quería que diga que no pudo leer el catálogo", errSalida)
	}
}

func TestScanAceptaElDirConIgual(t *testing.T) {
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "a.jpg"), "x")

	codigo, salida, errSalida := correr(t, "scan", "--dir="+dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "a.jpg") {
		t.Errorf("stdout = %q, quería que liste el contenido", salida)
	}
}

func TestScanIgnoraCarpetaSinPermisos(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos no bloquean la lectura")
	}
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "visible.jpg"), "x")
	cerrada := filepath.Join(dir, "cerrada")
	escribirArchivo(t, filepath.Join(cerrada, "oculta.jpg"), "x")
	if err := os.Chmod(cerrada, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cerrada, 0o755) })

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0: una carpeta sin permisos no corta el escaneo (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "visible.jpg") {
		t.Errorf("stdout = %q, quería que liste lo que sí se puede leer", salida)
	}
}

func TestScanThumbsSinCarpetaValidaNoRompeElListado(t *testing.T) {
	// La carpeta de miniaturas no se puede crear: el listado sigue igual,
	// sólo sin miniaturas.
	base := t.TempDir()
	bloqueo := filepath.Join(base, "bloqueo")
	escribirArchivo(t, bloqueo, "bloqueo")
	minis := filepath.Join(bloqueo, "minis")
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "fondo.png"), "x")

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
		t.Errorf("thumb = %q, quería vacío: no se pudo crear la carpeta", items[0].Thumb)
	}
}

func TestSolidSizeConLetrasFalla(t *testing.T) {
	// "8xzz": tiene el separador pero los números no son números.
	dir := t.TempDir()
	codigo, _, errSalida := correr(t, "solid", "--color", "#fff", "--out", filepath.Join(dir, "f.png"), "--size", "8xzz")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--size inválido") {
		t.Errorf("stderr = %q, quería que diga que el tamaño no sirve", errSalida)
	}
}

func TestStillFallaSiNoPuedeCrearElDirectorio(t *testing.T) {
	stubFFmpeg(t, `exit 0`)
	base := t.TempDir()
	bloqueo := filepath.Join(base, "bloqueo")
	escribirArchivo(t, bloqueo, "bloqueo")
	video := filepath.Join(base, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	out := filepath.Join(bloqueo, "cuadro.jpg")

	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", out)
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "crear") {
		t.Errorf("stderr = %q, quería que diga que no pudo crear el directorio", errSalida)
	}
}

func TestHumanTitleSinTextoDevuelveElNombre(t *testing.T) {
	// Un nombre que es sólo extensión no deja nada que limpiar: se devuelve
	// tal cual para que el panel no muestre una tarjeta sin título.
	if got := humanTitle(".png"); got != ".png" {
		t.Errorf("humanTitle(\".png\") = %q, quería \".png\"", got)
	}
}
