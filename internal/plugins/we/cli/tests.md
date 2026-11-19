# Tests de `internal/plugins/we/cli`

Qué prueba cada test del subpaquete. La capa de arriba (`internal/plugins/we`)
tiene su propia suite; esta cubre **el parseo de flags, la salida y los
códigos de exit**, que es lo que la capa de arriba no toca.

## Cómo se aísla

| Auxiliar | Qué hace |
|---|---|
| `captura` | Redirige `os.Stdout` o `os.Stderr` a un pipe y devuelve lo escrito. La lectura va en una goroutine para que una salida larga no tape al comando. |
| `correr` | Ejecuta `Run(args...)` y devuelve `(código, stdout, stderr)`. |
| `raizWorkshop` / `escribirItem` | Crea una raíz de workshop falsa (carpetas numéricas con `project.json`). |
| `homeConWorkshop` | Crea un `HOME` falso con el workshop adentro, para probar la autodetección (`we.DefaultRoot`). |
| `stubEnPATH` / `stubTool` | Escribe un ejecutable falso en un directorio temporal y lo pone al principio del `PATH`. |
| `stubFFmpeg` | Un `ffmpeg` de mentira que anota los argumentos con los que lo llamaron y escribe el archivo de salida. **El ffmpeg real nunca se ejecuta.** |
| `motorFalso` | Un `linux-wallpaperengine` de mentira que escribe el PNG que le piden. **El motor real nunca se ejecuta.** |
| `pathSinBinarios` | Deja el `PATH` y `B3L0W_FFMPEG_DIR` sin ningún programa: el estado "no hay ffmpeg" es determinista. |
| `escribirPNG` | Un PNG válido, para el stub del motor. |

Ningún test toca la red, el home del usuario ni binarios del sistema: todo va
a `t.TempDir()` y a `$HOME`/`$PATH` temporales.

## `root_test.go` — dispatch, help y códigos de salida

| Test | Qué fija |
|---|---|
| `TestRunSinArgsMuestraElUso` | Sin argumentos imprime el uso y sale con 0. |
| `TestRunAyudaSaleConCero` | `help`, `-h` y `--help` salen con 0. |
| `TestRunComandoDesconocidoFalla` | Un comando inventado avisa por stderr, imprime el uso por stdout y sale con 1. |
| `TestRunAyudaDeCadaComando` | Los 8 comandos documentan su uso en `--help` y salen con 0. |
| `TestHumanBytesEscalaLasUnidades` | La tabla de tamaños: B, KB, MB, GB. |
| `TestPickSinIdNiAllFalla` | `pick` sin `--id` ni `--all` falla diciendo qué falta. |
| `TestPickConIdInexistenteFalla` | `pick` con un id que no está falla. |
| `TestPickConAllDevuelveTodo` | `pick --all` devuelve el catálogo entero. |
| `TestPickConIdEligeSoloEse` | `pick --id` devuelve sólo ese item. |
| `TestCatalogDevuelveLosItems` | `catalog` devuelve los items y la ruta absoluta. |
| `TestCatalogVacioFalla` | Un catálogo sin items da `we.ErrEmpty`. |
| `TestCatalogInexistenteFalla` | Una raíz inexistente falla. |
| `TestCatalogSinRootBuscaElWorkshopPorDefecto` | Sin `--root` busca el workshop del `HOME` y explica qué probó. |

## `list_test.go` — `we list` y `we info`

