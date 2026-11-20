# Tests de `internal/plugins/anilist`

Dos capas: el paquete `anilist` (API, caché, carátulas) y `anilist/cli`
(parseo y salida de `b3l0w anilist`). Ningún test toca la API real: el
paquete usa `httptest.Server` con el transporte redirigido y la capa CLI usa
un `RoundTripper` falso sobre `http.DefaultTransport`.

```console
$ go test ./internal/plugins/anilist/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist/cli
```

## Archivos de test

| Archivo | Qué cubre |
|---|---|
| `library_test.go` | paginación de `MediaListCollection`, merge por `mediaId`, orden por título, `Partial` cuando una lista falla. También los helpers `serverClient` y `requestBody` que usa el resto de los tests del paquete |
| `covers_test.go` | ciclo completo del caché de carátulas: descarga, reuso por meta, GC de huérfanas, conteo de fallos, escritura del `.meta.json` |
| `libraryfile_test.go` | `{}` como lista vacía (Lua), entradas reales, JSON inválido |
| `token_test.go` | precedencia flag → env → `token.json`, error sin filtrar el secreto |
| `stats_test.go` | traducción de `statistics`, fallback desde la lista, slices vacíos, TTL del caché, usuario o cuenta ajena, error sin escribir caché |
| `fixes_test.go` | hallazgos CRÍTICOS y ALTOS (C1, A1–A4) |
| `medio_test.go` | hallazgos MEDIO y BAJO (M1–M9, B1–B3, B5) |
| `cli/root_test.go` | despacho y códigos de salida, flags de cada comando, stdout con `--json` como **un solo** documento JSON, errores a stderr, `status` y el caché de carátulas. También los helpers `capturar`, `afirmarUnSoloJSON`, `instalarAniListFalso` y `ejecutarEnSubproceso` |
| `cli/gc_test.go` | el GC con biblioteca parcial (C1) |
| `cli/user_test.go` | el flag `--user` en todos los subcomandos (B4) y el `covers --library` sin token (B5) |

## Tests por contrato

### Hallazgos MEDIO y BAJO (nuevos)

| Test | Contrato que fija |
|---|---|
| `TestLaAgendaFiltraPorTipoYEtiquetaConElTipoDeLaMedia` | M1: la agenda pide `type: ANIME` y la etiqueta `mediaType` sale del `type` de la media, no de un valor fijo |
| `TestElFallbackDeStatsUsaLaEscalaDeStatistics` | M2: las notas del fallback se piden en `POINT_100`, la misma escala de `statistics.scores` |
| `TestLaBibliotecaPideDeA500Entradas` | M3: `MediaListCollection` se pagina de a 500 entradas, no de a 50 |
| `TestElChunkArrancaEnUno` | M4: el primer chunk pedido es el 1; el 0 devuelve la misma página que el 1 |
| `TestElMergeDesempataPorProgreso` | M5: con `updatedAt` empatado gana la entrada más completa, no la primera que se vio |
| `TestReusedCuentaCaratulasYNoArchivos` | M6: `reused` cuenta carátulas adoptadas, no archivos (el `.jpg` y su `.meta.json` son una sola carátula) |
| `TestUnaCaratulaMasGrandeQueElTopeSeRechaza` | M7: una imagen que excede el tope de 16 MB es un fallo y no deja nada en disco (nunca un JPEG truncado cacheado) |
| `TestElCacheDeStatsNoSeSirveSinDueñoVerificable` | M8: sin `library.json` (o con `viewer` mal formado) el `stats.json` de otra cuenta no se sirve; con dueño verificable sí |
| `TestDataDirRespetaLasVariablesDeEstado` | M9: `DataDir` respeta `NOCTALIA_STATE_HOME`, después `XDG_STATE_HOME`, y el flag explícito gana |
| `TestElMetaSeEscribeDespuesDelRenameDelJpg` | B1: si el rename del JPG falla, el `.meta.json` no queda escrito (nadie ve meta nuevo con imagen vieja) |
| `TestElMetaSeEscribeAtomico` | B1: el `.meta.json` se reemplaza por rename (cambia de inode) y no queda ningún `.part` |
| `TestStripHTMLDecodificaEntidadesYNoSeComeElTexto` | B2: `stripHTML` decodifica entidades (`&mdash;` → `—`) y conserva el texto que sigue a un `<` suelto |
| `TestUnaColeccionNulaEsUnError` | B3: `MediaListCollection: null` es un error, no una biblioteca vacía sin error |
| `TestElReadmeNoDiceQueElCliExijaTokenParaTodo` | B5: el README documenta que `covers --library` corre sin token |
| `TestElFlagUserAceptaUnNombreEnLibrary` | B4: `library --user b3low` no es un error de parseo (el flag acepta id numérico o nombre) |
| `TestElFlagUserAceptaUnIdEnStats` | B4: `stats --user 123` resuelve el id contra la API en vez de buscar el usuario literal "123" |
| `TestElFlagUserEsElMismoEnTodosLosSubcomandos` | B4: `--user` es el mismo flag (string) en todos los subcomandos |
| `TestElFlagUserCeroEsElDelToken` | B4: `--user 0` conserva el significado del flag viejo (0 = el usuario del token) |
| `TestElArchivoDeBibliotecaNoPideToken` | B5: `covers --library` corre sin token y devuelve su JSON |

