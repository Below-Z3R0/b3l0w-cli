# TESTS — internal/update

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 48 |
| Verdes | 48 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **72.3 %** |
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
| `desktopPackages` | 100.0 % | nueva: la selección por canal |
| `versionMasNueva` | 100.0 % | nueva: la comparación numérica |
| `dryRun` | 77.8 % | antes 100 % (el plan de `--self` solo es otro camino) |
| `updateDotfiles` | 86.8 % | antes 81.7 % |
| `updatePrograms` | 86.7 % | |
| `updateServices` | 75.0 % | antes 15 % |
| `syncRegistry` | 70.0 % | antes 50 % |
| `avisoVersionCLI` | 85.7 % | nueva: el aviso de versión del check |
| `Run` | 60.9 % | antes 51.1 % |
| `updateSystem` | 55.6 % | |
| `checkDesktop` | 76.9 % | antes 58.3 % |
| `numerosVersion` | 88.9 % | nueva |
| `capasPedidas` | 100.0 % | nueva: qué capas cubre cada flag |
| `soloSelf` | 100.0 % | nueva: `--self` a secas corta después del CLI |
| `updatePlugins` | 31.6 % | antes 0 % |
| `check` | 50.0 % | antes 0 % |
| `checkPlugins` | 69.2 % | antes 0 % |
| **`selfUpdate`** | **0.0 %** | |

Antes de cerrar A1-A4 eran 31 tests y 50.0 % de cobertura, con cinco
funciones en 0 %. Después del cierre de M1-M6 son 48 tests y 72.3 %: de
las que estaban en 0 % solo queda `selfUpdate`, que baja un release de
GitHub y no se puede testear sin red.

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `update_test.go` | 25 tests: los selectores del prompt y su orden, `defaultDotfilesDir`, `fileExists`, `localVersion` y `compareVersions` (parseo, fallback a 0 sin vercmp o con salida inválida, y semántica real de epoch/pkgrel/vacío), `updatePrograms`, `updateServices`, `updateSystem`, `updateDotfiles`, `desktopUpdate` y el despacho de `Run` |
| `dotfiles_test.go` | 3 tests de regresión de bugs: el dry-run muestra los conflictos y no escribe nada, el dry-run anuncia cuántos conflictos hay, y los huérfanos se avisan |
| `confirm_test.go` | 1 test: la tabla completa de `confirmPrompt`, una fila por combinación de capas |
| `dryrun_test.go` | 3 tests de regresión del bug C1: el dry-run no crea snapshot, sin dry-run sí la crea, y `--adopt --dry-run` no escribe el estado |
| `alto_test.go` | 6 tests de regresión de los hallazgos ALTO (A1-A4), uno por hallazgo y sus contrastes |
| `medio_test.go` | 7 tests de regresión de los hallazgos MEDIO (M1-M6), uno por hallazgo más la caracterización de M4 |


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

### `alto_test.go` (6 tests)

Los tests de los hallazgos ALTO. Cada uno **falla si se revierte su
arreglo**: se comprobó revirtiendo cada arreglo a mano y viendo que el
test correspondiente pasa a rojo.

| Test | Qué fija |
|---|---|
| `TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta` | **A1.** Sin `noctalia` en el PATH, el update de dotfiles no aborta con 1 ni dice "repo TOML does not validate": avisa que el TOML no se validó y sigue. El `PATH` va a un temporal vacío, así que el validador no existe |
| `TestBug_A1_ConNoctaliaElTOMLInvalidoSigueAbortando` | **A1, contraste.** Con un `noctalia` fake que falla, el update sigue abortando con 1 y el mismo mensaje: el arreglo no puede ser "no validar nunca" |
| `TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida` | **A2.** Con un servicio instalado cuyo update falla, `updateServices` devuelve 1 y nombra el servicio que falló. El manifiesto sale del caché local del registry (`XDG_STATE_HOME` temporal) y el update del servicio se simula por el punto de inyección `updateService`: ni red ni Docker |
| `TestBug_A2_RunPropagaElFalloDeServicios` | **A2, la otra mitad.** `Run(--yes --services)` devuelve 1 cuando un servicio falla: el código de `updateServices` ya no se descarta |
| `TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal` | **A3.** El check del escritorio pregunta al AUR por `umbriel`/`noctalia` en canal `stable` y por `umbriel-git`/`noctalia-git` en `edge`. La consulta se intercepta por `aurLookup` y se mira qué paquetes se pidieron |
| `TestBug_A4_ElDryRunComparaLasVersionesComoNumeros` | **A4.** El dry-run solo anuncia una actualización cuando el release es más nuevo: `"v0.5.0"` contra `"0.5.0"` no es una actualización, `"v0.6.0"` contra `"0.5.0"` sí, `"v0.4.0"` contra `"0.5.0"` no (sería un downgrade) y `"v0.10.0"` contra `"0.9.0"` sí. 7 casos |
| `TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes` | **A4, la otra mitad.** El `check` no avisa de una versión que ya se tiene, y sí avisa de una más nueva |

