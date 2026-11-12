package docker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAccionesMandanElComando(t *testing.T) {
	casos := []struct {
		nombre string
		llamar func(Runner) (Action, error)
		quiere string
	}{
		{"start", func(r Runner) (Action, error) { return Start(context.Background(), r, "web") }, "start web"},
		{"stop", func(r Runner) (Action, error) { return Stop(context.Background(), r, "web") }, "stop web"},
		{"restart", func(r Runner) (Action, error) { return Restart(context.Background(), r, "web") }, "restart web"},
		{"remove", func(r Runner) (Action, error) { return Remove(context.Background(), r, "web") }, "rm -f web"},
		{"rmi", func(r Runner) (Action, error) { return RemoveImage(context.Background(), r, "img") }, "rmi img"},
		{"volume-rm", func(r Runner) (Action, error) { return RemoveVolume(context.Background(), r, "vol") }, "volume rm vol"},
		{"network-rm", func(r Runner) (Action, error) { return RemoveNetwork(context.Background(), r, "red") }, "network rm red"},
		{"pull", func(r Runner) (Action, error) { return Pull(context.Background(), r, "img") }, "pull img"},
		{"push", func(r Runner) (Action, error) { return Push(context.Background(), r, "img") }, "push img"},
		{"volume-create", func(r Runner) (Action, error) { return CreateVolume(context.Background(), r, "vol") }, "volume create vol"},
		{"network-create", func(r Runner) (Action, error) { return CreateNetwork(context.Background(), r, "red") }, "network create red"},
		{"pause", func(r Runner) (Action, error) { return Pause(context.Background(), r, "web") }, "pause web"},
		{"unpause", func(r Runner) (Action, error) { return Unpause(context.Background(), r, "web") }, "unpause web"},
		{"rename", func(r Runner) (Action, error) { return Rename(context.Background(), r, "web", "nuevo") }, "rename web nuevo"},
		{"diff", func(r Runner) (Action, error) { return Diff(context.Background(), r, "web") }, "diff web"},
		{"top", func(r Runner) (Action, error) { return Top(context.Background(), r, "web") }, "top web"},
		{"history", func(r Runner) (Action, error) { return History(context.Background(), r, "img") }, "history --no-trunc img"},
	}
	for _, c := range casos {
		f := nuevoFake().con(c.quiere, "salida")
		res, err := c.llamar(f.run)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if !res.OK {
			t.Errorf("%s: la acción debería estar OK", c.nombre)
		}
		if res.Command != "docker "+c.quiere {
			t.Errorf("%s: Command = %q, quería %q", c.nombre, res.Command, "docker "+c.quiere)
		}
		if res.Output != "salida" {
			t.Errorf("%s: Output = %q", c.nombre, res.Output)
		}
	}
}

func TestAccionesPropaganElError(t *testing.T) {
	f := nuevoFake().falla("stop web", "Error response from daemon: no such container", errors.New("no such container"))
	res, err := Stop(context.Background(), f.run, "web")
	if err == nil {
		t.Fatal("el error debería volver")
	}
	if res.OK {
		t.Error("la acción no debería estar OK si docker falló")
	}
	if !strings.Contains(res.Output, "no such container") {
		t.Errorf("el error no quedó en la salida: %q", res.Output)
	}
}

func TestRemoveNetworkRechazaLasNativas(t *testing.T) {
	for _, nativa := range []string{"bridge", "host", "none"} {
		f := nuevoFake()
		res, err := RemoveNetwork(context.Background(), f.run, nativa)
		if err == nil {
			t.Errorf("%s: las redes nativas deberían estar protegidas", nativa)
		}
		if res.OK {
			t.Errorf("%s: no debería estar OK", nativa)
		}
		if len(f.pedidos()) != 0 {
			t.Errorf("%s: no debería haber llamado a docker: %v", nativa, f.pedidos())
		}
	}
}

func TestRunImageArmaElComando(t *testing.T) {
	f := nuevoFake().con("run -d --name web --network shop -p 8080:80 -e CLAVE=valor nginx", "abc123")
	res, err := RunImage(context.Background(), f.run, RunOptions{
		Image:   "nginx",
		Name:    "web",
		Network: "shop",
		Ports:   []string{"8080:80"},
		Env:     []string{"CLAVE=valor"},
	})
	if err != nil {
		t.Fatalf("RunImage: %v", err)
	}
	if !res.OK || res.Output != "abc123" {
		t.Errorf("resultado mal: %+v", res)
	}
}

