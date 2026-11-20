# Tests de `internal/plugins/we`

Qué prueba cada test del módulo. Cubre los **dos** paquetes:

- `internal/plugins/we` (lógica): structs, funciones y errores. **No imprime**
  y no habla con el disco más allá de `t.TempDir()`.
- `internal/plugins/we/cli` (comandos `wecli`): parseo de flags, salida y
  códigos de exit.

El subpaquete `cli` tiene su propio mapa detallado en
[`cli/tests.md`](cli/tests.md). Acá está el mapa **del módulo completo**, con
una fila por archivo de test y una por test.

## Cómo se aísla

| Auxiliar | Qué hace |
|---|---|
| `t.TempDir()` | Cada test trabaja en un directorio temporal propio. |
| `t.Setenv("HOME", …)` | `DefaultRoot` y `toolDirs()` leen el home del usuario: con un `HOME` falso, el workshop y las copias portables de ffmpeg son los del test. |
| `t.Setenv("PATH", …)` | Para probar "no hay ffmpeg" o "ffmpeg está en el PATH" sin tocar el sistema. |
| `t.Setenv("B3L0W_FFMPEG_DIR", …)` | Apaga o apunta el override de ffmpeg. |
| stubs en el `PATH` | Un `ffmpeg` / `ffprobe` / `linux-wallpaperengine` de mentira que anota sus argumentos y escribe el archivo de salida. **Ni el ffmpeg real ni el motor real se ejecutan** salvo en los tests marcados como reales. |
| `escribirPNG` | Un PNG válido por `image/png`, para el stub del motor. |

Ningún test toca la red, el home real ni binarios del sistema.

## Mapa por archivo

### Paquete `we` (lógica)

| Archivo | Qué cubre | Tests |
|---|---|---|
| `catalog_test.go` | El escaneo del workshop, la normalización a `Item`, `Filter`/`Find`, `Slug`/`OutputName`. | 3 |
| `deps_test.go` | `CheckDeps` (workshop, ffmpeg, ffprobe, engine, record), los tags JSON y `MissingDeps`/`Summary`. | 7 |
| `extract_test.go` | `Extract`: camino rápido, `skipped`, validaciones, `copyable`, formatos y transcode real. | 7 |
| `ffmpeg_test.go` | `parseProbe`, `tailLines`, `Locate`, `Probe` real y `Run` con stderr. | 11 |
| `manifest_test.go` | Round-trip, `Upsert`, escritura atómica, errores de carga y el `items: []`. | 5 |
| `poster_test.go` | `Poster`/`Posters`: opciones, skip, overwrite, render con motor falso, timeouts, cancelación y lotes parciales. | 12 |

### Paquete `wecli` (comandos)

| Archivo | Qué cubre | Tests |
|---|---|---|
| `cli/root_test.go` | Dispatch, help y códigos de salida de `Run`; `humanBytes`; `pick` (ids, `--all`, errores); `catalog` (items, vacío, inexistente, autodetección). | 13 |

| Test | Qué fija |
|---|---|
| `TestScanNormalizaYOrdena` | `Scan` normaliza el `project.json`: `File` y `Preview` resueltos contra la carpeta del item, el `type` unificado, el item sin `project.json` legible vuelve con `Err` cargado (no aborta el scan) y el resultado queda ordenado por título. |
| `TestFilterYFind` | `Filter` filtra por tipo y por query (título o id, sin distinguir mayúsculas); `Find` devuelve el item por id y falla con un id inexistente. |
| `TestSlugYOutputName` | `Slug` produce el kebab-case esperado en cada caso y `OutputName` arma `<id>-<slug>.<ext>`. |

## `deps_test.go` — el sondeo del entorno

| Test | Qué fija |
|---|---|
| `TestCheckDepsRootVacio` | Con `root` vacío y autodetección: el workshop aparece con `Ok` y su `Path`; ffmpeg y ffprobe **no** aparecen disponibles; `record` dice que falta `grim`; el engine con `Ok=false` no trae `Path`. |
| `TestCheckDepsWorkshopInexistente` | Una raíz inexistente deja `Workshop.Ok=false` con `Items=0`, pero conserva el `Path` probed. |
| `TestCheckDepsCuentaItems` | `Workshop.Items` cuenta los items del catálogo. |
| `TestCheckDepsHerramientasEnPATH` | ffmpeg y ffprobe del `PATH` se resuelven con su ruta y su versión. |
| `TestCheckDepsFfmpegFueraDePATH` | Sin ffmpeg en el `PATH`, `Locate` lo encuentra en `~/.hermes/tools/ffmpeg-*/bin/`; el ffprobe hermano (en el mismo `bin/`) también se encuentra, aunque no esté en el `PATH`. |
| `TestDepsJSONTags` | Los tags JSON de `Deps`: las claves `engine`, `ffmpeg`, `ffprobe`, `workshop` y `record` están y `workshop`/`record` tienen la forma que espera el plugin. |
| `TestDepsMissingYSummary` | `MissingDeps` lista las obligatorias que faltan y vacío cuando están todas; `Summary` dice `ffmpeg:falta`/`record:` en el caso malo y `workshop:41 items`/`record:ok` en el bueno. |

