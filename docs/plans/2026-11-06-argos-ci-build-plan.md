# Argos CI build plan — de iterar a ciegas a preflight + base + app

Fecha: 2026-11-06. Objetivo: construir las imágenes de Argos en GitHub Actions sin descubrir una
dependencia nueva en cada corrida de 30–45 minutos.

## 1. Diagnóstico de lo que ya falló

| Variante | Falla | Causa | Estado |
|---|---|---|---|
| CPU | — | — | build verde, imagen publicada |
| NVIDIA | — | ORT GPU oficial, sin compilar | build verde |
| Intel | `-Werror=free-nonheap-object` con GCC 13 | ORT compila warnings como errores; el warning es del toolchain, no del código | fix: `--compile_no_warning_as_error`; falta verificar build completo |
| AMD | `Could not find migraphxConfig.cmake` | faltaba `migraphx-dev` | fix aplicado |
| AMD | `Could not find MIOpen` | MIGraphX exige MIOpen, rocBLAS y hipBLASLt; no venían en `rocm/dev` | fix aplicado: `migraphx-dev miopen-hip-dev rocblas-dev hipblaslt-dev half` |
| Desktop | artifact filename con `:` | epoch de Arch (`1:5.4...`) no es válido en artifacts/releases de GitHub | fix: renombrar `:` a `_`; el epoch queda dentro de `.PKGINFO` |
| Desktop | `.git/config: Permission denied` | `chown -R builder /work` tocaba el checkout | fix: chown solo `packaging/` |

La lección: el build no se valida con “lo intento y espero 40 minutos”. Se valida en capas cortas.

## 2. Reglas de investigación aplicadas

- GitHub-hosted runner: 4 vCPU, 16 GB RAM, 14 GB SSD; job máximo 6 h.
  https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- `free-disk-space` libera hasta ~31 GB (deja ~43 GB):
  https://github.com/jlumbroso/free-disk-space
- Cache Docker: `type=gha` tiene 10 GB y 7 días; para capas grandes, `type=registry` en GHCR:
  https://docs.docker.com/build/ci/github-actions/cache
- Actions de terceros se pinnean a SHA; `permissions` mínimo:
  https://docs.github.com/en/actions/reference/security/secure-use
- Multi-arch: runner nativo `ubuntu-24.04-arm` + `docker buildx imagetools create`; QEMU solo para
  pruebas chicas:
  https://docs.docker.com/build/ci/github-actions/multi-platform

## 3. La causa raíz de AMD, según el propio CMake de ORT

`cmake/onnxruntime_providers_migraphx.cmake` (ORT 1.31) hace:

```cmake
find_package(hip REQUIRED)
find_package(migraphx REQUIRED PATHS ${AMD_MIGRAPHX_HOME})
```

Y `migraphx-config.cmake` instalado por `migraphx-dev` declara:

```cmake
find_dependency(Threads)
find_dependency(MIOpen)
find_dependency(rocblas)
find_dependency(hipblaslt)
```

Por eso no alcanza con `migraphx-dev`: hay que instalar **MIOpen, rocBLAS y hipBLASLt de desarrollo**.
En runtime hacen falta `migraphx`, `miopen-hip` y `half`. El README de MIGraphX lista las mismas
dependencias (MIOpen, rocBLAS, HIP, half, protobuf).

Documentación oficial:
- Build EPs: https://onnxruntime.ai/docs/build/eps.html
- MIGraphX EP: https://onnxruntime.ai/docs/execution-providers/MIGraphX-ExecutionProvider.html
- CMake del EP: https://github.com/microsoft/onnxruntime/blob/v1.31.0/cmake/onnxruntime_providers_migraphx.cmake
- Dependencias de MIGraphX: https://github.com/ROCm/AMDMIGraphX

## 4. Intel: por qué falló y qué mirar

`onnxruntime_providers_openvino.cmake` exige OpenVINO ≥ 2026.0:

```cmake
find_package(OpenVINO REQUIRED COMPONENTS Runtime ONNX)
if(OpenVINO_VERSION VERSION_LESS 2026.0)
  message(FATAL_ERROR "OpenVINO 2026.0 and newer are supported.")
endif()
```

Ya usamos `openvino/ubuntu24_dev:2026.4.1`. El fallo restante fue GCC 13 tratando un warning como
error. ORT tiene el flag oficial `--compile_no_warning_as_error`; se agregó. El preflight debe
verificar la versión de OpenVINO y que `build.sh --help` reconozca el flag antes de compilar.

## 5. Plan en capas (cada capa falla barato)

### Capa 0 — preflight (2–5 min, sin clonar ORT)

Un job por variante, en el **builder image**:

AMD:

```sh
apt-get update
apt-get install -y --no-install-recommends \
  cmake git migraphx-dev miopen-hip-dev rocblas-dev hipblaslt-dev half

cat >/tmp/preflight/CMakeLists.txt <<'EOF'
cmake_minimum_required(VERSION 3.20)
project(preflight CXX)
find_package(Threads REQUIRED)
find_package(hip REQUIRED)
find_package(migraphx REQUIRED)
find_package(MIOpen REQUIRED)
find_package(rocblas REQUIRED)
find_package(hipblaslt REQUIRED)
message(STATUS "MIGraphX preflight OK")
EOF

cmake -S /tmp/preflight -B /tmp/preflight/build
```

