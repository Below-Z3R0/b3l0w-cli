# b3l0w

CLI de control del sistema: actualiza **el sistema base** y **los programas
propios** desde un solo comando, y muestra qué versión tiene cada cosa.

```
b3l0w status                 # sistema base (novedades) + versión de cada programa
b3l0w list                   # programas registrados, versión y binario
b3l0w update                 # CLI, sistema, programas y plugins
b3l0w update --system        # solo cachy-update / arch-update
b3l0w update --programs      # solo los programas propios
b3l0w update --plugins       # solo plugins y backends
b3l0w update --dotfiles      # TOML del repo sin pisar cambios locales
b3l0w update --adopt         # registra los TOML actuales como base
b3l0w update --check         # reporta, no toca nada
b3l0w update --self          # actualiza el propio b3l0w
b3l0w check [--keep=N]       # estado + snapshots de Limine
b3l0w channel [stable|edge]  # canal del escritorio
b3l0w plugins ...            # build/release/install/update/list/remove/info/search
b3l0w services ...           # list/hardware/images/sync/install/update/status
b3l0w extensions             # lista las extensiones binarias del CLI
b3l0w install <nombre>       # baja el programa del release a ~/.local/bin
b3l0w doctor                 # dependencias, programas, scripts y plugins
```

## Cómo está armado

Un core chico que **delega**. No sabe de GitHub, de assets ni de pacman:
ejecuta el `update` de cada programa y reporta. Así cada programa sigue
siendo dueño de sus operaciones y no hay dos implementaciones que se
separen en el primer release.

```
b3l0w/
├── cmd/b3l0w/          # dispatcher
├── internal/cli/       # salida, rutas, ejecución de comandos
├── internal/config/    # manifiesto de programas (JSON)
├── internal/programs/  # versión y update de cada programa
├── internal/system/    # adaptador de cachy-update / arch-update
├── internal/update/    # orquestación y código de salida
├── internal/doctor/    # chequeos del entorno
└── internal/plugins/   # extensiones b3l0w-<nombre>
```

Sin dependencias: solo la biblioteca estándar de Go.

## Instalar un programa

```
b3l0w install kitty-shapes
```

Baja el asset del último release del repo que dice el manifiesto y lo
deja en `~/.local/bin`. Valida con `version` antes de pisar el que está.

El manifiesto declara el asset porque cada repo publica el suyo:

```json
{ "name": "kitty-shapes", "source": "github:Below-Z3R0/kitty-shapes", "asset": "kitty-shapes-linux-x64" }
```

`install` deja el **binario**. La configuración (configs de fastfetch,
enganche de fish) la hace el propio programa con su `install`.

## Instalar el sistema

```
b3l0w install system                        # simulación del perfil desktop
b3l0w install system --profile=desktop,dev  # varios perfiles
b3l0w install system --apply                # lo aplica
```

Compara la definición embebida (`internal/systemdef/system.json`) contra la
máquina y lista lo que falta, separado por fuente:

| Fuente | Cómo se instala |
|---|---|
| `repo` | `pacman -S --needed` |
| `flatpak` | `flatpak install flathub` |
| `own` | `b3l0w install <programa>` (release; pacman en la Etapa 2) |
| `services` | `systemctl enable` |
| `groups` | `usermod -aG` |
| `files` | copia de `--dotfiles` con backup |

Perfiles: `desktop` (default), `dev`, `gaming`, `tools`. Los dotfiles viven en
`dotfiles/` y se aplican con `--dotfiles .` desde el repo.

## Actualizarse a sí mismo

```
b3l0w update --self
```

Es el caso especial del contrato: el core no puede delegar su propio
update. Baja su asset, valida la versión y se reemplaza con un rename
atómico. Si el binario pertenece a un paquete (`pacman -Qo`), no lo toca:
eso lo hace pacman.

## El contrato de cada programa

Un programa entra al core cuando cumple esto:

