# Tests — snapshots

Tests de `internal/snapshots`: los dos parsers, la poda y el reporte.

## Cómo correrlos

```bash
go test ./internal/snapshots/ -v
```

Desde la raíz del worktree (`branches/snapshots`).

Fuzzing (cada target corre ~2 minutos):

```bash
go test ./internal/snapshots/ -run FuzzParseEntries -fuzz FuzzParseEntries -fuzztime 120s
go test ./internal/snapshots/ -run FuzzParseIDs   -fuzz FuzzParseIDs   -fuzztime 120s
go test ./internal/snapshots/ -run FuzzPruneKeep   -fuzz FuzzPruneKeep   -fuzztime 120s
```

## Archivos

| Archivo | Qué cubre |
|---|---|
| `parse_edge_test.go` | `parseEntries` y `parseIDs` sobre salidas reales y casos borde. |
| `prune_test.go` | `Entries`, `IDs` y `Prune` contra una tool falsa en el PATH. |
| `report_test.go` | `Report`: con `--yes`, interactivo, y sin terminal. |
| `fuzz_test.go` | Fuzzing de los dos parsers y de la poda. |

`parse_test.go` (preexistente) queda como estaba.

## Cómo se falsea la tool

`limine.go` llama a la tool por nombre, así que alcanza con poner binarios
falsos adelante en el `PATH` (`fakeTools` en `prune_test.go`):

- `limine-snapper-info` imprime `$FAKE_INFO`
- `limine-snapper-list` imprime `$FAKE_LIST`
- `sudo` loguea sus argumentos en `$FAKE_LOG` y puede fallar
  (`FAKE_SUDO_FAIL_ON`, `FAKE_SUDO_EXIT`)

Así se puede ver qué se le pide a `limine-snapper-remove` y a
`limine-snapper-sync` sin tocar el sistema ni el código de producción.

Para la terminal, `ttyStdin` (`report_test.go`) mete un pipe en el fd 0 y
pone `/dev/null` como `os.Stdin`: el bufio del paquete `cli` sigue
leyendo del fd 0 (el pipe) mientras `cli.IsTTY()` ve un char device.

## Casos

### `parseEntries` — `parse_edge_test.go`

| Test | Qué prueba | Entrada | Salida esperada |
|---|---|---|---|
| `TestParseEntries_Tabla` | Tabla de 27 casos: separadores, sufijos y errores | ver tabla en el archivo | `n` y error según el caso |
| `real sin color` | Salida real de `limine-snapper-info` sin color | `realInfoPlain` | `0`, sin error |
| `real con color` | La misma salida con ANSI | `realInfoColor` | `0`, sin error — OK (bug 1) |
| `solo la linea de conteo` | Línea aislada | `Snapshot entries  : 39 (max: auto)` | `39` |
| `conteo cero` | Sin entradas | `Snapshot entries  : 0 (max 0)` | `0` |
| `tabuladores` | Separadores con tab | `Snapshot entries\t:\t39` | `39` |
| `sin espacio antes de dos puntos` | Formato compacto | `Snapshot entries: 39` | `39` |
| `sin espacio despues de dos puntos` | Sin espacio tras `:` | `Snapshot entries  :39` | `39` |
| `espacios alrededor` | Padding | `   Snapshot entries   :   39   ` | `39` |
| `crlf` | Fin de línea Windows | `Snapshot entries  : 39\r` | `39` |
| `crlf en medio` | CRLF en línea posterior | `...\r\nBoot usage : 1%` | `39` |
| `lineas en blanco alrededor` | Separadores vacíos | `\n\nSnapshot entries  : 39\n\n` | `39` |
| `sufijo max` | Sufijo real | `... : 39 (max: auto)` | `39` |
| `sufijo max sin dos puntos` | Sufijo real sin `:` | `... : 39 (max 0)` | `39` |
| `sufijo max auto` | Sufijo sin paréntesis | `... : 39 (max auto)` | `39` |
| `vacio` | Sin salida | `""` | error |
| `solo espacios` | Ruido | `"   \n\t\n  "` | error |
| `sin la linea` | Otra clave | `Version : 1.32.1\n...` | error |
| `solo encabezado sin dos puntos` | Sin `:` | `Snapshot entries` | error |
| `encabezado con dos puntos sin valor` | Valor vacío | `Snapshot entries  : ` | error |
| `valor no numerico` | Texto | `... : abc` | error |
| `valor decimal` | Decimal | `... : 39.5` | error |
| `valor gigante` | Overflow de int64 | `... : 99999999999999999999` | error |
| `valor mayor que int64` | Overflow exacto | `... : 9223372036854775808` | error |
| `digitos unicode` | Dígitos árabes | `... : ٣٩` | error |
| `otra clave` | `Snapshot files` | `Snapshot files    : 0 (0.0 MiB)` | error |
| `clave con el mismo prefijo` | Prefijo laxo (fragilidad) | `Snapshot entriesTotal : 39` | `39` |
| `TestParseEntries_SalidaRealConColor` | El bug ANSI, aislado | `realInfoColor` | parsea — OK (bug 1) |
| `TestParseEntries_ConteoNegativo` | Conteo negativo | `... : -5` | error — OK (bug 2) |
| `TestParseEntries_PrimeraLineaGana` | Conteo duplicado: gana el primero | dos líneas `Snapshot entries` | `1` |
| `TestParseEntries_NoMatcheaConsecuencia` | Prefijo laxo documentado | `Snapshot entries max : 40` | `40` |

