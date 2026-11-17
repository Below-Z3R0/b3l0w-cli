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

- `Exec` recorta el error de docker a la **primera línea** y a 200 caracteres:
  el stack entero no le sirve al panel.
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

Cobertura: **94.9%** del paquete `docker` y **75%** de `cli/root.go`.

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
| `cli/root_test.go` | Helpers del CLI: flags, rutas, pid, logs |
| `cli/commands_test.go` | El despacho completo y la salida JSON de cada comando |

**Nunca se ejecuta docker real en los tests.** Todo pasa por el runner
falso. Los dos comandos que necesitan un proceso de verdad (`logs-follow`,
`logs-stream`) usan un script `docker` falso en el `PATH`.

### Bugs y cosas a verificar

- **Corregido**: `Collect` pisaba `containers` con un `nil` cuando no había
  contenedores, y el snapshot mandaba `"containers": null` al plugin. Ahora
  la lista queda vacía.
- **Anotado, no cambiado**: `splitPath("/backup.tar.gz")` devuelve el
  archivo con la barra pegada, lo que deja una doble barra en el comando de
  `tar`. No rompe nada: `tar` la resuelve igual.
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
