package dockercli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// conRunnerFalso reemplaza el runner del CLI por uno falso y devuelve una
// función que restaura el original.
func conRunnerFalso(t *testing.T, salida string) func() {
	t.Helper()
	anterior := runner
	runner = func(ctx context.Context, args ...string) (string, error) {
		return salida, nil
	}
	return func() { runner = anterior }
}

// capturaStdout corre fn con stdout redirigido a un pipe y devuelve lo que
// escribió.
func capturaStdout(t *testing.T, fn func()) string {
	t.Helper()
	return captura(t, &os.Stdout, fn)
}

// capturaStderr corre fn con stderr redirigido a un pipe y devuelve lo que
// escribió.
func capturaStderr(t *testing.T, fn func()) string {
	t.Helper()
	return captura(t, &os.Stderr, fn)
}

func captura(t *testing.T, destino **os.File, fn func()) string {
	t.Helper()
	original := *destino
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	*destino = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	fn()
	w.Close()
	*destino = original
	return <-done
}

func TestComandoDesconocidoDevuelveDos(t *testing.T) {
	codigo := Run([]string{"no-existe"})
	if codigo != 2 {
		t.Errorf("código = %d, quería 2", codigo)
	}
}

func TestAyudaDevuelveCero(t *testing.T) {
	for _, flag := range []string{"help", "-h", "--help"} {
		salida := capturaStdout(t, func() {
			if codigo := Run([]string{flag}); codigo != 0 {
				t.Errorf("%s: código = %d, quería 0", flag, codigo)
			}
		})
		if !strings.Contains(salida, "b3l0w docker") {
			t.Errorf("%s: la ayuda no se imprimió: %q", flag, salida)
		}
	}
}

func TestSinArgumentosImprimeLaAyuda(t *testing.T) {
	salida := capturaStdout(t, func() {
		if codigo := Run(nil); codigo != 2 {
			t.Errorf("código = %d, quería 2", codigo)
		}
	})
	if !strings.Contains(salida, "uso: b3l0w docker") {
		t.Errorf("no imprimió la ayuda: %q", salida)
	}
}

func TestSnapshotDevuelveJSON(t *testing.T) {
	restaurar := conRunnerFalso(t, "")
	defer restaurar()
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"snapshot", "--json"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	var snap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(salida)), &snap); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	// El plugin Luau hace #payload.containers: la lista tiene que existir.
	if _, ok := snap["containers"]; !ok {
		t.Errorf("falta 'containers' en el snapshot: %v", snap)
	}
	if snap["containers"] == nil {
		t.Errorf("'containers' no puede ser null: %v", snap)
	}
}

func TestInspectDevuelveJSON(t *testing.T) {
	payload := `{"Id":"abc123def456","Name":"/web","State":{"Status":"running"},"Config":{"Image":"nginx:latest"}}`
	restaurar := conRunnerFalso(t, payload)
	defer restaurar()
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"inspect", "web", "--json"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	var info map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(salida)), &info); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if info["name"] != "web" {
		t.Errorf("name = %v, quería web", info["name"])
	}
}

