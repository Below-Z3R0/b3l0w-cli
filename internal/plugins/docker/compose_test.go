package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGroupProjectsAgrupaYOrdena(t *testing.T) {
	contenedores := []Container{
		{ID: "1", Name: "web", Project: "shop", Service: "web", State: "running"},
		{ID: "2", Name: "db", Project: "shop", Service: "db", State: "running", Health: "unhealthy"},
		{ID: "3", Name: "cache", Project: "shop", Service: "cache", State: "exited"},
		{ID: "4", Name: "api", Project: "otro", Service: "api", State: "running"},
		{ID: "5", Name: "suelto", State: "running"},
	}
	proyectos := GroupProjects(contenedores)
	if len(proyectos) != 2 {
		t.Fatalf("esperaba 2 proyectos, obtuve %d: %+v", len(proyectos), proyectos)
	}
	// Ordenados por nombre.
	if proyectos[0].Name != "otro" || proyectos[1].Name != "shop" {
		t.Errorf("orden = %s, %s", proyectos[0].Name, proyectos[1].Name)
	}
	shop := proyectos[1]
	if shop.Containers != 3 || shop.Running != 2 || shop.Stopped != 1 {
		t.Errorf("conteos mal: %+v", shop)
	}
	if shop.State != "partial" {
		t.Errorf("State = %q, quería partial", shop.State)
	}
	if shop.Unhealthy != 1 {
		t.Errorf("Unhealthy = %d, quería 1", shop.Unhealthy)
	}
	if strings.Join(shop.Services, ",") != "cache,db,web" {
		t.Errorf("Services = %v, quería ordenados", shop.Services)
	}
	// El contenedor sin proyecto no arma ninguno.
	for _, p := range proyectos {
		if p.Name == "" {
			t.Error("un contenedor sin proyecto no debería crear un proyecto")
		}
	}
}

func TestGroupProjectsEstados(t *testing.T) {
	casos := []struct {
		nombre  string
		estados []string
		quiere  string
	}{
		{"todos corriendo", []string{"running", "running"}, "running"},
		{"mixto", []string{"running", "exited"}, "partial"},
		{"todos parados", []string{"exited", "created"}, "stopped"},
	}
	for _, c := range casos {
		var contenedores []Container
		for i, e := range c.estados {
			contenedores = append(contenedores, Container{ID: string(rune('a' + i)), Project: "p", State: e})
		}
		proyectos := GroupProjects(contenedores)
		if len(proyectos) != 1 {
			t.Fatalf("%s: esperaba 1 proyecto", c.nombre)
		}
		if proyectos[0].State != c.quiere {
			t.Errorf("%s: State = %q, quería %q", c.nombre, proyectos[0].State, c.quiere)
		}
	}
}

func TestGroupProjectsSinContenedores(t *testing.T) {
	if proyectos := GroupProjects(nil); len(proyectos) != 0 {
		t.Errorf("esperaba 0, obtuve %d", len(proyectos))
	}
}

func TestComposeActionAgregaDetachedAlUp(t *testing.T) {
	f := nuevoFake().con("compose -p shop up -d", "contenedor creado")
	res, err := ComposeAction(context.Background(), f.run, "shop", "up")
	if err != nil {
		t.Fatalf("ComposeAction: %v", err)
	}
	if !res.OK {
		t.Error("debería estar OK")
	}
	if res.Command != "docker compose -p shop up -d" {
		t.Errorf("Command = %q", res.Command)
	}
}

func TestComposeActionSinDetachedEnOtherAcciones(t *testing.T) {
	for _, accion := range []string{"down", "restart", "stop", "pull"} {
		f := nuevoFake().con("compose -p shop "+accion, "ok")
		res, err := ComposeAction(context.Background(), f.run, "shop", accion)
		if err != nil {
			t.Fatalf("%s: %v", accion, err)
		}
		if res.Command != "docker compose -p shop "+accion {
			t.Errorf("%s: Command = %q", accion, res.Command)
		}
	}
}

