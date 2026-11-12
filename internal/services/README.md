# internal/services — los servicios Docker de la distro

> Un **servicio** es un stack de contenedores que la distro publica como
> imagen y administra con compose. El CLI no compila nada: consume las
> imágenes que el CI publica en GHCR.

## 1. Qué es un servicio

| Pieza | Dónde vive | Qué es |
|---|---|---|
| **Manifiesto** | `services/services.json` (repo de distro) | Definición: id, imagen, variantes, puertos, health, compose |
| **Imágenes** | GHCR (`ghcr.io/below-z3r0/<id>`) | Un tag por variante: `cpu-latest`, `cuda-latest`, `intel-latest`, `amd-latest` |
| **Repo del servicio** | `~/Documents/Github/<repo>` | El `compose.yaml` (y el override por variante) que el CLI levanta |

El manifiesto **no** dice qué está publicado**: eso lo dice el registry.
`Available` es una caché; `b3l0w services sync` la recalcula.

Verbos: `list` (default), `search <txt>`, `hardware`, `images [id]`,
`sync`, `install <id> [--variant=]`, `update`, `status`, `doctor`,
`logs <id> [--tail=N]`, `restart|stop|remove <id> [--purge]`,
`variant <id> <variante>`, `rollback <id>`, `prune`. Aliases: `-S`=install,
`-R`=remove, `-Q`=list, `-U`/`-Syu`=update, `-Ss`=search.

## 2. El manifiesto

`manifest.go` lo maneja, con la jerarquía de `internal/registry`:
**caché fresca (1 h) → red → caché vieja → embebido**. El embebido
(`//go:embed services.json`) es el último recurso, para que una
instalación limpia sin red igual tenga catálogo. El remoto es
`raw.githubusercontent.com/Below-Z3R0/b3l0w/main/services/services.json`
y la caché vive en `~/.local/state/b3l0w/registry/services.json`.

`parseManifest` **rechaza** el manifiesto si no tiene versión o no tiene
servicios, y valida cada id con `system.SafeName`: el id es el nombre
del archivo de estado (`<id>.json`), así que un `../` haría que el CLI
escribiera fuera del directorio de estado (fix `1f7fadc`).
`SaveManifest` ordena por id: el diff del repo de distro no es ruido.

Campos que importan: `image` (repo en GHCR; sin él el servicio no se
puede instalar), `variants` (las que **define**) vs `available` (las que
**están publicadas**, caché), `ep` (execution provider, para el
reporte), `ports`, `health` (URL que responde cuando está listo),
`overrides` (compose extra por variante) y `models` (comando que instala
los modelos; el CLI solo lo muestra).

## 3. El estado local

`~/.local/state/b3l0w/services/<id>.json`. Un archivo por servicio
instalado: qué variante corre, con qué digest, desde cuándo, y **qué
imagen anterior hay** para poder volver atrás.

```json
{"id": "argos", "variant": "cuda", "image": "ghcr.io/below-z3r0/argos:cuda-latest",
 "digest": "ghcr.io/below-z3r0/argos@sha256:...", "tag": "cuda-latest",
 "installed_at": "2026-11-10T19:18:59-03:00", "updated_at": "2026-11-10T19:18:59-03:00",
 "health": "healthy", "provider": "CUDA", "ports": [6333, 7711],
 "previous_image": "ghcr.io/below-z3r0/argos:cpu-latest", "previous_tag": "cpu-latest"}
```

`previous_*` es lo que hace posible el rollback: volver atrás es un pull
de otro tag, no una recompilación ni una copia local de la imagen vieja.
`ListStates` saltea lo que no es estado (subdirectorios, `.json` roto) y
ordena por id. El directorio se puede mover con `--state-dir`.

## 4. El ciclo de update

El corazón del paquete: `install.go` → `Installer.Update`.

```
  digest local (RepoDigest)  ─┐
  digest remoto (header)     ─┴─→ NormalizeDigest ─→ ¿iguales?
                                       (sha256:...)     │ sí → "al día", no se toca nada
                                                      │ no → pull + compose up -d (IMAGE=<tag>)
                                                      │        → GET al health (hasta 120 s)
                                                      │        → si no responde: pull + up con
                                                      │          el tag anterior (rollback automático)
```

