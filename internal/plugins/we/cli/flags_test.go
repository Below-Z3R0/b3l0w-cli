package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// Los ocho comandos tienen que rechazar un flag sin valor en vez de tomarlo
// como si fuera el valor siguiente (o como si no estuviera).

func TestFlagsSinValorFallanEnTodosLosComandos(t *testing.T) {
	casos := []struct {
		comando string
		args    []string
	}{
		{"list", []string{"list", "--root"}},
		{"list", []string{"list", "--type"}},
		{"list", []string{"list", "--query"}},
		{"deps", []string{"deps", "--root"}},
		{"convert", []string{"convert", "--out"}},
		{"convert", []string{"convert", "--root"}},
		{"convert", []string{"convert", "--id"}},
		{"convert", []string{"convert", "--method"}},
		{"convert", []string{"convert", "--format"}},
		{"convert", []string{"convert", "--crf"}},
		{"convert", []string{"convert", "--jobs"}},
		{"convert", []string{"convert", "--manifest"}},
		{"poster", []string{"poster", "--out"}},
		{"poster", []string{"poster", "--root"}},
		{"poster", []string{"poster", "--id"}},
		{"poster", []string{"poster", "--size"}},
		{"poster", []string{"poster", "--jobs"}},
		{"poster", []string{"poster", "--manifest"}},
		{"still", []string{"still", "--file"}},
		{"still", []string{"still", "--out"}},
		{"still", []string{"still", "--size"}},
		{"still", []string{"still", "--at"}},
		{"solid", []string{"solid", "--color"}},
		{"solid", []string{"solid", "--out"}},
		{"solid", []string{"solid", "--size"}},
		{"scan", []string{"scan", "--dir"}},
		{"scan", []string{"scan", "--kind"}},
		{"scan", []string{"scan", "--max"}},
		{"scan", []string{"scan", "--depth"}},
		{"scan", []string{"scan", "--sort"}},
		{"scan", []string{"scan", "--thumbs"}},
		{"scan", []string{"scan", "--thumb-size"}},
	}
	for _, caso := range casos {
		codigo, _, errSalida := correr(t, caso.args...)
		if codigo != 1 {
			t.Errorf("%s: código = %d, quería 1", caso.args[0], codigo)
			continue
		}
		flag := caso.args[len(caso.args)-1]
		if !strings.Contains(errSalida, flag) {
			t.Errorf("%s: stderr = %q, quería que nombre el flag %q", caso.args[0], errSalida, flag)
		}
		if !strings.Contains(errSalida, "necesita") {
			t.Errorf("%s: stderr = %q, quería que pida el valor", caso.args[0], errSalida)
		}
	}
}

func TestConvertRehaceElManifiestoIlegible(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()
	// Un manifiesto a medias no se ignora en silencio: se rehace.
	escribirArchivo(t, filepath.Join(out, "manifest.json"), "{ roto")

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(errSalida, "aviso") {
		t.Errorf("stderr = %q, quería el aviso del manifiesto ilegible", errSalida)
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("el manifiesto rehecho no es JSON: %v", err)
	}
	if len(m.Items) != 1 || m.Items[0].ID != "111" {
		t.Errorf("manifiesto = %+v, quería el item 111", m.Items)
	}
}

func TestConvertUsaElManifiestoIndicado(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()
	manifiesto := filepath.Join(out, "otro.json")

	codigo, salida, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0", "--manifest", manifiesto)
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

func TestScanExpandeElTildeEnLaCarpeta(t *testing.T) {
	home := t.TempDir()
	imagen := filepath.Join(home, "fotos", "fondo.png")
	escribirArchivo(t, imagen, "x")
	t.Setenv("HOME", home)

	codigo, salida, errSalida := correr(t, "scan", "--dir", "~/fotos")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, imagen) {
		t.Errorf("stdout = %q, quería que liste %q", salida, imagen)
	}
}

func TestScanIgnoraCarpetaInexistente(t *testing.T) {
	// Una carpeta que no existe no es un error: la lista sale vacía.
	codigo, salida, errSalida := correr(t, "scan", "--dir", filepath.Join(t.TempDir(), "no-existe"))
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if strings.TrimSpace(salida) != "" {
		t.Errorf("stdout = %q, quería una lista vacía", salida)
	}
}

func TestInfoIgnoraLasBanderasQueNoConoce(t *testing.T) {
	// `info` no tiene --root: el catálogo es siempre el del sistema.
	homeConWorkshop(t, map[string]string{"111": `{"title":"Alfa","type":"video"}`})
	codigo, _, errSalida := correr(t, "info", "111", "--root", "/tmp")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestListaMuestraCuantosDeCuantos(t *testing.T) {
	// El resumen dice "X de Y": X es lo que quedó del filtro, Y el total.
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"scene"}`,
		"333": `{"title":"Gamma","type":"video"}`,
	})
	_, _, errSalida := correr(t, "list", "--root", root, "--type", "video")
	if !strings.Contains(errSalida, "2 de 3 wallpapers") {
		t.Errorf("stderr = %q, quería \"2 de 3 wallpapers\"", errSalida)
	}
}
