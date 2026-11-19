package wecli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func TestListaMuestraLaTabla(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
		"222": `{"title":"Beta","type":"scene"}`,
	})
	codigo, salida, errSalida := correr(t, "list", "--root", root)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	for _, want := range []string{"ID", "TIPO", "TAMAÑO", "TÍTULO", "111", "Alfa", "222", "Beta"} {
		if !strings.Contains(salida, want) {
			t.Errorf("stdout = %q, quería que contenga %q", salida, want)
		}
	}
	if !strings.Contains(errSalida, "2 de 2 wallpapers") {
		t.Errorf("stderr = %q, quería el conteo de wallpapers", errSalida)
	}
}

func TestListaEnJSONEmiteElCatalogoCompleto(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	codigo, salida, errSalida := correr(t, "list", "--root", root, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var items []we.Item
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 || items[0].ID != "111" || items[0].Type != "video" {
		t.Errorf("items = %+v, quería el item 111 de tipo video", items)
	}
}

func TestListaFiltraPorTipo(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"scene"}`,
	})
	codigo, salida, _ := correr(t, "list", "--root", root, "--type", "video")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "111") {
		t.Errorf("stdout = %q, quería el item 111", salida)
	}
	if strings.Contains(salida, "222") {
		t.Errorf("stdout = %q, no quería el item 222 (es scene)", salida)
	}
}

func TestListaFiltraPorConsulta(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"scene"}`,
	})
	codigo, salida, _ := correr(t, "list", "--root", root, "--query", "bet")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "222") || strings.Contains(salida, "111") {
		t.Errorf("stdout = %q, quería sólo el item 222", salida)
	}
}

func TestListaAceptaElValorConIgual(t *testing.T) {
	// "--root=DIR" es una forma válida: el parseo la tiene que aceptar.
	root := raizWorkshop(t, map[string]string{"111": `{"title":"Alfa","type":"video"}`})
	codigo, salida, errSalida := correr(t, "list", "--root="+root)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "111") {
		t.Errorf("stdout = %q, quería el item 111", salida)
	}
}

func TestListaAceptaElTipoConIgual(t *testing.T) {
	// "--type=video" también: el parseo acepta las dos formas para todos
	// los flags con valor.
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
		"222": `{"title":"Beta","type":"scene"}`,
	})
	codigo, salida, errSalida := correr(t, "list", "--root", root, "--type=video")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "111") || strings.Contains(salida, "222") {
		t.Errorf("stdout = %q, quería sólo el item 111", salida)
	}
}

func TestListaOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "list", "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestListaRootSinValorFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "list", "--root")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--root") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestListaCatalogoVacioFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "list", "--root", t.TempDir())
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "vacío") {
		t.Errorf("stderr = %q, quería que diga que el catálogo está vacío", errSalida)
	}
}

func TestListaMarcaLosItemsRotos(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"333": `{ roto`,
	})
	codigo, salida, _ := correr(t, "list", "--root", root)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "(roto)") {
		t.Errorf("stdout = %q, quería que marque el item roto", salida)
	}
}

func TestInfoMuestraElDetalle(t *testing.T) {
	homeConWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4","preview":"p.jpg"}`,
	})
	codigo, salida, errSalida := correr(t, "info", "111")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	for _, want := range []string{"id:", "111", "título:", "Alfa", "tipo:", "video", "tamaño:", "archivo:", "preview:"} {
		if !strings.Contains(salida, want) {
			t.Errorf("stdout = %q, quería que contenga %q", salida, want)
		}
	}
}

func TestInfoEnJSONEmiteElItem(t *testing.T) {
	homeConWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	codigo, salida, errSalida := correr(t, "info", "111", "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var item we.Item
	if err := json.Unmarshal([]byte(salida), &item); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if item.ID != "111" || item.Title != "Alfa" {
		t.Errorf("item = %+v, quería el item 111", item)
	}
}

func TestInfoSinIdFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "info")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "falta el id") {
		t.Errorf("stderr = %q, quería que pida el id", errSalida)
	}
}

func TestInfoOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "info", "111", "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestInfoConIdInexistenteFalla(t *testing.T) {
	homeConWorkshop(t, map[string]string{"111": `{"title":"Alfa","type":"video"}`})
	codigo, _, errSalida := correr(t, "info", "999")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no existe el wallpaper") {
		t.Errorf("stderr = %q, quería que diga que no existe", errSalida)
	}
}
