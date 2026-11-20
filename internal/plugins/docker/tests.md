# Tests de `internal/plugins/docker`

Mapa de los tests del módulo: **qué contrato fija cada uno**.

Después de la ronda de arreglos (los 21 hallazgos de `bugs.md`), los tests
existentes que fijaban un contrato **equivocado** se ajustaron al contrato
nuevo —no se rompieron: se corrigió lo que estaba mal— y se agregaron los
tests de cada arreglo en `fixes_test.go`.

Total: **168 tests** en 13 archivos de test, más `fakes_test.go` que no es un
test sino la herramienta.

Cobertura medida sobre `main` en `8c2fdd9`:

```console
$ go test ./internal/plugins/docker/ -count=1 -cover
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker	0.007s	coverage: 96.7% of statements

$ go test ./internal/plugins/docker/cli/ -count=1 -cover
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker/cli	3.916s	coverage: 79.1% of statements
```

## Herramienta

### `fakes_test.go` — el runner falso (no es un test)

El `Runner` inyectable es lo que permite testear sin daemon. `fakeRunner`
indexa por los **argumentos unidos por espacio** y devuelve un error si el
comando no está registrado: un test falla si el código llama algo que no
esperaba, en vez de devolver vacío en silencio.

| Helper | Contrato que fija |
|---|---|
| `nuevoFake()` | Devuelve un runner **sin respuestas**: cualquier comando no registrado es un error. |
| `.con(clave, salida)` | Registra la salida de un comando exacto (clave = args unidos por espacio, en orden). |
| `.falla(clave, salida, err)` | Registra un comando que devuelve error **con** salida parcial. |
| `.llamado(clave)` | Dice si ese comando exacto se pidió. |
| `.pedidos()` | Lista los comandos ejecutados, en orden. |
| `.run(ctx, args...)` | El `Runner` de mentira. **Es seguro para uso concurrente** (`sync.Mutex`): `Collect` lanza tres goroutines contra el mismo runner y un mapa sin candalo explotaría en `go test -race`. |

## `actions_test.go` — las acciones y sus validaciones

| Test | Contrato que fija |
|---|---|
| `TestAccionesMandanElComando` | Las 17 acciones simples arman **exactamente** el comando esperado (`start web`, `rm -f web`, `volume rm vol`, `network rm red`, `history --no-trunc img`, …), con `Command` = `"docker " + comando`, `OK: true` y `Output` = la salida de docker. |
| `TestAccionesPropaganElError` | Si docker falla, la acción vuelve con `OK: false` y **el error en `Output`**. |
| `TestRemoveNetworkRechazaLasNativas` | `bridge`, `host` y `none` se rechazan **sin llamar a docker** (el runner no registra ninguna llamada) y con `OK: false`. |
| `TestRunImageArmaElComando` | `docker run -d --name web --network shop -p 8080:80 -e CLAVE=valor nginx`: el orden de los flags es el esperado. |
| `TestRunImageSinImagenFalla` | Sin imagen, falla y **no llama a docker**. |
| `TestRunImageRechazaPuertoInvalido` | Un puerto no numérico falla con el mensaje "puerto inválido" y **no llama a docker**. |
| `TestRunImageRechazaVariableSinIgual` | Una variable sin `=` falla con "variable inválida". |
| `TestLogsUsaElTailPorDefecto` | `tail <= 0` ⇒ `docker logs --tail 200 web`. |
| `TestLogsRespetaElTailPedido` | `tail` explícito se respeta tal cual. |
| `TestBuildValidaLaEtiqueta` | Etiqueta vacía o que empieza con `-` ⇒ falla **sin llamar a docker**. |
| `TestBuildValidaElContextoYElDockerfile` | Contexto y Dockerfile que empiezan con `-` ⇒ fallan sin llamar a docker. |
| `TestBuildArmaElComando` | `docker build -t app -f Dockerfile .`. |
| `TestBuildSinContextoUsaElActual` | Con contexto vacío ⇒ `docker build -t app .` (el "." implícito). |
| `TestAccionesConArchivoArmanElComando` | Las tres acciones con archivo: `save -o /tmp/x.tar img`, `load -i /tmp/x.tar`, `export -o /tmp/x.tar web`. |
| `TestAllowedActions` | Las acciones de la lista están permitidas y una inventada no. **Fija la lista como contrato**, aunque nadie la use (ver `bugs.md` D17). |