## `extract_test.go` — la extracción del video

| Test | Qué fija |
|---|---|
| `TestExtractCopiaExactaSinFfmpeg` | El camino rápido copia byte a byte sin ffmpeg: `Method=copy`, `Bytes` del tamaño de la fuente, **permisos del origen** (`0640`) y contenido idéntico. |
| `TestExtractSkipSiElDestinoExiste` | Sin `Overwrite` devuelve `Skipped` con el tamaño del existente y **no lo pisa**; con `Overwrite` reemplaza el contenido. |
| `TestExtractRechazaTipoNoVideo` | Un item `scene`/`web` falla, el error sugiere el método `record` y **no** se crea el directorio de salida. |
| `TestExtractValidaciones` | Las validaciones previas: tipo no video, `File` vacío, `OutDir` vacío y `CRF` negativo, cada una con su mensaje. |
| `TestExtractCopyable` | La regla `copyable`: mismo contenedor (`mp4`), `CRF` 0 y origen `.mp4` ⇒ copia; cualquier otra cosa ⇒ recodifica. |
| `TestExtractNormalizaFormato` | `normalizeFormat` acepta `mp4`/`webm`/vacío (con o sin punto) y rechaza lo demás (`gif` incluido). |
| `TestExtractTranscodificaReal` | **Usa el ffmpeg real**: recodifica a mp4 y webm, `Method=transcode`, el archivo queda con la extensión pedida, `Probe` ve el codec y las dimensiones esperadas (96x64) y **no queda el temporal** en el directorio. |

## `ffmpeg_test.go` — localización, ejecución y parseo del probe

| Test | Qué fija |
|---|---|
| `TestParseProbeFixture` | `parseProbe` sobre un JSON de ffprobe: dimensiones 3840x2160, codec `h264` y duración 35.2. |
| `TestParseProbeDuracionDesdeElStream` | Cuando el `format` no trae duración, se usa la del stream de video. |
| `TestParseProbeIgnoraStreamsQueNoSonVideo` | Los streams que no son de video (audio, subtítulos) se saltan. |
| `TestParseProbeJSONRoto` | Un JSON inválido da error, no un `Meta` vacío (`ffmpeg.go:197`). |
| `TestParseProbeDuracionBasura` | `duration` con basura (`N/A`, `nan`) da 0 sin invalidar el resto; una duración numérica también se acepta. |
| `TestFfmpegTailLines` | `tailLines` devuelve las últimas N líneas (20 por defecto), normaliza `\r`, y da vacío para una entrada vacía o de sólo espacios. |
| `TestFfmpegLocateReportaRutasProbadas` | `Locate` devuelve la ruta absoluta con `Ok`; si no encuentra el binario, el error **nombra el binario** y lista las rutas probadas. |
| `TestFfmpegProbeReal` | **Usa el ffmpeg real**: `Probe` sobre un clip generado ve 64x48, codec `h264` y una duración en el rango de 1 s. |
| `TestProbeArchivoInexistente` | `Probe` de un archivo que no existe da error. |
| `TestRunPropagaStderr` | `Run` devuelve un error que incluye el **stderr** del binario y su exit code; un comando exitoso no devuelve error. |
| `TestProbeSinFfprobe` | Sin ffprobe resoluble, `Probe` falla con un error que lo dice. |

## `manifest.go` — el índice de lo generado

| Test | Qué fija |
|---|---|
| `TestManifestRoundTrip` | `SaveManifest` → `LoadManifest` conserva `Version`, `Generated`, `root`/`out` y los `Entry` completos; el JSON sale con los **tags** (`id`, `title`, `type`, `status`, `file`, `source`, `method`, `bytes`, `duration`, `width`, `height`, `error`) y no con los nombres Go. |
| `TestManifestUpsertReemplazaYAgrega` | `Upsert` **reemplaza** el entry con el mismo id (no duplica) y **agrega** el nuevo al final, sin cambiar el orden de los anteriores. |
| `TestManifestSaveEsAtomico` | `SaveManifest` deja permisos `0644`, **no deja archivos sueltos** en el directorio y el segundo guardado **no** deja contenido viejo (renombra el temporal). |
| `TestManifestLoadErrores` | Un manifest inexistente **envuelve `os.ErrNotExist`**; uno vacío y uno truncado fallan. |
| `TestManifestSaveSinItemsSerializaArrayVacio` | Sin items, `items` se serializa como `[]` y **no** como `null`; al releer, `Version` toma el default `ManifestVersion`. |