### `parseIDs` — `parse_edge_test.go`

| Test | Qué prueba | Entrada | Salida esperada |
|---|---|---|---|
| `TestParseIDs_Tabla` | Tabla de 23 casos | ver tabla en el archivo | lista según el caso |
| `real vacia` | Tabla real sin entradas | `realListEmpty` | `nil` |
| `real con filas` | Tabla real con entradas | `realListRows` | `[350 453 454]` |
| `solo encabezado` | Solo la cabecera | ` ID │ Date │ Description ` | `nil` |
| `encabezado y separador` | Cabecera + separador | tabla real vacía | `nil` |
| `separador con espacios` | Separador de guiones largos | `─────┼───...` | `nil` |
| `tabla del repo` | Formato del README | tabla con 3 filas | `[350 453 454]` |
| `tabuladores` | Separador con tab | `350\t2026-10-19\tx` | `[350]` |
| `crlf` | Fin de línea Windows | ` 350 │ ... │ x\r` | `[350]` |
| `descripcion vacia` | Fila sin descripción | ` 350 │ ... │` | `[350]` |
| `descripcion numerica` | La descripción es un número | ` 350 │ ... │ 99` | `[350]` |
| `ids desordenados` | Orden de entrada invertido | `454` y `350` | `[350 454]` |
| `lineas en blanco` | Filas vacías | `"\n\n 350 │ a │ b\n\n"` | `[350]` |
| `vacio` | Sin salida | `""` | `nil` |
| `solo espacios` | Ruido | `"   \n\t\n"` | `nil` |
| `un solo campo` | Fila con una sola columna | `" 350"` | `nil` |
| `sin espacio tras la barra` | Separador pegado | `" 350│2026-10-19│x"` | `nil` |
| `id no numerico` | ID de texto | `" abc │ ..."` | `nil` |
| `id decimal` | ID decimal | `" 35.0 │ ..."` | `nil` |
| `id gigante` | Overflow de int64 | `" 99999999999999999999 │ x │ y"` | `nil` |
| `digitos unicode` | Dígitos árabes | `" ٣٥٠ │ ..."` | `nil` |
| `TestParseIDs_IDNegativo` | ID negativo | `" -5 │ ..."` | `nil` — OK (bug 3) |
| `TestParseIDs_Duplicados` | Fila repetida | dos filas con ID 350 | `[350]` — OK (bug 4) |
| `TestParseIDs_Ordenado` | Orden ascendente | IDs desordenados | `[350 453 454]` |
| `TestParseIDs_NoTocaLasFilasVecinas` | La cabecera no se cuela | cabecera + 1 fila | `[350]` |
| `TestParseEntriesYParseIDs_MismaSalidaReal` | Los dos parsers con datos reales | salidas reales | `0` y 3 IDs |
| `TestParseIDs_NoDevuelveLaFilaDeBoot` | `Last snapshot : ID: 558` no da IDs | `realInfoPlain` | `nil` |