## `containers_test.go` — `docker ps`, `images`, `volume ls`, `network ls`, `Collect`

| Test | Contrato que fija |
|---|---|
| `TestContainersParseaLaFila` | Una fila real de `docker ps --format '{{json .}}'` ⇒ ID corto de 12, nombre, imagen, estado, **salud desde el `Status`** ("Up 3 hours (healthy)" ⇒ `healthy`), uptime ("3 hours"), project y service de las labels, y el puerto parseado. |
| `TestContainersContenedorParado` | Contenedor parado: sin salud, sin uptime, `HasShell: false`, y **`Ports` queda `nil`** (fixture: `"Ports":""`). |
| `TestContainersOrdenaPorProyectoYNombre` | Orden por proyecto y después por nombre; sin proyecto van primero ("" < "zoo"). |
| `TestContainersNombreConComaSeCorta` | `"web,web2"` ⇒ el nombre es `web`: se corta en la primera coma (los alias de red). |
| `TestContainersDeduceEstadoSiVieneVacio` | `State` vacío + `Status` "Up 5 seconds" ⇒ `running`; `Status` "Created" ⇒ `exited`. |
| `TestContainersSaltaLineasBasura` | Línea vacía, línea de texto ("Cannot connect to the Docker daemon") y JSON truncado se saltan; sólo queda la fila válida. |
| `TestContainersPropagaElError` | El error del daemon vuelve. |
| `TestImagesMarcaLasDangling` | `Repository: "<none>"` ⇒ `Dangling: true`; `nginx:latest` ⇒ `false`. |
| `TestVolumesYRedes` | `volume ls` ⇒ nombre/driver/mountpoint. `network ls` ⇒ `bridge` es `Builtin`, `shop_default` no. |
| `TestCollectCuentaYAgrupa` | El snapshot cuenta running/stopped/unhealthy, trae las cuatro listas, agrupa el proyecto `shop` con el estado `partial` (2 de 3 corriendo), `Unhealthy: 1` y los **servicios ordenados** ("cache,db,web"). |
| `TestCollectPropagaElError` | El error del daemon queda en `snap.Error`. |
| `TestHealthFromStatus` | Tabla de 7 casos: `(healthy)`, `(unhealthy)`, `(health: starting)` ⇒ `""`, `(starting)` ⇒ `"starting"`, "Exited (0)…" ⇒ `""`, vacío ⇒ `""`, y el explícito gana sobre el `Status`. |
| `TestUptimeFromStatus` | "Up 3 hours (healthy)" ⇒ "3 hours"; "Up Less than a second"; "Exited (0)…" ⇒ `""`. |
| `TestLabelValue` | Extrae la label por clave; una clave ausente ⇒ `""`; el valor puede tener `=` adentro (se parte en el primero). |
| `TestShortID` | ID de 15+ chars ⇒ 12; ID corto ⇒ tal cual; vacío ⇒ vacío. |
| `TestPrimeraLinea` | Recorta a la primera línea; vacío ⇒ `""`; 500 caracteres ⇒ 200. |

## `ports_test.go` — el mapeo de puertos

