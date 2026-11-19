package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// stubTool escribe un binario falso en el PATH y devuelve su directorio.
func stubTool(t *testing.T, nombre string) {
	t.Helper()
	stubEnPATH(t, nombre, "#!/bin/sh\necho \"$@\"\nexit 0\n")
}

func TestDepsMuestraLaTabla(t *testing.T) {
	// Sin ffmpeg ni ffprobe: el comando reporta lo que falta y sale con 2.
	pathSinBinarios(t)
	t.Setenv("HOME", t.TempDir())

	codigo, salida, errSalida := correr(t, "deps")
	if codigo != 2 {
		t.Errorf("código = %d, quería 2 (falta ffmpeg y workshop)", codigo)
	}
	for _, want := range []string{"ffmpeg", "ffprobe", "engine", "workshop", "record"} {
		if !strings.Contains(salida, want) {
			t.Errorf("stdout = %q, quería la fila %q", salida, want)
		}
	}
	if !strings.Contains(salida, "FALTA") {
		t.Errorf("stdout = %q, quería que marque lo que falta", salida)
	}
	if errSalida != "" {
		t.Errorf("stderr = %q, la tabla va por stdout", errSalida)
	}
}

func TestDepsEnJSONEmiteElReporteCompleto(t *testing.T) {
	pathSinBinarios(t)
	t.Setenv("HOME", t.TempDir())

	codigo, salida, errSalida := correr(t, "deps", "--json")
	if codigo != 2 {
		t.Errorf("código = %d, quería 2", codigo)
	}
	var deps we.Deps
	if err := json.Unmarshal([]byte(salida), &deps); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if deps.Ffmpeg.Ok {
		t.Error("ffmpeg no está en el PATH, no debería dar por bueno")
	}
	if deps.Ffprobe.Ok {
		t.Error("ffprobe no está en el PATH, no debería dar por bueno")
	}
	if deps.Workshop.Ok {
		t.Error("no hay workshop en el HOME de prueba, no debería dar por bueno")
	}
	if errSalida != "" {
		t.Errorf("stderr = %q, el JSON va por stdout", errSalida)
	}
}

func TestDepsSaleConCeroSiEstanLoImprescindible(t *testing.T) {
	stubTool(t, "ffmpeg")
	stubTool(t, "ffprobe")
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	t.Setenv("HOME", t.TempDir())

	codigo, salida, errSalida := correr(t, "deps", "--root", root)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "ok") {
		t.Errorf("stdout = %q, quería que marque lo que está", salida)
	}
	if !strings.Contains(salida, "1 items") {
		t.Errorf("stdout = %q, quería el conteo de items del workshop", salida)
	}
}

func TestDepsEnJSONCuentaLosItemsDelWorkshop(t *testing.T) {
	stubTool(t, "ffmpeg")
	stubTool(t, "ffprobe")
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"video"}`,
	})
	t.Setenv("HOME", t.TempDir())

	codigo, salida, errSalida := correr(t, "deps", "--root", root, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var deps we.Deps
	if err := json.Unmarshal([]byte(salida), &deps); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if !deps.Ffmpeg.Ok || !deps.Workshop.Ok {
		t.Errorf("deps = %+v, quería ffmpeg y workshop ok", deps)
	}
	if deps.Workshop.Items != 2 {
		t.Errorf("items = %d, quería 2", deps.Workshop.Items)
	}
}

func TestDepsConWorkshopInvalidoSaleConDos(t *testing.T) {
	stubTool(t, "ffmpeg")
	stubTool(t, "ffprobe")
	t.Setenv("HOME", t.TempDir())

	codigo, _, _ := correr(t, "deps", "--root", filepath.Join(t.TempDir(), "no-existe"))
	if codigo != 2 {
		t.Errorf("código = %d, quería 2 (falta el workshop)", codigo)
	}
}

func TestDepsOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "deps", "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestDepsAceptaElRootConIgual(t *testing.T) {
	stubTool(t, "ffmpeg")
	stubTool(t, "ffprobe")
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	t.Setenv("HOME", t.TempDir())

	codigo, salida, errSalida := correr(t, "deps", "--root="+root, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var deps we.Deps
	if err := json.Unmarshal([]byte(salida), &deps); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if deps.Workshop.Items != 1 {
		t.Errorf("items = %d, quería 1", deps.Workshop.Items)
	}
}

func TestDepsSinRootBuscaElWorkshopPorDefecto(t *testing.T) {
	stubTool(t, "ffmpeg")
	stubTool(t, "ffprobe")
	homeConWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})

	codigo, salida, errSalida := correr(t, "deps")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "1 items") {
		t.Errorf("stdout = %q, quería el conteo del workshop del HOME", salida)
	}
}

func TestDepsNoEscribeNadaEnDisco(t *testing.T) {
	pathSinBinarios(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	antes, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	correr(t, "deps")
	despues, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(antes) != len(despues) {
		t.Errorf("deps escribió %d archivos de más", len(despues)-len(antes))
	}
}
