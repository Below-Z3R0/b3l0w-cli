// Package dockercli arma los comandos de `b3l0w docker`.
package dockercli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker"
)

// runner es el ejecutor de docker. En producción es docker.Exec; los tests
// lo reemplazan por una func falsa para no tocar el daemon real.
var runner docker.Runner = docker.Exec

// usage documenta los subcomandos que expone `b3l0w docker`.
const usage = `b3l0w docker — contenedores, imágenes, volúmenes y redes

uso: b3l0w docker <comando> [opciones]

  snapshot [--all] [--json]        todo el estado de una pasada
  start|stop|restart <id>          control de un contenedor
  remove <id>                      borra un contenedor (-f)
  rmi <id>                         borra una imagen
  volume-rm <nombre>               borra un volumen
  network-rm <nombre>              borra una red (no las nativas)
  run --image IMG [--name N] [--network NET] [--port P] [--env K=V]
  logs <id> [--tail N]             últimas líneas
  logs-stream <id>                 sigue el log en 2º plano a un archivo
  logs-stop <id>                   corta el seguimiento
  compose-file <proyecto>          el YAML del stack
  stats [--json]                   CPU/RAM/IO por contenedor (una muestra)
  inspect <id> [--json]            env, labels, montajes, redes, health
  events [--since 6h] [--json]       últimos eventos de contenedor
  compose <proyecto> [--action A]    up|down|restart|stop|pull de un stack
  info [--json]                    CPUs, RAM, versión, driver, conteos
  df [--json]                      espacio usado y recuperable
  prune <destino> [--json]         containers|images|volumes|builder|system
  exec --id ID CMD... [--json]      corre un comando en el contenedor
  cp --from ID RUTA --to RUTA       copia archivos ("id:" = dentro del contenedor)
  commit --id ID [--repo R] [--tag T] [--message M]   crea una imagen
  pause|unpause --id ID             congela / reanuda un contenedor
  rename --id ID --name N           renombra un contenedor
  tag --id ID --repo R [--tag T]     etiqueta una imagen
  top <id> [--json]                 procesos del contenedor
  history <id> [--json]             historial de capas de una imagen

  pull <imagen>                   descarga una imagen del registro
  push <imagen>                   sube una imagen al registro
  search <término> [--limit N]    busca imágenes en Docker Hub
  volume-create <nombre>          crea un volumen
  volume-inspect <nombre>         detalle de un volumen (json de docker)
  volume-export --name V --out F  respalda un volumen a un tar.gz
  volume-import --name V --in F   restaura un tar.gz en un volumen
  network-create <nombre>         crea una red
  network-inspect <nombre>        detalle de una red (json de docker)
  port <id>                       puertos publicados (0.0.0.0:8080->80/tcp)
  update --id ID [--cpus N] [--memory M]   límites de CPU y memoria
  logs-follow <id> [--tail N]     sigue los logs en vivo (Ctrl-C para salir)
  save --image I --out F          exporta una imagen a un tar
  load --in F                     importa una imagen desde un tar
  export --id C --out F           exporta el filesystem de un contenedor
  diff --id C                     muestra los cambios del filesystem
  build --tag IMG [--file F] [--context DIR]  construye una imagen
`

// Run despacha un subcomando de `b3l0w docker` y devuelve el código de salida:
// 0 correcto, 1 error de uso o de docker. Sin argumentos imprime la ayuda.
// comandosMutantes son los subcomandos que cambian algo. Cada uno tiene que
// estar en docker.AllowedActions: el CLI consulta la lista y lo que no está
// tiene que fallar en vez de ejecutar algo no previsto (D17).
var comandosMutantes = map[string]bool{
	"start": true, "stop": true, "restart": true, "remove": true,
	"rmi": true, "volume-rm": true, "network-rm": true, "run": true,
	"pause": true, "unpause": true, "pull": true, "push": true,
	"volume-create": true, "network-create": true,
	"save": true, "load": true, "export": true, "diff": true,
	"build": true, "commit": true, "rename": true, "tag": true,
	"cp": true, "exec": true, "update": true, "prune": true,
	"volume-export": true, "volume-import": true, "compose": true,
}

func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
		return 2
	}
	if comandosMutantes[args[0]] && !docker.AllowedActions[args[0]] {
		fmt.Fprintf(os.Stderr, "acción no permitida: %s\n", args[0])
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "snapshot":
		return snapshot(args[1:])
	case "start", "stop", "restart", "remove":
		return control(args[0], args[1:])
	case "rmi", "volume-rm", "network-rm":
		return removeResource(args[0], args[1:])
	case "run":
		return runImage(args[1:])
	case "logs":
		return logs(args[1:])
	case "stats":
		return statsCmd(args[1:])
	case "events":
		return eventsCmd(args[1:])
	case "compose":
		return composeCmd(args[1:])
	case "info":
		return infoCmd(args[1:])
	case "inspect":
		return inspectCmd(args[1:])
	case "df":
		return dfCmd(args[1:])
	case "exec":
		return execCmd(args[1:])
	case "cp":
		return copyCmd(args[1:])
	case "commit":
		return commitCmd(args[1:])
	case "pause":
		return pauseCmd(args, true)
	case "unpause":
		return pauseCmd(args, false)
	case "rename":
		return renameCmd(args[1:])
	case "tag":
		return tagCmd(args[1:])
	case "top":
		return topCmd(args[1:])
	case "history":
		return historyCmd(args[1:])
	case "prune":
		return pruneCmd(args[1:])
	case "pull":
		return pullCmd(args[1:])
	case "push":
		return pushCmd(args[1:])
	case "volume-create":
		return createCmd(args[1:], "volume")
	case "network-create":
		return createCmd(args[1:], "network")
	case "save":
		return saveCmd(args[1:])
	case "load":
		return loadCmd(args[1:])
	case "export":
		return exportCmd(args[1:])
	case "diff":
		return diffCmd(args[1:])
	case "build":
		return buildCmd(args[1:])
	case "port":
		return portCmd(args[1:])
	case "search":
		return searchCmd(args[1:])
	case "update":
		return updateCmd(args[1:])
	case "volume-inspect":
		return volumeInspectCmd(args[1:])
	case "network-inspect":
		return networkInspectCmd(args[1:])
	case "volume-export":
		return volumeExportCmd(args[1:])
	case "volume-import":
		return volumeImportCmd(args[1:])
	case "logs-follow":
		return logsFollowCmd(args[1:])
	case "logs-stream":
		return logsStreamCmd(args[1:])
	case "logs-stop":
		return logsStopCmd(args[1:])
	case "compose-file":
		return composeFileCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", args[0])
		fmt.Print(usage)
		return 2
	}
}