### `Entries` / `IDs` — `prune_test.go`

| Test | Qué prueba | Escenario | Salida esperada |
|---|---|---|---|
| `TestEntries_LeeElConteo` | Lee el conteo | tool con `39 (max 0)` | `39` |
| `TestEntries_SinLaTool` | La tool no está | PATH vacío | error |
| `TestIDs_LeeLaTabla` | Lee la tabla | 3 filas | `[350 453 454]` |

### `Prune` — `prune_test.go`

| Test | Qué prueba | Escenario | Salida esperada |
|---|---|---|---|
| `TestPrune_BorraLasViejas` | Borra las viejas y luego sincroniza | 3 IDs, keep 2 | 1 borrada; `remove 350` + `sync` |
| `TestPrune_UnSoloRemoveConTodosLosIDs` | Un solo `remove` con todos los IDs | 3 IDs, keep 1 | `remove 350 453` |
| `TestPrune_KeepMayorQueLaCantidad` | keep mayor que la lista | 3 IDs, keep 10 | 0 borradas, sin `sudo` |
| `TestPrune_KeepIgualALaCantidad` | keep igual a la lista | 3 IDs, keep 3 | 0 borradas, sin `sudo` |
| `TestPrune_KeepCero` | keep 0 borra todo | 3 IDs, keep 0 | 3 borradas |
| `TestPrune_KeepNegativo` | keep negativo | 3 IDs, keep -1 | nada — OK (bug 5) |
| `TestPrune_KeepNegativoConPocaCapacidad` | keep negativo con slice chico | 1 ID, keep -1 | sin panic — OK (bug 5) |
| `TestPrune_ListaVacia` | Lista vacía | 0 filas | 0 borradas, sin `sudo` |
| `TestPrune_ListaConUnSoloID` | Un solo ID, keep 1 | 1 fila | 0 borradas, sin `sudo` |
| `TestPrune_UnSoloIDYKeepCero` | Un solo ID, keep 0 | 1 fila | 1 borrada: `remove 350` |
| `TestPrune_ErrorEnRemove` | El remove falla | `sudo` falla | error con la salida del comando |
| `TestPrune_ErrorEnSync` | El sync falla | `sudo` falla en sync | error que empieza con `sync:` |
| `TestPrune_NoHaceSyncSiElRemoveFalla` | No sincroniza si no se borró | `sudo` falla | sin llamada a `sync` |
| `TestPrune_SinLaToolDeListado` | La tool de listado no está | PATH vacío | error |

### `Report` — `report_test.go`

