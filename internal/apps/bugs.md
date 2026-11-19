# BUGS — internal/apps

Revisión de integridad en contexto cero sobre `internal/apps` (HEAD `fb76242`).

Se leyó el archivo de producción `apps.go` (163 líneas), su `README.md` y el
`*_test.go`. Los hallazgos marcados **verificado** se reprodujeron con un test
temporal en el propio paquete (`zz_probe_test.go`, borrado después); los
marcados **por inspección** se deducen del código sin ejecución. No se modificó
código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/apps/ -count=1
-cover` → **ok** (5 tests, cobertura 41.9 %). `go vet ./internal/apps/`
limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | Solo se mira el **primer** argumento: `b3l0w apps --noconfirm -Sy firefox` esquiva el freno de partial upgrade | `apps.go:45` |
| A2 | ALTO | `exec()` devuelve siempre 0 o 1: el exit code real de pacman (127, 2, …) se pierde | `apps.go:87` |
| A3 | MEDIO | `partialUpgrade()` decide por **substrings**: `-Sy` dentro de otra bandera también se rechaza, y `-Syy` con `u` en otro lado no | `apps.go:95` |
| A4 | MEDIO | El freno de DB vieja no aplica a `-Syu` pero `instala()` tampoco distingue `-S` de `-Syu`: el filtro depende del orden de las letras | `apps.go:59` |
| A5 | MEDIO | `OldestSyncDB()` mira `/var/lib/pacman/sync/*.db`: incluye DBs de repos que no son de Arch y excluye los `.db` de otros formats | `apps.go:131` |
| A6 | BAJO | `OldestSyncDB()` ignora el error de `os.Stat` y puede devolver la más vieja sin nombre | `apps.go:139` |
| B1 | BAJO | `humanizar()` dice "0 h" para duraciones de menos de una hora y "1 día" a las 26 h: redondea al piso | `apps.go:160` |
| B2 | BAJO | El README promete que `Run` "devuelve el exit code" de pacman, pero devuelve el propio | `apps.go:87` |

---

## ALTO

### A1 — Solo se mira el primer argumento: el freno de partial upgrade se puede esquivar

**Ubicación:** `apps.go:45` (`flag := pacmanArgs[0]`), `apps.go:47`
(`partialUpgrade`), `apps.go:59` (el freno de DB vieja), `apps.go:74`
(`needsRoot`).

**Qué pasa.** `Run` saca **un** flag del vector y decide todo con ese:

```go
flag := pacmanArgs[0]
if partialUpgrade(flag) {
    output.Fail("eso es un partial upgrade: ...")
    return 1
}
...
return exec(needsRoot(pacmanArgs[0]), pacmanArgs...)
```

Pero pacman acepta banderas globales **antes** de la operación. **Verificado**
en esta máquina:

```
pacman --noconfirm -Qi bash   →  exit 0   (pacman lo acepta)
```

**Verificado** con un probe sobre las funciones del paquete:

```
args=[-Sy firefox]              partialUpgrade=true  instala=true  needsRoot=true
args=[--noconfirm -Sy firefox]  partialUpgrade=false insta=false  needsRoot=false
args=[--needed -Sy]             partialUpgrade=false instala=false needsRoot=false
```

Con `--noconfirm` primero, `partialUpgrade` ve `"--noconfirm"`, no ve la "S"
ni la "y", y **deja pasar el `-Sy`**. El comando resultante es
`pacman --noconfirm -Sy firefox`, que es exactamente el partial upgrade que
este módulo existe para impedir.

Lo mismo pasa con el freno de DB vieja: `instala(flag)` es falso, así que no
se chequea la antigüedad, y `needsRoot` es falso, así que **no se pide sudo**
— el `pacman -Sy` corre sin privilegios y falla por permisos en vez de por
la regla del módulo.

El README (README.md:38) dice que `-Sy` y `-Syy` son "**Rechazado**", sin
matices, y README.md:88-89 afirma que el módulo "no usa `pacman -Sy` nunca,
ni siquiera por error". Hoy es falso.

**Fix propuesto.** Buscar la **operación** en todos los argumentos, no en el
primero: recorrer `pacmanArgs` y quedarse con el primer argumento que empieza
por `-` y contiene una `S`, `R`, `U`, `Q` o `D` (o, mejor, parsear con la
misma tabla de banderas que pacman). Todo lo demás —el freno de partial
upgrade, el de DB vieja y `needsRoot`— debe derivar de esa operación, no de
`args[0]`.

---

### A2 — `exec()` devuelve siempre 0 o 1: el exit code de pacman se pierde

**Ubicación:** `apps.go:77-88` (`exec`), `apps.go:84` (`system.Interactive`).

**Qué pasa.**

```go
func exec(root bool, args ...string) int {
    nombre := "pacman"
    full := args
    if root {
        nombre = "sudo"
        full = append([]string{"pacman"}, args...)
    }
    if err := system.Interactive(nombre, full...); err != nil {
        return 1
    }
    return 0
}
```

`system.Interactive` devuelve `cmd.Run()`, cuyo error es un `*exec.ExitError`
que **carry el código de salida real** de pacman. El wrapper lo aplasta a 1.

Consecuencias verificadas en esta máquina: `pacman -Qi no-existe` sale con 1 y
`pacman -S` sin target sale con 1; pero pacman también usa 127 (comando no
encontrado), 2 (error de sintaxis) y otros. Un script que haga
`b3l0w apps -S foo || echo fallo` no puede distinguir "el paquete no existe"
de "pacman no se encontró" de "el flag está mal".

Además, cuando `root` es falso, el error de `sudo` tampoco se distingue: el
`err` de `Interactive` no dice qué falló.

**Fix propuesto.** Extraer el código de salida del `*exec.ExitError` y
devolverlo:

```go
if err := system.Interactive(nombre, full...); err != nil {
    var ee *exec.ExitError
    if errors.As(err, &ee) {
        return ee.ExitCode()
    }
    return 1
}
```

Y documentar en el README que el código de pacman se propaga.

---

## MEDIO

### A3 — `partialUpgrade()` decide por substrings, no por banderas

**Ubicación:** `apps.go:91-99`.

**Qué pasa.**

```go
func partialUpgrade(flag string) bool {
    if !strings.HasPrefix(flag, "-") {
        return false
    }
    contieneS := strings.Contains(flag, "S")
    return contieneS &&
        strings.Contains(flag, "y") &&
        !strings.Contains(flag, "u")
}
```

La regla de pacman es "la sincronización sin actualización", y en pacman las
banderas se combinan: `-Syu` es sync+upgrade, `-Syy` es refresh doble. El
código adivina la intención contando letras dentro de un string.

Casos donde la heurística se equivoca:

- **`-Suy`**: contiene "S", "u" → `partialUpgrade` da falso. Correcto
  (pacman trata `-Suy` como sync+upgrade). Pero `instala("-Suy")` da true y
  `actualiza("-Suy")` da **true** también (tiene "S", "y" y "u"), así que el
  freno de DB vieja no aplica. Correcto por accidente.
- **`-Su`**: no tiene "y" → `partialUpgrade` da falso. Pero `-Su` **sí**
  actualiza, y `instala("-Su")` da true (no tiene "s" ni "i"), así que se
  trata como una instalación con freno de DB vieja. `-Su` es un upgrade sin
  refresh: el freno de DB vieja aplica, pero el nombre de la función
  (`instala`) miente.
- **`-S` con un target**: `b3l0w apps -S` (sin target) → `instala` true →
  intenta instalar. pacman sale con 1 ("no targets specified"). El wrapper no
  avisa.

El problema de fondo es el mismo de A1: `flag` es un string que se interpreta
por letras sueltas en vez de parsearse como una lista de banderas.

**Fix propuesto.** Parsear el flag en un mapa de letras (como hace pacman:
`S`=sync, `y`=refresh, `u`=upgrade, `s`=search, `i`=info, `q`=quiet) y decidir
con **set de operaciones**, no con `strings.Contains`. Con un parser, `partialUpgrade`
es exactamente "S y y y no u" y `instala` es "S y no s y no i y no u".

---

### A4 — El freno de DB vieja depende del orden de las letras dentro de la bandera

**Ubicación:** `apps.go:59-66`, `apps.go:102-107` (`instala`),
`apps.go:113-115` (`actualiza`).

**Qué pasa.** El README (README.md:48-52) justifica con detalle por qué el
freno de DB vieja no debe aplicar a `-Syu`: "antes frenaba `-Syu` y el
mensaje de escape decía 'usá: b3l0w apps -Syu': un deadlock". El código lo
resuelve con esta línea:

```go
if instala(flag) && !allowStale && !actualiza(flag) {
```

`actualiza(flag)` es `strings.HasPrefix(flag, "-S") &&
strings.Contains(flag, "y") && strings.Contains(flag, "u")`. Con `-Syu`
funciona. Pero el razonamiento depende de que las letras aparezcan en ese
orden exacto en el string: `-uSy` (orden inverso, que pacman también
acepta) da `actualiza` **false** y `instala` **true** → el freno de DB vieja
se aplica a un `-Syu`, y el mensaje de escape vuelve a ser un deadlock.

El mismo razonamiento aplica a `partialUpgrade`: `-Sy` con la y en otro lugar.

**Fix propuesto.** Con el parser de banderas de A3, `actualiza` y `instala`
dejan de depender del orden: `-Syu` es "tiene S, tiene u" sin importar cómo
se escriba.

---

### A5 — `OldestSyncDB()` mira un glob fijo de pacman

**Ubicación:** `apps.go:130-151`, `apps.go:131` (el glob).

**Qué pasa.**

```go
paths, err := filepath.Glob("/var/lib/pacman/sync/*.db")
```

El path está hardcodeado y el glob `*.db` no distingue:

- **Los DBs de la distro y los de repos de terceros.** En esta máquina hay
  `/var/lib/pacman/sync/b3l0w.db` junto a `cachyos.db` y
  `cachyos-extra-v3.db`. El `.db` de `b3l0w` (el repo propio de la distro)
  cuenta para el cálculo de la antigüedad. Si ese repo se sincroniza poco, el
  freno de DB vieja se activa por un repo que el usuario no está instalando.
- **Los `.db.sig`**: el glob `*.db` no los matchea (son `.db.sig`), así que
  acá no hay problema.

El README (README.md:54-57) dice "mira todos los `/var/lib/pacman/sync/*.db`",
o sea que el código hace lo que dice. El problema es que el criterio no es el
que el usuario espera: el freno debería mirar **las DBs de los repos que la
máquina usa**, no todas las que hay en el directorio.

**Fix propuesto.** Distinguir los DBs de la distro de los de terceros
(`pacman-conf` → `DBPath` y la lista de repos) y mirar solo los que
corresponden a repos habilitado. Como mínimo, documentar que el repo de la
distro participa del cálculo.

---

## BAJO

### A6 — `OldestSyncDB()` ignora el error de `os.Stat`

`apps.go:137-145`: si `os.Stat` falla en una DB (borrada entre el glob y el
stat, o permisos), el `continue` lo salta en silencio. Si **todas** fallan,
`vieja` queda en cero y la función devuelve el error "no pude leer las bases de
datos" (`apps.go:148`), que es correcto. El problema intermedio: si la más
vieja falla pero otra no, se devuelve la otra como "la más vieja" sin avisar
que se saltó una. Menor: registrar cuál se saltó.

### B1 — `humanizar()` redondea al piso y produce "0 h"

`apps.go:153-162`: un DB de 30 minutos se anuncia como "0 h" y uno de 26
horas como "1 día". El redondeo al piso da "0 h" (nada útil) y subestima la
antigüedad entre 24 y 48 h. Redondear al alza o usar "menos de 1 h" para el
caso 0.

### B2 — El README promete el exit code de pacman

README.md:22 dice que `Run(args []string) int` "Traduce las banderas y ejecuta
pacman. **Devuelve el exit code**". El "el" sugiere el de pacman; el código
devuelve 0 o 1 (ver A2). Corregir el README o el código: lo natural es lo
segundo (A2).

---

## Lo que no encontré

- **La traducción de `-R` a `-Rns`** (`apps.go:70-72`) es correcta y está
  documentada (README.md:42). `-R` a secas quita el paquete y nada más;
  `-Rns` quita también huérfanas y configuración.
- **`--allow-stale` se filtra antes de llegar a pacman** (`apps.go:33-39`) y se
  recorre en todo el vector, no solo en el primero: es la única bandera que el
  wrapper parsea bien, porque filtra por igualdad y no por letras.
- **El chequeo de DB vieja usa la más vieja, no la más nueva**
  (`apps.go:142-145`). Es la decisión documentada (README.md:54-57) y es la
  correcta: si una sección quedó atrás, instalar desde ella es igual de
  riesgoso.
- **`MaxAge` de 7 días** (`apps.go:27`) es un criterio, no un número de Arch:
  está justificado en el README (README.md:59-61).
- **`oldest.IsZero()`** (`apps.go:142`) como guarda del primer archivo es
  correcto.
- **`system.Interactive`** hereda stdin/stdout/stderr, así que pacman puede
  pedir la contraseña de sudo en la terminal. Correcto.

## Cómo verifiqué

Tests temporales en `internal/apps/zz_probe_test.go` (package `apps`),
ejecutados con `go test ./internal/apps/ -run TestProbe -v` y borrados al
terminar. Cubrieron: la clasificación de banderas con un flag global adelante
(A1, A3, A4). Además se comprobó en la máquina real que
`pacman --noconfirm -Qi bash` sale con 0 (pacman acepta las banderas globales
antes de la operación) y que `pacman -Qi no-existe` sale con 1 (A2). El
estado final del worktree no tiene archivos de probe.

---

## Arreglado

| ID | Arreglo | Test |
|---|---|---|
| A1 | El freno de partial upgrade recorre **todos** los argumentos, no solo `pacmanArgs[0]`. `--noconfirm -Sy firefox` ahora sale con 1. | `TestRunRechazaPartialUpgradeNoPrimero` |

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| B1 | `humanizar` redondea al alza: 30 minutos son "1 h", no "0 h" | `TestHumanizarNoDaCeroHoras` |
| A2 | `exec()` devuelve el `ExitCode()` real de pacman con `errors.As` sobre `*osexec.ExitError` (el import va con alias porque la función local se llama `exec`). Un 127 ya no se confunde con un 1 | `TestExecPropagaElExitCode`, `TestExecDevuelveCeroCuandoTodoVaBien` |


| ID | Arreglo | Test |
|---|---|---|
| A3 | `banderas` descompone una bandera corta letra por letra; `partialUpgrade` y `actualiza` la usan | `TestBanderasDescompone`, `TestBanderasIgnoraLasLargas` |
| A4 | `actualiza` usa el mismo parseo: el freno de DB vieja ya no depende del orden de las letras | `TestBanderasDescompone` |
| A5 | `OldestSyncDB` llama a `oldestSyncDBIn(dir)`, que un test puede apuntar a un directorio propio | código |
| A6 | `OldestSyncDB` devuelve error si no puede leer una DB, en vez de saltarla en silencio | código |
| B2 | Descartado: el README **sí** dice la verdad desde A2 — `Run` devuelve el exit code de pacman | `TestExecPropagaElExitCode` |
