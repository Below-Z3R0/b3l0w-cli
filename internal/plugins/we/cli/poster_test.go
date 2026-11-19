package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// motorFalso pone un linux-wallpaperengine de mentira en el PATH. El motor
// real levanta un render por wallpaper y necesita GPU: para probar el CLI
// alcanza con que escriba el PNG que le piden.
func motorFalso(t *testing.T, cuerpo string) string {
	t.Helper()
	dir := stubEnPATH(t, "linux-wallpaperengine", "#!/bin/sh\n"+cuerpo+"\n")
	return dir
}

func TestPosterGeneraElPNGConElMotor(t *testing.T) {
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

	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4","preview":"p.jpg"}`,
	})
	out := t.TempDir()
	log := filepath.Join(t.TempDir(), "motor.log")
	t.Setenv("WE_TEST_LOG", log)

	codigo, salida, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	manifiesto := strings.TrimSpace(salida)
	if filepath.Base(manifiesto) != "manifest.json" {
		t.Errorf("stdout = %q, quería la ruta del manifiesto", salida)
	}
	poster := filepath.Join(out, "111.png")
	if _, err := os.Stat(poster); err != nil {
		t.Fatalf("no escribió %s: %v", poster, err)
	}
	// El motor recibe la geometría pedida: con otro separador la rechaza.
	datos, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(datos), "--bg 111") {
		t.Errorf("args del motor = %q, quería --bg 111", datos)
	}
	if !strings.Contains(string(datos), "--window 0x0x1280x720") {
		t.Errorf("args del motor = %q, quería --window 0x0x1280x720", datos)
	}

	var m we.Manifest
	datosManifiesto, err := os.ReadFile(manifiesto)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datosManifiesto, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != 1 || m.Items[0].ID != "111" || m.Items[0].Status != "ok" {
		t.Errorf("manifiesto = %+v, quería el item 111 con status ok", m.Items)
	}
}

func TestPosterFallaSiElMotorNoEscribeElPNG(t *testing.T) {
	motorFalso(t, `exit 1`)
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--id", "111")
	if codigo == 0 {
		t.Error("con el motor caído el comando debería fallar")
	}
	if !strings.Contains(errSalida, "poster") {
		t.Errorf("stderr = %q, quería que explique el fallo", errSalida)
	}
}

func TestPosterFallaSiTodosLosWallpapersFallan(t *testing.T) {
	motorFalso(t, `exit 1`)
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"video"}`,
	})
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "poster", "--out", out, "--root", root, "--all")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "poster") {
		t.Errorf("stderr = %q, quería que explique el fallo", errSalida)
	}
}

func TestPosterSinOutFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "poster", "--root", t.TempDir())
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--out") {
		t.Errorf("stderr = %q, quería que pida --out", errSalida)
	}
}

func TestPosterSizeInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "poster", "--out", t.TempDir(), "--size", "grande")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--size") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestPosterJobsInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "poster", "--out", t.TempDir(), "--jobs", "0")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--jobs") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestPosterSinIdNiAllFalla(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	codigo, _, errSalida := correr(t, "poster", "--out", t.TempDir(), "--root", root)
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--id") {
		t.Errorf("stderr = %q, quería que pida --id o --all", errSalida)
	}
}

func TestPosterOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "poster", "--out", t.TempDir(), "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}
