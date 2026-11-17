# `internal/plugins/we` — soporte de Wallpaper Engine

Extensión del CLI `b3l0w` sobre la biblioteca de wallpapers de **Wallpaper Engine** (Steam
Workshop, app `431960`) y sobre carpetas de imágenes locales. Es la capa pesada que consume el
plugin de Noctalia `b3l0w/wallpaper`: el host de plugins da 25 ms de CPU por callback y recorrer
el workshop en Luau medía 26–30 ms, así que el trabajo real vive acá, en Go.

## Qué es un plugin en el CLI y cómo se invoca

Un "plugin" de `b3l0w` en este contexto es una **extensión binaria**: un ejecutable propio,
`b3l0w-<nombre>`, que el core del CLI encuentra en su directorio privado
(`~/.local/lib/b3l0w/plugins/`) y al que le pasa los argumentos tal cual. No es un script de
shell ni una biblioteca Go. Este módulo se expone como la extensión `b3l0w-we`, desde
`cmd/b3l0w-we/main.go`, que llama a `wecli.Run(os.Args[1:])`:

```bash
b3l0w we <comando> [opciones]   # el core resuelve b3l0w-we
b3l0w we help                   # ayuda
```

> `b3l0w plugin-exec` es otra cosa: corre los scripts `.sh` embebidos en `internal/plugins/scripts/`. Ahí **no** está `we` (sólo `igpu-power-menu.sh`), así que `b3l0w plugin-exec we` no funciona.

El código está en dos capas: la **lógica** (`internal/plugins/we`, este dir) expone structs,
funciones y errores y **no imprime nada**; los **comandos** (`internal/plugins/we/cli`) arman
flags, salida y códigos de exit. Por eso `b3l0w we list --json | jq` funciona sin filtrar logs.

## Comandos

Todos aceptan `--help`. Los que emiten JSON lo hacen **una sola vez** y completo.

| Comando | Qué hace | Dónde vive | Qué escribe en disco |
|---|---|---|---|
| `we list` | catálogo del workshop (tabla o JSON) | `catalog.go` + `cli/list.go` | nada |
| `we info <id>` | detalle de un item, con `properties` | `catalog.go` + `cli/list.go` | nada |
| `we convert --out DIR` | saca el video de un wallpaper | `extract.go` + `cli/convert.go` | `<id>-<slug>.<ext>` y `manifest.json` |
| `we poster --out DIR` | PNG del wallpaper con el motor | `poster.go` + `cli/poster.go` | `<id>.png` y `manifest.json` |
| `we still --file V --out J` | un cuadro de un video local | `cli/still.go` (ffmpeg) | la imagen de salida |
| `we scan --dir R` | lista imágenes/videos de una carpeta | `cli/scan.go` | con `--thumbs`: miniaturas JPEG |
| `we solid --color H --out P` | PNG de un solo color | `cli/solid.go` | el PNG |
| `we deps` | qué falta en el sistema | `deps.go` + `cli/deps.go` | nada |

## Topes de seguridad

Dos entradas del usuario se usan **como capacidad de memoria**, así que llevan un techo duro.
Sin él, un valor absurdo revienta la máquina (OOM kill del kernel): el comando no "falla", mata
el proceso.

| Comando | Tope | Cómo se calcula | Por qué |
|---|---|---|---|
| `we solid --size` | `8192x8192` | se rechaza `width*height > 1<<26` | RGBA son 4 bytes por píxel: `100000x100000` son 40 GB de una, y el kernel mata el proceso |
| `we scan --max` | `100000` | se rechaza `max > 100000` | `--max` es la capacidad del slice de resultados: un valor enorme reserva esa memoria de una y mata el proceso |

El tope de `solid` es generoso para un fondo (3840x2160 entra sin problema) y corta el caso
patológico. **No los relajés sin rehacer el cálculo de memoria**: son la defensa contra un input
hostil o un typo.

## El wallpaper sólido (`we solid`)

Genera un PNG de un solo color con la biblioteca estándar (`image/png`), sin ffmpeg ni motor.
Acepta `#rrggbb` y `#rgb` (este último se expande duplicando cada dígito: `#f0a` → `#ff00aa`),
también el valor sin `#`; cualquier otra cosa se rechaza con error. El PNG es opaco (alpha
`255`), sin canal transparente. Tamaño default `3840x2160`, override con `--size ANCHOxALTO`.
Imprime por stdout la ruta del PNG.

