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
b3l0w services ...           # list/search/hardware/images/sync/install/update/status
b3l0w extensions             # lista las extensiones binarias del CLI
b3l0w sync [cat] [--check]   # refresca las DB del registry
b3l0w apps [-S|-Syu|-R|-Q]   # wrapper de pacman con freno de partial upgrade
b3l0w install <nombre>       # baja y verifica el programa del release
b3l0w doctor                 # dependencias, programas, scripts y plugins
```

### Verificación de assets

Cada release debe publicar `SHA256SUMS` y `SHA256SUMS.asc`. El
instalador baja los tres archivos del **mismo** release, verifica la
firma con la clave embebida y recién después compara el hash:

```
1. gpg --verify SHA256SUMS.asc SHA256SUMS   (clave embebida, keyring temporal)
2. sha256(asset) == línea de SHA256SUMS
3. asset version                             (que el binario responda)
4. recién ahí se mueve a ~/.local/bin
```

Si el release no está firmado, **no se instala**. Para saltearlo en un
release viejo, de forma explícita:

```bash
b3l0w install <nombre> --insecure
```

El self-update (`b3l0w update --self`) no tiene `--insecure`: un
binario sin firma no reemplaza al que está corriendo.

`b3l0w update --self` busca el release más nuevo con tag `v*`, no
`/releases/latest`: el repo también publica los paquetes del
escritorio, y ese release queda como "latest" sin tener el binario.

### Publicar un release

Firmá **antes** de subir. Un release sin `SHA256SUMS.asc` está roto para
el usuario final: `b3l0w install` y `b3l0w update --self` se niegan a
instalar (fail-closed).

```bash
export GNUPGHOME="$HOME/.gnupg-b3l0w"
bash scripts/maintainers/release.sh Below-Z3R0/<repo> <tag> <asset>...
```

El script calcula `SHA256SUMS`, lo firma, verifica la firma y recién
entonces sube los tres archivos juntos. Si la firma falla, no se publica
nada: el release nunca existe sin firma.

Para firmar un release ya publicado (un forgot):

```bash
bash scripts/maintainers/sign-release.sh Below-Z3R0/<repo> <tag>
```

### Gate de release

Ningún release se da por terminado sin firma. Antes de anunciarlo:

```bash
b3l0w doctor          # debe decir "firmado", no "SIN FIRMA"
b3l0w update --check  # ídem
```

La clave privada nunca va a CI: la firma se hace en local y se sube el
resultado. Fingerprint `9AE2C00AD3A9A64706A4335CC6E1D74A07833042`.

### Atajos estilo pacman

Los verbos son la forma canónica. Las banderas cortas existen para que
las categorías se sientan parecidas:

| Bandera | Verbo | services | plugins |
|---|---|---|---|
| `-S` | `install` | sí | sí |
| `-R` / `-Rns` | `remove` | sí | sí |
| `-Q` / `-Qe` | `list` | sí | sí |
| `-U` / `-Syu` | `update` | sí | sí |
| `-Ss` / `-Qs` | `search` | sí | sí |

```bash
b3l0w services -Q          # mismo que b3l0w services list
b3l0w services -S argos    # mismo que b3l0w services install argos
b3l0w plugins -Ss anilist  # mismo que b3l0w plugins search anilist
```

**No se reimplementa pacman.** `services -Syu` verifica y pregunta si
actualizar; los servicios son Docker, no paquetes. Y `-Ss` solo existe
donde hay catálogo que consultar.

## Registry

Las DB de la distro (`system.json`, `services/services.json`,
`plugins/plugins.json`) se cargan con una sola jerarquía:

```
caché fresco (1 h) -> red -> caché viejo -> embebido
```

La red es `raw.githubusercontent.com/Below-Z3R0/b3l0w/main`. No hay
Pages ni workflow: los archivos del repo son la fuente.

```
~/.local/state/b3l0w/registry/<categoría>.json
```

| Comando | Qué hace |
|---|---|
| `b3l0w sync` | baja las tres DB |
| `b3l0w sync services` | solo una |
| `b3l0w sync --check` | estado local, sin red |

`b3l0w update` sincroniza antes de comparar. Sin eso se actualiza
contra un catálogo viejo: es el `-Sy` que le faltaba.

**El embebido es el último recurso**, no la fuente normal: en una
instalación limpia, sin el repo clonado y sin red, el CLI igual tiene
catálogo.

## Apps (pacman)

`b3l0w apps` no reimplementa pacman: traduce banderas y agrega una
regla que pacman no tiene.

| Comando | Qué hace |
|---|---|
| `apps` | `pacman -Q` (lista instalados) |
| `apps -Syu` | sync + upgrade — **el correcto para actualizar** |
| `apps -S <pkg>` | instala, **solo si la DB tiene menos de 7 días** |
| `apps -Sy` / `-Syy` | **rechazado**: partial upgrade |
| `apps -R <pkg>` | se expande a `pacman -Rns` |
| `apps -Ss` / `-Si` / `-Q` | consultas, sin sudo |
| `--allow-stale` | fuerza `-S` con la DB vieja |

`pacman -Sy <pkg>` mezcla paquetes nuevos con librerías viejas: es la
forma clásica de romper Arch. El CLI lo niega y ofrece `-Syu`.

## Empaquetado

```
packaging/
  b3l0w-keyring/     clave del repo [b3l0w] + pacman-key --populate
  b3l0w-cli/         compila Go
  boot-preview/      compila Go
  kitty-shapes/      instala el binario de Bun
```

```bash
# construir uno
cd packaging/b3l0w-cli
rsync -a --exclude .git --exclude packaging ../../ src/
makepkg -f

# publicar en b3l0w-packages (pide la passphrase)
bash scripts/maintainers/repo-publish.sh --push packaging/*/*.pkg.tar.zst
```

El script reemplaza los symlinks `b3l0w.db` / `b3l0w.files` por copias
reales: `raw.githubusercontent` no sigue symlinks y pacman recibiría el
texto del enlace.

## Firmas

Los paquetes se publican en
[`b3l0w-packages`](https://github.com/Below-Z3R0/b3l0w-packages) y se
firman con la clave de release de b3l0w.

```
Fingerprint: 9AE2 C00A D3A9 A647 06A4  335C C6E1 D74A 0783 3042
Vence:       2031-11-05
Clave pública: keys/b3l0w.asc
```

**La clave privada nunca está en este repo ni en CI.** Vive en
`~/.gnupg-b3l0w` en el equipo del mantenedor; el backup va cifrado a un
medio offline. El `.gitignore` de `keys/` bloquea `*.gpg`, `*-secret*` y
`*-revoke*` para que un `git add -A` no los suba.

### Generar la clave (una sola vez)

```bash
bash scripts/maintainers/gen-signing-key.sh
```

Crea un `GNUPGHOME` dedicado, una clave RSA 4096 solo de firma con
vencimiento a 5 años, el certificado de revocación y un backup cifrado.

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