## `poster_test.go` — el PNG vía el motor

| Test | Qué fija |
|---|---|
| `TestPosterOptionsNormalize` | `normalize` completa los ceros: `Size` → `DefaultPosterSize` (incluso con una dimensión en cero o negativa), `Delay` → `defaultScreenshotDelay`, `Jobs` → la mitad de las CPUs con tope de 4 y mínimo 1; y **no pisa** los valores explícitos. |
| `TestPosterSalteaSiElPNGYaExiste` | Con el PNG ya hecho, `Poster` devuelve la ruta y **no toca el PNG existente**. |
| `TestPosterOverwriteSiPideRender` | Con `Overwrite`, `Poster` vuelve a renderizar (el error del engine menciona el engine). |
| `TestPosterRenderizaConEngineFalso` | El engine recibe `--bg <id>`, `--window 0x0x<w>x<h>`, `--screenshot` y `--screenshot-delay`; se escribe `<out>/<id>.png` como PNG completo y **no quedan archivos de más** en el directorio. |
| `TestPosterRespetaSizeYDelay` | El PNG renderizado sale con el `size` y el `delay` pedidos. |
| `TestPosterValidaEntrada` | Sin `OutDir` o sin `ID` falla antes de arrancar el engine. |
| `TestPosterContextCancelado` | Con el contexto cancelado, el error envuelve `context.Canceled`. |
| `TestPosterCancelaYNoDejaElEngineVivo` | La cancelación corta el render **a tiempo** y no deja el engine vivo. |
| `TestPosterTimeoutSinPNG` | Si el engine no escribe el PNG, el `renderTimeout` corta y da error de timeout, a tiempo. |
| `TestPostersFallaParcialNoAborta` | Un render fallido **no aborta** el lote: los 3 resultados viajan, el que falló trae `Err` **con la salida del engine**, los otros dos tienen su PNG en disco. |
| `TestPostersSkipYOverwrite` | El item cuyo PNG ya existe viene `Skipped` y su PNG no se toca; el otro sí se renderiza; con `Overwrite` ya no se saltea ninguno y se regenera un PNG válido. |
| `TestPostersVacioYTodoFallido` | Un lote vacío no es un error; sin `OutDir` falla **antes** de arrancar workers; cuando todos fallan, los resultados parciales viajan igual. |

## `cli/` — la capa de comandos

El detalle por test está en [`cli/tests.md`](cli/tests.md). Acá el índice de
archivos y qué cubre cada uno.

