package docker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── D1 / D13 / D14 ───────────────────────────────────────────────────────

// TestInspectLeeElHealthDeState es el D1: `docker inspect` pone el health
// en State.Health, no en Config.Health. Con el parser viejo el health
// salía siempre vacío.
func TestInspectLeeElHealthDeState(t *testing.T) {
	payload := `{"Id":"abc123","Name":"/web","State":{"Status":"running","Health":{"Status":"unhealthy","Log":[{"Output":"fallo 1"},{"Output":"fallo 2"}]}},"Config":{"Image":"nginx"}}`
	f := nuevoFake().con("inspect --format {{json .}} web", payload)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if info.Health != "unhealthy" {
		t.Errorf("Health = %q, quería unhealthy (viene de State.Health)", info.Health)
	}
	if len(info.HealthLog) != 2 {
		t.Errorf("HealthLog = %v, quería las dos entradas", info.HealthLog)
	}
}

// TestInspectIgnoraElHealthDeConfig documenta que el health ya no se busca
// en Config: ahí no está.
func TestInspectIgnoraElHealthDeConfig(t *testing.T) {
	payload := `{"Id":"abc123","Name":"/web","State":{"Status":"running"},"Config":{"Image":"nginx","Health":{"Status":"healthy","Log":[{"Output":"ok"}]}}}`
	f := nuevoFake().con("inspect --format {{json .}} web", payload)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if info.Health != "" {
		t.Errorf("Health = %q, quería vacío (docker no manda el health en Config)", info.Health)
	}
}

// TestHealthLogTomaLasUltimasCinco es el D13: el log va de más antiguo a
// más reciente, así que se conservan las últimas cinco entradas.
func TestHealthLogTomaLasUltimasCinco(t *testing.T) {
	entradas := make([]string, 0, 8)
	for i := 1; i <= 8; i++ {
		entradas = append(entradas, `{"Output":"linea `+string(rune('0'+i))+`"}`)
	}
	payload := `{"Id":"x","Name":"/c","State":{"Status":"running","Health":{"Status":"healthy","Log":[` +
		strings.Join(entradas, ",") + `]}}}`
	f := nuevoFake().con("inspect --format {{json .}} c", payload)
	info, err := InspectContainer(context.Background(), f.run, "c")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if len(info.HealthLog) != 5 {
		t.Fatalf("HealthLog tiene %d entradas, quería 5", len(info.HealthLog))
	}
	quiere := []string{"linea 4", "linea 5", "linea 6", "linea 7", "linea 8"}
	if strings.Join(info.HealthLog, ",") != strings.Join(quiere, ",") {
		t.Errorf("HealthLog = %v, quería %v", info.HealthLog, quiere)
	}
}

// TestListasDelSnapshotNoSonNull es el D14: en el JSON del plugin ninguna
// lista puede salir `null`.
func TestListasDelSnapshotNoSonNull(t *testing.T) {
	f := nuevoFake().
		con("ps --no-trunc --format {{json .}}", `{"ID":"c1","Names":"web","State":"running","Status":"Up 1 hour"}`+"\n").
		con("images --format {{json .}}", "").
		con("volume ls --format {{json .}}", "").
		con("network ls --format {{json .}}", "")
	snap := Collect(context.Background(), f.run, false)
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, clave := range []string{"containers", "images", "volumes", "networks", "projects"} {
		if strings.Contains(string(data), `"`+clave+`":null`) {
			t.Errorf("%q salió null en %s", clave, data)
		}
	}
	if !strings.Contains(string(data), `"ports":[]`) {
		t.Errorf("ports de un contenedor sin puertosxitió null o no vacío: %s", data)
	}
}

// TestStatsYDFNoSonNull es el D14 para `stats` y `df`.
func TestStatsYDFNoSonNull(t *testing.T) {
	f := nuevoFake().
		con("stats --no-stream --format {{json .}}", "").
		con("system df --format {{json .}}", "").
		con("system df", "total")
	stats, err := Stats(context.Background(), f.run)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	data, _ := json.Marshal(map[string]any{"stats": stats})
	if strings.Contains(string(data), `"stats":null`) {
		t.Errorf("stats salió null: %s", data)
	}
	filas, _, err := DiskUsage(context.Background(), f.run)
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	data, _ = json.Marshal(map[string]any{"rows": filas})
	if strings.Contains(string(data), `"rows":null`) {
		t.Errorf("rows salió null: %s", data)
	}
}

