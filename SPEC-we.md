# SPEC — `b3l0w we` (soporte de Wallpaper Engine)

Extensión del CLI `b3l0w` para operar sobre Wallpaper Engine desde terminal y para
servir de backend al plugin de Noctalia `b3l0w/wallpaper`.

- Repo: `~/Documents/Github/b3l0w`
- Binario del plugin: `~/.local/lib/b3l0w/plugins/b3l0w-we`
- Invocación: `b3l0w we <comando>` (el CLI lo encuentra en su directorio de plugins,
  nunca en el PATH)
- Código: `internal/plugins/we` (lógica) + `internal/plugins/we/cli` (comandos)

## Por qué existe

Dos motivos concretos, medidos:

1. **El host de plugins de Noctalia da 25 ms de CPU por callback.** Recorrer el
   workshop y parsear 40 `project.json` en Luau medía 26–30 ms: el `set_source` del
   plugin moría a mitad de camino y la fuente no cambiaba. En Go son 2 ms.
2. **Hay trabajo que no se puede hacer en Luau**: decodificar frames con ffmpeg,
   medir el catálogo, generar miniaturas, escribir PNG. Eso vive acá.

## Contexto verificado de la máquina

No re-investigar; está comprobado en este equipo:

| Dato | Valor |
|---|---|
| Motor | `/usr/sbin/linux-wallpaperengine` (C++/Qt, layer-shell + X11) |
| Catálogo | `~/.steam/steam/steamapps/workshop/content/431960/<id>/` |
| Items actuales | 40 (`video` 33, `scene` 5, `web` 2 — cambia con el Workshop) |
| ffmpeg | `~/.hermes/tools/ffmpeg-9.0.1-linux-x64/bin/ffmpeg` (no está en PATH) |
| `project.json` | `title`, `type`, `file`, `preview`, `general.properties`, `contentrating`, `tags` |
| `type` | capitalización inconsistente: `video`/`Video`, `scene`/`Scene` → se normaliza |
| `--screenshot` | renderiza offscreen y escribe PNG; lento (decenas de segundos) |
| `video` | el `file` es mp4/webm real: se copia sin motor (0 GPU, ~1 s) |

## Invariantes

1. **stdout es sólo datos.** Tabla, rutas o un único documento JSON. Logs y errores
   van a stderr, sin excepción.
2. **Cero dependencias externas** de Go: sólo la biblioteca estándar.
3. **ffmpeg nunca se asume en PATH**: se resuelve con `Locate`/`LocateProbe`, que
   también miran las rutas de herramientas de Hermes.
4. **Un item roto no aborta el catálogo**: se devuelve con `error` cargado.
5. **Ningún proceso del motor queda vivo** al terminar: cada corrida cierra el suyo.
6. **Sin `panic` ni `os.Exit`** fuera de `main`/`cli`.

## Layout

```
cmd/b3l0w-we/main.go            wiring: llama a cli.Run
internal/plugins/we/
    doc.go                      documentación del paquete
    catalog.go                  Scan, Filter, Find, LoadProject, NormalizeType
    extract.go                  remux / recodificación con ffmpeg
    poster.go                   --screenshot del motor (Poster, Posters)
    ffmpeg.go                   Locate, LocateProbe, Run, Probe
    manifest.go                 manifest.json (id → archivo generado)
    deps.go                     qué falta en el sistema
    cli/root.go                 dispatch + help + códigos de salida
    cli/{list,convert,poster,still,scan,solid,thumbs,deps}.go
```

## Comandos

Todos aceptan `--help`. Los que emiten JSON lo hacen una sola vez y completo.

### `we list [--json] [--root DIR] [--type video|scene|web] [--query TXT]`

Catálogo del workshop. Sin `--json` imprime `ID  TYPE  SIZE  TITLE`.
`--query` busca en el título sin distinguir mayúsculas; `--type` filtra por tipo.

```bash
b3l0w we list --type video
b3l0w we list --json | jq '.[] | select(.properties != null) | .id'
```

### `we info <id> [--json]`

Un item del catálogo, con `properties` incluidas. Exit 1 si el id no existe.

### `we convert --out DIR [--id ID]... [--all] [flags]`

Convierte wallpapers a video.

| Flag | Default | Qué hace |
|---|---|---|
| `--out DIR` | — (requerido) | carpeta de salida; se crea |
| `--id ID` | — | repetible; sin `--all` ni `--id` no hace nada |
| `--all` | false | todos los del catálogo |
| `--method` | `auto` | `auto` \| `extract` \| `record` |
| `--format` | `mp4` | `mp4` \| `webm` |
| `--crf` | 18 | `0` = copiar sin recodificar |
| `--overwrite` | false | pisa lo existente |
| `--jobs` | NumCPU/2 | conversiones en paralelo |
| `--manifest` | `<out>/manifest.json` | dónde dejar el manifiesto |
| `--json` | false | resultado en JSON |

Comportamiento:

- `file` es mp4 y `--format mp4` y `--crf 0` → **copia** byte a byte (md5 idéntico,
  verificado). Si no, `ffmpeg -c:v libx264 -crf N -preset veryfast -pix_fmt yuv420p -an`.
- Nombre de salida: `<id>-<slug(title)>.<ext>`, slug kebab-case, máximo 60 chars.
- Nunca sobrescribe sin `--overwrite`: si existe, el item queda `skipped`.
- `record` es experimental (captura con `grim`); si falla, el item queda `failed` con
  el motivo y el resto continúa.

### `we poster --out DIR [--id ID]... [--all] [--size 1280x720] [--jobs N] [--overwrite] [--manifest RUTA]`