| Comando | Contrato |
|---|---|
| `prog version` | una línea: `prog vX.Y.Z` |
| `prog update` | se actualiza solo, sin preguntar, exit 0/1 |
| `prog doctor` | chequeos, exit != 0 si algo falta |
| `prog install [--dry-run] [--uninstall]` | idempotente, con backup |

Hoy lo cumplen `boot-preview` y `kitty-shapes`.

## El sistema base

`cachy-update` en CachyOS es un **symlink a `arch-update`**, y pertenece a
un paquete: no se renombra ni se reescribe. El CLI lo llama por nombre,
con fallback al upstream:

```
cachy-update -> arch-update
```

Su ejecución principal es **interactiva**: muestra las news, pide
confirmación y después ofrece huérfanos, `pacnew`, kernel pendiente y
servicios a reiniciar. No tiene `--yes`. Por eso:

- `status` / `update --check` usan `-l`, que no pregunta.
- `b3l0w update` le **entrega la terminal** en vez de fingir que lo
  automatiza.
- sin terminal, la mitad del sistema se rechaza con un mensaje claro y
  se sugiere `--programs`.

## Manifiesto

`~/.config/b3l0w/programs.json`:

```json
{
  "version": 1,
  "programs": [
    { "name": "boot-preview", "source": "github:Below-Z3R0/boot-preview" },
    { "name": "kitty-shapes", "source": "github:Below-Z3R0/kitty-shapes" }
  ]
}
```

Si no existe, se usa ese default. Se edita a mano o con
`b3l0w program add|remove`.

## Extensiones del CLI

Los binarios `b3l0w-<nombre>` en `~/.local/lib/b3l0w/plugins/` (0700) son
**extensiones del CLI**, no plugins de Noctalia: `b3l0w-anilist` (el binario)
y `b3l0w/anilist` (el plugin de Noctalia) son cosas distintas. Por eso
tienen su propio comando:

```bash
b3l0w extensions        # lista las extensiones
b3l0w <nombre> <args>   # ejecuta una
```

**No se buscan en el PATH a propósito.** Cualquier cosa que pueda escribir
un `b3l0w-update` en un directorio del PATH lo haría correr con este
usuario, y este CLI pide sudo. `git` acepta ese riesgo; acá no.

## Servicios (Docker)

Cada servicio es un stack de contenedores con su propio repo: el compose,
el Dockerfile y los workflows viven ahí. El repo `b3l0w` es el
**recopilatorio**: `services/services.json` dice qué existe, qué variantes
hay publicadas y cómo se llama la imagen.

El CLI **consume** lo que el CI publica en GHCR. No compila: el build
pesado ocurre una vez, en el runner, no en la máquina del usuario.

```bash
b3l0w services list                  # tabla: estado, variante, imagen
b3l0w services hardware              # CPU, GPU y variante recomendada
b3l0w services images [id]           # tags publicados en el registry
b3l0w services sync                  # actualiza el manifiesto (como pacman -Sy)
b3l0w services install <id>          # pull + up + healthcheck
b3l0w services install <id> --variant=intel
b3l0w services update [id]           # digest nuevo + pull + recreate
b3l0w services status [id]           # contenedores y health
b3l0w services doctor                # docker, compose, GPU, grupos, disco
b3l0w services logs <id> [--tail=N]  # últimas líneas
b3l0w services restart|stop <id>
b3l0w services remove <id> [--purge]
b3l0w services variant <id> <var>    # cambia de variante sin reinstalar
b3l0w services rollback <id>       # vuelve a la imagen anterior
b3l0w services prune               # borra imágenes colgantes
```

### La variante se elige por hardware, no por suposición

`b3l0w services hardware` detecta CPU y GPUs por sysfs (`/sys/class/drm`),
clasifica integrada vs dedicada y recomienda la variante:

| Hardware | Variante | Provider |
|---|---|---|
| NVIDIA dedicada | `cuda` | CUDA |
| AMD dedicada (gfx9+) | `amd` | MIGraphX |
| Intel Arc | `intel` | OpenVINO |
| Intel iGPU | `intel` | OpenVINO (oneDNN) |
| AMD APU | `amd` | MIGraphX si es gfx9+, si no CPU |
| Sin GPU | `cpu` | CPU |

