# TESTS — internal/services

Qué prueba cada test del módulo, qué quedó sin cubrir y por qué.

**Regla dura del módulo:** ningún test toca el Docker real. El
`DockerAdapter` llama a `docker` por el `PATH`, así que los tests
ponen un stub delante (`rollback_test.go:20-50`): registra cada
invocación en un log y puede fallar a propósito (`STUB_FAIL`,
`STUB_PS_OUT`, `STUB_INSPECT_OUT`). El registry se prueba con
`httptest.NewTLSServer` (el código arma las URLs con `https://` fijo,
`registry.go:45`). El hardware se prueba con `detectHardwareAt(
cpuinfo, drmRoot)` y directorios temporales (`hardware.go:64`).

## Estado de la suite

| Métrica | Valor |
|---|---|
| Funciones de test | 69 (8 archivos) |
| Casos ejecutados | 71 (69 top-level + 2 subtests) |
| Verdes hoy | 71 |
| **Rojos hoy** | **0** |
| Skips | 0 |
| Cobertura | **46.9 %** de statements (444/946) |
| Archivos de producción | 10 (2.088 líneas) |
| Archivos de test | 8 (1.854 líneas) |

Medido con `go test ./internal/services/ -count=1 -cover` →
`ok ... 0.054s coverage: 46.9% of statements`, y `go test -v` para el
desglose de verdes.

Los 71 casos pasan: 69 top-level más los 2 subtests de
`TestSwitchVariantRechazaLoQueNoCorresponde`. Los bugs que el módulo **sí** tiene (ver
`bugs.md`) no tienen test: son los que nadie escribió todavía. No hay
ningún test rojo en este módulo.

## Cobertura por archivo

| Archivo | Cobertura | Statements | Funciones y su cobertura |
|---|---|---|---|
| `hardware.go` | **92.6 %** | 125/135 | ninguna a 0 %: `DetectHardware` 100, `detectHardwareAt` 100, `detectGPUsAt` 100, `VariantsFor` 100, `isDedicated` 100, `Recommend` 92.3; las más bajas: `DescribeGPU` 71.4, `ueventDriver` 71.4, `readVRAM` 85.7 |
| `variant.go` | **86.0 %** | 43/50 | ninguna a 0 %: `repoName` 100, `contiene` 100, `SwitchVariant` 85.0, `ServiceDir` 75.0 |
| `manifest.go` | **84.1 %** | 58/69 | `LoadDefault` **0 %**; `parseManifest` 100, `Find` 100, `Embedded` 100, `LoadState` 100, `ListStates` 93.8, `LoadManifest` 83.3, `Save` 83.3, `SaveManifest` 75.0, `DefaultManifestPath` 75.0, `DefaultStateDir` 75.0 |
| `rollback.go` | **80.8 %** | 42/52 | `PruneImages` **0 %**; `NormalizeDigest` 100, `Rollback` 93.3, `ImageInUse` 85.7, `RemoveImage` 57.1 |
| `registry.go` | **77.9 %** | 67/86 | `NewRegistry` **0 %**; `AvailableVariants` 100, `LatestTag` 100, `Digest` 80.0, `Tags` 75.0, `token` 71.4, `imageRef` 66.7, `BaseURLFor` 66.7 |
| `search.go` | **38.5 %** | 15/39 | `recortar` **0 %**, `RenderSearch` **0 %**; `Match` 100, `coincide` 100 |
| `install.go` | **33.7 %** | 57/169 | `Install` **0 %**, `looksLikeArgos` **0 %**, `parseProvider` **0 %**; `Update` 65.5, `TagFor` 63.6, `ResolveVariant` 50.0, `shortDigest` 40.0, `repoDir` 33.3, `verify` 17.6 |
| `docker.go` | **27.1 %** | 32/118 | 13 funciones a 0 %: `HaveComposeV2`, `ComposeVersion`, `DockerOK`, `DiskFree`, `freeSpace`, `PS`, `Down`, `Logs`, `HTTPGet`, `PortInUse`, `GroupExists`, `NVIDIAToolkitInstalled`, `RenderDRI`; con cobertura: `NewDocker` 100, `composeFiles` 100, `Pull` 100, `Up` 100, `runEnv` 88.9, `ImageDigest` 75.0, `run` 66.7 |
| `doctor.go` | **4.5 %** | 5/111 | 7 funciones a 0 %: `Doctor`, `hasNvidia`, `Remove`, `Restart`, `Stop`, `StateDirReady`, `VariantForTag`; con cobertura: `ComposeFiles` 60.0, `ComposeFile` 66.7 |
| `report.go` | **0.0 %** | 0/117 | las 6 funciones a 0 %: `CheckRegistry`, `contains`, `Sync`, `sameList`, `RenderList`, `RenderHardware` |