| Test | Qué prueba | Escenario | Salida esperada |
|---|---|---|---|
| `TestReport_ConYes_Poda` | `--yes` poda y reporta | 39 entradas, keep 5 | 0; "5 entradas borradas"; `remove` con 5 IDs + `sync` |
| `TestReport_ConYes_KeepCeroSeNormaliza` | keep 0 → 20 | 30 IDs | borra 10 |
| `TestReport_ConYes_KeepNegativoSeNormaliza` | keep -5 → 20 | 30 IDs | borra 10 |
| `TestReport_ConYes_TotalMenorQueKeepNoPoda` | No hay nada que podar | 0 entradas | 0, sin `sudo` |
| `TestReport_ConYes_FallaElConteoNoFrena` | La tool de conteo no está | PATH vacío | 0 + aviso |
| `TestReport_ConYes_FallaLaPodaDevuelveUno` | La poda falla | `sudo` falla | 1 + error por stderr |
| `TestReport_ConYes_FallaElSyncDevuelveUno` | El sync falla | `sudo` falla en sync | 1 + `sync:` |
| `TestReport_ConYes_SinLaToolDeListado` | El listado no está | solo `info` | 1 |
| `TestReport_ElMensajeDeQuedanMiente` | El cierre dice `keep`, no el total real | 39 totales, keep 5 | "quedan 34" — OK (bug 6) |
| `TestReport_ConDiscrepanciaEntreConteoYLista` | Conteo y lista no coinciden | info 39, lista 10, keep 20 | "quedan 39" — OK (bug 7) |
| `TestReport_Interactivo` | 4 escenarios interactivos en orden | "y"+5, "c", "y"+"c", "n" | poda con 5; los otros 3 cancelan sin tocar `sudo` |
| `TestReport_SinTerminalCancela` | Sin terminal no poda | stdin a archivo | 0 + "cancelled", sin `sudo` |

### Fuzzing — `fuzz_test.go`

| Test | Qué prueba | Invariantes |
|---|---|---|
| `FuzzParseEntries` | `parseEntries` con entradas arbitrarias | no entra en panic; el error es siempre el de "no pude leer"; el número devuelto aparece en la entrada |
| `FuzzParseIDs` | `parseIDs` con entradas arbitrarias | no entra en panic; la lista viene ordenada; cada ID viene de alguna fila; no hay más IDs que filas |
| `FuzzPruneKeep` | `Prune` con cualquier `keep >= 0` y cualquier lista | no entra en panic; no borra más de lo que hay; el conteo de borradas es coherente |

Resultados de las campañas:

| Target | Ejecuciones | Resultado |
|---|---|---|
| `FuzzParseEntries` | 19.394.630 | PASS, sin panics nuevos |
| `FuzzParseIDs` | 11.690.211 | PASS, sin panics nuevos |
| `FuzzPruneKeep` | 58.589 | PASS, sin panics nuevos |

El fuzzer de `parseEntries` encontró por su cuenta el conteo negativo
(`Snapshot entries:-1`), que quedó fijado de forma determinística en
`TestParseEntries_ConteoNegativo`. Ese chequeo se sacó del fuzz target
para que la campaña no cortara en el primer input negativo.

## Bugs encontrados

Los nueve tests que documentaban los bugs fallaban a propósito: fijaban
el comportamiento esperado sin tocar el código de producción. Los siete
bugs ya están arreglados (ver "Arreglado" en cada uno) y los nueve tests
pasan.

### Bug 1 — Los códigos ANSI rompen `parseEntries` (crítico)

`limine-snapper-info` colorea la salida cuando el proceso hereda un `TERM`
con color, que es el caso normal en una terminal interactiva:

```
\x1b[1;33mSnapshot entries  : \x1b[0m0 (max 0)
```

`parseEntries` busca el prefijo `Snapshot entries` sobre la línea sin
limpiar, así que no lo encuentra y devuelve
`no pude leer 'Snapshot entries'`.

Verificado contra el binario real:

| `TERM` | `parseEntries` |
|---|---|
| (sin `TERM`) | `n=0`, sin error |
| `dumb` | `n=0`, sin error |
| `xterm-256color` | **error** |
| `screen` | **error** |

Impacto: en cualquier terminal normal `Entries()` falla, `Report()`
avisa "snapshots de Limine: no pude leer 'Snapshot entries'" y **nunca
poda**. El módulo queda muerto en su caso de uso principal. El CLI
captura la salida con `exec.Command`, que hereda `TERM` del entorno.

Fix esperado: quitar los códigos ANSI antes de parsear (o no depender del
prefijo exacto de la etiqueta).