**La normalización del digest** es el bug clásico y el que más cuesta ver.
Local: `docker image inspect` devuelve el **RepoDigest**
(`ghcr.io/.../argos@sha256:abc...`); remoto: el header
`Docker-Content-Digest` devuelve **solo** `sha256:abc...`. Sin
normalizar los dos nunca son iguales y el update rehace el stack en cada
corrida. `NormalizeDigest` (en `rollback.go`) deja solo el `sha256:...`.

**El rollback automático**: si el health no responde después del
recreate, `Update` vuelve solo a la imagen anterior y el error que se ve
es `rollback a <tag>: ...`, así el usuario sabe que quedó atrás. El
`rollback` manual es el mismo recorrido pero explícito: usa
`previous_*`, deja la nueva como `previous_*` (para poder volver
adelante) y borra la imagen que quedó atrás.

**El env, no un flag**: compose no tiene `--env` (tiene `--env-file`).
`${IMAGE}` se resuelve desde el entorno del proceso, así que `runEnv` lo
inyecta siempre. Si se pierde, compose levanta su default
(`${IMAGE:-intel-latest}`) en vez de la variante pedida, y el estado dice
una cosa mientras corre otra (fix `a346a61`).

## 5. Variantes por hardware

`hardware.go` detecta el hardware **sin binarios externos**:
`/proc/cpuinfo` → `vendor_id` y `flags` (avx2, avx512f, solo
informativo); `/sys/class/drm/cardN/device/vendor` → `0x10de` NVIDIA,
`0x1002` AMD, `0x8086` Intel; `.../uevent` → `DRIVER=i915|xe|nvidia|
amdgpu`; `.../mem_info_vram_total` → VRAM (AMD/Intel). `cardN` y
`cardN-DP` son el mismo device: solo cuenta el primero.

Dedicada: NVIDIA siempre; AMD si VRAM > 1 GiB; Intel si driver `xe` **y**
VRAM > 1 GiB (Arc). Ranking: NVIDIA ded. 100 > AMD ded. 90 > Intel ded.
80 > Intel iGPU 70 > AMD iGPU 60 > sin GPU 0.

| Hardware | Variante | Provider | Motivo |
|---|---|---|---|
| NVIDIA dedicada | `cuda` | CUDA | GPU dedicada NVIDIA |
| AMD dedicada | `amd` | MIGraphX | GPU dedicada AMD (gfx9+) |
| Intel Arc | `intel` | OpenVINO | Intel Arc dedicada |
| Intel iGPU | `intel` | OpenVINO | OpenVINO CPU (oneDNN) y GPU por device_type |
| AMD APU | `amd` | MIGraphX | si es gfx9+, si no cae a CPU |
| Sin GPU | `cpu` | CPU | ONNX Runtime siempre trae CPU |

**La CPU siempre está soportada**: es el piso. Una variante acelerada
que el host no soporta nunca se ofrece.

`Recommend` elige: (1) la acelerada que el host soporta **y** está
publicada; (2) si no está publicada, la que el host soporta, avisando
`"(no publicada: falta el build)"`; (3) si no hay GPU, la primera
publicada. Regla dura: **la variante recomendada nunca es una que no
exista en el registry** — recomendar una imagen inexistente rompe la
instalación.

## 6. Por qué los servicios NO se actualizan solos

Los `apps` se actualizan con `pacman` porque son archivos en disco: los
reemplaza el gestor y listo. Un servicio es un **stack corriendo**, con
volúmenes, puertos y datos. Actualizarlo sin que el usuario lo pida
significa cortar un servicio en producción sin avisar, migrar un volumen
a un esquema que la imagen anterior no entiende, y consumir 15-25 GiB de
disco y red sin que nadie lo pidiera.

