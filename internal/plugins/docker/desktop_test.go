package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecInContainerCorreElComando(t *testing.T) {
	f := nuevoFake().con("exec web ls -la /etc", "total 4\ndrwxr-xr-x")
	res, err := ExecInContainer(context.Background(), f.run, "web", []string{"ls", "-la", "/etc"})
	if err != nil {
		t.Fatalf("ExecInContainer: %v", err)
	}
	if !res.Running {
		t.Error("debería marcar que está corriendo")
	}
	if res.Exit != 0 {
		t.Errorf("Exit = %d, quería 0", res.Exit)
	}
	if res.Output != "total 4\ndrwxr-xr-x" {
		t.Errorf("Output = %q", res.Output)
	}
}

func TestExecInContainerSinComandoFalla(t *testing.T) {
	f := nuevoFake()
	if _, err := ExecInContainer(context.Background(), f.run, "web", nil); err == nil {
		t.Fatal("sin comando debería fallar")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestExecInContainerPropagaElError(t *testing.T) {
	f := nuevoFake().falla("exec web ls", "container is not running", errors.New("container is not running"))
	res, err := ExecInContainer(context.Background(), f.run, "web", []string{"ls"})
	if err == nil {
		t.Fatal("el error debería volver")
	}
	if res.Running {
		t.Error("no debería marcar que está corriendo")
	}
}

func TestCopyYCopyFrom(t *testing.T) {
	f := nuevoFake().
		con("cp /tmp/origen web:/destino", "copiado").
		con("cp web:/origen /tmp/destino", "copiado")
	res, err := Copy(context.Background(), f.run, "web", "/tmp/origen", "/destino")
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if res.Path != "/destino" {
		t.Errorf("Path = %q", res.Path)
	}
	res, err = CopyFrom(context.Background(), f.run, "web", "/origen", "/tmp/destino")
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
	}
	if res.Path != "/tmp/destino" {
		t.Errorf("Path = %q", res.Path)
	}
}

func TestCommitArmaElComando(t *testing.T) {
	f := nuevoFake().con("commit -m mensaje web repo:1.0", "sha256:abc")
	res, err := Commit(context.Background(), f.run, "web", "repo", "1.0", "mensaje")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Command != "docker commit -m mensaje web repo:1.0" {
		t.Errorf("Command = %q", res.Command)
	}
}

func TestCommitSinTagUsaLatest(t *testing.T) {
	f := nuevoFake().con("commit web repo:latest", "sha256:abc")
	if _, err := Commit(context.Background(), f.run, "web", "repo", "", ""); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !f.llamado("commit web repo:latest") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestCommitSinRepoNoAgregaImagen(t *testing.T) {
	f := nuevoFake().con("commit web", "sha256:abc")
	if _, err := Commit(context.Background(), f.run, "web", "", "", ""); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !f.llamado("commit web") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestTagUsaLatestPorDefecto(t *testing.T) {
	f := nuevoFake().con("tag img repo:latest", "")
	if _, err := Tag(context.Background(), f.run, "img", "repo", ""); err != nil {
		t.Fatalf("Tag: %v", err)
	}
	if !f.llamado("tag img repo:latest") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestRunCommandPasaLosArgumentos(t *testing.T) {
	f := nuevoFake().con("port web", "0.0.0.0:8080->80/tcp")
	res, err := RunCommand(context.Background(), f.run, "port", []string{"web"})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.Command != "docker port web" {
		t.Errorf("Command = %q", res.Command)
	}
	if res.Output != "0.0.0.0:8080->80/tcp" {
		t.Errorf("Output = %q", res.Output)
	}
}

func TestPrettyBytes(t *testing.T) {
	casos := map[int64]string{
		0:                  "0 B",
		512:                "512 B",
		1024:               "1.0 KB",
		1536:               "1.5 KB",
		1024 * 1024:        "1.0 MB",
		1024 * 1024 * 1024: "1.0 GB",
	}
	for bytes, quiere := range casos {
		if got := PrettyBytes(bytes); got != quiere {
			t.Errorf("PrettyBytes(%d) = %q, quería %q", bytes, got, quiere)
		}
	}
}

func TestGetInfoParseaElJSON(t *testing.T) {
	salida := `{"ServerVersion":"27.1.1","Driver":"overlay2","DockerRootDir":"/var/lib/docker","NCPU":8,"MemTotal":16000000000,"Containers":5,"ContainersRunning":2,"ContainersPaused":1,"ContainersStopped":2,"Images":12,"OSType":"linux","Architecture":"x86_64"}`
	f := nuevoFake().con("info --format json", salida)
	info, err := GetInfo(context.Background(), f.run)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.ServerVersion != "27.1.1" || info.Driver != "overlay2" {
		t.Errorf("versión/driver mal: %+v", info)
	}
	if info.NCPU != 8 || info.MemTotal != 16000000000 {
		t.Errorf("hardware mal: %+v", info)
	}
	if info.Containers != 5 || info.Running != 2 || info.Paused != 1 || info.Stopped != 2 {
		t.Errorf("conteos mal: %+v", info)
	}
	if info.Images != 12 || info.OS != "linux" || info.Arch != "x86_64" {
		t.Errorf("sistema mal: %+v", info)
	}
	if info.RootDir != "/var/lib/docker" {
		t.Errorf("RootDir = %q", info.RootDir)
	}
}

func TestGetInfoConJSONMaloFalla(t *testing.T) {
	f := nuevoFake().con("info --format json", "{roto")
	if _, err := GetInfo(context.Background(), f.run); err == nil {
		t.Fatal("un JSON roto debería ser error")
	}
}

func TestGetInfoPropagaElError(t *testing.T) {
	f := nuevoFake().falla("info --format json", "", errors.New("daemon caído"))
	if _, err := GetInfo(context.Background(), f.run); err == nil {
		t.Fatal("el error debería volver")
	}
}

func TestExecUsaLaPrimeraLineaDelError(t *testing.T) {
	// primeraLinea recorta el error de docker a una sola línea: el stack
	// entero no sirve para el panel.
	got := primeraLinea("Error response from daemon: no such container\nat main.main()")
	if strings.Contains(got, "\n") {
		t.Errorf("primeraLinea = %q, quería una sola línea", got)
	}
	if !strings.HasPrefix(got, "Error response from daemon") {
		t.Errorf("primeraLinea = %q", got)
	}
}
