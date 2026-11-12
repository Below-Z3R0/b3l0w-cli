package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Una fila real de `docker stats --no-stream --format '{{json .}}'`.
const statFila = `{"ID":"abc123def456","Name":"web","CPUPerc":"12.34%","MemPerc":"5.67%","MemUsage":"12.5MiB / 15.6GiB","NetIO":"1.2kB / 3.4kB","BlockIO":"0B / 0B","PIDs":"7"}`

func TestStatsParseaLaFila(t *testing.T) {
	f := nuevoFake().con("stats --no-stream --format {{json .}}", statFila+"\n")
	lista, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("esperaba 1, obtuve %d", len(lista))
	}
	s := lista[0]
	if s.ID != "abc123def456" || s.Name != "web" {
		t.Errorf("fila mal: %+v", s)
	}
	if s.CPUPct != 12.34 {
		t.Errorf("CPUPct = %v, quería 12.34", s.CPUPct)
	}
	if s.MemPct != 5.67 {
		t.Errorf("MemPct = %v, quería 5.67", s.MemPct)
	}
	if s.MemUsage != "12.5MiB / 15.6GiB" {
		t.Errorf("MemUsage = %q", s.MemUsage)
	}
	if s.PIDs != 7 {
		t.Errorf("PIDs = %d, quería 7", s.PIDs)
	}
}

func TestStatsVariosContenedores(t *testing.T) {
	f := nuevoFake().con("stats --no-stream --format {{json .}}", strings.Join([]string{
		`{"ID":"a1","Name":"web","CPUPerc":"1.00%","MemPerc":"2.00%","PIDs":"3"}`,
		`{"ID":"b2","Name":"db","CPUPerc":"50.50%","MemPerc":"99.99%","PIDs":"120"}`,
	}, "\n")+"\n")
	lista, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(lista))
	}
	if lista[1].Name != "db" || lista[1].CPUPct != 50.5 || lista[1].PIDs != 120 {
		t.Errorf("segunda fila mal: %+v", lista[1])
	}
}

func TestStatsPorcentajeIrlegibleEsCero(t *testing.T) {
	// Un contenedor recién arrancado puede venir con "--" en vez de número.
	f := nuevoFake().con("stats --no-stream --format {{json .}}",
		`{"ID":"a1","Name":"nuevo","CPUPerc":"--","MemPerc":"","PIDs":"--"}`+"\n")
	lista, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("esperaba 1, obtuve %d", len(lista))
	}
	if lista[0].CPUPct != 0 || lista[0].MemPct != 0 || lista[0].PIDs != 0 {
		t.Errorf("un valor ilegible debería quedar en 0: %+v", lista[0])
	}
}

func TestStatsSaltaLineasBasura(t *testing.T) {
	f := nuevoFake().con("stats --no-stream --format {{json .}}", strings.Join([]string{
		"",
		"no es json",
		`{"ID":"roto"`,
		statFila,
	}, "\n")+"\n")
	lista, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(lista) != 1 || lista[0].Name != "web" {
		t.Fatalf("solo debía quedar la fila válida: %+v", lista)
	}
}

func TestStatsSinContenedores(t *testing.T) {
	f := nuevoFake().con("stats --no-stream --format {{json .}}", "")
	lista, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("esperaba 0, obtuve %d", len(lista))
	}
}

func TestStatsPropagaElError(t *testing.T) {
	f := nuevoFake().falla("stats --no-stream --format {{json .}}", "", errors.New("daemon caído"))
	if _, err := Stats(context.Background(), f.run); err == nil {
		t.Fatal("el error debería volver")
	}
}

func TestPercent(t *testing.T) {
	casos := map[string]float64{
		"12.34%": 12.34,
		"0.00%":  0,
		"100%":   100,
		"  7.5%": 7.5,
		"--":     0,
		"":       0,
		"n/a":    0,
	}
	for valor, quiere := range casos {
		if got := percent(valor); got != quiere {
			t.Errorf("percent(%q) = %v, quería %v", valor, got, quiere)
		}
	}
}
