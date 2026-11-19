# BUGS — internal/channel

Revisión de integridad en contexto cero sobre `internal/channel` (HEAD
`fb76242`).

Se leyó el archivo de producción `channel.go` (61 líneas), su `README.md`, el
`*_test.go` y los consumidores en `cmd/b3l0w/update.go` e
`internal/update/update.go`. Los hallazgos marcados **verificado** se
reprodujeron con un test temporal en el propio paquete (`zz_probe_test.go`,
borrado después); los marcados **por inspección** se deducen del código sin
ejecución. No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/channel/
-count=1 -cover` → **ok** (2 tests, cobertura 60.0 %). `go vet
./internal/channel/` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Current()` no valida lo que lee del archivo: un canal escrito a mano ("nightly", "EDGE") se devuelve tal cual | `channel.go:29` |
| A2 | ALTO | `Current()` devuelve "" si el archivo no existe y `Detect()` también puede fallar: el canal queda vacío y el consumidor no lo sabe | `channel.go:33` |
| A3 | MEDIO | `Detect()` corre `pacman -Q` **dos veces** por llamada, y las dos veces con la salida al stdout del proceso | `channel.go:38` |
| A4 | MEDIO | `Path()` no respeta `XDG_CONFIG_HOME`: usa `~/.config/b3l0w` fijo mientras el resto del CLI usa `paths.ConfigDir()` | `channel.go:23` |
| A5 | MEDIO | `installed()` no distingue "no instalado" de "pacman no está" o "pacman falló": cualquier error es "no instalado" | `channel.go:60` |
| B1 | BAJO | `Set()` no es atómico: un corte deja el archivo a medias y `Current()` lee un canal sin la palabra completa | `channel.go:56` |

---

## ALTO

### A1 — `Current()` no valida lo que lee del archivo

**Ubicación:** `channel.go:27-34`, `channel.go:29` (el `TrimSpace`).

**Qué pasa.**

```go
func Current() string {
    if data, err := os.ReadFile(Path()); err == nil {
        if value := strings.TrimSpace(string(data)); value != "" {
            return value
        }
    }
    return Detect()
}
```

Se lee el archivo, se le quita el espacio y se devuelve **lo que sea**,
sin comprobar que sea `stable` o `edge`. La validación existe, pero **solo
en la escritura** (`Set`, `channel.go:46`).

**Verificado** con un probe:

```
archivo con "nightly"  → Current() == "nightly"
archivo con "EDGE"     → Current() == "EDGE"
```

El archivo está en `~/.config/b3l0w/channel`: lo puede editar el usuario a
mano, lo puede escribir otro programa, lo puede dejar a medias un
intercambio de archivos. Nada lo valida al leer.

El daño está en el consumidor. `internal/update/update.go:599-603`:

```go
ch := channel.Current()
pkgs := []string{"umbriel-git", "noctalia-git"}
if ch == channel.Stable {
    pkgs = []string{"umbriel", "noctalia"}
}
```

Cualquier valor que no sea exactamente `"stable"` cae en la rama **edge**:
el CLI le va a decir al usuario que actualice `umbriel-git` y `noctalia-git`
—los paquetes de desarrollo— cuando el usuario está en stable y lo que
tiene es un typo en el archivo del canal. Y con "nightly" (un canal que no
existe) el mensaje imprime `canal: nightly` (`cmd/b3l0w/update.go:98`).

El README (README.md:59-61) dice: "**`Set` valida antes de escribir.** Un
canal que no sea `stable` ni `edge` es un error, no un archivo con garbage
adentro". La segunda mitad de esa frase es justamente la que no se cumple:
**sí puede haber garbage adentro**.

**Fix propuesto.** Validar también en la lectura: si el valor no es
`Stable` ni `Edge`, ignorarlo y caer a `Detect()` (o devolver un error).
Extraer la validación a una función sola y usarla en `Set` y en `Current`:

```go
func valido(v string) bool { return v == Stable || v == Edge }
```

---

### A2 — `Current()` puede devolver un canal vacío sin que nadie lo note

**Ubicación:** `channel.go:27-34`, `channel.go:33` (`Detect`).

**Qué pasa.** `Current()` devuelve `Detect()` cuando el archivo no existe o
está vacío. `Detect()` (`channel.go:37-42`) devuelve `Stable` o `Edge`, nunca
vacío — **pero** ambos dependen de que `installed()` funcione:

```go
func Detect() string {
    if installed("umbriel-git") || installed("noctalia-git") {
        return Edge
    }
    return Stable
}
```

`installed()` es `exec.Command("pacman", "-Q", pkg).Run() == nil`
(`channel.go:60`). Si `pacman` no está en el PATH, el comando falla, el
error se descarta y el resultado es "no instalado" → `Detect()` devuelve
`Stable`. En una máquina sin pacman (o con pacman roto, o sin permisos para
la base de datos), el canal reportado es **stable** aunque el usuario pueda
tener el escritorio de desarrollo.

O sea: el fallback silencioso y el default silencioso se refuerzan. El
"stable" inventado se propaga a `internal/update/update.go:600` (que
recomienda los paquetes de stable) y a `cmd/b3l0w/update.go:98` (que
imprime `canal: stable`).

No es un crash ni una pérdida de datos: es un reporte **falso**. El README
(README.md:54-57) presenta `Detect()` como "el estado real de la máquina, no
una preferencia que alguien pueda olvidar", y cuando la máquina no tiene
pacman, el estado real no es ninguno de los dos.

**Fix propuesto.** `installed()` debería distinguir tres casos: instalado,
no instalado, y "no se pudo preguntar" (pacman ausente o error). `Detect()`
debería devolver el error en el tercer caso, y `Current()` propagarlo, en
vez de inventar un canal.

---

## MEDIO

### A3 — `Detect()` corre `pacman -Q` dos veces y deja la salida al stdout

**Ubicación:** `channel.go:38` (la llamada), `channel.go:59-61`
(`installed`).

**Qué pasa.** `Detect()` pregunta por dos paquetes:

```go
if installed("umbriel-git") || installed("noctalia-git") {
```

Como `||` es corto-circuito, si `umbriel-git` está instalado corre **una**
consulta; si no, corre **dos**. Cada una es un proceso de pacman que arranca
(~50-100 ms cada uno).

Peor: `exec.Command("pacman", "-Q", pkg).Run()` **no captura la salida**.
Cuando el paquete no está instalado, pacman escribe en su stderr
("error: package 'umbriel-git' was not found") y, con `-Q` sobre un paquete
inexistente, también en stdout. Como `cmd.Stdout`/`cmd.Stderr` son `nil`,
**la salida va al stdout/stderr del proceso padre**: el usuario ve el error
de pacman impreso en medio de la salida del CLI.

Verificado en esta máquina: `pacman -Q umbriel-git` escribe
`error: package 'umbriel-git' was not found` y sale con 1. Ese texto aparece
en la terminal del usuario cada vez que se consulta el canal y el paquete no
está instalado.

Lo correcto es una sola consulta: `pacman -Q umbriel-git noctalia-git` (que
acepta varios paquetes y devuelve 1 si alguno falta, pero con la salida
capturada) o, mejor, `pacman -Qq` con la salida descartada.

**Fix propuesto.** Capturar la salida (`cmd.Stdout`/`cmd.Stderr` a
`io.Discard`) y hacer una sola consulta para los dos paquetes.

---

### A4 — `Path()` no respeta `XDG_CONFIG_HOME`

**Ubicación:** `channel.go:18-24`, `channel.go:23`.

**Qué pasa.**

```go
func Path() string {
    home, err := os.UserHomeDir()
    if err != nil {
        return ""
    }
    return filepath.Join(home, ".config", "b3l0w", "channel")
}
```

El path está hardcodeado a `~/.config/b3l0w/channel`. Pero el resto del CLI
usa `kit/paths`, que **sí** respeta `XDG_CONFIG_HOME`:

```go
// kit/paths/paths.go:7-13
func ConfigDir() string {
    if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
        return filepath.Join(dir, "b3l0w")
    }
    ...
}
```

Y el manifiesto de programas —que vive en el mismo `~/.config/b3l0w/`— sale
de `paths.ManifestPath()` (`kit/paths/paths.go:24`), o sea de `ConfigDir()`.

Con `XDG_CONFIG_HOME` seteado, el CLI tiene **dos directorios de config
distintos**: el manifiesto de programas va a `$XDG_CONFIG_HOME/b3l0w/` y el
canal se queda en `~/.config/b3l0w/`. El README del módulo (README.md:24,
README.md:41-44) dice "Vive en el home del usuario, no en `/etc`", lo cual
es cierto, pero no menciona que rompe la convención del resto del proyecto.