El contraste es el dato: lo que está bien testeado es lo que es lógica
pura (`hardware.go`, `manifest.go`, `registry.go`); lo que no está
testeado es lo que habla con el mundo (`install.go`, `doctor.go`,
`report.go`, `docker.go`).

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `docker_env_test.go` | el env `IMAGE` llega al proceso de docker (1 test) |
| `hardware_paths_test.go` | detección de hardware con `/proc` y `/sys` falsos, matriz de variantes, ranking y `Recommend` (14 tests) |
| `hardware_test.go` | las funciones puras de GPU: vendor, dedicada vs integrada, `VariantsFor`, `Recommend`, `AvailableVariants`, `LatestTag` (9 tests) |
| `install_test.go` | el update por digest, la resolución de variante y la elección de tag (10 tests) |
| `manifest_test.go` | parseo y validación del manifiesto, round-trip del manifiesto y del estado, `ListStates` (18 tests) |
| `rollback_test.go` | normalización del digest, el rollback completo, sus fallos de pull y de up, `ImageInUse`; además el stub de docker (8 tests) |
| `search_test.go` | el filtro de búsqueda del manifiesto (6 tests) |
| `variant_test.go` | `SwitchVariant`: pull, recreate con el tag correcto, estado previo para el rollback, rechazos sin tocar docker (3 tests) |

---

## Qué prueba cada archivo

### `docker_env_test.go` — 1 test

| Test | Qué fija |
|---|---|
| `TestUpPasaElEnvImage` | el env con `IMAGE` llega al comando de docker. Si el env se pierde, compose levanta su default (`${IMAGE:-intel-latest}`) en vez de la variante pedida: el servicio queda con la imagen vieja y el estado dice otra cosa (fix `a346a61` en el README) |

Usa un stub `docker` propio (no el de `rollback_test.go`) que escribe el
env que recibió a un archivo, y mete el directorio del stub delante del
`PATH`.

### `hardware_paths_test.go` — 14 tests

Arma un árbol de DRM falso con la misma forma que `/sys/class/drm`
(`cardN/device/{vendor,uevent,mem_info_vram_total}`) y un
`/proc/cpuinfo` falso.

| Test | Qué fija |
|---|---|
| `TestDetectHardwareAtCPUVendor` | el vendor del cpuinfo que le pasan (GenuineIntel / AuthenticAMD) |
| `TestDetectHardwareAtCPUInfoIlegible` | sin `/proc/cpuinfo` no hay vendor ni flags, y el CLI no se cae |
| `TestDetectHardwareAtCPUFlags` | solo mira `avx2` y `avx512f`, en ese orden |
| `TestDetectGPUsAtDetectaCadaVendor` | detecta NVIDIA, AMD e Intel con su driver, su VRAM y su nombre; la NVIDIA sale primera (ranking) |
| `TestDetectGPUsAtIgnoraLoQueNoEsGPU` | los conectores (`card0-DP`), los vendors desconocidos y los `card` sin `device` no son GPUs |
| `TestDetectGPUsAtSinDirectorio` | sin `/sys/class/drm` no hay GPUs, pero el CLI no se cae |
| `TestDetectHardwareAtNoSeComeElHost` | `DetectHardware()` lee el `/proc` y el `/sys` reales: solo comprueba que no se caiga (el contenido depende de la máquina) |
| `TestVariantsForDaLaVarianteAceleradaYLaCPU` | la matriz de 5 hardwares: siempre 2 variantes (la acelerada + `cpu`), con su provider y su motivo |
| `TestVariantsForSinGPUSoloCPU` | sin GPU hay una sola variante, `cpu`, y el motivo dice "sin GPU" |
| `TestVariantsForTomaLaMejorGPU` | con varias GPUs manda la mejor según el ranking, no la primera de la lista |
| `TestGPURankOrdenaPorDedicated` | el ranking: NVIDIA ded. 100 > AMD ded. 90 > Intel ded. 80 > Intel iGPU 70 > AMD iGPU 60 > NVIDIA sin marcar 0 |
| `TestRecommendSinVariantesPublicadasFalla` | sin catálogo no hay nada que recomendar |
| `TestRecommendAvisaSiElBuildNoEstaPublicado` | si la variante que el hardware pide no está publicada, el motivo dice "falta el build" |
| `TestRecommendUsaLaCPUCuandoNoHayAceleracion` | con GPU AMD integrada y solo la CPU publicada, la CPU es la que va |