| Test | Qué fija |
|---|---|
| `TestListaMuestraLaTabla` | La tabla con sus 4 columnas y el conteo por stderr. |
| `TestListaEnJSONEmiteElCatalogoCompleto` | `--json` emite un array parseable con los items. |
| `TestListaFiltraPorTipo` | `--type` deja pasar sólo ese tipo. |
| `TestListaFiltraPorConsulta` | `--query` busca en el título, sin distinguir mayúsculas. |
| `TestListaAceptaElValorConIgual` | `--root=DIR` es una forma válida. |
| `TestListaAceptaElTipoConIgual` | `--type=video` también (bug 1 de `bugs.md`). |
| `TestListaOpcionDesconocidaFalla` | Un flag inexistente sale con 1. |
| `TestListaRootSinValorFalla` | `--root` sin valor falla nombrando el flag. |
| `TestListaCatalogoVacioFalla` | Un catálogo vacío avisa que está vacío. |
| `TestListaMarcaLosItemsRotos` | Un `project.json` roto sale como `(roto)`. |
| `TestInfoMuestraElDetalle` | `we info` imprime id, título, tipo, tamaño, archivo y preview. |
| `TestInfoEnJSONEmiteElItem` | `we info --json` emite un item parseable. |
| `TestInfoSinIdFalla` | `we info` sin id avisa que falta. |
| `TestInfoOpcionDesconocidaFalla` | Un flag inexistente en `info` sale con 1. |
| `TestInfoConIdInexistenteFalla` | Un id que no está en el catálogo avisa que no existe. |

## `scan_test.go` — `we scan`

| Test | Qué fija |
|---|---|
| `TestScanExtensionesImprimeLaLista` | `--extensions` imprime el array JSON de extensiones, ordenado. |
| `TestScanExtensionesDeVideo` | `--kind video` cambia la lista de extensiones. |
| `TestScanListaLasImagenes` | Lista sólo imágenes, respeta la profundidad y salta ocultos. |
| `TestScanRespetaLaProfundidad` | `--depth 1` deja de bajar al segundo nivel. |
| `TestScanEnJSONTraeLosCamposDelPlugin` | El JSON trae los campos que consume la tarjeta (`id`, `file`, `type`, `size`, `mod`, `name`, `title`). |
| `TestScanConvierteElNombreEnTitulo` | El título es el nombre sin extensión y con `_`/`-` como espacios. |
| `TestScanFiltraPorVideos` | `--kind video` lista sólo videos y los marca como `video`. |
| `TestScanRespetaElMaximo` | `--max` corta la lista. |
| `TestScanTomaLaCarpetaSuelta` | Una ruta sin `--dir` se toma como carpeta. |
| `TestScanSinCarpetaFalla` | Sin carpeta avisa que hay que pasar `--dir`. |
| `TestScanDirSinValorFalla` | `--dir` sin valor falla. |
| `TestScanMaxInvalidoFalla` | `--max` no numérico falla. |
| `TestScanMaxGrandeFalla` | `--max 100001` falla: el tope existe para no reservar esa memoria de una. |
| `TestScanSortInvalidoFalla` | `--sort` inválido falla. |
| `TestScanDepthInvalidoFalla` | `--depth` negativo falla. |
| `TestScanThumbSizeInvalidoFalla` | `--thumb-size` menor a 64 falla. |
| `TestScanThumbsSinValorFalla` | `--thumbs` sin valor falla. |
| `TestScanOpcionDesconocidaFalla` | Un flag inexistente falla (bug 3 de `bugs.md`). |
| `TestScanOrdenaPorNombre` | El orden default es por nombre. |
| `TestScanOrdenaPorFecha` | `--sort date` pone el más nuevo primero. |
| `TestScanOrdenAleatorioNoPierdeArchivos` | `--sort random` no pierde ni duplica archivos. |

## `convert_test.go` — `we convert`

| Test | Qué fija |
|---|---|
| `TestConvertSacaElVideoYEscribeElManifiesto` | Extrae el video, lo nombra `<id>-<slug>.<ext>` y escribe el manifiesto con `status: ok`. |
| `TestConvertEnJSONEmiteElManifiesto` | `--json` emite el manifiesto entero. |
| `TestConvertFallaSiElWallpaperNoEsVideo` | Un item `scene` falla: sale con 2 y el manifiesto lo marca `failed`. |
| `TestConvertSinOutFalla` | Sin `--out` avisa que falta. |
| `TestConvertSinIdNiAllFalla` | Sin `--id` ni `--all` avisa que falta. |
| `TestConvertConIdInexistenteFalla` | Un id que no está avisa que no existe. |
| `TestConvertCrfInvalidoFalla` | `--crf` no numérico falla. |
| `TestConvertJobsInvalidoFalla` | `--jobs 0` falla. |
| `TestConvertOpcionDesconocidaFalla` | Un flag inexistente falla. |
| `TestConvertAceptaLosValoresConIgual` | `--out=`, `--root=`, `--id=` y `--crf=` funcionan. |
| `TestConvertSalteaLoQueYaExiste` | La segunda corrida marca el item `skipped` en vez de reescribirlo. |