// sacarFlag quita un flag booleano de la lista: Go deja de parsear flags en el
// primer argumento posicional, así que `inspect <id> --json` perdía el --json
// y respondía en texto.
// extraerValor saca `--flag valor` de cualquier posición: Go deja de parsear
// flags en el primer posicional, así que `logs <id> --tail 120` perdía el tail
// y devolvía el log entero (el panel publicaba 200 líneas y no llegaban).
// ── logs en vivo ───────────────────────────────────────────────────────
//
// Noctalia NO tiene timers ni exec asíncrono en Lua, así que un `--follow`
// no se puede mantener desde el plugin. El seguimiento lo hace este
// proceso: arranca `docker logs -f` desacoplado (setsid) y lo va escribiendo
// a un archivo; el panel lee ese archivo y se refresca con `watch`.

func dirLogs() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp"
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "noctalia", "docker-logs")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// rutaLog devuelve el archivo y el pid de un contenedor.
//
// El nombre es el id saneado (minúsculas, lo que no es alfanumérico a "_")
// más un hash corto del id original. El hash es lo que evita que dos
// contenedores distintos compartan el archivo de log y el pidfile: el
// saneo por sí solo no alcanza, porque "Web" y "web" son dos contenedores
// válidos y distintos en docker y los dos daban "web.log" (D11).
func rutaLog(id string) (string, string) {
	seguro := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '_'
	}, id)
	if len(seguro) > 32 {
		seguro = seguro[:32]
	}
	suma := sha256.Sum256([]byte(id))
	nombre := seguro + "-" + hex.EncodeToString(suma[:4])
	dir := dirLogs()
	return filepath.Join(dir, nombre+".log"), filepath.Join(dir, nombre+".pid")
}

// logsStreamCmd sigue el log de un contenedor en segundo plano.
//
// El comando se arma con exec.Command y los argumentos separados: el id
// nunca pasa por un shell (un id con `$(...)` ejecutaba comandos, D2). La
// redirección al archivo la hace Go con un *os.File.
func logsStreamCmd(args []string) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "uso: logs-stream <contenedor>")
		return 2
	}
	id := args[0]
	archivo, pidFile := rutaLog(id)
	// Si ya hay un seguimiento vivo no se duplica. El pid del pidfile se
	// valida contra el /proc: si el número fue reciclado, no es nuestro.
	if pid, start := leerSeguimiento(pidFile); pid > 0 && esSeguimiento(pid, start) {
		// Ya está corriendo: se devuelve la ruta para que el panel la lea.
		fmt.Println(archivo)
		return 0
	}
	// Un pidfile viejo (de un proceso que ya no es el nuestro) se borra: si
	// no, el `logs-stop` podría matar a un proceso ajeno (D3).
	borrarPidfile(pidFile)
	// El archivo lo abre Go y el hijo escribe en él: el id nunca pasa por
	// un shell (un id con `$(...)` ejecutaba comandos, D2).
	salida, err := os.OpenFile(archivo, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no pude crear %s: %v\n", archivo, err)
		return 1
	}
	// `setsid`: el proceso sobrevive a la salida del CLI. Sin el `sh -c` el
	// pid guardado es el de docker, así que `logs-stop` corta el seguimiento
	// y no un shell intermediario.
	cmd := exec.Command("docker", "logs", "-f", "--tail", "200", id)
	cmd.Stdout = salida
	cmd.Stderr = salida
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = salida.Close()
		fmt.Fprintf(os.Stderr, "no pude seguir el log: %v\n", err)
		return 1
	}
	// El padre no escribe más: el hijo tiene su propia copia del archivo.
	_ = salida.Close()
	pid := cmd.Process.Pid
	// El Wait corre en una goroutine: si el proceso muere enseguida, docker
	// falló (por ejemplo, el contenedor no existe) y el error quedó escrito
	// en el archivo de log. Antes el comando devolvía 0 y dejaba el pidfile
	// de un proceso muerto (D20).
	terminado := make(chan error, 1)
	go func() { terminado <- cmd.Wait() }()
	select {
	case <-terminado:
		detalle, _ := os.ReadFile(archivo)
		msg := strings.TrimSpace(string(detalle))
		if msg == "" {
			msg = "docker terminó enseguida"
		}
		fmt.Fprintf(os.Stderr, "no pude seguir el log de %s: %s\n", id, msg)
		return 1
	case <-time.After(400 * time.Millisecond):
	}
	// El pidfile lleva el pid y el starttime del proceso: con sólo el pid no
	// se puede saber si el número fue reciclado (D3).
	contenido := strconv.Itoa(pid)
	if start, ok := leerStartTime(pid); ok {
		contenido = fmt.Sprintf("%d %d", pid, start)
	}
	if err := os.WriteFile(pidFile, []byte(contenido), 0o644); err != nil {
		// Sin pid guardado nadie puede parar el `docker logs -f`: el
		// proceso quedaría huérfano escribiendo para siempre. Se mata el
		// grupo entero (setsid lo dejó como líder) y se espera a que
		// muera, así que no queda ni vivo ni zombi. El SIGKILL va también
		// al grupo: si `docker` ignorara el SIGTERM, el Wait de abajo se
		// quedaría esperando para siempre.
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		// Se espera al Wait de la goroutine: el proceso tiene que estar
		// muerto y recogido, no zombi.
		<-terminado
		// Un pid a medias (una escritura a medias) apuntaría a otro
		// proceso y `logs-stop` mataría al equivocado. Sólo se borra si
		// es un archivo común: un directorio no se toca.
		borrarPidfile(pidFile)
		fmt.Fprintf(os.Stderr, "no pude guardar el pidfile %s: %v\n", pidFile, err)
		return 1
	}
	// El padre no espera al hijo: queda huérfano a propósito. El Wait de
	// la goroutine lo recoge cuando termine.
	// Sólo la ruta: el service la publica y el panel la lee con readFile.
	fmt.Println(archivo)
	return 0
}