### Hallazgos CRÍTICOS y ALTOS (ola anterior)

| Test | Contrato que fija |
|---|---|
| `TestSyncDevuelveErrorSiLasDosListasFallan` | C1: `Sync` devuelve error si fallan las dos listas (nunca una biblioteca vacía sin error) |
| `TestSyncConGcNoBorraSiLaBibliotecaEstaIncompleta` | C1: `sync --gc` no borra carátulas con biblioteca parcial y sale con código 1 |
| `TestGCMantieneLaCaratulaSiLaDescargaFalla` | A1: una descarga fallida no vuelve borrable la carátula que ya estaba |
| `TestReuseNoCorrompeElMetaDelOrigen` | A2: adoptar con `--reuse` no reescribe el `.meta.json` del caché de origen |
| `TestFetchSchedulePaginaLaListaYLosCapitulos` | A3/A4: las dos queries de la agenda siguen `pageInfo` hasta agotar |

### Tests previos del módulo

| Test | Contrato que fija |
|---|---|
| `TestFetchListPaginaYMergeaPorMediaID` | la biblioteca pagina, mergea por `mediaId` (gana la más reciente) y ordena por título |
| `TestSyncMarcaParcialSiUnaListaFalla` | `Sync` devuelve la lista que vino con `Partial=true` cuando la otra falla |
| `TestSyncCoversDescargaReutilizaYBorraHuerfanos` | descarga, reuso por meta, GC de huérfanas y conteo de fallos de las carátulas |
| `TestReadLibraryFileToleraListasVacias` | `{}` se lee como lista vacía y un JSON inválido es un error |
| `TestTokenPrecedenciaYDegradacion` | el token sale del flag, del entorno o de `token.json`, y el error no filtra el secreto |
| `TestFetchStatsTraduceYOrdena` | la traducción de `statistics` y el orden de géneros y notas |
| `TestStatsFallbackDesdeLaLista` | el fallback recalcula géneros y notas desde la lista del usuario |
| `TestStatsNoPideListaSiLaApiTraeAgregados` | con los agregados de la API no se pide la lista |
| `TestStatsFallbackFallidoNoPierdeLosTotales` | si el fallback falla, los totales se conservan |
| `TestFetchStatsUsuarioInexistente` | un usuario inexistente es un error, no ceros |
| `TestFetchStatsExigeNombre` | `FetchStats` sin nombre de usuario es un error |
| `TestStatsSinDatosDevuelveArraysVacios` | los arrays del contrato nunca vienen `null` (Luau los cuenta) |
| `TestLoadStatsSirveCacheFresco` | un caché fresco y del dueño correcto se sirve sin tocar la red |
| `TestLoadStatsCacheViejoConsulta` | un caché vencido se consulta de nuevo |
| `TestLoadStatsRefreshFuerzaConsulta` | `--refresh` ignora el caché |
| `TestLoadStatsCacheDeOtroUsuarioNoSeUsa` | el caché de otro usuario no se usa |
| `TestLoadStatsSinUsuarioPreguntaAlViewer` | sin `--user`, el nombre sale del `Viewer` |
| `TestLoadStatsCacheDeOtraCuentaNoSeSirve` | el caché de otra cuenta (detectado por `library.json`) no se sirve |
| `TestLoadStatsEscribeCache` | `stats.json` se escribe y el segundo llamado no toca la red |
| `TestLoadStatsErrorNoDejaCache` | un error de la API no deja caché escrito |
| `TestLoadStatsCacheCorruptoConsulta` | un `stats.json` corrupto se ignora y se consulta la API |
| `TestSinArgumentosImprimeElUsoYDevuelveDos` | sin comando se imprime el uso y se sale con 2 |
| `TestComandoDesconocidoVaAStderrYDevuelveDos` | un comando desconocido va a stderr con código 2 |
| `TestHelpImprimeElUsoYDevuelveCero` | `help` imprime el uso y sale con 0 |
| `TestSyncDevuelveBibliotecaYCubiertasEnUnSoloJSON` | `sync --json` emite un único documento JSON con biblioteca y carátulas |
| `TestSyncEnTextoMuestraUsuarioYCubiertas` | la salida humana de `sync` muestra usuario, totales y carátulas |
| `TestSyncRespetaElDirectorioDeCubiertas` | `--covers-dir` manda sobre el del data dir |
| `TestSyncSinTokenFallaConCodigoUno` | sin token, `sync` sale con 1 |
| `TestSyncEnJSONElErrorEsUnDocumentoJSON` | el error de `sync --json` es un documento JSON, no texto |
| `TestLibraryDevuelveLaBibliotecaEnUnSoloJSON` | `library --json` emite un único documento JSON |
| `TestLibraryConUnaListaRotaMarcaParcial` | `library` marca `partial` cuando una lista falla |
| `TestLibraryEnTextoAvisaSiUnaListaFallo` | la salida humana avisa que una lista no se pudo traer |
| `TestLibraryEnTextoMuestraLosTotales` | la salida humana de `library` muestra anime y manga |
| `TestCoversDesdeElArchivoDeBibliotecaNoPideToken` | `covers --library` no necesita token (la excepción que documenta B5) |
| `TestCoversTomaElArchivoDeBibliotecaDeLua` | `covers --library` acepta el `library.json` que escribe Luau |
| `TestCoversConArchivoInexistenteFalla` | un `--library` inexistente es un error |
| `TestCoversEnTextoMuestraElResumen` | la salida humana de `covers` muestra válidas, nuevas, reusadas, fallidas y borradas |
| `TestMutateExigeMediaIDYProgreso` | `mutate` sin `--media-id` o `--progress` falla |
| `TestMutateGuardaElProgreso` | `mutate` guarda el progreso y devuelve el JSON |
| `TestMutateEnTextoMuestraLoGuardado` | la salida humana de `mutate` muestra media, progreso y estado |
| `TestDiscoverUsaElKindPedido` | `discover` manda el `kind` pedido |
| `TestDiscoverSeasonalPideLaTemporada` | `seasonal` manda la temporada y el año en curso |
| `TestDiscoverConCoversDirDescargaLasCaratulas` | `discover --covers-dir` baja las carátulas de la página |
| `TestDiscoverEnTextoMuestraComoMaximoDoceItems` | la salida humana de `discover` muestra hasta 12 items |
| `TestDiscoverConKindInvalidoFalla` | un `kind` inventado es un error |
| `TestDiscoverEnTextoMuestraElCatalogo` | la salida humana de `discover` muestra el catálogo |
| `TestStatsDevuelveLosTotalesEnUnSoloJSON` | `stats --json` emite un único documento JSON |
| `TestStatsConsultaElUsuarioPedido` | `stats --user` consulta ese usuario |
| `TestStatsConErrorDejaStdoutVacio` | el error de `stats` va sólo a stderr (stdout queda vacío para el plugin) |
| `TestStatsSirveElCacheSinPreguntarALaAPI` | con el caché fresco y el dueño verificable, `stats` no pregunta a la API |
| `TestStatsEnTextoMuestraLosTotalesYLosGeneros` | la salida humana de `stats` muestra totales y géneros |
| `TestMediaExigeElMediaID` | `media` sin `--media-id` falla |
| `TestMediaDevuelveLaFichaEnUnSoloJSON` | `media --json` emite un único documento JSON |
| `TestMediaEnTextoMuestraLaFicha` | la salida humana de `media` muestra formato, géneros, próximo capítulo y sinopsis |
| `TestScheduleDevuelveLaAgendaEnUnSoloJSON` | `schedule --json` emite un único documento JSON |
| `TestScheduleEnTextoMuestraLosCapitulos` | la salida humana de `schedule` muestra los capítulos |
| `TestComandoQueConsultaLaApiFallaConCodigoUno` | un error de la API sale con 1 |
| `TestSinTokenTodosLosComandosFallanConCodigoUno` | todos los comandos fallan con 1 sin token |
| `TestEmitFallaSiElJSONNoSePuedeCodificar` | un JSON que no se puede codificar sale con 1 |
| `TestStatusUsaElDataDirPorDefecto` | `status` usa el data dir por defecto |
| `TestCoversConArchivoDeBibliotecaInvalidoFalla` | un `library.json` inválido es un error |
| `TestFallaSiNoPuedeCrearElDirectorioDeCubiertas` | un directorio de carátulas que no se puede crear es un error |
| `TestStatusReportaElTokenYElCache` | `status` informa si hay token y cuántas carátulas hay |
| `TestStatusDetectaElTokenEnElDisco` | `status` detecta el `token.json` del data dir |
| `TestStatusAceptaElTokenExplicito` | `status --token` cuenta como token presente |
| `TestStatusEnTextoMuestraElEstado` | la salida humana de `status` muestra datos, token y caché |
| `TestFlagInvalidoSaleConCodigoDos` | un flag desconocido sale con 2 (flagsets con `flag.ExitOnError`, en subproceso) |
| `TestAyudaDeUnSubcomandoSaleConCodigoCero` | `--help` de un subcomando sale con 0 |
| `TestCadaSubcomandoDelUsoTieneUnTest` | cada subcomando del texto de uso tiene su test |
| `TestSyncAdoptaElCacheViejoConReuse` | `sync --reuse` adopta el caché viejo |
| `TestSyncConGcBorraLasHuerfanas` | `sync --gc` borra las carátulas que ya no están en la biblioteca |
| `TestStatsConRefreshIgnoraElCache` | `stats --refresh` consulta la API aunque el caché sea fresco |
| `TestDiscoverCadaKindMandaSuSort` | cada `kind` de `discover` manda su propio `sort` |
| `TestHelperProceso` | helper: corre `Run` en un subproceso para los flagsets con `ExitOnError` |