| Test | Contrato que fija |
|---|---|
| `TestParsePortsDedupe` | El mismo puerto en IPv4 e IPv6 es **un** puerto (no dos chips), con la URL `http://localhost:43211`. |
| `TestParsePortsOnlyInternal` | `"80/tcp, 443/tcp"` ⇒ 2 puertos, `HostPort` 0 y **sin URL**. |
| `TestParsePortsBindingIP` | `"127.0.0.1:8080->80/tcp"` ⇒ `HostIP` con la IP y `Container` 80. |
| `TestPortValido` | Válidos: `8080:80`, `80`, `127.0.0.1:8080:80/tcp`. Inválidos: `""`, `abc`, `8080:`, `8080:abc`. |
| `TestParsePortsIPv6` | `"[::]:8080->80/tcp"` ⇒ `HostIP` = `::` (conserva los dos puntos, pierde los corchetes), puerto 8080, contenedor 80, proto `tcp`. |
| `TestParsePortsIPv6Espefico` | `"[fe80::1]:8080->80/tcp"` ⇒ `HostIP` = `fe80::1` (una barra en la IP no rompe el parseo). |
| `TestParsePortsIPv4YIPv6NoSeDuplican` | `"0.0.0.0:8080->80/tcp, [::]:8080->80/tcp"` ⇒ **un** puerto, y **gana el primero que apareció** (`0.0.0.0`). |
| `TestParsePortsUDP` | El proto sale del sufijo: `udp`. |
| `TestParsePortsOrdenaPorPuertoDeHost` | El resultado viene ordenado por puerto de host (80, 443, 9000). |
| `TestParsePortsSalidaVacia` | Sin puertos ⇒ **`nil`** (fixture que fija el contrato del `null` del JSON, ver `bugs.md` D14). |
| `TestParsePortsDescartaLoRoto` | Un puerto sin número de contenedor se **descarta** sin romper la fila. |
| `TestParsePortsSoloInternoNoTieneURL` | `"80/tcp"` ⇒ `URL` vacío y `HostPort` 0: no se abre desde el navegador. |

## `inspect_test.go` — `docker inspect`

| Test | Contrato que fija |
|---|---|
| `TestInspectAplanaElContenedor` | El JSON se aplana: ID corto, nombre sin la barra inicial, imagen, estado, `StartedAt` presente, `RestartCount`, `RestartPolicy`, `Platform`, **`Command` = `Path` + `Args`** (no `Config.Cmd`), env, labels. |
| `TestInspectMontaReadOnly` | `RW:true` ⇒ `ReadOnly: false`; `RW:false` ⇒ `ReadOnly: true`. |
| `TestInspectListaLasRedes` | Las redes del `NetworkSettings.Networks` se listan (el test las ordena para comparar: **el orden no es estable**, ver `bugs.md`, sección "No reproducidos"). |
| `TestInspectCortaElHealthLog` | Con 7 entradas, el log se corta en **5** y la primera conservada es `"linea 1"`. Fija que se toman las **primeras** cinco (ver `bugs.md` D13). |
| `TestInspectContenedorParado` | Contenedor parado: `State` = `exited`, `FinishedAt` presente, `StartedAt` vacío. |
| `TestInspectSalidaVaciaEsError` | La salida vacía es **error**, no un contenedor vacío. |
| `TestInspectJSONMalformadoEsError` | Un JSON cortado es error. |
| `TestInspectPropagaElError` | El error de docker vuelve. |

## `stats_test.go` — `docker stats`

| Test | Contrato que fija |
|---|---|
| `TestStatsParseaLaFila` | Una fila real ⇒ ID, nombre, `CPUPct` 12.34, `MemPct` 5.67, `MemUsage`, `PIDs` 7. |
| `TestStatsVariosContenedores` | Varias filas ⇒ 2 stats con sus porcentajes y PIDs. |
| `TestStatsPorcentajeIrlegibleEsCero` | `"--"` y `""` en los porcentajes y en `PIDs` ⇒ **0**, la fila no se rompe. |
| `TestStatsSaltaLineasBasura` | Línea vacía, texto y JSON truncado se saltan. |
| `TestStatsSinContenedores` | Sin contenedores ⇒ 0 stats (el slice queda `nil`, ver `bugs.md` D14). |
| `TestStatsPropagaElError` | El error del daemon vuelve. |
| `TestPercent` | Tabla: `"12.34%"`⇒12.34, `"0.00%"`⇒0, `"100%"`⇒100, `"  7.5%"`⇒7.5, `"--"`⇒0, `""`⇒0, `"n/a"`⇒0. |

## `system_test.go` — `docker system df` y prune

