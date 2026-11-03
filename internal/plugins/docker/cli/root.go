// Package dockercli arma los comandos de `b3l0w docker`.
package dockercli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w/internal/plugins/docker"
)

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
  stats [--json]                   CPU/RAM/IO por contenedor (una muestra)
  inspect <id> [--json]            env, labels, montajes, redes, health
  events [--since 6h] [--json]       últimos eventos de contenedor
  compose <proyecto> [--action A]    up|down|restart|stop|pull de un stack
  info [--json]                    CPUs, RAM, versión, driver, conteos
  df [--json]                      espacio usado y recuperable
  prune <destino> [--json]         containers|images|volumes|builder|system
  exec --id ID CMD... [--json]      corre un comando en el contenedor
  cp --from ID RUTA --to RUTA       copia archivos (RUTA con "id:" = del contenedor)
  commit --id ID [--repo R] [--tag T] [--message M]   crea una imagen
  pause|unpause --id ID             congela / reanuda un contenedor
  rename --id ID --name N           renombra un contenedor
  tag --id ID --repo R [--tag T]     etiqueta una imagen
  top <id> [--json]                 procesos del contenedor
  history <id> [--json]             historial de capas de una imagen

  pull <imagen>                   descarga una imagen del registro
  push <imagen>                   sube una imagen al registro
  volume-create <nombre>          crea un volumen
  network-create <nombre>         crea una red
  save --image I --out F          exporta una imagen a un tar
  load --in F                     importa una imagen desde un tar
  export --id C --out F           exporta el filesystem de un contenedor
  diff --id C                     muestra los cambios del filesystem
