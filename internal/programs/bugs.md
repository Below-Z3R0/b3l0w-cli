# BUGS — internal/programs

Revision de integridad sobre `internal/programs` (HEAD `fb76242`).
Se leyó `programs.go` (135 líneas), `install.go` (124), `pacman.go` (67),
`uninstall.go` (53), los 4 `*_test.go` y `README.md`. Se leyeron también
las dependencias que el módulo invoca: `kit/system/system.go`
(`ReplaceFile`, `PackageOwner`, `RunTimeout`, `CommandPath`, `SafeName`),
`kit/fetch/fetch.go` (`ToTemp`), `kit/verify` y `internal/config/manifest.go`.
Los hallazgos marcados **verificado** se reprodujeron con tests temporales
en `t.TempDir()` (archivo `zz_probe_test.go`, borrado después). No se
modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/programs/ -count=1`
→ **ok** (0.447s), 12 tests verdes, cobertura **27.6 %**. `go vet` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Packaged` usa `pacman -Si` (BD **sync**): da `true` para cualquier paquete de cualquier repo, no solo los de `[b3l0w]` | `pacman.go:30` |
| A2 | ALTO | `Uninstall` borra el binario **antes** de guardar el manifiesto: si el save falla, el programa sigue registrado y el archivo ya no está | `uninstall.go:43-51` |
| M1 | MEDIO | `ParseVersion` matchea una fecha como si fuera versión: `"2026.11.06"` → `"v2026.11.06"` | `programs.go:25` |
| M2 | MEDIO | `InstallVerified` deja el binario instalado sin permiso de ejecución (`0644`): no se puede correr | `install.go:91` |
| M3 | MEDIO | `AssetURL` acepta `github:owner/repo/extra`: la ruta extra se cuela en la URL | `install.go:25` |
| M4 | MEDIO | `Uninstall` borra el manifiesto con `paths.ManifestPath()` fijo, ignorando el manifiesto que recibió | `uninstall.go:30,49` |
| M5 | MEDIO | `Install` no valida que el asset exista antes de bajarlo: el error llega tarde y como 404 de curl | `install.go:65-72` |
| B1 | BAJO | `ownedByB3l0w` reconoce `b3l0w-*` por prefijo: un paquete ajeno con ese prefijo se trata como propio | `pacman.go:64-67` |
| B2 | BAJO | `Current` ejecuta `bin version` sin sanitizar: un binario con espacios en la ruta no se puede invocar | `programs.go:51` |
| B3 | BAJO | `Update` no distingue "el update falló" de "el update no salió bien": el mensaje es la última línea | `programs.go:121-123` |

---

## ALTO

### A1 — `Packaged` consulta la BD sync, no la de instalados

**Ubicación:** `pacman.go:26-32`, en particular `pacman.go:30`.

**Qué pasa.** `Packaged` decide si un programa "ya se distribuye como
paquete de `[b3l0w]`" (el comentario en `pacman.go:23-25`) ejecutando:

```go
_, err := system.RunTimeout(5*time.Second, "pacman", "-Si", PackageName(name))
return err == nil
```

`pacman -Si` significa "información de un paquete **disponible** en los
repositorios" (la BD **sync**). `pacman -Qi` es el de los **instalados**
(la BD local). Con `-Si`, la pregunta real no es "¿este programa es
nuestro?" sino "¿existe un paquete con este nombre en algún repo
habilitado?".

**Reproducción (verificado).** En esta máquina, con el repo `[b3l0w]`
habilitado:

```
Packaged("firefox")     = true    ← paquete de cachyos-extra-v3, no nuestro
Packaged("bash")        = true    ← paquete de core, no nuestro
Packaged("kitty-shapes")= true    ← este sí es nuestro
Packaged("b3l0w-cli")   = true    ← este también
```

El nombre de la función y su comentario prometen "¿es un paquete de
`[b3l0w]`?" y la respuesta real es "¿existe en algún repo?". Las dos
coinciden **por casualidad**, porque los nombres de los paquetes propios
(`boot-preview`, `kitty-shapes`) no colisionan con nada.

**Consecuencias.**

1. **`Install` toma el camino equivocado** (`install.go:48-55`): si
   `Packaged(p.Name)` da `true`, `Install` delega en `InstallPackage` y
   **nunca baja el asset del release**. Un programa cuyo nombre choque
   con cualquier paquete de la distro (un `firefox`, un `htop`) se instala
   desde pacman en vez de desde su release de GitHub — y nunca se verifica
   la firma `SHA256SUMS.asc` que el módulo dice exigir
   (`install.go:44-47`).
