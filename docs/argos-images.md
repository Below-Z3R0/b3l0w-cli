# Argos: elección de imagen por hardware (spec para `b3l0w`)

Documento de arranque para la sesión que integre Argos en el CLI del sistema. El lado de las imágenes
y sus recetas de build está en `~/Documents/Github/argos/docs/IMAGES.md`.

## Qué tiene que hacer el CLI

1. Detectar el hardware del host (CPU + todas las GPUs).
2. Recomendar una de las tres imágenes (`argos:nvidia`, `argos:intel`, `argos:amd`).
3. Dejar que el usuario elija otra.
4. Persistir la elección y levantar el stack.
5. Verificar que arrancó y reportar el provider en uso.

## Detección

### CPU

- `/proc/cpuinfo` → `vendor_id`: `GenuineIntel` | `AuthenticAMD`.
- `flags` → `avx2`, `avx512f` (solo para mostrar; no cambia la imagen).

### GPUs

Fuente primaria, sin binarios externos:

- `/sys/class/drm/card*/device/vendor`: `0x10de` NVIDIA, `0x1002` AMD, `0x8086` Intel.
- `/sys/class/drm/card*/device/uevent` → `DRIVER=i915|xe|nvidia|amdgpu`.
- `/sys/class/drm/card*/device/mem_info_vram_total` (AMD/Intel) para distinguir dedicada de integrada.

Fallback si existe `lspci`: `lspci -mm -nn` para nombre legible.

Dedicada vs integrada:

| Vendor | Regla |
|---|---|
| NVIDIA | siempre dedicada |
| AMD | `mem_info_vram_total` > 1 GiB → dedicada; si no existe o es chico → integrada |
| Intel | driver `xe` **y** VRAM > 1 GiB → Arc dedicada; `i915`/`xe` sin VRAM → integrada |

Ranking para elegir (mayor gana):

```
NVIDIA dedicada 100 > AMD dedicada 90 > Intel dedicada 80 > Intel iGPU 70 > AMD iGPU 60 > sin GPU 0
```

## Matriz de recomendación

| Hardware | Imagen | Motivo |
|---|---|---|
| Cualquier NVIDIA dedicada | `argos:nvidia` | CUDA |
| AMD dedicada | `argos:amd` | MIGraphX |
| Intel Arc dedicada | `argos:intel` | OpenVINO |
| Intel iGPU | `argos:intel` | OpenVINO GPU |
| AMD iGPU (APU) | `argos:amd` | MIGraphX GPU |
| Sin GPU, CPU Intel | `argos:intel` | OpenVINO CPU (oneDNN) |
| Sin GPU, CPU AMD | `argos:amd` | MLAS; ZenDNN opcional |
| Sin GPU, CPU desconocida | `argos:amd` | MLAS funciona en cualquier x86_64 |

Si el CPU y la dGPU son de marcas distintas (Intel + NVIDIA), la imagen acelera la GPU y el CPU queda
en MLAS. El CLI debe decirlo en el motivo.

## Pseudocódigo

```go
type GPU struct {
    Vendor    string // nvidia | amd | intel
    Driver    string
    Name      string
    Dedicated bool
}

func Recommend(cpu string, gpus []GPU) (image, reason string) {
    best := pickByRank(gpus) // ranking de arriba
    if best != nil && best.Dedicated {
        return imageFor(best.Vendor), "GPU dedicada " + best.Vendor
    }
    if best != nil {
        return imageFor(best.Vendor), "GPU integrada " + best.Vendor
    }
    switch cpu {
    case "GenuineIntel":
        return "argos:intel", "sin GPU; OpenVINO acelera el CPU Intel"
    case "AuthenticAMD":
        return "argos:amd", "sin GPU; CPU AMD sobre MLAS"
    default:
        return "argos:amd", "sin GPU detectada; imagen compatible con CPU genérica"
    }
}
```

## Comandos