Intel:

```sh
# openvino/ubuntu24_dev:2026.4.1
cmake -S /tmp/preflight -B /tmp/preflight/build  # find_package(OpenVINO ... Runtime ONNX)
# comprobar versión >= 2026.0
```

Además:

```sh
# el flag debe existir antes de usarlo
/ort/build.sh --help 2>&1 | grep -q compile_no_warning_as_error
```

Si el preflight falla, **no se lanza el build de 30–45 min**. Se corrige el Dockerfile y se repite
solo el preflight.

### Capa 1 — base ORT (una vez por versión)

Separar la base de la app:

```text
argos-ort-gpu:1.31      nvidia/cuda runtime + ORT GPU oficial
argos-ort-intel:1.31    openvino runtime + ORT compilado
argos-ort-amd:1.31      rocm runtime + ORT/MIGraphX compilado
        ↓
argos:{cpu,gpu,intel,amd}-<sha>
```

- La base compila ORT **una vez**; la app solo copia binarios.
- Cache `type=registry` en GHCR; no `type=gha` para estas capas.
- `--progress=plain` y log guardado como artifact si falla.
- Base AMD: `rocm/dev` como **builder**; runtime con `migraphx`, `miopen-hip`, `half` y las libs
  necesarias, no el toolchain completo.

### Capa 2 — app + smoke (2–5 min)

- `docker run --rm image argosctl version`
- `ldd` sobre `libonnxruntime.so` y el provider: cero `not found`.
- Con un sidecar Qdrant: `argosd` + `curl /readyz`; el smoke no debe necesitar modelos reales.
- Solo después: push a GHCR con tags `<variant>-<sha>` y `<variant>-latest`.

## 6. Decisión pendiente: ROCm 6.4 vs 7.2

La matriz oficial de MIGraphX llega hasta ORT 1.23.2/ROCm 7.2; ORT 1.31 no aparece. Hay dos caminos:

1. **ROCm 6.4 + MIGraphX 2.12**: imagen más chica (3.98 GB sin comprimir), ya probada para el
   configure una vez instaladas las deps. Riesgo: API de MIGraphX más vieja que la que espera ORT
   1.31.
2. **ROCm 7.2 + MIGraphX 2.13/MIOpen 3.5**: más cerca de la matriz actual, imagen más grande y más
   disco de build. Riesgo: el runner necesita más limpieza y el compile puede ser más largo.

El plan es hacer **preflight en ambas** y elegir la que pase configure; si 6.4 compila, quedarse con
6.4 por tamaño. Si falla por API, cambiar a 7.2 sin tocar la app.

Alternativa a evaluar si el compile de ORT sigue siendo frágil: el plugin EP
`onnxruntime-ep-amdgpu` (ORT ≥ 1.24, plugin ABI). Evita compilar ORT entero, pero el binding Go
tendría que registrar el plugin; no es el camino inicial.

## 7. Workflow propuesto

```text
preflight-amd ─┐
preflight-intel├─> base-amd/base-intel/base-gpu ─> app-amd/app-intel/app-gpu
preflight-gpu  ┘
```

- `preflight-*`: 5 min, sin cache de ORT.
- `base-*`: 30–60 min, cache registry, publica base.
- `app-*`: 5 min, depende de base, smoke y push.
- `fail-fast: false` solo dentro de la matriz de app; los preflight son puerta dura.
- `concurrency` por variante; cancelar runs viejos.
- Actions pinneadas a SHA; `permissions: packages: write` solo en base/app.

## 8. Criterios de aceptación

- Preflight AMD e Intel verdes antes de cualquier compile.
- Base AMD e Intel publicadas con digest y sin `not found` en `ldd`.
- App CPU/GPU/Intel/AMD arranca y responde `/readyz`.
- Imágenes con labels de source/revision y tags inmutables.
- Si una variante falla, el log dice **etapa** (`preflight`, `configure`, `compile`, `runtime`) y no
  se reintenta a ciegas.

## 9. Desktop (aparte, ya validado hasta packaging)

El build de los 3 paquetes Arch ya llegó a `package()`; falló solo el nombre de artifact por el
epoch. El fix está aplicado. Para no repetir 18 minutos:

1. Preflight de PKGBUILDs: `bash -n`, `makepkg --printsrcinfo`.
2. Build real solo cuando cambie un PKGBUILD o upstream.
3. Renombrar `:` a `_` antes de artifact/release; el epoch queda en `.PKGINFO`.
4. La prueba de sesión Wayland sigue siendo de la VM/CLI, no del runner.

## 10. Próximo paso

Implementar primero **preflight** (dos jobs cortos) y correrlos. No lanzar base/app hasta que ambos
pasen. Después, base AMD/Intel con ROCm 6.4; si falla configure o compile por API, cambiar a 7.2.