`--variant=` siempre gana sobre la recomendación. Si la variante elegida
no está publicada, el comando lo dice con las que sí.

**La recomendación no es una garantía:** después de levantar el stack, el
CLI lee el provider real del `/status` del servicio y lo muestra. Si pediste
aceleración y el provider real es `CPU`, la aceleración no está funcionando.

### El manifiesto se sincroniza contra el registry

`services.json` declara `available` (lo publicado). `b3l0w services list`
consulta el registry en vivo y avisa si el manifiesto quedó viejo, igual que
una base de pacman desactualizada. `b3l0w services sync` lo corrige.

### El update pide permiso y hace rollback

`b3l0w update` incluye services: compara el digest local con el remoto y,
si cambió, pregunta por cada servicio. Con `--yes` aplica todo. Si el
healthcheck falla después del recreate, vuelve a la imagen anterior y avisa.

**Rollback barato:** como la imagen vieja sigue en el registry, volver
atrás es un `pull` de otro tag. No hace falta recompilar ni guardar
imágenes localmente.

**La PC no se llena:** si el update sale bien, el CLI borra la imagen
anterior (`docker image rm`) y deja solo la nueva. `b3l0w services prune`
limpia las capas colgantes del registry local.

Los servicios son **opcionales**: `install system` no los instala. No hay
servicios instalados, `b3l0w update` no toca nada.

## Plugins de Noctalia

El repo `b3l0w-plugins` es la **fuente única**: una carpeta por plugin y un
`catalog.toml` en la raíz. `main` es producción (lo que clona Noctalia) y
`dev` es integración (tests, docs de desarrollo, trabajo en curso).

```bash
b3l0w plugins build [--check]  # regenera catalog.toml y plugins.json
b3l0w plugins release [--push] # copia producción de dev a main, sin tests/
b3l0w plugins enable <id>      # habilita un plugin en Noctalia
b3l0w plugins disable <id>     # deshabilita un plugin en Noctalia
b3l0w plugins list             # instalado vs registry, con origen
b3l0w plugins install <id>     # agrega/refresca el source; no copia local
b3l0w plugins update [id]      # refresca el source
b3l0w plugins remove <id>      # borra local o deshabilita si viene del source
b3l0w plugins info <id>
b3l0w plugins search <texto>
```

### La compatibilidad de API la resuelve Noctalia, no el CLI

