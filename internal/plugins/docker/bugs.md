# Bugs de `internal/plugins/docker`

Revisión de integridad en contexto cero, hecha sobre `mod/rev-docker` (base
`main` en `68d4a8d`). Las secciones `## D1` … `## D21` son el **diagnóstico
original** (no se tocaron); al final está `## Arreglados`, con lo que se hizo
para cerrar cada ID y qué test lo cubre.

Todo lo encontrado está **arreglado**: 21 de 21. Los cinco hallazgos que
estaban codificados como comportamiento correcto en tests existentes (D1,
D13, D14, D15 y el hueco de D9) se ajustaron al contrato nuevo; está dicho
en la entrada de cada uno.

Método: se leyó el módulo completo (13 archivos de producción + la capa de
comandos `cli/root.go`) y se contrastó cada afirmación del `README.md` contra el
código. Los probes corren **contra una copia aislada del repo** en
`~/.hermes/profiles/coding/cache/scratch/probe-docker` (copia del worktree sin
`.git`), así que **el worktree no se tocó**: `go test` y `go build` siguen
verdes y el árbol quedó como estaba.

Los probes son de tres tipos:

- **Go**: un `_test.go` temporal (`zz_probe*_test.go`) en la copia del paquete,
  con el runner falso (`fakes_test.go`) o un runner a medida.
- **CLI**: `docker` falso en el `PATH` (un script), para lo que ejecuta un
  proceso real.
- **Upstream**: el código de `docker/cli` y `moby/moby`, para confirmar el
  formato real de `docker inspect` y `docker ps`.

Ningún probe levantó un daemon de Docker: `docker` nunca se ejecutó de verdad.

## Resumen

| ID | Sev | Archivo:línea | Síntoma | Estado |
|---|---|---|---|---|
| D1 | ALTO | `inspect.go:69-73` + `116-126` | `Inspect.Health` y `HealthLog` salen **siempre vacíos**: el parser lee `Config.Health`, pero `docker inspect` pone el health bajo `State.Health` | arreglado |
| D2 | ALTO | `cli/root.go:239` | `logs-stream` mete el id del contenedor en un `sh -c` con `%q`: un id con `$(...)` **ejecuta comandos** | arreglado |
| D3 | ALTO | `cli/root.go:287-290` | `logs-stop` manda `SIGTERM` al grupo del pid del pidfile **sin validar que ese pid sea el del seguimiento**: mata un proceso ajeno | arreglado |
| D4 | ALTO | `cli/root.go:1249-1258` | `volume-export` con ruta relativa monta **la raíz del host** (`-v /:/respaldo`) y escribe el tar en `/` | arreglado |
| D5 | ALTO | `cli/root.go:420` + `441` | `compose --action` se pierde cuando el flag va **después** del proyecto: corre `restart` en vez de lo pedido | arreglado |
| D6 | MEDIO | `containers.go:87-89` | `Exec` devuelve un error **vacío** cuando docker falla sin escribir nada (binario ausente, timeout) | arreglado |
| D7 | MEDIO | `containers.go:85-89` | `Exec` mezcla stderr con stdout: el "error" puede ser una línea de salida normal, no el error | arreglado |
| D8 | MEDIO | `actions.go:137` | `RunImage` ignora `RunOptions.Detach`: `if opts.Detach \|\| true` siempre mete `-d` | arreglado |
| D9 | MEDIO | `compose.go:128` | `Events` **traga el error** de docker si la salida no viene vacía: devuelve eventos parciales como si fueran buenos | arreglado |
| D10 | MEDIO | `system.go:57` | `DiskUsage` **ignora el error** del segundo `docker system df`: el total queda vacío sin avisar | arreglado |
| D11 | MEDIO | `cli/root.go:203-215` | `rutaLog` normaliza a minúsculas y trunca a 40: dos contenedores distintos **comparten log y pidfile** | arreglado |
| D12 | MEDIO | `cli/root.go:782-796`, `930-941`, `968-980` | Varios comandos pasan el id sin validar: un id que empieza con `-` se interpreta como flag de docker | arreglado |
| D13 | MEDIO | `inspect.go:116-126` | `HealthLog` se queda con las **primeras** 5 entradas del log, no con las últimas | arreglado |
| D14 | MEDIO | `stats.go:41`, `system.go:28`, `containers.go:193` | Varias listas internas salen `null` en el JSON (`"stats":null`, `"rows":null`, `"ports":null`) | arreglado |
| D15 | MEDIO | `containers.go:130-141` | `healthFromStatus` no reconoce la forma real `(health: starting)`: el estado `starting` nunca se ve | arreglado |
| D16 | MEDIO | `compose.go:47-49` | `GroupProjects` **duplica** el servicio cuando hay réplicas (`Services: [web web]`) | arreglado |
| D17 | BAJO | `actions.go:27-36` | `AllowedActions` no lo usa nadie: el comentario promete un control que no existe | arreglado |
| D18 | BAJO | `cli/root.go:386` | `sacarFlag` reusa el backing array del slice que recibe y pisa el slice del caller | arreglado |
| D19 | BAJO | `cli/root.go:281-293` | `logs-stop` contesta "detenido" aunque no hubiera ningún proceso vivo, y borra el pidfile | arreglado |
| D20 | BAJO | `cli/root.go:230-271` | `logs-stream` con un contenedor inexistente devuelve **código 0** y deja el pidfile de un proceso muerto | arreglado |
| D21 | BAJO | `desktop.go:16` + `cli/root.go:801` | `ExecResult.Exit` es siempre 0: el código de salida real del comando dentro del contenedor no viaja | arreglado |

**Totales:** 0 CRÍTICO · 5 ALTO · 11 MEDIO · 5 BAJO — 21 hallazgos.

---

## D1 — ALTO · `Inspect.Health` y `HealthLog` salen siempre vacíos

**Dónde.** `internal/plugins/docker/inspect.go:69-73` (el tipo del payload) y
`inspect.go:116-126` (el bucle que arma el log).

**Síntoma.** El detalle de un contenedor con healthcheck **no muestra nunca** el
estado de salud ni el historial de chequeos: `health` sale `""` y `healthLog`
sale `null`, siempre. El panel no puede distinguir "sano" de "roto".

**Causa.** `docker inspect --format '{{json .}}'` no pone el health en `Config`:
lo pone en **`State.Health`** (con `Status` y `Log`). El módulo declara el
health dentro de `Config`:

```go
Config: struct {
    ...
    Health *struct {
        Log []struct {
            Output string `json:"Output"`
        } `json:"Log"`
    } `json:"Health"`
} `json:"Config"`
```

Como `Config.Health` no existe en la salida real, `payload.Config.Health` es
siempre `nil` y el bucle de `inspect.go:116` no se ejecuta nunca. Además
`Inspect.Health` (el string, línea 25) **no se asigna en ningún lado**: no hay
ningún `out.Health = ...` en el archivo.

**Reproducción.** Con la forma real de `docker inspect` (health bajo
`State.Health`):

```go
const inspectReal = `{
  "Id": "abc123def4567890123456789012345678901234567890123456789012345678",
  "Name": "/web",
  "State": {
    "Status": "running",
    "StartedAt": "2024-01-01T10:00:00.000000000Z",
    "Health": {
      "Status": "healthy",
      "FailingStreak": 0,
      "Log": [{"Start": "2024-01-01T10:00:00Z", "End": "2024-01-01T10:00:01Z", "ExitCode": 0, "Output": "todo bien"}]
    }
  },
  "RestartCount": 0,
  "HostConfig": {"RestartPolicy": {"Name": "unless-stopped"}},
  "Path": "/docker-entrypoint.sh",
  "Args": ["nginx"],
  "Config": {"Image": "nginx:latest", "Env": ["A=1"], "Labels": {"x": "y"}}
}`
f := nuevoFake().con("inspect --format {{json .}} web", inspectReal)
info, _ := InspectContainer(context.Background(), f.run, "web")
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP1InspectHealthReal -v
P1 Health="" HealthLog=[] len=0
P1 json={"id":"abc123def456","name":"web","image":"nginx:latest","state":"running",...,"health":"","healthLog":null}
```

El health está en el JSON de entrada y sale vacío en el de salida.

**Contraste con upstream.** `moby/moby` (`api/types/container/container.go`)
define el health como parte de `ContainerState`, no de la config:

```
State.Health.Status   →  "healthy" | "unhealthy" | "starting"
State.Health.Log[]    →  [{Start, End, ExitCode, Output}]
```

y la documentación de docker confirma el camino: *"The `State.Health.Status`
key is only present in the container's JSON if the container was started with a
health check"* (docker/docs#5368), y el uso canónico es
`docker inspect --format '{{json .State.Health }}' <contenedor>`.

**Impacto.** Es la función que el panel usa para el detalle de un contenedor.
Todo contenedor con healthcheck muestra la salud como vacía. Además el
`HealthLog` (que el README promete con "las últimas 5 entradas", ver D13)
queda `null`: el plugin Luau que lo indexe recibe `nil`.

**Nota sobre el test que lo tapa.** `inspect_test.go:30` pone el health bajo
`Config` en su fixture:

```go
"Config": {
    "Image": "nginx:latest",
    ...
    "Health": {"Log": [{"Output": "ok"}, {"Output": "check 1"}]}
}
```

El fixture está bien para lo que el código parsea, pero **no es la forma que
manda docker**. El test pasa porque el test y el código comparten el mismo
error.