| Test | Contrato que fija |
|---|---|
| `TestDiskUsageParseaLasFilas` | `TotalCount` y `Active` llegan como **string** (con `int` la fila se descartaría), `Reclaimable` sin el porcentaje, y el total es la última línea del `docker system df` en texto. |
| `TestDiskUsageSaltaLasFilasRotas` | Una fila que no es JSON se salta; la que sí lo es, se parsea. |
| `TestDiskUsagePropagaElError` | El error del daemon vuelve. |
| `TestLimpiarReclaimable` | `"1.2GB (27%)"` ⇒ `"1.2GB"`; `"0B (0%)"` ⇒ `"0B"`; sin porcentaje ⇒ tal cual. |
| `TestUltimaLinea` | Última línea no vacía; con espacios alrededor ⇒ recortada; vacío ⇒ `""`. |
| `TestPruneTraduceElDestino` | Cada destino ⇒ su comando: `container prune -f`, `image prune -f`, `volume prune -f`, `builder prune -f`, `system prune -f`. |
| `TestPruneRechazaDestinosDesconocidos` | Un destino desconocido falla con el mensaje que lista los válidos y **no llama a docker**. |
| `TestPruneDetailedArmaCadaDestino` | El prune granular arma el comando de cada destino, incluido `system` con `todas` ⇒ `prune -a --volumes --force`. |
| `TestPruneDetailedRechazaDestinosDesconocidos` | Idem para `PruneDetailed`: falla sin llamar a docker. |

## `compose_test.go` — los stacks y los eventos

| Test | Contrato que fija |
|---|---|
| `TestGroupProjectsAgrupaYOrdena` | Agrupa por project, ordena los proyectos por nombre, cuenta running/stopped/unhealthy, estado `partial`, y los servicios del proyecto **ordenados**. Un contenedor sin project no arma proyecto. |
| `TestGroupProjectsEstados` | Los tres estados: todos corriendo ⇒ `running`; mixto ⇒ `partial`; todos parados ⇒ `stopped`. |
| `TestGroupProjectsSinContenedores` | Sin contenedores ⇒ 0 proyectos. |
| `TestComposeActionAgregaDetachedAlUp` | `up` ⇒ `docker compose -p shop up -d`. |
| `TestComposeActionSinDetachedEnOtherAcciones` | `down`, `restart`, `stop`, `pull` **no** llevan `-d`. |
| `TestComposeActionCortaLaSalidaLarga` | La salida se corta a **400 caracteres**. |
| `TestComposeActionPropagaElError` | El error de docker vuelve con `OK: false`. |
| `TestEventsParseaElJSON` | Parsea el JSON de `docker events`: acción, atributos (`name`, `image`, `exitCode`), ID corto, y el **orden invertido** (lo más nuevo primero). |
| `TestEventsUsaLaVentanaPorDefecto` | La ventana por defecto es `6h` y el comando lleva `--until 0s`. |
| `TestEventsRespetaLaVentanaPedida` | La ventana pedida se usa tal cual. |
| `TestEventsSaltaLineasBasura` | Línea vacía, texto, JSON truncado y un evento sin `Action` se saltan. |
| `TestEventsPropagaElError` | El error del daemon vuelve (cuando la salida está vacía). |

## `desktop_test.go` — exec, cp, commit, tag, `PrettyBytes`, `GetInfo`

| Test | Contrato que fija |
|---|---|
| `TestExecInContainerCorreElComando` | `docker exec web ls -la /etc` ⇒ `Running: true`, `Exit: 0`, salida recortada. |
| `TestExecInContainerSinComandoFalla` | Sin comando falla **sin llamar a docker**. |
| `TestExecInContainerPropagaElError` | El error vuelve con `Running: false`. |
| `TestCopyYCopyFrom` | `docker cp /tmp/origen web:/destino` (host ⇒ contenedor) y `docker cp web:/origen /tmp/destino` (contenedor ⇒ host); `Path` es el destino. |
| `TestCommitArmaElComando` | `docker commit -m mensaje web repo:1.0`. |
| `TestCommitSinTagUsaLatest` | Sin tag ⇒ `repo:latest`. |
| `TestCommitSinRepoNoAgregaImagen` | Sin repo ⇒ `docker commit web` a secas. |
| `TestTagUsaLatestPorDefecto` | Sin tag ⇒ `docker tag img repo:latest`. |
| `TestRunCommandPasaLosArgumentos` | `RunCommand` arma `docker port web` y devuelve la salida. |
| `TestPrettyBytes` | `0`⇒"0 B", `512`⇒"512 B", `1024`⇒"1.0 KB", `1536`⇒"1.5 KB", `1 MiB`⇒"1.0 MB", `1 GiB`⇒"1.0 GB". |
| `TestGetInfoParseaElJSON` | `docker info --format json` ⇒ versión, driver, rootDir, NCPU, MemTotal, los cuatro conteos, imágenes, OS y arch. |
| `TestGetInfoConJSONMaloFalla` | Un JSON roto es error. |
| `TestGetInfoPropagaElError` | El error del daemon vuelve. |
| `TestExecUsaLaPrimeraLineaDelError` | `primeraLinea` recorta el error a **una sola línea** (el stack entero no sirve al panel). |