### `hardware_test.go` — 9 tests

Las funciones puras, sin filesystem.

| Test | Qué fija |
|---|---|
| `TestVendorFromID` | `0x10de`→nvidia, `0x1002`→amd, `0x8086`→intel, lo demás vacío |
| `TestIsDedicated` | NVIDIA siempre dedicada; AMD con VRAM > 1 GiB; Intel solo con driver `xe` **y** VRAM > 1 GiB |
| `TestVariantsFor` | `Recommend` sobre 7 hardwares, con el catálogo completo |
| `TestRecommendFallsBackToPublished` | la variante que el host soporta pero no está publicada → cae a la publicada |
| `TestRecommendPrefersPublishedAcceleration` | con NVIDIA y `cuda` publicada, recomienda `cuda` |
| `TestAvailableVariants` | deduce las variantes publicadas de los tags; `buildcache-*` no cuentan |
| `TestAvailableVariantsDefault` | `default` existe si hay algún tag que no sea de caché; solo caché no cuenta |
| `TestLatestTag` | el tag más nuevo de una variante; el tag exacto (`rocm`) también es suyo |
| `TestDescribeGPU` | arma una línea legible no vacía |

### `install_test.go` — 10 tests

El corazón del módulo: el update por digest y la resolución de variante.
El registry falso (`fakeRegistry`, `install_test.go:17-48`) sirve el
token y los tags, y el digest remoto por el header
`Docker-Content-Digest`, como en un registry de verdad.

| Test | Qué fija |
|---|---|
| `TestUpdateNoActualizaSiElDigestEsElMismo` | el digest local (`repo@sha256:...`) y el remoto (`sha256:...`) normalizados dan igual: no se toca docker y no se marca `UpdatedAt` |
| `TestUpdateActualizaSiElDigestCambia` | digest distinto → un pull, un `up -d` y el estado guardado con el digest nuevo |
| `TestUpdateFallaSiElRegistryNoDevuelveDigest` | un registry sin el header no se puede comparar: avisa y no toca docker |
| `TestUpdateNoActualizaSiElDigestRemotoQuedaVacio` | un digest remoto que normaliza a vacío no dispara update |
| `TestUpdateFallaSiNoEstaInstalado` | sin estado, o con estado sin imagen, el update falla |
| `TestResolveVariantEligeLaDelUsuario` | la variante que pide el usuario gana aunque no sea la del hardware, y el motivo no está vacío |
| `TestResolveVariantFallaConUnaVarianteNoPublicada` | pedir una variante que no existe es un error, no un fallback silencioso |
| `TestResolveVariantFallaSinVariantesPublicadas` | sin catálogo no hay variante |
| `TestTagForEligeElTagDeLaVariante` | el tag sale del catálogo, no de lo que dice el manifiesto: `cuda` → `cuda-latest` |
| `TestTagPorDefectoTomaElTagDeVersion` | la variante `default` toma el tag de versión (`3.10.4`), saltando `arm64-`, `sha-` y `latest` |

### `manifest_test.go` — 18 tests

