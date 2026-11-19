package wecli

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captura corre fn con el descriptor indicado redirigido a un pipe y
// devuelve lo que se escribió. La lectura va en una goroutine para que una
// salida larga no tape al comando.
func captura(t *testing.T, destino **os.File, fn func()) string {
	t.Helper()
	original := *destino
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	*destino = w
	hecho := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		hecho <- b.String()
	}()
	fn()
	w.Close()
	*destino = original
	return <-hecho
}

// correr ejecuta Run con los argumentos dados y devuelve el código de
// salida, lo que salió por stdout y lo que salió por stderr.
func correr(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var codigo int
	var salida string
	errSalida := captura(t, &os.Stderr, func() {
		salida = captura(t, &os.Stdout, func() {
			codigo = Run(args)
		})
	})
	return codigo, salida, errSalida
}

// escribirArchivo crea un archivo con el contenido dado, creando la carpeta
// que falta.
func escribirArchivo(t *testing.T, path, contenido string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
		t.Fatalf("escribir %s: %v", path, err)
	}
}

// escribirItem crea un item de workshop (carpeta numérica con project.json).
func escribirItem(t *testing.T, root, id, project string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	escribirArchivo(t, filepath.Join(dir, "project.json"), project)
}

// raizWorkshop arma una raíz de workshop falsa y devuelve su ruta.
func raizWorkshop(t *testing.T, items map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for id, project := range items {
		escribirItem(t, root, id, project)
	}
	return root
}

// homeConWorkshop arma un HOME falso con el workshop adentro, para que
// we.DefaultRoot lo encuentre. Devuelve la raíz del workshop.
func homeConWorkshop(t *testing.T, items map[string]string) string {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".steam", "steam", "steamapps", "workshop", "content", "431960")
	for id, project := range items {
		escribirItem(t, root, id, project)
	}
	t.Setenv("HOME", home)
	return root
}

// stubEnPATH escribe un ejecutable en un directorio temporal y lo pone al
// principio del PATH. Devuelve el directorio del stub.
func stubEnPATH(t *testing.T, nombre, cuerpo string) string {
	t.Helper()
	dir := t.TempDir()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatalf("stub %s: %v", nombre, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// pathSinBinarios deja el PATH sin ningún programa del sistema.
func pathSinBinarios(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("B3L0W_FFMPEG_DIR", t.TempDir())
}

// escribirPNG crea un PNG válido en disco (lo necesita el motor).
func escribirPNG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("crear %s: %v", path, err)
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("codificar %s: %v", path, err)
	}
}