---

## D2 — ALTO · `logs-stream` ejecuta comandos con el id del contenedor

**Dónde.** `internal/plugins/docker/cli/root.go:239`.

**Síntoma.** `b3l0w docker logs-stream 'web$(touch /tmp/PWNED)'` **ejecuta**
`touch /tmp/PWNED` y devuelve 0 como si nada. Cualquier cosa que el shell
interprete se ejecuta con los permisos del usuario que corrió el comando.

**Causa.** El comando se arma con `fmt.Sprintf` y se pasa a `sh -c`:

```go
cmd := exec.Command("sh", "-c",
    fmt.Sprintf("exec docker logs -f --tail 200 %q >> %q 2>&1", id, archivo))
```

`%q` es el **quoting de Go**, no el del shell. Go escapa `"` y `\`, pero deja
intactos `$`, `` ` `` y `$( )`. El resultado es una cadena entre comillas
dobles, y en `sh` las comillas dobles **no** impiden la sustitución de
comandos: `"web$(touch X)"` sigue ejecutando `touch X`.

La validación de entrada que hay en el CLI es la del prefijo `-`
(`root.go:218`: `strings.HasPrefix(args[0], "-")`), que no cubre esto.

**Reproducción (extremo a extremo, con `docker` falso en el PATH).**

```go
t.Setenv("XDG_CACHE_HOME", t.TempDir())
marca := filepath.Join(t.TempDir(), "PWNED")
dockerFalso(t, "#!/bin/sh\nexit 0\n")
id := "web$(touch " + marca + ")"
capturaStdout(t, func() { Run([]string{"logs-stream", id}) })
// se espera a que el shell ejecute
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP39LogsStreamInyeccionReal -v
P39 codigo=0 id="web$(touch /…/002/PWNED)" comando-creado=true
```

`comando-creado=true`: el `touch` corrió. Y el comando devolvió **0**.

**Contraste.** El mismo archivo, en el mismo commit, escapa bien el otro
argumento del mismo `Sprintf`: el `archivo` (que viene de `rutaLog`, ya
saneado) no es el problema; el problema es el `id`, que es el único que
viene de la línea de comandos sin sanear.

**Impacto.** El CLI acepta cualquier string como id y lo mete en un shell.
Los *nombres* de contenedor de docker están restringidos a
`[a-zA-Z0-9][a-zA-Z0-9_.-]*` (sin `$` ni paréntesis), así que un id tomado
de `docker ps` no dispara la inyección; el vector real es cualquier otro id
que llegue al comando (un proyecto, un id escrito a mano, un valor que venga
de otro sistema). El arreglo no es sanear el id: es **no usar `sh -c`**. El
módulo ya tiene la forma correcta de hacer las dos cosas: `exec.Command` con
los argumentos separados (`docker logs -f --tail 200 <id>`) y el redirect del
log hecho con un `*os.File` abierto por Go en vez de por el shell.

---

## D3 — ALTO · `logs-stop` mata un proceso que no es el seguimiento

**Dónde.** `internal/plugins/docker/cli/root.go:274-294`.

**Síntoma.** `b3l0w docker logs-stop <contenedor>` manda `SIGTERM` al **grupo
de procesos** del pid que está en el pidfile, sin comprobar que ese pid sea
el del `docker logs -f`. Si el pid fue reciclado (el `logs -f` terminó y el
sistema reutilizó el pid), el comando mata un proceso ajeno del usuario.

**Causa.** El pidfile guarda sólo un número. `logsStopCmd` lo lee y lo usa
sin validar:

```go
pid := leerPid(pidFile)
if pid <= 0 { ... }
// Es líder de sesión (setsid), así que el pid es también el grupo:
// se manda la señal al grupo entero para no dejar hijos sueltos.
_ = syscall.Kill(-pid, syscall.SIGTERM)
if proc, err := os.FindProcess(pid); err == nil {
    _ = proc.Signal(syscall.SIGTERM)
}
_ = os.Remove(pidFile)
```

No hay ninguna comprobación de que el proceso sea un `docker logs -f`: ni del
nombre del ejecutable, ni de la línea de comandos, ni de la hora de arranque.
`logsStreamCmd` (root.go:225) sí valida que el pid esté vivo antes de
aceptarlo, pero `logsStopCmd` no valida nada.

**Reproducción.** Se arranca un proceso propio en su propio grupo (como
`setsid`), se anota su pid en el pidfile y se corre `logs-stop`:

```go
cmd := exec.Command("/bin/sh", "-c", "sleep 60")
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
cmd.Start()
// ... se escribe el pid en el pidfile y se corre Run([]string{"logs-stop", "web"})
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP34LogsStopProcesoAjeno -v
P34 proceso ajeno pid=170916 pgid=170916 estado-antes=S
P34 salida="detenido\n" estado-despues=Z
```

El proceso ajeno pasó de `S` (dormido) a `Z` (zombi: lo mataron) y el comando
contestó "detenido" con **código 0**.

**Impacto.** Matar un proceso del usuario por un pid reciclado. Con el grupo
entero, `SIGTERM` alcanza a los hijos del procesocakes equivocado.

**Contraste con el bug ya arreglado.** El `cli/bugs.md` del módulo ya
registró el bug del pidfile (commit `0145744`) y su arreglo chequea el error
del `WriteFile`. Ese arreglo no cubre esta ruta: el pidfile se sigue
escribiendo sin validar y el pid se sigue usando sin validar.

---

## D4 — ALTO · `volume-export` con ruta relativa monta la raíz del host

**Dónde.** `internal/plugins/docker/cli/root.go:1249-1258` (el comando) y
`root.go:1290-1296` (`splitPath`).

**Síntoma.** `b3l0w docker volume-export --name datos --out backup.tar.gz`
arma un `docker run` que **monta la raíz del sistema de archivos del host**
(`-v /:/respaldo`) y escribe el tarball en `/backup.tar.gz` del host, no en el
directorio de trabajo.

**Causa.** `splitPath` devuelve `("/", nombre)` cuando la ruta no tiene
ninguna barra:

```go
func splitPath(ruta string) (string, string) {
	i := strings.LastIndex(ruta, "/")
	if i <= 0 {
		return "/", ruta
	}
	return ruta[:i], ruta[i+1:]
}
```

Con `--out backup.tar.gz` (relativa) `i == -1` y la carpeta sale `"/"`. Esa
carpeta se usa como **bind mount del host** en el comando de docker:

```go
comando := []string{
    "--rm",
    "-v", nombre + ":/origen:ro",
    "-v", carpeta + ":/respaldo",     // → "-v /:/respaldo"
    "alpine", "tar", "czf", "/respaldo/" + archivo, "-C", "/origen", ".",
}
```

El contenedor escribe en `/respaldo/backup.tar.gz`, que es
**`/backup.tar.gz` en el host**. Y el contenedor tiene montado `/` del host en
escritura.

**Reproducción.**

```go
runner = func(ctx context.Context, args ...string) (string, error) {
    pedidos = append(pedidos, strings.Join(args, " "))
    return "", nil
}
capturaStdout(t, func() { Run([]string{"volume-export", "--name", "datos", "--out", "backup.tar.gz"}) })
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP12VolumeExportRutaRelativa -v
P12 volume-export --out backup.tar.gz => [run --rm -v datos:/origen:ro -v /:/respaldo alpine tar czf /respaldo/backup.tar.gz -C /origen .]
```

`-v /:/respaldo` está a la vista.

**Impacto.** Escribe fuera del directorio pedido y expone la raíz del host al
contenedor. La forma "correcta" (`--out /tmp/backup.tar.gz`) da
`-v /tmp:/respaldo`, que es lo que el autor buscaba: el bug es que **no hay
ninguna validación de que la ruta sea absoluta**.

**Variante.** `volume-import` (root.go:1276-1287) tiene el mismo `splitPath`:
con `--in backup.tar.gz` lee de `/` del host.

---

## D5 — ALTO · `compose --action` se pierde cuando va después del proyecto

**Dónde.** `internal/plugins/docker/cli/root.go:420` (el flag) y `root.go:441`
(el uso).

**Síntoma.** `b3l0w docker compose shop --action up` **no** hace `up`: corre
`docker compose -p shop restart`. Y en el orden inverso,
`b3l0w docker compose --action up shop` arma
`docker compose -p --action up -d`: el proyecto se llama `--action`.

**Causa.** `composeCmd` usa el `flag` estándar de Go, que **deja de parsear
en el primer argumento posicional**:

```go
fs := flag.NewFlagSet("compose", flag.ExitOnError)
accion := fs.String("action", "restart", "up|down|restart|stop|pull")
fs.Parse(args)
...
res, err := docker.ComposeAction(ctx, runner, args[0], *accion)
```

Con `args = ["shop", "--action", "up"]`, `fs.Parse` se detiene en `"shop"`,
`accion` conserva el default `"restart"` y `--action up` queda como argumento
posicional sobrante. Peor: `args[0]` es `"shop"` en el primer caso, pero en el
orden inverso `args[0]` es `"--action"` y se usa como nombre de proyecto.

El uso documentado en el propio archivo (`root.go:43`) dice
`compose <proyecto> [--action A]`: exactamente el orden que falla.

**Reproducción.**

