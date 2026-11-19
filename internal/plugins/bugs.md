# BUGS — internal/plugins

Revisión de integridad en contexto cero sobre `internal/plugins` (HEAD `fb76242`).
Este paquete es la **capa transversal** que comparten los cuatro plugins de
Noctalia (`we`, `docker`, `anilist`, `ytmusic`): catálogo, manifiesto, instalación,
source y extensiones del CLI. Los plugins individuales viven en
`internal/plugins/<nombre>/` y quedan fuera de esta revisión.

Se leyeron los 7 archivos de producción (`exec.go`, `install.go`, `plugins.go`,
`registry.go`, `remove.go`, `scripts.go`, `source.go`), los 3 `*_test.go`, el
`README.md` y los puntos de integración en `cmd/b3l0w/plugins.go` y
`cmd/b3l0w/main.go`. Los hallazgos marcados **verificado** se reprodujeron con
tests temporales en `t.TempDir()` (archivo `zz_probe_test.go`, borrado después);
los marcados **por inspección** se deducen del código sin ejecución. No se
modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/plugins/ -count=1
-cover` → **ok** (8 tests, cobertura 28.8 %). `go vet ./internal/plugins/` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| C1 | ALTO | `installOne` borra **fuera** del directorio de plugins: `entry.Path` del manifiesto se usa crudo en un `os.RemoveAll` | `install.go:200` |
| C2 | ALTO | `slug()` devuelve `..` para un id con `..`: `installOne` borra el directorio **padre** del de plugins | `install.go:196` |
| A1 | ALTO | `Find()` no restringe el nombre al directorio privado: traversal → el CLI ejecuta binarios de cualquier parte del disco | `plugins.go:48` |
| A2 | MEDIO | `Discover()` y `Find()` discrepan sobre symlinks: se listan extensiones que no se pueden correr | `plugins.go:37-38` |
| A3 | MEDIO | `--dry-run` de install/update **agrega el source a Noctalia**: efecto real con el flag de preview | `install.go:35` |
| A4 | MEDIO | `installOne` borra la versión instalada **antes** de descargar la nueva: un fallo de red deja al usuario sin plugin | `install.go:196` |
| A5 | MEDIO | El parser de `plugin.toml` no entiende arrays TOML de varias líneas: `dependencies` sale con comentarios adentro | `registry.go:289` |
| A6 | MEDIO | `AutoUpdate()` busca `auto_update` como prefijo de línea, no como clave de `[plugins]`: lee el valor equivocado | `source.go:69` |
| A7 | MEDIO | `repairSourceRepo()` es código muerto: el README y el docstring de `UpdateSource` describen una reparación que no ocurre | `source.go:162` |
| A8 | MEDIO | `SupportedPluginAPI` no se aplica en ningún lado: el CLI acepta cualquier `plugin_api >= 1` | `registry.go:39` |
| B1 | BAJO | `parse()` rechaza un manifiesto con lista de plugins vacía como si fuera inválido | `registry.go:76` |
| B2 | BAJO | El prefijo del tarball (`b3l0w-plugins-main/`) está hardcodeado: si cambia el repo o la rama, el error miente | `install.go:214` |
| B3 | BAJO | `updateFromSource` imprime las versiones al revés: "manifiesto -> source" | `install.go:172` |
| B4 | BAJO | `TestDependencies` depende del estado real del home y de la red: con `HOME` aislado falla | `deps_test.go:6` |

---

## ALTO

### C1 — `installOne` borra fuera del directorio de plugins con el `Path` del manifiesto

**Ubicación:** `install.go:200` (`os.RemoveAll`), con el origen del dato en
`registry.go:41-50` (`Entry.Path`).

**Qué pasa.** `Entry.Path` viene del JSON del manifiesto (el embebido
`plugins.json` o el que baja el registry) y se usa **crudo** en un
`os.RemoveAll`:

```go
if entry.Path != slug(entry) {
    if err := os.RemoveAll(filepath.Join(pluginsDir, entry.Path)); err != nil {
```

`filepath.Join` normaliza los `..`, así que un manifiesto con
`"path": "../../.."` borra `~/.local` en vez de una carpeta de plugin. El
manifiesto por defecto se baja de la red (`registry.go:26-32` →
`registry.Load(registry.Plugins, embedded)`), o sea que un manifiesto
manipulado alcanza el `RemoveAll`.

**Contraste dentro del mismo paquete.** `Remove()` sí valida: `remove.go:34`
llama a `system.SafeName(dir)` y aborta con "directorio inválido en el
manifiesto". La validación existe y `installOne` no la usa.

**Fix propuesto.** Validar `entry.Path` (y `slug(entry)`) con
`system.SafeName` antes de cualquier `RemoveAll`, igual que hace `remove.go`.
O mejor: borrar solo el directorio que se va a crear (`dest`), nunca un
segundo path tomado del manifiesto.

---

### C2 — `slug()` no valida el id: `..` escapa del directorio de plugins

**Ubicación:** `registry.go:95-98` (`slug`), `install.go:190` (`dest`),
`install.go:196` (`RemoveAll`).

**Qué pasa.** `slug()` toma lo que sigue a la última barra del id:

```go
if i := strings.LastIndex(e.ID, "/"); i >= 0 && i+1 < len(e.ID) {
    return e.ID[i+1:]
}
return e.Path
```

Con `id = "b3l0w/../../.."` devuelve `".."` (verificado con probe), y
`filepath.Join(pluginsDir, "..")` da el **padre** del directorio de plugins.
`installOne` borra ese path (`install.go:196`) y después crea `dest` con
`MkdirAll`. Resultado: `b3l0w plugins install` borra `~/.local/share/noctalia`.

El mismo `slug()` alimenta `install.go:190` y `install.go:214` (prefijo del
tarball) y `remove.go:31` — donde sí está protegido por `SafeName`.

**Fix propuesto.** `slug()` debería rechazar ids con `..` o con `/` en el
resultado, o normalizar y validar con `system.SafeName` antes de devolver.

---

### A1 — `Find()` no restringe el nombre al directorio privado

**Ubicación:** `plugins.go:48` (`Find`), `plugins.go:57-66` (`Run`).

**Qué pasa.** `Find` construye la ruta con el nombre crudo:

```go
path := filepath.Join(paths.PluginsDir(), Prefix+name)
info, err := os.Stat(path)
if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
    return ""
}
```

`filepath.Join` resuelve los `..`, así que un nombre con traversal apunta a
cualquier ejecutable del disco. **Verificado:** con
`XDG_DATA_HOME=/tmp/x`, `Find("a/../../../../../bin/sh")` devuelve
`"/bin/sh"` (el `Prefix` `b3l0w-` se pega al nombre y el `..` lo atraviesa).

Y no es una función muerta: `cmd/b3l0w/main.go:78-83` despacha **cualquier**
comando desconocido a `plugins.Run(command, rest)`, que ejecuta ese binario
heredando la terminal.

**Por qué importa.** El invariante del paquete (README.md:22-24) es que las
extensiones se corren **solo** del directorio privado, "nunca en el PATH",
justamente para que cualquiera que pueda escribir en el PATH nologe poner un
`b3l0w-update` y ejecutarlo con este usuario. Con el traversal, el directorio
privado deja de ser una frontera.

**Fix propuesto.** En `Find`, rechazar nombres que contengan `/` o `..` (o
comprobar con `filepath.Rel` que el resultado siga dentro de
`paths.PluginsDir()`). Lo mismo para `ExecScript`, que hoy depende de que
`embed.FS` rechace el path.

---

## MEDIO

### A2 — `Discover()` y `Find()` discrepan sobre symlinks

**Ubicación:** `plugins.go:37-38` (`Discover`), `plugins.go:49-50` (`Find`).

**Qué pasa.** `Discover` usa `entry.Info()`, que para una entrada de
`os.ReadDir` describe **el symlink mismo** (modo `0777`, siempre con bit de
ejecución). `Find` usa `os.Stat`, que **sigue** el symlink.

**Verificado** con un directorio de plugins de prueba:

```
Discover() = [dirlink link]
  Find("dirlink") = ""                      (symlink a un directorio)
  Find("link")    = ".../b3l0w-link"         (symlink a /bin/sh)
```

Dos consecuencias: `b3l0w extensions` lista `b3l0w-dirlink` y al correrlo el
CLI dice "no existe la extensión"; y un symlink a un ejecutable fuera del
directorio aparece listado y es ejecutable, que es justo lo que el
directorio privado debía impedir.

**Fix propuesto.** Usar la misma semántica en los dos: `Find` con `os.Lstat`
y chequeo de `ModeSymlink`, o `Discover` Stat-seguidor con el filtro de
directorios que ya hace `Find`.

---

### A3 — `--dry-run` de install/update agrega el source a Noctalia

**Ubicación:** `install.go:35` (`EnsureSource`), `install.go:56` (mismo camino
en `Update`), `source.go:19-31` (`EnsureSource`).

**Qué pasa.** `Install` primero pregunta si el source está y, si no,
**agrega el source a Noctalia** antes de mirar `dryRun`:

```go
if sourceReady() {
    return installFromSource(targets, dryRun)
}
if err := EnsureSource(); err == nil && sourceReady() {
    return installFromSource(targets, dryRun)
}
```

`EnsureSource` corre `noctalia msg plugins source add b3l0w git <url>`
(`source.go:26`). O sea: en una máquina con Noctalia pero sin el source
`b3l0w`, `b3l0w plugins install --dry-run` **deja el source agregado**.
`installFromSource` respeta el `dryRun` (`install.go:78-83`), pero es tarde:
la mutación ya ocurrió.

El README (README.md:75) dice que todos los comandos aceptan `--dry-run`, lo
que sugiere que el preview no toca nada.

**Fix propuesto.** Mover la llamada a `EnsureSource` adentro de la rama
`!dryRun`, o devolver temprano con el preview antes de preguntar por el
source.

---

### A4 — `installOne` borra la versión instalada antes de descargar la nueva

**Ubicación:** `install.go:196` (`RemoveAll`), `install.go:208`
(`downloadTarball`).

**Qué pasa.** El orden es: borrar la carpeta actual → `MkdirAll` → **bajar el
tarball** → extraer. Si la descarga falla (red caída, HTTP != 200), el error
vuelve pero la carpeta vieja ya está borrada y quedó un directorio vacío en
`~/.local/share/noctalia/plugins/<slug>`.

El usuario pierde la versión que tenía funcionando por un fallo de red
puntual, y el directorio vacío confunde a `Origin`/`InstalledVersion`
(`install.go:340`, `install.go:351`), que leen `plugin.toml` de adentro.

**Fix propuesto.** Bajar y extraer a un temporal y recién ahí reemplazar el
destino (`rename`), o al menos no borrar hasta tener el tarball en disco.

---

### A5 — El parser de `plugin.toml` no entiende arrays TOML de varias líneas

**Ubicación:** `registry.go:289` (`reDeps`), `registry.go:326-341` (el parseo),
`registry.go:288` (`reTags`, mismo problema).

**Qué pasa.** El parser es una regex sobre el texto plano:

```go
reDeps = regexp.MustCompile(`(?m)^dependencies\s*=\s*\[([^\]]*)\]`)
```

`[^\]]` también matchea saltos de línea, así que un array multilínea se
captura entero, **con los comentarios `#` de adentro**, y después se parte
por comas (`registry.go:335-340`).