2. **`Uninstall` destructivo** (`uninstall.go:24-33`): si `Packaged` da
   `true`, `Uninstall` llama a `RemovePackage` → `sudo pacman -Rns
   --noconfirm <paquete>`. Con un nombre que colisione, se **desinstala el
   paquete de la distro**, con `-Rns` (borra también la configuración y las
   dependencias que quedan huérfanas). Esto **no** lo pedía el usuario: el
   usuario pidió quitar el programa del manifiesto.

El segundo punto es el grave: `b3l0w uninstall htop` (con `htop` en el
manifiesto) ejecutaría `sudo pacman -Rns --noconfirm htop`.

**Fix propuesto.** Preguntar por el repo, no por la existencia:

```go
func Packaged(name string) bool {
    if !system.HaveCommand("pacman") {
        return false
    }
    out, err := system.RunTimeout(5*time.Second, "pacman", "-Qi", PackageName(name))
    if err != nil {
        return false
    }
    // -Qi lista "Repository : b3l0w" para los nuestros.
    return strings.Contains(out, "Repository") && ownedByB3l0w(repoDe(out))
}
```

O, si la intención era "¿está disponible para instalar?", separar las dos
preguntas: `Packaged` (¿es nuestro?) y `Available` (¿está en un repo?).
Y en `Uninstall`, el `sudo pacman -Rns` debería llevar el **repo** del
paquete, no adivinar.

**Nota.** El mismo `Packaged` se usa en `Install` (`install.go:51`) y en
`Uninstall` (`uninstall.go:24`), así que el fix tiene que corregir ambos
caminos.

---

### A2 — `Uninstall` borra el archivo antes de guardar el manifiesto

**Ubicación:** `uninstall.go:43-51`.

**Qué pasa.** `Uninstall` hace, en este orden:

```go
if _, err := os.Stat(path); err == nil {
    if err := os.Remove(path); err != nil {   // 1. borra el binario
        return "", fmt.Errorf("no se pudo borrar %s: %w", path, err)
    }
}
m.Remove(name)                                  // 2. saca del manifiesto en memoria
if err := m.Save(paths.ManifestPath()); err != nil {   // 3. lo persiste
    return "", fmt.Errorf("no se pudo guardar el manifiesto: %w", err)
}
```

Si el paso 3 falla (disco lleno, permisos, manifiesto en solo lectura), el
binario **ya fue borrado** y el manifiesto **sigue en disco con el
programa registrado**. El estado queda invertido: el manifiesto dice que
el programa está instalado y el archivo no existe.

El siguiente `b3l0w update` lo trata como "programa instalado": llama a
`programs.Update(name)` (`internal/update/update.go:483`), que da `no está instalado`
(`programs.go:107`) y cuenta como **fallo** (`internal/update/update.go:487-489`), así que
el update entero sale con código 1. Y `b3l0w uninstall <nombre>` otra vez
da `no está registrado`... no: `m.Get(name)` sí lo encuentra (el manifiesto
no se actualizó), así que reintenta, y esta vez `os.Stat(path)` falla y no
hay nada que borrar: termina bien y limpia el manifiesto. El usuario tiene
que correr el uninstall **dos veces** para que el sistema quede consistente.

