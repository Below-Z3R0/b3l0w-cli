package docker

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
)

// Una fila real de `docker inspect --format '{{json .}}'`.
const inspectJSON = `{
  "Id": "abc123def4567890123456789012345678901234567890123456789012345678",
  "Name": "/web",
  "State": {
    "Status": "running",
    "StartedAt": "2024-01-01T10:00:00.000000000Z",
    "FinishedAt": "0001-01-01T00:00:00Z"
  },
  "RestartCount": 2,
  "HostConfig": {"RestartPolicy": {"Name": "unless-stopped"}},
  "Platform": "linux",
  "Path": "/docker-entrypoint.sh",
  "Args": ["nginx", "-g", "daemon off;"],
  "Config": {
    "Image": "nginx:latest",
    "Env": ["PATH=/usr/bin", "NGINX_VERSION=1.27"],
    "Labels": {"com.docker.compose.project": "shop"},
    "Cmd": ["nginx", "-g", "daemon off;"],
    "Health": {"Log": [{"Output": "ok"}, {"Output": "check 1"}]}
  },
  "Mounts": [
    {"Type": "bind", "Source": "/srv/datos", "Destination": "/data", "Mode": "rw", "RW": true},
    {"Type": "volume", "Source": "vol_cache", "Destination": "/cache", "Mode": "ro", "RW": false}
  ],
  "NetworkSettings": {"Networks": {"bridge": {}, "shop_default": {}}}
}`

func TestInspectAplanaElContenedor(t *testing.T) {
	f := nuevoFake().con("inspect --format {{json .}} web", inspectJSON)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if info.ID != "abc123def456" {
		t.Errorf("ID = %q, quería el id corto", info.ID)
	}
	if info.Name != "web" {
		t.Errorf("Name = %q, quería web (sin la barra inicial)", info.Name)
	}
	if info.Image != "nginx:latest" {
		t.Errorf("Image = %q", info.Image)
	}
	if info.State != "running" {
		t.Errorf("State = %q, quería running", info.State)
	}
	if info.StartedAt == "" {
		t.Error("falta StartedAt")
	}
	if info.RestartCount != 2 {
		t.Errorf("RestartCount = %d, quería 2", info.RestartCount)
	}
	if info.RestartPolicy != "unless-stopped" {
		t.Errorf("RestartPolicy = %q", info.RestartPolicy)
	}
	if info.Platform != "linux" {
		t.Errorf("Platform = %q", info.Platform)
	}
	// El comando es Path + Args, no Config.Cmd.
	if strings.Join(info.Command, " ") != "/docker-entrypoint.sh nginx -g daemon off;" {
		t.Errorf("Command = %v", info.Command)
	}
	if len(info.Env) != 2 || info.Env[0] != "PATH=/usr/bin" {
		t.Errorf("Env = %v", info.Env)
	}
	if info.Labels["com.docker.compose.project"] != "shop" {
		t.Errorf("Labels = %v", info.Labels)
	}
}

func TestInspectMontaReadOnly(t *testing.T) {
	f := nuevoFake().con("inspect --format {{json .}} web", inspectJSON)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if len(info.Mounts) != 2 {
		t.Fatalf("esperaba 2 montajes, obtuve %d", len(info.Mounts))
	}
	if info.Mounts[0].Type != "bind" || info.Mounts[0].Source != "/srv/datos" || info.Mounts[0].Destination != "/data" {
		t.Errorf("montaje mal: %+v", info.Mounts[0])
	}
	if info.Mounts[0].ReadOnly {
		t.Error("RW:true debería ser de escritura")
	}
	if !info.Mounts[1].ReadOnly {
		t.Error("RW:false debería ser de sólo lectura")
	}
}

func TestInspectListaLasRedes(t *testing.T) {
	f := nuevoFake().con("inspect --format {{json .}} web", inspectJSON)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	// El mapa de redes no tiene orden: se ordena para comparar.
	redes := append([]string(nil), info.Networks...)
	sort.Strings(redes)
	if strings.Join(redes, ",") != "bridge,shop_default" {
		t.Errorf("Networks = %v, quería bridge y shop_default", redes)
	}
}

func TestInspectCortaElHealthLog(t *testing.T) {
	// Siete entradas: el log se corta en cinco.
	entradas := make([]string, 0, 7)
	for i := 1; i <= 7; i++ {
		entradas = append(entradas, `{"Output":"linea `+string(rune('0'+i))+`"}`)
	}
	payload := `{"Id":"x","Name":"/c","State":{"Status":"running"},"Config":{"Health":{"Log":[` +
		strings.Join(entradas, ",") + `]}}}`
	f := nuevoFake().con("inspect --format {{json .}} c", payload)
	info, err := InspectContainer(context.Background(), f.run, "c")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if len(info.HealthLog) != 5 {
		t.Fatalf("HealthLog tiene %d entradas, quería 5 (el tope)", len(info.HealthLog))
	}
	if info.HealthLog[0] != "linea 1" {
		t.Errorf("HealthLog[0] = %q", info.HealthLog[0])
	}
}

func TestInspectContenedorParado(t *testing.T) {
	payload := `{"Id":"x","Name":"/viejo","State":{"Status":"exited","FinishedAt":"2024-01-01T12:00:00Z"},"Config":{"Image":"alpine"}}`
	f := nuevoFake().con("inspect --format {{json .}} viejo", payload)
	info, err := InspectContainer(context.Background(), f.run, "viejo")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if info.State != "exited" {
		t.Errorf("State = %q, quería exited", info.State)
	}
	if info.FinishedAt == "" {
		t.Error("un contenedor parado tiene FinishedAt")
	}
	if info.StartedAt != "" {
		t.Errorf("StartedAt = %q, quería vacío", info.StartedAt)
	}
}

func TestInspectSalidaVaciaEsError(t *testing.T) {
	f := nuevoFake().con("inspect --format {{json .}} web", "")
	if _, err := InspectContainer(context.Background(), f.run, "web"); err == nil {
		t.Fatal("la salida vacía debería ser error, no un contenedor vacío")
	}
}

func TestInspectJSONMalformadoEsError(t *testing.T) {
	f := nuevoFake().con("inspect --format {{json .}} web", `{"Id": "x", "Name":`)
	if _, err := InspectContainer(context.Background(), f.run, "web"); err == nil {
		t.Fatal("un JSON cortado debería ser error")
	}
}

func TestInspectPropagaElError(t *testing.T) {
	f := nuevoFake().falla("inspect --format {{json .}} web", "No such object", errors.New("no such container"))
	if _, err := InspectContainer(context.Background(), f.run, "web"); err == nil {
		t.Fatal("el error de docker debería volver")
	}
}
