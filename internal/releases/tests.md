# TESTS — internal/releases

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 2 |
| Verdes | 2 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **60.0 %** |
| Comando | `go test ./internal/releases/ -count=1 -cover` |

Cobertura por función (medida con `go tool cover -func`):

| Función | Cobertura |
|---|---|
| `Check` | 77.8 % |
| `checkCLI` | 69.2 % |
| `githubRepo` | 100.0 % |
| **`Render`** | **0.0 %** |

`Render` es la función que **imprime** el estado y la que **devuelve el
número de releases sin firma** — el valor con el que `doctor` arma su exit
code (`internal/doctor/doctor.go:289`) y con el que `update --check`
avisa (`internal/update/update.go:543-545`). Es la función más importante
del módulo y no tiene **ningún** test.

La suite tarda **1.809s** (contra 0.005s de los otros módulos): el test
espera los reintentos de `kit/releases.HasSignature` ante el 404 del CDN
(`kit/releases/signature.go:66-85`, con `retryDelay = 300ms`).

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `signature_test.go` | 2 tests: el chequeo de un manifiesto mixto (un repo firmado, uno sin firma y una fuente no-GitHub, más el CLI propio) y el parseo de `github:owner/repo` con sus casos inválidos |

## Detalle de cada test

### `signature_test.go`

| Test | Qué fija |
|---|---|
| `TestCheckSoloProgramasDeGitHub` | El camino completo de `Check` con un manifiesto mixto: un repo con `SHA256SUMS.asc` (firmado), un repo sin el asset (sin firma) y una fuente `file:///` (que debe saltearse). Verifica que salen 3 estados (2 programas de GitHub + el CLI propio), que el primero está firmado, que el segundo no y con el tag `v2.0.0`, y que el tercero es el CLI con su tag. Inyecta `kitreleases.SetBases` y `cliTag`/`cliRepo`, restaurándolos con `t.Cleanup` |
| `TestGitHubRepo` | El parseo de la fuente: `github:owner/repo` → `("owner/repo", true)`, y los inválidos `github:owner`, `github:/repo`, `file:///tmp` → `("", false)`. Es una tabla con los 4 casos |

## Qué quedó sin cubrir (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **`Render` completo** | Nadie lo probó | **El 40 % del módulo sin cubrir.** `Render` decide qué se imprime y **cuántos releases sin firma se cuentan**, y ese número es el exit code de `doctor`. No hay ningún test que verifique: que un release firmado salga como `Ok`, que uno sin firma salga como `Fail` y **sume 1**, que uno no consultable salga como `Warn` y **no sume**, ni que se imprima la línea con el comando `b3l0w-manteiner sign release` (`signature.go:106`) |
| **`Render` con lista vacía** | `signature.go:93-95` devuelve 0 sin imprimir nada | El contrato "sin estados no imprime sección" no está protegido |
| **Un repo de GitHub que no existe (404)** | El test solo cubre el 404 del *asset* de firma, no del *repo* | Verificado con un `httptest` que devuelve 404 en todo: el estado queda `err="sin releases"`, `signed=false`. El `switch` de `Render` lo muestra como warning y **no** lo cuenta como error, así que no se confunde con "SIN FIRMA" — pero el mensaje no dice por qué falló (**bug B1**) |
| **Un error de red** | Requiere un server que corte la conexión | `st.Err` recibe el error crudo; no hay test que fije el texto |
| **`Check` con un programa cuyo `LatestTag` falla** | El test cubre el éxito y el "no hay asset de firma", no el fallo del tag | La rama `signature.go:55-59` (error de `LatestTag` → `st.Err`) no está cubierta: es la que llena el `Err` del caso 404 |
| **`checkCLI` cuando `selfupdate.Check` falla** | `cliTag` siempre devuelve `("v1.0.0", nil)` en el test | La rama `signature.go:76-79` (error al buscar el tag del CLI) no está cubierta |
| **`githubRepo` con ruta extra** (`github:owner/repo/extra`) | La tabla de casos no incluye ese input | **Bug M1 (MEDIO).** Verificado: devuelve `("owner/repo/extra", true)` en vez de `("", false)` |
| **El `client` declarado en el paquete** | Nadie lo usa; el test inyecta por `kitreleases.SetBases` | **Bug M2 (MEDIO).** Es código muerto que sugiere una costura de inyección que no existe |
| **`Render` escribe en stdout** | Nadie captura la salida | Si `Render` cambia de `fmt.Printf` a `output.*`, el formato cambia y nada lo detecta. Hoy hay una inconsistencia: `signature.go:106` usa `fmt.Printf` mientras el resto del módulo usa `output.*` (**bug B2**) |
| **La concurrencia** | `cliTag`/`cliRepo` son globales mutables | Igual que en `versioncheck`: dos tests que los cambien a la vez se pisan. El `t.Cleanup` los restaura, pero no serializa |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/releases/ -count=1 -cover

# Ver la cobertura por función (Render aparece con 0.0 %):
go test ./internal/releases/ -count=1 -coverprofile=/tmp/releases.cover
go tool cover -func=/tmp/releases.cover
```
