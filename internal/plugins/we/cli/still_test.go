package wecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubFFmpeg pone un ffmpeg de mentira en el PATH que anota los argumentos
// con los que lo llamaron y escribe el archivo de salida. El ffmpeg real no
// se ejecuta nunca en los tests.
func stubFFmpeg(t *testing.T, cuerpo string) string {
	t.Helper()
	dir := stubEnPATH(t, "ffmpeg", "#!/bin/sh\n"+cuerpo+"\n")
	t.Setenv("WE_TEST_LOG", filepath.Join(t.TempDir(), "ffmpeg.log"))
	return dir
}

// argumentosDeFfmpeg devuelve los argumentos con los que se llamó al stub.
func argumentosDeFfmpeg(t *testing.T) []string {
	t.Helper()
	datos, err := os.ReadFile(os.Getenv("WE_TEST_LOG"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil // el stub nunca corrió
		}
		t.Fatalf("leer el log del ffmpeg falso: %v", err)
	}
	var salida []string
	for _, linea := range strings.Split(strings.TrimSpace(string(datos)), "\n") {
		if linea != "" {
			salida = append(salida, linea)
		}
	}
	return salida
}

func TestStillSacaElCuadroDeUnVideo(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	out := filepath.Join(dir, "cuadro.jpg")

	codigo, salida, errSalida := correr(t, "still", "--file", video, "--out", out)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, out) {
		t.Errorf("stdout = %q, quería la ruta de la imagen", salida)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("no escribió la imagen: %v", err)
	}

	args := argumentosDeFfmpeg(t)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-y", "-i " + video, "frames:v 1", "-vf scale=320:-2", "-q:v 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args de ffmpeg = %q, faltaba %q", joined, want)
		}
	}
	// ffmpeg escribe a un temporal en el mismo directorio (con la misma
	// extensión) y el comando lo renombra al final: el destino nunca queda
	// a medias.
	// El log guarda una línea por llamada: el último argumento es el archivo.
	partes := strings.Fields(args[len(args)-1])
	if len(partes) == 0 {
		t.Fatal("ffmpeg no recibió ningún argumento")
	}
	temporal := partes[len(partes)-1]
	if filepath.Dir(temporal) != filepath.Dir(out) {
		t.Errorf("ffmpeg escribió en %q, tenía que ser en la carpeta de %q", temporal, out)
	}
	if filepath.Ext(temporal) != filepath.Ext(out) {
		t.Errorf("el temporal %q no tiene la extensión de %q", temporal, out)
	}
	if temporal == out {
		t.Error("ffmpeg no debería escribir directamente sobre el destino")
	}
}

func TestStillBuscaElSegundoPedido(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")

	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"), "--at", "2.5")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	args := argumentosDeFfmpeg(t)
	if !strings.Contains(strings.Join(args, " "), "-ss 2.50") {
		t.Errorf("args de ffmpeg = %q, quería -ss 2.50", args)
	}
}

func TestStillReintentaDesdeElPrincipio(t *testing.T) {
	// Un video corto falla en el segundo pedido: el comando reintenta desde
	// el principio antes de darse por vencido.
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
case "$*" in
  *"-ss 2.50"*) echo "no hay frame" >&2; exit 1 ;;
esac
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")

	codigo, salida, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"), "--at", "2.5")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 tras el reintento (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "c.jpg") {
		t.Errorf("stdout = %q, quería la ruta de la imagen", salida)
	}
	args := strings.Join(argumentosDeFfmpeg(t), " ")
	if !strings.Contains(args, "-ss 2.50") || !strings.Contains(args, "-ss 0") {
		t.Errorf("args de ffmpeg = %q, quería el intento en 2.50 y el reintento en 0", args)
	}
}

func TestStillDeUnaImagenNoBuscaFrame(t *testing.T) {
	// Un JPG ya es un cuadro: pasarle -ss hace que ffmpeg no escriba nada.
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	imagen := filepath.Join(dir, "foto.jpg")
	escribirArchivo(t, imagen, "imagen falsa")

	codigo, _, errSalida := correr(t, "still", "--file", imagen, "--out", filepath.Join(dir, "c.jpg"))
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if args := strings.Join(argumentosDeFfmpeg(t), " "); strings.Contains(args, "-ss") {
		t.Errorf("args de ffmpeg = %q, no quería -ss para una imagen", args)
	}
}

func TestStillDeUnGifArrancaEnCero(t *testing.T) {
	// Un GIF suele durar menos de un segundo: buscar el segundo 1 no
	// devuelve nada.
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	gif := filepath.Join(dir, "anim.gif")
	escribirArchivo(t, gif, "gif falso")

	codigo, _, errSalida := correr(t, "still", "--file", gif, "--out", filepath.Join(dir, "c.jpg"), "--at", "5")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	args := strings.Join(argumentosDeFfmpeg(t), " ")
	if !strings.Contains(args, "-ss 0") {
		t.Errorf("args de ffmpeg = %q, quería -ss 0", args)
	}
	if strings.Contains(args, "-ss 5") {
		t.Errorf("args de ffmpeg = %q, no quería saltar al segundo 5", args)
	}
}

func TestStillFallaSiElArchivoNoExiste(t *testing.T) {
	dir := t.TempDir()
	codigo, _, errSalida := correr(t, "still", "--file", filepath.Join(dir, "no-existe.mp4"), "--out", filepath.Join(dir, "c.jpg"))
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no puedo leer") {
		t.Errorf("stderr = %q, quería que diga que no puede leer", errSalida)
	}
}

func TestStillSinFileNiOutFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "still")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--file") || !strings.Contains(errSalida, "--out") {
		t.Errorf("stderr = %q, quería que pida los dos flags", errSalida)
	}
}

func TestStillSizeInvalidoFalla(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"), "--size", "grande")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--size") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestStillAtInvalidoFalla(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"), "--at", "-3")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--at") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestStillOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "still", "--file", "x", "--out", "y", "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestStillNoBorraLaMiniaturaPreviaSiFfmpegFalla(t *testing.T) {
	// El comando no puede dejar al usuario sin miniatura: si ffmpeg falla, la
	// que ya estaba tiene que seguir en disco.
	stubFFmpeg(t, `exit 1`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	out := filepath.Join(dir, "thumbs", "t.jpg")
	escribirArchivo(t, out, "MINIATURA PREVIA")

	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", out)
	if codigo != 1 {
		t.Fatalf("código = %d, quería 1 (stderr: %s)", codigo, errSalida)
	}
	contenido, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("la miniatura previa no quedó en disco: %v", err)
	}
	if string(contenido) != "MINIATURA PREVIA" {
		t.Errorf("la miniatura previa cambió a %q", contenido)
	}
}

func TestStillFallaSiFfmpegNoEscribe(t *testing.T) {
	stubFFmpeg(t, `echo "$@" >> "$WE_TEST_LOG"`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")

	codigo, _, errSalida := correr(t, "still", "--file", video, "--out", filepath.Join(dir, "c.jpg"))
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no se pudo extraer") {
		t.Errorf("stderr = %q, quería que diga que no pudo extraer", errSalida)
	}
}