## `fixes_test.go` — los arreglos de los hallazgos

Un test por hallazgo, cada uno **falla si se revierte el arreglo** (verificado
revirtiendo el cambio en el código y corriendo el test).

| Test | Hallazgo | Contrato que fija |
|---|---|---|
| `TestInspectLeeElHealthDeState` | D1 | El health sale de `State.Health` (no de `Config`): `Health` y `HealthLog` se llenan. |
| `TestInspectIgnoraElHealthDeConfig` | D1 | Un health bajo `Config` (donde docker no lo manda) **no** se lee. |
| `TestHealthLogTomaLasUltimasCinco` | D13 | De 8 entradas se conservan las **últimas** cinco. |
| `TestListasDelSnapshotNoSonNull` | D14 | Ninguna lista del snapshot sale `null` en el JSON. |
| `TestStatsYDFNoSonNull` | D14 | `stats` y `df` salen `[]`, no `null`. |
| `TestInspectNoTieneListasNull` | D14 | `env`, `mounts`, `healthLog` y `labels` no salen `null`. |
| `TestExecSinBinarioDevuelveElErrorReal` | D6 | Sin docker en el `PATH`, el error no es la cadena vacía y nombra al binario. |
| `TestExecTomaElErrorDeStderr` | D7 | El mensaje del error sale de **stderr**, no de una línea de progreso de stdout. |
| `TestGetInfoAguantaUnWarningPorStderr` | D7 | Un warning por stderr no rompe el parseo del JSON de `info`. |
| `TestRunImageRespetaDetach` | D8 | `Detach: false` ⇒ el comando **no** lleva `-d`. |
| `TestRunImageConDetachPoneElDashD` | D8 | `Detach: true` ⇒ `-d`. |
| `TestEventsPropagaElErrorConSalidaParcial` | D9 | Si docker falla con salida parcial, el error vuelve y no hay eventos. |
| `TestDiskUsagePropagaElErrorDelTotal` | D10 | El error del **segundo** `system df` (el del total) se propaga. |
| `TestExecInContainerRechazaIdConGuion` | D12 | Un id con `-` se rechaza **sin llamar a docker**. |
| `TestLasAccionesRechazanIdsConGuion` | D12 | Las 21 acciones del paquete rechazan un id con `-`, sin llamar a docker, y con el motivo en `Output`. |
| `TestCopyRechazaIdConGuion` | D12 | `Copy` y `CopyFrom` también rechazan el id con guion. |
| `TestHealthStartingEsReconocido` | D15 | `(health: starting)` ⇒ `"starting"` (la forma real de docker). |
| `TestGroupProjectsNoRepiteServicios` | D16 | Un servicio escalado (`--scale web=2`) aparece **una vez** en `Services`. |
| `TestAllowedActionsIncluyeLasAccionesDelPaquete` | D17 | La lista incluye las 19 acciones del paquete y no acciones inventadas. |
| `TestExecResultPropagaElCodigoDeSalida` | D21 | `ExecResult.Exit` lleva el código real (`exit status 1` ⇒ `1`). |
| `TestExecResultExitEnElErrorDelDaemon` | D21 | Si no se conoce el código, `Exit` **no** dice 0. |

## `cli/root_test.go` — los helpers del CLI