| Test | Qué fija |
|---|---|
| `TestParseManifestLeeElManifiesto` | el manifiesto tiene version, servicios y **todos** los campos de cada uno (id, name, repo, image, variants, available, gpu, ep, ports, health, state, size_gb, overrides, description, components, models, compose) |
| `TestParseManifestRechazaLoInvalido` | 6 casos: JSON roto, sin versión, sin servicios, id con `../`, id con barra, id vacío. Un manifiesto roto tiene que fallar, no devolver uno vacío que el CLI despiese |
| `TestParseManifestAceptaElManifiestoEmbebido` | el JSON que va dentro del binario parsea, y todos sus servicios tienen id y state |
| `TestLoadManifestLeeElArchivo` | el manifiesto se lee del path que le pasan |
| `TestLoadManifestFallaSiNoEstaElArchivo` | un path inexistente falla |
| `TestManifestFindDevuelveElServicioYNilSiNoEsta` | `Find` devuelve el servicio o nil, nunca un id que no existe |
| `TestSaveManifestOrdenaPorID` | el manifiesto se guarda ordenado por id (el diff del repo de distro no es ruido) y termina en newline |
| `TestSaveManifestCreaElDirectorio` | guardar en un path nuevo crea el directorio |
| `TestSaveManifestYLoadManifestEsUnRoundTrip` | lo que se guarda se vuelve a leer igual, campo por campo |
| `TestStateSaveYLoadEsUnRoundTrip` | el estado guardado se vuelve a leer igual: id, variant, image, tag, digest, timestamps, health, provider, ports, previous*, compose services y warnings |
| `TestStateSaveEscribeElArchivoDelID` | el nombre del archivo es el id del servicio |
| `TestStateSaveCreaElDirectorio` | el directorio de estado se crea si no existe |
| `TestLoadStateFallaSiNoEsta` | un estado que no existe falla |
| `TestListStatesLeeTodosYOrdena` | lista los estados del directorio, ordenados por id |
| `TestListStatesIgnoraLoQueNoEsEstado` | un directorio sin estados no es un error; y en un directorio con ruido (`notas.txt`, `roto.json`, `subdir.json`) solo aparece el estado válido |
| `TestStateSaveConDirectorioSinPermisosFalla` | si no puede escribir, el error se propaga (no se pierde en silencio) |
| `TestDefaultStateDirEsElEsperado` | el estado vive en `~/.local/state/b3l0w/services` |
| `TestDefaultManifestPathEsElEsperado` | el manifiesto por defecto vive en `<repo>/services/services.json` |

### `rollback_test.go` — 8 tests

Además de los tests, este archivo define el **stub de docker** que
todos los demás usan (`dockerStub`, `rollback_test.go:16-81`): un
binario `docker` falso en el `PATH` que loguea cada llamada y puede
fallar a propósito según `STUB_FAIL`.

| Test | Qué fija |
|---|---|
| `TestNormalizeDigest` | 7 formas del digest → la misma forma `sha256:...` (con repo delante, con espacio, con newline, con `@` al principio, vacío) |
| `TestNormalizeDigestComparaLocalYRemoto` | la propiedad que importa: el mismo digest escrito de las dos formas da el mismo string, y dos digests distintos siguen distintos |
| `TestRollbackSinEstadoNoHaceNada` | sin estado no hay a qué volver, y no se llama a docker |
| `TestRollbackSinImagenAnteriorNoHaceNada` | sin tag previo guardado no hay imagen a la que volver, y no se llama a docker |
| `TestRollbackVuelveALaImagenAnterior` | el ciclo completo: pull del tag anterior, recreate con ese tag en el env, estado guardado (imagen nueva como `previous`), y la imagen que quedó atrás borrada |
| `TestRollbackFallaSiElPullFalla` | sin la imagen anterior no hay rollback: falla con `pull:` y **no guarda** el estado |
| `TestRollbackFallaSiElUpFalla` | si el stack no levanta con la imagen anterior, avisa y **no guarda** el estado |
| `TestImageInUseMiroLosContainers` | la imagen que usa un container no se puede borrar; `docker ps` se llama dos veces |

### `search_test.go` — 6 tests