Tests: `TestParseEntries_Tabla/real_con_color`,
`TestParseEntries_SalidaRealConColor`.

**Arreglado:** `parseEntries` limpia las secuencias ANSI antes de parsear
(`ansiRe` en `limine.go`). Verificado contra el binario real: con
`TERM=xterm-256color` la salida trae 11 secuencias ANSI y ahora devuelve
`n=0` sin error, igual que con `TERM=dumb`.

### Bug 2 — `parseEntries` acepta un conteo negativo

`Snapshot entries  : -5` devuelve `-5` sin error. `Report` compara
`-5 <= keep` y da por hecho que no hay entradas.

Fix esperado: un conteo negativo es un error de parseo.

Test: `TestParseEntries_ConteoNegativo`.

**Arreglado:** `parseEntries` ignora un conteo negativo y termina devolviendo
el error de siempre (`no pude leer 'Snapshot entries'`). Se conserva ese
mismo mensaje a propósito: el fuzz target exige que todo error de
`parseEntries` sea ese.

### Bug 3 — `parseIDs` acepta IDs negativos

` -5 │ ... ` produce la lista `[-5]`. `Prune` la pasa tal cual a
`limine-snapper-remove`, y la tool aborta el lote entero si un ID no
existe: un solo ID basura tumba la poda completa.

Fix esperado: descartar IDs `<= 0` como se descartan las filas no
numéricas.

Test: `TestParseIDs_IDNegativo`.

**Arreglado:** `parseIDs` descarta los IDs `<= 0`.

### Bug 4 — `parseIDs` no deduplica

Un ID repetido queda repetido en la lista. `Prune` lo pasa dos veces a
`limine-snapper-remove` y cuenta las entradas borradas dos veces.

Fix esperado: deduplicar.

Test: `TestParseIDs_Duplicados`.

**Arreglado:** `parseIDs` deduplica con un `map` de IDs vistos.

### Bug 5 — `Prune` con `keep` negativo (crítico)

`Prune` hace `ids[:len(ids)-keep]`. Con `keep` negativo:

- si la capacidad del slice alcanza, mete un **ID 0 inventado** en el
  lote y borra de más;
- si no alcanza, entra en **panic** (`slice bounds out of range`) y tira
  el proceso.

Medido:

| IDs | `keep` | Resultado |
|---|---|---|
| 3 | `-1` | borra `[350 453 454 0]` (el 0 es inventado) |
| 3 | `-2` | panic |
| 1 | `-1` | panic |

`Report` normaliza `keep <= 0` a 20, así que el CLI no llega con un keep
negativo (`main.go` además exige `n > 0`). Pero `Prune` es API pública —
el README publica su firma — y un caller que pase un keep negativo
destruye entradas de arranque o revienta el binario.

Fix esperado: `keep <= 0` es un no-op o un error, nunca un índice
negativo.

Tests: `TestPrune_KeepNegativo`, `TestPrune_KeepNegativoConPocaCapacidad`.

**Arreglado:** `Prune` rechaza `keep < 0` con un error antes de tocar nada.
`keep == 0` sigue significando "borrar todo" (lo fija
`TestPrune_KeepCero`).

### Bug 6 — El mensaje de cierre dice un número falso

`Report` cierra con `"%d entradas borradas; quedan %d"` usando `keep`
como "quedan". El número real de entradas que quedan es el total menos
las borradas.

Con 39 entradas totales y `keep 5`, quedan 34 y el mensaje dice
"quedan 5". El usuario cree que le quedan 5 entradas de arranque cuando
le quedan 34, y deja de limpiar.

Fix esperado: informar el total real (`total - borradas`).

Test: `TestReport_ElMensajeDeQuedanMiente`.

**Arreglado:** el cierre dice `"%d entradas borradas; quedan %d"` con
`total - removed` en vez de `keep`.

### Bug 7 — El conteo y la lista pueden no coincidir (crítico)