**Paletas**: el módulo **no maneja paletas**. No hay concepto de paleta, tema ni colección acá:
el color entra ya resuelto como un hex por `--color` y sale como un PNG de ese color. Las
paletas viven en el plugin de Noctalia (verificar: el plugin está en otro repo, `b3l0w-plugins`,
así que no se puede verificar desde acá qué paleta le pasa).

## Escaneo de una biblioteca de imágenes (`we scan`)

El criterio es recursivo con `--depth` (default 2), salteando directorios ocultos (`.algo`) y
`node_modules`, y también archivos ocultos. Extensiones: imágenes `.jpg .jpeg .png .webp .bmp
.avif .gif .tif .tiff` y videos `.mp4 .webm .mkv .mov .avi .m4v`, elegidas con `--kind
image|video`; `--extensions` imprime la lista en JSON y sale. Con `--json` el resultado es un
array de objetos con `id`, `file`, `type`, `size`, `mod`, `name`, `title` y `thumb` (opcional):
`title` es el nombre del archivo sin extensión con `_` y `-` convertidos en espacios, porque sin
él la tarjeta mostraba la ruta absoluta.

`--thumbs DIR` genera un JPEG por imagen con ffmpeg, en cuatro procesos en paralelo, reusado
mientras el original no cambie de fecha. El nombre es `v-<djb2(ruta)>.jpg`: es el **mismo**
esquema que usa el plugin en Luau (`video_thumb_path`), así que si cambiás la fórmula hay que
cambiarla allá o el plugin deja de encontrar las miniaturas.

`DefaultRoot` es otra cosa y no va con `scan`: devuelve la **primera raíz del workshop de
Steam** que exista en disco, probando en orden `~/.steam/steam/…`, `~/.local/share/Steam/…` y
`~/.steam/root/…`, todas terminadas en el subpath `WorkshopSub`
(`steamapps/workshop/content/431960`). Si ninguna existe devuelve un error que lista las rutas
probadas.

## El catálogo del workshop

`Scan` recorre la raíz y lee el `project.json` de cada carpeta de nombre numérico (los ids del
workshop), normalizándolo a un `Item`. El `type` viene con capitalización inconsistente
(`Video`, `video`): `NormalizeType` lo baja a minúsculas y lo unifica a `video | scene | web |
unknown`. `Filter(items, typ, query)` filtra por tipo y busca en el título o en el id;
`Find(items, id)` busca uno por id. `Slug` convierte el título en un nombre de archivo seguro
(kebab-case, 60 caracteres máximo) y `OutputName` arma `<id>-<slug>.<ext>`. `Properties`
conserva el bloque `general.properties` **tal cual** (sin interpretarlo): es lo que el plugin
usa para configurar cada fondo. **Un item roto no aborta el catálogo**: si falta o no se puede
parsear el `project.json`, vuelve con `Err` cargado y el resto sigue.

## Dependencias externas (`we deps`)

`CheckDeps(ctx, root)` sondea el entorno y devuelve un `Deps` con cinco bloques: `Engine`,
`Ffmpeg`, `Ffprobe`, `Workshop` y `Record`. **Nunca falla por una dependencia ausente**: lo que
falta queda con `Ok=false` y el CLI decide el exit code (`we deps` sale con 2 si falta `ffmpeg`
o el workshop). `MissingDeps()` devuelve las **obligatorias** que faltan: `ffmpeg` y `workshop`
— el motor es opcional, porque los videos se extraen sin render, así que no entra en la lista.
`Summary()` arma una línea legible (`engine:ok ffmpeg:falta …`) para el modo tabla. El sondeo de
versión tiene timeout de 3 s: si un binario se cuelga, `we deps` no se queda esperando. `Record`
decide si se puede grabar video de un wallpaper renderizado: hacen falta `grim` (capturar
frames) y el motor, y se reporta el primer faltante.

`Locate` (ffmpeg) y `LocateProbe` (ffprobe) resuelven el binario en este orden: el `PATH`;
después `~/.hermes/tools/ffmpeg-*/bin/`, en orden inverso para que gane la versión más nueva
(ffprobe se busca ahí mismo, como hermano del ffmpeg, porque el tarball los trae juntos); y por
último `B3L0W_FFMPEG_DIR`, que manda **sólo** ese directorio. Si no aparece, el error lista las
rutas probadas.

## Extracción de video (`we convert`)

