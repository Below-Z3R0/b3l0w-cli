package wecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Las formas del parseo y los flags que cambian lo que hace el comando.

func TestStillAceptaLosValoresConIgual(t *testing.T) {
	stubFFmpeg(t, `
echo "$@" >> "$WE_TEST_LOG"
for a in "$@"; do out="$a"; done
printf 'jpeg' > "$out"
`)
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	escribirArchivo(t, video, "video falso")
	out := filepath.Join(dir, "cuadro.jpg")

	codigo, salida, errSalida := correr(t, "still", "--file="+video, "--out="+out)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, out) {
		t.Errorf("stdout = %q, quería la ruta de la imagen", salida)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("no escribió la imagen: %v", err)
	}
}

func TestListaAceptaLaConsultaConIgual(t *testing.T) {
	// "--query=bet" es una forma válida: el parseo la tiene que aceptar.
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"scene"}`,
	})
	codigo, salida, errSalida := correr(t, "list", "--root="+root, "--query=bet")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "222") || strings.Contains(salida, "111") {
		t.Errorf("stdout = %q, quería sólo el item 222", salida)
	}
}

func TestPosterAceptaLosValoresConIgual(t *testing.T) {
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

	codigo, _, errSalida := correr(t, "poster", "--out="+out, "--root="+root, "--id=111")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if _, err := os.Stat(filepath.Join(out, "111.png")); err != nil {
		t.Errorf("no escribió el poster: %v", err)
	}
}

func TestPosterUsaElManifiestoIndicado(t *testing.T) {
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
	manifiesto := filepath.Join(out, "otro.json")

	codigo, salida, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111", "--manifest", manifiesto)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, manifiesto) {
		t.Errorf("stdout = %q, quería la ruta del manifiesto pedido", salida)
	}
	if _, err := os.Stat(manifiesto); err != nil {
		t.Errorf("no escribió el manifiesto en la ruta pedida: %v", err)
	}
}

func TestPosterNoRepiteElRenderSiElPNGYaExiste(t *testing.T) {
	origen := t.TempDir()
	fixture := filepath.Join(origen, "base.png")
	escribirPNG(t, fixture)
	t.Setenv("WE_TEST_PNG", fixture)
	motorFalso(t, `
echo "$@" >> "$WE_TEST_LOG"
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
	log := filepath.Join(t.TempDir(), "motor.log")
	t.Setenv("WE_TEST_LOG", log)

	if codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111"); codigo != 0 {
		t.Fatalf("primera corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111")
	if codigo != 0 {
		t.Fatalf("segunda corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	if llamadas := len(lineasDelLog(t, log)); llamadas != 1 {
		t.Errorf("el engine corrió %d veces, quería 1: el PNG ya estaba", llamadas)
	}
}

func TestPosterRepiteElRenderConOverwrite(t *testing.T) {
	origen := t.TempDir()
	fixture := filepath.Join(origen, "base.png")
	escribirPNG(t, fixture)
	t.Setenv("WE_TEST_PNG", fixture)
	motorFalso(t, `
echo "$@" >> "$WE_TEST_LOG"
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
	log := filepath.Join(t.TempDir(), "motor.log")
	t.Setenv("WE_TEST_LOG", log)

	if codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111"); codigo != 0 {
		t.Fatalf("primera corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111", "--overwrite")
	if codigo != 0 {
		t.Fatalf("segunda corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	if llamadas := len(lineasDelLog(t, log)); llamadas != 2 {
		t.Errorf("el engine corrió %d veces, quería 2: con --overwrite se rehace", llamadas)
	}
}

// lineasDelLog devuelve las líneas no vacías del log del binario falso.
func lineasDelLog(t *testing.T, path string) []string {
	t.Helper()
	datos, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("leer el log: %v", err)
	}
	var salida []string
	for _, linea := range strings.Split(strings.TrimSpace(string(datos)), "\n") {
		if linea != "" {
			salida = append(salida, linea)
		}
	}
	return salida
}