El conteo viene de `limine-snapper-info` y la lista de
`limine-snapper-list`. Son comandos distintos y el README ya lo admite
("no es necesariamente el mismo conjunto"), pero el módulo no lo
contempla.

Con `info` diciendo 39 y la lista teniendo 10 filas, `keep 20`:

- `Report` ve `39 > 20` y pregunta "¿Reducir de 39 a 20?";
- el usuario dice que sí;
- `Prune` ve `len(ids)=10 <= 20` y **no borra nada**;
- el mensaje dice "0 entradas borradas; quedan 20".

Impacto: el usuario confirma una reducción y no ocurre nada, sin aviso.

Fix esperado: usar una sola fuente de verdad, o avisar cuando el conteo
y la lista no coinciden.

Test: `TestReport_ConDiscrepanciaEntreConteoYLista`.

**Arreglado:** el cierre dice la verdad (`total - borradas`) y, cuando el
conteo decía que sobraban entradas pero no se borró ninguna, avisa que la
lista no las tenía todas. La lista sigue siendo la fuente de verdad de
*qué* se borra; el conteo, de *cuántas* hay.

## Qué NO está cubierto

- **Los comandos reales.** Todo corre contra binarios falsos en el `PATH`.
  El formato de `limine-snapper-remove` (rangos `1 3 5..7`), qué hace la
  tool con un ID inexistente y si aborta el lote no se probó contra el
  binario real: haría falta root y tocaría la ESP.
- **El orden "ID más bajo = más viejo".** El módulo asume que Limine
  numera las entradas de forma creciente y que el ID más bajo es la más
  vieja. El README lo marca como "verificar" y no se puede probar sin
  crear entradas de verdad.
- **El watcher del sistema.** `limine-snapper-sync.service` puede volver
  a agregar entradas después de la poda. Fuera del alcance del módulo.
- **`sudo` sin terminal.** `Prune` llama a `sudo` con `exec.Command`, que
  no conecta stdin: sin `NOPASSWD` o credencial cacheada falla. El
  README lo anota; acá el `sudo` falso siempre funciona.
- **Locale.** El catálogo de gettext de `limine-snapper-sync` está
  traducido al ruso, pero en esta máquina el locale no está generado y
  la salida sale en inglés. Si el label "Snapshot entries" se tradujera,
  `parseEntries` dejaría de encontrarlo: mismo bug que el 1, otra causa.
- **El `keep` que escribe el usuario.** `cli.AskInt` devuelve el default
  ante una respuesta inválida, así que "abc" deja el `keep` original.
  Eso es del paquete `cli`, no de este módulo.
- **La carrera entre el conteo y la poda.** Entre `Entries()` y `Prune()`
  el sistema puede cambiar. No se probó.

## Estado

Verificado con `go test ./internal/snapshots/ -count=1 -v`:

| Métrica | Valor |
|---|---|
| Casos totales reportados | 125 (47 funciones de test + 78 subcasos de tabla) |
| Casos que pasan | 125 |
| Casos que fallan | 0 |
| Bugs reales encontrados | 7 (los 7 arreglados) |
| Bugs en código de producción | 7 (arreglados en `limine.go` y `report.go`) |
| Fuzz targets | 3 (19,4M + 11,7M + 58.589 ejecuciones, sin panics nuevos) |

Los 9 tests que documentaban los bugs siguen intactos: son la
especificación. Los que fallaban ahora pasan.

Único cambio en un archivo de test: el helper `entriesHasNumber` del fuzz
target `FuzzParseEntries` reimplementaba el match del label sin limpiar
ANSI, así que reportaba un falso fallo sobre la salida real coloreada
("n=0 no aparece en la entrada"). Se le agregó la misma limpieza que
hace el parser; el invariante no se relajó.

El conteo incluye los 3 tests preexistentes de `parse_test.go` y los 3
fuzz targets (que corren su corpus semilla en modo normal).
