# TESTS — internal/selfupdate

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 13 |
| Verdes | 13 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **83.3 %** |
| Comando | `go test ./internal/selfupdate/ -count=1 -cover` |

Cobertura por función (medida con `go tool cover -func`):

| Función | Cobertura |
|---|---|
| `Run` | 91.2 % |
| `Check` | 80.0 % |
| `compareVersions` | 100.0 % |
| `numeros` | 88.9 % |
| `withV` | 100.0 % |
| **`Repository`** | **0.0 %** |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `selfupdate_test.go` | 13 tests: el prefijo `v` de la versión, la lectura del tag con la convención `v*` (ignorando un release de escritorio), el error con HTTP 500 y con JSON inválido, el rechazo del binario que pertenece a un paquete, el error cuando el asset bajado no responde, que no cambie nada si la versión es igual, la comparación de versiones (incluido `v0.5.0-dev` contra `v0.5.0`), que no descargue si el tag es viejo, que no degrade, que reemplace cuando la versión es distinta, que no reemplace si la firma falla, y que propague el error de reemplazo |

## Detalle de cada test

El módulo tiene una costura de inyección unusually completa
(`selfupdate.go:32-47`): `apiURL`, `fetchAsset`, `replaceFile`,
`packageOwner`, `currentVersion`, `verifyRelease`, `checkRelease` y
`resolveExe` son `var`s de paquete. El helper `conInyectables`
(`selfupdate_test.go:15-59`) las sustituye todas y las restaura con
`t.Cleanup`. Por eso la suite corre en 0.005s, sin red y sin tocar el
binario en ejecución.

### `selfupdate_test.go`

| Test | Qué fija |
|---|---|
| `TestWithV` | El prefijo `v`: `"0.5.0"` → `"v0.5.0"`, `"v0.5.0"` → `"v0.5.0"`, `""` → `"v"` |
| `TestCheckLeeElTag` | `Check` contra un `httptest` que devuelve un release de escritorio (`desktop-2026.11.06-r1`) y uno del CLI (`v0.6.0`): se queda con el `v*`. También verifica que el `User-Agent` enviado sea `b3l0w` |
| `TestCheckFallaConHTTPNoOK` | Un HTTP 500 produce error cuyo mensaje menciona "HTTP 500" |
| `TestCheckFallaConJSONInvalido` | Un body que no es JSON produce error |
| `TestRunRechazaBinarioDePaquete` | Si `packageOwner` dice que el binario es de pacman, `Run` falla con un error que menciona "pacman" y **no descarga** (el `fetch` inyectado llama a `t.Fatal` si se ejecuta) |
| `TestRunFallaSiElBinarioNoResponde` | Si el asset bajado no responde a `version`, `Run` propaga el error mentioning "no responde" |
| `TestRunNoCambiaSiLaVersionEsIgual` | Con la versión local igual a la del asset: `Changed == false`, `Before == After`, y el `replace` inyectado **no** se llama (contador en 0) |
| `TestCompareVersions` | Tabla de 6 casos: `v0.5.0` vs `v0.4.0` → 1, inverso → -1, iguales → 0, **`v0.5.0-dev` vs `v0.5.0` → 0** (los sufijos se ignoran), `0.10.0` vs `0.9.0` → 1 (comparación numérica, no de strings), `v1.0.0` vs `v0.99.99` → 1 |
| `TestRunNoDescargaSiElTagEsViejo` | Si el tag publicado es anterior a la versión local, **no** se baja nada (contador de descargas en 0) y se deja una `Note` |
| `TestRunNoDegrada` | Si el asset bajado declara una versión **menor** que la local, no se reemplaza y se deja la `Note` que lo explica |
| `TestRunReemplazaSiLaVersionEsDistinta` | El camino feliz: `Changed == true`, `After == "v0.6.0"`, y `replace` se llama con `(tmp, exe)` en el orden correcto |
| `TestRunNoReemplazaSiLaFirmaFalla` | Si la verificación de firma falla, `Run` da error y el `replace` **no** se llama. Es el test del fail-closed |
| `TestRunPropagaElErrorDeReemplazo` | Si `replace` falla con "permiso denegado", el error se propaga mentioning el mensaje original |