func logsStopCmd(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: logs-stop <contenedor>")
		return 2
	}
	_, pidFile := rutaLog(args[0])
	pid, start := leerSeguimiento(pidFile)
	if pid <= 0 {
		fmt.Println("sin seguimiento")
		return 0
	}
	// Antes de mandar una señal se comprueba que el proceso sea realmente el
	// seguimiento: un pid reciclado es otro proceso del usuario y matarlo a
	// él no es lo que se pidió (D3). Sin identidad verificable no se mata
	// nada y el pidfile viejo se borra.
	if !esSeguimiento(pid, start) {
		borrarPidfile(pidFile)
		fmt.Println("sin seguimiento")
		return 0
	}
	// Es líder de sesión (setsid), así que el pid es también el grupo:
	// se manda la señal al grupo entero para no dejar hijos sueltos.
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
	borrarPidfile(pidFile)
	fmt.Println("detenido")
	return 0
}

// borrarPidfile borra el pidfile. Sólo si es un archivo común: un directorio
// no se toca (puede ser el marcador de un fallo del WriteFile).
func borrarPidfile(archivo string) {
    if info, err := os.Stat(archivo); err == nil && info.Mode().IsRegular() {
        _ = os.Remove(archivo)
    }
}

// leerSeguimiento lee el pidfile: el pid y, si está, el starttime del
// proceso con el que se arrancó.
func leerSeguimiento(archivo string) (int, int) {
	datos, err := os.ReadFile(archivo)
	if err != nil {
		return 0, 0
	}
	campos := strings.Fields(string(datos))
	if len(campos) == 0 {
		return 0, 0
	}
	pid, err := strconv.Atoi(campos[0])
	if err != nil {
		return 0, 0
	}
	start := 0
	if len(campos) > 1 {
		if n, err := strconv.Atoi(campos[1]); err == nil {
			start = n
		}
	}
	return pid, start
}

// leerPid devuelve el pid del pidfile (0 si no hay o no es un número).
func leerPid(archivo string) int {
	pid, _ := leerSeguimiento(archivo)
	return pid
}

// camposStat devuelve los campos de /proc/<pid>/stat que siguen al nombre
// del proceso (que va entre paréntesis y puede tener espacios).
func camposStat(pid int) ([]string, bool) {
	datos, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return nil, false
	}
	fin := strings.LastIndex(string(datos), ")")
	if fin < 0 {
		return nil, false
	}
	return strings.Fields(string(datos)[fin+1:]), true
}

// leerStartTime devuelve el starttime del proceso (campo 22 de
// /proc/<pid>/stat): los ticks desde el arranque del sistema. Cada proceso
// tiene el suyo, así que dos procesos nunca comparten el valor.
func leerStartTime(pid int) (int, bool) {
	campos, ok := camposStat(pid)
	if !ok || len(campos) < 20 {
		return 0, false
	}
	// campos[0] es el estado (campo 3 del stat), así que el starttime
	// (campo 22) es el índice 19.
	start, err := strconv.Atoi(campos[19])
	if err != nil {
		return 0, false
	}
	return start, true
}

// esSeguimiento dice si el proceso es el `docker logs -f` que arrancó este
// CLI. Se comparan el pid y el starttime: un pid reciclado es otro proceso
// con el mismo número, y mandarle la señal mataría a un proceso ajeno (D3).
func esSeguimiento(pid int, start int) bool {
	if pid <= 0 {
		return false
	}
	actual, ok := leerStartTime(pid)
	if !ok {
		return false
	}
	if start > 0 {
		return actual == start
	}
	// Un pidfile viejo (sin starttime) no se puede validar por número: se
	// mira la línea de comandos, que tiene que ser un `docker logs`.
	return lineaDeComando(pid) != ""
}

// lineaDeComando devuelve la línea de comandos del proceso si es un
// seguimiento de logs, o "" si no se puede leer o no es uno.
func lineaDeComando(pid int) string {
	datos, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	texto := strings.ReplaceAll(string(datos), "\x00", " ")
	if !strings.Contains(texto, "logs") {
		return ""
	}
	return strings.TrimSpace(texto)
}

func procesoVivo(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ── compose ────────────────────────────────────────────────────────────

// composeFileCmd devuelve el YAML de un stack. `docker compose ls` da los
// archivos de configuración; de ahí se lee el principal.
func composeFileCmd(args []string) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "uso: compose-file <proyecto>")
		return 2
	}
	nombre := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := docker.RunCommand(ctx, runner, "compose", []string{"ls", "--all", "--format", "json"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "docker compose ls: %v\n", err)
		return 1
	}
	if !res.OK {
		fmt.Fprint(os.Stderr, res.Output)
		return 1
	}
	var proyectos []struct {
		Name        string `json:"Name"`
		Status      string `json:"Status"`
		ConfigFiles string `json:"ConfigFiles"`
	}
	if err := json.Unmarshal([]byte(res.Output), &proyectos); err != nil {
		fmt.Fprintf(os.Stderr, "salida inesperada: %v\n", err)
		return 1
	}
	for _, proyecto := range proyectos {
		if !strings.EqualFold(proyecto.Name, nombre) {
			continue
		}
		archivos := strings.Split(proyecto.ConfigFiles, ",")
		ruta := strings.TrimSpace(archivos[0])
		contenido, err := os.ReadFile(ruta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "no pude leer %s: %v\n", ruta, err)
			return 1
		}
		fmt.Printf("# %s\n%s", ruta, string(contenido))
		return 0
	}
	fmt.Fprintf(os.Stderr, "no encontré el stack %q\n", nombre)
	return 1
}