`Extract(ctx, item, opts)` saca el video de un wallpaper de tipo `video` sin tocar el motor.
**Camino rápido**: si el origen ya es `.mp4`, el formato pedido es `mp4` y el `CRF` es 0, se
copia byte a byte (0 GPU, ~1 s) — es la regla `copyable`. **Camino lento**: si no, se recodifica
con ffmpeg (`libx264` + `yuv420p` para mp4, `libvpx-vp9` para webm, siempre `-an` porque un
wallpaper no usa el audio, y `-preset veryfast`). **Nunca sobreescribe** sin `Overwrite`: si el
destino existe devuelve `Result{Skipped: true}` con el tamaño del existente. Escribe a un
temporal en el mismo directorio y recién ahí renombra, así el destino nunca queda a medias. Sólo
funciona para `type == "video"`; para `scene`/`web` el error sugiere el método `record`. `Probe`
(ffprobe) devuelve duración, ancho, alto y codec del primer stream de video, y `Run` ejecuta un
binario devolviendo, si falla, un error con las últimas 20 líneas de su stderr.

> **Gap conocido**: `--method` se parsea y se valida, pero `convert` siempre llama a `Extract`; el valor nunca se lee. El método `record` que anuncia el `--help` no está implementado en esta capa. `we deps` sí reporta si `record` sería viable (`grim` + motor).

## El manifest

`manifest.json` es el índice de lo generado: un `Entry` por wallpaper con `id`, `title`, `type`,
`status` (`ok | skipped | failed`), `file`, `source`, `method`, `bytes`, `duration`, `width`,
`height` y `error`. `SaveManifest` escribe de forma **atómica** (serializa a un temporal, lo
sincroniza y recién ahí lo renombra), así que un corte a mitad de escritura deja el manifest
anterior intacto. `LoadManifest` envuelve `os.ErrNotExist` si no está, para que el llamador
decida si arranca uno nuevo; un manifest vacío o truncado es error y nunca se ignora en
silencio. `Upsert` reemplaza el `Entry` con el mismo id o lo agrega al final. Los tags JSON del
manifest son **el contrato con el plugin de Noctalia**: no se tocan sin coordinar con el plugin.

## Decisiones técnicas (y por qué)

1. **El paquete no imprime** — la salida la arma `cli`, para que stdout sea parseable por `jq`.
2. **El catálogo es tolerante a fallos** — un item roto vuelve con `Err` en vez de abortar el
   scan.
3. **Ningún proceso del motor queda vivo** — el engine no termina tras capturar, así que el
   paquete lo mata y lo cosecha; sin ese kill un wallpaper que no renderiza cuelga el lote.
4. **El PNG se valida antes de aceptarlo** — `pngComplete` exige firma PNG y chunk `IEND`.
5. **ffmpeg nunca se asume en el PATH** — `Locate`/`LocateProbe` miran las copias portables.
6. **Todo comando externo tiene timeout** — 2 min por render, 3 s por sondeo de versión.
7. **Sólo biblioteca estándar** — cero dependencias externas de Go.

## Archivos del módulo

Las primeras siete filas son el paquete `we` (lógica); las de `cli/` son el subpaquete `wecli`
(comandos). Fuera del módulo: `cmd/b3l0w-we/main.go` (wiring) y `SPEC-we.md` (contrato con el
plugin, en la raíz del repo).

| Archivo | Qué hace |
|---|---|
| `doc.go` | documentación del paquete y sus invariantes |
| `catalog.go` | `DefaultRoot`, `NormalizeType`, `LoadProject`, `Scan`, `Filter`, `Find`, `Slug`, `OutputName` |
| `deps.go` | `Deps`, `CheckDeps`, `MissingDeps`, `Summary`, `ToolOK`, `WorkshopOK`, `RecordOK` |
| `extract.go` | `Extract`, `ExtractOptions`, `Result`: copia o recodifica el video |
| `ffmpeg.go` | `Locate`, `LocateProbe`, `Run`, `Probe`: localización y ejecución de ffmpeg |
| `manifest.go` | `Manifest`, `Entry`, `LoadManifest`, `SaveManifest`, `Upsert` |
| `poster.go` | `Poster`, `Posters`: PNG por wallpaper vía `--screenshot` del motor |
| `cli/root.go` | dispatch, help y códigos de salida |
| `cli/list.go` | `we list` y `we info` |
| `cli/convert.go` | `we convert`: extrae y actualiza el manifest |
| `cli/poster.go` | `we poster`: renderiza PNGs y actualiza el manifest |
| `cli/still.go` | `we still`: un cuadro de un video |
| `cli/scan.go` | `we scan`: imágenes/videos de una carpeta + tope de `--max` |
| `cli/solid.go` | `we solid`: PNG de un color + tope de `--size` |
| `cli/thumbs.go` | `we scan --thumbs`: miniaturas JPEG en paralelo |
| `cli/deps.go` | `we deps`: tabla o JSON del estado del entorno |