### `medio_test.go` (7 tests)

Los tests de los hallazgos MEDIO. Cada uno **falla si se revierte su
arreglo**: se comprobó revirtiendo cada arreglo a mano y viendo que el
test correspondiente pasa a rojo.

| Test | Qué fija |
|---|---|
| `TestM1_CapasPedidasSelfNoArrastraLasDemas` | **M1.** `--self` cuenta como selector: solo se quiere la capa del CLI y el prompt es "Update b3l0w? [Y/n] ", no la lista de las seis capas. `--self --system` quiere las dos, y sin flags se quieren todas. También fija que `--self` a secas corta después del CLI (`soloSelf`) |
| `TestM2_ElPreviewQueFallaAbortaAntesDelPull` | **M2.** Con el estado de dotfiles corrupto, el preview no se puede hacer: el update aborta con 1 y avisa que no se pudo previsualizar. Antes seguía al pull real, que fallaba con el mismo error y sin mostrar los conflictos |
| `TestM3_ElTOMLDeUmbrielSeValida` | **M3.** Con `umbriel/config.toml` inválido y el validador fake en el PATH, el update aborta con 1 antes de aplicar nada. Antes solo se validaba el TOML de Noctalia |
| `TestM4_ElEstadoSeLeeDelHome` | **M4, caracterización.** El estado de dotfiles se escribe y se lee de `$HOME`. Este test **pasa también con el arreglo revertido**: en Linux `os.UserHomeDir()` ya es `$HOME`, así que fija el comportamiento, no el cambio |
| `TestM4_SinHomeElErrorDiceQueFalta` | **M4, la otra mitad.** Sin `$HOME` el update aborta con 1 y el error dice qué variable falta, en vez del crudo de Go ("$HOME is not defined") |
| `TestM5_UnSourceQueNoSeLeeEsUnError` | **M5.** Con Noctalia en el PATH y los sources ilegibles, `updatePlugins` aborta con 1 y nombra el error. Antes caía en silencio al camino que actualiza otra lista de plugins (el test lo ve: sin el arreglo salía con 0 y listaba los plugins del manifiesto) |
| `TestM6_SinAurElCheckFalla` | **M6.** Si el AUR no contesta, `check` sale con 1: sin red no puede decir que todo está al día. El AUR, las releases y el manifiesto de plugins entran por fakes: ni red ni máquina |

### `dryrun_test.go` (3 tests)

| Test | Qué fija |
|---|---|
| `TestDryRunNoCreaSnapshot` | **C1.** `--dotfiles --dry-run` no llama a `snapper`: el log del fake tiene que quedar vacío |
| `TestSinDryRunSiCreaSnapshot` | **C1, contraste.** Sin dry-run la snapshot se pide: si este test falla, el anterior pasaría por la razón equivocada |
| `TestDryRunNoAdopta` | `--adopt --dry-run` no escribe el estado de dotfiles |