**Verificado** contra los `plugin.toml` reales de esta máquina (el de
`screen-toolkit` tiene el array de dependencias partido en varias líneas con
comentarios):

```
deps=[# Pixel pipelines over shell captures plus the window-picker click waiter.
          "slurp hyprpicker tesseract imagemagick zbar qrencode curl jq translate-shell
          # Recording backends and their helpers.
          "ffmpeg ffprobe gpu-screenrec ... ]
```

El consumidor es `doctor`: `internal/doctor/doctor.go:261` usa
`Dependencies()` para avisar "falta dependencia X" → **falsos positivos** con
nombres de dependencia que son texto de comentario.

**Fix propuesto.** Parsear el bloque `[...]` respetando comillas y comentarios,
o dejar de parsear TOML a mano y usar un parser (el repo decidió no tener
dependencias, pero un parser chico y correcto para este subconjunto alcanza).
Alternativa mínima: cortar la captura en el primer `#` de línea propia y
filtrar las entradas que empiezan por `#`.

---

### A6 — `AutoUpdate()` busca `auto_update` como prefijo de línea

**Ubicación:** `source.go:67-77` (el escaneo), `source.go:69` (el `HasPrefix`).

**Qué pasa.** La función no parsea TOML: recorre las líneas del archivo y
devuelve el valor de la **primera** que empieza con `auto_update`:

```go
if !strings.HasPrefix(line, "auto_update") {
    continue
}
```

Dos problemas:

1. No mira la sección: cualquier `auto_update` de cualquier tabla gana.
2. El prefijo matchea también `auto_update_interval`, `auto_update_mode`, etc.

**Verificado** con un `settings.toml` de prueba:

```
[general]
auto_update_interval = 300

[plugins]
auto_update = "none"
   → AutoUpdate() == "300"      (debía ser "none")

[other]
auto_update = "all"

[plugins]
auto_update = "none"
   → AutoUpdate() == "all"       (debía ser "none")
```

En esta máquina el resultado real es `"none"` (el `settings.toml` de Noctalia
tiene `auto_update = "none"` dentro de `[plugins]`, y el `config.toml` tiene
`auto_update = "all"`), o sea que hoy acierta por suerte: el `settings.toml` es
el primero que se lee y su `auto_update` está en `[plugins]`. El consumidor es
`internal/doctor/doctor.go:246`, que decide qué avisos dar.

**Fix propuesto.** Buscar la clave **dentro** de la tabla `[plugins]`
(rastreando la sección actual), y comparar el nombre de la clave exacto, no
un prefijo.

---

### A7 — `repairSourceRepo()` es código muerto

**Ubicación:** `source.go:162` (la función), `source.go:150-157`
(`UpdateSource`), `README.md:89-90` (la documentación).