| Test | Contrato que fija |
|---|---|
| `TestExtraerValorSacaElFlagYSuValor` | `extraerValor` saca el flag **y su valor** del resto. |
| `TestExtraerValorAceptaElFormatoConIgual` | El formato `--tail=120` también funciona. |
| `TestExtraerValorSinElFlag` | Sin el flag ⇒ `ok: false`, valor vacío, resto intacto. |
| `TestSacarFlagQuitaTodasLasOcurrencias` | `sacarFlag` quita **todas** las ocurrencias del flag. |
| `TestSacarFlagSinElFlag` | Sin el flag ⇒ `ok: false` y el resto intacto. |
| `TestPartirRutaContenedor` | `"61555f00de09:/etc/hostname"` ⇒ (id, ruta); `"id:/ruta"` ⇒ ("id", ruta); `"/ruta/normal"` ⇒ ("", ruta); `"archivo.txt"` ⇒ ("", archivo). |
| `TestSplitPath` | `"/tmp/backup.tar.gz"` ⇒ ("/tmp", "backup.tar.gz"); `"/a/b/c/d.tar"` ⇒ ("/a/b/c", "d.tar"); `"backup.tar.gz"` ⇒ ("/", "backup.tar.gz"); `"/backup.tar.gz"` ⇒ ("/", "/backup.tar.gz") (la barra pegada, documentada como inocua en el README). |
| `TestRecorta` | Recorta a N caracteres y agrega `…`; cuenta **runes**, no bytes. |
| `TestListaFlagAcumula` | El flag repetible acumula: dos `--port` dan dos valores y `String()` los separa por coma. |
| `TestRutaLogSaneaElNombre` | `"Web/Server 01"` ⇒ `web_server_01.log`: sin `/` ni espacios en el nombre del archivo. |
| `TestRutaLogAcortaElNombre` | Un id de 100 caracteres ⇒ nombre de archivo de ≤ 45. |
| `TestLeerPid` | Lee un pid numérico; archivo inexistente ⇒ 0; contenido no numérico ⇒ 0. |
| `TestProcesoVivo` | El pid 0 y el -1 no son procesos; el proceso actual sí. |

## `cli/commands_test.go` — el despacho completo

| Test | Contrato que fija |
|---|---|
| `TestComandoDesconocidoDevuelveDos` | Un subcomando inexistente ⇒ **código 2**. |
| `TestAyudaDevuelveCero` | `help`, `-h`, `--help` ⇒ código 0 e imprime la ayuda. |
| `TestSinArgumentosImprimeLaAyuda` | Sin argumentos ⇒ código 2 y la ayuda por stdout. |
| `TestSnapshotDevuelveJSON` | `snapshot --json` ⇒ JSON parseable **con `containers` presente y no `null`** (el plugin Luau hace `#payload.containers`). |
| `TestInspectDevuelveJSON` | `inspect web --json` ⇒ JSON con el nombre del contenedor. |
| `TestStatsDevuelveJSON` | `stats --json` ⇒ `{"stats":[…], "at":…}` con el nombre del contenedor y `at` > 0. |
| `TestPruneConDestinoInvalidoDevuelveDos` | Destino inválido ⇒ **código 2**; en modo JSON el motivo viaja por **stdout**, en modo texto por **stderr**. |
| `TestControlSinIdDevuelveDos` | `start`/`stop`/`restart`/`remove` sin id ⇒ código 2. |
| `TestLogsStopSinSeguimiento` | Sin pidfile ⇒ "sin seguimiento" y código 0. |
| `TestLogsStopMataElProceso` | Con un pidfile que apunta a un proceso vivo, `logs-stop` lo mata y **borra el pidfile**. |
| `TestComandosSimplesConRunnerFalso` | Los 34 comandos "simples" (`port`, `search`, `update`, `volume-inspect`, `network-inspect`, `top`, `history`, `pull`, `push`, `save`, `load`, `export`, `diff`, `build`, `tag`, `rename`, `commit`, `pause`, `unpause`, `volume-create`, `network-create`, `rmi`, `volume-rm`, `start`, `stop`, `restart`, `remove`, `run`, `logs`, `network-rm`) ⇒ **código 0**. Es la red de seguridad para que un comando nuevo no se cuelgue sin que nadie lo note. |
| `TestComandosDeLecturaConRunnerFalso` | `df --json`, `info --json`, `events --json`, `compose ls` y `snapshot --json` ⇒ **JSON parseable** (lo que necesita el plugin). |
| `TestVolumeExportEImportArmanElComando` | `volume-export` y `volume-import` con un path absoluto ⇒ código 0. |
| `TestVolumeExportRechazaNombresConGuion` | Un nombre o path que empieza con `-` ⇒ **código 1**. |
| `TestComposeFileLeeElYAML` | `compose-file shop` lee el archivo de `ConfigFiles` e imprime su contenido. |
| `TestComposeFileConProyectoInexistente` | Un proyecto que no está en la lista ⇒ **código 1** y el mensaje por stderr. |
| `TestLogsFollowUsaElTailPorDefecto` | `logs-follow web` ⇒ `docker logs --follow --tail 200 web` (verificado con un `docker` falso en el `PATH` que anota sus argumentos). |
| `TestLogsFollowRespetaElTail` | `--tail 50` ⇒ `docker logs --follow --tail 50 web`. |
| `TestLogsStreamMataElProcesoSiNoPuedeGuardarElPid` | Si el pidfile no se puede escribir (un **directorio** en su lugar ⇒ `EISDIR`), el comando **mata el `docker logs -f` que acaba de arrancar**, sale con **código 1** y nombra el pidfile en stderr. Verifica en `/proc` que no queda vivo ni el script ni su grupo. Es el bug ya arreglado (commit `0145744`). |
| `TestExecYCPConRunnerFalso` | `exec --id web ls` y las dos direcciones de `cp` ⇒ código 0. |
| `TestExecSinIdDevuelveUno` | `exec` sin `--id` ⇒ **código 1**. |
| `TestBuildSinTagDevuelveUno` | `build` sin `--tag` ⇒ **códig 1**. |
| `TestCopySinDestinoDevuelveUno` | `cp` sin `--to` ⇒ **código 1**. |