## Qué quedó sin cubierto (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **`check` completo** | Necesita `system.List` (cachy-update), la API de GitHub (releases sin firma) y el registry de plugins | **0 % de cobertura.** Es `b3l0w update --check`, el comando que el usuario usa para saber si hay que actualizar. Ninguna de sus ramas (sistema, programas, releases, plugins) está probada |
| **`checkDesktop`** | Necesitaba red (AUR) | **58.3 %.** **Bug A3 (ALTO) cerrado**: el check pregunta por los paquetes del canal. Lo que no está probado es el caso "no se pudo consultar AUR" |
| **`checkPlugins`** | Necesita el registry y los plugins instalados | **0 %.** El bucle de comparación de versiones por plugin no está probado |
| **`selfUpdate`** | Baja un release de GitHub y reemplaza el binario en ejecución | **0 %.** Toda la rama que reporta "b3l0w vX -> vY" |
| **`updatePlugins`** | Corre `noctalia` y puede clonar el repo con git | **0 %.** **Bug M5 (MEDIO)**: el camino elegido depende de que `plugins.Sources()` no falle, y ese error no se distingue del "no hay sources" |
| **`updateServices` con estados** | El estado se arma en un temporal, pero el update real necesita el registry de imágenes | 75 %: la salida temprana (sin estado), el caso "el manifiesto no carga" y el caso "el servicio falla". **Bug A2 (ALTO) cerrado**: el error de cada servicio llega al código de salida |
| **La validación del TOML de Noctalia** | `noctalia config validate` no está en el PATH de los tests | **Bug A1 (ALTO) cerrado**: sin el binario, `updateDotfiles` avisa y sigue en vez de salir con 1. **M3 (MEDIO)**: el TOML de Umbriel no se valida nunca |
| **El snapshot en dry-run** | El fake de `snapper` en los tests no deja marker | **Bug C1 (CRÍTICO) cerrado**: `dryrun_test.go` mete un `snapper` que loguea y verifica que el log queda vacío |
| **El prompt con `--self` solo** | `TestConfirmPrompt` prueba `confirmPrompt` directo, no la combinación de flags que produce `Run` | **Bug M1 (MEDIO)**, verificado: el prompt dice "Update b3l0w, system, programs, plugins, services, dotfiles?" cuando solo se actualiza el CLI |
| **El dry-run con versiones comparadas como strings** | La versión local se inyecta con `version.Version` | **Bug A4 (ALTO) cerrado**: `alto_test.go` prueba 7 combinaciones de versiones locales y remotas |
| **`Run` con `--system` y `--programs` juntos** | `cmd/b3l0w/update.go:63-66` lo rechaza **antes** de llegar al módulo | Correcto, pero es un invariante que vive en el cmd y no en el paquete: si alguien llama `update.Run` directamente, la comprobación no está |
| **`syncRegistry`** | Descarga de `raw.githubusercontent.com` | 70 %: el camino sin kinds y el de un kind. **Bug B2 (BAJO)**: el error de un kind no se distingue del de otro |
| **`updateSystem` con actualizador real** | `cachy-update` es interactivo (news, confirmaciones, kernel) | El camino feliz no está. El README (`README.md:139-140`) lo reconoce |
| **`updateDotfiles` sin `--adopt`** | Escribe en `~/.config` de verdad | 81.7 %: falta el camino que aplica archivos de verdad y el que resuelve un conflicto con `--force` |
| **El preview de conflictos que falla** | Requiere un estado corrupto | **Bug M2 (MEDIO)**: el `err == nil &&` de `update.go:372` descarta el error del preview |
| **`Run` con todas las capas** | Cada capa necesita su entorno | Solo hay tests con `--programs` y con `--dry-run --programs` |
| **La cancelación** | Se prueba sin terminal, no respondiendo "n" | **Bug B1 (BAJO)**: cancelar sale con 0, igual que completar |
| **La concurrencia** | No hay `t.Parallel()` | Los tests usan `t.Setenv`, capturan `os.Stdout` global y ahora sustituyen variables de paquete (`selfCheck`, `aurLookup`, `registrySync`, `updateService`): **no pueden** correr en paralelo. Es una restricción real del diseño de los tests, no un bug |
| **La captura de stdout** | `capturarSalida` reemplaza `os.Stdout` globalmente | Si dos tests corrieran en paralelo, sus salidas se mezclarían. Hoy no es un problema |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/update/ -count=1 -cover

# Ver la cobertura por función (cinco funciones aparecen con 0.0 %):
go test ./internal/update/ -count=1 -coverprofile=/tmp/update.cover
go tool cover -func=/tmp/update.cover
```