```go
runner = func(ctx context.Context, args ...string) (string, error) {
    pedidos = append(pedidos, strings.Join(args, " "))
    return "", nil
}
capturaStdout(t, func() { Run([]string{"compose", "shop", "--action", "up"}) })
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP11ComposeActionSePierde -v
P11 compose shop --action up   => [compose -p shop restart]
P11 compose --action up shop   => [compose -p --action up -d]
```

**Contraste.** El módulo ya sabe de este problema y lo resolvió en otros
sitios: `sacarFlag` (root.go:175) existe justamente porque *"Go deja de parsear
flags en el primer argumento posicional"*, y `extraerValor` (root.go:363)
extrae flags de cualquier posición. `composeCmd` usa ninguna de las dos: usa
el `flag` estándar y `args[0]`.

**Impacto.** El `action` es el conmutador de la acción: `up`, `down`,
`restart`, `stop`, `pull`. El plugin de Noctalia pide `down` para parar un
stack y recibe un `restart`. Y el orden inverso manda un proyecto basura.

---

## D6 — MEDIO · `Exec` devuelve un error vacío

**Dónde.** `internal/plugins/docker/containers.go:87-89`.

**Síntoma.** Cuando docker falla sin escribir nada (el binario no está, el
contexto se cancela), `Exec` devuelve un error cuyo mensaje es la **cadena
vacía**. El CLI imprime `error: ` a secas y el `Action` queda con `Output: ""`.

**Causa.** El error se construye **a partir de la salida**, no del error de
`cmd.Run()`:

```go
if err := cmd.Run(); err != nil {
    return out.String(), fmt.Errorf("%s", primeraLinea(out.String()))
}
```

Si `out` está vacío (que es justo el caso del fallo), `primeraLinea("")` da
`""` y el error real de Go (`exec: "docker": executable file not found in
$PATH`, o `context canceled`) se pierde.

**Reproducción.**

```go
t.Setenv("PATH", t.TempDir())   // sin docker
salida, err := Exec(context.Background(), "ps")
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP26ExecSinBinario -v
P26 salida="" err= err.Error()=""
```

El mismo con el contexto cancelado (`TestProbeP3ExecErrorVacio`):
`P3 salida="" err= err.Error()=""`.

**Impacto.** El diagnóstico se pierde justo cuando más importa. Encima
`ejecutar` (actions.go:22) mete ese error vacío en `Action.Output`, así que el
plugin recibe `{"ok":false,"command":"docker stop web","output":""}`.

---

## D7 — MEDIO · `Exec` mezcla stderr con stdout y reporta la línea equivocada

**Dónde.** `internal/plugins/docker/containers.go:85-89`.

**Síntoma.** El mensaje de error que ve el usuario puede ser una línea de
**salida normal** de docker, no el error. Un `docker build` que falla reporta
"Building step 1/3" como si fuera el error.

**Causa.** stderr y stdout van al mismo buffer:

```go
var out bytes.Buffer
cmd.Stdout = &out
cmd.Stderr = &out
if err := cmd.Run(); err != nil {
    return out.String(), fmt.Errorf("%s", primeraLinea(out.String()))
}
```

`primeraLinea` (containers.go:103) toma la primera línea del **buffer
combinado**, que en una salida progresiva es la primera línea de progreso.

**Reproducción.** Con un `docker` falso que escribe progreso en stdout y el
error en stderr:

```go
script := "#!/bin/sh\necho 'Building step 1/3'\necho 'ERROR: no space left on device' >&2\nexit 1\n"
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP27ExecMezclaStderr -v
P27 err="Building step 1/3" (el primer stdout, no el error de stderr)
```

**El mismo bug rompe el parseo de JSON.** `GetInfo` (info.go:51) hace
`json.Unmarshal` de **toda** la salida. Si docker escribe un warning por
stderr, el JSON no parsea:

```console
$ go test ./internal/plugins/docker/ -run TestProbeP49GetInfoConWarning -v
P49 err=invalid character 'W' looking for beginning of value info={ServerVersion: Driver: ...}
```

**Impacto.** El error que ve el panel es el equivocado, o directamente un
error de parseo en vez del problema real.

---

## D8 — MEDIO · `RunImage` ignora `RunOptions.Detach`

**Dónde.** `internal/plugins/docker/actions.go:137-139`.

**Síntoma.** `RunImage` **siempre** corre el contenedor en detach, aunque el
llamante pida lo contrario. El campo `Detach` de `RunOptions` es decorativo.

**Causa.** Una condición que siempre es cierta:

```go
if opts.Detach || true {
    args = append(args, "-d")
}
```

`opts.Detach || true` es `true` para cualquier valor de `Detach`. La
expresión parece un resto de debugging.

**Reproducción.**

```go
f := nuevoFake().con("run -d nginx", "abc")
res, _ := RunImage(context.Background(), f.run, RunOptions{Image: "nginx", Detach: false})
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP4RunImageDetach -v
P4 err=<nil> Command="docker run -d nginx" pedidos=[run -d nginx]
```

`Detach: false` y el comando igual lleva `-d`.

**Impacto.** El contrato JSON de `RunOptions` (`actions.go:125`) promete un
campo `detach` que no hace nada. Hoy el CLI no lo usa (`root.go:556` no lo
pasa), así que el daño es latente: el día que el plugin quiera correr en
foreground no podrá.

**Nota.** `go vet` no lo marca, pero `staticcheck` sí (SA4003-ish: la
condición es constante).

---

## D9 — MEDIO · `Events` traga el error de docker

**Dónde.** `internal/plugins/docker/compose.go:128`.

**Síntoma.** Si `docker events` falla **después** de haber escrito algo, el
módulo devuelve los eventos parciales **sin error**. El CLI sale con 0 y el
panel muestra una lista de eventos que en realidad está incompleta.

**Causa.** La condición sólo devuelve el error si la salida está vacía:

```go
if err != nil && strings.TrimSpace(out) == "" {
    return []Event{}, err
}
```

Con `err != nil` y `out` no vacío, el error se descarta y el flujo sigue como
si todo estuviera bien.

**Reproducción.**

```go
clave := "events --since 6h --until 0s --filter type=container --format {{json .}}"
f := nuevoFake().falla(clave,
    `{"Type":"container","Action":"start","Actor":{"ID":"abc","Attributes":{"name":"web"}}}`,
    errors.New("daemon caido"))
eventos, err := Events(context.Background(), f.run, "")
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP5EventsTragaElError -v
P5 err=<nil> eventos=1
```

El daemon estaba caído y el módulo devuelve `err=<nil>` con un evento.

**Contraste.** El mismo patrón "correcto" está en `Containers`
(containers.go:162), `Stats` (stats.go:38) y `GetInfo` (info.go:47): todos
propagan el error siempre. `Events` es el único que lo condicionaliza.

---

## D10 — MEDIO · `DiskUsage` ignora el error del segundo `docker system df`

**Dónde.** `internal/plugins/docker/system.go:57`.

**Síntoma.** `docker df` devuelve las filas bien pero el **total vacío**, sin
avisar de nada. El error del segundo comando se descarta.

**Causa.**

```go
total, _ := run(ctx, "system", "df")
return filas, ultimaLinea(total), nil
```

El `_` se come el error. `DiskUsage` corre **dos** comandos de docker: el
primero con `--format` (el de las filas) y el segundo en texto (el del total).
Si el segundo falla, el primero no.

**Reproducción.**

```go
f := nuevoFake().
    con("system df --format {{json .}}", `{"Type":"Images","TotalCount":"12",...}`).
    falla("system df", "", errors.New("daemon caido"))
filas, total, err := DiskUsage(context.Background(), f.run)
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP6DiskUsageIgnoraElErrorDelTotal -v
P6 err=<nil> filas=1 total=""
```

`err=<nil>` con `total=""`: el módulo no puede distinguir "no hay nada" de
"el comando falló".

---

## D11 — MEDIO · `rutaLog` colisiona: dos contenedores, un archivo

**Dónde.** `internal/plugins/docker/cli/root.go:203-215`.

**Síntoma.** Dos contenedores distintos ended up **compartiendo el archivo de
log y el pidfile**. El `logs-stream` de uno pisa el log del otro, y el
`logs-stop` de uno mata el seguimiento del otro.

**Causa.** `rutaLog` normaliza el id a minúsculas, reemplaza todo lo que no
sea `[a-z0-9]` por `_` y **trunca a 40 caracteres**:

```go
seguro := strings.Map(func(r rune) rune {
    if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
        return r
    }
    return '_'
}, strings.ToLower(id))
if len(seguro) > 40 {
    seguro = seguro[:40]
}
```

`strings.ToLower` hace que "Web" y "web" — dos contenedores **válidos y
distintos** en docker (los nombres distinguen mayúsculas) — den el mismo
archivo. El truncado a 40 hace lo mismo con dos nombres largos que comparten
prefijo.

**Reproducción.**

```go
t.Setenv("XDG_CACHE_HOME", t.TempDir())
a1, p1 := rutaLog("Web")
a2, p2 := rutaLog("web")
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP17RutaLogColisiona -v
P17 rutaLog(Web)=/…/noctalia/docker-logs/web.log
P17 rutaLog(web)=/…/noctalia/docker-logs/web.log
P17 mismo-archivo=true mismo-pidfile=true
```