## Los huecos que había (y cómo se cerraron)

Cada fila es un hueco de test que **explicaba** un hallazgo de `bugs.md`. Los
21 están cerrados: el hueco se tapó con un test nuevo o el test viejo se
ajustó al contrato correcto.

| Hueco (antes) | Hallazgo | Cómo se cerró |
|---|---|---|
| El fixture de `inspect_test.go` ponía el health bajo `Config`, que **no es** donde lo manda docker (`State.Health`). El test pasaba porque el test y el código compartían el mismo error. | **D1** | Fixture movido a `State.Health` + `TestInspectLeeElHealthDeState` |
| Ningún test pasaba un id con metacaracteres de shell a `logs-stream`. | **D2** | `TestLogsStreamNoEjecutaComandosDelId` |
| `TestLogsStopMataElProceso` **requería** que el pid apunte a un proceso: nunca probaba el caso del pid reciclado ni el de un proceso ajeno. | **D3** | `TestLogsStopNoMataUnProcesoAjeno` (el pidfile ahora lleva pid + starttime) |
| `TestVolumeExportEImportArmanElComando` sólo probaba paths **absolutos**; el caso relativo (que monta `/`) no estaba. | **D4** | `TestVolumeExportConRutaRelativaNoMontaLaRaiz` |
| `compose --action` no se testeaba en ningún orden: el test de `compose` es el de `compose ls`. | **D5** | `TestComposeAceptaElActionDespuesDelProyecto` |
| No había test de `Exec` con el binario ausente o el contexto cancelado. | **D6** | `TestExecSinBinarioDevuelveElErrorReal` |
| No había test de `Exec` con docker que escribe en stderr. | **D7** | `TestExecTomaElErrorDeStderr` + `TestGetInfoAguantaUnWarningPorStderr` |
| `RunImage` siempre se llamaba sin `Detach`, así que el `|| true` no se notaba. | **D8** | `TestRunImageRespetaDetach` |
| `TestEventsPropagaElError` usaba un runner que falla **con salida vacía**: justo el caso que `Events` sí manejaba. El caso "falla con salida parcial" no estaba. | **D9** | `TestEventsPropagaElErrorConSalidaParcial` |
| `TestDiskUsagePropagaElError` hacía fallar el **primer** `system df`; el segundo (el del total) nunca fallaba en un test. | **D10** | `TestDiskUsagePropagaElErrorDelTotal` |
| `TestRutaLogSaneaElNombre` probaba un id con `/` y espacios, no dos ids que colisionen. | **D11** | `TestRutaLogNoColisiona` |
| Sólo `search`, `update` y `volume-export/import` tenían test del prefijo `-`. Los demás no. | **D12** | `TestIdsConGuionSeRechazan` (CLI) + `TestLasAccionesRechazanIdsConGuion` (paquete) |
| `TestInspectCortaElHealthLog` afirmaba que la primera conservada es `"linea 1"`: **codificaba** el bug. | **D13** | Test ajustado (ahora exige las últimas cinco) + `TestHealthLogTomaLasUltimasCinco` |
| `TestParsePortsSalidaVacia` **exigía** que `Ports` sea `nil`: **codificaba** el `null` del JSON. | **D14** | Test ajustado (ahora exige lista vacía) + `TestListasDelSnapshotNoSonNull` |
| `TestHealthFromStatus` afirmaba que `(health: starting)` ⇒ `""`: **codificaba** el bug. | **D15** | Test ajustado + `TestHealthStartingEsReconocido` |
| `TestGroupProjectsAgrupaYOrdena` usaba un servicio distinto por contenedor: nunca probaba réplicas. | **D16** | `TestGroupProjectsNoRepiteServicios` |
| `TestAllowedActions` verificaba la lista contra sí misma; ningún test verificaba que el CLI la use. | **D17** | `TestComandoFueraDeLaListaNoSeEjecuta` + `TestTodasLasAccionesMutantesEstanPermitidas` |
| `TestSacarFlagQuitaTodasLasOcurrencias` no comprobaba que el slice de entrada quede intacto. | **D18** | `TestSacarFlagNoMutaElSliceDelCaller` |
| `TestLogsStopSinSeguimiento` cubría el pidfile ausente, no el pidfile con un pid muerto. | **D19** | `TestLogsStopConProcesoMuertoNoDiceDetenido` |
| Ningún test usaba un `docker` falso que **fallara** en `logs-stream`. | **D20** | `TestLogsStreamConContenedorInexistenteFalla` |
| `TestExecInContainerCorreElComando` usaba el camino feliz: el `Exit` en 0 del camino de error no se comprobaba. | **D21** | `TestExecResultPropagaElCodigoDeSalida` + `TestExecResultExitEnElErrorDelDaemon` |