func extraerValor(args []string, nombre string) ([]string, string, bool) {
	resto := make([]string, 0, len(args))
	valor := ""
	encontrado := false
	for i := 0; i < len(args); i++ {
		if args[i] == nombre && i+1 < len(args) {
			valor = args[i+1]
			encontrado = true
			i++
			continue
		}
		if v, ok := strings.CutPrefix(args[i], nombre+"="); ok {
			valor = v
			encontrado = true
			continue
		}
		resto = append(resto, args[i])
	}
	return resto, valor, encontrado
}

func sacarFlag(args []string, nombre string) ([]string, bool) {
	encontrado := false
	// Copia propia: con `args[:0]` se reusaba el backing array del slice
	// del caller y le pisaba los elementos (D18).
	resto := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == nombre {
			encontrado = true
			continue
		}
		resto = append(resto, arg)
	}
	return resto, encontrado
}

func eventsCmd(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	_ = jsonFlag
	desde := "6h"
	fs := flag.NewFlagSet("events", flag.ExitOnError)
	fs.StringVar(&desde, "since", "6h", "ventana (6h, 30m, 24h)")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	eventos, err := docker.Events(ctx, runner, desde)
	if err != nil {
		fmt.Fprintln(os.Stderr, "events:", err)
		return 1
	}
	data, _ := json.Marshal(map[string]any{"since": desde, "events": eventos, "total": len(eventos)})
	fmt.Println(string(data))
	return 0
}

func composeCmd(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	_ = jsonFlag
	// El flag se saca a mano: el `flag` de Go deja de parsear en el primer
	// posicional, así que `compose <proyecto> --action up` perdía la acción
	// y corría la de por defecto (D5).
	args, accion, hayAccion := extraerValor(args, "--action")
	if !hayAccion || accion == "" {
		accion = "restart"
	}

	// `compose ls` lista los stacks: se arma desde el snapshot de
	// contenedores (las etiquetas de compose), sin un proceso extra.
	if len(args) >= 1 && args[0] == "ls" {
		ctx, cancel := ctxDe()
		defer cancel()
		snap := docker.Collect(ctx, runner, true)
		projects := docker.GroupProjects(snap.Containers)
		data, _ := json.Marshal(map[string]any{"projects": projects})
		fmt.Println(string(data))
		return 0
	}

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: b3l0w docker compose ls\n       b3l0w docker compose <proyecto> --action up|down|restart|stop|pull")
		return 2
	}
	if strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "compose: falta el nombre del proyecto")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ComposeAction(ctx, runner, args[0], accion)
	data, _ := json.Marshal(map[string]any{"ok": err == nil && res.OK, "command": res.Command, "output": res.Output})
	fmt.Println(string(data))
	if err != nil {
		return 1
	}
	return 0
}

func infoCmd(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	_ = jsonFlag
	ctx, cancel := ctxDe()
	defer cancel()
	info, err := docker.GetInfo(ctx, runner)
	if err != nil {
		fmt.Fprintln(os.Stderr, "info:", err)
		return 1
	}
	data, _ := json.Marshal(info)
	fmt.Println(string(data))
	return 0
}

func ctxDe() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

// ctxRespaldo es el timeout del respaldo de volúmenes: empaquetar o
// restaurar un volumen grande tarda bastante más que un comando normal.
func ctxRespaldo() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}

func snapshot(args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	all := fs.Bool("all", false, "incluir detenidos")
	asJSON := fs.Bool("json", false, "salida JSON")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	snap := docker.Collect(ctx, runner, *all)
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(snap)
		if snap.Error != "" {
			return 1
		}
		return 0
	}
	fmt.Printf("contenedores %d (running %d · parados %d · unhealthy %d)\n",
		len(snap.Containers), snap.Running, snap.Stopped, snap.Unhealthy)
	for _, c := range snap.Containers {
		salud := ""
		if c.Health != "" {
			salud = " [" + c.Health + "]"
		}
		fmt.Printf("  %-12s %-28s %-10s %s%s\n", c.ID, recorta(c.Name, 28), c.State, c.Image, salud)
	}
	fmt.Printf("imágenes %d · volúmenes %d · redes %d\n", len(snap.Images), len(snap.Volumes), len(snap.Networks))
	if snap.Error != "" {
		fmt.Fprintln(os.Stderr, "aviso:", snap.Error)
	}
	return 0
}

func control(action string, args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "falta el id del contenedor\n")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.Action
	var err error
	switch action {
	case "start":
		res, err = docker.Start(ctx, runner, args[0])
	case "stop":
		res, err = docker.Stop(ctx, runner, args[0])
	case "restart":
		res, err = docker.Restart(ctx, runner, args[0])
	default:
		res, err = docker.Remove(ctx, runner, args[0])
	}
	return salidaJSON(res, err, jsonFlag)
}

func removeResource(what string, args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "falta el recurso\n")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.Action
	var err error
	switch what {
	case "rmi":
		res, err = docker.RemoveImage(ctx, runner, args[0])
	case "volume-rm":
		res, err = docker.RemoveVolume(ctx, runner, args[0])
	default:
		res, err = docker.RemoveNetwork(ctx, runner, args[0])
	}
	return salidaJSON(res, err, jsonFlag)
}

func runImage(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	image := fs.String("image", "", "imagen")
	name := fs.String("name", "", "nombre")
	network := fs.String("network", "", "red")
	asJSON := fs.Bool("json", false, "salida JSON")
	var ports, env listaFlag
	fs.Var(&ports, "port", "puerto host:contenedor (repetible)")
	fs.Var(&env, "env", "variable CLAVE=valor (repetible)")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.RunImage(ctx, runner, docker.RunOptions{
		Image: *image, Name: *name, Network: *network, Ports: ports, Env: env,
		// El CLI corre el contenedor en segundo plano: `Detach` se respeta
		// en el paquete (D8), así que el CLI lo pide explícitamente.
		Detach: true,
	})
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(res)
		if err != nil {
			return 1
		}
		return 0
	}
	return salida(res, err)
}