**Qué pasa.** `repairSourceRepo` está definida y **no la llama nadie**: el
único hit en todo el repo es su propia definición y el comentario de dos
líneas arriba. Y sin embargo:

- `README.md:89-90`: "`repairSourceRepo()` arregla un clon con el working tree
  vacío (todo el índice como "D"), que ya pasó: lo restaura desde HEAD".
- `source.go:150-151` (docstring de `UpdateSource`): "UpdateSource repara el
  clon si quedó con archivos borrados y pide a Noctalia que refresque el
  source".

`UpdateSource` (`source.go:152-157`) solo corre `noctalia msg plugins update
<name>`: **no repara nada**. El comment dice que "nos pasó" y el arreglo
previsto existe pero nunca se enchufa.

**Fix propuesto.** Llamar `repairSourceRepo(name)` desde `UpdateSource` antes
de pedirle el refresh a Noctalia (el `git status --porcelain` que ya hace la
función es el detector), o borrar la función y corregir los dos textos.

---

### A8 — `SupportedPluginAPI` no se aplica en ningún lado

**Ubicación:** `registry.go:37-39` (la constante), `registry.go:129` (el
chequeo real), `README.md:43-44`.

**Qué pasa.** La constante dice:

```go
// SupportedPluginAPI es el plugin_api máximo que este CLI acepta.
const SupportedPluginAPI = 43
```

Pero el único control de `plugin_api` en el paquete es
`registry.go:129`: `if meta.PluginAPI < 1 { ... }`. Un plugin que pida API 99
pasa el build y llega al catálogo. `SupportedPluginAPI` no aparece en ningún
archivo de producción: su único uso es `registry_test.go:120`, y ese test
afirma justamente lo contrario de la constante
(`TestCollectAcceptsNewAPI`: "build no debe rechazar una API nueva").

El README repite el límite como si existiera: "`plugin_api` es la API que el
plugin necesita (el CLI acepta hasta `SupportedPluginAPI`, 43)".

**Nota.** Puede ser una decisión deliberada — que el CLI no sea el que
limite, sino el shell — pero entonces la constante y su comentario mienten,
y el README también. Hoy las tres cosas dicen cosas distintas.

**Fix propuesto.** Decidir y dejarlo escrito: o `Collect` rechaza
`plugin_api > SupportedPluginAPI` (y el test cambia), o se borra la constante
y se corrige el README para decir que el límite lo aplica el shell.

---

## BAJO

### B1 — `parse()` rechaza un manifiesto con lista vacía

`registry.go:76-78`: `if m.Version == 0 || len(m.Plugins) == 0` devuelve
"manifiesto de plugins vacío o sin versión". Un manifiesto válido con
`version: 1` y `plugins: []` (todos los plugins dados de baja) se trata como
archivo inválido, con un mensaje que no dice cuál de las dos condiciones
falló. Separar los dos casos y devolver un error distinto para cada uno.

### B2 — El prefijo del tarball está hardcodeado

`install.go:214`: `prefix := "b3l0w-plugins-main/" + slug(entry) + "/"`. El
nombre de la carpeta raíz de un tarball de GitHub es `<repo>-<rama>`, así que
este string ata el install al repo `b3l0w-plugins` y a la rama `main`. Si el
repo se renombra o cambia la rama por defecto, la extracción no encuentra
nada y el error que sale es "el plugin no trae plugin.toml"
(`install.go:219`), que apunta al plugin y no a la causa. Sacar el prefijo
del primer componente del tarball y no del nombre del repo.