Por eso el update de servicios **verifica y pregunta**: `b3l0w services
update` compara el digest de cada servicio instalado y, si hay uno
nuevo, pregunta (`Actualizar <id>? [Y/n]`). Sin TTY,
`confirm.Confirm` devuelve false — abortar es más seguro que actualizar
a ciegas. `b3l0w update` (el global) incluye servicios, pero también
pregunta. La excepción es `--yes`: ahí el usuario ya dijo que sí. El
verbo canónico sigue siendo `update`, alias `-U`/`-Syu`; no hay un
`-Syu` que actualice el sistema, eso es de `apps`.

(Verificar: el plan de la distro más allá del código es que los
servicios se actualicen por digest y nunca por tag mutable. El código
usa digest para decidir y tags inmutables para tirar.)

## 7. Decisiones y por qué

| Decisión | Por qué |
|---|---|
| **Todo por compose v2** | El v1 no soporta `--project-directory` ni perfiles. `HaveComposeV2` lo verifica |
| **El CLI no compila** | Las imágenes las publica el CI. Si el CLI compilara, la distro tendría dos fuentes de verdad |
| **Env `IMAGE` en vez de flag** | Compose no tiene `--env`; `${IMAGE}` sale del entorno del proceso |
| **Variante por hardware, no por config** | La imagen que sirve depende de la GPU. La detección es cero-config; un flag obligatorio en cada install es fricción |
| **La CPU siempre disponible** | ONNX Runtime trae CPU en todas partes: garantiza que el servicio instale |
| **Digest para decidir, no tag** | Un tag se puede republicar; el digest no. Comparar tags da falsos positivos |
| **Rollback automático** | Un update que rompe el servicio y deja al usuario sin él es peor que un update que no pasó |
| **Se borra la imagen que quedó atrás** | Si no, cada update deja una capa de GiB. `RemoveImage` no borra lo que un container está usando |
| **Los modelos no se instalan** | Son GiB de descarga y los baja el CTL del servicio (`argosctl models install`) |
| **`SafeName` en los ids** | El id es un nombre de archivo: un `../` escribiría fuera del directorio de estado |
| **El embebido es el último recurso** | Una instalación limpia, sin repo y sin red, igual tiene catálogo |
| **Sin `Runner` inyectable en el adapter** | Se eliminó: causaba que el env se perdiera (`a346a61`). Los tests usan un stub de `docker` en el PATH |

El manifiesto **no** decide qué está publicado (eso lo dice el registry,
`Available` es caché) ni qué variante usar (eso lo decide el hardware y
el usuario).

## 8. Archivos, inyección y tests

`manifest.go` (manifiesto y estado local) · `services.json` (embebido) ·
`docker.go` (adapter compose v2, con env para el `IMAGE`) ·
`registry.go` (tags y digest de GHCR) · `install.go` (`Installer`) ·
`rollback.go` (digest, rollback, borrado de imágenes) · `hardware.go`
(detección, ranking, recomendación) · `doctor.go` (chequeos del host) ·
`report.go` (`CheckRegistry`, `Sync`, render) · `search.go` (filtro).

Para testear: Docker con un stub de `docker` en el PATH (nunca se toca
el Docker real), registry con `httptest.NewTLSServer` y el host en la
imagen, hardware con `detectHardwareAt(cpuinfo, drmRoot)` y dirs
temporales, estado con `Installer.StateDir` y `Installer.RepoDir`.

`go test ./internal/services/`

`rollback_test.go` (normalización del digest, rollback completo, fallos
de pull y de up, `ImageInUse`) · `hardware_paths_test.go` (detección con
`/proc` y `/sys` falsos, matriz de variantes, ranking, `Recommend`) ·
`install_test.go` (update por digest, resolución de variante, elección de
tag) · `manifest_test.go` (parseo, validaciones, round-trip del
manifiesto y del estado, `ListStates`) · más los previos
`hardware_test.go`, `search_test.go` y `docker_env_test.go`.

**Nunca se toca Docker real.** El adapter llama a `docker` por el PATH,
así que los tests ponen un stub delante: registra cada llamada y puede
fallar a propósito, para probar los caminos de error.