**Fix propuesto.** Usar `paths.ConfigDir()`:
`filepath.Join(paths.ConfigDir(), "channel")`. Es un cambio de una línea y
deja el canal junto al manifiesto.

---

### A5 — `installed()` no distingue "no instalado" de "pacman falló"

**Ubicación:** `channel.go:59-61`.

**Qué pasa.**

```go
func installed(pkg string) bool {
    return exec.Command("pacman", "-Q", pkg).Run() == nil
}
```

Cualquier fallo es "no instalado": pacman ausente, base de datos corrupta,
permisos, o el propio paquete con un error. Pacman sale con 1 tanto para
"no instalado" como para otros errores, así que el código de salida no
alcanza a distinguirlos — pero el **stderr** sí dice cuál fue.

Esto se combina con A2: el fallo de pacman se traduce a un canal `stable`
inventado.

**Fix propuesto.** Capturar la salida de pacman y mirar si dice
"was not found" (no instalado) o algo más (error). Devolver tres estados
(instalado / no instalado / error) en vez de un booleano, y que `Detect()`
propague el error.

---

## BAJO

### B1 — `Set()` no es atómico

`channel.go:56`: `os.WriteFile(path, []byte(name+"\n"), 0o644)` trunca y
después escribe. Un corte deja el archivo a medias ("st", "e", "") y
`Current()` lo devuelve tal cual (ver A1): el canal queda en un estado que
nadie pidió. Con un archivo de 6 bytes la ventana es mínima, pero el efecto
es el mismo: un canal escrito a medias se propaga al consumidor.

**Fix propuesto.** Escribir a `path + ".tmp"` y `rename`. Y como el valor es
una constanteKnown, `Set` podría usar `os.WriteFile` sobre un temporal con
`fsync`.

---

## Lo que no encontré

- **`Set()` valida antes de escribir** (`channel.go:46-48`): un canal que no
  sea `stable` ni `edge` es un error con el mensaje exacto que documenta el
  README (README.md:59-61). Correcto en el camino de escritura.
- **`Set()` crea el directorio** con `0755` y escribe el archivo con `0644`
  (`channel.go:53-56`), como dice el README (README.md:63-64). Correcto.
- **`Path()` devuelve "" si no hay home** (`channel.go:20-22`) y `Set` lo
  maneja con un error explícito ("no se pudo resolver el home",
  `channel.go:51`). Correcto: no escribe en un path vacío.
- **`Current()` trimmea el contenido** (`channel.go:29`): un archivo con
  `"stable\n"` (que es lo que escribe `Set`) se lee bien. El problema no es
  el trim, es la falta de validación (A1).
- **`Current()` con el archivo vacío (solo espacios) cae a `Detect()`**:
  verificado con el probe, devuelve `stable` (o `edge` según la máquina).
  Correcto.
- **La detección mira los dos paquetes de desarrollo**
  (`channel.go:38`): `umbriel-git` o `noctalia-git` → edge. Es lo que
  documenta el README (README.md:14-15, README.md:54-57) y lo que el
  instalador deja en la máquina. Correcto.
- **`Detect()` es consultable por sí mismo** (`channel.go:37`): el README
  (README.md:66-68) lo justifica y `internal/update/update.go` lo usa vía
  `Current()`. La separación existe.

## Cómo verifiqué

Tests temporales en `internal/channel/zz_probe_test.go` (package `channel`),
ejecutados con `go test ./internal/channel/ -run TestProbe -v` y borrados al
terminar. Cubrieron: un archivo con "nightly" y con "EDGE" devueltos tal cual
(A1) y el archivo vacío cayendo a `Detect()` (A2). Se comprobó además en la
máquina real que `pacman -Q umbriel-git` escribe "error: package ... was not
found" y sale con 1 (A3, A5). El estado final del worktree no tiene
archivos de probe.

---

## Arreglado

| ID | Arreglo | Test |
|---|---|---|
| A1 | `Current()` acepta solo `stable` o `edge`. Un valor desconocido en el archivo se ignora y se cae a `Detect()`. | `TestCurrentIgnoraUnCanalDesconocido` |

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| B1 | `Set` escribe a un temporal y renombra: un corte deja el canal viejo, no uno a medias | `TestSetNoDejaTemporales` |