func logs(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	_ = jsonFlag
	args, valor, hay := extraerValor(args, "--tail")
	tail := 80
	if hay {
		if n, err := strconv.Atoi(valor); err == nil && n > 0 {
			tail = n
		}
	}
	if tail > 400 {
		tail = 400
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "falta el id")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	texto, err := docker.Logs(ctx, runner, args[0], tail)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Print(texto)
	return 0
}

func statsCmd(args []string) int {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	lista, err := docker.Stats(ctx, runner)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"stats": lista, "at": time.Now().Unix()})
		return 0
	}
	for _, stat := range lista {
		fmt.Printf("  %-24.24s cpu %5.1f%%  mem %5.1f%%  %s  pids %d\n",
			stat.Name, stat.CPUPct, stat.MemPct, stat.MemUsage, stat.PIDs)
	}
	return 0
}

func inspectCmd(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	asJSON := fs.Bool("json", jsonFlag, "salida JSON")
	fs.Parse(args)
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "falta el id")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	info, err := docker.InspectContainer(ctx, runner, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(info)
		return 0
	}
	fmt.Printf("%s (%s) %s\nrestart %s · policy %s · redes %s\n",
		info.Name, info.ID, info.State, info.StartedAt, info.RestartPolicy, strings.Join(info.Networks, ", "))
	fmt.Printf("env %d · labels %d · montajes %d\n", len(info.Env), len(info.Labels), len(info.Mounts))
	for _, mount := range info.Mounts {
		fmt.Printf("  %s → %s (%s)\n", mount.Source, mount.Destination, mount.Type)
	}
	return 0
}

func dfCmd(args []string) int {
	fs := flag.NewFlagSet("df", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	filas, total, err := docker.DiskUsage(ctx, runner)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"rows": filas, "total": total})
		return 0
	}
	for _, fila := range filas {
		fmt.Printf("  %-12s %3d (%d activos)  %-10s recuperable %s\n", fila.Type, fila.Total, fila.Active, fila.Size, fila.Reclaimable)
	}
	_ = total
	return 0
}