**Fix propuesto.** Guardar el manifiesto **antes** de borrar el archivo, o
hacer la operación atómica en el otro sentido: preparar el nuevo
manifiesto, guardarlo, y solo después borrar el binario. Si el borrado
falla, el manifiesto ya no lo lista (el programa se considera "desinstalado
con un archivo suelto") y un segundo intento limpia el archivo. Alternativa
mínima: si `m.Save` falla, **restaurar** el binario (no es posible
fácilmente), o al menos avisar con claridad que el manifiesto quedó
desactualizado.

---

## MEDIO

### M1 — `ParseVersion` matchea una fecha como si fuera una versión

**Ubicación:** `programs.go:25` (la regex) y `programs.go:37-43`.

**Qué pasa.** La regex es `v?(\d+\.\d+\.\d+)`: tres dígitos separados
por puntos, en cualquier parte de la salida. Los tags de release de este
proyecto usan **fechas** como versión: `desktop-2026.11.06-r1`,
`desktop-6`. Una salida de `prog version` que contenga esa forma produce
una versión falsa.

**Reproducción (verificado).**

```go
ParseVersion("kitty-shapes 2026.11.06-r1") → "v2026.11.06"
ParseVersion("Copyright 2024.01.15")       → "v2024.01.15"
ParseVersion("prog v1.2.3")                → "v1.2.3"   ← correcto
```

El README afirma que la regex "acepta `v1.2.3` y `1.2.3`" y que es "la
única forma de leer una versión en todo el CLI" (`README.md:79-81`). La
regex es más laxa de lo que dice: acepta cualquier terna de enteros.

**Consecuencia.** `Current` (`programs.go:59-62`) devolvería
`"v2026.11.06"` como versión de un programa, y `check`
(`internal/update/update.go:534`) lo mostraría como si fuera la versión real. La
comparación con el AUR (`internal/update/update.go:588`) usaría `vercmp` sobre dos
versiones sin sentido.

**Fix propuesto.** Anclar la regex a una forma de versión real: exigir que
sea la **última** token de la línea y que no tenga sufijos de fecha. Por
ejemplo `(?m)^\S*\s+v?(\d+\.\d+\.\d+(?:[-+][\w.]+)?)\s*$`, o validar que
el resultado no tenga la forma `YYYY.MM.DD`.

---

### M2 — El binario instalado queda sin permiso de ejecución

**Ubicación:** `install.go:91` (la llamada) y
`kit/system/system.go:167-176` (la implementación).

**Qué pasa.** `InstallVerified` termina con:

```go
if err := system.ReplaceFile(tmp, dest); err != nil {
    return "", fmt.Errorf("no se pudo escribir %s: %w", dest, err)
}
```

`system.ReplaceFile` **conserva el modo del destino** si existe, y si no
existe usa `0o644` (`kit/system/system.go:167-170`):

```go
modo := os.FileMode(0o644)
if info, err := os.Stat(dest); err == nil {
    modo = info.Mode().Perm()
}
```

En la **primera** instalación, `~/.local/bin/<nombre>` no existe: se crea
en `0644`. El asset descargado sí venía en `0755` (`kit/fetch/fetch.go:33`
hace el `chmod`), pero `ReplaceFile` no copia el modo del origen: usa el
del destino, o `0644` si no hay destino.

**Reproducción (verificado).**

```go
src  = 0755 (el asset bajado)
dest = no existe todavia
system.ReplaceFile(src, dest)
// modo del destino: -rw-r--r--   (ejecutable = false)
```

El mismo bug afecta a `internal/selfupdate/selfupdate.go:201`, aunque ahí
el destino (el binario en ejecución) sí existe y conserva el modo: solo
se manifiesta cuando `ReplaceFile` **crea** el archivo.

**Consecuencia.** `b3l0w install kitty-shapes` deja el binario en
`~/.local/bin/kitty-shapes` sin `+x`. El CLI informa
"kitty-shapes v1.0.0 en ~/.local/bin/kitty-shapes" (`cmd/b3l0w/programs.go:81`)
pero el usuario no puede ejecutarlo: `permission denied`.

**Fix propuesto.** En `ReplaceFile`, cuando el destino no existe, usar el
modo del **origen**:

```go
modo := os.FileMode(0o755)
if info, err := os.Stat(dest); err == nil {
    modo = info.Mode().Perm()
} else if info, err := os.Stat(src); err == nil {
    modo = info.Mode().Perm()
}
```

Y en `InstallVerified`, hacer un `os.Chmod(dest, 0o755)` explícito
después de la escritura, que es la garantía que el módulo quiere
("un asset de otra arquitectura no reemplaza algo que funciona",
`README.md:105-107`).

---

### M3 — `AssetURL` acepta una ruta extra y la cuela en la URL

**Ubicación:** `install.go:25`.

**Qué pasa.** Igual que el bug M1 de `internal/releases`:

```go
owner, repo, ok := strings.Cut(strings.TrimPrefix(p.Source, "github:"), "/")
if !ok || owner == "" || repo == "" {
    return "", fmt.Errorf("fuente no soportada para %s: %q (usá github:owner/repo)", p.Name, p.Source)
}
return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", owner, repo, p.Asset), nil
```

`strings.Cut` parte por el primer `/`. Para `github:owner/repo/extra`,
`repo` queda como `"repo/extra"`: es no vacío, así que pasa la validación.

**Reproducción (verificado).**

```go
AssetURL(config.Program{Name:"x", Source:"github:owner/repo/extra", Asset:"a"})
// → "https://github.com/owner/repo/extra/releases/latest/download/a", err = nil
```

La URL resultante no existe: el error llega como un 404 de `curl` al
bajar el asset, no como "fuente mal formada".

**Fix propuesto.** Exigir que `repo` no contenga `/`:

```go
if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
    return "", fmt.Errorf(...)
}
```

---

### M4 — `Uninstall` guarda en una ruta fija, no en la del manifiesto

**Ubicación:** `uninstall.go:30` y `uninstall.go:49`.

**Qué pasa.** `Uninstall` recibe el manifiesto `m *config.Manifest` como
argumento, pero **no** sabe de dónde vino: persiste siempre en
`paths.ManifestPath()`:

```go
if err := m.Save(paths.ManifestPath()); err != nil {
```

`config.Manifest` no guarda su propia ruta (`internal/config/manifest.go`):
`Save(path string)` la recibe como parámetro (`internal/config/manifest.go:65`). Así que
`Uninstall` no puede guardar "en el mismo lugar del que vino" sin
cambiar la API.

Hoy el consumidor (`cmd/b3l0w/programs.go:104`) carga siempre de
`paths.ManifestPath()`, así que **no hay bug observable**. Es deuda de
API: la firma sugiere que el manifiesto recibido es el que se va a
modificar, y no es cierto.

**Fix propuesto.** Que `Manifest` guarde su `path` al cargarse, o que
`Uninstall` reciba la ruta. Alternativa mínima: que `config.Manifest` tenga
un campo `path string` no serializado, que `Load` rellena y que `Save()`
(use sin argumentos) consulta.

---

### M5 — `Install` no valida el asset antes de bajarlo

**Ubicación:** `install.go:65-72`.

**Qué pasa.** `InstallVerified` arma la URL (`install.go:65`), la baja
(`install.go:70`) y recién ahí verifica la firma (`install.go:76-80`).
Si el `Source` o el `Asset` del manifiesto están mal, el error aparece
**después** de descargar el archivo entero, y el mensaje es el de `curl`
("no se pudo bajar https://…") en vez de "el asset no existe".

`AssetURL` valida que `p.Asset` no esté vacío (`install.go:19-21`) y que
el source sea `github:owner/repo` (`install.go:22-28`), pero **no** valida
que el asset exista en el release. Eso solo se puede saber consultando la
API de releases, que el módulo no hace.

**Fix propuesto.** Como mínimo, un `HEAD` a la URL del asset antes de
bajarlo (o consultar `kit/releases` para el tag y ver si el asset está
en la lista), para fallar rápido con un mensaje claro.

---

## BAJO

### B1 — `ownedByB3l0w` reconoce `b3l0w-*` por prefijo

**Ubicación:** `pacman.go:64-67`.

**Qué pasa.** La función decide si un paquete es del repo propio por el
**prefijo** del nombre:

```go
func ownedByB3l0w(pkg string) bool {
    return pkg == "b3l0w-cli" || strings.HasPrefix(pkg, "b3l0w-") ||
        pkg == "boot-preview" || pkg == "kitty-shapes"
}
```

Un paquete de otro repo llamado `b3l0w-foo` se tomaría por nuestro, y
`Uninstall` lo quitaría con `sudo pacman -Rns` (`uninstall.go:37`). El
riesgo real es bajo (nadie publica `b3l0w-*` en otro repo), pero el
comentario de `pacman.go:63` dice "dice si un paquete pertenece al repo
propio", y el prefijo no lo demuestra.

**Fix propuesto.** Preguntar a pacman: `pacman -Qi` y leer el campo
`Repository`.

---

### B2 — `Current` no escapa el path del binario

**Ubicación:** `programs.go:51`.

**Qué pasa.** `Current` ejecuta `system.RunTimeout(10*time.Second, bin, "version")`.
`bin` viene de `system.CommandPath(program.Name)` (`programs.go:81`), que
devuelve una ruta del `PATH`, así que en la práctica no tiene espacios.
Pero `Current` es **exportado**: cualquier llamador puede pasarle
`"/opt/mi programa/bin"`, y `exec.Command` lo trata como un solo argumento
(correcto), pero el error que devuelve menciona la ruta sin comillas,
queda ambiguo. Riesgo bajo, deuda de API.

---

### B3 — `Update` no distingue el motivo del fallo

**Ubicación:** `programs.go:115-123`.

**Qué pasa.** `Update` devuelve `(mensaje, ok)`. Cuando el comando falla,
el mensaje es `lastLine(out)` de la salida (o el error si no hubo salida):
`programs.go:117-122`. El llamador (`internal/update/update.go:483-489`) lo muestra como
"programa: falló" sin más contexto. El README dice que `Update` "devuelve
su salida y si salió bien" (`README.md:56`) — pero en realidad devuelve
**la última línea**, no la salida.

**Fix propuesto.** Devolver la salida completa (o las últimas N líneas) y
un error tipado, para que el llamador pueda distinguir "el update dijo
que no" de "el update se cayó".

---

## Lo que no encontré

- **La verificación de firma es fail-closed y en el orden correcto.**
  `VerifyRelease` (`install.go:102-124`) baja `SHA256SUMS` y
  `SHA256SUMS.asc` **del mismo release** que el asset, y `verify.Sums`
  valida la firma antes de que se ejecute nada. Si falta cualquiera de los
  dos, el error es `verify.ErrSinFirma` (`install.go:110,116`) y no se
  instala nada. Correcto, y el test `TestVerifyReleaseSinFirma` lo fija.
- **La derivación de la base del release es correcta.**
  `VerifyRelease` la saca con `strings.TrimSuffix(assetURL, "/"+assetName)`
  (`install.go:103-106`): si la URL no termina con el nombre del asset,
  devuelve un error explícito en vez de adivinar. Verificado:
  `VerifyRelease("https://example.com/otro", "b3l0w-linux-amd64", ...)`
  → "no pude derivar la URL del release de https://example.com/otro".
- **El asset se valida antes de pisar el que funciona.**
  `Current(tmp)` (`install.go:82`) ejecuta el asset baixado y exige que
  responda antes del `ReplaceFile`. El README lo promete
  (`README.md:105-107`) y el código lo cumple.
- **`InstallPath` valida el nombre con `SafeName`**
  (`install.go:34`, `kit/system/system.go:91-109`): un nombre con `/` o
  `..` se rechaza. Es la defensa contra el path traversal vía manifiesto.
- **`Statuses` distingue "no instalado" de "instalado sin versión"**
  (`programs.go:80-95`): el primero deja `Bin` vacío y `Installed` en
  `false`; el segundo llena `Err`. El consumidor `check`
  (`internal/update/update.go:528-535`) los reporta distinto. Correcto.
- **`Update` no se auto-actualiza un programa que es de un paquete**
  (`programs.go:111-113`): si `pacman -Qo` dice que el binario tiene
  dueño, lo dice y no lo ejecuta. Correcto, y es la decisión que
  documenta el README (`README.md:92-94`).
- **`Update` no aborta en el primer fallo** (`programs.go:100-103`): la
  decisión es del llamador. `updatePrograms` (`internal/update/update.go:474-497`) sigue
  con el resto y cuenta los fallos. Correcto.
- **`Uninstall` respeta al dueño del paquete** (`uninstall.go:35-42`): si
  el binario es de un paquete que no es nuestro, **no** lo borra a mano y
  devuelve un error que dice que se quita con pacman. El README lo
  promete (`README.md:109-114`) y el código lo cumple.
- **`InstallPackage` limpia el binario suelto que gana en el PATH**
  (`pacman.go:42-46`): si quedó un `~/.local/bin/<nombre>` viejo, lo
  borra para que no tape la versión del paquete. Correcto.
- **`ParseVersion` normaliza siempre con `v` adelante**
  (`programs.go:42`): "1.2.3" y "v1.2.3" dan la misma salida. Es lo que
  promete el README (`README.md:79-81`).
- **La regex no es la única forma de leer una versión en el CLI**: el
  README (`README.md:79-81`) dice que `ParseVersion` es "la única forma de
  leer una versión en todo el CLI", pero `internal/selfupdate` tiene su
  propia comparación (`numeros`, `internal/selfupdate/selfupdate.go:121-134`) y `internal/update`
  usa `vercmp` (`internal/update/update.go:626-636`). La afirmación es falsa, pero no es
  un bug del módulo: es una inexactitud del README.
- **No hay inyección de shell**: todo se ejecuta con `exec.Command` y
  argumentos separados (`kit/system/system.go:16-37`). No hay `sh -c` con
  strings concatenadas.
- **No hay path traversal desde el manifiesto**: `InstallPath` valida el
  nombre con `SafeName` y el destino se arma con `filepath.Join` sobre el
  home. Correcto.

## Cómo verifiqué

Tests temporales en `internal/programs/zz_probe_test.go` (package
`programs`), ejecutados con `go test ./internal/programs/ -run TestProbe -v`
y borrados al terminar. Cubrieron: el modo del destino tras `ReplaceFile`
(M2), el comportamiento de `Packaged` contra la BD sync real de la máquina
(A1), `ParseVersion` con fechas (M1), `AssetURL` con ruta extra (M3), y la
derivación de la base del release en `VerifyRelease`. El bug A2 (orden
borrar-guardar) es **por inspección del código**: no se reprodujo porque
hacer fallar `m.Save` en un `t.TempDir()` requiere montar un filesystem
con permisos o cuota, y el modo de fallo se dedujo del orden de las
sentencias (`uninstall.go:43-51`). El estado final del worktree no tiene
archivos de prueba nuevos.