func TestComposeActionCortaLaSalidaLarga(t *testing.T) {
	f := nuevoFake().con("compose -p shop pull", strings.Repeat("x", 900))
	res, err := ComposeAction(context.Background(), f.run, "shop", "pull")
	if err != nil {
		t.Fatalf("ComposeAction: %v", err)
	}
	if len(res.Output) != 400 {
		t.Errorf("la salida debería quedar en 400 caracteres, quedó en %d", len(res.Output))
	}
}

func TestComposeActionPropagaElError(t *testing.T) {
	f := novoFakeCompose()
	res, err := ComposeAction(context.Background(), f.run, "shop", "up")
	if err == nil {
		t.Fatal("el error debería volver")
	}
	if res.OK {
		t.Error("no debería estar OK")
	}
}

func novoFakeCompose() *fakeRunner {
	return nuevoFake().falla("compose -p shop up -d", "no such service", errors.New("no such service"))
}

func TestEventsParseaElJSON(t *testing.T) {
	salida := strings.Join([]string{
		`{"Type":"container","Action":"start","time":1700000000,"Actor":{"ID":"abc123def456789","Attributes":{"name":"web","image":"nginx:latest"}}}`,
		`{"Type":"container","Action":"die","time":1700000100,"Actor":{"ID":"def456abc123789","Attributes":{"name":"viejo","exitCode":"1"}}}`,
	}, "\n")
	f := nuevoFake().con("events --since 6h --until 0s --filter type=container --format {{json .}}", salida)
	eventos, err := Events(context.Background(), f.run, "")
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(eventos) != 2 {
		t.Fatalf("esperaba 2 eventos, obtuve %d", len(eventos))
	}
	// Lo más nuevo primero.
	if eventos[0].Action != "die" || eventos[1].Action != "start" {
		t.Errorf("orden = %s, %s (quería die, start)", eventos[0].Action, eventos[1].Action)
	}
	if eventos[1].Name != "web" || eventos[1].Image != "nginx:latest" {
		t.Errorf("atributos mal: %+v", eventos[1])
	}
	if eventos[1].ID != "abc123def456" {
		t.Errorf("ID = %q, quería el corto", eventos[1].ID)
	}
	if eventos[0].Exit != "1" {
		t.Errorf("Exit = %q, quería 1", eventos[0].Exit)
	}
}

func TestEventsUsaLaVentanaPorDefecto(t *testing.T) {
	f := nuevoFake().con("events --since 6h --until 0s --filter type=container --format {{json .}}", "")
	if _, err := Events(context.Background(), f.run, ""); err != nil {
		t.Fatalf("Events: %v", err)
	}
	if !f.llamado("events --since 6h --until 0s --filter type=container --format {{json .}}") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestEventsRespetaLaVentanaPedida(t *testing.T) {
	f := nuevoFake().con("events --since 30m --until 0s --filter type=container --format {{json .}}", "")
	if _, err := Events(context.Background(), f.run, "30m"); err != nil {
		t.Fatalf("Events: %v", err)
	}
	if !f.llamado("events --since 30m --until 0s --filter type=container --format {{json .}}") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestEventsSaltaLineasBasura(t *testing.T) {
	salida := strings.Join([]string{
		"",
		"no es json",
		`{"Type":"container","Action":`,
		`{"Type":"container"}`,
		`{"Type":"container","Action":"start","Actor":{"Attributes":{"name":"web"}}}`,
	}, "\n")
	f := nuevoFake().con("events --since 6h --until 0s --filter type=container --format {{json .}}", salida)
	eventos, err := Events(context.Background(), f.run, "")
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(eventos) != 1 || eventos[0].Action != "start" {
		t.Fatalf("solo debía quedar el evento válido: %+v", eventos)
	}
}

func TestEventsPropagaElError(t *testing.T) {
	f := nuevoFake().falla("events --since 6h --until 0s --filter type=container --format {{json .}}", "", errors.New("daemon caído"))
	if _, err := Events(context.Background(), f.run, ""); err == nil {
		t.Fatal("el error debería volver")
	}
}