| Test | Qué fija |
|---|---|
| `TestMatchPorID` | match por id, sin distinguir mayúsculas |
| `TestMatchPorDescripcion` | match por descripción |
| `TestMatchPorVariante` | match por variante |
| `TestMatchVacioDevuelveTodo` | una consulta vacía devuelve todos (para que `-Ss` sin argumentos sea `list`) |
| `TestMatchSinResultados` | una consulta que no matchea devuelve vacío |
| `TestMatchNoDistingueMayusculas` | `SeAnImE` matchea `seanime` |

### `variant_test.go` — 3 tests

`SwitchVariant` es la función nueva del módulo (`variant.go`, commit
`ce79fed`). El registry falso de este archivo (`registryFalso`,
`variant_test.go:17-33`) y el servidor de prueba
(`servidorDePrueba`, `variant_test.go:37-54`) arman un manifiesto con
un servicio y su estado instalado.

| Test | Qué fija |
|---|---|
| `TestSwitchVariantAplicaYPersiste` | el cambio aplica: la variante y el tag nuevos (con `["cpu","rocm"]`, `LatestTag` tiene que elegir `rocm`, no el primero), el estado en disco con la variante nueva y la anterior guardada para el rollback, y **el `up` con `IMAGE=rocm`** (recrear es lo que aplica la variante) |
| `TestSwitchVariantRechazaLoQueNoCorresponde` | 2 subtests: servicio desconocido y variante no publicada. Un rechazo no toca el estado ni llama a docker |
| `TestSwitchVariantSinInstalarFalla` | sin estado no hay a qué cambiar, y no se llama a docker |

---

## Qué quedó sin cubrir (y por qué)

| Caso | Por qué no está |
|---|---|
| **`Installer.Install` entero** (`install.go:78`) | **el hueco más grande del módulo**. Instalar requiere `Registry.Tags` real, un `docker` real y un health real. La seam que falta no es del test: `Install` llama a `DetectHardware()` (`install.go:87`) y a `HTTPGet` (`install.go:171`), que son globales y no se pueden inyectar. `Installer` ya tiene campos inyectables para `Docker`, `Registry`, `StateDir` y `RepoDir` — falta un `Hardware` y un `Health` |
| **`verify` (`install.go:164`)** | 17.6 % de cobertura. El camino del health que responde y da provider sí se puede probar con un `httptest`; falta el camino del timeout de 120 s (que en la probe tardó 120 s reales) y el de los warnings de Argos |
| **`Doctor` entero** (`doctor.go:18`) | 4.5 %. Es una función de 100 líneas con cinco entradas de error (daemon caído, compose v1, disco bajo, sin GPU, con NVIDIA) y dos salidas (puertos de los instalados, variantes vs hardware). Sale de `output.Step`/`Ok`/`Warn`/`Fail` a stdout, así que probarlo implica capturar la salida. Los bugs de `doctor.go` (M4, M5) están exactamente en ese hueco |
| **`CheckRegistry` y `Sync` (`report.go:28,80`)** | 0 %. `report.go` **no tiene ni un test**. Es la capa que decide qué variante se ofrece y qué se marca como "desactualizado". Es testeable: `Registry` ya es inyectable y `Manifest` es un struct plano — falta el test |
| **`RenderList` y `RenderHardware`** | 0 %. Imprimen a stdout con `fmt.Printf`. testeables con captura de stdout, como el resto |
| **`recortar` y `RenderSearch`** (`search.go:43,55`) | 0 %. `RenderSearch` imprime; `recortar` es puro y se puede testear directo |
| **Los comandos de compose** (`PS`, `Down`, `Logs`, `Remove`, `Restart`, `Stop`) | 0 %. El stub de docker ya registra las llamadas: alcanza con un test que verifique los argumentos que arma cada comando (los `-f` absolutos, el `--project-directory`) |
| **`HTTPGet`** (`docker.go:243`) | **0 %**: ningún test lo llama, ni siquiera el camino 200. Es la función del bug A1 (`bugs.md`): con un `httptest` que devuelva 500 se ve que devuelve `err == nil` y el `verify` marca `healthy` |
| **`PortInUse`, `GroupExists`, `NVIDIAToolkitInstalled`, `RenderDRI`** | 0 %. Leen el sistema (`ss`, `getent`, `/usr/bin`, `/dev/dri`). Se pueden testear con un `PATH` stub para `ss`/`getent`; los otros dos dependen del hardware de la máquina |
| **`DiskFree` y `freeSpace`** (`docker.go:114,127`) | 0 %. `freeSpace` es testeable con un `t.TempDir()`; `DiskFree` necesita un `docker info` stub que devuelva un `DockerRootDir` |
| **La concurrencia** (dos `update` sobre el mismo servicio) | No hay lock en el estado (como en `internal/dotfiles`). El estado es un read-modify-write: dos updates simultáneos se pisan el `Digest` y el `Previous`. No lo reproduje — queda como **verificar** |
| **El `docker` real** | Por diseño: los tests usan un stub en el `PATH`. Un test de integración contra Docker real exigiría un daemon en la máquina de CI |
| **El registry real (GHCR)** | Por diseño: `httptest.NewTLSServer` con TLS porque `Registry.token` arma la URL con `https://` fijo (`registry.go:45`) |

