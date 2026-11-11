# Snapshots de Limine

## Qué es y por qué existe

Cada snapshot de snapper genera una entrada de arranque en Limine (kernel e
initramfs copiados a la partición ESP). `limine-snapper-sync` las crea
solas, así que la partición boot se llena sin que nadie las mire. Este
módulo cuenta cuántas hay y borra las más viejas cuando superan un límite,
dejando el menu de arranque en un estado manejable.

## Qué NO hace

- No crea ni borra snapshots de snapper: eso es `internal/snapshot` (crea) y
  el propio `snapper` (borra). Acá solo se tocan entradas de arranque.
- No edita `/boot/efi/limine.conf` a mano ni reescribe la ESP: la
  escriben `limine-snapper-remove` y `limine-snapper-sync`.
- No instala ni actualiza Limine, snapper o el paquete `limine-snapper-sync`:
  eso es `internal/system` y `internal/systemdef`.
- No restaura snapshots ni decide qué kernel entra al menu: eso es
  `limine-snapper-restore` y la config de Limine.

## Features

### Conteo de entradas de arranque

- **Entrada:** stdout de `limine-snapper-info` (línea `Snapshot entries : N`).
- **Salida:** `int` con la cantidad de entradas de snapshot que Limine tiene.
- **Errores:** `no pude leer 'Snapshot entries'` si la línea no aparece, el
  número no parsea o es negativo; el error de `cli.Run` si el comando no
  existe o falla.
- **Salida coloreada:** la tool colorea el output cuando hereda un `TERM`
  con color (o sea, en cualquier terminal interactivo). Las secuencias ANSI
  se limpian antes de parsear.

### Listado de IDs

- **Entrada:** stdout de `limine-snapper-list` (tabla `ID │ Date │ Description`).
- **Salida:** `[]int` con los IDs de entrada, ordenados de menor a mayor.
- **Errores:** solo los de `cli.Run`. Las filas que no son numéricas
  (encabezado, separador de tabla) se descartan sin error, y también los
  IDs inválidos (`<= 0`) y los repetidos.

### Poda

- **Entrada:** `keep int` (cuántas entradas conservar) y la lista de IDs.
- **Salida:** `int` con cuántas entradas se borraron.
- **Errores:** envuelve la salida de los comandos externos:
  `"<err>: <out>"` para el remove y `"sync: <err>: <out>"` para el sync.
  Un `keep` negativo es un error: nunca se toca la ESP.

### Reporte interactivo

- **Entrada:** `keep int` (≤ 0 se normaliza a 20) y `yes bool` (no preguntar).
- **Salida:** `int` con el código de salida del comando (0 si está todo bien o
  el usuario canceló, 1 si la poda falló). Los mensajes salen por stdout/stderr
  con `cli.Ok`, `cli.Warn` y `cli.Fail`.
- **Errores:** si el conteo falla, avisa y devuelve 0 sin frenar el update; si
  la poda falla, avisa por stderr y devuelve 1.
- **Cierre:** el mensaje dice cuántas entradas quedan de verdad
  (`total - borradas`), no el `keep` pedido. Si el conteo decía que sobraban
  entradas pero no se borró ninguna, avisa que la lista no las tenía todas.

## Cómo lo consume otro módulo

```go
// firma exacta
func Report(keep int, yes bool) int
func Entries() (int, error)
func IDs() ([]int, error)
func Prune(keep int) (int, error)
```

- `Report` es la entrada de los comandos. La usan `cmd/b3l0w/main.go`
  (`checkCommand` → `b3l0w check [--keep=N] [--yes]`) y
  `internal/update/update.go` (`Run` → `b3l0w update [--keep=N] [--yes]`).
- Las otras tres son la parte sin UI: `update.go` y `main.go` solo llaman a
  `Report`.
- Devuelven el error de `cli.Run` tal cual; `Report` es la que decide qué
  hacer con él (aviso, no abortar).
- No tienen estado propio: no hay variables globales, archivos ni caches. Todo
  el estado vive en Limine, en la ESP y en los comandos externos. Por eso los
  `parse*` son funciones puras y se testean sin binarios instalados.

Ejemplos:

```bash
b3l0w check --keep=10     # pregunta si reducir a 10
b3l0w check --yes         # reduce sin preguntar
b3l0w update --keep=10    # lo mismo, al final del update
```

## Archivos

