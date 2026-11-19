# Tests de `internal/plugins/anilist/cli`

Qué prueba cada test del subpaquete. El dominio (`internal/plugins/anilist`)
tiene su propia suite; esta cubre **el parseo de flags, la salida y los
códigos de salida**, que es justo lo que el dominio no toca.

## Cómo se aísla

| Auxiliar | Qué hace |
|---|---|
| `capturar` | Redirige `os.Stdout` y `os.Stderr` a tuberías y devuelve `(código, stdout, stderr)`. La lectura va en goroutines para que una salida larga no tape al comando. |
| `afirmarUnSoloJSON` | Decodifica stdout y exige que sea **UN** documento JSON y nada más. Decodifica con `json.Decoder` (no con `strings.Contains`) porque un `println` de progreso antes del JSON rompe al consumidor del plugin y un substring no lo ve. |
| `instalarAniListFalso` | Reemplaza `http.DefaultTransport` por un `RoundTripper` que responde las consultas de AniList y las descargas de carátulas. El cliente del dominio no deja inyectar el transporte, así que se cambia el global y se restaura al terminar el test. **La API real nunca se toca.** |
| `responderPorDefecto` | Reparte las respuestas por el contenido de la query: viewer, biblioteca, descubrimiento, ficha, mutación, estadísticas, agenda. |
| `variablesDe` | Devuelve las variables de la última consulta que coincida con una subcadena: sirve para verificar el **armado del request**. |
| `escribir` | Escribe un archivo con el contenido dado. |
| `escribirToken` | Deja un `token.json` válido en el data dir. |
| `sinTokenDeEntorno` | Vacía `ANILIST_TOKEN`: el token tiene que venir del flag o del disco, nunca del entorno del que corra el test. |
| `ejecutarEnSubproceso` | Corre `Run` en un proceso aparte. Necesario porque los flagsets usan `flag.ExitOnError`: un flag desconocido mata el proceso con código 2 y no se puede comprobar en el propio proceso de test. |

Ningún test toca la red, el home del usuario ni el disco real: todo va a
`t.TempDir()`, con `$HOME` y `ANILIST_TOKEN` temporales.

## `root_test.go` — despacho, ayuda y códigos de salida

| Test | Qué fija |
|---|---|
| `TestSinArgumentosImprimeElUsoYDevuelveDos` | Sin argumentos imprime el uso y sale con 2. |
| `TestComandoDesconocidoVaAStderrYDevuelveDos` | Un comando inventado avisa por stderr, imprime el uso por stdout y sale con 2. |
| `TestHelpImprimeElUsoYDevuelveCero` | `help`, `-h` y `--help` salen con 0. |
| `TestFlagInvalidoSaleConCodigoDos` | Cada subcomando con un flag desconocido sale con 2 y el error del flag en stderr. |
| `TestAyudaDeUnSubcomandoSaleConCodigoCero` | `sync --help` sale con 0 y muestra el uso del flagset. |
| `TestCadaSubcomandoDelUsoTieneUnTest` | Todo subcomando anunciado en el texto de ayuda tiene un test asignado: falla si se agrega uno sin testear. |

## `root_test.go` — sync

| Test | Qué fija |
|---|---|
| `TestSyncDevuelveBibliotecaYCubiertasEnUnSoloJSON` | `--json` devuelve un solo documento con viewer, anime, manga, covers y coversDir; las carátulas bajan al disco y la biblioteca se pide con el id del Viewer. |
| `TestSyncEnTextoMuestraUsuarioYCubiertas` | Sin `--json` muestra el usuario y el resumen de carátulas. |
| `TestSyncRespetaElDirectorioDeCubiertas` | `--covers-dir` manda: las carátulas se escriben ahí y `coversDir` lo refleja. |
| `TestSyncAdoptaElCacheViejoConReuse` | `--reuse` adopta el caché viejo con hardlink: la carátula válida no se vuelve a bajar y el resumen cuenta `reused` y `downloaded`. |
| `TestSyncConGcBorraLasHuerfanas` | `--gc` borra la carátula que no está en la biblioteca y el resumen cuenta `removed`. |
| `TestSyncSinTokenFallaConCodigoUno` | Sin token sale con 1 y el motivo en stderr. |
| `TestSyncEnJSONElErrorEsUnDocumentoJSON` | Con `--json` el error es un documento JSON, no texto suelto. |

## `root_test.go` — library

| Test | Qué fija |
|---|---|
| `TestLibraryDevuelveLaBibliotecaEnUnSoloJSON` | Devuelve anime, manga y `partial: false`. |
| `TestLibraryConUnaListaRotaMarcaParcial` | Si una lista falla, la otra vale igual y `partial` queda en true. |
| `TestLibraryEnTextoAvisaSiUnaListaFallo` | El aviso de biblioteca parcial sale por stdout en modo texto. |
| `TestLibraryEnTextoMuestraLosTotales` | Muestra usuario, anime y manga. |

## `root_test.go` — covers

| Test | Qué fija |
|---|---|
| `TestCoversDesdeElArchivoDeBibliotecaNoPideToken` | Con `--library` no hace falta token: se leen las entradas del archivo y se bajan las carátulas. |
| `TestCoversTomaElArchivoDeBibliotecaDeLua` | Un `{}` de Lua es una lista vacía, no un error. |
| `TestCoversConArchivoInexistenteFalla` | Un `--library` inexistente sale con 1. |
| `TestCoversConArchivoDeBibliotecaInvalidoFalla` | Un `--library` con JSON roto sale con 1. |
| `TestCoversEnTextoMuestraElResumen` | El resumen de carátulas sale por stdout en modo texto. |