func TestRunImageSinImagenFalla(t *testing.T) {
	f := nuevoFake()
	res, err := RunImage(context.Background(), f.run, RunOptions{Name: "web"})
	if err == nil {
		t.Fatal("sin imagen debería fallar")
	}
	if res.OK {
		t.Error("no debería estar OK")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestRunImageRechazaPuertoInvalido(t *testing.T) {
	f := nuevoFake()
	_, err := RunImage(context.Background(), f.run, RunOptions{Image: "nginx", Ports: []string{"no-es-un-puerto"}})
	if err == nil {
		t.Fatal("un puerto inválido debería fallar")
	}
	if !strings.Contains(err.Error(), "puerto inválido") {
		t.Errorf("el error debería nombrar el puerto: %v", err)
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestRunImageRechazaVariableSinIgual(t *testing.T) {
	f := nuevoFake()
	_, err := RunImage(context.Background(), f.run, RunOptions{Image: "nginx", Env: []string{"SIN_VALOR"}})
	if err == nil {
		t.Fatal("una variable sin '=' debería fallar")
	}
	if !strings.Contains(err.Error(), "variable inválida") {
		t.Errorf("el error debería nombrar la variable: %v", err)
	}
}

func TestLogsUsaElTailPorDefecto(t *testing.T) {
	f := nuevoFake().con("logs --tail 200 web", "linea 1\nlinea 2")
	salida, err := Logs(context.Background(), f.run, "web", 0)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if salida != "linea 1\nlinea 2" {
		t.Errorf("salida = %q", salida)
	}
	if !f.llamado("logs --tail 200 web") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestLogsRespetaElTailPedido(t *testing.T) {
	f := nuevoFake().con("logs --tail 20 web", "x")
	if _, err := Logs(context.Background(), f.run, "web", 20); err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if !f.llamado("logs --tail 20 web") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestBuildValidaLaEtiqueta(t *testing.T) {
	f := nuevoFake()
	if _, err := Build(context.Background(), f.run, "", ".", ""); err == nil {
		t.Error("sin etiqueta debería fallar")
	}
	if _, err := Build(context.Background(), f.run, "-malo", ".", ""); err == nil {
		t.Error("una etiqueta que empieza con '-' debería fallar")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestBuildValidaElContextoYElDockerfile(t *testing.T) {
	f := nuevoFake()
	if _, err := Build(context.Background(), f.run, "app", "-contexto", ""); err == nil {
		t.Error("un contexto que empieza con '-' debería fallar")
	}
	if _, err := Build(context.Background(), f.run, "app", ".", "-archivo"); err == nil {
		t.Error("un Dockerfile que empieza con '-' debería fallar")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

func TestBuildArmaElComando(t *testing.T) {
	f := nuevoFake().con("build -t app -f Dockerfile .", "ok")
	res, err := Build(context.Background(), f.run, "app", ".", "Dockerfile")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !res.OK {
		t.Error("debería estar OK")
	}
}

func TestBuildSinContextoUsaElActual(t *testing.T) {
	f := nuevoFake().con("build -t app .", "ok")
	if _, err := Build(context.Background(), f.run, "app", "  ", ""); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !f.llamado("build -t app .") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

func TestAccionesConArchivoArmanElComando(t *testing.T) {
	casos := []struct {
		nombre string
		llamar func(Runner) (Action, error)
		quiere string
	}{
		{"save", func(r Runner) (Action, error) {
			return SaveImage(context.Background(), r, "img", "/tmp/x.tar")
		}, "save -o /tmp/x.tar img"},
		{"load", func(r Runner) (Action, error) {
			return LoadImage(context.Background(), r, "/tmp/x.tar")
		}, "load -i /tmp/x.tar"},
		{"export", func(r Runner) (Action, error) {
			return ExportContainer(context.Background(), r, "web", "/tmp/x.tar")
		}, "export -o /tmp/x.tar web"},
	}
	for _, c := range casos {
		f := nuevoFake().con(c.quiere, "ok")
		res, err := c.llamar(f.run)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if res.Command != "docker "+c.quiere {
			t.Errorf("%s: Command = %q, quería %q", c.nombre, res.Command, "docker "+c.quiere)
		}
	}
}

func TestAllowedActions(t *testing.T) {
	// La lista es la documentación de lo que el CLI acepta: si alguien saca
	// una acción, el plugin deja de poder invocarla.
	for _, accion := range []string{"start", "stop", "restart", "remove", "rmi", "run", "build", "pause"} {
		if !AllowedActions[accion] {
			t.Errorf("%q debería estar permitido", accion)
		}
	}
	if AllowedActions["inventado"] {
		t.Error("una acción que no existe no debería estar permitida")
	}
}
