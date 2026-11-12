package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Una fila real de `docker ps --format '{{json .}}'`.
const filaPS = `{"ID":"abc123def456789","Names":"web","Image":"nginx:latest","State":"running","Status":"Up 3 hours (healthy)","Command":"nginx","Ports":"0.0.0.0:8080->80/tcp","CreatedAt":"2024-01-01 10:00:00 +0000 UTC","Labels":"com.docker.compose.project=shop,com.docker.compose.service=web"}`

func TestContainersParseaLaFila(t *testing.T) {
	run := nuevoFake().con("ps --no-trunc --format {{json .}}", filaPS+"\n")
	lista, err := Containers(context.Background(), run.run, false)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("esperaba 1 contenedor, obtuve %d", len(lista))
	}
	c := lista[0]
	if c.ID != "abc123def456" {
		t.Errorf("ID = %q, quería el id corto de 12", c.ID)
	}
	if c.Name != "web" || c.Image != "nginx:latest" || c.State != "running" {
		t.Errorf("fila mal: %+v", c)
	}
	if c.Health != "healthy" {
		t.Errorf("Health = %q, quería healthy (va en el Status)", c.Health)
	}
	if c.Uptime != "3 hours" {
		t.Errorf("Uptime = %q, quería '3 hours'", c.Uptime)
	}
	if c.Project != "shop" || c.Service != "web" {
		t.Errorf("labels de compose mal: %+v", c)
	}
	if !c.HasShell {
		t.Error("un contenedor corriendo debería ofrecer shell")
	}
	if len(c.Ports) != 1 || c.Ports[0].HostPort != 8080 {
		t.Errorf("puertos mal: %+v", c.Ports)
	}
}

func TestContainersContenedorParado(t *testing.T) {
	run := nuevoFake().con("ps -a --no-trunc --format {{json .}}",
		`{"ID":"deadbeefcafe","Names":"viejo","Image":"alpine","State":"exited","Status":"Exited (0) 2 hours ago","Ports":""}`+"\n")
	lista, err := Containers(context.Background(), run.run, true)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("esperaba 1, obtuve %d", len(lista))
	}
	c := lista[0]
	if c.State != "exited" {
		t.Errorf("State = %q, quería exited", c.State)
	}
	if c.Health != "" || c.Uptime != "" {
		t.Errorf("un contenedor parado no tiene salud ni uptime: %+v", c)
	}
	if c.HasShell {
		t.Error("un contenedor parado no debería ofrecer shell")
	}
	if c.Ports != nil {
		t.Errorf("sin puertos publicados la lista debería quedar vacía: %+v", c.Ports)
	}
}

func TestContainersOrdenaPorProyectoYNombre(t *testing.T) {
	run := nuevoFake().con("ps --no-trunc --format {{json .}}", strings.Join([]string{
		`{"ID":"c3","Names":"zebra","State":"running","Status":"Up 1 minute","Labels":"com.docker.compose.project=zoo"}`,
		`{"ID":"c2","Names":"alfa","State":"running","Status":"Up 1 minute","Labels":"com.docker.compose.project=zoo"}`,
		`{"ID":"c1","Names":"solo","State":"running","Status":"Up 1 minute"}`,
	}, "\n")+"\n")
	lista, err := Containers(context.Background(), run.run, false)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(lista) != 3 {
		t.Fatalf("esperaba 3, obtuve %d", len(lista))
	}
	// Sin proyecto van primero ("" < "zoo"), y dentro de un proyecto por nombre.
	if lista[0].Name != "solo" || lista[1].Name != "alfa" || lista[2].Name != "zebra" {
		t.Errorf("orden = %s, %s, %s", lista[0].Name, lista[1].Name, lista[2].Name)
	}
}

func TestContainersNombreConComaSeCorta(t *testing.T) {
	// `docker ps` separa con coma los nombres de los contenedores que
	// comparten alias de red.
	run := nuevoFake().con("ps --no-trunc --format {{json .}}",
		`{"ID":"c1","Names":"web,web2","State":"running","Status":"Up 1 minute"}`+"\n")
	lista, err := Containers(context.Background(), run.run, false)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if lista[0].Name != "web" {
		t.Errorf("Name = %q, quería web (se corta en la coma)", lista[0].Name)
	}
}