func pruneCmd(args []string) int {
	args, jsonFlag := sacarFlag(args, "--json")
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	asJSON := fs.Bool("json", jsonFlag, "salida JSON")
	fs.Parse(args)
	validos := map[string]bool{"containers": true, "images": true, "volumes": true, "builder": true, "system": true}
	destino := fs.Arg(0)
	if destino == "" {
		fmt.Fprintln(os.Stderr, "falta el destino: containers|images|volumes|builder|system")
		return 2
	}
	if !validos[destino] {
		// Sin esto el plugin recibía {"ok":false} sin motivo.
		msg := fmt.Sprintf("destino inválido: %q (containers|images|volumes|builder|system)", destino)
		if *asJSON {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": msg, "command": ""})
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res, err := docker.Prune(ctx, runner, destino)
	if *asJSON {
		cuerpo := map[string]any{"ok": err == nil && res.OK, "command": res.Command, "output": res.Output}
		if err != nil {
			cuerpo["error"] = err.Error()
		}
		json.NewEncoder(os.Stdout).Encode(cuerpo)
		if err != nil {
			return 1
		}
		return 0
	}
	return salida(res, err)
}

// salidaJSON distingue el modo máquina: el plugin de Noctalia parsea stdout, y
// si acá se imprime texto el callback recibe basura y reporta error aunque la
// acción haya funcionado.
func salidaJSON(res docker.Action, err error, asJSON bool) int {
	if asJSON {
		cuerpo := map[string]any{"ok": err == nil, "command": res.Command, "output": res.Output}
		if err != nil {
			cuerpo["error"] = err.Error()
		}
		data, _ := json.Marshal(cuerpo)
		fmt.Println(string(data))
		if err != nil {
			return 1
		}
		return 0
	}
	return salida(res, err)
}

func salida(res docker.Action, err error) int {
	if res.Command != "" {
		fmt.Println("$", res.Command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if res.Output != "" {
		fmt.Println(res.Output)
	}
	return 0
}

func recorta(texto string, max int) string {
	if len(texto) <= max {
		return texto
	}
	return texto[:max-1] + "…"
}

// listaFlag es un flag repetible (--port 8080 --port 9090).
type listaFlag []string

// String implementa flag.Value y se usa sólo para imprimir el valor actual.
func (l *listaFlag) String() string { return strings.Join(*l, ",") }

// Set agrega un valor cada vez que el flag aparece en la línea de comandos.
func (l *listaFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// ── funciones de Docker Desktop ──────────────────────────────────────────

// jsonFlag de un comando: el flag puede venir en cualquier posición.
func flagJSON(args []string) ([]string, bool) {
	return sacarFlag(args, "--json")
}

// runSimple ejecuta un comando de docker con un argumento y devuelve la
// salida. Se usa para `top` y `history`, que no tienen forma propia.
func runSimple(args []string, comando string) (docker.Action, error) {
	resto, _ := flagJSON(args)
	if len(resto) == 0 {
		return docker.Action{OK: false, Command: "docker " + comando}, fmt.Errorf("%s: falta el argumento", comando)
	}
	ctx, cancel := ctxDe()
	defer cancel()
	return docker.RunCommand(ctx, runner, comando, resto)
}

// execCmd corre un comando dentro de un contenedor en ejecución.
func execCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	argv, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("exec: falta --id")
	}
	if len(argv) == 0 {
		return errorSimple("exec: falta el comando")
	}
	if codigo := idInvalido("exec", id); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ExecInContainer(ctx, runner, id, argv)
	if err != nil {
		return imprimirExec(res, err, jsonFlag)
	}
	return imprimirExec(res, nil, jsonFlag)
}

// imprimirExec imprime el resultado de exec.
func imprimirExec(res docker.ExecResult, err error, jsonFlag bool) int {
	if jsonFlag {
		cuerpo := map[string]any{"ok": err == nil, "running": res.Running, "exit": res.Exit, "output": res.Output}
		if err != nil {
			cuerpo["error"] = err.Error()
		}
		data, _ := json.Marshal(cuerpo)
		fmt.Println(string(data))
		if err != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println(res.Output)
	return 0
}

// partirRutaContenedor separa el prefijo de contenedor de una ruta de
// docker. "61555f00de09:/etc/hostname" → ("61555f00de09", "/etc/hostname").
// "id:/ruta" → ("id", "/ruta"): el prefijo "id" es un marcador que
// significa "el contenedor que indica --from". Una ruta del host sin
// prefijo → ("", ruta).
func partirRutaContenedor(ruta string) (string, string) {
	idx := strings.Index(ruta, ":")
	if idx < 0 {
		return "", ruta
	}
	prefijo := ruta[:idx]
	resto := ruta[idx+1:]
	if prefijo == "id" {
		return "id", resto
	}
	return prefijo, resto
}

// copyCmd copia archivos de o hacia un contenedor.
func copyCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	// IMPORTANTE: `extraerValor` devuelve el resto YA SIN el flag que
	// saca. Hay que usar ese resto, no el original: si se conserva el
	// original, `--from` sigue en la lista y el origen posicional acaba
	// siendo el propio "--from" (docker responde "unknown flag: --from").
	resto, id, ok := extraerValor(resto, "--from")
	if !ok || id == "" {
		return errorSimple("cp: falta --from <contenedor>")
	}
	if codigo := idInvalido("cp", id); codigo != 0 {
		return codigo
	}
	resto, destino, okDestino := extraerValor(resto, "--to")
	if !okDestino || destino == "" {
		return errorSimple("cp: falta --to <destino>")
	}
	// El origen es el primer argumento posicional que sobra.
	origen := ""
	for _, arg := range resto {
		if !strings.HasPrefix(arg, "-") {
			origen = arg
			break
		}
	}
	if origen == "" {
		return errorSimple("cp: falta la ruta de origen")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.CopyResult
	var err error
	// Normalización de las dos rutas. `docker cp` toma "host/ruta" o
	// "contenedor:/ruta" en CADA lado:
	//   - el origen con prefijo ("id:" o "<id>:") = dentro del contenedor
	//   - el destino con prefijo = dentro del contenedor
	// Si el origen está dentro del contenedor, el destino es del host; si el
	// origen es del host, el destino es del contenedor. Prefijar el id dos
	// veces ("61555f00de09:61555f00de09:/etc/hostname") hace que docker
	// responda que no encuentra el archivo.
	origenCont, origenRuta := partirRutaContenedor(origen)
	destCont, destRuta := partirRutaContenedor(destino)
	// "id:" significa "el contenedor que indica --from". Un id completo
	// también vale, pero el contenedor de la operación es el de --from:
	// si no coincidieran, el id de la ruta se ignora.
	if origenCont == "id" || origenCont == id {
		origenCont = id
	}
	if destCont == "id" || destCont == id {
		destCont = id
	}
	switch {
	case origenCont != "":
		// Del contenedor al host.
		if destCont != "" {
			return errorSimple("cp: no se puede copiar entre contenedores")
		}
		res, err = docker.CopyFrom(ctx, runner, origenCont, origenRuta, destRuta)
	case destCont != "":
		// Del host al contenedor.
		res, err = docker.Copy(ctx, runner, destCont, origen, destRuta)
	default:
		return errorSimple("cp: el origen o el destino debe ser una ruta dentro del contenedor (id:/ruta)")
	}
	if err != nil {
		return imprimirCopy(res, err, jsonFlag)
	}
	return imprimirCopy(res, nil, jsonFlag)
}

// imprimirCopy imprime el resultado de cp.
func imprimirCopy(res docker.CopyResult, err error, jsonFlag bool) int {
	if jsonFlag {
		cuerpo := map[string]any{"ok": err == nil, "path": res.Path, "output": res.Output}
		if err != nil {
			cuerpo["error"] = err.Error()
		}
		data, _ := json.Marshal(cuerpo)
		fmt.Println(string(data))
		if err != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// commitCmd crea una imagen desde el estado de un contenedor.
func commitCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("commit: falta --id")
	}
	if codigo := idInvalido("commit", id); codigo != 0 {
		return codigo
	}
	_, repo, _ := extraerValor(resto, "--repo")
	_, tag, _ := extraerValor(resto, "--tag")
	_, mensaje, _ := extraerValor(resto, "--message")
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Commit(ctx, runner, id, repo, tag, mensaje)
	return salidaJSON(res, err, jsonFlag)
}

// pauseCmd congela o reanuda un contenedor.
func pauseCmd(args []string, pausar bool) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		if pausar {
			return errorSimple("pause: falta --id")
		}
		return errorSimple("unpause: falta --id")
	}
	if codigo := idInvalido(accionPause(pausar), id); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.Action
	var err error
	if pausar {
		res, err = docker.Pause(ctx, runner, id)
	} else {
		res, err = docker.Unpause(ctx, runner, id)
	}
	return salidaJSON(res, err, jsonFlag)
}

// renameCmd cambia el nombre de un contenedor.
func renameCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("rename: falta --id")
	}
	if codigo := idInvalido("rename", id); codigo != 0 {
		return codigo
	}
	_, nombre, _ := extraerValor(resto, "--name")
	if nombre == "" {
		return errorSimple("rename: falta --name")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Rename(ctx, runner, id, nombre)
	return salidaJSON(res, err, jsonFlag)
}

// tagCmd etiqueta una imagen.
func tagCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("tag: falta --id")
	}
	if codigo := idInvalido("tag", id); codigo != 0 {
		return codigo
	}
	_, repo, _ := extraerValor(resto, "--repo")
	if repo == "" {
		return errorSimple("tag: falta --repo")
	}
	_, tag, _ := extraerValor(resto, "--tag")
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Tag(ctx, runner, id, repo, tag)
	return salidaJSON(res, err, jsonFlag)
}

// topCmd lista los procesos de un contenedor.
func topCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	res, err := runSimple(resto, "top")
	return salidaJSON(res, err, jsonFlag)
}