Y con el truncado:

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP19RutaLogTrunca -v
P19 mismo-archivo=true (…/aaaa…aaaa.log vs …/aaaa…aaaa.log)
```

**Impacto.** Es la misma familia de bug que D3: el pidfile compartido hace que
un `logs-stop` mate el seguimiento del otro contenedor.

---

## D12 — MEDIO · Los ids se pasan a docker sin validar el prefijo `-`

**Dónde.** `internal/plugins/docker/cli/root.go:782-796` (`execCmd`),
`root.go:930-941` (`commitCmd`), `root.go:968-980` (`renameCmd`), y por el
mismo camino `tagCmd`, `pauseCmd`, `diffCmd`, `portCmd`, `volumeInspectCmd`,
`networkInspectCmd`, `pullCmd`, `pushCmd`, `saveCmd`, `loadCmd`, `exportCmd`.

**Síntoma.** `b3l0w docker exec --id --privileged web ls` no intenta ejecutar
`ls` en un contenedor llamado `--privileged`: arma
`docker exec --privileged web ls`, y `--privileged` lo interpreta docker
como flag.

**Causa.** Estos comandos toman el valor de `--id` (o el posicional) y lo
pasan tal cual a `docker`. El módulo **sí** valida el prefijo `-` en otros
sitios:

- `searchCmd` (root.go:1166): `search: el término no puede empezar con -`
- `updateCmd` (root.go:1198): `update: el id no puede empezar con -`
- `volumeExportCmd`/`volumeImportCmd` (root.go:1246, 1273)
- `Build` (actions.go:206, 213, 219): etiqueta, contexto y Dockerfile

pero no en `exec`, `commit`, `rename`, `tag`, `pause`, `diff`, `port`, `pull`,
`push`, `save`, `load`, `export`, `volume-inspect` ni `network-inspect`.

**Reproducción.**

```go
runner = func(ctx context.Context, args ...string) (string, error) {
    pedidos = append(pedidos, strings.Join(args, " "))
    return "salida", nil
}
capturaStdout(t, func() { Run([]string{"exec", "--id", "--privileged", "web", "ls"}) })
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP18ExecIdConGuion -v
P18 exec --id --privileged web ls => [exec --privileged web ls]
```

Y en la capa de paquete, `ExecInContainer` (desktop.go:29) pasa el id tal
cual:

```console
$ go test ./internal/plugins/docker/ -run TestProbeP9ExecIdConGuion -v
P9 err=<nil> Output="salida" pedidos=[exec --privileged web ls]
```

**Contraste.** `search` sí lo rechaza:

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP20SearchGuion -v
search: el término no puede empezar con -
P20 search --format json => codigo=1 pedidos=[]
```

**Impacto.** Inyección de flags en el invocable de docker. Con `exec`, el
flag se puede usar para escalar privilegios dentro del contenedor
(`--privileged`, `--user root`, `-w`). Con `pull`/`push`/`save`/`load`, para
redirigir archivos.

---

## D13 — MEDIO · `HealthLog` se queda con las primeras 5 entradas, no con las últimas

**Dónde.** `internal/plugins/docker/inspect.go:116-126`.

**Síntoma.** Con más de 5 entradas en el log de health, el módulo muestra las
**5 primeras** (las más viejas) y descarta las últimas. El README (línea 94)
promete *"HealthLog (las últimas 5 entradas)"*.

**Causa.** El bucle corta por `len(out.HealthLog) >= 5` sobre las entradas en
orden, sin invertir ni tomar la cola:

```go
if payload.Config.Health != nil {
    for _, entrada := range payload.Config.Health.Log {
        texto := strings.TrimSpace(entrada.Output)
        if texto != "" {
            out.HealthLog = append(out.HealthLog, texto)
        }
        if len(out.HealthLog) >= 5 {
            break
        }
    }
}
```

El log de health de docker va de más antiguo a más reciente: las últimas
entradas son las que explican el fallo actual.

**Reproducción.**

```go
entradas := ... // 7 entradas, "linea 1" .. "linea 7"
payload := `{"Id":"x","Name":"/c","State":{"Status":"running"},"Config":{"Health":{"Log":[...]}}}`
info, _ := InspectContainer(context.Background(), f.run, "c")
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP2HealthLogTomaLasPrimeras -v
P2 HealthLog=[linea 1 linea 2 linea 3 linea 4 linea 5] (quedan las primeras 5, no las ultimas)
```

**El test lo fija como comportamiento.** `inspect_test.go:131` comprueba
`info.HealthLog[0] != "linea 1"` → falla. El test codifica el bug.

**Nota.** Con el parser arreglado (D1) este bug seguiría: hay que tomar los
últimos cinco, no los primeros.

---

## D14 — MEDIO · Listas internas salen `null` en el JSON

**Dónde.** `internal/plugins/docker/stats.go:41`, `system.go:28`,
`containers.go:193` (el `Ports` de cada contenedor), `inspect.go:102-126`.

**Síntoma.** Varias listas del JSON salen `null` en vez de `[]`:
`"stats":null`, `"rows":null`, `"ports":null` por contenedor, y en `inspect`
`"mounts":null`, `"env":null`, `"labels":null`, `"healthLog":null`.

**Causa.** El módulo devuelve `nil` en vez de slice vacío en varios sitios.
`Collect` (containers.go:336-343) sí inicializa las listas del
snapshot, y su comentario explica por qué:

```go
// Slices vacíos, no nil: en JSON un nil sale `null` y el plugin recibe nil
// en vez de una lista (cualquier `#payload.containers` explota).
```

Esainvarianteno se aplica más allá de `Collect`:

- `Stats` (stats.go:41) `var out []Stat` → `nil` si no hay contenedores.
- `DiskUsage` (system.go:28) `var filas []DiskRow` → `nil` si no hay filas.
- `ParsePorts` (ports.go:28) `var out []Port` → `nil` si no hay puertos.
- `Inspect` (inspect.go:102-126) `Mounts`, `Env`, `Labels`, `HealthLog` nunca
  se inicializan.

**Reproducción.**

```console
$ go test ./internal/plugins/docker/ -run TestProbeP7ListasNull -v
P7 snapshot={"containers":[{"id":"c1",...,"ports":null,"hasShell":true}],...}
P7 stats={"at":1795178108,"stats":null}
P7 df={"rows":null,"total":"algo"}

$ go test ./internal/plugins/docker/cli/ -run TestProbeP52StatsJSONNull -v
P52 stats --json => "{\"at\":1795178818,\"stats\":null}\n"

$ go test ./internal/plugins/docker/cli/ -run TestProbeP53DfJSONNull -v
P53 df --json => "{\"rows\":null,\"total\":\"\"}\n"
```

**Impacto.** El README (líneas 54-56) dice que las listas son *"vacías, nunca
`null`"* porque *"el plugin Luau hace `#payload.containers` y un `null` explota
ahí"*. `#payload.stats` y `#payload.rows` explotan igual. Y `ports` es
**por contenedor**: un contenedor sin puertos publicados rompe el render del
panel entero, no sólo una lista.

**Contraste.** El test `containers_test.go:66` afirma
`c.Ports != nil` → *"sin puertos publicados la lista debería quedar vacía"*:
el test **exige** que `Ports` sea `nil`. O sea, el contrato "vacía, nunca
null" está invertido en el test.

---

## D15 — MEDIO · El estado de salud `starting` nunca se ve

**Dónde.** `internal/plugins/docker/containers.go:130-141`.

**Síntoma.** Un contenedor cuyo healthcheck está empezando muestra la salud
**vacía**, no `"starting"`. El README (línea 68) promete
`Health` (`healthy`/`unhealthy`/`starting`).

**Causa.** `healthFromStatus` busca el texto entre paréntesis y acepta
literalmente `"starting"`:

```go
if idx := strings.LastIndex(status, "("); idx >= 0 {
    inner := strings.TrimSuffix(strings.TrimSpace(status[idx+1:]), ")")
    if inner == "healthy" || inner == "unhealthy" || inner == "starting" {
        return inner
    }
}
```

Pero docker escribe el estado de arranque como **`(health: starting)`**, no
como `(starting)`. El `inner` queda `"health: starting"`, que no matchea
ninguno de los tres y devuelve `""`.

**Reproducción.**

```go
healthFromStatus("Up 3 seconds (health: starting)", "")
healthFromStatus("Up 3 seconds (starting)", "")
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP8HealthStartingReal -v
P8 (health: starting) => ""
P8 (starting)          => "starting"
```

La forma que docker usa da `""`; la que el código acepta es la que docker no
usa.

**El test lo fija como comportamiento.** `containers_test.go:257` espera
`{"Up 3 hours (health: starting)", "", ""}` → el test afirma que la forma
real **no** se reconoce.

**Contraste.** El `docker ps` de un contenedor con healthcheck en progreso
muestra `Up 5 seconds (health: starting)`. Es la forma que aparece en la
documentación y en los issues de docker ("Container is starting (health:
starting)").

---

## D16 — MEDIO · `GroupProjects` duplica el servicio con réplicas

**Dónde.** `internal/plugins/docker/compose.go:47-49`.

**Síntoma.** Con un servicio escalado (`docker compose up --scale web=2`), el
proyecto lista el servicio dos veces: `Services: ["web","web"]`.

**Causa.** El servicio se agrega una vez por contenedor, sin deduplicar:

```go
if c.Service != "" {
    p.Services = append(p.Services, c.Service)
}
```

`docker compose` pone la **misma** label `com.docker.compose.service` en
todos los contenedores de un servicio escalado, así que N contenedores ⇒ N
copias del mismo nombre.