```
b3l0w argos doctor   [--json]     # hardware + recomendación + motivo
b3l0w argos images   [--json]     # 3 imágenes, tamaño, soporte, estado local
b3l0w argos install  [--image nvidia|intel|amd] [--yes] [--no-start]
b3l0w argos status                # readyz, provider, modelos cargados
```

`doctor` debe mostrar algo así:

```
CPU        Intel (avx2, avx512f)
GPU        card0 Intel UHD (i915, integrada)
Imagen     argos:intel  · recomendada: GPU integrada Intel
Otras      argos:nvidia (8.7 GB) · argos:amd (~9 GB)
```

## Flujo de `install` (build local)

Decisión actual: **no hay registry**. La imagen se construye en la máquina del usuario desde el
Dockerfile del repo de Argos.

1. Detectar y recomendar (o usar `--image`).
2. Verificar el repo de Argos en `~/Documents/Github/argos`:
   - si no existe → clonarlo,
   - si existe → `git pull` (avisar si hay cambios locales).
3. Mostrar variante, tamaño estimado y motivo; confirmar salvo `--yes`.
4. Chequear disco libre: CPU/NVIDIA ~15 GB, AMD ~25 GB. Si no alcanza, abortar con el número exacto.
5. Construir: `make image-gpu` / `make image-cpu` (o `docker build --target ... --build-arg ORT_FLAVOR=... -t argos:local-<variante>-<sha7> .`).
6. Escribir `ARGOS_IMAGE=<tag>` en `~/Documents/Docker/argos/.env` **preservando** las demás claves.
7. Elegir el override de dispositivos:
   - NVIDIA: `-f compose.yaml -f compose.gpu.yml`
   - Intel/AMD: `-f compose.yaml -f compose.dri.yml` (monta `/dev/dri`)
   - sin GPU: solo `compose.yaml`
8. `docker compose up -d`.
9. Esperar `http://127.0.0.1:7712/readyz` (timeout 90 s; el build ya terminó).
10. Si no hay modelos instalados, correr `argosctl models install --yes …` dentro del contenedor.
11. Reportar `/v1/status`: provider real (`CPU`, `CUDA`, `OpenVINO`, `MIGraphX`).

### Rebuild y actualización

- Tag local con el commit: `argos:local-<variante>-<sha7>`; el CLI guarda ese tag en `.env`.
- `b3l0w argos update`:
  1. `git -C ~/Documents/Github/argos pull`
  2. si cambió `HEAD` o el `Dockerfile` → reconstruir la variante elegida
  3. `docker compose up -d` (recrea si el tag cambió)
- Consultar el label `org.opencontainers.image.revision` (si se agrega al build) evita reconstruir de
  más; mientras tanto, comparar el tag guardado con el `sha7` actual.
- Las capas de Docker hacen que un rebuild por cambio de código Go sea de ~1 min; solo un cambio de
  ORT/Dockerfile paga el build completo.

### Registry (opcional, futuro)

Si más adelante se publican, el flujo cambia `make image-*` por `docker compose pull`. Las imágenes de
un registry **no caducan**; el límite de 6 h es del runner de GitHub Actions, no del almacenamiento.

## Inventario, "descarga" y actualizaciones

**No hay registry**: no existe una imagen que se descargue. En este esquema, "descargar" son tres cosas:

1. `git clone` / `git pull` del repo **`Below-Z3R0/argos`** — **todavía no está creado en GitHub**;
   el repo local no tiene remote.
2. Docker baja y cachea las **imágenes base** (golang, debian, nvidia/cuda, openvino, rocm).
3. La imagen de Argos se **construye** en la máquina y se taggea `argos:local-<variante>-<sha7>`.

El inventario que ya tenés se extiende con una entrada por variante:

```
variante, source_repo, source_sha, image_tag, image_id,
revision_label, built_at, size_bytes, previous_tag
```

La imagen lleva el label `org.opencontainers.image.revision`, que se compara con el SHA del repo para
saber si el binario dentro corresponde al código:

