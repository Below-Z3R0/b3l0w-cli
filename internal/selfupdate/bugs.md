# BUGS — internal/selfupdate

Revision de integridad sobre `internal/selfupdate` (HEAD `fb76242`).
Se leyó `selfupdate.go` (206 líneas), `selfupdate_test.go` (331 líneas),
`README.md`, y las dependencias que el módulo invoca: `kit/fetch/fetch.go`,
`kit/system/system.go` (`ReplaceFile`, `PackageOwner`) y
`internal/programs/install.go` (`Current`, `VerifyRelease`). Los hallazgos
marcados **verificado** se reprodujeron con tests temporales
(`zz_probe_test.go`, borrado después). No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/selfupdate/ -count=1`
→ **ok** (0.005s), 13 tests verdes, cobertura **83.3 %**. `go vet` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Check` devuelve el **primer** tag `v*` por orden de creación, no el mayor: con un backport puede devolver una versión vieja | `selfupdate.go:82-89` |
| A2 | MEDIO | `numeros` ignora el cuarto componente y todo lo que va tras un `-` o `+`: `v1.0.0-rc1` es indistinguible de `v1.0.0` | `selfupdate.go:121-134` |
| A3 | MEDIO | `withV("dev")` produce `"vdev"`, que `numeros` lee como `0.0.0`: una build local se cree **más vieja que cualquier release** | `selfupdate.go:138-143,158` |
| A4 | MEDIO | `replaceFile` conserva el modo del destino: un binario nuevo queda `0644` y sin permiso de ejecución | `selfupdate.go:201` |
| B1 | BAJO | `per_page=30`: con más de 30 releases publicados, el tag `v*` puede quedar fuera de la ventana | `selfupdate.go:27` |

---

## ALTO

### A1 — `Check` devuelve el primer tag `v*`, no el mayor

**Ubicación:** `selfupdate.go:82-89`.

**Qué pasa.** `Check` pide los 30 releases más recientes y devuelve el
**primero** cuyo tag empieza con `v`:

```go
for _, release := range releases {
    if release.Draft || release.Prerelease {
        continue
    }
    if strings.HasPrefix(release.TagName, "v") {
        return release.TagName, nil
    }
}
```

La API de GitHub devuelve los releases **en orden de creación descendente**,
no en orden de versión. La convención `v*` funciona solo mientras los tags
se publiquen en orden creciente de versión. En cuanto se publique un
backport (un `v0.4.1` después de un `v0.5.0`), el primer `v*` de la lista
es el backport y `Check` devuelve una versión **más vieja** que la
instalada.

**Reproducción (verificado).**

```go
// el servidor devuelve los releases en orden de creación, como la API real
srv := httptest.NewServer(... `[
    {"tag_name":"desktop-2026.11.06-r1",...},
    {"tag_name":"v0.4.0",...},
    {"tag_name":"v0.9.0",...},
    {"tag_name":"v0.3.0",...}
]`))
tag, _ := Check()
// tag == "v0.4.0"   (el mayor de la lista es v0.9.0)
```

Consecuencia: `Run` compara `tag` contra la versión local
(`selfupdate.go:167`) y, si `tag <= local`, **no hace nada** y dice "el
release es más viejo que el binario". El usuario queda clavado en una
versión intermedia sin ningún release que lo actualice. Y `internal/update`
usa el mismo `Check` para el aviso de `update --check`
(`internal/update/update.go:504-506`): anuncia "b3l0w: v0.9.0 -> v0.4.0", una
"actualización" a una versión anterior.

El repositorio ya tiene el caso: los tags reales son `desktop-6`,
`desktop-2026.11.06-r1`, `v0.4.0`, `v0.3.1`, `v0.3.0`, `v0.2.1`,
`v0.2.0`, `v0.1.0` — hoy el orden de creación coincide con el de versión,
pero nada lo garantiza.

**Fix propuesto.** Devolver el **mayor** tag `v*` por valor, no el primero
por posición. Como `numeros` ya sabe comparar versiones
(`selfupdate.go:108-119`), el fix es:

```go
var mejor string
for _, release := range releases {
    if release.Draft || release.Prerelease {
        continue
    }
    if !strings.HasPrefix(release.TagName, "v") {
        continue
    }
    if mejor == "" || compareVersions(release.TagName, mejor) > 0 {
        mejor = release.TagName
    }
}
if mejor == "" {
    return "", fmt.Errorf("no hay releases del CLI con tag v* en %s", repo)
}
return mejor, nil
```

(Con el A2 corregido, `compareVersions` debería distinguir bien las
versiones.)

---

## MEDIO

### A2 — `numeros` ignora el cuarto componente y todo lo que sigue al `-`

**Ubicación:** `selfupdate.go:121-134`.

**Qué pasa.** `numeros` parte la versión en `.`, se queda con **tres**
componentes (`if i >= 3 { break }`) y descarta todo lo que aparece tras el
primer `-` o `+`:

```go
v = strings.TrimPrefix(v, "v")
if i := strings.IndexAny(v, "-+"); i >= 0 {
    v = v[:i]
}
```