`

// Run despacha un subcomando de `b3l0w docker` y devuelve el código de salida:
// 0 correcto, 1 error de uso o de docker. Sin argumentos imprime la ayuda.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
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
	resto := args[:0]
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
	eventos, err := docker.Events(ctx, docker.Exec, desde)
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
	fs := flag.NewFlagSet("compose", flag.ExitOnError)
	accion := fs.String("action", "restart", "up|down|restart|stop|pull")
	fs.Parse(args)

	// `compose ls` lista los stacks: se arma desde el snapshot de
	// contenedores (las etiquetas de compose), sin un proceso extra.
	if len(args) >= 1 && args[0] == "ls" {
		ctx, cancel := ctxDe()
		defer cancel()
		snap := docker.Collect(ctx, docker.Exec, true)
		projects := docker.GroupProjects(snap.Containers)
		data, _ := json.Marshal(map[string]any{"projects": projects})
		fmt.Println(string(data))
		return 0
	}

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: b3l0w docker compose ls\n       b3l0w docker compose <proyecto> --action up|down|restart|stop|pull")
		return 2
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ComposeAction(ctx, docker.Exec, args[0], *accion)
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
	info, err := docker.GetInfo(ctx, docker.Exec)
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

func snapshot(args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	all := fs.Bool("all", false, "incluir detenidos")
	asJSON := fs.Bool("json", false, "salida JSON")
	fs.Parse(args)
	ctx, cancel := ctxDe()
	defer cancel()
	snap := docker.Collect(ctx, docker.Exec, *all)
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
		res, err = docker.Start(ctx, docker.Exec, args[0])
	case "stop":
		res, err = docker.Stop(ctx, docker.Exec, args[0])
	case "restart":
		res, err = docker.Restart(ctx, docker.Exec, args[0])
	default:
		res, err = docker.Remove(ctx, docker.Exec, args[0])
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
		res, err = docker.RemoveImage(ctx, docker.Exec, args[0])
	case "volume-rm":
		res, err = docker.RemoveVolume(ctx, docker.Exec, args[0])
	default:
		res, err = docker.RemoveNetwork(ctx, docker.Exec, args[0])
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
	res, err := docker.RunImage(ctx, docker.Exec, docker.RunOptions{
		Image: *image, Name: *name, Network: *network, Ports: ports, Env: env,
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
	texto, err := docker.Logs(ctx, docker.Exec, args[0], tail)
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
	lista, err := docker.Stats(ctx, docker.Exec)
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
	info, err := docker.InspectContainer(ctx, docker.Exec, fs.Arg(0))
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
	filas, total, err := docker.DiskUsage(ctx, docker.Exec)
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
	res, err := docker.Prune(ctx, docker.Exec, destino)
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
	return docker.RunCommand(ctx, docker.Exec, comando, resto)
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
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ExecInContainer(ctx, docker.Exec, id, argv)
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

// copyCmd copia archivos de o hacia un contenedor.
func copyCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--from")
	if !ok || id == "" {
		return errorSimple("cp: falta --from <contenedor>")
	}
	// El origen es el primer argumento posicional que sobra.
	origen := ""
	if len(resto) > 0 {
		origen = resto[0]
	}
	if origen == "" {
		return errorSimple("cp: falta la ruta de origen")
	}
	_, destino, _ := extraerValor(resto, "--to")
	if destino == "" {
		return errorSimple("cp: falta --to <destino>")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.CopyResult
	var err error
	if strings.Contains(origen, ":") {
		res, err = docker.CopyFrom(ctx, docker.Exec, id, origen, destino)
	} else {
		res, err = docker.Copy(ctx, docker.Exec, id, origen, destino)
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
	_, repo, _ := extraerValor(resto, "--repo")
	_, tag, _ := extraerValor(resto, "--tag")
	_, mensaje, _ := extraerValor(resto, "--message")
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Commit(ctx, docker.Exec, id, repo, tag, mensaje)
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
	ctx, cancel := ctxDe()
	defer cancel()
	var res docker.Action
	var err error
	if pausar {
		res, err = docker.Pause(ctx, docker.Exec, id)
	} else {
		res, err = docker.Unpause(ctx, docker.Exec, id)
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
	_, nombre, _ := extraerValor(resto, "--name")
	if nombre == "" {
		return errorSimple("rename: falta --name")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Rename(ctx, docker.Exec, id, nombre)
	return salidaJSON(res, err, jsonFlag)
}

// tagCmd etiqueta una imagen.
func tagCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("tag: falta --id")
	}
	_, repo, _ := extraerValor(resto, "--repo")
	if repo == "" {
		return errorSimple("tag: falta --repo")
	}
	_, tag, _ := extraerValor(resto, "--tag")
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Tag(ctx, docker.Exec, id, repo, tag)
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

// pullCmd descarga una imagen del registro.
func pullCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("pull: falta el nombre de la imagen")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Pull(ctx, docker.Exec, resto[0])
	return salidaJSON(res, err, jsonFlag)
}

// pushCmd sube una imagen al registro.
func pushCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	if len(resto) == 0 {
		return errorSimple("push: falta el nombre de la imagen")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Push(ctx, docker.Exec, resto[0])
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
		res, err = docker.CreateVolume(ctx, docker.Exec, resto[0])
	} else {
		res, err = docker.CreateNetwork(ctx, docker.Exec, resto[0])
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
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.SaveImage(ctx, docker.Exec, imagen, destino)
	return salidaJSON(res, err, jsonFlag)
}

// loadCmd importa una imagen desde un tar.
func loadCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, origen, ok := extraerValor(resto, "--in")
	if !ok || origen == "" {
		return errorSimple("load: falta --in")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.LoadImage(ctx, docker.Exec, origen)
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
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.ExportContainer(ctx, docker.Exec, id, destino)
	return salidaJSON(res, err, jsonFlag)
}

// diffCmd muestra los cambios del sistema de archivos del contenedor.
func diffCmd(args []string) int {
	resto, jsonFlag := flagJSON(args)
	_, id, ok := extraerValor(resto, "--id")
	if !ok || id == "" {
		return errorSimple("diff: falta --id")
	}
	ctx, cancel := ctxDe()
	defer cancel()
	res, err := docker.Diff(ctx, docker.Exec, id)
	return salidaJSON(res, err, jsonFlag)
}