func TestContainersDeduceEstadoSiVieneVacio(t *testing.T) {
	run := nuevoFake().con("ps --no-trunc --format {{json .}}", strings.Join([]string{
		`{"ID":"c1","Names":"a","Status":"Up 5 seconds"}`,
		`{"ID":"c2","Names":"b","Status":"Created"}`,
	}, "\n")+"\n")
	lista, err := Containers(context.Background(), run.run, false)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if lista[0].State != "running" {
		t.Errorf("State = %q, quería running (deducido del Status)", lista[0].State)
	}
	if lista[1].State != "exited" {
		t.Errorf("State = %q, quería exited (deducido del Status)", lista[1].State)
	}
}

func TestContainersSaltaLineasBasura(t *testing.T) {
	run := nuevoFake().con("ps --no-trunc --format {{json .}}", strings.Join([]string{
		"",
		"Cannot connect to the Docker daemon",
		`{"ID":"roto","Names":`,
		`{"ID":"c1","Names":"bueno","State":"running","Status":"Up 1 minute"}`,
	}, "\n")+"\n")
	lista, err := Containers(context.Background(), run.run, false)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(lista) != 1 || lista[0].Name != "bueno" {
		t.Fatalf("solo debía quedar la fila válida: %+v", lista)
	}
}

func TestContainersPropagaElError(t *testing.T) {
	run := nuevoFake().falla("ps --no-trunc --format {{json .}}", "", errors.New("daemon caído"))
	if _, err := Containers(context.Background(), run.run, false); err == nil {
		t.Fatal("el error del daemon debería volver")
	}
}