// historyCmd devuelve el historial de capas de una imagen.
func historyCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	res, err := runSimple(resto, "history")
	return salidaJSON(res, err, jsonFlag)
}

// errorSimple imprime un error por stderr y devuelve 1.
func errorSimple(msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	return 1
}

// idValido dice si un identificador se puede pasar a docker. Un id que
// empieza con "-" lo interpretaría docker como un flag suyo (--privileged,
// --user root) y no como el nombre de un contenedor (D12).
func idValido(id string) bool {
	return id != "" && !strings.HasPrefix(id, "-")
}

// accionPause devuelve el nombre del comando de pause o unpause.
func accionPause(pausar bool) string {
	if pausar {
		return "pause"
	}
	return "unpause"
}

// idInvalido devuelve 1 si el id no se puede pasar a docker.
func idInvalido(comando string, id string) int {
	if idValido(id) {
		return 0
	}
	return errorSimple(comando + ": el id no puede empezar con -")
}

// pullCmd descarga una imagen del registro.
func pullCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("pull: falta el nombre de la imagen")
	}
	if codigo := idInvalido("pull", resto[0]); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Pull(ctx, runner, resto[0])
	return salidaJSON(res, err, jsonFlag)
}

// pushCmd sube una imagen al registro.
func pushCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("push: falta el nombre de la imagen")
	}
	if codigo := idInvalido("push", resto[0]); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Push(ctx, runner, resto[0])
	return salidaJSON(res, err, jsonFlag)
}

// createCmd crea un volumen o una red nueva.
func createCmd(args []string, clase string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple(clase + "-create: falta el nombre")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.Action
	var err error
	if clase == "volume" {
		res, err = docker.CreateVolume(ctx, runner, resto[0])
	} else {
		res, err = docker.CreateNetwork(ctx, runner, resto[0])
	}
	return salidaJSON(res, err, jsonFlag)
}

// saveCmd exporta una imagen a un tar.
func saveCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, imagen, okImagen := extraerValor(resto, "--image")
	_, destino, okDestino := extraerValor(resto, "--out")
	if !okImagen || !okDestino || imagen == "" || destino == "" {
		return errorSimple("save: falta --image o --out")
	}
	if codigo := idInvalido("save", imagen); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.SaveImage(ctx, runner, imagen, destino)
	return salidaJSON(res, err, jsonFlag)
}

// loadCmd importa una imagen desde un tar.
func loadCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, origen, ok := extraerValor(resto, "--in")
	if !ok || origen == "" {
		return errorSimple("load: falta --in")
	}
	if codigo := idInvalido("load", origen); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.LoadImage(ctx, runner, origen)
	return salidaJSON(res, err, jsonFlag)
}

// exportCmd exporta el sistema de archivos de un contenedor a un tar.
func exportCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, okID := extraerValor(resto, "--id")
	_, destino, okDestino := extraerValor(resto, "--out")
	if !okID || !okDestino || id == "" || destino == "" {
		return errorSimple("export: falta --id o --out")
	}
	if codigo := idInvalido("export", id); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ExportContainer(ctx, runner, id, destino)
	return salidaJSON(res, err, jsonFlag)
}

// diffCmd muestra los cambios del sistema de archivos del contenedor.
func diffCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("diff: falta --id")
	}
	if codigo := idInvalido("diff", id); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Diff(ctx, runner, id)
	return salidaJSON(res, err, jsonFlag)
}

// buildCmd construye una imagen desde un Dockerfile.
func buildCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	resto, etiqueta, ok := extraerValor(resto, "--tag")
	if !ok || etiqueta == "" {
		return errorSimple("build: falta --tag <imagen>")
	}
	resto, dockerfile, _ := extraerValor(resto, "--file")
	resto, contexto, _ := extraerValor(resto, "--context")
	// El contexto es el único posicional que puede quedar.
	if contexto == "" {
		for _, arg := range resto {
			if !strings.HasPrefix(arg, "-") {
				contexto = arg
				break
			}
		}
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Build(ctx, runner, etiqueta, contexto, dockerfile)
	return salidaJSON(res, err, jsonFlag)
}

// ── puertos, búsqueda y límites ─────────────────────────────────────────

// portCmd: el mapeo de puertos publicado por un contenedor
// (`0.0.0.0:8080->80/tcp`). Es lo que el panel necesita para ofrecer
// "abrir en el navegador".
func portCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("port: falta el id o nombre del contenedor")
	}
	if codigo := idInvalido("port", resto[0]); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.RunCommand(ctx, runner, "port", resto)
	return salidaJSON(res, err, jsonFlag)
}

// searchCmd: busca imágenes en Docker Hub.
func searchCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	resto, limite, hayLimite := extraerValor(resto, "--limit")
	if !hayLimite || limite == "" {
		limite = "25"
	}
	if len(resto) == 0 {
		return errorSimple("search: falta el término de búsqueda")
	}
	if strings.HasPrefix(resto[0], "-") {
		return errorSimple("search: el término no puede empezar con -")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	// `--format json`: una línea JSON por resultado con Name, Description,
	// StarCount e IsOfficial. La tabla de texto obliga a parsear columnas
	// alineadas y se rompe con las descripciones largas.
	comando := append([]string{"--limit", limite, "--format", "{{json .}}"}, resto...)
	res, err := docker.RunCommand(ctx, runner, "search", comando)
	return salidaJSON(res, err, jsonFlag)
}

// updateCmd: cambia los límites de CPU y memoria de un contenedor vivo.
func updateCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	resto, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("update: falta --id <contenedor>")
	}
	resto, cpus, hayCpus := extraerValor(resto, "--cpus")
	resto, memoria, hayMemoria := extraerValor(resto, "--memory")
	var flags []string
	if hayCpus && cpus != "" {
		flags = append(flags, "--cpus", cpus)
	}
	if hayMemoria && memoria != "" {
		flags = append(flags, "--memory", memoria)
	}
	if len(flags) == 0 {
		return errorSimple("update: falta --cpus <n> o --memory <tamaño>")
	}
	if strings.HasPrefix(id, "-") {
		return errorSimple("update: el id no puede empezar con -")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.RunCommand(ctx, runner, "update", append(flags, id))
	return salidaJSON(res, err, jsonFlag)
}