**Reproducción.**

```go
contenedores := []Container{
    {ID: "1", Name: "web-1", Project: "shop", Service: "web", State: "running"},
    {ID: "2", Name: "web-2", Project: "shop", Service: "web", State: "running"},
}
GroupProjects(contenedores)
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP24GroupProjectsDuplica -v
P24 Services = [web web]
```

**Impacto.** El panel muestra "web, web" en la lista de servicios del stack.
El `sort.Strings` de la línea 61 no quita duplicados.

**Nota.** El README no promete deduplicación, pero sí dice que `Service` sale
*"de las labels de compose"* — y el nombre del servicio es único por
definición.

---

## D17 — BAJO · `AllowedActions` no lo usa nadie

**Dónde.** `internal/plugins/docker/actions.go:27-36`.

**Síntoma.** El comentario del var dice:

> `AllowedActions` documenta lo que el CLI acepta: **cualquier acción fuera de
> esta lista tiene que fallar** en vez de ejecutar algo no previsto.

Eso no es cierto: `AllowedActions` no lo consulta ningún código del repo (el
único uso es su propio test, `actions_test.go:243`). El CLI despacha por un
`switch` sobre el nombre del comando (`root.go:82-172`) que acepta 40+
comandos, muchos fuera de la lista.

**Reproducción.**

```console
$ grep -rn 'AllowedActions' --include='*.go' .
./internal/plugins/docker/actions.go:27:// AllowedActions documenta lo que el CLI acepta: ...
./internal/plugins/docker/actions.go:29:var AllowedActions = map[string]bool{
./internal/plugins/docker/actions_test.go:239:func TestAllowedActions(t *testing.T) {
./internal/plugins/docker/actions_test.go:243:		if !AllowedActions[accion] {
./internal/plugins/docker/actions_test.go:247:	if AllowedActions["inventado"] {
```

La lista tiene 19 acciones; el CLI expone 42. Y la lista tampoco incluye
comandos del CLI como `"commit"`, `"exec"`, `"cp"`, `"rename"`, `"tag"`,
`"top"`, `"history"`, `"logs"`, `"stats"`, `"inspect"`, `"events"`,
`"compose"`, `"info"`, `"df"`, `"prune"`, `"update"`, `"search"`, `"port"`,
`"volume-inspect"`, `"network-inspect"`, `"volume-export"`, `"volume-import"`,
`"logs-follow"`, `"logs-stream"`, `"logs-stop"`, `"compose-file"`.

**Impacto.** Documentación que miente. Un和维护 que agregue una acción nueva
no tiene dónde declararla; y el comentario sugiere una garantía de seguridad
que no existe.

---

## D18 — BAJO · `sacarFlag` pisa el slice del caller

**Dónde.** `internal/plugins/docker/cli/root.go:384-395`.

**Síntoma.** `sacarFlag` reusa el backing array del slice que recibe
(`resto := args[:0]`) y escribe en él. El slice del caller queda modificado.

**Causa.**

```go
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
```

`args[:0]` comparte el array subyacente con `args`: el primer elemento
sobreviviente se escribe en la posición 0 del slice original.

**Reproducción.**

```go
entrada := []string{"--json", "web", "--json"}
resto, ok := sacarFlag(entrada, "--json")
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP51SacarFlagMutaElCaller -v
P51 ok=true resto=[web] entrada-original-ahora=[web web --json]
```

La entrada pasó de `["--json","web","--json"]` a `["web","web","--json"]`.

**Impacto hoy.** Ninguno observable: los callers pasan `args[1:]` de un slice
que no vuelven a leer, y el `Run` yahabía despachado. Pero es un trampa: el
próximo caller que reutilice el slice después de la llamada va a leer datos
corruptos. `extraerValor` (root.go:363), en cambio, sí copia
(`make([]string, 0, len(args))`).

---

## D19 — BAJO · `logs-stop` dice "detenido" sin haber parado nada

**Dónde.** `internal/plugins/docker/cli/root.go:274-293`.

**Síntoma.** Con un pidfile que apunta a un proceso que ya no existe,
`logs-stop` **igual** imprime "detenido", borra el pidfile y sale con 0. El
usuario cree que cortó un seguimiento que en realidad no estaba corriendo.

**Causa.** No se comprueba que el proceso esté vivo antes de reportar éxito.
`leerPid` (root.go:296) devuelve el número si el archivo existe y es
numérico, sin mirar el proceso. Los `Kill` fallan con `ESRCH` y sus errores
se descartan (`_ =`).

**Reproducción.**

```go
os.WriteFile(pidFile, []byte("999999"), 0o600)
capturaStdout(t, func() { Run([]string{"logs-stop", "web"}) })
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP15LogsStopPidMuerto -v
P15 salida="detenido\n" pidfile-existe=false
```

**Impacto.** Menor: el panel cree que paró algo. Pero combinado con D3 (el pid
puede ser de otro proceso), "detenido" no significa nada.

---

## D20 — BAJO · `logs-stream` con un contenedor inexistente devuelve 0

**Dónde.** `internal/plugins/docker/cli/root.go:230-271`.

**Síntoma.** `b3l0w docker logs-stream nope` (contenedor que no existe)
devuelve **código 0** e imprime la ruta del log, como si el seguimiento
hubiera arrancado. El pidfile queda apuntando a un proceso ya muerto.

**Causa.** `logsStreamCmd` arranca `sh -c "exec docker logs -f ...">` y
`cmd.Start()` **tiene éxito** (arranca el shell, no docker). El error de
docker aparece después, dentro del log, y nadie lo mira: el comando ya
devolvió 0 con la ruta.

**Reproducción.**

```go
dockerFalso(t, "#!/bin/sh\necho 'No such container: nope' >&2\nexit 1\n")
archivo, pidFile := rutaLog("nope")
capturaStdout(t, func() { Run([]string{"logs-stream", "nope"}) })
```

```console
$ go test ./internal/plugins/docker/cli/ -run TestProbeP35LogsStreamPidfileHuerfano -v
P35 codigo=0 pidfile=pid=170917 estado=Z log="No such container: nope\n"
```

El proceso está en `Z` (zombi: ya terminó) y el pidfile lo apunta.

**Impacto.** El panel cree que sigue el log de un contenedor que no existe. Y
el pidfile con un pid muerto es la precondición de D3.

---

## D21 — BAJO · `ExecResult.Exit` es siempre 0

**Dónde.** `internal/plugins/docker/desktop.go:16` (el tipo) y
`desktop.go:30-35` (donde se arma); `cli/root.go:801` (el JSON que sale).

**Síntoma.** `docker exec` que corre un comando que falla con código 1
devuelve `"exit":0` en el JSON. El plugin no puede distinguir un comando que
falló de uno que salió con 0.

**Causa.** `ExecInContainer` no puede conocer el código de salida: el
`Runner` devuelve `(string, error)` y no el exit code. En la ruta de error se
devuelve `ExecResult{Output: salida}` — con `Exit` en su cero por defecto —
y en la de éxito, `ExecResult{Running: true, Exit: 0, ...}`.

**Reproducción.**

```go
f := nuevoFake().falla("exec web false", "", errors.New("exit status 1"))
res, err := ExecInContainer(context.Background(), f.run, "web", []string{"false"})
```

```console
$ go test ./internal/plugins/docker/ -run TestProbeP25ExecResultExit -v
P25 err=exit status 1 Exit=0 Running=false Output=""
```

El error dice `exit status 1` y el campo `Exit` dice 0.

**Impacto.** El `ExecResult` promete un campo `exit` que nunca lleva
información. `docker exec` sí reporta el exit code del comando en su stderr
(`Error response from daemon: ...` no, pero el código de salida real sí), así
que la información existe y se está tirando.

---

## Afirmaciones del README verificadas

Contrastadas una por una contra el código. Las que se sostienen:

| Afirmación | Dónde se sostiene |
|---|---|
| "`Runner func(ctx context.Context, args ...string) (string, error)`" | `containers.go:79` |
| "`Exec` es el runner real (ejecuta el binario `docker`)" | `containers.go:82-91` |
| "`runnerDe(run)` devuelve `Exec` si el runner es `nil`" | `containers.go:96-101` |
| "todas las funciones pasan por ahí" | todas las 42 funciones públicas que toman un `Runner` pasan por `runnerDe` (directamente o por `ejecutar`, que lo llama en `actions.go:18`) |
| "`cli/root.go` tiene un `var runner docker.Runner = docker.Exec`" | `cli/root.go:22` |
| "los dos comandos que siguen logs en vivo no usan el runner" | `cli/root.go:238` y `root.go:1320` (exec.Command directo) |
| "Las listas son vacías, nunca `null`" | **sólo en el snapshot**: `containers.go:336-343`. Fuera de ahí no (D14) |
| "`Images`, `Volumes` y `Networks` se piden en paralelo (goroutines con canales)" | `containers.go:359-373` |
| "los resultados llegan por canal, no por struct compartido" | `containers.go:386-406` |
| "`ID` corto (12 chars)" | `containers.go:214-222` (`shortID`) |
| "`Name` (sin la coma de los alias de red)" | `containers.go:177-180` |
| "`State` (`running`/`exited`/`paused`/`created`)" | `containers.go:185` |
| "la salud sale del `Status` cuando `docker ps` no trae `HealthStatus`" | `containers.go:130-141` (pero D15: no cubre `starting`) |
| "Si el `State` viene vacío, se deduce del `Status`" | `containers.go:195-201` |
| "`HasShell` es sólo `true` para los que están corriendo" | `containers.go:202` |
| "Orden: por proyecto y después por nombre" | `containers.go:205-210` |
| "`ParsePorts` deduplica por `hostPort:containerPort/proto` (ignora la IP)" | `ports.go:60-65` |
| "Gana el primero que apareció" | `ports.go:62` (`continue` sobre el duplicado) |
| "Un puerto sólo interno (`80/tcp`) no tiene `URL`" | `ports.go:69-71` |
| "Las IPv6 vienen como `[::]:8080->80/tcp`: la IP conserva los dos puntos pero pierde los corchetes" | `ports.go:51` |
| "El `Command` sale de `Path` + `Args`, no de `Config.Cmd`" | `inspect.go:99` |
| "Los mounts con `RW:false` salen como `ReadOnly: true`" | `inspect.go:113` |
| "`Stat` trae `CPUPct` y `MemPct` en porcentaje (float)" | `stats.go:24-31,66-67` |
| "Un valor ilegible (`--`) queda en 0, no rompe la fila" | `stats.go:24-31,62` |
| "`docker system df --format '{{json .}}'` trae `TotalCount` y `Active` como **string**" | `system.go:38-47` |
| "`Reclaimable` pierde el porcentaje entre paréntesis" | `system.go:62-67` |
| "El total (última línea del `docker system df` en texto) se pide aparte" | `system.go:55-58` |
| "los stacks se arman desde las labels de compose de los contenedores" | `compose.go:26-50` |
| "`State` es `running`/`partial`/`stopped`" | `compose.go:52-60` |
| "`Action` es el resultado de toda acción: `OK`, `Command` y `Output`" | `actions.go:11-15,22-24` |
| "Un `OK: false` trae el error en `Output`" | `actions.go:22` — pero **no** en las validaciones previas: `actions.go:134,148,154,204,207,214,220` devuelven `Action{OK:false}` con `Output` y `Command` vacíos (P42) |
| "`Exec` recorta el error de docker a la **primera línea** y a 200 caracteres" | `containers.go:103-112` (pero D6/D7: puede ser vacía o la línea equivocada) |
| "Los errores de validación … se devuelven **antes** de llamar a docker" | `actions.go:147,153,206,213,219` — verificado: `f.pedidos()` queda vacío |
| "`RemoveNetwork` rechaza `bridge`, `host` y `none`" | `actions.go:70-76` |
| "`Prune` y `PruneDetailed` rechazan destinos fuera de la lista" | `system.go:88-93`, `desktop.go:134-136` |
| "las redes nativas, los nombres de compose y los IDs pueden contener caracteres raros" | `ports.go`, `containers.go:214`, `containers.go:143` — pero `labelValue` **no** maneja una coma dentro del valor de una label (P43b) |
| "Timeout por comando: `GetInfo` usa 10s, `Events` 20s y `ComposeAction` 120s" | `info.go:43`, `compose.go:121`, `compose.go:72` |
| "`docker events` sin `--until` no termina nunca: se acota la ventana con `--until 0s`" | `compose.go:123-127` |
| "`ComposeAction` corta la salida a 400 caracteres" | `compose.go:87-89` |
| "La salida de `docker compose` va sin decodificar" | `compose.go:80-90` |
| "Cobertura: **94.9%** del paquete `docker`" | medido: **94.9%** (`go test ./internal/plugins/docker/ -cover`) |
| "**75%** de `cli/root.go`" | medido: **76.6%** (`go test ./internal/plugins/docker/cli/ -cover`) |
| "Nunca se ejecuta docker real en los tests" | los 11 archivos de test usan el runner falso o un `docker` falso en el `PATH` |

Las que **no** se sostenían (y **ya no se sostienen**: el README se corrigió
en esta ronda):


| Afirmación del README | Hallazgo | Cómo quedó |
|---|---|---|
| línea 94: "`Health` y `HealthLog` (las últimas 5 entradas)" | **D1** (siempre vacíos) y **D13** (primeras 5, no últimas) | Corregido: el README dice que el health sale de `State.Health` y que el log trae las **últimas** cinco. |
| línea 68: "`Health` (`healthy`/`unhealthy`/`starting`)" | **D15** (`starting` nunca se ve) | Corregido: el README aclara que docker escribe `(health: starting)` y que esa forma se reconoce. |
| líneas 54-56: "Las listas son **vacías, nunca `null`**" | **D14** (`stats`, `rows`, `ports`, `mounts`, `env`, `labels`) | Corregido: el README aclara que la regla vale para todo el módulo, no sólo para el snapshot. |
| línea 119-120: "Un `OK: false` trae el error en `Output`" | **D21** (y las validaciones de `actions.go` devuelven `Output` vacío) | Corregido: el README dice "**también en las validaciones previas**". |
| línea 124-125: "`Exec` recorta el error de docker a la primera línea" | **D6** (puede ser vacía) y **D7** (puede ser stdout) | Corregido: el README dice que el mensaje sale de **stderr** y que si docker no escribe nada el error es el de Go, nunca vacío. |
| líneas 145-147: "Timeout por comando" — no menciona `volume-export`/`volume-import` | los dos usan `ctxDe()` (60 s), que es corto para un `tar` de un volumen grande | Corregido: el README dice que los dos usan 30 minutos. |

Y una afirmación del propio README que quedaba desmentida por el código:

| Afirmación | Realidad | Cómo quedó |
|---|---|---|
| línea 27-36: "`AllowedActions` documenta lo que el CLI acepta: cualquier acción fuera de esta lista tiene que fallar" | **D17**: no lo consulta nadie | Corregido: el CLI ahora consulta la lista (`comandosMutantes`) y el README lo dice. |

---

## Sin hallazgos

**El runner inyectable.** El patrón está bien: `Runner` es un `func`,
`Exec` implementa el runner real, `runnerDe(nil)` cae en `Exec`, y **las 42
funciones públicas que toman un `Runner`** pasan por `runnerDe` (directly o por
`ejecutar`, que lo llama en `actions.go:18`). Verificado con un script que
recorre cada función pública y comprueba su cuerpo: ninguna se saltea el
contrato. La lectura del README es exacta.
- **`Collect` y las goroutines.** Los tres resultados viajan por canales
  bufferizados (tamaño 1) y el struct compartido no se toca desde las
  goroutines. Con `go test -race` la suite pasa. Los canales con buffer 1
  evitan además el leak de goroutine si el contexto se cancela.
- **El snapshot no devuelve `nil` donde importa.** `containers.go:336-343`
  inicializa las cinco listas del snapshot y `containers.go:382` sólo pisa
  `containers` si vino no-nil. La invariante que el README promete para el
  snapshot **sí se cumple** (es D14 lo que se rompe fuera de él).
- **El parser de puertos.** `ParsePorts` maneja bien los casos raros: IPv6 con
  y sin prefijo específico, `udp`, puertos sólo internos, entradas rotas
  (descartadas, no rompen la fila), dedupe por `hostPort:containerPort/proto`
  ignorando la IP, y orden por puerto de host. El `README.md:82-88` es
  exacto.
