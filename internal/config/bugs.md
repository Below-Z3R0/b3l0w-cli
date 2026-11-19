# BUGS — internal/config

Revisión de integridad en contexto cero sobre `internal/config` (HEAD
`fb76242`).

Se leyó el archivo de producción `manifest.go` (110 líneas), su `README.md`,
el `*_test.go` y los puntos de integración en `cmd/b3l0w/programs.go` e
`internal/programs/`. Los hallazgos marcados **verificado** se reprodujeron
con un test temporal en el propio paquete (`zz_probe_test.go`, borrado
después); los marcados **por inspección** se deducen del código sin ejecución.
No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/config/ -count=1
-cover` → **ok** (4 tests, cobertura 84.4 %). `go vet ./internal/config/`
limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | MEDIO | `Load()` acepta una `version` futura (99) sin error: el manifiesto se interpreta con las reglas de la versión 1 | `manifest.go:58` |
| A2 | MEDIO | `Load()` devuelve un manifiesto **vacío** (sin error) si el JSON es `null`, `{}` o `{"programs": null}`: el usuario pierde sus programas sin aviso | `manifest.go:55` |
| A3 | MEDIO | `Get()` devuelve un puntero al slice: un `Remove()` posterior deja el puntero apuntando a otro programa | `manifest.go:80` |
| A4 | BAJO | `Save()` no es atómico: un corte deja el manifiesto truncado y `Load()` falla después | `manifest.go:73` |
| A5 | BAJO | `Remove()` no compacta el slice: deja el último elemento duplicado en el array subyacente | `manifest.go:105` |
| B1 | BAJO | `Add()` y `AddProgram()` no validan `name` ni `source`: se puede registrar un programa con nombre vacío | `manifest.go:93` |

---

## MEDIO

### A1 — `Load()` acepta una versión de formato futura sin avisar

**Ubicación:** `manifest.go:54-61`, `manifest.go:58` (el `if m.Version == 0`).

**Qué pasa.**

```go
var m Manifest
if err := json.Unmarshal(data, &m); err != nil {
    return nil, err
}
if m.Version == 0 {
    m.Version = Schema
}
```

El único control de versión es "si es 0, poné 1". Un manifiesto con
`"version": 99` —de una versión futura del CLI, o de un archivo editado a
mano— se carga **sin error** y se interpreta con las reglas de la versión 1.

**Verificado** con un probe:

```
contenido: {"version": 99, "programs": [{"name":"x"}]}
   → Load() → version=99  programs=1  err=<nil>