// ── inspect de volúmenes y redes ────────────────────────────────────────

func volumeInspectCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("volume-inspect: falta el nombre del volumen")
	}
	if codigo := idInvalido("volume-inspect", resto[0]); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.RunCommand(ctx, runner, "volume", append([]string{"inspect"}, resto...))
	return salidaJSON(res, err, jsonFlag)
}

func networkInspectCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("network-inspect: falta el nombre de la red")
	}
	if codigo := idInvalido("network-inspect", resto[0]); codigo != 0 {
		return codigo
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.RunCommand(ctx, runner, "network", append([]string{"inspect"}, resto...))
	return salidaJSON(res, err, jsonFlag)
}

// ── respaldo de volúmenes ───────────────────────────────────────────────

// volumeExportCmd empaqueta un volumen en un tar.gz. Docker no tiene un
// comando propio para esto: el camino oficial es un contenedor efímero que
// monta el volumen como origen y la carpeta destino como punto de escritura.
func volumeExportCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	resto, nombre, ok := extraerValor(resto, "--name")
	if !ok || nombre == "" {
		return errorSimple("volume-export: falta --name <volumen>")
	}
	resto, destino, ok := extraerValor(resto, "--out")
	if !ok || destino == "" {
		return errorSimple("volume-export: falta --out <archivo.tar.gz>")
	}
	if strings.HasPrefix(nombre, "-") || strings.HasPrefix(destino, "-") {
		return errorSimple("volume-export: ni el nombre ni la salida pueden empezar con -")
	}
	carpeta, archivo, err := rutaRespaldo(destino)
	if err != nil {
		return errorSimple("volume-export: " + err.Error())
	}
	// El respaldo de un volumen grande tarda más que el timeout general.
	ctx, cancel := ctxRespaldo()
	defer cancel()
	comando := []string{
		"--rm",
		"-v", nombre + ":/origen:ro",
		"-v", carpeta + ":/respaldo",
		"alpine", "tar", "czf", "/respaldo/" + archivo, "-C", "/origen", ".",
	}
	res, err := docker.RunCommand(ctx, runner, "run", comando)
	return salidaJSON(res, err, jsonFlag)
}

// volumeImportCmd restaura un tar.gz en un volumen (lo crea si no existe).
func volumeImportCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	resto, nombre, ok := extraerValor(resto, "--name")
	if !ok || nombre == "" {
		return errorSimple("volume-import: falta --name <volumen>")
	}
	resto, origen, ok := extraerValor(resto, "--in")
	if !ok || origen == "" {
		return errorSimple("volume-import: falta --in <archivo.tar.gz>")
	}
	if strings.HasPrefix(nombre, "-") || strings.HasPrefix(origen, "-") {
		return errorSimple("volume-import: ni el nombre ni el origen pueden empezar con -")
	}
	carpeta, archivo, err := rutaRespaldo(origen)
	if err != nil {
		return errorSimple("volume-import: " + err.Error())
	}
	// La restauración de un volumen grande tarda más que el timeout general.
	ctx, cancel := ctxRespaldo()
	defer cancel()
	comando := []string{
		"--rm",
		"-v", nombre + ":/destino",
		"-v", carpeta + ":/respaldo:ro",
		"alpine", "tar", "xzf", "/respaldo/" + archivo, "-C", "/destino",
	}
	res, err := docker.RunCommand(ctx, runner, "run", comando)
	return salidaJSON(res, err, jsonFlag)
}

// rutaRespaldo resuelve la ruta de un archivo de respaldo a una ruta
// absoluta y devuelve la carpeta que se monta y el nombre del archivo.
//
// Una ruta relativa se resuelve dentro del directorio actual y no puede
// salirse de ahí. Antes, una ruta sin barra ("backup.tar.gz") devolvía la
// raíz del sistema y el comando montaba "/" del host en el contenedor y
// escribía el tarball en "/" (D4).
func rutaRespaldo(ruta string) (string, string, error) {
	if strings.TrimSpace(ruta) == "" {
		return "", "", fmt.Errorf("la ruta está vacía")
	}
	absoluta := ruta
	if !filepath.IsAbs(ruta) {
		base, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("no pude resolver el directorio actual: %w", err)
		}
		absoluta = filepath.Join(base, ruta)
		if !dentroDe(base, absoluta) {
			return "", "", fmt.Errorf("la ruta %q se sale del directorio actual", ruta)
		}
	}
	absoluta = filepath.Clean(absoluta)
	carpeta := filepath.Dir(absoluta)
	archivo := filepath.Base(absoluta)
	if carpeta == "/" {
		return "", "", fmt.Errorf("la ruta %q no puede quedar en la raíz del sistema", ruta)
	}
	if archivo == "." || archivo == ".." || archivo == string(filepath.Separator) {
		return "", "", fmt.Errorf("la ruta %q no es un archivo", ruta)
	}
	return carpeta, archivo, nil
}

// dentroDe dice si la ruta está dentro de la base.
func dentroDe(base, ruta string) bool {
	rel, err := filepath.Rel(base, ruta)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ── logs en vivo ────────────────────────────────────────────────────────

// logsFollowCmd sigue los logs de un contenedor hasta que se corta. Escribe
// directo a stdout: acumular en memoria no tendría sentido con --follow.
func logsFollowCmd(args []string) int {
	resto, _ := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("logs-follow: falta el id del contenedor")
	}
	resto, cola, hayCola := extraerValor(resto, "--tail")
	if !hayCola || cola == "" {
		cola = "200"
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comando := append([]string{"logs", "--follow", "--tail", cola}, resto...)
	return correrEnVivo(ctx, "logs", comando)
}

// correrEnVivo corre docker con la salida conectada a la terminal: es para
// los comandos que no terminan (--follow).
func correrEnVivo(ctx context.Context, comando string, args []string) int {
	cmd := exec.CommandContext(ctx, "docker", append([]string{comando}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