func TestStatsDevuelveJSON(t *testing.T) {
	restaurar := conRunnerFalso(t, `{"ID":"abc","Name":"web","CPUPerc":"1.5%","MemPerc":"2.5%","PIDs":"3"}`)
	defer restaurar()
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"stats", "--json"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	var cuerpo struct {
		Stats []map[string]any `json:"stats"`
		At    int64            `json:"at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(salida)), &cuerpo); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(cuerpo.Stats) != 1 {
		t.Fatalf("stats = %v", cuerpo.Stats)
	}
	if cuerpo.Stats[0]["name"] != "web" {
		t.Errorf("name = %v", cuerpo.Stats[0]["name"])
	}
	if cuerpo.At <= 0 {
		t.Errorf("at = %d, quería un timestamp", cuerpo.At)
	}
}

func TestPruneConDestinoInvalidoDevuelveDos(t *testing.T) {
	restaurar := conRunnerFalso(t, "")
	defer restaurar()
	// En modo JSON el motivo del error viaja en stdout, no en stderr.
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"prune", "--json", "inventado"}); codigo != 2 {
			t.Errorf("código = %d, quería 2", codigo)
		}
	})
	if !strings.Contains(salida, "destino inválido") {
		t.Errorf("no explicó el error: %q", salida)
	}
	// Y en modo texto va a stderr.
	errSalida := capturaStderr(t, func() {
		if codigo := Run([]string{"prune", "inventado"}); codigo != 2 {
			t.Errorf("código = %d, quería 2", codigo)
		}
	})
	if !strings.Contains(errSalida, "destino inválido") {
		t.Errorf("no explicó el error por stderr: %q", errSalida)
	}
}

func TestControlSinIdDevuelveDos(t *testing.T) {
	for _, accion := range []string{"start", "stop", "restart", "remove"} {
		if codigo := Run([]string{accion}); codigo != 2 {
			t.Errorf("%s: código = %d, quería 2", accion, codigo)
		}
	}
}

func TestLogsStopSinSeguimiento(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"logs-stop", "web"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if !strings.Contains(salida, "sin seguimiento") {
		t.Errorf("salida = %q, quería 'sin seguimiento'", salida)
	}
}

func TestLogsStopMataElProceso(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Un proceso propio (no docker) como stand-in del seguimiento.
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("no se pudo arrancar sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	_, pidFile := rutaLog("web")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"logs-stop", "web"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if !strings.Contains(salida, "detenido") {
		t.Errorf("salida = %q, quería 'detenido'", salida)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("el archivo de pid debería haberse borrado: %v", err)
	}
}

func TestComandosSimplesConRunnerFalso(t *testing.T) {
	// Cada comando llama al runner y devuelve su salida. Se recorren todos
	// para que un comando nuevo no se cuelgue sin que nadie lo note.
	casos := []struct {
		nombre string
		args   []string
	}{
		{"port", []string{"port", "web"}},
		{"search", []string{"search", "nginx"}},
		{"update", []string{"update", "--id", "web", "--cpus", "2"}},
		{"volume-inspect", []string{"volume-inspect", "datos"}},
		{"network-inspect", []string{"network-inspect", "bridge"}},
		{"top", []string{"top", "web"}},
		{"history", []string{"history", "img"}},
		{"pull", []string{"pull", "nginx"}},
		{"push", []string{"push", "nginx"}},
		{"save", []string{"save", "--image", "img", "--out", "/tmp/x.tar"}},
		{"load", []string{"load", "--in", "/tmp/x.tar"}},
		{"export", []string{"export", "--id", "web", "--out", "/tmp/x.tar"}},
		{"diff", []string{"diff", "--id", "web"}},
		{"build", []string{"build", "--tag", "app", "--context", "."}},
		{"tag", []string{"tag", "--id", "img", "--repo", "repo"}},
		{"rename", []string{"rename", "--id", "web", "--name", "nuevo"}},
		{"commit", []string{"commit", "--id", "web"}},
		{"pause", []string{"pause", "--id", "web"}},
		{"unpause", []string{"unpause", "--id", "web"}},
		{"volume-create", []string{"volume-create", "datos"}},
		{"network-create", []string{"network-create", "red"}},
		{"rmi", []string{"rmi", "img"}},
		{"volume-rm", []string{"volume-rm", "datos"}},
		{"start", []string{"start", "web"}},
		{"stop", []string{"stop", "web"}},
		{"restart", []string{"restart", "web"}},
		{"remove", []string{"remove", "web"}},
		{"run", []string{"run", "--image", "nginx", "--name", "web", "--port", "8080:80"}},
		{"logs", []string{"logs", "web", "--tail", "50"}},
		{"network-rm", []string{"network-rm", "red"}},
	}
	for _, c := range casos {
		restaurar := conRunnerFalso(t, "salida")
		codigo := 0
		capturaStdout(t, func() {
			codigo = Run(c.args)
		})
		restaurar()
		if codigo != 0 {
			t.Errorf("%s: código = %d, quería 0", c.nombre, codigo)
		}
	}
}

func TestComandosDeLecturaConRunnerFalso(t *testing.T) {
	// df, info, events y compose devuelven JSON: se verifica que sale
	// parseable, que es lo que necesita el plugin.
	casos := [][]string{
		{"df", "--json"},
		{"info", "--json"},
		{"events", "--json"},
		{"compose", "ls"},
		{"snapshot", "--json"},
	}
	for _, args := range casos {
		restaurar := conRunnerFalso(t, "{}")
		salida := capturaStdout(t, func() {
			Run(args)
		})
		restaurar()
		var cuerpo map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(salida)), &cuerpo); err != nil {
			t.Errorf("%v: la salida no es JSON: %v (%q)", args, err, salida)
		}
	}
}

func TestVolumeExportEImportArmanElComando(t *testing.T) {
	casos := [][]string{
		{"volume-export", "--name", "datos", "--out", "/tmp/backup.tar.gz"},
		{"volume-import", "--name", "datos", "--in", "/tmp/backup.tar.gz"},
	}
	for _, args := range casos {
		restaurar := conRunnerFalso(t, "salida")
		codigo := 0
		capturaStdout(t, func() {
			codigo = Run(args)
		})
		restaurar()
		if codigo != 0 {
			t.Errorf("%v: código = %d, quería 0", args, codigo)
		}
	}
}

func TestVolumeExportRechazaNombresConGuion(t *testing.T) {
	if codigo := Run([]string{"volume-export", "--name", "-malo", "--out", "/tmp/x.tar.gz"}); codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if codigo := Run([]string{"volume-import", "--name", "datos", "--in", "-malo"}); codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
}

func TestComposeFileLeeElYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(yaml, []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ls := `[{"Name":"shop","Status":"running(1)","ConfigFiles":"` + yaml + `"}]`
	restaurar := conRunnerFalso(t, ls)
	defer restaurar()
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"compose-file", "shop"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if !strings.Contains(salida, "services:") {
		t.Errorf("no imprimió el YAML: %q", salida)
	}
}

func TestComposeFileConProyectoInexistente(t *testing.T) {
	restaurar := conRunnerFalso(t, `[{"Name":"otro","Status":"running(1)","ConfigFiles":"/tmp/x.yml"}]`)
	defer restaurar()
	capturaStderr(t, func() {
		if codigo := Run([]string{"compose-file", "shop"}); codigo != 1 {
			t.Errorf("código = %d, quería 1", codigo)
		}
	})
}

// dockerFalso escribe un script llamado "docker" en un directorio temporal y
// lo pone al principio del PATH. Sirve para los comandos que ejecutan docker
// fuera del runner (los que siguen logs en vivo): el binario real nunca se
// toca.
func dockerFalso(t *testing.T, cuerpo string) string {
	t.Helper()
	dir := t.TempDir()
	archivo := filepath.Join(dir, "docker")
	if err := os.WriteFile(archivo, []byte(cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestLogsFollowUsaElTailPorDefecto(t *testing.T) {
	// El script anota los argumentos con los que lo llamaron.
	marca := filepath.Join(t.TempDir(), "args.txt")
	dockerFalso(t, "#!/bin/sh\necho \"$@\" > "+marca+"\n")
	if codigo := Run([]string{"logs-follow", "web"}); codigo != 0 {
		t.Errorf("código = %d, quería 0", codigo)
	}
	datos, err := os.ReadFile(marca)
	if err != nil {
		t.Fatalf("el docker falso no corrió: %v", err)
	}
	if !strings.Contains(string(datos), "logs --follow --tail 200 web") {
		t.Errorf("args = %q, quería 'logs --follow --tail 200 web'", datos)
	}
}

func TestLogsFollowRespetaElTail(t *testing.T) {
	marca := filepath.Join(t.TempDir(), "args.txt")
	dockerFalso(t, "#!/bin/sh\necho \"$@\" > "+marca+"\n")
	if codigo := Run([]string{"logs-follow", "web", "--tail", "50"}); codigo != 0 {
		t.Errorf("código = %d, quería 0", codigo)
	}
	datos, err := os.ReadFile(marca)
	if err != nil {
		t.Fatalf("el docker falso no corrió: %v", err)
	}
	if !strings.Contains(string(datos), "--tail 50") {
		t.Errorf("args = %q, quería --tail 50", datos)
	}
}

func TestExecYCPConRunnerFalso(t *testing.T) {
	casos := [][]string{
		{"exec", "--id", "web", "ls"},
		{"cp", "--from", "web", "id:/etc/hostname", "--to", "/tmp/hostname"},
		{"cp", "--from", "web", "/tmp/origen", "--to", "id:/destino"},
	}
	for _, args := range casos {
		restaurar := conRunnerFalso(t, "salida")
		codigo := 0
		capturaStdout(t, func() {
			codigo = Run(args)
		})
		restaurar()
		if codigo != 0 {
			t.Errorf("%v: código = %d, quería 0", args, codigo)
		}
	}
}

func TestExecSinIdDevuelveUno(t *testing.T) {
	if codigo := Run([]string{"exec", "--id", "web"}); codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
}

func TestBuildSinTagDevuelveUno(t *testing.T) {
	if codigo := Run([]string{"build"}); codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
}

func TestCopySinDestinoDevuelveUno(t *testing.T) {
	if codigo := Run([]string{"cp", "--from", "web", "/origen"}); codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
}
