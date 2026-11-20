# internal/plugins/docker

Plugin de Docker para `b3l0w`. Habla con el daemon y devuelve un snapshot
JSON que el plugin de Noctalia renderiza sin parsear nada.

El plugin original hacía `docker ps --format '{{json .}}'` + `awk`/`cut` por
contenedor y decodificaba de a 25 filas por tick. Acá el trabajo pesado y el
parseo de puertos viven en Go y el service sólo publica estado.

## El runner inyectable

El paquete no llama a `docker` directamente: todo pasa por un `Runner`.

```go
type Runner func(ctx context.Context, args ...string) (string, error)
```

`Exec` es el runner real (ejecuta el binario `docker`). Todas las funciones
públicas lo reciben como argumento, así que un test pasa una func falsa y
nunca toca el daemon:

```go
f := nuevoFake().con("ps --no-trunc --format {{json .}}", lineaJSON)
lista, err := Containers(context.Background(), f.run, false)
```

El runner falso (`fakes_test.go`) indexa por los argumentos unidos por
espacio, así que un comando que no se registró es un error y el test falla en
vez de devolver vacío en silencio.

**Contrato del `nil`**: `runnerDe(run)` (en `containers.go`) devuelve `Exec`
si el runner es `nil`, y todas las funciones pasan por ahí. Antes el
contrato era desigual: tres lo chequeaban (`ComposeAction`, `Events`,
`GetInfo`) y las demás reventaban con un panic. Ahora es el mismo en todas.

**El CLI**: `cli/root.go` tiene un `var runner docker.Runner = docker.Exec`.
Los tests lo reemplazan por una func falsa para probar el despacho de
subcomandos sin Docker. Los dos comandos que siguen logs en vivo
(`logs-follow`, `logs-stream`) no usan el runner: necesitan el proceso real
y su stdout conectado a la terminal, así que se prueban con un script
`docker` falso en el `PATH`.

**`AllowedActions` es una barrera real**: el CLI consulta la lista antes de
correr cualquier subcomando que mute estado (`comandosMutantes` en
`cli/root.go`). Si la acción no está en la lista, el comando falla con
código 2 **sin llamar a docker**.

### El CLI: `logs-stream`, `logs-stop` y las rutas de respaldo

- **`logs-stream` no usa un shell.** El comando se arma con
  `exec.Command("docker", "logs", "-f", "--tail", "200", id)` y la
  redirección al archivo la hace Go con un `*os.File`. El id nunca pasa por
  un shell, así que un id con `$(...)`, `;` o backticks no ejecuta nada.
- **El pidfile lleva pid + starttime** (el campo 22 de `/proc/<pid>/stat`).
  `logs-stop` verifica que el proceso vivo tenga ese starttime antes de
  mandarle la señal: un número de pid reciclado no es el seguimiento y no se
  mata. Sin proceso vivo, `logs-stop` dice "sin seguimiento" (nunca
  "detenido") y borra el pidfile obsoleto.
- **`logs-stream` avisa si docker muere enseguida**: un contenedor inexistente
  sale con código 1 y el error de docker, y no deja pidfile de un proceso
  muerto.
- **El nombre del archivo de log lleva un hash del id original**: el saneo
  solo no alcanza, porque "Web" y "web" son dos contenedores válidos y
  distintos en docker.
- **Las rutas de `volume-export`/`volume-import` se resuelven a absolutas**
  dentro del directorio actual. Una ruta relativa **no** puede convertirse
  en `/` (que era el bind mount de la raíz del host), y lo que queda fuera
  del directorio se rechaza.

## Salidas

### `Snapshot` (`b3l0w docker snapshot`)

```json
{
  "containers": [], "images": [], "volumes": [], "networks": [], "projects": [],
  "running": 0, "stopped": 0, "unhealthy": 0, "at": 1700000000
}
```

Las listas son **vacías, nunca `null`**: el plugin Luau hace
`#payload.containers` y un `null` explota ahí. Por eso `Collect` sólo
reemplaza una lista por la que vino de `docker` si no es `nil`.

`Images`, `Volumes` y `Networks` se piden en paralelo (goroutines con
canales): en serie son cuatro procesos que el panel esperaba uno atrás del
otro. Los resultados llegan por canal, no por struct compartido: escribir el
struct desde tres goroutines es una carrera aunque los campos sean distintos
(lo detecta `go test -race`).