## `poster_test.go` — `we poster`

| Test | Qué fija |
|---|---|
| `TestPosterGeneraElPNGConElMotor` | El motor recibe `--bg <id>` y `--window 0x0x1280x720`, se escribe `<out>/<id>.png` y el manifiesto queda con `status: ok`. |
| `TestPosterFallaSiElMotorNoEscribeElPNG` | Si el motor no escribe el PNG, el comando falla. |
| `TestPosterFallaSiTodosLosWallpapersFallan` | Si fallan todos, sale con 1. |
| `TestPosterSinOutFalla` | Sin `--out` avisa que falta. |
| `TestPosterSizeInvalidoFalla` | `--size` que no es `ANCHOxALTO` falla. |
| `TestPosterJobsInvalidoFalla` | `--jobs 0` falla. |
| `TestPosterSinIdNiAllFalla` | Sin `--id` ni `--all` avisa que falta. |
| `TestPosterOpcionDesconocidaFalla` | Un flag inexistente falla. |

## `still_test.go` — `we still`

| Test | Qué fija |
|---|---|
| `TestStillSacaElCuadroDeUnVideo` | ffmpeg recibe `-y -i <video> -frames:v 1 -vf scale=320:-2 -q:v 3 <salida>` y se imprime la ruta de la imagen. |
| `TestStillBuscaElSegundoPedido` | `--at 2.5` se traduce a `-ss 2.50`. |
| `TestStillReintentaDesdeElPrincipio` | Si el cuadro no sale en el segundo pedido, reintenta con `-ss 0`. |
| `TestStillDeUnaImagenNoBuscaFrame` | Para un JPG no se pasa `-ss` (ya es un cuadro). |
| `TestStillDeUnGifArrancaEnCero` | Un GIF arranca en `-ss 0` aunque se pida otro segundo. |
| `TestStillFallaSiElArchivoNoExiste` | Un archivo inexistente avisa que no se puede leer. |
| `TestStillSinFileNiOutFalla` | Faltan `--file` y `--out`. |
| `TestStillSizeInvalidoFalla` | `--size` que no es `ANCHOxALTO` falla. |
| `TestStillAtInvalidoFalla` | `--at` negativo falla. |
| `TestStillOpcionDesconocidaFalla` | Un flag inexistente falla. |
| `TestStillFallaSiFfmpegNoEscribe` | Si ffmpeg no escribe el archivo, el comando falla. |

## `solid_test.go` — `we solid`

| Test | Qué fija |
|---|---|
| `TestParseHexColorAceptaLasFormasDocumentadas` | `#rrggbb`, `rrggbb`, `#rgb` y `#f0a` → el color correcto, siempre opaco. |
| `TestParseHexColorRechazaLoInvalido` | Las formas raras dan error. |
| `TestSolidGeneraElPNGDelColor` | El PNG sale del tamaño y del color pedidos, con alpha 255. |
| `TestSolidCreaElDirectorioDeSalida` | Crea la carpeta de destino si falta. |
| `TestSolidSinColorNiOutFalla` | Faltan `--color` y `--out`. |
| `TestSolidColorInvalidoFalla` | Un color inválido falla. |
| `TestSolidSizeInvalidoFalla` | `--size` inválido falla. |
| `TestSolidSizeDemasiadoGrandeFalla` | `100000x100000` falla: el tope existe para que el kernel no mate el proceso. |
| `TestSolidOpcionDesconocidaFalla` | Un flag inexistente falla (bug 4 de `bugs.md`). |
| `TestSolidAceptaElValorConIgual` | `--color=`, `--out=` y `--size=` funcionan. |

## `thumbs_test.go` — `we scan --thumbs`