| Archivo | Qué hace |
|---|---|
| `limine.go` | Lectura de `limine-snapper-info`/`-list` y poda con `-remove` + `-sync`. |
| `report.go` | `Report`: cuenta, pregunta el límite y llama a `Prune`. |
| `parse_test.go` | Tests de los dos parsers sobre salidas reales de los scripts. |

## Decisiones

### Contar y borrar con los scripts de `limine-snapper-sync`

- **Por qué:** `limine.conf` es un formato de otro proyecto y la tool maneja
  la ESP, los archivos de kernel, los hooks y el lock de la partición boot.
- **Alternativa descartada:** parsear y editar `/boot/efi/limine.conf` a mano.
- **Costo:** dependencia de cuatro binarios externos (paquete
  `limine-snapper-sync`) y de su formato de salida en texto. Un cambio de
  formato o de idioma rompe los parsers.

### "Viejo" = ID más bajo

- **Por qué:** Limine numera las entradas de forma creciente y la salida de
  `limine-snapper-list` no trae un timestamp usable para ordenar (la fecha es
  texto libre y la zona horaria no viene).
- **Alternativa descartada:** parsear la columna `Date` para comparar
  antigüedades.
- **Costo:** si Limine reinicia la numeración de IDs, el orden por antigüedad
  se rompe (verificar).

### Un solo `limine-snapper-remove` con todos los IDs viejos

- **Por qué:** la herramienta acepta varios IDs o rangos (`1 3 5..7`), así que
  una llamada toma el lock de la partición boot una sola vez y reescribe la
  config una sola vez.
- **Alternativa descartada:** un `remove` por ID, en un loop.
- **Costo:** es un lote. Si un ID no existe, el comando puede fallar entero y
  no se borra nada (verificar el comportamiento exacto con IDs inválidos).

### `sudo` explícito y `limine-snapper-sync` al final

- **Por qué:** `remove` deja `limine.conf` desactualizado; `sync` regenera las
  entradas que quedan. Y `remove`/`sync` escriben en la ESP, así que corren
  como root.
- **Alternativa descartada:** confiar en `limine-snapper-sync.service` (el
  watcher del sistema) para que sincronice solo.
- **Costo:** `sudo` se invoca sin terminal (`exec.Command` no conecta stdin),
  así que necesita `NOPASSWD` o una credencial cacheada. El watcher del
  sistema puede volver a agregar entradas después (verificar).

### Límite propio en vez de leer la config del sistema

- **Por qué:** el CLI no debería depender del archivo de configuración de otro
  paquete (`/etc/limine-snapper-sync.conf`, con `MAX_SNAPSHOT_ENTRIES` y
  `LIMIT_USAGE_PERCENT`). El límite es efímero: aplica a esa corrida.
- **Alternativa descartada:** editar `MAX_SNAPSHOT_ENTRIES` y dejar que la
  tool aplique el límite.
- **Costo:** conviven dos mecanismos de poda. El de la tool puede volver a
  agregar entradas y el conteo de `Entries()` (entradas en Limine) no es
  necesariamente el mismo conjunto que la lista de `IDs()`.

### `keep <= 0` significa "cancelado"

- **Por qué:** `cli.AskInt` devuelve -1 cuando el usuario escribe `c`, y el
  Report trata cualquier `keep <= 0` como cancelación, igual que hace
  `cli.Confirm` con "c".
- **Alternativa descartada:** un prompt aparte con "conservar 0" como opción
  válida.
- **Costo:** no hay forma de pedir que queden cero entradas: se interpreta
  como cancelación.

## Instalación y actualización

Este módulo no se instala por separado: es parte del binario `b3l0w`
(paquete `b3l0w-cli`, `/usr/bin/b3l0w`). Se actualiza con `b3l0w update
--self` o con pacman si el binario pertenece al paquete.

Del lado del sistema necesita el paquete `limine-snapper-sync`
(`cachyos/limine-snapper-sync`), que provee `limine-snapper-info`,
`limine-snapper-list`, `limine-snapper-remove` y `limine-snapper-sync`. Está
declarado en el perfil `desktop` de `internal/systemdef/system.json` (lista
`repo` y servicio `limine-snapper-sync.service`).

Si `limine-snapper-info` no está o no se puede leer, `Report` avisa y
devuelve 0: la poda de snapshots no frena el resto del update.
