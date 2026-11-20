# Tests de `internal/plugins/ytmusic`

18 archivos de test: 16 en el paquete `ytmusic` y 2 en `ytmusic/cli`.

```bash
go test ./internal/plugins/ytmusic/...
go test ./internal/plugins/ytmusic/... -cover
```

**Ninguna prueba sale a la red.** Las URLs de innertube son constantes del
paquete, así que lo que se intercepta es el **transporte HTTP**: un
`httptest.Server` local con un `RoundTripper` que reescribe el host. Para mpv
hay un **socket unix falso** que habla el protocolo IPC. Para `yt-dlp` hay un
**binario falso en el PATH**.

| Archivo | Qué cubre |
|---|---|
| `fakeserver_test.go` | La infra: servidor innertube falso (reescribe el host), fixtures de respuesta (`songRenderer`, `panelSong`, `radioPayload`, `shelfPayload`, `cardPayload`) y helpers de payload |
| `model_test.go` | `DataDir` (flag explícito, `NOCTALIA_STATE_HOME`, state local), `EnsureDir`, la escritura atómica de `writeJSON`, `readJSON` con archivo vacío / roto / inexistente y `songDedup` |
| `parse_test.go` | El parseo de innertube sobre fixtures reales: `songsFromRenderers`, `continuationOf`, `playlistCards`, `feedFrom`, `durationToSeconds`, `splitMeta` |
| `parse_edge_test.go` | Los helpers de JSON y los feeders en los casos raros (payloads vacíos, formas inesperadas, `asMap`/`asSlice`/`asString`/`asInt`/`textOf`) |
| `home_search_test.go` | El fallback de home a la primera playlist y la búsqueda con el filtro de canciones |
| `charts_test.go` | El ranking de charts: preferencia Top 100 > Top 50 > Top > Trending, los +10 "songs" y los +5 "global", y el chart vacío como error |
| `radio_test.go` | La máquina de estados de la sesión: crear, deduplicar, `next` (pide más, siembra otra radio cuando se agota), `prev`, `shuffle`, `enqueue`, `queue` y el cursor fuera de rango |
| `durations_test.go` | `clockOf`, `durationToSeconds` con entradas raras, `videoSeconds` contra el endpoint `player` y `EnrichDurations` (sólo pide las que faltan, respeta el límite, con límite cero no hace nada, ignora los errores del player, respeta el contexto cancelado) |
| `thumbs_test.go` | La descarga de portadas: baja y deja la ruta local, respeta el límite, cachea por `videoId`, un 404 deja la canción sin portada, un cuerpo chico es error, sin URL o sin directorio no hace nada |
| `client_test.go` | El sobre de innertube: los headers del cliente WEB_REMIX, `playabilityError`, los 4xx/5xx, un cuerpo que no es JSON, una respuesta vacía, `short` y `snippet` |
| `player_test.go` | `parseSeconds`, `shellQuote` (incluido un `videoId` con comilla simple), `Status` sin player, `Stop` sin player y `alive` con un PID inválido |
| `player_ipc_test.go` | El IPC de mpv contra el socket falso: `ipcCommand` (descarta eventos, propaga el error), `ipcNumber`, `ipcBool`, y `Status` con EOF, `starting`, pausa y `Seek` |
| `player_stop_test.go` | `Stop` mata el grupo entero y limpia el estado y el socket |
| `fixes_test.go` | Los hallazgos Y1–Y5 (CRÍTICOS y ALTOS) |
| `resto_test.go` | Los hallazgos Y6–Y14, Y18 y Y20 (MEDIO y BAJO) |
| `coverage_test.go` | Y19: la cobertura que declara el README es la real |
| `cli/root_test.go` | El parseo de flags del CLI (banderas en cualquier posición, `--x=y`, `--x y`), `flags.value`, `flags.number`, el reloj y el despacho de comandos |
| `cli/resto_test.go` | Los hallazgos Y14–Y17 del CLI |

## Tests por contrato

### Y1–Y5 (CRÍTICOS y ALTOS, ola del 2026-11-19)

| Test | Contrato que fija |
|---|---|
| `TestStopNoMataUnPIDQueNoEsNuestro` | `Stop` sólo manda señal si el `startTime` del `player.json` coincide con el del proceso vivo |
| `TestNoSeBorraElArchivoQueDiceElSocket` | Ni `stop` ni `status` borran el path que trae el campo `socket` del JSON |
| `TestFindRendererEsDeterminista` | El mismo payload da siempre el mismo token de continuación |
| `TestElRebaseNoTruncaLaColaEnDisco` | Si el guardado final del rebase falla, la cola vieja queda entera en disco |
| `TestWriteJSONConcurrenteNoPierdeEscrituras` | Dos escritores concurrentes del mismo archivo no fallan ni pierden escrituras |

### Y6–Y20 (MEDIO y BAJO, ola del 2026-11-20)

