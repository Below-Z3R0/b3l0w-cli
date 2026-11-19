# TESTS — internal/update

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 31 |
| Verdes | 31 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **50.0 %** |
| Comando | `go test ./internal/update/ -count=1 -cover` |

Cobertura por función (medida con `go tool cover -func`):

| Función | Cobertura | Nota |
|---|---|---|
| `confirmPrompt` | 100.0 % | |
| `defaultDotfilesDir` | 100.0 % | |
| `fileExists` | 100.0 % | |
| `localVersion` | 100.0 % | |
| `compareVersions` | 100.0 % | |
| `desktopUpdate` | 100.0 % | |
| `updatePrograms` | 86.7 % | |
| `updateDotfiles` | 74.1 % | |
| `dryRun` | 61.1 % | |
| `updateSystem` | 55.6 % | |
| `Run` | 51.1 % | |
| `syncRegistry` | 50.0 % | |
| `updateServices` | 15.0 % | |
| **`updatePlugins`** | **0.0 %** | |
| **`selfUpdate`** | **0.0 %** | |
| **`check`** | **0.0 %** | |
| **`checkPlugins`** | **0.0 %** | |
| **`checkDesktop`** | **0.0 %** | |

Cinco funciones con 0 % de cobertura, entre ellas **`check`** — la que
implementa `b3l0w update --check`, el comando que el usuario usa para
"saber si hay que actualizar" — y **`checkDesktop`**, que ya tiene un bug
probado (A3).

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `update_test.go` | 25 tests: los selectores del prompt y su orden, `defaultDotfilesDir`, `fileExists`, `localVersion` y `compareVersions` (parseo, fallback a 0 sin vercmp o con salida inválida, y semántica real de epoch/pkgrel/vacío), `updatePrograms`, `updateServices`, `updateSystem`, `updateDotfiles`, `desktopUpdate` y el despacho de `Run` |
| `dotfiles_test.go` | 3 tests de regresión de bugs: el dry-run muestra los conflictos y no escribe nada, el dry-run anuncia cuántos conflictos hay, y los huérfanos se avisan |
| `confirm_test.go` | 1 test: la tabla completa de `confirmPrompt`, una fila por combinación de capas |

## Detalle de cada test

El módulo tiene una regla de testeabilidad tajante (`update_test.go:21-26`):
**ningún test puede mutar la máquina**. Los binarios que el código busca en
el PATH (`vercmp`, `pacman`, `snapper`, `noctalia`, `cachy-update`) se
reemplazan por scripts falsos en un directorio temporal y el `PATH` se acota
a ese directorio. El helper `marcador` (`update_test.go:119-127`) deja un
archivo en disco si el fake llega a ejecutarse: si el código toca el
sistema, el test falla.

### `update_test.go` (25 tests)