```sh
docker image inspect "$tag" --format '{{index .Config.Labels "org.opencontainers.image.revision"}}'
```

### Primera instalación

1. Detectar hardware y recomendar variante.
2. Verificar `docker` y espacio (15 GB; AMD 25 GB).
3. Clonar el repo si falta; si existe, avisar por cambios locales.
4. Confirmar variante, tiempo estimado y tamaño.
5. Construir: `make image-<variante> IMAGE=argos:local-<variante>-<sha7>`.
6. Verificar el label de revisión contra el SHA.
7. Escribir `ARGOS_IMAGE` en `~/Documents/Docker/argos/.env`.
8. `compose up -d` con el override (`compose.gpu.yml` o `compose.dri.yml`).
9. Esperar `/readyz`; instalar modelos si faltan; reportar provider.

### Actualizaciones

1. `git -C <repo> fetch origin`.
2. Comparar `HEAD` con `origin/main` y el hash del Dockerfile.
3. Si hay cambios: mostrar `git log --oneline HEAD..origin/main` y preguntar
   **actualizar ahora / más tarde**.
4. Aceptar: `git pull` → build con el SHA nuevo → verificar label → `up -d`. Guardar el tag anterior
   como `previous_tag`.
5. Rechazar: registrar el SHA omitido para no volver a preguntar en cada arranque.

### Rollback

`b3l0w argos rollback` toma `previous_tag`, escribe `ARGOS_IMAGE` y hace `up -d`. Las imágenes viejas
se conservan; no se poda automáticamente.

Si más adelante se publica en un registry, el paso 5 cambia por `docker compose pull` y el inventario
guarda el **digest** en lugar de construirlo.

### Falta del lado de Argos

- **Crear y pushear `Below-Z3R0/argos`** (hoy no existe en GitHub).
- Validar los builds Intel/AMD una vez (en curso).
- Ya hecho: labels de revisión, tags con SHA en el Makefile, overrides de GPU/DRI, `readyz`.

## Contrato con Argos

- Imagen parametrizable: el compose ya usa `image: ${ARGOS_IMAGE:-argos:local}`.
- Puertos loopback: `7712` (argosd), `7711` (MCP), `6333/6334` (Qdrant).
- Volumen `argos-home` con modelos, config y manifiesto; `./qdrant` y `./qdrant-snapshots` en el host.
- El EP se decide en caliente con `resources.devices.embed/rerank`; el CLI no setea providers, solo
  elige la imagen.
- `doctor` de Argos verifica los EPs de verdad; usalo para validar la elección.

## Casos borde

| Caso | Comportamiento |
|---|---|
| Sin Docker o sin permiso | Error claro antes de tocar `.env` |
| Imagen no publicada todavía | Fallback a build local si existe el repo; si no, error |
| Usuario elige imagen sin soporte para su GPU | Aviso y confirmación explícita |
| Multi-GPU | Recomendar la de mayor ranking; mostrar todas |
| Stack ya corriendo con otra imagen | `up -d` recrea el contenedor; avisar |
| Host sin `/dev/dri` | No usar `compose.dri.yml` |

## Criterios de aceptación

- En un host sin GPU: recomienda `intel` o `amd` según CPU, instala y `/v1/status` muestra `CPU`.
- En Intel iGPU: recomienda `intel`, el provider es `OpenVINO`.
- En NVIDIA: recomienda `nvidia`, provider `CUDA`.
- En AMD: recomienda `amd`, provider `MIGraphX`.
- `--image` siempre gana sobre la recomendación.
- El `.env` conserva claves previas.

## Pendiente del lado de Argos (no bloquea el CLI)

- Targets `runtime-intel` y `runtime-amd` en el Dockerfile (Intel/AMD todavía no construidos).
- Publicación de las tres imágenes en un registry (`ghcr.io/<usuario>/argos:<tag>`).
- `compose.dri.yml` (montaje de `/dev/dri`) — crear junto con el target Intel/AMD.
