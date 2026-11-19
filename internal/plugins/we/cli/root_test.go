package wecli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func TestRunSinArgsMuestraElUso(t *testing.T) {
	codigo, salida, _ := correr(t)
	if codigo != 0 {
		t.Errorf("código = %d, quería 0", codigo)
	}
	if !strings.Contains(salida, "uso: b3l0w we") {
		t.Errorf("salida = %q, quería el texto de uso", salida)
	}
}

func TestRunAyudaSaleConCero(t *testing.T) {
	for _, flag := range []string{"help", "-h", "--help"} {
		codigo, salida, _ := correr(t, flag)
		if codigo != 0 {
			t.Errorf("%s: código = %d, quería 0", flag, codigo)
		}
		if !strings.Contains(salida, "uso: b3l0w we") {
			t.Errorf("%s: salida = %q, quería el uso", flag, salida)
		}
	}
}

func TestRunComandoDesconocidoFalla(t *testing.T) {
	codigo, salida, errSalida := correr(t, "inventado")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "comando desconocido") {
		t.Errorf("stderr = %q, quería que nombre el comando", errSalida)
	}
	if !strings.Contains(salida, "uso: b3l0w we") {
		t.Errorf("stdout = %q, quería el uso para que se sepa qué hay", salida)
	}
}

func TestRunAyudaDeCadaComando(t *testing.T) {
	// Cada comando tiene que documentarse solo: es lo que el usuario ve
	// cuando se equivoca de bandera.
	for _, cmd := range []string{"list", "info", "convert", "poster", "still", "scan", "solid", "deps"} {
		codigo, salida, _ := correr(t, cmd, "--help")
		if codigo != 0 {
			t.Errorf("%s --help: código = %d, quería 0", cmd, codigo)
		}
		if !strings.Contains(salida, "uso: b3l0w we "+cmd) {
			t.Errorf("%s --help: salida = %q, quería el uso del comando", cmd, salida)
		}
	}
}

func TestHumanBytesEscalaLasUnidades(t *testing.T) {
	casos := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1023:       "1023 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1048576:    "1.0 MB",
		1073741824: "1.0 GB",
	}
	for n, want := range casos {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, quería %q", n, got, want)
		}
	}
}

func TestPickSinIdNiAllFalla(t *testing.T) {
	items := []we.Item{{ID: "1", Title: "Uno"}}
	_, err := pick(items, nil, false)
	if err == nil {
		t.Fatal("pick sin --id ni --all debería fallar")
	}
	if !strings.Contains(err.Error(), "--id") {
		t.Errorf("error = %v, quería que diga qué falta", err)
	}
}

func TestPickConIdInexistenteFalla(t *testing.T) {
	items := []we.Item{{ID: "1", Title: "Uno"}}
	if _, err := pick(items, []string{"999"}, false); err == nil {
		t.Error("pick con un id que no está debería fallar")
	}
}

func TestPickConAllDevuelveTodo(t *testing.T) {
	items := []we.Item{{ID: "1"}, {ID: "2"}}
	got, err := pick(items, nil, true)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("pick --all = %d items, quería 2", len(got))
	}
}

func TestPickConIdEligeSoloEse(t *testing.T) {
	items := []we.Item{{ID: "1"}, {ID: "2"}}
	got, err := pick(items, []string{"2"}, false)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if len(got) != 1 || got[0].ID != "2" {
		t.Errorf("pick = %+v, quería el item 2", got)
	}
}

func TestCatalogDevuelveLosItems(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video"}`,
	})
	items, abs, err := catalog(root)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if abs != root {
		t.Errorf("abs = %q, quería %q", abs, root)
	}
	if len(items) != 1 || items[0].ID != "111" {
		t.Errorf("items = %+v, quería el item 111", items)
	}
}

func TestCatalogVacioFalla(t *testing.T) {
	_, _, err := catalog(t.TempDir())
	if !errors.Is(err, we.ErrEmpty) {
		t.Errorf("error = %v, quería ErrEmpty", err)
	}
}

func TestCatalogInexistenteFalla(t *testing.T) {
	_, _, err := catalog(filepath.Join(t.TempDir(), "no-existe"))
	if err == nil {
		t.Error("catalog de una ruta inexistente debería fallar")
	}
}

func TestCatalogSinRootBuscaElWorkshopPorDefecto(t *testing.T) {
	// Sin HOME con workshop, el comando dice qué probó y dónde.
	t.Setenv("HOME", t.TempDir())
	_, _, err := catalog("")
	if err == nil {
		t.Fatal("catalog sin workshop debería fallar")
	}
	if !strings.Contains(err.Error(), "no encontré el workshop") {
		t.Errorf("error = %v, quería que explique que no encontró el workshop", err)
	}
}