Consecuencias:

- `v1.0.0-rc1` → `[1 0 0]`: un release candidate es **indistinguible** del
  release final. El README (`README.md:72-75`) dice que se ignoran los
  sufijos "`-dev`, `+build`", pero el corte es en el **primer** `-` o `+`,
  así que también se come los `-rc`, `-beta`, `-pre`.
- `v1.0.0.1` → `[1 0 0]`: el cuarto componente se pierde.
- `v1.0` → `[1 0 0]`: `v1.0` y `v1.0.0` son iguales.

**Reproducción (verificado).**

```go
numeros("v1.0.0")      → [1 0 0]
numeros("v1.0.0-rc1")  → [1 0 0]
numeros("v1.0.0+build5")→ [1 0 0]
numeros("v1.0.0-dev")  → [1 0 0]
numeros("v1.0.0-1")    → [1 0 0]
numeros("v1.0.0.1")    → [1 0 0]
compareVersions("v1.0.0-rc1", "v1.0.0") → 0   // treated as equal
```

El caso `v1.0.0-rc1` vs `v1.0.0` es el relevante: con `cmp == 0`,
`Run` (`selfupdate.go:167`) decide que el release no es más nuevo y **no
actualiza**, dejando al usuario en el rc para siempre.

**Fix propuesto.** Separar el sufijo del número y comparar prereleases por
debajo del release final (como hace `vercmp` de pacman): un `-rc` es
**menor** que el release sin sufijo. Y no truncar a tres componentes sin
motivo: comparar la tupla completa o, al menos, no perder el cuarto.

---

### A3 — `withV("dev")` produce `"vdev"`, que `numeros` lee como `0.0.0`

**Ubicación:** `selfupdate.go:138-143` y `selfupdate.go:158`.

**Qué pasa.** `Run` hace `result.Before = withV(version.Version)`. En una
compilación local, `version.Version` es `"dev"` (`internal/version/version.go:12`).
`withV` le antepone la `v` sin mirar el contenido:

```go
func withV(v string) string {
    if strings.HasPrefix(v, "v") {
        return v
    }
    return "v" + v
}
```

`withV("dev")` → `"vdev"`. Y `numeros("vdev")` → quita la `v` → `"dev"` →
no hay `-` ni `+` → parte por `.` → `["dev"]` → `strconv.Atoi("dev")` falla
y devuelve `0` (el error se descarta en `selfupdate.go:131`) → `[0 0 0]`.

**Reproducción (verificado).**

```go
withV("dev")                              → "vdev"
numeros("vdev")                           → [0 0 0]
numeros("dev")                            → [0 0 0]
compareVersions("v0.4.0", withV("dev"))   → 1   // "el release es más nuevo"
```

Consecuencia: una build local (`dev`) se cree **más vieja que cualquier
release**, así que `b3l0w update --self` sobre un binario compilado a mano
**baja y reemplaza el binario del usuario por el release publicado**. Para
alguien que compila el CLI desde el repo para probar sus cambios, eso
destruye su build.

El README dice que el módulo "no degrada un binario de desarrollo"
(`README.md:72-75`), pero el mecanismo que lo implementa (`numeros`
ignorando sufijos) no cubre el caso real del `dev`.

**Fix propuesto.** Tratar `dev` explícitamente antes de comparar: si
`version.Version` no es una versión parseable, no hay nada que comparar y
`Run` debería devolver un `Note` explicativo sin tocar nada. Algo como:

```go
if !versionRe.MatchString(version.Version) {
    result.Note = "versión local no comparable (build de desarrollo): no se toca"
    return result, nil
}
```

---

### A4 — `replaceFile` deja el binario sin permiso de ejecución

**Ubicación:** `selfupdate.go:201` (la llamada) y
`kit/system/system.go:167-176` (la implementación).

**Qué pasa.** `Run` reemplaza el binario en ejecución con
`system.ReplaceFile(tmp, exe)`. `ReplaceFile` **conserva el modo del
destino** si existe, y si no existe usa `0o644` (`kit/system/system.go:167-170`):

```go
modo := os.FileMode(0o644)
if info, err := os.Stat(dest); err == nil {
    modo = info.Mode().Perm()
}
```

El caso normal (`dest` = el binario en ejecución) conserva `0755` y está
bien. Pero el caso "el destino no existe" deja el binario en `0644`:
**sin permiso de ejecución**. El `tmp` que se descarga sí viene con `0755`
(`kit/fetch/fetch.go:33` hace el `chmod`), pero `ReplaceFile` no copia el
modo del origen: usa el del destino, o `0644` si no hay destino.

**Reproducción (verificado).**

```go
src  = 0755 (el asset bajado)
dest = no existe
system.ReplaceFile(src, dest)
// modo del destino: -rw-r--r--  (ejecutable=false)
```

El mismo applies a `internal/programs/install.go:91`: un programa nuevo
instalado en `~/.local/bin/<nombre>` queda sin `+x` y no se puede ejecutar.
Ahí el destino tampoco existe en la primera instalación.