PNG por wallpaper usando `--screenshot` del motor. Escribe `<id>.png`, actualiza el
manifiesto y es idempotente. Sólo tiene sentido para `scene`/`web`.

### `we still --file VIDEO --out IMAGEN [--size 320x180] [--at 1.0]`

Extrae un cuadro de un video. Es lo que el plugin usa para la miniatura de los videos
locales. Imprime la ruta generada.

### `we scan [--dir RUTA]... [--kind image|video] [--max 400] [--depth 2] [--sort name|date|random] [--thumbs DIR [--thumb-size 320]] [--json] [--extensions]`

Lista imágenes o videos de una carpeta, recursivo, sin salir de `--depth`.

- `--sort name` (alfabético) | `date` (mtime descendente) | `random` (mezcla en cada corrida).
- `--thumbs DIR`: genera una miniatura JPEG por imagen en `DIR` y la devuelve en el
  campo `thumb`. Cuatro ffmpeg en paralelo; se reusa mientras el original no cambie.
- `--extensions`: imprime en JSON las extensiones soportadas y sale.

Salida (`--json`), un objeto por archivo:

```json
{
  "id": "/home/u/Pictures/Wallpapers/Abstract/contents/screenshot.png",
  "file": "/home/u/Pictures/Wallpapers/Abstract/contents/screenshot.png",
  "type": "image",
  "size": 177268,
  "mod": 1786473324,
  "name": "screenshot",
  "title": "screenshot",
  "thumb": "/home/u/.local/state/noctalia/plugins/data/b3l0w/wallpaper/thumbs/v-96ca8377.jpg"
}
```

`title` y `thumb` existen porque sin ellos la tarjeta del plugin mostraba la ruta
absoluta como título y caía al placeholder de "sin miniatura".

### `we solid --color '#rrggbb' --out archivo.png [--size 3840x2160]`

Genera un PNG de un solo color. Noctalia no tiene un primitivo de "fondo de color":
el plugin apunta `setWallpaper` a este archivo. Acepta `#rgb` y `#rrggbb`. 3840x2160
pesa ~180 KB. Imprime la ruta.

### `we deps [--json] [--root DIR]`

Qué falta en el sistema, con ruta y versión:

```json
{
  "engine":  {"path": "/usr/sbin/linux-wallpaperengine", "ok": true},
  "ffmpeg":  {"path": "...", "ok": true, "version": "ffmpeg version n9.0.1..."},
  "ffprobe": {"path": "...", "ok": true, "version": "..."},
  "workshop":{"path": "...", "items": 40, "ok": true},
  "record":  {"supported": true, "reason": ""}
}
```

Exit 0 si `ffmpeg` y `workshop` están OK; el motor es opcional.

## Miniaturas: contrato de nombres

`v-<djb2(ruta)>.jpg`, con djb2 de 32 bits sobre los bytes de la ruta y hex sin
padding. Es el **mismo** esquema que usa el plugin en Luau (`video_thumb_path`), así
que `apply_cached_thumbs` reconoce las miniaturas generadas por el CLI sin cambios.
Si se cambia acá, hay que cambiarlo allá o el plugin deja de encontrarlas.

Regla de caché: se regenera sólo si el original es más nuevo o la miniatura está vacía.
Ancho por defecto 320 px, alto proporcional (`scale=320:-2`).

## Integración con el plugin `b3l0w/wallpaper`

| Fuente del plugin | Comando que usa |
|---|---|
| Wallpaper Engine | `we list --json` (catálogo) y `we still` (miniaturas de preview) |
| Imágenes locales | `we scan --json --thumbs DIR` |
| Videos locales | `we scan --kind video --json` + `we still` |
| Wallhaven | no usa el CLI (HTTP directo desde Luau) |
| Color sólido | `we solid --color … --out …` |
| Aplicar un item fuera de catálogo | nada: el plugin lee `project.json` |

Motivo de que sea un proceso y no una biblioteca: el plugin corre en Luau dentro del
shell; el CLI es un binario aparte que se invoca con `runAsync`. El host mata la
acción si un callback pasa de 25 ms de CPU, y el trabajo pesado no puede vivir ahí.

## Errores y diagnóstico

| Síntoma | Causa | Qué hacer |
|---|---|---|
| `no se encontró el workshop` | Steam no instalado o biblioteca en otra ruta | pasar `--root`, o revisar `we deps` |
| `ffmpeg no encontrado` | no está en PATH ni en las rutas de Hermes | instalar ffmpeg; `we deps` dice qué falta |
| conversión `skipped` | el archivo ya existe | `--overwrite` si se quiere pisar |
| item con `error` en `list --json` | `project.json` ilegible | el resto del catálogo sigue válido |
| miniaturas ausentes | ffmpeg falló en ese formato (avif, gif animado) | el item queda sin `thumb`; la tarjeta muestra el placeholder |
| `we` no aparece | el binario no está en `~/.local/lib/b3l0w/plugins/` | `go build -o` ahí; `b3l0w doctor` lo reporta |

## Calidad

```bash
gofmt -l .                       # vacío
go vet ./...                     # limpio
go test ./...                    # verde
go build -o ~/.local/bin/b3l0w ./cmd/b3l0w
go build -o ~/.local/lib/b3l0w/plugins/b3l0w-we ./cmd/b3l0w-we
```

Reglas de código: cada función exportada con doc comment; errores con
`fmt.Errorf("...: %w", err)`; tests con `t.TempDir()` y fixtures escritos en el test,
nunca red.