### `Container`

`ID` corto (12 chars), `Name` (sin la coma de los alias de red), `Image`,
`State` (`running`/`exited`/`paused`/`created`), `Status`, `Health`
(`healthy`/`unhealthy`/`starting`), `Uptime`, `Project` y `Service` (de las
labels de compose), `Ports` y `HasShell`.

- La salud sale del `Status` cuando `docker ps` no trae `HealthStatus`:
  `Up 3 hours (healthy)` → `healthy`.
- Si el `State` viene vacío, se deduce del `Status`: `Up ...` → `running`,
  si no → `exited`.
- `HasShell` es sólo `true` para los que están corriendo.
- Orden: por proyecto y después por nombre.

### `Port`

`HostIP`, `HostPort`, `Container`, `Proto` y `URL`.

`ParsePorts` deduplica por `hostPort:containerPort/proto` (**ignora la IP**):
`0.0.0.0:8080` y `[::]:8080` son el mismo puerto publicado y mostraban dos
chips. Gana el primero que apareció. Un puerto sólo interno (`80/tcp`) no
tiene `URL` porque no se abre desde el navegador.

Las IPv6 vienen como `[::]:8080->80/tcp`: la IP conserva los dos puntos pero
pierde los corchetes.

### `Inspect`

`docker inspect --format '{{json .}}'` aplanado: `State`, `RestartCount`,
`RestartPolicy`, `Platform`, `Command` (Path + Args), `Env`, `Labels`,
`Mounts`, `Networks`, `Health` y `HealthLog` (las últimas 5 entradas).

El `Command` sale de `Path` + `Args` (lo que realmente corre), no de
`Config.Cmd` (lo que se pidió). Los mounts con `RW:false` salen como
`ReadOnly: true`.

### `Stat` y `DiskRow`

`Stat` trae `CPUPct` y `MemPct` en porcentaje (float), `MemUsage`, `NetIO`,
`BlockIO` y `PIDs`. Un valor ilegible (`--`, que sale en contenedores recién
arrancados) queda en 0, no rompe la fila.

`docker system df --format '{{json .}}'` trae `TotalCount` y `Active` como
**string**, no como número: con `int` el unmarshal falla y la fila se
descarta en silencio. `Reclaimable` pierde el porcentaje entre paréntesis.
El total (última línea del `docker system df` en texto) se pide aparte.

### `Project` y `Action`

Los stacks se arman desde las labels de compose de los contenedores, no desde
`docker compose ls`: ese comando no dice el estado agregado y obligaría a un
proceso extra por refresco. `State` es `running` (todos corriendo), `partial`
(algunos) o `stopped` (ninguno).

`Action` es el resultado de toda acción: `OK`, `Command` (el comando con su
texto, para que el panel pueda mostrarlo) y `Output`. Un `OK: false` trae el
error en `Output`.

## Errores

La salida de `docker` y su stderr van a buffers separados: `os/exec` los
copia en goroutines distintas, así que el buffer de salida va protegido con
un mutex (`bufferSeguro`). Sin eso, `go test -race` reporta la carrera.

- `Exec` recorta el error de docker a la **primera línea** y a 200 caracteres:
  el stack entero no le sirve al panel. El mensaje sale de **stderr**, no de
  stdout: stdout es la salida normal del comando y su primera línea es
  a menudo una línea de progreso, no el error. Si docker no escribe nada, el
  error es el propio de Go (`exec: "docker": executable file not found in
  $PATH`, `context canceled`), nunca una cadena vacía.
- Los errores de validación (puerto inválido, variable sin `=`, etiqueta de
  build que empieza con `-`) se devuelven **antes** de llamar a docker: el
  comando no se ejecuta y el `Action` viene con `OK: false`.
- `RemoveNetwork` rechaza `bridge`, `host` y `none` con su propio mensaje
  (docker también los rechaza, pero el motivo es más claro).
- `Prune` y `PruneDetailed` rechazan destinos fuera de la lista.
- Las redes nativas, los nombres de compose y los IDs pueden contener
  caracteres raros: `ParsePorts`, `shortID` y `labelValue` los manejan sin
  romperse.

## Decisiones