**Nota.** En el flujo normal de `selfupdate` el destino **sí** existe (es el
binario en ejecución), así que el modo se conserva y el bug no se
manifesta. Se manifiesta cuando `ReplaceFile` crea el archivo: en
`internal/programs` es el caso común de una primera instalación.

**Fix propuesto.** En `ReplaceFile`, si el destino no existe, usar el modo
del **origen** (y si el origen tampoco es ejecutable, `0755`):

```go
modo := os.FileMode(0o755)
if info, err := os.Stat(dest); err == nil {
    modo = info.Mode().Perm()
} else if info, err := os.Stat(src); err == nil {
    modo = info.Mode().Perm()
}
```

---

## BAJO

### B1 — `per_page=30`: el tag `v*` puede quedar fuera de la ventana

**Ubicación:** `selfupdate.go:27`.

**Qué pasa.** `api` pide los 30 releases más recientes
(`?per_page=30`). El repo publica **también** los releases de escritorio
(`desktop-6`, `desktop-2026.11.06-r1`, …), y cada uno consume una de las
30 posiciones. Cuando se acumulen 30 releases que no sean del CLI, el tag
`v*` más reciente queda fuera de la respuesta y `Check` falla con "no hay
releases del CLI con tag v* en Below-Z3R0/b3l0w-cli".

Hoy hay 8 releases en total, así que no está cerca. Pero la API no ofrece
paginación aquí: no hay forma de mirar más atrás.

**Fix propuesto.** Subir `per_page` (GitHub acepta hasta 100) y/o filtrar
por tag en el servidor (`GET /repos/{o}/{r}/releases/tags/v*` si existe un
release con ese tag exacto, o usar `/git/matching-refs/tags/v`).

---

## Lo que no encontré

- **El fail-closed de la firma está bien.** `verifyRelease` se llama
  **antes** de `replaceFile` (`selfupdate.go:183-185` vs `201`), y un error
  de verificación aborta sin tocar el binario. Verificado por
  `TestRunNoReemplazaSiLaFirmaFalla`. Correcto.
- **El binario bajado se prueba antes de reemplazar**
  (`selfupdate.go:187-190`): si el asset no responde a `version`, no se
  toca el ejecutable. Correcto.
- **La comparación ocurre antes de la descarga** (`selfupdate.go:167-174`):
  si el tag no es más nuevo, no se bajan los 11 MB. Verificado por
  `TestRunNoDescargaSiElTagEsViejo`. Correcto y bien pensado.
- **El no-degradar funciona para versiones comparables**
  (`selfupdate.go:193-200`): si el asset bajado declara una versión menor
  que la local, no se reemplaza y se deja la nota. Correcto para
  `v0.5.0` vs `v0.4.0`; el hueco es el caso `dev` (A3) y el de los
  prereleases (A2).
- **El reemplazo es atómico.** `system.ReplaceFile` copia a
  `<dest>.new` en el mismo directorio y hace `rename`
  (`kit/system/system.go:172-176`): no queda un archivo a medias y no se
  pisa un binario en ejecución. Correcto, tal como dice el README
  (`README.md:85-88`).
- **El rechazo del binario de pacman** (`selfupdate.go:154-156`) es
  correcto: `packageOwner(exe)` con `pacman -Qo` y un error claro.
- **`Run` devuelve un `Result` útil**: `Before`, `After`, `Changed` y
  `Note` (`selfupdate.go:93-101`). El `Note` explica el motivo de no
  cambiar, que es lo que permite al caller distinguir "no había nada"
  de "no se pudo".
- **El timeout de la consulta** (`selfupdate.go:65`, 8 s) está bien puesto.
- **El `User-Agent` sí se manda** (`selfupdate.go:64`): `b3l0w`. Es lo que
  `internal/versioncheck` **no** hace (bug A3 de ese módulo).

## Cómo verifiqué

Tests temporales en `internal/selfupdate/zz_probe_test.go` (package
`selfupdate`), ejecutados con `go test ./internal/selfupdate/ -run
TestProbe -v` y borrados al terminar. Cubrieron: el orden de `Check` con
una lista de releases en orden de creación (A1), el comportamiento de
`numeros` con sufijos y cuarto componente (A2), y el caso `dev` de
`withV`/`numeros`/`compareVersions` (A3). El bug A4 se reprodujo en un
probe del paquete `internal/programs` con `system.ReplaceFile` y un destino
inexistente. El estado final del worktree no tiene archivos de prueba
nuevos.

---

## Arreglado

| ID | Arreglo | Test |
|---|---|---|
| A1 | `Check` elige el tag de versión más alta con `compareVersions`, no el primero de la lista. Draft y prerelease se siguen ignorando. | `TestCheckDevuelveElMayor` y `TestCheckIgnoraDraftYPrerelease` |

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| B1 | `per_page=100`: el repo publica los `desktop-*`, y con 30 posiciones el tag `v*` del CLI podía quedar fuera | `TestAPIUsaPerPage100` |