| Archivo | Qué cubre | Tests |
|---|---|---|
| `cli/root_test.go` | Dispatch, help y códigos de salida de `Run`; `humanBytes`; `pick` (ids, `--all`, errores); `catalog` (items, vacío, inexistente, autodetección). | 13 |
| `cli/list_test.go` | `we list` (tabla, JSON, filtro por tipo y por query, flags con `=`, opciones desconocidas, root sin valor, catálogo vacío, items rotos) y `we info` (detalle, JSON, sin id, opción desconocida, id inexistente). | 15 |
| `cli/scan_test.go` | `we scan`: `--extensions` (imagen y video), listado, profundidad, JSON con los campos del plugin, título legible, filtro por videos, `--max`, carpeta suelta, sin carpeta, flags sin valor, `--max` inválido y grande, `--sort` inválido, `--depth` inválido, `--thumb-size` inválido, `--thumbs` sin valor, opción desconocida, y los tres ordenes (nombre, fecha, aleatorio). | 21 |
| `cli/convert_test.go` | `we convert`: extrae y escribe el manifiesto, `--json`, falla si no es video, `--out`/`--id`/`--all` faltantes, id inexistente, `--crf`/`--jobs` inválidos, opción desconocida, flags con `=`, y el `skipped` de lo que ya existe. | 11 |
| `cli/poster_test.go` | `we poster`: PNG con el motor, falla si el motor no escribe, sale con 1 si fallan todos, `--out` faltante, `--size`/`--jobs` inválidos, sin `--id` ni `--all`, opción desconocida. | 8 |
| `cli/still_test.go` | `we still`: el cuadro de un video, `--at`, el reintento desde el principio, imágenes sin `-ss`, GIF desde cero, archivo inexistente, `--file`/`--out` faltantes, `--size`/`--at` inválidos, opción desconocida, y el fallo si ffmpeg no escribe. | 11 |
| `cli/solid_test.go` | `we solid`: `parseHexColor` (las formas documentadas y las inválidas), el PNG del color con alpha 255, la creación del directorio, `--color`/`--out` faltantes, color inválido, `--size` inválido y demasiado grande, opción desconocida, flags con `=`. | 10 |
| `cli/thumbs_test.go` | El nombre `v-<djb2(ruta)>.jpg` con los valores exactos (contrato con el plugin), que dos rutas no compartan miniatura, la regla de frescura, la generación de miniaturas, que los videos no pasen por ffmpeg, y el reuso de la miniatura existente. | 6 |
| `cli/deps_test.go` | `we deps`: la tabla con sus marcas, el JSON completo, salida 0 con lo imprescindible, el conteo de items del workshop, salida 2 con workshop inválido, opción desconocida, `--root=`, el root por defecto y que no escriba nada en disco. | 9 |
| `cli/flags_test.go` | El parseo en todos los comandos: los 32 flags con valor rechazan el flag sin valor nombrándolo; el manifiesto ilegible se rehace con aviso; `--manifest` manda; `~/` se expande; carpeta inexistente da lista vacía; `info` no tiene `--root`; el resumen "X de Y". | 7 |
| `cli/errores_test.go` | Los caminos de error: catálogo inexistente, manifiesto que es un directorio, directorio de salida que no se puede crear (en `convert` y en `poster`), manifiesto ilegible que se rehace, item fallido marcado `failed` en el manifiesto, item roto en `info`, workshop ausente, ffmpeg ausente, destino de `solid` que no se puede escribir, directorios sin permisos, ocultos, y el `thumb` vacío cuando ffmpeg falla. | 16 |
| `cli/banderas_test.go` | Las banderas que cambian el resultado: los flags con `=` en `still`/`list`/`poster`, `--manifest` manda, y el render que no se repite salvo con `--overwrite`. | 6 |
| `cli/limites_test.go` | Los bordes: `--all` procesa todo, `--overwrite` rehace, `--format webm`, `--method extract` se acepta, formato inexistente, `--dir=`, carpeta sin permisos, carpeta de miniaturas inválida, `--size 8xzz`, destino de `still` que no se puede crear, y `humanTitle` sin texto. | 12 |
| `cli/helpers_test.go` | Sin tests: son los auxiliares compartidos (`captura`, `correr`, `escribirItem`, `raizWorkshop`, `homeConWorkshop`, los stubs de ffmpeg y del motor, `pathSinBinarios`, `escribirPNG`). Los usa el resto de la suite. | 0 |

## Tests de los fixes (18)

Cada uno fija un hallazgo del `bugs.md` y se verificó revirtiendo su arreglo.

| Test | Archivo | Contrato que fija |
|---|---|---|
| `TestScanRechazaUnFileQueSaleDelItem` | `catalog_test.go` | Un `file` con `..` fuera del item no se resuelve (C1) |
| `TestScanAceptaUnFileDentroDelItem` | `catalog_test.go` | Un `file` relativo dentro del item sí se resuelve (C1) |
| `TestConvertNoCopiaUnArchivoDeFueraDelItem` | `cli/convert_test.go` | `convert` no copia un item con el `file` fuera (C1) |
| `TestStillNoBorraLaMiniaturaPreviaSiFfmpegFalla` | `cli/still_test.go` | Si ffmpeg falla, la miniatura anterior sigue en disco (A1) |
| `TestSolidElTopeDeSizeNoSeSaltaConOverflow` | `cli/solid_test.go` | `--size` gigante no desborda: da error (A2) |
| `TestSolidNoPisaElArchivoDeSalida` | `cli/solid_test.go` | `solid` no pisa la salida sin `--overwrite` (A3) |
| `TestSolidConOverwriteReemplazaElArchivo` | `cli/solid_test.go` | Con `--overwrite` sí reemplaza (A3) |
| `TestFilterConUnTipoQueNoExisteNoDevuelveElCatalogoEntero` | `catalog_test.go` | Un tipo inexistente devuelve vacío, no todo (M1) |
| `TestListaFallaConUnTipoInvalido` | `cli/list_test.go` | `--type` inválido falla con motivo (M1) |
| `TestToolDirsOrdenanPorVersionYNoAlfabetico` | `ffmpeg_test.go` | ffmpeg-10.0 gana sobre ffmpeg-9.0 (M2) |
| `TestScanKindInvalidoFalla` | `cli/scan_test.go` | `--kind` inválido falla (M3) |
| `TestScanKindEnMayusculasSeAcepta` | `cli/scan_test.go` | `--kind VIDEO` se normaliza (M3) |
| `TestScanNoDejaUnaMiniaturaACortarEnDisco` | `cli/thumbs_test.go` | Una miniatura cortada no queda cacheada (M4) |
| `TestManifestLoadRechazaUnaVersionDesconocida` | `manifest_test.go` | `version` 99 es error, no se acepta (M5) |
| `TestConvertNoTocaUnManifiestoDeVersionDesconocida` | `cli/convert_test.go` | `convert` no reescribe un manifiesto futuro (M5) |
| `TestSlugConservaLasLetrasAcentuadas` | `catalog_test.go` | `Café Ñandú` → `cafe-nandu` (B1) |
| `TestPosterDelayDocumentadoEnFrames` | `poster_test.go` | `Delay` documentado en frames (B2) |
| `TestSummaryDistingueElWorkshopAusenteDelVacio` | `deps_test.go` | `workshop:falta` vs `workshop:ok` (B3) |

