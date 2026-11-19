# BUGS — internal/update

Revision de integridad sobre `internal/update` (HEAD `fb76242`).
Se leyó `update.go` (636 líneas), los 3 `*_test.go` (`update_test.go` 687,
`dotfiles_test.go` 161, `confirm_test.go` 25) y `README.md`. Se leyeron
también los puntos de integración: `cmd/b3l0w/update.go` (el parseo de
flags y el rechazo `--system`/`--programs`), `kit/confirm/confirm.go`,
`kit/system/system.go`, `internal/dotfiles/pull.go` (el `Pull` que se
llama), `internal/selfupdate`, `internal/releases` e
`internal/versioncheck`. Los hallazgos marcados **verificado** se
reprodujeron con tests temporales en `t.TempDir()` (archivo
`zz_probe_test.go`, borrado después). No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/update/ -count=1`
→ **ok** (0.036s), 31 tests verdes, cobertura **50.0 %**. `go vet` limpio.

## Cierre de los ALTO (A1-A4)

Después de la revisión original se cerraron los cuatro hallazgos ALTO.
Cada arreglo tiene su test, y **cada test se revirtió a mano para
comprobar que falla** (ver "Cómo se verificaron", abajo).

Estado de la suite hoy: `go test ./internal/update/ -count=1` → **ok**,
**41 tests verdes**, cobertura **64.2 %**. `go vet` y `gofmt` limpios.

## Cierre de los MEDIO (M1-M6)

Después del cierre de los ALTO se cerraron los seis hallazgos MEDIO. Cada
arreglo tiene su test, y **cada test se revirtió a mano para comprobar
que falla** (ver "Cómo se verificaron", abajo).

M4 **no se reproduce**: en Linux `os.UserHomeDir()` ya devuelve `$HOME`, así
que el cambio propuesto no altera ningún comportamiento observable (el
test especificado pasa idéntico antes y después). De ese hallazgo solo se
arregló la otra mitad, que sí era un defecto: el mensaje crudo de Go
cuando no hay home.

Estado de la suite hoy: `go test ./internal/update/ -count=1` → **ok**,
**48 tests verdes**, cobertura **72.0 %**. `go vet` y `gofmt` limpios.

## Resumen

| ID | Sev | Qué | Línea | Estado |
|---|---|---|---|---|
| C1 | CRÍTICO | `--dotfiles --dry-run` crea una **snapshot real** de snapper: el dry-run no es un dry-run | `update.go:361` | arreglado |
| A1 | ALTO | `updateDotfiles` aborta con código 1 si `noctalia` no está instalado: la validación del TOML se confunde con "no hay Noctalia" | `update.go:335-339` | **arreglado** |
| A2 | ALTO | `updateServices` ignora el error de cada servicio: un servicio que no se actualiza se reporta como si todo bien | `update.go:234-237` | **arreglado** |
| A3 | ALTO | `checkDesktop` compara siempre contra los `-git`, aunque el canal sea `stable` | `update.go:578` | **arreglado** |
| A4 | ALTO | `dryRun` anuncia una actualización comparando strings: `"v0.5.0"` vs `"0.5.0"` se ven distintos | `update.go:156` | **arreglado** |
| M1 | MEDIO | `Run` con `--self` pide confirmación de **todas** las capas, no solo del CLI | `update.go:94` | **arreglado** |
| M2 | MEDIO | `updateDotfiles` hace el pull aunque el preview haya fallado: el error del preview se descarta | `update.go:372` | **arreglado** |
| M3 | MEDIO | `updateDotfiles` solo valida el TOML de Noctalia, no el de Umbriel | `update.go:335` | **arreglado** |
| M4 | MEDIO | El estado de dotfiles se deriva de `os.UserHomeDir()` en vez de `$HOME`: un `HOME` distinto rompe la ruta | `update.go:342-347` | **no se reproduce** (la otra mitad, arreglada) |
| M5 | MEDIO | `updatePlugins` solo usa la ruta de plugins de Noctalia si el comando existe: sin Noctalia, `LoadDefault` decide | `update.go:188-206` | **arreglado** |
| M6 | MEDIO | `check` y `checkDesktop` mezclan red con salida: un timeout de AUR no distingue "todo bien" de "no se consultó" | `update.go:578-582` | **arreglado** |
| B1 | BAJO | `Run` sale con 0 cuando el usuario cancela: un script no puede distinguir "cancelado" de "hecho" | `update.go:95-97` | pendiente |
| B2 | BAJO | `syncRegistry` no distingue los kinds: un fallo de sync no dice qué categoría falló | `update.go:261-265` | pendiente |
| B3 | BAJO | `confirmPrompt` con `--self` solo lista "b3l0w", pero `Run` ejecuta el CLI primero y corta: el prompt no lo dice | `update.go:445-469` | pendiente |
| B4 | BAJO | `localVersion` no distingue "no instalado" de "pacman falló" | `update.go:612-622` | pendiente |

---

## CRÍTICO

### C1 — `--dotfiles --dry-run` crea una snapshot real del home

**Ubicación:** `update.go:361` (dentro de `updateDotfiles`).

**Qué pasa.** `Run` evalúa las banderas en este orden:

```go
if opts.Check { return check(m) }                    // update.go:52
if opts.Dotfiles || opts.Adopt { return updateDotfiles(opts) }   // update.go:57  ← CORTA AQUÍ
if opts.Desktop { return desktopUpdate() }            // update.go:62
...
if opts.DryRun { return dryRun(m, opts) }              // update.go:78  ← NUNCA LLEGA
```

La rama de `Dotfiles` (`update.go:57`) está **antes** que la de `DryRun`
(`update.go:78`). Con `--dotfiles --dry-run` se entra a `updateDotfiles`
con `opts.DryRun == true`, y ahí:

```go
// Snapshot antes de tocar el home.
if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
    output.Warn("no home snapshot: " + err.Error())
}                                                        // update.go:361-363
```

`snapshot.Create` (`internal/snapshot/snapshot.go:19-31`) ejecuta
`snapper -c home create -d "b3l0w pre-dotfiles"`: **crea una snapshot
real** del subvolume del home. No es una simulación: consume espacio en
disco, aparece en `snapper list` y el usuario tiene que limpiarla.

El propio módulo se contradice: el comentario en `update.go:70-77` dice
"`--dry-run` tiene que ser un dry-run de verdad... Con el flag se muestra
el plan y no se toca nada", y el README (`README.md:52`) promete
"Muestra el plan y **no toca nada**".

**Reproducción (verificado).** Con un fake de `snapper` que deja un
marker en disco:

```go
// marker: si el fake se ejecuta, deja un archivo
updateDotfiles(Options{DryRun: true, DotfilesDir: repo})
// -> el marker existe: el dry-run EJECUTO snapper
```

Y por el camino completo, con `Run`:

```go
Run(&config.Manifest{}, Options{Dotfiles: true, DryRun: true, DotfilesDir: repo})
// HALLAZGO: --dotfiles --dry-run EJECUTO snapper (snapshot real)
// Run(--dotfiles --dry-run) = 0
```

**Consecuencia.** `b3l0w update --dotfiles --dry-run` es el comando que un
usuario ejecuta **precisamente para no tocar nada** (para ver qué
cambiaría, antes de un update real). Crea una snapshot del home cada vez.
Con un `home` de 100 GB, cada dry-run copia (con `snapper`, por
`btrfs send/receive` o por el mecanismo de subvolumen) el estado
completo. Y como el snapshot se llama siempre `"b3l0w pre-dotfiles"`, se
acumulan varias snapshots indistinguibles.

**Fix propuesto.** No crear la snapshot cuando `opts.DryRun` es true:

```go
if !opts.DryRun {
    if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
        output.Warn("no home snapshot: " + err.Error())
    }
}
```

Y, en general, revisar el orden de las ramas de `Run`: la de `DryRun`
(`update.go:78`) debería ir **antes** que la de `Dotfiles` (`update.go:57`),
o `dryRun` debería incluir el preview de dotfiles (que hoy sí hace,
`update.go:371-384`, pero solo cuando se entra por la rama de `Dotfiles`).

---

## ALTO

### A1 — Sin `noctalia` instalado, el update de dotfiles aborta

**Estado: ARREGLADO.** Test: `TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta`
(contraste: `TestBug_A1_ConNoctaliaElTOMLInvalidoSigueAbortando`).

**Ubicación:** `update.go:335-339` (original).

**Qué pasaba.** `updateDotfiles` validaba el TOML de Noctalia **antes de
todo**:

```go
if path := filepath.Join(dir, "noctalia", "config.toml"); fileExists(path) {
    if out, err := kitsystem.RunCombined("noctalia", "config", "validate", path); err != nil {
        output.Fail("repo TOML does not validate: " + out)
        return 1
    }
}
```

Si el binario `noctalia` **no está instalado**, `RunCombined` falla con
"executable file not found in $PATH" y el comando sale con **código 1**,
sin aplicar nada.

El problema: el mensaje dice "repo TOML does not validate", cuando en
realidad el TOML puede ser perfectamente válido — lo que falta es el
programa que lo valida. Son dos fallos distintos con el mismo mensaje y el
mismo código de salida.

**Reproducción (verificado).** Con un repo que tiene `noctalia/config.toml`
y un `PATH` sin `noctalia`:

```
updateDotfiles = 1
stderr: "✗ repo TOML does not validate:"     ← mensaje vacío, sin causa
```

**Arreglo aplicado.** La validación solo corre si el validador está
instalado; si no, se avisa y se sigue:

```go
if path := filepath.Join(dir, "noctalia", "config.toml"); fileExists(path) {
    if kitsystem.HaveCommand("noctalia") {
        if out, err := kitsystem.RunCombined("noctalia", "config", "validate", path); err != nil {
            output.Fail("repo TOML does not validate: " + out)
            return 1
        }
    } else {
        output.Warn("noctalia no está instalado: no se valida el TOML del repo")
    }
}
```

El mensaje de error del TOML inválido **no cambia**: si el validador está
y el TOML no valida, el update sigue abortando con 1 y el mismo texto.
Eso es lo que fija el test de contraste: el arreglo no puede ser "no
validar nunca".

El mismo patrón (`HaveCommand`) ya lo usaba `updatePlugins`
(`update.go:188`); ahora las dos mitades del módulo son consistentes.

**M3 sigue pendiente:** el TOML de Umbriel todavía no se valida.

---

### A2 — `updateServices` descarta el error de cada servicio

**Estado: ARREGLADO.** Tests: `TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida`
y `TestBug_A2_RunPropagaElFalloDeServicios`.

**Ubicación:** `update.go:234-237` y `update.go:131` (original).

**Qué pasaba.** El bucle de `updateServices` imprimía el error y seguía
con el siguiente servicio, pero la función **siempre** devolvía 0:

```go
newState, err := in.Update(*svc, state, true, yes)
if err != nil {
    output.Fail(fmt.Sprintf("%s: %v", state.ID, err))
    continue          // ← el error se imprime y se pierde
}
...
return 0             // update.go:243
```

Y en `Run` el return ni siquiera se miraba:

```go
if servicesWanted {
    updateServices(opts.Yes)   // update.go:131 — el return se descarta
}
```

Consecuencia: si los servicios fallan al actualizarse, `b3l0w update`
imprime tres `✗` y después `✓ update complete` y sale con **código 0**.

**Arreglo aplicado.** Se cuentan los fallos y el resultado vuelve por la
cadena:

```go
fallos := 0
for _, state := range states {
    ...
    if err != nil {
        output.Fail(fmt.Sprintf("%s: %v", state.ID, err))
        fallos++
        continue
    }
    ...
}
if fallos > 0 {
    return 1
}
return 0
```

y en `Run`:

```go
if servicesWanted {
    if code := updateServices(opts.Yes); code != 0 {
        return code
    }
}
```

**Lo que NO cambió:** el update sigue con el resto de los servicios (el
README lo promete: "sigue con el resto de los servicios"). Lo que cambia
es el **resultado**: ahora el error se refleja en el código de salida.

**Nota sobre el orden.** El código de `updateServices` se propaga antes de
los dotfiles, así que un fallo de servicio ahora corta la cadena (como ya
pasaba con `updatePrograms`). Es el mismo contrato que el resto de las
capas.

---

### A3 — `checkDesktop` compara contra los paquetes `-git` aunque el canal sea `stable`

**Estado: ARREGLADO.** Test: `TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal`.

**Ubicación:** `update.go:578` y `update.go:600-604` (original).

**Qué pasaba.** `checkDesktop` preguntaba al AUR por los paquetes
**-git**, siempre:

```go
remote, err := versioncheck.AUR([]string{"umbriel-git", "noctalia-git"})
```

Y `desktopUpdate` (la otra mitad del módulo) **sí** respetaba el canal.
Las dos mitades del mismo archivo discrepaban: en un sistema de canal
`stable`, `b3l0w update --check` preguntaba por `umbriel-git` (que no
está instalado) y decía "no instalado", cuando el sistema tiene
`umbriel` instalado y al día.

**Reproducción (verificado).** Con el canal en `stable` y un `pacman`
fake que solo responde para `umbriel`:

```
checkDesktop (canal stable) dice:
"! umbriel-git: no instalado (stable)
 ! noctalia-git: no instalado (stable)"