Noctalia soporta un **rango** de API (la doc de v5.4.0 dice "3 through
43") y el catálogo de cada source trae filas `[[plugin.release]]` con la
revisión anterior y su `rev`. Cuando el plugin más nuevo pide más API del
que el shell soporta, Noctalia instala la revisión compatible y la
etiqueta "older version" en el store.

El CLI **no calcula el API ni bloquea el update**: refresca el source y
compara la versión **materializada** contra la del catálogo. Si difieren,
se ve sin abrir la GUI:

```
b3l0w/umbriel-companion   1.2.4   source/enabled (catálogo 1.3.0)
```

Esa diferencia *es* el held-back: Noctalia eligió la revisión que su
rango soporta.

`catalog.toml` y `plugins/plugins.json` se generan: no se editan a mano.
`plugins build` escribe el catálogo en `b3l0w-plugins` y el manifiesto en
`b3l0w`; commiteá los dos. `plugins sync` fue eliminado; `plugins release`
copia solo las carpetas de producción, **regenera el catálogo desde `main`**
(nunca desde `dev`), limpia `tests/`, `docs/`, `fixtures/`, `*.py`,
`*.md` (salvo `LICENSE`), `package.json` y locks, y commitea en `main`.
Un merge normal `dev → main` arrastraría los tests.

Un `.releaseignore` opcional dentro de una carpeta de plugin agrega rutas
relativas a borrar en el release (una por línea).

Los **backends** de los plugins viven embebidos en
`internal/plugins/scripts/` y se ejecutan con `b3l0w plugin-exec <script>`.
No se copian a `/usr/local/bin`; `plugins scripts` ya no existe.

## Extensiones del CLI

Los tres binarios de `internal/plugins/` (`anilist`, `docker`, `we`) son
extensiones del CLI, no plugins de Noctalia. Se compilan a
`b3l0w-<nombre>` en `~/.local/lib/b3l0w/plugins/` y se ejecutan como
`b3l0w <nombre> …`.

### `b3l0w we` — Wallpaper Engine

Catálogo del Workshop de Steam, conversión a video, posters, miniaturas y fondos
de color. Es además el backend del plugin de Noctalia `b3l0w/wallpaper`.

```
b3l0w we list [--json]              catálogo del workshop
b3l0w we info <id> [--json]         un wallpaper con sus propiedades
b3l0w we convert --out DIR          convierte a video (extract o record)
b3l0w we poster --out DIR           PNG por wallpaper (usa el motor)
b3l0w we still --file V --out J     cuadro de un video (miniaturas)
b3l0w we scan --dir R --kind image  imágenes o videos de una carpeta
b3l0w we solid --color '#rrggbb'    wallpaper de un solo color
b3l0w we deps [--json]              qué falta en el sistema
```

Detalle completo, esquemas JSON y contrato con el plugin:
[SPEC-we.md](SPEC-we.md).

Lo que no es obvio:

- `--crf 0` **copia** el archivo en vez de recodificar (md5 idéntico); es el modo
  para los wallpapers `video`, que no necesitan el motor.
- `we scan --thumbs DIR` genera miniaturas de 320 px con cuatro ffmpeg en paralelo
  y las cachea por hash de ruta: 55 fondos en 4 s la primera vez, 12 ms después.
- El nombre de las miniaturas (`v-<djb2(ruta)>.jpg`) es contrato con el plugin de
  Noctalia; cambiarlo acá implica cambiarlo allá.

### `b3l0w docker` — contenedores

```
b3l0w docker snapshot [--json]      estado completo en una pasada
b3l0w docker start|stop|restart ID
b3l0w docker logs ID [--tail N]
b3l0w docker stats [--json]
b3l0w docker inspect ID [--json]
b3l0w docker run --image IMG [--name N] [--network NET] [--port P] [--env K=V]
b3l0w docker remove|rmi|volume-rm|network-rm
```

### `b3l0w anilist` — biblioteca y carátulas

```
b3l0w anilist sync                  biblioteca + carátulas en un paso
b3l0w anilist discover trending|popular|top|new|seasonal
b3l0w anilist library               sólo la biblioteca
b3l0w anilist covers                sólo las carátulas
b3l0w anilist mutate                guarda progreso/estado de una entrada
b3l0w anilist status                token y caché
```

## Roadmap

- **Etapa 2**: repo pacman `[b3l0w]` firmado y PKGBUILDs. Los programas
  pasan a ser paquetes y su `update` detecta el dueño con `pacman -Qo`.
- **Rollback y snapshots** (snapper): pendiente.
- **Checksums de los assets** (`SHA256SUMS`): pendiente.
- **Extensiones**: `b3l0w-containers`, cuando el setup de containers esté
  terminado.

## Compilar

```bash
go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v0.1.0" -o b3l0w ./cmd/b3l0w
go build -o ~/.local/lib/b3l0w/plugins/b3l0w-we ./cmd/b3l0w-we
go build -o ~/.local/lib/b3l0w/plugins/b3l0w-docker ./cmd/b3l0w-docker
go build -o ~/.local/lib/b3l0w/plugins/b3l0w-anilist ./cmd/b3l0w-anilist
go test ./...
bash scripts/smoke-test.sh ./b3l0w
```

Los plugins se construyen **por separado**: `go build -o ARCHIVO ./cmd/x ./cmd/y`
falla porque no puede escribir dos paquetes en un mismo archivo.