```

El README (README.md:88-90) dice: "**`Version` con default 1.** Si el archivo
no tiene `version`, se le pone `Schema`. Los archivos viejos siguen sirviendo;
la versión está para cuando el formato cambie de verdad". O sea, la versión
está para **cuando el formato cambie**, pero el día que cambie, este código
no va a proteger nada: un CLI viejo va a leer un manifiesto nuevo sin
enterarse y a interpretarlo con la gramática vieja. El campo `Version` existe
precisamente para poder **rechazar** lo desconocido, y no se usa para eso.

**Fix propuesto.** Rechazar las versiones que no conoces:

```go
switch {
case m.Version == 0:
    m.Version = Schema
case m.Version > Schema:
    return nil, fmt.Errorf("manifiesto de una versión más nueva (%d); actualizá el CLI", m.Version)
}
```

Y, si se quiere tolerar el futuro, al menos avisar por stderr en vez de
aceptar en silencio.

---

### A2 — `Load()` devuelve un manifiesto vacío (sin error) ante un JSON semiválido

**Ubicación:** `manifest.go:54-61`.

**Qué pasa.** `json.Unmarshal` acepta `null`, `{}` y `{"programs": null}` sin
error: son JSON válidos. El manifiesto resultante tiene `Programs == nil` y
`Version` corregido a 1.

**Verificado** con un probe:

```
content="null"             → version=1  programs=0  (nil=true)  err=<nil>
content="{}"               → version=1  programs=0  (nil=true)  err=<nil>
content={"programs": null} → version=1  programs=0  (nil=true)  err=<nil>
```

El módulo tiene dos modos de falla documentados: el archivo **no existe**
→ se devuelve el default (`manifest.go:47-49`), y el archivo **está
corrupto** → se devuelve el error de parseo (`manifest.go:55-57`). Este
tercer caso —**el archivo existe y es JSON válido pero no tiene programas**—
cae en el limbo: no es el default y no es un error. El usuario ve un CLI
que dice "no tenés programas registrados" y no tiene idea de que su archivo
se vació.

Y el daño se perpetúa: `cmd/b3l0w/programs.go:164-166` guarda el manifiesto
después de cada `add`/`remove`, así que un `b3l0w program add x` sobre un
manifiesto en este estado **reescribe el archivo con un solo programa** y los
demás desaparecen para siempre.

**Fix propuesto.** Distinguir "manifiesto vacío" de "manifiesto que dice que
está vacío": si el JSON parsea pero no trae la lista de programas, tratarlo
como corrupto (error explícito) en vez de devolver un manifiesto vacío. Como
mínimo, `if len(m.Programs) == 0 && el archivo tenía contenido` → error
"manifiesto sin programas: se borró o se editó a mano".

---

### A3 — `Get()` devuelve un puntero al slice, y `Remove()` lo deja colgando

**Ubicación:** `manifest.go:77-84` (`Get`), `manifest.go:102-110` (`Remove`),
`manifest.go:105` (el `append` que compacta).

**Qué pasa.** `Get` devuelve `&m.Programs[i]`, un puntero **dentro** del
slice. `Remove` compacta el slice **en el mismo array**:

```go
m.Programs = append(m.Programs[:i], m.Programs[i+1:]...)
```

El `append` copia los elementos hacia adelante, sobreescribiendo la posición
`i`. Un puntero obtenido antes a un elemento **posterior** a `i` ahora apunta
a un programa distinto.

**Verificado** con un probe:

```
m := Default()               // [boot-preview, kitty-shapes]
m.Add("extra", "x")          // [boot-preview, kitty-shapes, extra]
p := m.Get("kitty-shapes")   // puntero al índice 1
m.Remove("boot-preview")     // compacta: [kitty-shapes, extra]
   → p.Name == "extra"       // el puntero ahora apunta a otro programa