// TestInspectNoTieneListasNull es el D14 para el detalle de un contenedor.
func TestInspectNoTieneListasNull(t *testing.T) {
	payload := `{"Id":"abc123","Name":"/web","State":{"Status":"running"},"Config":{"Image":"nginx"}}`
	f := nuevoFake().con("inspect --format {{json .}} web", payload)
	info, err := InspectContainer(context.Background(), f.run, "web")
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, clave := range []string{"env", "mounts", "healthLog"} {
		if strings.Contains(string(data), `"`+clave+`":null`) {
			t.Errorf("%q salió null en %s", clave, data)
		}
	}
	if !strings.Contains(string(data), `"labels":{}`) {
		t.Errorf("labels salió null o vacío: %s", data)
	}
}

// ── D6 / D7 ──────────────────────────────────────────────────────────────

// dockerEnPath escribe un script "docker" en un directorio temporal y lo
// pone al principio del PATH. El binario real nunca se ejecuta.
func dockerEnPath(t *testing.T, cuerpo string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(cuerpo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestExecSinBinarioDevuelveElErrorReal es el D6: sin docker en el PATH el
// error no puede ser la cadena vacía.
func TestExecSinBinarioDevuelveElErrorReal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	salida, err := Exec(context.Background(), "ps")
	if err == nil {
		t.Fatal("sin docker en el PATH debería fallar")
	}
	if err.Error() == "" {
		t.Fatal("el error no puede ser la cadena vacía: se pierde el diagnóstico")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("el error debería nombrar al binario: %q", err.Error())
	}
	if salida != "" {
		t.Errorf("salida = %q, quería vacía", salida)
	}
}

// TestExecTomaElErrorDeStderr es el D7: el mensaje del error sale de
// stderr, no de la primera línea de stdout (que es una línea de progreso).
func TestExecTomaElErrorDeStderr(t *testing.T) {
	dockerEnPath(t, "#!/bin/sh\necho 'Building step 1/3'\necho 'ERROR: no space left on device' >&2\nexit 1\n")
	_, err := Exec(context.Background(), "build", ".")
	if err == nil {
		t.Fatal("el comando debería fallar")
	}
	if !strings.Contains(err.Error(), "no space left") {
		t.Errorf("el error debería ser el de stderr: %q", err.Error())
	}
	if strings.Contains(err.Error(), "Building step") {
		t.Errorf("el error tomó una línea de stdout: %q", err.Error())
	}
}

// TestGetInfoAguantaUnWarningPorStderr es el D7 aplicado al parseo del
// JSON: un warning de docker por stderr no puede romper el unmarshal.
func TestGetInfoAguantaUnWarningPorStderr(t *testing.T) {
	dockerEnPath(t, "#!/bin/sh\necho 'WARNING: bridge-nf-call-iptables is disabled' >&2\necho '{\"ServerVersion\":\"27.1.1\",\"NCPU\":8}'\n")
	info, err := GetInfo(context.Background(), Exec)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.ServerVersion != "27.1.1" || info.NCPU != 8 {
		t.Errorf("info mal: %+v", info)
	}
}

// ── D8 ───────────────────────────────────────────────────────────────────

// TestRunImageRespetaDetach es el D8: `Detach` decide si el contenedor
// queda en segundo plano. La condición era `opts.Detach || true`.
func TestRunImageRespetaDetach(t *testing.T) {
	f := nuevoFake().con("run nginx", "abc123")
	if _, err := RunImage(context.Background(), f.run, RunOptions{Image: "nginx", Detach: false}); err != nil {
		t.Fatalf("RunImage: %v", err)
	}
	if f.llamado("run -d nginx") {
		t.Error("con Detach:false no debería llevar -d")
	}
	if !f.llamado("run nginx") {
		t.Errorf("llamó %v", f.pedidos())
	}
}

// TestRunImageConDetachPoneElDashD es el otro lado del D8.
func TestRunImageConDetachPoneElDashD(t *testing.T) {
	f := nuevoFake().con("run -d nginx", "abc123")
	if _, err := RunImage(context.Background(), f.run, RunOptions{Image: "nginx", Detach: true}); err != nil {
		t.Fatalf("RunImage: %v", err)
	}
	if !f.llamado("run -d nginx") {
		t.Errorf("llamó %v, quería 'run -d nginx'", f.pedidos())
	}
}

// ── D9 / D10 ─────────────────────────────────────────────────────────────

// TestEventsPropagaElErrorConSalidaParcial es el D9: si docker falla
// después de escribir algo, los eventos son parciales y hay que avisar.
func TestEventsPropagaElErrorConSalidaParcial(t *testing.T) {
	clave := "events --since 6h --until 0s --filter type=container --format {{json .}}"
	f := nuevoFake().falla(clave,
		`{"Type":"container","Action":"start","Actor":{"ID":"abc","Attributes":{"name":"web"}}}`,
		errors.New("daemon caido"))
	eventos, err := Events(context.Background(), f.run, "")
	if err == nil {
		t.Fatal("el error de docker debería volver aunque haya salida parcial")
	}
	if len(eventos) != 0 {
		t.Errorf("con error no se devuelven eventos parciales: %+v", eventos)
	}
}

// TestDiskUsagePropagaElErrorDelTotal es el D10: el segundo `system df`
// (el del total) también puede fallar y hay que avisar.
func TestDiskUsagePropagaElErrorDelTotal(t *testing.T) {
	f := nuevoFake().
		con("system df --format {{json .}}", `{"Type":"Images","TotalCount":"12","Active":"3"}`).
		falla("system df", "", errors.New("daemon caido"))
	filas, total, err := DiskUsage(context.Background(), f.run)
	if err == nil {
		t.Fatal("el error del total debería volver")
	}
	if total != "" {
		t.Errorf("total = %q, quería vacío", total)
	}
	if len(filas) != 1 {
		t.Errorf("filas = %+v", filas)
	}
}

// ── D12 (capa del paquete) ───────────────────────────────────────────────

// TestExecInContainerRechazaIdConGuion es el D12 en la capa del paquete: un
// id que empieza con "-" lo tomaría docker como un flag suyo.
func TestExecInContainerRechazaIdConGuion(t *testing.T) {
	f := nuevoFake()
	_, err := ExecInContainer(context.Background(), f.run, "--privileged", []string{"ls"})
	if err == nil {
		t.Fatal("un id que empieza con '-' debería rechazarse")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

// ── D15 ──────────────────────────────────────────────────────────────────

// TestHealthStartingEsReconocido es el D15: docker escribe el estado de
// arranque como "(health: starting)", no como "(starting)".
func TestHealthStartingEsReconocido(t *testing.T) {
	if got := healthFromStatus("Up 3 hours (health: starting)", ""); got != "starting" {
		t.Errorf("healthFromStatus = %q, quería starting", got)
	}
	if got := healthFromStatus("Up 3 hours (health: unhealthy)", ""); got != "unhealthy" {
		t.Errorf("healthFromStatus = %q, quería unhealthy", got)
	}
}

// ── D16 ──────────────────────────────────────────────────────────────────

// TestGroupProjectsNoRepiteServicios es el D16: un servicio escalado
// (`--scale web=2`) aparece en varios contenedores con la misma label.
func TestGroupProjectsNoRepiteServicios(t *testing.T) {
	contenedores := []Container{
		{ID: "1", Name: "web-1", Project: "shop", Service: "web", State: "running"},
		{ID: "2", Name: "web-2", Project: "shop", Service: "web", State: "running"},
		{ID: "3", Name: "db-1", Project: "shop", Service: "db", State: "running"},
	}
	proyectos := GroupProjects(contenedores)
	if len(proyectos) != 1 {
		t.Fatalf("esperaba 1 proyecto, obtuve %d", len(proyectos))
	}
	if strings.Join(proyectos[0].Services, ",") != "db,web" {
		t.Errorf("Services = %v, quería db,web (sin repetir)", proyectos[0].Services)
	}
	if proyectos[0].Containers != 3 {
		t.Errorf("Containers = %d, quería 3", proyectos[0].Containers)
	}
}

// ── D21 ──────────────────────────────────────────────────────────────────

// TestExecResultExitPropagaElCodigo es el D21: el código de salida real
// del comando dentro del contenedor tiene que viajar en `Exit`.
func TestExecResultPropagaElCodigoDeSalida(t *testing.T) {
	f := nuevoFake().falla("exec web false", "", errors.New("exit status 1"))
	res, err := ExecInContainer(context.Background(), f.run, "web", []string{"false"})
	if err == nil {
		t.Fatal("el error debería volver")
	}
	if res.Exit != 1 {
		t.Errorf("Exit = %d, quería 1", res.Exit)
	}
	if res.Running {
		t.Error("no debería marcar que está corriendo")
	}
}

// TestExecResultExitEnElErrorDelDaemon: si no se conoce el código, el
// campo no puede decir 0 (que significa "terminó bien").
func TestExecResultExitEnElErrorDelDaemon(t *testing.T) {
	f := nuevoFake().falla("exec web ls", "", errors.New("container is not running"))
	res, err := ExecInContainer(context.Background(), f.run, "web", []string{"ls"})
	if err == nil {
		t.Fatal("el error debería volver")
	}
	if res.Exit == 0 {
		t.Error("Exit = 0 en un comando que falló: no puede decir que terminó bien")
	}
}

// ── D17 ──────────────────────────────────────────────────────────────────

// TestAllowedActionsCubreLasAccionesMutantes es el D17: la lista tiene que
// incluir todo lo que el CLI puede mutar. El CLI la consulta antes de
// ejecutar (ver cli/fixes_test.go).
func TestAllowedActionsIncluyeLasAccionesDelPaquete(t *testing.T) {
	for _, accion := range []string{
		"start", "stop", "restart", "remove", "rmi", "volume-rm", "network-rm",
		"run", "pause", "unpause", "pull", "push", "volume-create",
		"network-create", "save", "load", "export", "diff", "build",
	} {
		if !AllowedActions[accion] {
			t.Errorf("%q debería estar permitido", accion)
		}
	}
	if AllowedActions["inventado"] {
		t.Error("una acción que no existe no debería estar permitida")
	}
}

// ── D12 (capa del paquete): ninguna acción pasa un id con guion a docker ──

// TestLasAccionesRechazanIdsConGuion es el D12 en la capa del paquete: un id
// que empieza con "-" lo interpretaría docker como un flag suyo.
func TestLasAccionesRechazanIdsConGuion(t *testing.T) {
	f := nuevoFake()
	acciones := []struct {
		nombre  string
		llamar  func(Runner) (Action, error)
	}{
		{"start", func(r Runner) (Action, error) { return Start(context.Background(), r, "--privileged") }},
		{"stop", func(r Runner) (Action, error) { return Stop(context.Background(), r, "--privileged") }},
		{"restart", func(r Runner) (Action, error) { return Restart(context.Background(), r, "--privileged") }},
		{"remove", func(r Runner) (Action, error) { return Remove(context.Background(), r, "--privileged") }},
		{"rmi", func(r Runner) (Action, error) { return RemoveImage(context.Background(), r, "--privileged") }},
		{"volume-rm", func(r Runner) (Action, error) { return RemoveVolume(context.Background(), r, "--privileged") }},
		{"network-rm", func(r Runner) (Action, error) { return RemoveNetwork(context.Background(), r, "--privileged") }},
		{"pull", func(r Runner) (Action, error) { return Pull(context.Background(), r, "--privileged") }},
		{"push", func(r Runner) (Action, error) { return Push(context.Background(), r, "--privileged") }},
		{"volume-create", func(r Runner) (Action, error) { return CreateVolume(context.Background(), r, "--privileged") }},
		{"network-create", func(r Runner) (Action, error) { return CreateNetwork(context.Background(), r, "--privileged") }},
		{"save", func(r Runner) (Action, error) { return SaveImage(context.Background(), r, "--privileged", "/tmp/x.tar") }},
		{"load", func(r Runner) (Action, error) { return LoadImage(context.Background(), r, "--privileged") }},
		{"export", func(r Runner) (Action, error) { return ExportContainer(context.Background(), r, "--privileged", "/tmp/x.tar") }},
		{"diff", func(r Runner) (Action, error) { return Diff(context.Background(), r, "--privileged") }},
		{"pause", func(r Runner) (Action, error) { return Pause(context.Background(), r, "--privileged") }},
		{"unpause", func(r Runner) (Action, error) { return Unpause(context.Background(), r, "--privileged") }},
		{"rename", func(r Runner) (Action, error) { return Rename(context.Background(), r, "--privileged", "nuevo") }},
		{"tag", func(r Runner) (Action, error) { return Tag(context.Background(), r, "--privileged", "repo", "1") }},
		{"top", func(r Runner) (Action, error) { return Top(context.Background(), r, "--privileged") }},
		{"history", func(r Runner) (Action, error) { return History(context.Background(), r, "--privileged") }},
		{"commit", func(r Runner) (Action, error) { return Commit(context.Background(), r, "--privileged", "repo", "1", "") }},
	}
	for _, a := range acciones {
		res, err := a.llamar(f.run)
		if err == nil {
			t.Errorf("%s: un id con guion debería rechazarse", a.nombre)
		}
		if res.OK {
			t.Errorf("%s: no debería estar OK", a.nombre)
		}
		if res.Output == "" {
			t.Errorf("%s: el motivo debería estar en Output", a.nombre)
		}
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}

// TestCopyRechazaIdConGuion: `cp` también.
func TestCopyRechazaIdConGuion(t *testing.T) {
	f := nuevoFake()
	if _, err := Copy(context.Background(), f.run, "--privileged", "origen", "/destino"); err == nil {
		t.Error("cp con un id con guion debería rechazarse")
	}
	if _, err := CopyFrom(context.Background(), f.run, "--privileged", "/origen", "/destino"); err == nil {
		t.Error("cp con un id con guion debería rechazarse")
	}
	if len(f.pedidos()) != 0 {
		t.Errorf("no debería haber llamado a docker: %v", f.pedidos())
	}
}