| Test | Qué fija |
|---|---|
| `TestThumbNameUsaElHashAcordadoConElPlugin` | El nombre `v-<djb2(ruta)>.jpg` con los valores exactos: es contrato con el plugin de Noctalia. |
| `TestThumbNameCambiaConLaRuta` | Dos rutas distintas no comparten miniatura. |
| `TestFreshSoloDaPorFrescoSiLaMiniaturaEsMasNueva` | Una miniatura más vieja, vacía o sin origen no se reusa. |
| `TestScanGeneraLasMiniaturas` | `--thumbs` genera un JPEG por imagen y lo agrega al JSON. |
| `TestScanNoGeneraMiniaturasDeVideos` | Los videos no pasan por ffmpeg: un video necesita extraer un cuadro. |
| `TestScanReusaLaMiniaturaExistente` | La segunda pasada no vuelve a llamar a ffmpeg si el original no cambió. |

## `deps_test.go` — `we deps`

| Test | Qué fija |
|---|---|
| `TestDepsMuestraLaTabla` | La tabla con ffmpeg, ffprobe, engine, workshop y record; marca lo que falta con `FALTA` y sale con 2. |
| `TestDepsEnJSONEmiteElReporteCompleto` | `--json` emite el reporte de `we.Deps`. |
| `TestDepsSaleConCeroSiEstanLoImprescindible` | Con ffmpeg y workshop presentes sale con 0. |
| `TestDepsEnJSONCuentaLosItemsDelWorkshop` | El JSON cuenta los items del workshop. |
| `TestDepsConWorkshopInvalidoSaleConDos` | Un workshop inexistente sale con 2. |
| `TestDepsOpcionDesconocidaFalla` | Un flag inexistente sale con 1. |
| `TestDepsAceptaElRootConIgual` | `--root=DIR` es una forma válida (bug 2 de `bugs.md`). |
| `TestDepsSinRootBuscaElWorkshopPorDefecto` | Sin `--root` usa el workshop del `HOME`. |
| `TestDepsNoEscribeNadaEnDisco` | `deps` no escribe nada. |

## `flags_test.go` — el parseo, en todos los comandos

| Test | Qué fija |
|---|---|
| `TestFlagsSinValorFallanEnTodosLosComandos` | Los 32 flags con valor de los 8 comandos rechazan el flag sin valor nombrándolo. |
| `TestConvertRehaceElManifiestoIlegible` | Un manifiesto a medias no se ignora en silencio: se avisa y se rehace. |
| `TestConvertUsaElManifiestoIndicado` | `--manifest` manda sobre el `<out>/manifest.json` por defecto. |
| `TestScanExpandeElTildeEnLaCarpeta` | `~/fotos` se expande al home del usuario. |
| `TestScanIgnoraCarpetaInexistente` | Una carpeta que no existe da una lista vacía, no un error. |
| `TestInfoIgnoraLasBanderasQueNoConoce` | `info` no tiene `--root`: el catálogo es siempre el del sistema. |
| `TestListaMuestraCuantosDeCuantos` | El resumen dice "X de Y": X es la selección, Y el catálogo (bug 5 de `bugs.md`). |

## `errores_test.go` — los caminos de error