---

## Decisiones pendientes

No se escribió ningún test nuevo: el encargo es documentar, no tocar
tests. Estas son las decisiones que un escritor de tests tendría que
tomar antes de agregar cobertura.

### D1 — Cómo testear `Install`

`Install` necesita hardware y health inyectables. Hay dos caminos:

1. **Agregar campos a `Installer`** (`Hardware Hardware`, `Health
   func() (string, error)`), como ya se hizo con `Docker`, `Registry`,
   `StateDir` y `RepoDir`. Es lo consistente con el resto del struct,
   pero es un cambio de código de producción.
2. **Probar `Install` con un `httptest` real** y `DetectHardware()`
   real. El hardware no se puede controlar, así que el test solo puede
   afirmar cosas que no dependan de la GPU (el `pull`, el `up`, el env,
   el estado guardado).

La opción 1 es la que corresponde: sin ella, `install.go:78` no puede
tener un test útil, y es el camino que más usuarios tocan.

### D2 — El env sin `IMAGE` (bug A5)

Un test que fije el env del `up` cuando `state.Tag` está vacío
necesita decidir qué se espera: ¿que el `up` lleve el tag resuelto, o
que el update lo guarde antes? El fix del bug elige las dos cosas
(`install.go:281-283` + persistir el tag). El test debería fijar la
combinación.

### D3 — El `Previous` del update (bug A2)

El rollback manual, tal como está documentado en el README, no puede
volver atrás con tags mutables. Un test que fije "el rollback vuelve a
la imagen anterior" **falla hoy** y recién pasa cuando el diseño del
rollback cambia (guardar el digest anterior y volver por digest). Es un
test rojo que documenta la decisión pendiente, como los `TestBug_` de
`internal/dotfiles`.

### D4 — Qué se afirma del `doctor`

`Doctor` escribe con `output.Ok`/`Warn`/`Fail`, que van a stdout. Un
test tiene que capturar stdout (o el package `output` necesita una
seam). Decidir eso es lo primero antes de escribir un solo test del
doctor.

### D5 — `report.go` sin tests

`CheckRegistry` y `Sync` son funciones de datos: reciben un
`*Manifest` y devuelven estructuras. Se pueden testear sin tocar
stdout: `Sync` se verifica por el `Manifest` que deja en disco y por
el `changed` que devuelve. `RenderList`/`RenderHardware` sí necesitan
captura de stdout. Conviene separar las dos cosas.

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/services/ -count=1

# Con cobertura
go test ./internal/services/ -count=1 -cover

# Ver qué queda sin cubrir, por función
go test ./internal/services/ -count=1 -coverprofile=/tmp/svc.cov
go tool cover -func=/tmp/svc.cov | grep -v '100.0%'

# Un archivo solo
go test ./internal/services/ -count=1 -run 'TestSwitchVariant' -v

# Con el detector de carreras
go test ./internal/services/ -count=1 -race
```

**Nunca se toca Docker real.** El adapter llama a `docker` por el
`PATH`, así que los tests ponen un stub delante: registra cada llamada
y puede fallar a propósito, para probar los caminos de error.