| Test | Qué fija |
|---|---|
| `TestConfirmPromptSinSeleccion` | `confirmPrompt` con las 6 capas en `false` → `"Update? [Y/n] "` (sin lista) |
| `TestConfirmPromptOrdenDeLasCapas` | El **orden** del prompt: el usuario tiene que leer la lista en el mismo orden en que se ejecutan, no en el que se pasaron los flags. 4 casos |
| `TestDefaultDotfilesDirPrefiereElRepoDeGithub` | Con repo y share presentes, gana el repo de GitHub (es el que el usuario editó) |
| `TestDefaultDotfilesDirCaeAlShare` | Sin repo, se usa la copia compartida |
| `TestDefaultDotfilesDirSinDotfiles` | Sin ninguno → `""` (el llamador avisa que falta `--dotfiles-dir`) |
| `TestDefaultDotfilesDirSinHome` | Sin `HOME` → `""`, no un panic |
| `TestFileExists` | Un archivo existe → `true`; un **directorio** también (el `os.Stat` lo acepta, y el test lo documenta); un path inexistente → `false`; `""` → `false` |
| `TestCompareVersionsLeeLaSalidaDeVercmp` | La salida de `vercmp` se traduce tal cual: `-1`, `0`, `1`, `" -1 \n"` (con espacios), `"1"` (sin newline) |
| `TestCompareVersionsSinVercmpDevuelveCero` | Sin `vercmp` en el PATH → 0. Es la regla que evita inventar updates |
| `TestCompareVersionsConSalidaInvalida` | Una salida que no es un número (`"abc"`, `""`, `"1.5"`) → 0, no un update inventado |
| `TestCompareVersionsConVercmpQueFalla` | `vercmp` sale con error → 0 |
| `TestCompareVersionsPasaLaVersionLocalPrimero` | El **orden de los argumentos** importa: `vercmp` compara el primero contra el segundo. Un fake que devuelve 1 solo si `$1 == "local"` |
| `TestCompareVersionsConVercmpReal` | El `vercmp` **de verdad** (se saltea si no está instalado): fija la semántica con epoch (`1:1.0` > `2.0`), pkgrel (`1.0-1` < `1.0-2`) y vacíos (`""` < `"1.0"` < `"1.0"`). No lee la base de datos ni toca nada |
| `TestLocalVersionLeeLaVersion` | El formato de `pacman -Q` es "nombre version": el segundo campo es la versión |
| `TestLocalVersionConSalidaIncompleta` | Si pacman no devuelve nombre y versión, no hay versión que informar |
| `TestLocalVersionConError` | El error de pacman se traduce en versión desconocida, no en un panic |
| `TestUpdateProgramsSinManifiesto` | Sin programas registrados avisa y no ejecuta nada |
| `TestUpdateProgramsReportaElProgramaAusente` | Un programa que no está instalado se cuenta como **fallo**: avisa "no está instalado" y "1 de 1 programas fallaron", y devuelve 1. Con el `PATH` acotado, `CommandPath` no lo encuentra y no se ejecuta nada |
| `TestUpdateServicesSinEstado` | Sin estado guardado no se anuncia la actualización de servicios (no toca Docker) |
| `TestUpdateSystemSinActualizador` | Sin `cachy-update` ni `arch-update`: se omite el sistema. Pedirlo solo → error (código 1); pedirlo junto a otra capa → solo un aviso (código 0) |
| `TestUpdateSystemSinTerminal` | El actualizador del sistema es interactivo: sin terminal no se lanza. El fake deja marker, y el test verifica que **no** se creó |
| `TestUpdateDotfilesSinDirectorio` | Sin repo de dotfiles avisa y sale con error, sin tocar el home |
| `TestUpdateDotfilesAdopt` | El modo `--adopt` registra el hash de lo que hay en el home como base y **no toca ni un archivo**: se verifica el contenido del archivo local, que el estado guarde el hash del **local** (no el del repo) y la ruta relativa, y que **no** se ejecuten `snapper` ni `noctalia` (markers) |
| `TestDesktopUpdateMuestraElComandoPorCanal` | El escritorio no se actualiza con pacman: se muestra el comando según el canal (`stable` → `pacman -S umbriel noctalia`, `edge` → `pacman -S umbriel-git noctalia-git`) y el fake de pacman deja marker: el CLI **no** lo ejecuta |
| `TestRunSinYesCancelaSinTerminal` | Sin `--yes` y sin terminal el update se cancela: imprime "cancelled" y sale con 0. Nunca se actualiza a ciegas |
| `TestRunConYesReportaElProgramaAusente` | El flujo completo con `--yes` llega a los programas y reporta los que no están instalados, sin ejecutar nada del sistema |
| `TestRunDryRunMuestraElPlan` | El dry-run informa qué haría ("Dry-run: no se toca nada") y cuenta los programas ("programas: 1 se revisarian") |

### `dotfiles_test.go` (3 tests)

El helper `escenarioConflicto` (`dotfiles_test.go:15-53`) arma el caso real
de conflicto: el repo tiene un archivo, el home tiene **otro** contenido y
el estado tiene una **tercera** versión como base (para que los dos hayan
cambiado y sea un conflicto de verdad, no un "kept").

| Test | Qué fija |
|---|---|
| `TestBug_ElDryRunMuestraLosConflictos` | Regresión: el dry-run tiene que mostrar los conflictos, nombrarlos y **no escribir nada** (ni el archivo ni el `.new`) |
| `TestBug_ElDryRunAnunciaCuantosConflictosHay` | El dry-run tiene que decir **cuántos** archivos-conflicto se pisarían con `--force`; sin ese número el usuario no puede decidir |
| `TestBug_LosHuerfanosSeAvisan` | Los huérfanos (en el estado pero no en el repo) se avisan y se nombran. Regresión: el campo existía en `internal/dotfiles` pero la CLI no lo imprimía |

### `confirm_test.go` (1 test)

| Test | Qué fija |
|---|---|
| `TestConfirmPrompt` | La tabla completa de `confirmPrompt`: una fila por combinación de capas (todas, y cada una sola). Fija el formato exacto del prompt, incluido el espacio final |

