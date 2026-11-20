package dockercli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker"
)

// runnerGrabador anota los comandos que se pidieron al runner.
type runnerGrabador struct {
	pedidos []string
}

func (g *runnerGrabador) run(ctx context.Context, args ...string) (string, error) {
	g.pedidos = append(g.pedidos, strings.Join(args, " "))
	return "salida", nil
}

func conRunnerGrabador(t *testing.T, g *runnerGrabador) func() {
	t.Helper()
	anterior := runner
	runner = g.run
	return func() { runner = anterior }
}

// escribirPidfile escribe el pidfile con el formato nuevo: pid y starttime.
// El starttime es lo que permite comprobar que el pid no fue reciclado.
func escribirPidfile(t *testing.T, ruta string, pid int, starttime int) {
	t.Helper()
	if err := os.WriteFile(ruta, []byte(strconv.Itoa(pid)+" "+strconv.Itoa(starttime)), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ── D2 ───────────────────────────────────────────────────────────────────

// TestLogsStreamNoEjecutaComandosDelId es el D2: el id del contenedor nunca
// puede pasar por un shell. Con `sh -c` un id con `$(...)` ejecutaba
// comandos.
func TestLogsStreamNoEjecutaComandosDelId(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	marca := filepath.Join(dir, "PWNED")
	argsFile := filepath.Join(dir, "args.txt")
	pidFile := filepath.Join(dir, "pid.txt")
	// El docker falso anota sus argumentos y se queda vivo.
	dockerFalso(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\necho $$ > "+pidFile+"\nsleep 300\n")
	defer limpiarProcesoDocker(dir, pidFile)

	id := "web$(touch " + marca + ")"
	codigo := 0
	capturaStdout(t, func() {
		codigo = Run([]string{"logs-stream", id})
	})
	if codigo != 0 {
		t.Errorf("código = %d, quería 0", codigo)
	}
	// Lo importante: el comando del id no se ejecutó.
	if _, err := os.Stat(marca); !os.IsNotExist(err) {
		t.Fatalf("se ejecutó el comando del id: %s existe", marca)
	}
	// Y el id llegó a docker como un argumento literal, no como código.
	time.Sleep(200 * time.Millisecond)
	datos, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("el docker falso no corrió: %v", err)
	}
	lineas := strings.Split(strings.TrimSpace(string(datos)), "\n")
	if len(lineas) != 5 {
		t.Fatalf("args = %v, quería 5 (logs -f --tail 200 <id>)", lineas)
	}
	if lineas[0] != "logs" || lineas[1] != "-f" || lineas[2] != "--tail" || lineas[3] != "200" {
		t.Errorf("args = %v, quería logs -f --tail 200", lineas)
	}
	if lineas[4] != id {
		t.Errorf("el id llegó como %q, quería %q literal", lineas[4], id)
	}
}

// TestLogsStreamConIdConPuntoYComa tampoco rompe: el id va literal.
func TestLogsStreamAceptaIdsConCaracteresRaros(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	pidFile := filepath.Join(dir, "pid.txt")
	dockerFalso(t, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\necho $$ > "+pidFile+"\nsleep 300\n")
	defer limpiarProcesoDocker(dir, pidFile)

	id := "web; touch /tmp/nada"
	capturaStdout(t, func() {
		Run([]string{"logs-stream", id})
	})
	time.Sleep(200 * time.Millisecond)
	datos, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("el docker falso no corrió: %v", err)
	}
	if !strings.Contains(string(datos), id) {
		t.Errorf("args = %q, quería el id literal", datos)
	}
}

// ── D3 ───────────────────────────────────────────────────────────────────

// TestLogsStopNoMataUnProcesoAjeno es el D3: el pid del pidfile puede
// estar reciclado. Con sólo el pid se mataba a un proceso ajeno.
func TestLogsStopNoMataUnProcesoAjeno(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Un proceso propio, en su propio grupo, como el del probe.
	cmd := exec.Command("/bin/sh", "-c", "sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("no se pudo arrancar el proceso: %v", err)
	}
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	pid := cmd.Process.Pid
	// El pidfile dice que el proceso es otro (starttime que no es el suyo):
	// es el caso del pid reciclado.
	_, pidFile := rutaLog("web")
	escribirPidfile(t, pidFile, pid, 1)

	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"logs-stop", "web"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if strings.Contains(salida, "detenido") {
		t.Errorf("salida = %q, no puede decir 'detenido' si no paró nada", salida)
	}
	if !strings.Contains(salida, "sin seguimiento") {
		t.Errorf("salida = %q, quería 'sin seguimiento'", salida)
	}
	// Y el proceso ajeno sigue vivo.
	if !procesoVivo(pid) {
		t.Fatal("logs-stop mató un proceso que no era el seguimiento")
	}
}

// TestLogsStopMataElSeguimientoValido: con el pid y el starttime que
// corresponden, el seguimiento se corta.
func TestLogsStopMataElSeguimientoValido(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cmd := exec.Command("/bin/sh", "-c", "sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("no se pudo arrancar el proceso: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	start, ok := leerStartTime(cmd.Process.Pid)
	if !ok {
		t.Skip("no se pudo leer el starttime del proceso")
	}
	_, pidFile := rutaLog("web")
	escribirPidfile(t, pidFile, cmd.Process.Pid, start)

	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"logs-stop", "web"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if !strings.Contains(salida, "detenido") {
		t.Errorf("salida = %q, quería 'detenido'", salida)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("el pidfile debería haberse borrado: %v", err)
	}
	// El proceso es hijo del test: hay que recogerlo para saber que murió.
	estado := make(chan error, 1)
	go func() { _, err := cmd.Process.Wait(); estado <- err }()
	select {
	case <-estado:
	case <-time.After(3 * time.Second):
		t.Error("el seguimiento sigue vivo")
	}
}

// ── D4 ───────────────────────────────────────────────────────────────────

// TestVolumeExportConRutaRelativaNoMontaLaRaiz es el D4: una ruta
// relativa no puede convertirse en la raíz del host.
func TestVolumeExportConRutaRelativaNoMontaLaRaiz(t *testing.T) {
	g := &runnerGrabador{}
	restaurar := conRunnerGrabador(t, g)
	defer restaurar()
	capturaStdout(t, func() {
		if codigo := Run([]string{"volume-export", "--name", "datos", "--out", "backup.tar.gz"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if len(g.pedidos) != 1 {
		t.Fatalf("pedidos = %v", g.pedidos)
	}
	if strings.Contains(g.pedidos[0], "-v /:/respaldo") {
		t.Errorf("el comando monta la raíz del host: %s", g.pedidos[0])
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.pedidos[0], "-v "+cwd+":/respaldo") {
		t.Errorf("el comando no monta el directorio actual: %s", g.pedidos[0])
	}
	if !strings.Contains(g.pedidos[0], "/respaldo/backup.tar.gz") {
		t.Errorf("el tarball no quedó con su nombre: %s", g.pedidos[0])
	}
}

// TestVolumeExportRechazaRutaFueraDelDirectorio: una ruta relativa que se
// sale del directorio actual se rechaza.
func TestVolumeExportRechazaRutaFueraDelDirectorio(t *testing.T) {
	g := &runnerGrabador{}
	restaurar := conRunnerGrabador(t, g)
	defer restaurar()
	codigo := 0
	capturaStderr(t, func() {
		codigo = Run([]string{"volume-export", "--name", "datos", "--out", "../fuera.tar.gz"})
	})
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if len(g.pedidos) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", g.pedidos)
	}
}

// TestVolumeImportConRutaRelativaNoMontaLaRaiz es el D4 para el import.
func TestVolumeImportConRutaRelativaNoMontaLaRaiz(t *testing.T) {
	g := &runnerGrabador{}
	restaurar := conRunnerGrabador(t, g)
	defer restaurar()
	capturaStdout(t, func() {
		if codigo := Run([]string{"volume-import", "--name", "datos", "--in", "backup.tar.gz"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if len(g.pedidos) != 1 {
		t.Fatalf("pedidos = %v", g.pedidos)
	}
	if strings.Contains(g.pedidos[0], "-v /:/respaldo") {
		t.Errorf("el comando monta la raíz del host: %s", g.pedidos[0])
	}
}

// ── D5 ───────────────────────────────────────────────────────────────────

// TestComposeAceptaElActionDespuesDelProyecto es el D5: el flag puede
// venir en cualquier posición.
func TestComposeAceptaElActionDespuesDelProyecto(t *testing.T) {
	casos := [][]string{
		{"compose", "shop", "--action", "up"},
		{"compose", "--action", "up", "shop"},
		{"compose", "shop", "--action=up"},
	}
	for _, args := range casos {
		g := &runnerGrabador{}
		restaurar := conRunnerGrabador(t, g)
		capturaStdout(t, func() {
			if codigo := Run(args); codigo != 0 {
				t.Errorf("%v: código = %d, quería 0", args, codigo)
			}
		})
		restaurar()
		if len(g.pedidos) != 1 {
			t.Fatalf("%v: pedidos = %v", args, g.pedidos)
		}
		if g.pedidos[0] != "compose -p shop up -d" {
			t.Errorf("%v: comando = %q, quería 'compose -p shop up -d'", args, g.pedidos[0])
		}
	}
}

// TestComposeSinActionUsaRestart: el default sigue siendo restart.
func TestComposeSinActionUsaRestart(t *testing.T) {
	g := &runnerGrabador{}
	restaurar := conRunnerGrabador(t, g)
	defer restaurar()
	capturaStdout(t, func() {
		if codigo := Run([]string{"compose", "shop"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if len(g.pedidos) != 1 || g.pedidos[0] != "compose -p shop restart" {
		t.Errorf("pedidos = %v, quería 'compose -p shop restart'", g.pedidos)
	}
}

// ── D11 ──────────────────────────────────────────────────────────────────

// TestRutaLogNoColisiona es el D11: dos contenedores distintos no pueden
// compartir el archivo de log ni el pidfile.
func TestRutaLogNoColisiona(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a1, p1 := rutaLog("Web")
	a2, p2 := rutaLog("web")
	if a1 == a2 {
		t.Errorf("'Web' y 'web' comparten el log: %s", a1)
	}
	if p1 == p2 {
		t.Errorf("'Web' y 'web' comparten el pidfile: %s", p1)
	}
	largo := strings.Repeat("a", 100)
	l1, _ := rutaLog(largo + "uno")
	l2, _ := rutaLog(largo + "dos")
	if l1 == l2 {
		t.Errorf("los ids largos con el mismo prefijo comparten el log: %s", l1)
	}
}

// ── D12 ──────────────────────────────────────────────────────────────────

// TestIdsConGuionSeRechazan es el D12: un id que empieza con "-" lo
// interpretaría docker como un flag suyo.
func TestIdsConGuionSeRechazan(t *testing.T) {
	casos := [][]string{
		{"exec", "--id", "--privileged", "web", "ls"},
		{"commit", "--id", "--privileged"},
		{"rename", "--id", "--privileged", "--name", "nuevo"},
		{"tag", "--id", "--privileged", "--repo", "repo"},
		{"pause", "--id", "--privileged"},
		{"unpause", "--id", "--privileged"},
		{"diff", "--id", "--privileged"},
		{"export", "--id", "--privileged", "--out", "/tmp/x.tar"},
		{"port", "--privileged"},
		{"pull", "--privileged"},
		{"push", "--privileged"},
		{"save", "--image", "--privileged", "--out", "/tmp/x.tar"},
		{"load", "--in", "--privileged"},
		{"volume-inspect", "--privileged"},
		{"network-inspect", "--privileged"},
	}
	for _, args := range casos {
		g := &runnerGrabador{}
		restaurar := conRunnerGrabador(t, g)
		codigo := 0
		capturaStderr(t, func() {
			codigo = Run(args)
		})
		restaurar()
		if codigo != 1 {
			t.Errorf("%v: código = %d, quería 1", args, codigo)
		}
		if len(g.pedidos) != 0 {
			t.Errorf("%v: no debería haber llamado a docker: %v", args, g.pedidos)
		}
	}
}

// ── D17 ──────────────────────────────────────────────────────────────────

// TestComandoFueraDeLaListaNoSeEjecuta es el D17: la lista de acciones
// permitidas tiene que ser una barrera de verdad.
func TestComandoFueraDeLaListaNoSeEjecuta(t *testing.T) {
	original, existed := docker.AllowedActions["start"]
	delete(docker.AllowedActions, "start")
	defer func() {
		if existed {
			docker.AllowedActions["start"] = original
		}
	}()
	g := &runnerGrabador{}
	restaurar := conRunnerGrabador(t, g)
	defer restaurar()
	codigo := 0
	capturaStderr(t, func() {
		codigo = Run([]string{"start", "web"})
	})
	if codigo == 0 {
		t.Error("una acción fuera de la lista debería fallar")
	}
	if len(g.pedidos) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", g.pedidos)
	}
}

// TestTodasLasAccionesMutantesEstanPermitidas: la lista del paquete tiene
// que incluir todo lo que el CLI puede mutar.
func TestTodasLasAccionesMutantesEstanPermitidas(t *testing.T) {
	for accion := range comandosMutantes {
		if !docker.AllowedActions[accion] {
			t.Errorf("%q es un comando mutante pero no está en AllowedActions", accion)
		}
	}
}

// ── D18 ──────────────────────────────────────────────────────────────────

// TestSacarFlagNoMutaElSliceDelCaller es el D18: `sacarFlag` no puede
// reusar el backing array del slice que recibe.
func TestSacarFlagNoMutaElSliceDelCaller(t *testing.T) {
	entrada := []string{"--json", "web", "--json"}
	copia := append([]string(nil), entrada...)
	resto, ok := sacarFlag(entrada, "--json")
	if !ok {
		t.Fatal("debería encontrar --json")
	}
	if strings.Join(resto, ",") != "web" {
		t.Errorf("resto = %v, quería [web]", resto)
	}
	if strings.Join(entrada, ",") != strings.Join(copia, ",") {
		t.Errorf("el slice de entrada cambió: %v (era %v)", entrada, copia)
	}
}

// ── D19 ──────────────────────────────────────────────────────────────────

// TestLogsStopConProcesoMuertoNoDiceDetenido es el D19: sin un proceso
// vivo no se puede decir que se detuvo algo.
func TestLogsStopConProcesoMuertoNoDiceDetenido(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	_, pidFile := rutaLog("web")
	if err := os.WriteFile(pidFile, []byte("999999 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	salida := capturaStdout(t, func() {
		if codigo := Run([]string{"logs-stop", "web"}); codigo != 0 {
			t.Errorf("código = %d, quería 0", codigo)
		}
	})
	if strings.Contains(salida, "detenido") {
		t.Errorf("salida = %q, no puede decir 'detenido' si no había proceso", salida)
	}
	if !strings.Contains(salida, "sin seguimiento") {
		t.Errorf("salida = %q, quería 'sin seguimiento'", salida)
	}
}

// ── D20 ──────────────────────────────────────────────────────────────────

// TestLogsStreamConContenedorInexistenteFalla es el D20: si docker falla
// (el contenedor no existe), el comando tiene que fallar y no dejar un
// pidfile de un proceso muerto.
func TestLogsStreamConContenedorInexistenteFalla(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dockerFalso(t, "#!/bin/sh\necho 'No such container: nope' >&2\nexit 1\n")
	codigo := 0
	salida := capturaStderr(t, func() {
		codigo = Run([]string{"logs-stream", "nope"})
	})
	if codigo == 0 {
		t.Error("con un contenedor inexistente el comando debería fallar")
	}
	if !strings.Contains(salida, "No such container") {
		t.Errorf("stderr = %q, quería el error de docker", salida)
	}
	_, pidFile := rutaLog("nope")
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("no debería quedar un pidfile de un proceso muerto: %v", err)
	}
}