## Qué no cubre la suite

Los huecos verificados durante la revisión de integridad
([`bugs.md`](bugs.md)). Cada uno es un comportamiento que ningún test fija:

| Hueco | Test que falta | Hallazgo |
|---|---|---|
| El `file`/`preview` de `project.json` puede apuntar fuera de la carpeta del item | Un `Scan` con `file: "../secret.mp4"` que afirme que la ruta queda contenida | **C1** |
| `we still` borra el destino antes de extraer | Un `still` con ffmpeg que falla que afirme que la miniatura previa sigue en disco | **A1** |
| El tope de `--size` de `solid` se salta con overflow | Un `--size 4611686018427387904x4611686018427387904` que afirme que da error y no panic | **A2** |
| `we solid` pisa el archivo de salida | Un `solid` sobre un archivo existente que afirme que avisa o se niega | **A3** |
| `Filter` con un tipo inválido o `unknown` | Un `Filter(items, "unknown", "")` que afirme que devuelve sólo los `unknown` | **M1** |
| El orden de las copias portables de ffmpeg | Un `toolDirs()` con `ffmpeg-9.0` y `ffmpeg-10.0` que afirme que gana la 10.0 | **M2** |
| `--kind` inválido | Un `scan --kind videoz` que afirme que falla | **M3** |
| Una miniatura a medias queda cacheada | Un `scan --thumbs` con ffmpeg que escribe a medias que afirme que la siguiente pasada reintenta | **M4** |
| `LoadManifest` con `version` desconocido | Un `LoadManifest` de `{"version":99}` que afirme que da error | **M5** |
| `Slug` con acentos | Un `Slug("Café")` que afirme que conserva la `é` | **B1** |
| `PosterOptions.Delay` en frames | El doc del campo debería decir frames, como el de la constante | **B2** |
| `Summary()` y el bloque del workshop | Un `Summary()` que afirme `workshop:falta` | **B3** |

Además, sin test propio:

- **`we.Probe` no lo llama nadie en producción.** El grep del módulo lo
  muestra sólo definido en `ffmpeg.go:136` y usado en sus propios tests. Por
  eso el `Entry.Duration`, `Entry.Width` y `Entry.Height` del manifiesto
  quedan **siempre en 0**, aunque el README los liste como parte del
  contrato.
- **La rama `return 2` de `cli/poster.go:161` es inalcanzable**: cuando
  fallan todos los renders, `we.Posters` devuelve el error antes
  (`poster.go:294-296`) y el comando sale con 1. Ya está anotado en
  [`cli/bugs.md`](cli/bugs.md).
- **El contrato de nombres de miniaturas no se puede verificar contra el
  plugin Luau desde acá**: `internal/plugins/luau-shared/` no contiene el
  plugin de wallpaper (ver `bugs.md`, "Sin revisar"). Del lado Go, el test
  fija los valores exactos del hash.

## Verificación

```console
$ cd /home/b3low-z3r0/Documents/Github/Cli/branches/rev-we
$ go test ./internal/plugins/we/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we	3.028s
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we/cli	0.217s
```

**Totales:** 190 tests (45 en el paquete `we`, 145 en `wecli`), 0 fallos.
Ningún test fue modificado durante la revisión.

Los conteos salen de contar las funciones `Test*` de cada archivo:
`grep -rhoE '^func (Test[A-Za-z0-9_]+)' internal/plugins/we/*_test.go | wc -l`
→ 45, y lo mismo sobre `internal/plugins/we/cli/*_test.go` → 145.