## Qué quedó sin cubierto (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **`check` completo** | Necesita `system.List` (cachy-update), AUR y la API de GitHub | **0 % de cobertura.** Es `b3l0w update --check`, el comando que el usuario usa para saber si hay que actualizar. Ninguna de sus ramas (sistema, programas, releases, desktop, plugins) está probada |
| **`checkDesktop`** | Necesita red (AUR) | **0 %.** **Bug A3 (ALTO)**, verificado con un `pacman` fake: con canal `stable` pregunta por `umbriel-git` y dice "no instalado" |
| **`checkPlugins`** | Necesita el registry y los plugins instalados | **0 %.** El bucle de comparación de versiones por plugin no está probado |
| **`selfUpdate`** | Baja un release de GitHub y reemplaza el binario en ejecución | **0 %.** Toda la rama que reporta "b3l0w vX -> vY" |
| **`updatePlugins`** | Corre `noctalia` y puede clonar el repo con git | **0 %.** **Bug M5 (MEDIO)**: el camino elegido depende de que `plugins.Sources()` no falle, y ese error no se distingue del "no hay sources" |
| **`updateServices` con estados** | Necesita Docker y servicios instalados | 15 %: solo la salida temprana (sin estado). **Bug A2 (ALTO)**: el error de cada servicio se imprime pero no llega al código de salida |
| **La validación del TOML de Noctalia** | `noctalia config validate` no está en el PATH de los tests | **Bug A1 (ALTO)**, verificado: sin el binario, `updateDotfiles` sale con 1 y dice "repo TOML does not validate:" (mensaje vacío). **M3 (MEDIO)**: el TOML de Umbriel no se valida nunca |
| **El snapshot en dry-run** | El fake de `snapper` en los tests no deja marker | **Bug C1 (CRÍTICO)**, verificado con un marker: `--dotfiles --dry-run` **crea una snapshot real**. Ningún test lo detecta porque `fakeBin` (sin marker) no falla si se ejecuta |
| **El prompt con `--self` solo** | `TestConfirmPrompt` prueba `confirmPrompt` directo, no la combinación de flags que produce `Run` | **Bug M1 (MEDIO)**, verificado: el prompt dice "Update b3l0w, system, programs, plugins, services, dotfiles?" cuando solo se actualiza el CLI |
| **El dry-run con versiones comparadas como strings** | El test usa una versión local inyectada | **Bug A4 (ALTO)**: `"0.5.0"` vs `"v0.5.0"` se ven distintos y el dry-run anuncia una actualización inexistente |
| **`Run` con `--system` y `--programs` juntos** | `cmd/b3l0w/update.go:63-66` lo rechaza **antes** de llegar al módulo | Correcto, pero es un invariante que vive en el cmd y no en el paquete: si alguien llama `update.Run` directamente, la comprobación no está |
| **`syncRegistry`** | Descarga de `raw.githubusercontent.com` | 50 %: solo el camino sin kinds. **Bug B2 (BAJO)**: el error de un kind no se distingue del de otro |
| **`updateSystem` con actualizador real** | `cachy-update` es interactivo (news, confirmaciones, kernel) | El camino feliz no está. El README (`README.md:139-140`) lo reconoce |
| **`updateDotfiles` sin `--adopt`** | Escribe en `~/.config` de verdad | 74 %: falta el camino que aplica archivos de verdad y el que resuelve un conflicto con `--force` |
| **El preview de conflictos que falla** | Requiere un estado corrupto | **Bug M2 (MEDIO)**: el `err == nil &&` de `update.go:372` descarta el error del preview |
| **`Run` con todas las capas** | Cada capa necesita su entorno | Solo hay tests con `--programs` y con `--dry-run --programs` |
| **La cancelación** | Se prueba sin terminal, no respondiendo "n" | **Bug B1 (BAJO)**: cancelar sale con 0, igual que completar |
| **La concurrencia** | No hay `t.Parallel()` | Los tests usan `t.Setenv` y capturan `os.Stdout` global: **no pueden** correr en paralelo. Es una restricción real del diseño de los tests, no un bug |
| **La captura de stdout** | `capturarSalida` reemplaza `os.Stdout` globalmente | Si dos tests corrieran en paralelo, sus salidas se mezclarían. Hoy no es un problema |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/update/ -count=1 -cover

# Ver la cobertura por función (cinco funciones aparecen con 0.0 %):
go test ./internal/update/ -count=1 -coverprofile=/tmp/update.cover
go tool cover -func=/tmp/update.cover
```