### B3 — `updateFromSource` imprime las versiones al revés

`install.go:172`: `fmt.Printf("  ▸ %s: %s -> %s (source)\n", entry.ID,
entry.Version, meta.Version)`. `entry.Version` es la versión del **manifiesto**
(la destino) y `meta.Version` la que está **instalada** (el origen): el
mensaje se lee "1.0.0 -> 0.9.0" cuando lo que pasa es que el source está más
viejo. El camino gemelo sin source (`install.go:69`) sí usa el orden
correcto (`local -> entry.Version`).

### B4 — `TestDependencies` depende del estado real del home y de la red

`deps_test.go:6` llama a `Dependencies()` → `LoadDefault()` →
`registry.Load(registry.Plugins, embedded)`, que baja `plugins/plugins.json`
de `raw.githubusercontent.com` (o usa el caché), y después busca
`plugin.toml` bajo `DefaultPluginsDir()`, que es
`~/.local/share/noctalia/plugins` y `~/.local/state/noctalia/plugins/materialized`
(`install.go:316-327`).

**Verificado:** con `HOME=$(mktemp -d)` el test **falla**:

```
deps_test.go:11: no leyó ningún plugin
--- FAIL: TestDependencies (0.51s)
```

El mapa de dependencias solo tiene entradas para los plugins que están
materializados en la máquina. En un contenedor o en CI, sin Noctalia
instalado, el test es rojo. Además `t.Fatal` en `deps_test.go:10-11` no
distingue "no hay plugins" de "no se pudo leer".

---

## Lo que no encontré

- **El `plugin_api` embebido.** `plugins.json` tiene 10 plugins, todos con
  `plugin_api: 43`, y `SupportedPluginAPI` es 43: hoy coinciden.
- **La jerarquía del registry.** `LoadDefault()` usa
  `registry.Load(registry.Plugins, embedded)`: caché → red → caché viejo →
  embebido, igual que el resto de la distro. Correcto.
- **La protección contra path traversal en el tarball.**
  `install.go:277-280` arma el destino con `filepath.Join` y exige que
  empiece con `dest + separador`: un `hdr.Name` con `..` se rechaza con
  "ruta insegura en el tar". Correcto (el problema de traversal está en el
  path del manifiesto, no en el del tarball).
- **El parseo de `plugin.toml` para plugins del repo propio.** Los 10 plugins
  de `b3l0w-plugins` que hay en esta máquina tienen `id`, `name`, `version` y
  `plugin_api` en una sola línea y arrays de una línea: el parser acierta con
  ellos. El problema (A5) aparece con arrays multilínea, que sí usan los
  plugins de la comunidad.
- **`Remove()` con `dryRun`.** `remove.go:41-45` imprime el destino y no
  borra: correcto.
- **`ExecScript`.** `exec.go:17` lee de un `embed.FS`, que rechaza paths con
  `..`, así que no hay traversal por ese lado.

## Cómo verifiqué

Tests temporales en `internal/plugins/zz_probe_test.go` (package `plugins`,
`t.TempDir()`), ejecutados con `go test ./internal/plugins/ -run TestProbe -v`
y borrados al terminar. Cubrieron: traversal por `slug()` y por `entry.Path`
(C1, C2), traversal por `Find()` (A1), symlinks en `Discover`/`Find` (A2),
`AutoUpdate()` con tabla ajena y clave `auto_update_interval` (A6),
`Dependencies()` con `HOME` aislado (B4) y el parser contra los
`plugin.toml` reales de la máquina (A5). El estado final del worktree no
tiene archivos de probe (`git status --porcelain` no muestra nada en los cinco
módulos de esta revisión).

---

## Arreglados

Los tres caminos de borrado/ejecución fuera del directorio privado, con
`system.SafeName` como validación (el mismo helper que ya usaba `remove.go`):

| ID | Arreglo | Test |
|---|---|---|
| C1 | `installOne` valida `entry.Path` y el nombre del slug antes de los dos `os.RemoveAll`. | `TestInstallOneNoBorraFueraDelDirectorio` |
| C2 | `slug()` valida con `SafeName` y devuelve `""` si el nombre no es un componente simple: nunca más `..`. | `TestSlugNuncaDevuelveUnPath` |
| A1 | `Find()` valida el nombre antes de componer el path: `"../../bin/sh"` devuelve `""`. | `TestFindRechazaTraversal` |