func TestImagesMarcaLasDangling(t *testing.T) {
	run := nuevoFake().con("images --format {{json .}}", strings.Join([]string{
		`{"ID":"a1","Repository":"nginx","Tag":"latest","Size":"100MB","CreatedAt":"ayer"}`,
		`{"ID":"b2","Repository":"<none>","Tag":"<none>","Size":"12MB","CreatedAt":"ayer"}`,
	}, "\n")+"\n")
	lista, err := Images(context.Background(), run.run)
	if err != nil {
		t.Fatalf("Images: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(lista))
	}
	if lista[0].Dangling {
		t.Error("nginx:latest no es dangling")
	}
	if !lista[1].Dangling {
		t.Error("<none> debería ser dangling")
	}
	if lista[0].ID != "a1" {
		t.Errorf("ID = %q", lista[0].ID)
	}
}

func TestVolumesYRedes(t *testing.T) {
	run := nuevoFake().
		con("volume ls --format {{json .}}",
			`{"Name":"datos","Driver":"local","Mountpoint":"/var/lib/docker/volumes/datos/_data"}`+"\n").
		con("network ls --format {{json .}}", strings.Join([]string{
			`{"ID":"n1","Name":"bridge","Driver":"bridge","Scope":"local"}`,
			`{"ID":"n2","Name":"shop_default","Driver":"bridge","Scope":"local"}`,
		}, "\n")+"\n")

	volumenes, err := Volumes(context.Background(), run.run)
	if err != nil {
		t.Fatalf("Volumes: %v", err)
	}
	if len(volumenes) != 1 || volumenes[0].Name != "datos" || volumenes[0].Driver != "local" {
		t.Fatalf("volúmenes mal: %+v", volumenes)
	}

	redes, err := Networks(context.Background(), run.run)
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(redes) != 2 {
		t.Fatalf("esperaba 2 redes, obtuve %d", len(redes))
	}
	if !redes[0].Builtin {
		t.Error("bridge es una red nativa")
	}
	if redes[1].Builtin {
		t.Error("shop_default no es nativa")
	}
}

func TestCollectCuentaYAgrupa(t *testing.T) {
	run := nuevoFake().
		con("ps --no-trunc --format {{json .}}", strings.Join([]string{
			`{"ID":"c1","Names":"web","State":"running","Status":"Up 3 hours (healthy)","Labels":"com.docker.compose.project=shop,com.docker.compose.service=web"}`,
			`{"ID":"c2","Names":"db","State":"running","Status":"Up 1 hour (unhealthy)","Labels":"com.docker.compose.project=shop,com.docker.compose.service=db"}`,
			`{"ID":"c3","Names":"cache","State":"exited","Status":"Exited (0) 1 hour ago","Labels":"com.docker.compose.project=shop,com.docker.compose.service=cache"}`,
		}, "\n")+"\n").
		con("images --format {{json .}}", `{"ID":"i1","Repository":"nginx","Tag":"latest"}`+"\n").
		con("volume ls --format {{json .}}", `{"Name":"datos","Driver":"local"}`+"\n").
		con("network ls --format {{json .}}", `{"ID":"n1","Name":"bridge","Driver":"bridge"}`+"\n")

	snap := Collect(context.Background(), run.run, false)
	if snap.Error != "" {
		t.Fatalf("Collect falló: %s", snap.Error)
	}
	if snap.Running != 2 || snap.Stopped != 1 || snap.Unhealthy != 1 {
		t.Errorf("conteos = running %d, stopped %d, unhealthy %d", snap.Running, snap.Stopped, snap.Unhealthy)
	}
	if len(snap.Images) != 1 || len(snap.Volumes) != 1 || len(snap.Networks) != 1 {
		t.Errorf("faltan recursos en el snapshot: %+v", snap)
	}
	if len(snap.Projects) != 1 {
		t.Fatalf("esperaba 1 proyecto, obtuve %d", len(snap.Projects))
	}
	p := snap.Projects[0]
	if p.Name != "shop" || p.Containers != 3 || p.Running != 2 || p.Stopped != 1 {
		t.Errorf("proyecto mal: %+v", p)
	}
	if p.State != "partial" {
		t.Errorf("State = %q, quería partial (2 de 3 corriendo)", p.State)
	}
	if p.Unhealthy != 1 {
		t.Errorf("Unhealthy = %d, quería 1", p.Unhealthy)
	}
	if strings.Join(p.Services, ",") != "cache,db,web" {
		t.Errorf("servicios = %v, quería ordenados", p.Services)
	}
}

func TestCollectPropagaElError(t *testing.T) {
	run := nuevoFake().falla("ps --no-trunc --format {{json .}}", "", errors.New("daemon caído"))
	snap := Collect(context.Background(), run.run, false)
	if snap.Error == "" {
		t.Fatal("el snapshot debería llevar el error")
	}
	if !strings.Contains(snap.Error, "daemon caído") {
		t.Errorf("el error se perdió: %s", snap.Error)
	}
}

func TestHealthFromStatus(t *testing.T) {
	casos := []struct {
		status    string
		explicito string
		quiere    string
	}{
		{"Up 3 hours (healthy)", "", "healthy"},
		{"Up 3 hours (unhealthy)", "", "unhealthy"},
		{"Up 3 hours (health: starting)", "", ""},
		{"Up 3 hours (starting)", "", "starting"},
		{"Exited (0) 2 hours ago", "", ""},
		{"", "", ""},
		{"Up 1 hour", "healthy", "healthy"},
	}
	for _, c := range casos {
		if got := healthFromStatus(c.status, c.explicito); got != c.quiere {
			t.Errorf("healthFromStatus(%q, %q) = %q, quería %q", c.status, c.explicito, got, c.quiere)
		}
	}
}

func TestUptimeFromStatus(t *testing.T) {
	casos := map[string]string{
		"Up 3 hours (healthy)":   "3 hours",
		"Up 2 minutes":           "2 minutes",
		"Up Less than a second":  "Less than a second",
		"Exited (0) 2 hours ago": "",
		"":                       "",
	}
	for status, quiere := range casos {
		if got := uptimeFromStatus(status); got != quiere {
			t.Errorf("uptimeFromStatus(%q) = %q, quería %q", status, got, quiere)
		}
	}
}

func TestLabelValue(t *testing.T) {
	labels := "com.docker.compose.project=shop,com.docker.compose.service=web"
	if got := labelValue(labels, "com.docker.compose.project"); got != "shop" {
		t.Errorf("project = %q, quería shop", got)
	}
	if got := labelValue(labels, "com.docker.compose.service"); got != "web" {
		t.Errorf("service = %q, quería web", got)
	}
	if got := labelValue(labels, "no.existe"); got != "" {
		t.Errorf("una label ausente debería quedar vacía, obtuve %q", got)
	}
	// El valor puede tener "=" adentro: se parte en el primero.
	if got := labelValue("a=b=c", "a"); got != "b=c" {
		t.Errorf("a = %q, quería b=c", got)
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("abc123def456789"); got != "abc123def456" {
		t.Errorf("shortID = %q, quería 12 caracteres", got)
	}
	if got := shortID("corto"); got != "corto" {
		t.Errorf("shortID = %q, quería el id tal cual", got)
	}
	if got := shortID(""); got != "" {
		t.Errorf("shortID vacío = %q", got)
	}
}

func TestPrimeraLinea(t *testing.T) {
	if got := primeraLinea("  hola\nmundo  "); got != "hola" {
		t.Errorf("primeraLinea = %q, quería hola", got)
	}
	if got := primeraLinea(""); got != "" {
		t.Errorf("primeraLinea vacío = %q", got)
	}
	largo := strings.Repeat("x", 500)
	if got := primeraLinea(largo); len(got) != 200 {
		t.Errorf("primeraLinea largo = %d caracteres, quería 200", len(got))
	}
}