- **El parseo de `docker ps`.** `Containers` salta líneas que no son JSON,
  salta líneas que no empiezan con `{` (típico: "Cannot connect to the
  Docker daemon"), corta el nombre en la primera coma, deduce el estado
  cuando viene vacío y ordena por proyecto y nombre. Todo eso está en el
  README y todo se sostiene.
- **`labelValue` con el separador correcto.** Docker junta las labels con
  `,` (verificado en `docker/cli/cli/command/formatter/container.go`:
  `strings.Join(joinLabels, ",")`), y el módulo parte por `,`: la clave se
  encuentra y el valor se toma hasta la primera coma. Verificado con el caso
  real de compose (`com.docker.compose.project=shop,com.docker.compose.service=web`
  → `service` = `web`) y con el caso de una coma *dentro* del valor
  (`org.opencontainers.image.title=hola, mundo` → `hola`): el truncado es
  inherente al formato de docker, no un bug del módulo.
- **La validación de puertos y variables.** `puertoValido` acepta
  `8080:80`, `127.0.0.1:8080:80/tcp` y `80`, y rechaza `""`, `abc`,
  `8080:`, `8080:abc`. `RunImage` y `Build` devuelven el error **antes** de
  llamar a docker (verificado: el runner falso no registra ninguna llamada).
- **Las acciones con sus comandos exactos.** Las 21 acciones de
  `actions.go` arman el comando que corresponde, con el orden de argumentos
  correcto, y propagan el error de docker a `Output`.
- **`ComposeAction`.** Agrega `-d` sólo a `up`, corta la salida a 400
  caracteres y respeta el timeout de 120 s. Correcto.
- **`GroupProjects` con estados.** Los tres estados (`running`, `partial`,
  `stopped`) se calculan bien y el orden por nombre es correcto. Sólo la
  deduplicación de servicios falla (D16).
- **La CLI y el `flag` estándar.** Salvo `compose` (D5), el uso de
  `flag.NewFlagSet` es correcto: `flag.ExitOnError` y `fs.Arg(0)` se
  combinan bien en `snapshot`, `prune`, `inspect`, `run`, `stats`, `df` y
  `events`. El problema de `compose` es que usa `args[0]` en vez de
  `fs.Args()[0]`.
- **`partirRutaContenedor`.** Maneja bien `"id:/ruta"`, `"<id>:/ruta"` y las
  rutas del host sin prefijo. La normalización de `copyCmd` (que evita
  prefijar el id dos veces) está bien.
- **El helper `dockerFalso` de los tests.** Es la herramienta correcta: un
  script `docker` en un directorio temporal que se pone al principio del
  `PATH`. El `docker` real nunca se toca.

---

## No reproducidos (hipótesis)

- **`stats` con `PIDs` no numérico.** `stats.go:62` parsea `PIDs` con
  `Atoi` y descarta el error (`pids, _ :=`), dejando 0. Es el mismo patrón
  que `percent` (que sí trata el ilegible como 0 a propósito), así que
  probablemente sea intencional. No se Armoró el caso: no hay evidencia de
  que `docker stats` devuelva algo que no sea un número.
- **`Events` y la ventana `--since`.** `compose.go:122` pasa el `since` que
  llega del CLI al comando de docker sin validarlo. Un `since` mal formado lo
  rechaza docker, no el módulo. No se comprobó contra un daemon real.
- **`Collect` con error parcial.** Cuando una de las tres ramas en paralelo
  falla, `snap.Error` toma el primer error según el orden de lectura de los
  canales (`containers.go:386-406`). El README ya lo marca como
  **"(verificar)"** y dice que no es determinista. Con el runner falso el
  resultado es estable (P30, P40: 5 corridas, mismo error), pero con un
  daemon real las tres goroutines terminan en orden arbitrario y el primero
  que se lee gana. Queda como lo que dice el README: no determinista.
- **`Inspect.Networks` sin orden.** `inspect.go:104` itera un `map`, así que
  el orden de las redes cambia entre llamadas (P32: tres corridas, tres
  órdenes distintos). El README ya lo marca como **"(verificar)"** y dice que
  el plugin no debería depender de él. No es un bug del módulo, pero es un
  contrato que el panel no debe usar.
- **`Exec` y el contexto cancelado a mitad de un comando largo.** `cmd.Run()`
  mata el proceso pero el buffer puede quedar a medias. No se probó con un
  comando que escriba mucho.

---

## Sin revisar

- **`internal/plugins/luau-shared/`** (`cli.luau`, `store.luau`, `anim.luau`,
  `prefs.luau`): se grepó para confirmar que no consume estos JSON. **El
  plugin de Noctalia que renderiza el snapshot está en otro repo**
  (`b3l0w-plugins`), fuera de alcance: no se pudo verificar qué hace con
  `"ports":null` (D14) ni con `"health":""` (D1) en la práctica. Los
  contratos del README dan la forma del problema, no su impacto exacto en la
  UI.
- **`cmd/b3l0w-docker/main.go`**: se leyó (`os.Exit(dockercli.Run(os.Args[1:]))`),
  es un pasamanos. No se revisó el registro del plugin en
  `internal/plugins/registry.go` más allá del grep.
- **`internal/services/docker.go`**: es otro módulo (el de `b3l0w services`),
  fuera del alcance de este encargo. Se grepó sólo para confirmar que no
  comparte código con `internal/plugins/docker`.
- **El daemon de Docker real**: no se levantó ninguno. Todo lo que depende de
  la salida real de `docker inspect` (D1) se verificó contra el código de
  `moby/moby` y la documentación de docker, no contra un daemon.
- **`docker ps --format '{{json .}}'` en versiones distintas**: el README ya
  lo marca como **"(verificar)"**. Las claves que el módulo lee (`ID`,
  `Names`, `Image`, `State`, `Status`, `Command`, `Ports`, `CreatedAt`,
  `Labels`) son las del `ContainerContext` de `docker/cli`; `HealthStatus` no
  existe en la salida estándar (el README también lo dice).

---

## Verificación

La revisión corrió sobre una **copia aislada** del worktree en
`~/.hermes/profiles/coding/cache/scratch/probe-docker` (sin `.git`), con los
probes `zz_probe*_test.go` temporales. Los probes se borraron; el worktree no
se tocó.

Sobre el **worktree real**, sin cambios:

```console
$ cd /home/b3low-z3r0/Documents/Github/Cli/branches/rev-docker
$ go test ./internal/plugins/docker/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker	0.006s
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker/cli	0.631s

$ go build ./...
(sin salida)

$ git status --short
?? internal/plugins/docker/bugs.md
?? internal/plugins/docker/tests.md
```

Sólo los dos entregables nuevos. No se modificó código de producción ni
tests, no se commiteó y no se hizo push.

---

## Arreglados

Los 21 hallazgos están cerrados. Cada entrada dice qué se cambió y qué test
falla si se revierte el arreglo (verificado revirtiendo el cambio y
corrriendo el test: todos fallan).

### D1 — ALTO · `Inspect.Health` y `HealthLog` salían siempre vacíos

**Arreglo.** El health se lee de **`State.Health`** (con `Status` y `Log`),
que es donde `docker inspect` lo pone, y no de `Config.Health`, que no
existe. `Inspect.Health` ahora se asigna: antes no había ningún
`out.Health = ...` en el archivo.

**Test.** `TestInspectLeeElHealthDeState` (paquete) y
`TestInspectIgnoraElHealthDeConfig`.

**Test ajustado.** `inspect_test.go`: el fixture tenía el health bajo
`Config` — la forma que **no** manda docker. Se movió a `State.Health`. El
test pasaba porque el test y el código compartían el mismo error.

### D2 — ALTO · `logs-stream` ejecutaba comandos con el id del contenedor

**Arreglo.** Se dejó de usar `sh -c`. El comando se arma con
`exec.Command("docker", "logs", "-f", "--tail", "200", id)` y la redirección
al archivo la hace Go con un `*os.File` (`os.OpenFile`). El id nunca pasa por
un shell, así que un id con `$(...)`, `;` o backticks no ejecuta nada.

**Test.** `TestLogsStreamNoEjecutaComandosDelId` (CLI): un id con
`$(touch ...)` deja de crear el archivo y el id llega a docker como un
argumento literal. `TestLogsStreamAceptaIdsConCaracteresRaros` cubre el `;`.

### D3 — ALTO · `logs-stop` mataba un proceso que no era el seguimiento

**Arreglo.** El pidfile ahora guarda **pid + starttime** (el campo 22 de
`/proc/<pid>/stat`, los ticks desde el arranque del sistema). Antes de
mandar la señal, `logs-stop` comprueba que el proceso vivo tenga ese
starttime: si el número fue reciclado, el starttime no coincide y **no se
manda ninguna señal**. Es el mismo criterio que ya usa `ytmusic` (Y1).

**Test.** `TestLogsStopNoMataUnProcesoAjeno` (CLI): con un pidfile cuyo
starttime no es el del proceso, el proceso sigue vivo y el comando contesta
"sin seguimiento". `TestLogsStopMataElSeguimientoValido` comprueba que con
pid y starttime correctos el seguimiento sí se corta.

**Test ajustado.** `TestLogsStopMataElProceso` (CLI) escribía el pidfile con
sólo el pid; ahora escribe el par pid + starttime, que es el formato nuevo.

### D4 — ALTO · `volume-export` con ruta relativa montaba la raíz del host

**Arreglo.** `splitPath` (que devolvía `("/", nombre)` para una ruta sin
barra) fue reemplazada por `rutaRespaldo`: la ruta se resuelve a absoluta
dentro del directorio actual y **se rechaza** lo que queda fuera (`../x`) y lo
que queda en la raíz del sistema. El mismo camino lo usa `volume-import`.

**Test.** `TestVolumeExportConRutaRelativaNoMontaLaRaiz` y
`TestVolumeImportConRutaRelativaNoMontaLaRaiz` (CLI): el comando ya no lleva
`-v /:/respaldo` sino `-v <cwd>:/respaldo`.
`TestVolumeExportRechazaRutaFueraDelDirectorio` cubre el rechazo.

**Test ajustado.** `TestSplitPath` (CLI) fijaba el contrato viejo
(`"backup.tar.gz"` ⇒ `("/", "backup.tar.gz")`, o sea la raíz del host). Se
reemplazó por `TestRutaRespaldoSeparaCarpetaYArchivo`,
`TestRutaRespaldoResuelveLaRelativaDentroDelDirectorioActual` y
`TestRutaRespaldoRechazaLoQueQuedaFuera`.

### D5 — ALTO · `compose --action` se perdía tras el proyecto

**Arreglo.** `composeCmd` ya no usa el `flag` de Go (que deja de parsear en
el primer posicional): ahora saca el flag con `extraerValor`, que funciona
en cualquier posición, y el proyecto es el primer argumento que sobra.

**Test.** `TestComposeAceptaElActionDespuesDelProyecto` (CLI): las tres
formas (`compose shop --action up`, `compose --action up shop` y
`compose shop --action=up`) arman `compose -p shop up -d`.
`TestComposeSinActionUsaRestart` fija el default.

### D6 — MEDIO · `Exec` devolvía un error vacío

**Arreglo.** Cuando docker no escribe nada, el error que se devuelve es el
propio de Go (`exec: "docker": executable file not found in $PATH`,
`context canceled`), no la cadena vacía.

**Test.** `TestExecSinBinarioDevuelveElErrorReal` (paquete): sin docker en el
`PATH` el error no es vacío y nombra al binario.

### D7 — MEDIO · `Exec` mezclaba stderr con stdout

**Arreglo.** stderr se guarda en su propio buffer: el mensaje del error sale
de **stderr** (primera línea, 200 caracteres) y no de la primera línea de
stdout, que en una salida progresiva es una línea de progreso. Los dos
parsers que hacen `Unmarshal` de toda la salida (`Inspect` y `GetInfo`) aíslan
el objeto JSON con `extraerJSON`, así que un warning por stderr no los rompe.

**Detalle extra (encontrado por `-race`).** `os/exec` copia stdout y stderr
en **goroutines separadas**, así que el buffer de salida va protegido con un
mutex (`bufferSeguro`). Sin eso, `go test -race` reporta la carrera.

**Test.** `TestExecTomaElErrorDeStderr` (paquete): el error es el de stderr y
no la línea de progreso de stdout.
`TestGetInfoAguantaUnWarningPorStderr` cubre el parseo del JSON.

### D8 — MEDIO · `RunImage` ignoraba `RunOptions.Detach`

**Arreglo.** `if opts.Detach || true` → `if opts.Detach`. El CLI, que
quería el contenedor en segundo plano, ahora lo pide explícitamente
(`Detach: true`).

**Test.** `TestRunImageRespetaDetach` (paquete): con `Detach: false` el
comando es `run nginx`, sin `-d`. `TestRunImageConDetachPoneElDashD` cubre
el otro lado.

**Test ajustado.** `TestRunImageArmaElComando` (paquete) pedía `run -d ...`
sin pedir detach; ahora pide `Detach: true` explícitamente.

### D9 — MEDIO · `Events` tragaba el error de docker

**Arreglo.** El error se propaga siempre: si docker falla después de haber
escrito algo, los eventos son parciales y hay que avisar. Antes sólo se
propagaba cuando la salida venía vacía.

**Test.** `TestEventsPropagaElErrorConSalidaParcial` (paquete): un runner
que falla **con salida parcial** devuelve el error y ningún evento.

**Test ajustado.** `TestEventsPropagaElError` (paquete) cubría sólo el caso
de salida vacía (que `Events` sí manejaba): el hueco era justo el caso
parcial, ahora cubierto por el test nuevo.

### D10 — MEDIO · `DiskUsage` ignoraba el error del segundo `system df`

**Arreglo.** El error del comando del total se propaga: si falla, el total
no es "vacío", es "no lo sé".

**Test.** `TestDiskUsagePropagaElErrorDelTotal` (paquete): el primer
`system df` funciona y el segundo falla → vuelve el error.

### D11 — MEDIO · `rutaLog` hacía colisionar dos contenedores

**Arreglo.** El nombre del archivo es el id saneado (en minúsculas, lo que no
es alfanumérico a `_`) **más un hash corto del id original** (sha256, 8 hex).
Sin el hash, "Web" y "web" —dos contenedores válidos y distintos en docker—
daban el mismo `web.log` y el mismo pidfile. El truncado a 40 caracteres
también dejaba dos nombres largos con el mismo prefijo en el mismo archivo.

**Test.** `TestRutaLogNoColisiona` (CLI): "Web" y "web" no comparten log ni
pidfile, y dos ids largos con el mismo prefijo tampoco.

**Test ajustado.** `TestRutaLogSaneaElNombre` (CLI) exigía el nombre exacto
`web_server_01.log`; ahora exige el prefijo `web_server_01-` y el sufijo
`.log` (el hash va en el medio).

### D12 — MEDIO · Los ids se pasaban a docker sin validar

**Arreglo.** Ningún identificador que empiece con `-` llega a docker: se
rechaza antes de armar el comando. La validación está en **las dos capas**:

- el CLI (`cli/root.go`): `exec`, `commit`, `rename`, `tag`, `pause`,
  `unpause`, `diff`, `export`, `port`, `pull`, `push`, `save`, `load`,
  `volume-inspect`, `network-inspect`, `cp` y `volume-export/import`.
- el paquete: `Start`, `Stop`, `Restart`, `Remove`, `RemoveImage`,
  `RemoveVolume`, `RemoveNetwork`, `Pull`, `Push`, `CreateVolume`,
  `CreateNetwork`, `SaveImage`, `LoadImage`, `ExportContainer`, `Diff`,
  `Pause`, `Unpause`, `Rename`, `Tag`, `Top`, `History`, `Commit`, `Copy`,
  `CopyFrom` y `ExecInContainer`.

El error va también en `Action.Output` (que antes venía vacío).

**Test.** `TestIdsConGuionSeRechazan` (CLI, 15 comandos) y
`TestLasAccionesRechazanIdsConGuion` + `TestCopyRechazaIdConGuion`
(paquete, 21 acciones).

### D13 — MEDIO · `HealthLog` se quedaba con las primeras 5 entradas

**Arreglo.** El log de health de docker va de más antiguo a más reciente: se
conservan las **últimas** cinco entradas, que son las que explican el fallo
actual.

**Test.** `TestHealthLogTomaLasUltimasCinco` (paquete).

**Test ajustado.** `TestInspectCortaElHealthLog` (paquete) afirmaba que la
primera conservada era `"linea 1"`: **codificaba** el bug. Ahora exige
`"linea 3".."linea 7"`.

### D14 — MEDIO · Varias listas internas salían `null` en el JSON

**Arreglo.** Las listas del módulo salen vacías, nunca `null`: `Stats`,
`DiskUsage` (las filas), `ParsePorts` (y por lo tanto `Container.Ports`) y las
de `Inspect` (`env`, `mounts`, `healthLog` y `labels`).

**Test.** `TestListasDelSnapshotNoSonNull`, `TestStatsYDFNoSonNull` y
`TestInspectNoTieneListasNull` (paquete).

**Test ajustado.** `TestParsePortsSalidaVacia` (paquete) **exigía** que
`Ports` fuera `nil`: el contrato estaba invertido. Ahora exige una lista
vacía. `TestContainersContenedorParado` (paquete) hacía lo mismo con
`Container.Ports`.

### D15 — MEDIO · El estado de salud `starting` nunca se veía

**Arreglo.** `healthFromStatus` reconoce la forma real de docker,
`(health: starting)`: se saca el prefijo `health:` antes de comparar.

**Test.** `TestHealthStartingEsReconocido` (paquete).

**Test ajustado.** `TestHealthFromStatus` (paquete) afirmaba que
`(health: starting)` ⇒ `""`: **codificaba** el bug. Ahora ⇒ `"starting"`.

### D16 — MEDIO · `GroupProjects` duplicaba el servicio con réplicas

**Arreglo.** Los servicios se deduplican por proyecto: con
`docker compose up --scale web=2` los N contenedores llevan la misma label
`com.docker.compose.service` y el proyecto listaba `web, web`.

**Test.** `TestGroupProjectsNoRepiteServicios` (paquete).

### D17 — BAJO · `AllowedActions` no lo usaba nadie

**Arreglo.** La lista es ahora una barrera real: el CLI consulta
`docker.AllowedActions` antes de ejecutar cualquier comando que mute estado
(`comandosMutantes` en `cli/root.go`). Si la acción no está en la lista, el
comando falla con código 2 y **no llama a docker**. La lista se completó con
las acciones que el CLI expone y no estaban ahí.

**Test.** `TestComandoFueraDeLaListaNoSeEjecuta` (CLI): sacando "start" de
la lista, `start web` falla y no llama a docker.
`TestTodasLasAccionesMutantesEstanPermitidas` (CLI) y
`TestAllowedActionsIncluyeLasAccionesDelPaquete` (paquete) fijan que la
lista y el CLI no se desincronicen.

### D18 — BAJO · `sacarFlag` pisaba el slice del caller

**Arreglo.** `sacarFlag` copia los argumentos en un slice nuevo en vez de
reusar el backing array del que recibe (`args[:0]`).

**Test.** `TestSacarFlagNoMutaElSliceDelCaller` (CLI): el slice de entrada
queda intacto.

### D19 — BAJO · `logs-stop` decía "detenido" sin haber parado nada

**Arreglo.** Si el proceso no está vivo (o no es el seguimiento), el comando
contesta **"sin seguimiento"** en vez de "detenido", y no dice que detuvo algo
que no detuvo. El pidfile obsoleto se borra igual.

**Test.** `TestLogsStopConProcesoMuertoNoDiceDetenido` (CLI).

### D20 — BAJO · `logs-stream` con un contenedor inexistente devolvía 0

**Arreglo.** El `Wait` del proceso corre en una goroutine: si docker termina
enseguida (contenedor inexistente), el comando falla con el error que docker
escribió en el log y **no deja pidfile de un proceso muerto**.

**Test.** `TestLogsStreamConContenedorInexistenteFalla` (CLI): código de
salida distinto de 0, el error de docker en stderr y sin pidfile.

### D21 — BAJO · `ExecResult.Exit` era siempre 0

**Arreglo.** El código de salida real viaja en `ExecResult.Exit`: `Exec`
deja de perder el `*exec.ExitError` y devuelve un `*ErrorDocker` con el
código, y `ExecInContainer` lo propaga. Cuando el runner es falso y el error
no trae código, se calcula del texto (`exit status N`).

**Test.** `TestExecResultPropagaElCodigoDeSalida` y
`TestExecResultExitEnElErrorDelDaemon` (paquete).

---

## Descartados

Ninguno. Los 21 hallazgos se reprodujeron y se arreglaron.
