# Tests de `internal/plugins/docker/cli`

Qué prueba cada test del subpaquete. El dominio (`internal/plugins/docker`)
tiene su propia suite; esta cubre **el despacho de subcomandos, los códigos
de salida y la salida JSON**, que es justo lo que el dominio no toca.

## Cómo se aísla

| Auxiliar | Qué hace |
|---|---|
| `conRunnerFalso` | Reemplaza el runner del CLI (`docker.Exec` en producción) por una func que devuelve una salida fija. **El daemon de Docker nunca se toca.** |
| `capturaStdout` / `capturaStderr` | Redirigen `os.Stdout` / `os.Stderr` a un pipe y devuelven lo escrito. La lectura va en una goroutine para que una salida larga no tape al comando. |
| `dockerFalso` | Escribe un script `docker` en un directorio temporal y lo pone al principio del `PATH`. Sirve para los dos comandos que necesitan un proceso real (`logs-follow`, `logs-stream`): **el binario real nunca se ejecuta.** |
| `procesosVivos` | Lee `/proc/*/cmdline` y devuelve los pids cuya línea de comandos menciona alguno de los caminos dados. |
| `procesosDelGrupo` / `leerStat` | Lee `/proc/*/stat` y devuelve los pids del grupo indicado (campo `pgrp`). Un hijo vivo del `docker logs -f` también es un huérfano. |
| `pidAnotado` | Devuelve el pid que el `docker` falso dejó anotado, o 0 si todavía no lo anotó. |
| `limpiarProcesoDocker` | Se lleva por delante al `docker` falso y a su grupo, para que una prueba que falle no deje procesos colgando. |

Ningún test toca la red, el daemon de Docker ni el home del usuario: todo va
a `t.TempDir()`, con `$XDG_CACHE_HOME` y `$PATH` temporales.

## `root_test.go` — helpers del CLI

| Test | Qué fija |
|---|---|
| `TestExtraerValorSacaElFlagYSuValor` | `extraerValor` saca el flag y su valor del medio de los argumentos. |
| `TestExtraerValorAceptaElFormatoConIgual` | `--flag=valor` también sirve. |
| `TestExtraerValorSinElFlag` | Sin el flag devuelve el resto intacto y `ok = false`. |
| `TestSacarFlagQuitaTodasLasOcurrencias` | `sacarFlag` quita **todas** las ocurrencias del flag. |
| `TestSacarFlagSinElFlag` | `sacarFlag` sin el flag deja los argumentos como estaban. |
| `TestPartirRutaContenedor` | `id:/ruta` se parte en id y ruta; una ruta normal queda con el prefijo vacío. |
| `TestSplitPath` | `splitPath` separa archivo y carpeta en rutas absolutas y relativas. |
| `TestRecorta` | `recorta` recorta por runes (no por bytes) y pone la elipsis. |
| `TestListaFlagAcumula` | `listaFlag` acumula valores y su `String` los junta con espacios. |
| `TestRutaLogSaneaElNombre` | `rutaLog` baja a minúsculas, cambia lo no alfanumérico por `_` y limita el nombre. |
| `TestRutaLogAcortaElNombre` | Un id larguísimo no produce un nombre de más de 45 caracteres. |
| `TestLeerPid` | `leerPid` lee un pid, devuelve 0 con un archivo inexistente y también con basura. |
| `TestProcesoVivo` | `procesoVivo` es falso para 0 y para negativos, y verdadero para el proceso actual. |

## `commands_test.go` — despacho, ayuda y códigos de salida

| Test | Qué fija |
|---|---|
| `TestComandoDesconocidoDevuelveDos` | Un subcomando inventado devuelve 2. |
| `TestAyudaDevuelveCero` | `help`, `-h` y `--help` devuelven 0 e imprimen la ayuda. |
| `TestSinArgumentosImprimeLaAyuda` | Sin argumentos se imprime la ayuda y se devuelve 2. |
| `TestSnapshotDevuelveJSON` | `snapshot --json` devuelve JSON **con** la lista `containers` y sin `null` (lo consume el plugin Luau). |
| `TestInspectDevuelveJSON` | `inspect --json` devuelve el `name` del contenedor. |
| `TestStatsDevuelveJSON` | `stats --json` devuelve el `name` y un `at` mayor que 0. |
| `TestPruneConDestinoInvalidoDevuelveDos` | Un destino de prune inválido devuelve 2 y explica el motivo: en JSON como documento, en texto por stderr. |
| `TestControlSinIdDevuelveDos` | `start`, `stop`, `restart` y `remove` sin id devuelven 2. |
| `TestLogsStopSinSeguimiento` | `logs-stop` sin pidfile dice `sin seguimiento` y devuelve 0. |
| `TestLogsStopMataElProceso` | `logs-stop` manda la señal, dice `detenido` y borra el pidfile. |
| `TestComandosSimplesConRunnerFalso` | Todos los subcomandos de una línea consultan el runner y devuelven 0. |
| `TestComandosDeLecturaConRunnerFalso` | `df`, `info`, `events`, `compose ls` y `snapshot` devuelven JSON parseable. |
| `TestVolumeExportEImportArmanElComando` | `volume-export` y `volume-import` arman el comando y devuelven 0. |
| `TestVolumeExportRechazaNombresConGuion` | Un nombre que empieza con `-` se rechaza con 1 (si no, sería un flag). |
| `TestComposeFileLeeElYAML` | `compose-file` imprime el YAML del stack. |
| `TestComposeFileConProyectoInexistente` | Un proyecto inexistente devuelve 1. |
| `TestLogsFollowUsaElTailPorDefecto` | `logs-follow` llama a `docker logs --follow --tail 200`. |
| `TestLogsFollowRespetaElTail` | `logs-follow --tail 50` manda `--tail 50`. |
| `TestLogsStreamMataElProcesoSiNoPuedeGuardarElPid` | Si el pidfile no se puede escribir, `logs-stream` devuelve 1, avisa por stderr y **no deja vivo al `docker logs -f`** ni a los de su grupo (bug corregido, ver `bugs.md`). |
| `TestExecYCPConRunnerFalso` | `exec` y `cp` llaman al runner y devuelven 0. |
| `TestExecSinIdDevuelveUno` | `exec` sin `--id` devuelve 1. |
| `TestBuildSinTagDevuelveUno` | `build` sin `--tag` devuelve 1. |
| `TestCopySinDestinoDevuelveUno` | `cp` sin `--out` devuelve 1. |

## Qué no está cubierto

- **`logs-stream` en el camino feliz.** Que el `docker logs -f` quede
  corriendo con setsid y con el pid guardado se verifica por partes (el
  `logs-stop` que lo mata y el `leerPid` que lo lee), no con un test del
  `logs-stream` que arranque y deje un proceso real detrás. La cobertura
  deliberada del fallo (el pidfile que no se puede escribir) es la que
  importa: es el camino en el que antes quedaba un huérfano.
- **La salida real de `docker logs -f`.** El script falso no produce logs de
  verdad; lo que se prueba es el armado del comando y el ciclo de vida del
  proceso, no el formato de la salida de Docker.
- **La carrera entre el `docker logs -f` que arranca y el `logs-stop` que lo
  mata.** `logs-stop` manda la señal al grupo y al proceso; que el `docker`
  real la obeyezca es cosa de Docker, no del CLI.