- **`Runner` inyectable**: hace que el paquete se pueda testear sin daemon.
  Es el patrón que usan los otros plugins del repo. El módulo hermano
  `internal/services` **no** lo tiene (su `DockerAdapter` llama al sistema
  directamente): no se copió de ahí.
- **El `nil` es `Exec`**: un `nil` de runner es un error de programación.
- **Los ids se validan antes de llegar a docker**: un identificador que
  empieza con `-` lo interpretaría docker como un flag suyo (`--privileged`,
  `--user root`) y no como el nombre de un contenedor. Se rechaza en la
  capa del CLI y en la del paquete, y el motivo va en `Output`.
- **Slices vacíos, nunca `nil`**: el plugin Luau indexa las listas sin
  preguntar.
- **Timeout por comando**: `GetInfo` usa 10s, `Events` 20s y `ComposeAction`
  120s. `docker events` sin `--until` no termina nunca: se acota la ventana
  con `--until 0s`.
- **`ComposeAction` corta la salida a 400 caracteres**: el panel muestra un
  resumen, no el log entero.
- **La salida de `docker compose` va sin decodificar**: el plugin la muestra
  como texto.

## Tests

```bash
go test ./internal/plugins/docker/...
go test -race ./internal/plugins/docker/...
```

Cobertura: **96.7%** del paquete `docker` y **79.1%** del subpaquete `cli`.

| Archivo | Qué cubre |
|---|---|
| `fakes_test.go` | El runner falso (no es un test, es la herramienta) |
| `containers_test.go` | `docker ps`/`images`/`volume ls`/`network ls`, `Collect`, salud, uptime, labels, IDs |
| `ports_test.go` | Mapeo de puertos: IPv6, UDP, dedupe, orden, internos, rotos |
| `inspect_test.go` | `docker inspect`: estado, mounts, redes, health log, JSON roto |
| `stats_test.go` | `docker stats`: porcentajes, PIDs, valores ilegibles |
| `system_test.go` | `docker system df`, prune y sus destinos |
| `compose_test.go` | `GroupProjects`, `ComposeAction`, `Events` |
| `actions_test.go` | Todas las acciones, sus validaciones y `Build` |
| `desktop_test.go` | exec, cp, commit, tag, `PrettyBytes`, `GetInfo` |
| `fixes_test.go` | Los arreglos de D1, D6–D10, D12, D13–D16, D17 y D21 |
| `cli/root_test.go` | Helpers del CLI: flags, rutas, pid, logs |
| `cli/commands_test.go` | El despacho completo y la salida JSON de cada comando |
| `cli/fixes_test.go` | Los arreglos de D2–D5, D11, D12, D17–D20 |

**Nunca se ejecuta docker real en los tests.** Todo pasa por el runner
falso. Los dos comandos que necesitan un proceso de verdad (`logs-follow`,
`logs-stream`) usan un script `docker` falso en el `PATH`.

### Bugs y cosas a verificar

- **Corregido**: `Collect` pisaba `containers` con un `nil` cuando no había
  contenedores, y el snapshot mandaba `"containers": null` al plugin. Ahora
  la lista queda vacía.
- **Corregido**: la revisión de integridad de `main` (8c2fdd9) encontró 21
  bugs en este módulo. Están todos arreglados y documentados en `bugs.md`,
  con el test de cada uno.
- **Corregido**: las rutas del respaldo (`volume-export`/`volume-import`)
  se resuelven a absolutas dentro del directorio actual. Antes una ruta
  relativa (`backup.tar.gz`) montaba **la raíz del host** (`-v /:/respaldo`)
  y escribía el tarball en `/`.
- **(verificar)** El formato exacto de `docker ps --format '{{json .}}'`
  depende de la versión del daemon: los tests usan el de Docker 27.x.
- **(verificar)** `HealthStatus` no existe en el `docker ps` estándar; el
  campo está en el struct por si el daemon lo manda, pero el parser de acá
  no lo usa (la salud sale del `Status`).
- **(verificar)** `NetworkSettings.Networks` es un map: el orden de
  `Inspect.Networks` no es estable. El plugin no debería depender de él.
- **(verificar)** El `Error` del snapshot lleva el **primer** error que
  aparece. Las cuatro consultas corren en paralelo, así que cuál es "el
  primero" no es determinista.