```

**Arreglo aplicado.** La selección de paquetes se extrajo a un helper
compartido por las dos mitades:

```go
func desktopPackages(ch string) []string {
    if ch == channel.Stable {
        return []string{"umbriel", "noctalia"}
    }
    return []string{"umbriel-git", "noctalia-git"}
}
```

`checkDesktop` usa `aurLookup(desktopPackages(ch))` y `desktopUpdate`
usa `desktopPackages(ch)`. Los dos caminos no pueden volver a
divergir.

---

### A4 — El dry-run compara versiones como strings

**Estado: ARREGLADO.** Tests: `TestBug_A4_ElDryRunComparaLasVersionesComoNumeros`
y `TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes`.

**Ubicación:** `update.go:156` y `update.go:504` (original).

**Qué pasaba.** `dryRun` decidía si había que actualizar comparando
strings:

```go
if tag, err := selfupdate.Check(); err == nil && tag != "" && tag != version.Version {
    output.Step(fmt.Sprintf("  b3l0w: %s -> %s", version.Version, tag))
}
```

`version.Version` viene del `-ldflags`. El `PKGBUILD` lo inyecta como
`v$pkgver`, o sea **con** la `v`. Pero el README de `internal/version`
muestra la forma `-X ...version.Version=v0.1.0` y el propio módulo tiene
un helper `withV` que existe **precisamente** porque "el ldflags puede
venir con ella (v0.2.0) o sin ella (0.2.0)".

Si el binario fue compilado con la versión **sin** la `v` ("0.5.0"), el
dry-run muestra "b3l0w: 0.5.0 -> v0.5.0": un plan que dice que hay que
actualizar a la versión que ya tenés. El mismo bug estaba en `check`
(`update.go:504`), que comparaba igual.

**Arreglo aplicado.** La comparación es numérica, no de strings:

```go
func versionMasNueva(remota, local string) bool {
    r := numerosVersion(remota)
    l := numerosVersion(local)
    for i := 0; i < 3; i++ {
        if r[i] != l[i] {
            return r[i] > l[i]
        }
    }
    return false
}
```

`numerosVersion` saca el prefijo `v`, corta los sufijos (`-dev`,
`+build`) y lee los tres primeros números. Es la misma regla con la que
`internal/selfupdate` decide si baja un release, replicada acá para no
tocar otro paquete (el `compareVersions` de `selfupdate` es privado).

El dry-run ahora solo anuncia la actualización cuando el release es
**más nuevo**, y `check` avisa por la misma vía (`avisoVersionCLI`, que
además conserva el `if version.Version` con "dev" que ya tenía).

**Efecto secundario bueno:** un release más viejo que el binario ya no
genera un plan de "downgrade".

---

## MEDIO

### M1 — `Run` con `--self` pide confirmación de todas las capas

**Ubicación:** `update.go:83-94`.

**Qué pasa.** Los selectores se derivan así:

```go
anySelector := opts.System || opts.Programs || opts.Plugins || opts.Services
selfWanted := opts.Self || !anySelector
systemWanted := opts.System || !anySelector
programsWanted := opts.Programs || !anySelector
...
dotfilesWanted := !anySelector
```

`opts.Self` **no** cuenta como selector: `anySelector` es `false` con solo
`--self`. Entonces `!anySelector` es `true` y **todas** las capas quedan
wanted: `systemWanted`, `programsWanted`, `pluginsWanted`,
`servicesWanted`, `dotfilesWanted`.

**Reproducción (verificado).** Con `Run(&config.Manifest{}, Options{Self: true})`
y la confirmación forzada como terminal:

```
PROMPT REAL: "Update b3l0w, system, programs, plugins, services, dotfiles? [Y/n]"
```

El usuario que pide `b3l0w update --self` ve un prompt que promete
actualizar el sistema, los programas, los plugins, los servicios y los
dotfiles. En realidad, más abajo (`update.go:110-112`) el flujo corta
después del CLI:

```go
if opts.Self && !anySelector {
    return 0
}
```

así que solo se actualiza el binario. El prompt miente sobre el alcance.

El README (`README.md:43`) dice "`--self` | Solo el binario (si va solo,
corta ahí)". El comportamiento del código es el correcto; **el prompt es
el que está mal**.

**Fix propuesto.** Tratar `Self` como selector para el prompt, o construir
el prompt a partir de lo que realmente se va a hacer. Lo más simple: que
`anySelector` incluya `opts.Self`, y que la rama de "corta ahí" se
compruebe antes.

---

### M2 — El preview de conflictos se descarta si falla

**Ubicación:** `update.go:371-384`.

**Qué pasa.** Antes del pull real, el módulo hace un preview:

```go
if !opts.Force {
    if preview, err := dotfiles.Pull(dir, home, state, false, true); err == nil && len(preview.Conflicts) > 0 {
        // ... preguntar ...
    }
}
```

El `err == nil &&` descarta el error **y** el preview. Si el preview falla
(estado corrupto, home sin permisos), el módulo no avisa y sigue al pull
real de `update.go:387`, que **falla con el mismo error** (el estado
corrupto sigue corrupto) y ahí sí sale con código 1. El usuario ve el
mismo error dos veces y nunca se le muestra el preview.

Más grave: si el preview falla por un motivo que el pull real **no**
reproduce, el pull real se ejecuta **sin la confirmation** que el preview
iba a pedir. Es un caso raro, pero el `err == nil &&` es una Cortina de
humo: oculta el motivo por el que no se mostró nada.

**Fix propuesto.** Separar los dos casos:

```go
preview, err := dotfiles.Pull(dir, home, state, false, true)
if err != nil {
    output.Warn("no se pudo previr los conflictos: " + err.Error())
} else if len(preview.Conflicts) > 0 {
    // ... preguntar ...
}
```

(El mismo patrón `err == nil &&` aparece en `internal/dotfiles`, que ya
lo tiene anotado como B10.)

---

### M3 — Solo se valida el TOML de Noctalia

**Ubicación:** `update.go:335`.

**Qué pasa.** La validación mira un solo archivo:

```go
if path := filepath.Join(dir, "noctalia", "config.toml"); fileExists(path) {
```

Pero el propio módulo, al final, le dice al usuario que valide **los
dos**:

```go
fmt.Printf("  %s
", output.Dim("validate with: noctalia config validate  |  umbriel config validate"))
```

`update.go:416`. El repo de dotfiles también trae `umbriel/config.toml`
(el repo tiene `templates/umbriel`), y ese **no** se valida antes de
aplicarlo. Un TOML de Umbriel roto se aplica al home sin que nadie lo
haya mirado.

**Fix propuesto.** Validar ambos TOMLs, en un bucle sobre la lista de
programas del escritorio.

---

### M4 — El estado de dotfiles se deriva de `os.UserHomeDir()`, no de `$HOME`

**Ubicación:** `update.go:342-347`.

**Qué pasa.**

```go
home, err := os.UserHomeDir()
if err != nil {
    output.Fail(err.Error())
    return 1
}
state := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
```

`os.UserHomeDir()` en Linux devuelve `$HOME`. En la práctica son lo mismo,
pero el resto del módulo **sí** usa `$HOME` explícitamente: los tests lo
acotan con `t.Setenv("HOME", home)`, y `defaultDotfilesDir`
(`update.go:421-436`) también usa `os.UserHomeDir()`.

El detalle real: `os.UserHomeDir()` **falla** si `$HOME` está vacío, y en
ese caso `updateDotfiles` sale con 1 y el mensaje crudo
("neither $HOME nor the user database are defined"). El README del módulo
no menciona este requirement.

**Fix propuesto.** Usar `os.Getenv("HOME")` con un fallback explícito, o
al menos un mensaje de error que diga qué variable falta.

---

### M5 — `updatePlugins` decide el camino según exista el comando `noctalia`

**Ubicación:** `update.go:188-206`.

**Qué pasa.** `updatePlugins` tiene dos caminos:

```go
if kitsystem.HaveCommand("noctalia") {
    if sources, err := plugins.Sources(); err == nil && len(sources) > 0 {
        if err := plugins.UpdateSources(); err != nil { ... }
        return 0
    }
}

m, err := plugins.LoadDefault()
if err != nil { ... }
if err := plugins.Update(m, "", plugins.DefaultPluginsDir(), false); err != nil { ... }
```

Si `noctalia` está instalado pero `plugins.Sources()` **falla** (un error
de red o de lectura), el `if` interno no entra y el código **cae al
camino de abajo**, que usa `LoadDefault()` y `Update()` — un camino
distinto, con otra lista de plugins. Un error transitorio de `Sources()`
cambia silenciosamente **qué** se actualiza.

Lo mismo con `len(sources) == 0`: no es un error, pero lleva al camino
alterno.

**Fix propuesto.** Distinguir "no hay sources" de "falló Sources()":

```go
if kitsystem.HaveCommand("noctalia") {
    sources, err := plugins.Sources()
    if err != nil {
        output.Warn("plugins: no se pudieron leer los sources: " + err.Error())
        return 1
    }
    if len(sources) > 0 {
        ...
    }
}
```

---

### M6 — `checkDesktop` no distingue "sin red" de "todo al día"

**Ubicación:** `update.go:578-582`.

**Qué pasa.** Si la consulta al AUR falla, el módulo avisa:

```go
remote, err := versioncheck.AUR([]string{"umbriel-git", "noctalia-git"})
if err != nil {
    output.Warn("escritorio: no se pudo consultar AUR")
    return
}
```

El warning es correcto, pero `check` (`update.go:500-547`) **no** lo
cuenta como error y devuelve 0 al final (`update.go:546`). Un `b3l0w
update --check` sin red sale con "todo al día" para el escritorio. El
README (`README.md:143-145`) reconoce que `checkDesktop` necesita AUR, y
el módulo degrade a warning es una decisión consciente — pero el
resultado combinado ("check pasó") es engañoso.

**Fix propuesto.** Devolver un código distinto cuando el escritorio no se
pudo consultar, o al menos imprimir el warning en stderr (que `output.Warn`
ya hace) y que `check` lo refleje.

---

## BAJO

### B1 — Cancelar sale con código 0

**Ubicación:** `update.go:93-97`.

**Qué pasa.**

```go
if !opts.Yes {
    if !confirm.Confirm(confirmPrompt(...), false, true) {
        output.Warn("cancelled")
        return 0
    }
}
```

Cancelar sale con 0, igual que completar el update. Un script que corre
`b3l0w update` no puede distinguir "el usuario canceló" de "se actualizó
todo". El README (`README.md:73-74`) documenta el comportamiento
("imprime `cancelled` y sale con 0"), así que es una decisión consciente.

---

### B2 — `syncRegistry` no dice qué categoría falló

**Ubicación:** `update.go:261-265`.

**Qué pasa.** El error de `registry.Sync` se pasa tal cual:

```go
for _, kind := range kinds {
    if _, err := registry.Sync(kind, nil); err != nil {
        output.Warn(err.Error())
    }
}
```

`registry.Sync` sí menciona el kind en su error
(`internal/registry/registry.go:118`: "... (se usa la copia local de %s)"),
así que el mensaje no es opaco. El problema es otro: **los tres syncs se
hacen aunque uno haya fallado**, y no hay resumen. Con `--system
--services --plugins` hay hasta tres warnings seguidos sin contexto de
que son independientes. Menor.

---

### B3 — `confirmPrompt` con `--self` solo no dice que corta ahí

**Ubicación:** `update.go:445-469` y `update.go:110-112`.

**Qué pasa.** Es la otra mitad de M1: el prompt arma la lista de capas
que se van a tocar, pero con `--self` solo la ejecución se corta después
del CLI (`update.go:110-112`). El prompt no tiene forma de decir "y nada
más". Documentado en el README (`README.md:64-68`), que explica el orden
pero no este caso.

---

### B4 — `localVersion` no distingue "no instalado" de "pacman falló"

**Ubicación:** `update.go:612-622`.

**Qué pasa.**

```go
func localVersion(pkg string) string {
    out, err := kitsystem.Run("pacman", "-Q", pkg)
    if err != nil {
        return ""
    }
    ...
}
```

Cualquier error de pacman (no instalado, base de datos corrupta, sin
permisos) devuelve `""`, que el consumidor interpreta como "no instalado"
(`update.go:586-587`). Un `pacman -Q` que falla por otra razón produce un
mensaje engañoso.

**Fix propuesto.** Distinguir el caso "no está instalado" (código de salida
de pacman y un mensaje en stderr que lo diga) de los demás errores.

---

## Lo que no encontré

- **El fail-closed de los conflictos es correcto.** El preview se hace
  **siempre**, incluso en dry-run (`update.go:367-384`), y para resolver
  un conflicto hacen falta **dos** confirmaciones
  (`update.go:377-381`). El README (`README.md:106-110`) lo promete y el
  test `TestBug_ElDryRunMuestraLosConflictos` lo verifica: el dry-run
  menciona el conflicto, nombra el archivo y **no** escribe ni el archivo
  ni el `.new`.
- **Los huérfanos se avisan y no se borran** (`update.go:407-409`): el
  archivo local nunca se borra por estar en el estado. Verificado por
  `TestBug_LosHuerfanosSeAvisan`.
- **`updatePrograms` no aborta en el primer fallo** (`update.go:474-497`):
  recorre todos los programas, cuenta los fallos y recién entonces
  devuelve 1. El README (`README.md:34-36`) lo promete.
- **`updateSystem` degrada bien** (`update.go:291-321`): sin actualizador
  o sin terminal, avisa y sigue; si el usuario pidió **solo** el sistema,
  sale con 1. Verificado por `TestUpdateSystemSinActualizador` y
  `TestUpdateSystemSinTerminal` (que además comprueba, con un marker, que
  el actualizador **no** se ejecutó).
- **El sistema se actualiza antes que los programas** (`update.go:115-124`):
  el orden del README (`README.md:10-19`) se respeta.
- **El CLI se actualiza primero** (`update.go:105-113`): si el binario es
  de un paquete, `selfUpdate` lo degrada a warning y no a error
  (`update.go:271-276`), que es lo que dice el README (`README.md:63-65`).
- **`dryRun` no toca nada del sistema**: no llama a `cachy-update`, ni a
  `pacman`, ni a `snapper` (el único `snapshot.Create` está en
  `updateDotfiles`, que `dryRun` no alcanza). Verificado: `TestRunDryRunMuestraElPlan`
  no encuentra ningún marker.
- **`check` no actualiza nada**: solo lee (`system.List`, `programs.Statuses`,
  `releases.Check`). El nombre de la función lo dice y el código lo
  cumple.
- **`defaultDotfilesDir` prefiere el repo de GitHub sobre la copia
  compartida** (`update.go:421-436`), con los tres casos verificados
  (repo presente, solo share, ninguno). Es la decisión del README.
- **La captura de salida de los tests no filtra el ANSI por accidente**
  (`update_test.go:31`): el regex quita los códigos de color para que los
  asserts no dependan de si stdout parece una terminal.
- **Los tests acotan `PATH` y `HOME`**: los fakes de `vercmp`, `pacman`,
  `snapper`, `noctalia` y `cachy-update` dejan un marker si se ejecutan, así
  que un test que tocara el sistema fallaría. La regla del módulo
  (`update_test.go:21-26`) se cumple.
- **`compareVersions` (vercmp) es fail-safe**: sin `vercmp`, devuelve 0
  ("igual"), o sea que **no inventa** actualizaciones. Es la decisión del
  README (`update.go:624-625`) y está cubierta por
  `TestCompareVersionsSinVercmpDevuelveCero`.
- **No hay path traversal**: `defaultDotfilesDir` arma rutas con
  `filepath.Join` sobre candidatos fijos, y `fileExists` solo comprueba.
- **No hay `exec.Command` con strings concatenadas**: todo va por
  `kit/system`, que usa `exec.Command` con argumentos separados.

## Cómo verifiqué

La revisión original se hizo con tests temporales en
`internal/update/zz_probe_test.go` (package `update`), ejecutados con
`go test ./internal/update/ -run TestProbe -v` y borrados al terminar.
Cubrieron: el snapshot en dry-run (C1, por `updateDotfiles` y por
`Run`), el prompt con `--self` (M1), la validación de Noctalia sin el
binario instalado (A1), `checkDesktop` con canal `stable` (A3). El bug A2
(es el error de un servicio individual) era **por inspección**: no se
reprodujo porque `updateServices` necesita Docker y estados reales, pero
el `return 0` incondicional de `update.go:243` y el `return` descartado de
`update.go:131` son verificables leyendo el código.

**El cierre de A1-A4 es verificable de punta a punta.** Los cuatro
arreglos se probaron con tests permanentes (`alto_test.go`) y, además,
**cada arreglo se revirtió a mano** para comprobar que su test falla: un
test que no falla al revertir el bug no prueba el bug. El detalle está
en "Cómo se verificaron", más abajo.

A2 se volvió verificable sin Docker gracias al punto de inyección
`updateService`: el test simula que el update del servicio falla y
comprueba que el error llega al código de salida.

El worktree no tiene archivos de prueba temporales: los que se usaron
para la revisión original se borraron, y los de A1-A4 son permanentes
(`alto_test.go`).

---

## Arreglado

| ID | Arreglo | Test |
|---|---|---|
| C1 | `updateDotfiles` no crea la snapshot cuando `opts.DryRun`: el flag promete no tocar nada y una snapshot es escribir. `--adopt` en dry-run tampoco ejecuta (registra estado y no tiene preview). | `TestDryRunNoCreaSnapshot` (y `TestSinDryRunSiCreaSnapshot` como contraste) |
| A1 | La validación del TOML de Noctalia solo corre si el validador está instalado. Sin `noctalia` se avisa y el update sigue; con el validador y el TOML inválido, el abort con código 1 y el mensaje son los de siempre. | `TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta` + `TestBug_A1_ConNoctaliaElTOMLInvalidoSigueAbortando` (contraste) |
| A2 | `updateServices` cuenta los fallos y devuelve 1 si hubo alguno; `Run` respeta ese código. El update sigue con el resto de los servicios (no cambia el contrato del README), lo que cambia es el resultado. | `TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida` + `TestBug_A2_RunPropagaElFalloDeServicios` |
| A3 | La selección de paquetes del escritorio se extrajo a `desktopPackages(ch)`, compartido por `checkDesktop` y `desktopUpdate`: las dos mitades ya no pueden divergir. | `TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal` |
| A4 | La comparación de versiones del CLI es numérica (`versionMasNueva`), no de strings: `"v0.5.0"` y `"0.5.0"` son la misma versión. El dry-run y el `check` usan la misma regla. | `TestBug_A4_ElDryRunComparaLasVersionesComoNumeros` + `TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes` |
| M1 | `--self` cuenta como selector: la decisión de qué capas se tocan se extrajo a `capasPedidas(opts)`, y la usan `Run` y el dry-run. El prompt ya no promete el sistema, los programas, los plugins, los servicios ni los dotfiles cuando en realidad solo se actualiza el CLI. `soloSelf(opts)` conserva el corte de `--self` a secas. | `TestM1_CapasPedidasSelfNoArrastraLasDemas` |
| M2 | Si el preview de conflictos falla, el update aborta con 1 y avisa que no se pudo previsualizar. Antes el error se descartaba y el pull real corría sin la confirmación que el preview iba a pedir. (El fix de la revisión era avisar y seguir; se abortó, que es lo más seguro: sin preview no se pisa un cambio local a ciegas.) | `TestM2_ElPreviewQueFallaAbortaAntesDelPull` |
| M3 | La validación del TOML recorre los dos programas del escritorio (`noctalia` y `umbriel`): un TOML de Umbriel roto aborta con 1 antes de aplicar. Si el validador no está instalado se avisa y se sigue, como ya hacía A1. | `TestM3_ElTOMLDeUmbrielSeValida` |
| M4 | **No se reproduce:** en Linux `os.UserHomeDir()` ya devuelve `$HOME`, y `kit/paths.Home()` es un wrapper que solo cambia el error por `""`. El test especificado (`t.Setenv("HOME", tmp)`) pasa idéntico antes y después, así que no hay arreglo que verificar. De la otra mitad del hallazgo sí se arregló: sin home el error dice qué variable falta, y la ruta del estado nunca es relativa. | `TestM4_SinHomeElErrorDiceQueFalta` (detecta el revert) + `TestM4_ElEstadoSeLeeDelHome` (caracterización: fija que el estado se lee de `$HOME`) |
| M5 | `updatePlugins` distingue "no hay sources" de "no se pudieron leer": si Noctalia está y la lectura de los sources falla, el update aborta con 1 en vez de caer en silencio al camino que actualiza OTRA lista de plugins. | `TestM5_UnSourceQueNoSeLeeEsUnError` |
| M6 | `checkDesktop` devuelve si pudo consultar y `check` cuenta ese fallo: sin red, `b3l0w update --check` sale con 1. El mensaje del warning no cambia: lo que mentía era el código de salida. | `TestM6_SinAurElCheckFalla` |

### Cómo se verificaron

Cada test de A1-A4 se revirtió a mano (el bloque exacto del arreglo se
volvió a poner como estaba) y el test tiene que **fallar**. Los seis
tests fallaron al revertir y pasaron al restaurar:

```
A1  TestBug_A1_SinNoctaliaElUpdateDeDotfilesNoAborta: FALLA (bien)
A2  TestBug_A2_ElErrorDeUnServicioLlegaAlCodigoDeSalida: FALLA (bien)
A2  TestBug_A2_RunPropagaElFalloDeServicios: FALLA (bien)
A3  TestBug_A3_CheckDesktopPreguntaPorLosPaquetesDelCanal: FALLA (bien)
A4  TestBug_A4_ElDryRunComparaLasVersionesComoNumeros: FALLA (bien)
A4  TestBug_A4_ElCheckNoAvisaDeLaVersionQueYaTenes: FALLA (bien)
```

Los seis MEDIO se verificaron igual (el bloque exacto del arreglo se
volvió a poner como estaba y el test tiene que fallar):

```
M1  TestM1_CapasPedidasSelfNoArrastraLasDemas: FALLA (bien)
M2  TestM2_ElPreviewQueFallaAbortaAntesDelPull: FALLA (bien)
M3  TestM3_ElTOMLDeUmbrielSeValida: FALLA (bien)
M4  TestM4_SinHomeElErrorDiceQueFalta: FALLA (bien)
M5  TestM5_UnSourceQueNoSeLeeEsUnError: FALLA (bien)
M6  TestM6_SinAurElCheckFalla: FALLA (bien)
```

El test de caracterización de M4 (`TestM4_ElEstadoSeLeeDelHome`) **pasa**
con el arreglo revertido: es a propósito, porque lo que fija ya era el
comportamiento correcto antes del cambio (ver la fila de M4).

### Puntos de inyección

Para probar sin tocar la red ni Docker, el módulo ahora tiene seis
variables inyectables (el mismo patrón que ya usa
`internal/selfupdate` con `apiURL`/`replaceFile`):

| Variable | Sustituye a |
|---|---|
| `selfCheck` | `selfupdate.Check` (releases del CLI) |
| `aurLookup` | `versioncheck.AUR` (consulta al AUR) |
| `registrySync` | `registry.Sync` (sync de las DB del registry) |
| `updateService` | `(*services.Installer).Update` (update de un servicio) |
| `releaseCheck` | `releases.Check` (firmas de los releases) |
| `pluginSources` | `plugins.Sources` (sources de plugins de Noctalia) |

Los tests los sustituyen por fakes y los restauran con `t.Cleanup`.
Ninguno toca la red, el home del usuario ni binarios del sistema: el
`PATH` y el `HOME` van a temporales y el manifiesto de servicios sale del
caché local del registry (`XDG_STATE_HOME` temporal).

## Arreglados (BAJO)

| ID | Arreglo | Test |
|---|---|---|
| B1 | Cancelar devuelve 1, no 0: un script distingue "cancelado" de "actualizado" | `TestRunSinYesCancelaSinTerminal` |
| B2 | El warning del sync dice la categoría y al final hay un resumen de cuántos catálogos quedaron sin refrescar | código |
| B3 | Descartado: lo resolvió M1. Con `--self` solo, `confirmPrompt` ya dice únicamente `b3l0w` | `TestM1_CapasPedidasSelfNoArrastraLasDemas` |
| B4 | `localVersion` devuelve `(string, error)`: `pacman -Q` exit 1 es "no instalado"; cualquier otro código es un fallo real | `TestLocalVersionNoInstalado`, `TestLocalVersionConError` |