## `root_test.go` — mutate

| Test | Qué fija |
|---|---|
| `TestMutateExigeMediaIDYProgreso` | Sin `--media-id` o `--progress` sale con 1 y lo dice en el JSON. |
| `TestMutateGuardaElProgreso` | La mutación manda mediaId, progress y status; el JSON trae lo que devolvió la API. |
| `TestMutateEnTextoMuestraLoGuardado` | La línea `guardado: media N → M` sale por stdout. |

## `root_test.go` — discover

| Test | Qué fija |
|---|---|
| `TestDiscoverUsaElKindPedido` | `--kind top` manda `SCORE_DESC`; `--page` y `--per-page` viajan tal cual. |
| `TestDiscoverCadaKindMandaSuSort` | `trending`, `popular` y `new` mandan su propio sort y el kind vuelve en el JSON. |
| `TestDiscoverSeasonalPideLaTemporada` | `seasonal` manda season, seasonYear del año actual y `POPULARITY_DESC`. |
| `TestDiscoverConKindInvalidoFalla` | Un kind inventado sale con 1 y avisa. |
| `TestDiscoverEnTextoMuestraElCatalogo` | El encabezado del catálogo y los items salen por stdout. |
| `TestDiscoverEnTextoMuestraComoMaximoDoceItems` | En texto se muestran doce items, no veinte. |
| `TestDiscoverConCoversDirDescargaLasCaratulas` | `--covers-dir` baja las carátulas de la página y las agrega al JSON. |

## `root_test.go` — stats

| Test | Qué fija |
|---|---|
| `TestStatsDevuelveLosTotalesEnUnSoloJSON` | Devuelve user, anime y manga en un solo documento. |
| `TestStatsConsultaElUsuarioPedido` | `--user` viaja en el `name` de la query. |
| `TestStatsConErrorDejaStdoutVacio` | El error de stats va **sólo** a stderr: stdout queda vacío porque el plugin lo lee como JSON. |
| `TestStatsSirveElCacheSinPreguntarALaAPI` | Con el caché fresco no se consulta la API (0 consultas). |
| `TestStatsConRefreshIgnoraElCache` | `--refresh` descarta el caché: consulta la API y devuelve los totales de ella, no los del archivo. |
| `TestStatsEnTextoMuestraLosTotalesYLosGeneros` | El resumen con géneros sale por stdout. |

## `root_test.go` — media

| Test | Qué fija |
|---|---|
| `TestMediaExigeElMediaID` | Sin `--media-id` sale con 1 y lo dice en el JSON. |
| `TestMediaDevuelveLaFichaEnUnSoloJSON` | Devuelve título, géneros, sinopsis sin HTML y el próximo capítulo; el id viaja en la query. |
| `TestMediaEnTextoMuestraLaFicha` | Título, géneros y próximo capítulo salen por stdout. |

## `root_test.go` — schedule

| Test | Qué fija |
|---|---|
| `TestScheduleDevuelveLaAgendaEnUnSoloJSON` | Devuelve los capítulos y hace dos consultas (la lista en curso y la agenda). |
| `TestScheduleEnTextoMuestraLosCapitulos` | El encabezado y los capítulos salen por stdout. |

## `root_test.go` — status

| Test | Qué fija |
|---|---|
| `TestStatusReportaElTokenYElCache` | `hasToken` falso sin token; cuenta sólo los `.jpg` y sus bytes. |
| `TestStatusDetectaElTokenEnElDisco` | `hasToken` verdadero con `token.json`. |
| `TestStatusAceptaElTokenExplicito` | `--token` es una opción válida y cuenta como token presente (bug corregido, ver `bugs.md`). |
| `TestStatusEnTextoMuestraElEstado` | Data dir, estado del token y caché salen por stdout. |
| `TestStatusUsaElDataDirPorDefecto` | Sin `--data-dir` usa el directorio del plugin bajo `$HOME`. |

## `root_test.go` — caminos de error compartidos

| Test | Qué fija |
|---|---|
| `TestSinTokenTodosLosComandosFallanConCodigoUno` | Sin token, todos los comandos que consultan la API salen con 1 y avisan. |
| `TestComandoQueConsultaLaApiFallaConCodigoUno` | Un error de la API sale con 1 y, con `--json`, como documento JSON en stdout. |
| `TestEmitFallaSiElJSONNoSePuedeCodificar` | Si el valor no se puede serializar, sale con 1, stdout vacío y el motivo en stderr. |
| `TestFallaSiNoPuedeCrearElDirectorioDeCubiertas` | `sync`, `covers` y `discover` salen con 1 si el directorio de carátulas no se puede crear. |

## Qué no está cubierto

- `--workers`: sólo cambia la concurrencia de las descargas; el resultado es el
  mismo con 1 o con 6, así que no hay comportamiento observable que fijar sin
  medir tiempos (y medir tiempos en un test es frágil).
- La descarga real contra el CDN de AniList: el `RoundTripper` falso devuelve
  bytes fijos, así que se verifica el armado del request y el estado en disco,
  no el contenido real de la imagen.
- El caché de `stats` vencido (más de 30 minutos): para que expire hay que
  escribir un `fetchedAt` viejo en el archivo; se cubre el camino fresco y el
  de `--refresh`, no el TTL vencido.