| Test | Qué fija |
|---|---|
| `TestConvertFallaSiElCatalogoNoExiste` | Una raíz inexistente avisa que no pudo leer el catálogo. |
| `TestConvertFallaSiElManifiestoEsUnDirectorio` | Si el manifiesto no se puede escribir, el comando avisa y sale con 1. |
| `TestConvertFallaSiNoPuedeCrearElDirectorioDeSalida` | Si el `--out` no se puede crear, avisa y sale con 1. |
| `TestPosterFallaSiNoPuedeCrearElDirectorioDeSalida` | Lo mismo en `poster`. |
| `TestPosterRehaceElManifiestoIlegible` | Un manifiesto a medias se rehace, no se ignora en silencio. |
| `TestPosterFallaSiElManifiestoEsUnDirectorio` | Si el manifiesto no se puede escribir, avisa y sale con 1. |
| `TestPosterMarcaElWallpaperQueFallo` | Un fallo parcial no es fatal: sale con 0, cuenta `1 fallidos` y marca el item `failed` en el manifiesto. |
| `TestInfoMuestraElErrorDeUnItemRoto` | `info` de un item con `project.json` roto imprime `error:`. |
| `TestInfoFallaSiNoHayWorkshop` | Sin workshop en el `HOME`, `info` avisa que no lo encontró. |
| `TestStillFallaSiNoHayFfmpeg` | Sin ffmpeg, `still` avisa que falta. |
| `TestSolidFallaSiNoPuedeEscribirElArchivo` | Si el destino no se puede abrir, avisa y sale con 1. |
| `TestSolidFallaSiNoPuedeCrearElDirectorio` | Si la carpeta del PNG no se puede crear, avisa y sale con 1. |
| `TestScanIgnoraLosArchivosOcultos` | Un archivo oculto no se lista. |
| `TestScanNoRepiteElMismoArchivo` | La misma carpeta dos veces no duplica los archivos. |
| `TestScanSinFfmpegNoGeneraMiniaturas` | Sin ffmpeg el listado sale igual, sin miniaturas. |
| `TestScanDejaLaMiniaturaVaciaSiFfmpegFalla` | Si el ffmpeg falla, el `thumb` queda vacío y el comando sale con 0. |

## `banderas_test.go` — las banderas que cambian el resultado

| Test | Qué fija |
|---|---|
| `TestStillAceptaLosValoresConIgual` | `--file=` y `--out=` funcionan. |
| `TestListaAceptaLaConsultaConIgual` | `--query=` funciona. |
| `TestPosterAceptaLosValoresConIgual` | `--out=`, `--root=` y `--id=` funcionan. |
| `TestPosterUsaElManifiestoIndicado` | `--manifest` manda sobre el `<out>/manifest.json` por defecto. |
| `TestPosterNoRepiteElRenderSiElPNGYaExiste` | Con el PNG ya hecho el engine no corre otra vez. |
| `TestPosterRepiteElRenderConOverwrite` | Con `--overwrite` el engine corre otra vez. |

## `limites_test.go` — los bordes

| Test | Qué fija |
|---|---|
| `TestConvertConTodosProcesaTodoElCatalogo` | `--all` convierte el catálogo entero. |
| `TestConvertSobreescribeLoQueYaExiste` | `--overwrite` rehace el archivo en vez de marcarlo `skipped`. |
| `TestConvertAceptaElFormatoWebm` | `--format webm` recodifica con `libvpx-vp9` y el archivo sale `.webm`. |
| `TestConvertAceptaElMetodoExtract` | `--method extract` se acepta y no rompe la extracción. |
| `TestConvertFallaSiElFormatoNoExiste` | Un formato que no existe sale con 2 y dice los soportados. |
| `TestPosterFallaSiElCatalogoNoExiste` | Una raíz inexistente avisa que no pudo leer el catálogo. |
| `TestScanAceptaElDirConIgual` | `--dir=` funciona. |
| `TestScanIgnoraCarpetaSinPermisos` | Una carpeta sin permisos no corta el escaneo (se salta como root). |
| `TestScanThumbsSinCarpetaValidaNoRompeElListado` | Si la carpeta de miniaturas no se puede crear, el listado sale igual. |
| `TestSolidSizeConLetrasFalla` | `--size 8xzz` falla: el separador está pero los números no son números. |
| `TestStillFallaSiNoPuedeCrearElDirectorio` | Si la carpeta de la imagen no se puede crear, avisa y sale con 1. |
| `TestHumanTitleSinTextoDevuelveElNombre` | Un nombre que es sólo extensión se devuelve tal cual. |

## Cobertura

```bash
go test ./internal/plugins/we/cli/ -cover
```

98,1 % de las sentencias del subpaquete (145 tests, 0 fallos). Lo que queda
sin cubrir son ramas que sólo se alcanzan con el sistema real: el error de
`filepath.Abs`, el error de serialización del JSON, el `png.Encode` que falla
y el `walkErr` de un archivo suelto. La rama `return 2` de `poster.go` no se
puede alcanzar: cuando fallan todos los renders, `we.Posters` devuelve el
error antes (ver `bugs.md`).