| Test | Contrato que fija |
|---|---|
| `TestPlayCortaElYTDLPColgado` | `Play` tiene timeout: un `yt-dlp` colgado se corta y el comando devuelve error |
| `TestResolveMetadataRespetaElContextoVencido` | `resolveMetadata` respeta el contexto y no se queda esperando |
| `TestPlayMataElProcesoSiNoGuardaElEstado` | Si el estado no se puede guardar, `Play` mata el proceso del reproductor: no queda huérfano |
| `TestEnsureDirAjustaLosPermisosDeUnDirectorioExistente` | `EnsureDir` deja el directorio en `0700` aunque ya existiera en `0755` |
| `TestThumbsNoEscribeFueraDelDirectorio` | Un `videoId` con `../` no saca la escritura del directorio de portadas |
| `TestThumbsConVideoIDHostilNoPideLaPortada` | Un `videoId` con `/` ni siquiera dispara la petición |
| `TestSessionPathNoSaleDeSessions` | `sessionPath` nunca devuelve una ruta fuera de `sessions/` |
| `TestLoadSessionRechazaUnIDConTraversal` | Un id de sesión con `../` no lee ningún JSON de fuera |
| `TestSaveSessionRechazaUnIDConTraversal` | Un id de sesión con `../` no escribe ninguna sesión fuera |
| `TestBestThumbUsaUnaPortadaGrandeSiNoHayChica` | Si la página sólo trae portadas de más de 400 px, la fila tiene portada |
| `TestBestThumbPrefiereLaChicaSiHayUnaChica` | La red de seguridad no reemplaza la preferencia por la portada chica |
| `TestDurationToSecondsNoDesborda` | Una duración enorme da `0`: nunca un segundo negativo ni un overflow |
| `TestDurationToSecondsSigueParseandoLoNormal` | El techo no rompe los casos buenos ("3:45" → 225, "1:02:03" → 3723) |
| `TestStatusConEstadoCorruptoEsErrorYLimpiaElArchivo` | Un `player.json` corrupto es un error (no "parado") y el archivo roto se limpia |
| `TestStatusSinPlayerDevuelveParadoSinError` | Sin `player.json`, `Status` dice "parado" con `exit 0` |
| `TestThumbsConLimiteNoPositivoNoBajaNinguna` | `limit <= 0` en `Thumbs` no baja ninguna portada |
| `TestEnrichDurationsConLimiteNoPositivoNoConsultaNinguno` | `limit <= 0` en `EnrichDurations` no consulta el player: los dos significan lo mismo |
| `TestElLimiteNegativoCaeAlDefault` | El CLI no deja pasar un `--limit` negativo: cae al default |
| `TestElRelojDelCLILlevaHoras` | El reloj del CLI lleva horas: 3.723 s se muestran como "1:02:03" |
| `TestSeekConUnArgumentoQueNoEsNumeroFalla` | `seek abc` falla con un error legible, en vez de saltar a 0 |
| `TestSeekConUnArgumentoParcialmenteNumeroTambiénFalla` | `seek 12abc` también falla (antes se aceptaba como 12) |
| `TestSeekSinArgumentoNoFallaPorElNumero` | `seek` sin argumento no es un error de parseo |
| `TestElComandoPlayNoTieneCodigoMuerto` | El comando `play` no tiene la rama muerta `song.Title = ""` |
| `TestElComandoPlayNoRellenaElTituloConElVideoId` | `play` sin `--title` deja el título vacío (lo resuelve `Play` con yt-dlp) |
| `TestElReintentoNoEsperaDespuesDelUltimoIntento` | La espera de 400 ms cae entre intentos: después del último no espera |
| `TestElREADMEDeclaraLaCoberturaReal` | La cobertura que declara el README es la real, medida en un subproceso |
| `TestElREADMEDistingueLosDosPaquetes` | El README dice la cobertura de cada paquete por separado, no una cifra ambigua |
| `TestElPlayerCompilaFueraDeUnix` | El paquete compila para `windows` y `darwin`: lo de Unix va con build tag |

### Contratos que ya existían y siguen fijados

| Test | Contrato que fija |
|---|---|
| `TestElSocketSeDerivaDelDataDir` | El socket se deriva del `dataDir`, nunca del JSON |
| `TestElEstadoDelPlayerSePuedeLeerYEscribir` | El `player.json` conserva el `startTime` de Y1 |
| `TestElGrupoDelPlayerSeMataEntero` | El mecanismo de grupo (que Y20 movió de archivo) sigue matando el grupo entero |
| `TestStatusInformaElFinDeLaPista` | EOF es la señal de "pasa a la siguiente" y llega antes de que muera el proceso |
| `TestStatusInformaQueEstaArrancando` | Sin `time-pos` se informa `starting` con la duración de catálogo |
| `TestPauseAlternaElEstado` | Pausar no es detener |
| `TestSeekAceptaPosicionesNegativas` | Un salto negativo se clava en 0 |

## Lo que no se testea

- `Play` y `resolveMetadata` contra el `yt-dlp` y el `mpv` **reales**: los
  tests los cubren con dobles en el PATH, no contra YouTube de verdad.
- El contrato real de innertube (los `params` por país, el prefijo `RDAMVM`,
  los shards): los fixtures son recortes de respuestas reales.
- El lado Noctalia que llama a este binario: qué hace el panel cuando el
  comando devuelve `{"error": ...}` con exit 1.
