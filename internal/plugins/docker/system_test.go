package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiskUsageParseaLasFilas(t *testing.T) {
	// Ojo: `docker system df --format '{{json .}}'` manda TotalCount y Active
	// como STRING. Si el parser los pidiera como int, la fila se descartaría.
	salida := strings.Join([]string{
		`{"Type":"Images","TotalCount":"12","Active":"3","Size":"4.5GB","Reclaimable":"1.2GB (27%)"}`,
		`{"Type":"Containers","TotalCount":"5","Active":"2","Size":"100MB","Reclaimable":"0B (0%)"}`,
	}, "\n")
	f := nuevoFake().
		con("system df --format {{json .}}", salida).
		con("system df", "TYPE            TOTAL   ACTIVE   SIZE      RECLAIMABLE\nImages              12        3   4.5GB     1.2GB (27%)\nContainers           5        2   100MB        0B (0%)")
	filas, total, err := DiskUsage(context.Background(), f.run)
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("esperaba 2 filas, obtuve %d: %+v", len(filas), filas)
	}
	if filas[0].Type != "Images" || filas[0].Total != 12 || filas[0].Active != 3 {
		t.Errorf("fila mal: %+v", filas[0])
	}
	if filas[0].Reclaimable != "1.2GB" {
		t.Errorf("Reclaimable = %q, quería sin el porcentaje", filas[0].Reclaimable)
	}
	if filas[1].Size != "100MB" {
		t.Errorf("Size = %q", filas[1].Size)
	}
	// El total es la última línea del `docker system df` en texto.
	if !strings.Contains(total, "Containers") {
		t.Errorf("total = %q, quería la última línea", total)
	}
}

func TestDiskUsageSaltaLasFilasRotas(t *testing.T) {
	f := nuevoFake().
		con("system df --format {{json .}}", "no es json\n{\"Type\":\"Images\",\"TotalCount\":\"12\"}").
		con("system df", "algo")
	filas, _, err := DiskUsage(context.Background(), f.run)
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	if len(filas) != 1 || filas[0].Total != 12 {
		t.Fatalf("filas = %+v", filas)
	}
}

func TestDiskUsagePropagaElError(t *testing.T) {
	f := nuevoFake().falla("system df --format {{json .}}", "", errors.New("daemon caído"))
	if _, _, err := DiskUsage(context.Background(), f.run); err == nil {
		t.Fatal("el error debería volver")
	}
}

func TestLimpiarReclaimable(t *testing.T) {
	casos := map[string]string{
		"1.2GB (27%)": "1.2GB",
		"0B (0%)":     "0B",
		"4.5GB":       "4.5GB",
		"":            "",
	}
	for valor, quiere := range casos {
		if got := limpiarReclaimable(valor); got != quiere {
			t.Errorf("limpiarReclaimable(%q) = %q, quería %q", valor, got, quiere)
		}
	}
}

func TestUltimaLinea(t *testing.T) {
	if got := ultimaLinea("uno\ndos\ntres"); got != "tres" {
		t.Errorf("ultimaLinea = %q, quería tres", got)
	}
	if got := ultimaLinea("  uno  \n  dos  "); got != "dos" {
		t.Errorf("ultimaLinea = %q, quería dos", got)
	}
	if got := ultimaLinea(""); got != "" {
		t.Errorf("ultimaLinea vacío = %q", got)
	}
}

func TestPruneTraduceElDestino(t *testing.T) {
	casos := map[string]string{
		"containers": "container prune -f",
		"images":     "image prune -f",
		"volumes":    "volume prune -f",
		"builder":    "builder prune -f",
		"system":     "system prune -f",
	}
	for destino, quiere := range casos {
		f := nuevoFake().con(quiere, "borrado")
		res, err := Prune(context.Background(), f.run, destino)
		if err != nil {
			t.Errorf("%s: %v", destino, err)
			continue
		}
		if res.Command != "docker "+quiere {
			t.Errorf("%s: Command = %q, quería %q", destino, res.Command, "docker "+quiere)
		}
	}
}

func TestPruneRechazaDestinosDesconocidos(t *testing.T) {
	f := nuevoFake()
	res, err := Prune(context.Background(), f.run, "todo")
	if err == nil {
		t.Fatal("un destino desconocido debería fallar")
	}
	if res.OK {
		t.Error("no debería estar OK")
	}
	if !strings.Contains(err.Error(), "prune inválido") {
		t.Errorf("el error debería explicar los destinos válidos: %v", err)
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestPruneDetailedArmaCadaDestino(t *testing.T) {
	casos := []struct {
		destino string
		todas   bool
		quiere  string
	}{
		{"containers", false, "prune --filter until=24h --force"},
		{"images", false, "prune --filter until=24h -a --force"},
		{"volumes", false, "prune --volumes --force"},
		{"networks", false, "prune --filter until=24h --force"},
		{"builder", false, "prune --filter until=24h --force"},
		{"system", false, "prune --force"},
		{"system", true, "prune -a --volumes --force"},
	}
	for _, c := range casos {
		f := nuevoFake().con(c.quiere, "ok")
		res, err := PruneDetailed(context.Background(), f.run, c.destino, c.todas)
		if err != nil {
			t.Errorf("%s: %v", c.destino, err)
			continue
		}
		if res.Command != "docker "+c.quiere {
			t.Errorf("%s: Command = %q, quería %q", c.destino, res.Command, "docker "+c.quiere)
		}
	}
}

func TestPruneDetailedRechazaDestinosDesconocidos(t *testing.T) {
	f := nuevoFake()
	res, err := PruneDetailed(context.Background(), f.run, "inventado", false)
	if err == nil {
		t.Fatal("un destino desconocido debería fallar")
	}
	if res.OK {
		t.Error("no debería estar OK")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}