```

El README (README.md:97-99) vende el puntero como una decisión: "**`Get`
devuelve un puntero al elemento.** El `Program` que se obtenga es el del
slice, no una copia: modificarlo modifica el manifiesto". Eso es cierto, pero
el README no dice que el puntero **caduca** con la siguiente mutación, y
`cmd/b3l0w/programs.go:48-59` usa exactamente ese patrón:

```go
program := m.Get(name)
if program == nil {
    m.AddProgram(...)
    m.Save(...)
    program = m.Get(name)   // vuelve a pedirlo: sí, el código ya lo sabe
}
```

El código del CLI lo sortea repidiendo el `Get`. `internal/programs/uninstall.go:16`
usa `m.Get(name) == nil` como comprobación y `m.Remove(name)` después: ahí
el puntero no se usa después del `Remove`, así que no hay bug hoy. Pero la API
es una trampa esperando a un consumidor.

**Fix propuesto.** Dos salidas, ambas válidas:

1. Que `Get` devuelva una **copia** (`*Program` con los valores copiados) y
   que exista un `Set`/`Update` explícito para modificar. El README hay que
   cambiarlo.
2. Que `Remove` no compacte en el lugar: `m.Programs = append(m.Programs[:i:i],
   m.Programs[i+1:]...)` con el límite de capacidad `[:i:i]` para forzar una
   copia, de modo que los punteros emitidos antes sigan apuntando a su
   elemento (que ya no está en el slice, pero no apunta a otro).

---

## BAJO

### A4 — `Save()` no es atómico

`manifest.go:65-74`: el guardado es `os.WriteFile(path, ..., 0o644)`, que
**trunca** el archivo y después escribe. Un corte (SIGKILL, energía) deja el
manifiesto a medias, y el próximo `Load` falla con un error de JSON que no
dice "tu manifiesto se cortó" sino "unexpected end of JSON input". El CLI se
queda sin manifiesto hasta que el usuario lo arregla a mano. Es el mismo
patrón que el `saveState` de `internal/dotfiles` (ver su BUGS.md, A4), allá
con la diferencia de que el estado del dotfiles tiene un mensaje de
recuperación y este no.

Es más grave que en dotfiles porque el manifiesto **no se regenera**: el
default de `Load` solo aplica si el archivo no existe.

**Fix propuesto.** Escribir a `path + ".tmp"`, `fsync` y `rename` (reescritura
atómica). Y en `Load`, cuando el JSON no parsea, renombrar el corrupto a
`programs.json.corrupt` y devolver un error que diga cómo recuperar.

### A5 — `Remove()` no compacta el slice: el último queda duplicado

`manifest.go:105`: `m.Programs = append(m.Programs[:i], m.Programs[i+1:]...)`
copia los elementos hacia adelante pero **no limpia la cola** del array
subyacente: la última posición conserva una copia del elemento que se
movió. No se ve desde afuera (el slice tiene el `len` correcto) pero
mantiene vivo un `Program` que ya no está en el manifiesto y puede
confundir a un `Get` con un índice fuera de rango, o a un serializador que
mire el array de atrás. `copy` + `m.Programs[len-1] = Program{}` lo
resuelve.

### B1 — `Add()` y `AddProgram()` no validan `name` ni `source`

`manifest.go:87-99`: `AddProgram` solo comprueba que el nombre no esté
repetido. No valida que el nombre no sea vacío ni que la fuente tenga forma
`github:owner/repo`. `internal/programs/install.go:19-30` valida el `Asset` y
la fuente **al usar**, no al registrar: el programa queda en el manifiesto y
falla recién cuando se lo quiere instalar. `cmd/b3l0w/programs.go:48-55`
pasa lo que el usuario escribió. Validar en `AddProgram` (mismo criterio que
`system.SafeName` para el nombre) y devolver el error ahí, no tres comandos
después.

---

## Lo que no encontré

- **`Default()` devuelve un manifiesto nuevo cada vez** (`manifest.go:33-41`),
  con dos programas: `boot-preview` y `kitty-shapes`. No hay estado global
  compartido: dos llamadas a `Default()` no se aliasean. Correcto.
- **`Load()` devuelve el default cuando el archivo no existe**
  (`manifest.go:47-49`): es la regla del README (README.md:74-76) y hace que
  el CLI funcione recién instalado. Correcto.
- **`Save()` crea el directorio** (`manifest.go:66-68`) con `0755` y escribe
  el archivo con `0644`, con JSON indentado a dos espacios y `\n` final
  (`manifest.go:69-73`). Coincide con el README (README.md:92-95).
- **`Get` es O(n)**: recorre el slice. Con una lista de programas cortos es
  irrelevante.
- **`Add` es idempotente por nombre** (`manifest.go:94-96`): agregar dos
  veces el mismo nombre devuelve `false` y no duplica. El README
  (README.md:78-81) lo documenta y es el patrón "agregá y avisá si algo
  cambió". Correcto.
- **`Remove` devuelve `false` si no estaba** (`manifest.go:109`): misma
  convención que `Add`. Correcto.
- **El `Schema = 1`** (`manifest.go:12`) y el `Version` que se escribe en el
  JSON coinciden: el archivo guardado tiene `"version": 1`.
- **La ruta del manifiesto no está en este módulo**: sale de
  `paths.ManifestPath()` (`kit/paths/paths.go:24`), que respeta
  `XDG_CONFIG_HOME`. El README (README.md:9) lo dice bien.

## Cómo verifiqué

Tests temporales en `internal/config/zz_probe_test.go` (package `config`),
ejecutados con `go test ./internal/config/ -run TestProbe -v` y borrados al
terminar. Cubrieron: la versión 99 aceptada (A1), los tres JSON
semiválidos que devuelven un manifiesto vacío (A2), y el puntero de `Get`
caducado por un `Remove` (A3). El estado final del worktree no tiene
archivos de probe.