## Lo que sí está bien cubierto

- **El runner falso indexa por argumentos**: un comando no registrado es un
  error, así que un test falla si el código llama algo que no esperaba, en vez
  de devolver vacío en silencio.
- **La concurrencia de `Collect`**: el `fakeRunner` es seguro con `sync.Mutex`
  porque `Collect` lanza tres goroutines contra el mismo runner. La suite pasa
  con `go test -race`.
- **La red de seguridad del despacho**: `TestComandosSimplesConRunnerFalso`
  recorre 34 comandos y exige código 0 en cada uno.
- **El contrato JSON del plugin**: cinco comandos de lectura tienen test de
  "la salida es JSON parseable", y `TestSnapshotDevuelveJSON` exige que
  `containers` no sea `null`.
- **Los dos comandos de proceso real**: `logs-follow` y `logs-stream` usan un
  script `docker` falso en el `PATH`, y el de `logs-stream` verifica en
  `/proc` que no quedan procesos vivos ni zombis (el bug ya corregido).
- **Las validaciones previas a docker**: puerto, variable, etiqueta de build,
  contexto, Dockerfile, red nativa y destino de prune — todas verificadas con
  el runner falso de que **no se llama a docker**.

## Verificación

```console
$ cd /home/b3low-z3r0/Documents/Github/Cli/branches/fix-docker
$ go build ./...
(sin salida)

$ go vet ./internal/plugins/docker/...
(sin salida)

$ go test ./internal/plugins/docker/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker	0.008s
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker/cli	3.914s

$ go test ./internal/plugins/docker/... -race -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker	0.010s
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker/cli	4.106s
```

Cada arreglo se verificó **revirtiéndolo**: se deshizo el cambio en el
código, se corrió el test del hallazgo y se comprobó que falla.