## Qué quedó sin cubierto (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **`Check` con releases fuera de orden de versión** | El test los pone en orden de creación = orden de versión | **Bug A1 (ALTO).** Verificado: con la lista `[desktop, v0.4.0, v0.9.0, v0.3.0]`, `Check` devuelve `v0.4.0` (el primero `v*`), no `v0.9.0`. Con un backport publicado, el CLI queda clavado en una versión vieja |
| **Prereleases (`-rc1`, `-beta`)** | `TestCompareVersions` solo cubre `-dev` | **Bug A2 (MEDIO).** Verificado: `numeros("v1.0.0-rc1") == numeros("v1.0.0") == [1 0 0]` y `compareVersions("v1.0.0-rc1","v1.0.0") == 0`. Un rc nunca se actualiza al release final |
| **El cuarto componente de la versión** (`v1.0.0.1`) | No está en la tabla de `TestCompareVersions` | Se pierde: `numeros("v1.0.0.1") == [1 0 0]`. Mismo bug A2 |
| **La versión local `"dev"`** | `conInyectables` permite fijarla (`"version": "0.5.0"`), pero ningún test usa `"dev"` | **Bug A3 (MEDIO).** Verificado: `withV("dev") == "vdev"`, `numeros("vdev") == [0 0 0]`, `compareVersions("v0.4.0", "vdev") == 1`. Una build local se cree más vieja que cualquier release y `update --self` la **reemplaza** |
| **La inyección por `-ldflags` de la versión real** | Requiere compilar el binario | `version.Version` real solo existe después del link. `internal/version` cubre el mecanismo; el valor concreto ("v0.5.0") lo simula el test |
| **El modo del binario después de reemplazar** | `replaceFile` está inyectado: nunca corre el `ReplaceFile` real | **Bug A4 (MEDIO).** Verificado en `internal/programs`: `system.ReplaceFile` con destino inexistente deja el archivo en `0644`, sin `+x`. El test de `selfupdate` no lo ve porque `replace` es un stub |
| **`Repository()`** | Nadie lo llama en los tests | 0 % de cobertura en una función que es una constante. Trivial, pero es la función que consume `internal/releases` (`internal/releases/signature.go:29`) |
| **`Check` con más de 30 releases** | `per_page=30` no se simula | **Bug B1 (BAJO).** Si el repo acumula 30 releases que no son del CLI, el tag `v*` queda fuera de la ventana y `Check` falla |
| **`Check` con `Draft` o `Prerelease`** | El test solo pone un release de escritorio | El filtro de drafts y prereleases (`selfupdate.go:83`) no está protegido por ningún test |
| **Un release `v*` sin el asset del CLI** | `Check` solo mira el tag | `Check` puede devolver un tag cuyo release no trae `b3l0w-linux-amd64`: el error aparece recién en la descarga, con un mensaje de `curl` |
| **El `fetch` que falla** (red caída, 404 del CDN) | El `fetchAsset` inyectado siempre devuelve `"/tmp/b3l0w-nuevo"` | El camino `selfupdate.go:177-180` (error de descarga) no está cubierto |
| **El `resolveExe` que falla** | Siempre devuelve una ruta | `selfupdate.go:149-152` (error al resolver el ejecutable) no está cubierto |
| **La race entre tests** | `conInyectables` restaura con `t.Cleanup` | Las vars son globales: dos tests en paralelo se pisan. Ninguno usa `t.Parallel()` hoy, así que no es un bug, pero es la restricción que impide agregarlo |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/selfupdate/ -count=1 -cover

# Ver la cobertura por función (Repository aparece con 0.0 %):
go test ./internal/selfupdate/ -count=1 -coverprofile=/tmp/selfupdate.cover
go tool cover -func=/tmp/selfupdate.cover
```
