# BUGS — internal/registry

Revisión de integridad en contexto cero sobre `internal/registry` (HEAD
`fb76242`).

Se leyó el archivo de producción `registry.go` (217 líneas), su `README.md` y
el `*_test.go`. Los hallazgos marcados **verificado** se reprodujeron con un
test temporal en el propio paquete (`zz_probe_test.go`, borrado después); los
marcados **por inspección** se deducen del código sin ejecución. No se modificó
código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/registry/
-count=1 -cover` → **ok** (7 tests, cobertura 67.6 %). `go vet
./internal/registry/` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Load()` no distingue "categoría desconocida" de "no hay red": un `Kind` mal escrito devuelve el embebido sin error | `registry.go:94` |
| A2 | MEDIO | `Load()` sirve una caché **corrupta** como si fuera válida: no valida el contenido | `registry.go:90` |
| A3 | MEDIO | `writeCache` no es atómico: un corte deja la caché corrupta y la sirve como "fresca" durante una hora | `registry.go:209` |
| A4 | MEDIO | `Stat()` no aplica el TTL: una caché de dos horas se reporta "caché" mientras `Load()` la da por "caché viejo" | `registry.go:135` |
| A5 | BAJO | `Sync()` devuelve la copia vieja **sin actualizarla**: el `Status` dice "caché viejo" pero el archivo sigue con la hora anterior | `registry.go:117` |
| A6 | BAJO | `defaultStateDir()` cae a `os.TempDir()` si no hay home: la caché queda en un directorio compartido | `registry.go:180` |
| B1 | BAJO | La respuesta de la red se acota a 4 MiB **sin avisar**: un body más grande se trunca en silencio | `registry.go:162` |

---

## ALTO

### A1 — `Load()` no distingue "categoría desconocida" de "no hay red"

**Ubicación:** `registry.go:87-108` (`Load`), `registry.go:143-147` (`fetch`).

**Qué pasa.** `fetch` valida la categoría:

```go
path, ok := paths[kind]
if !ok {
    return nil, fmt.Errorf("categoría desconocida: %s", kind)
}
```

Pero ese error **se pierde** en `Load`, que trata cualquier fallo de `fetch`
como "no hay red" y cae a la siguiente fuente:

```go
if data, err := fetch(kind); err == nil {
    writeCache(cache, data)
    return data, FromRemote, nil
}
```

El `err` se descarta. Después viene el caché viejo y, si no hay, el embebido.

**Verificado** con un probe en el paquete:

```
Load(Kind("no-existe"), []byte(`{"embebido":true}`))
   → data={"embebido":true}  src=embebido  err=<nil>
```

Un `Kind` mal escrito devuelve **la copia embebida de otra categoría** con
`err == nil`. El consumidor (`internal/plugins/registry.go:27`,
`internal/services/manifest.go:69`) no tiene forma de saber que la DB que le
sirven no es la que pidió: parsea el JSON equivocado y sigue.

El error "categoría desconocida" existe y es correcto, pero **es
inalcanzable** desde `Load`: solo `fetch` lo produce y `Load` lo traga.

**Fix propuesto.** Distinguir los dos casos en `Load`: si `fetch` falla por
categoría desconocida, devolver ese error; si falla por red, seguir con la
jerarquía. Una forma: que `fetch` devuelva un error sentinela (o un tipo de
error propio) y que `Load` lo compruebe con `errors.Is`.

---

## MEDIO

### A2 — `Load()` sirve una caché corrupta como válida

**Ubicación:** `registry.go:90` (`readCache`), `registry.go:193-203`.

**Qué pasa.** `readCache` decide si la copia local es usable mirando **solo**
la fecha y el tamaño:

```go
if err != nil || len(data) == 0 {
    return nil, false
}
```

No comprueba que el contenido sea parseable. Un caché truncado (por A3, o
porque alguien lo editó a mano) se sirve como fresco.

**Verificado** con un probe: con `plugins.json` conteniendo `{roto`,
`Load(Plugins, ...)` devuelve `data={roto  src=caché  err=<nil>`. O sea, el
registry **no detecta** el caché roto: el error aparece recién en el
consumidor, que hace `json.Unmarshal` y falla con un error de parseo que no
dice "tu caché local está corrupto" sino "el JSON de la DB está mal".

El README (README.md:112-114) es explícito de que el módulo "no interpreta el
contenido de las DB" — devolver bytes es correcto. Lo que falta es una
**comprobación mínima de integridad** al leer el caché, que es un problema
del registry, no del consumidor.

**Fix propuesto.** Validar el JSON al leer el caché (o al menos comprobar que
termina correctamente) y, si no parsea, tratarlo como caché inexistente
(`fresh = false`) para que caiga a la red. Alternativa: guardar el SHA-256
del body junto al caché y compararlo.

---

### A3 — `writeCache` no es atómico: una caché corrupta queda servible

**Ubicación:** `registry.go:205-210`.

**Qué pasa.** El guardado es un `os.WriteFile` a pelo:

```go
func writeCache(path string, data []byte) {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return
    }
    _ = os.WriteFile(path, data, 0o644)
}
```

`os.WriteFile` **trunca** el archivo y después escribe. Un corte (SIGKILL,
energía, un `write` parcial) deja el archivo a medias. Como el mtime queda
actualizado, `readCache` lo considera **fresco** y `Load` lo sirve — y por A2
no comprueba que esté completo. Resultado: la DB local queda corrupta y
servida como si fuera la buena, y **dura hasta que pase el TTL** (una hora)
o hasta el próximo `Sync`.

El error de `WriteFile` además se descarta (`_ =`), así que ni se avisa.

Es el mismo patrón que el `saveState` no atómico de `internal/dotfiles`
(ver su BUGS.md), pero acá el daño es menor: la caché se puede regenerar.

**Fix propuesto.** Escribir a `path + ".tmp"`, hacer `fsync` y `rename`
(reescritura atómica). Y no descartar el error de escritura.

---

### A4 — `Stat()` no aplica el TTL: contradice a `Load()`

**Ubicación:** `registry.go:131-138` (`Stat`), `registry.go:193-196`
(`readCache`).

**Qué pasa.** `Stat` describe lo que hay en disco sin mirar la fecha:

```go
func Stat(kind Kind) Status {
    path := cachePath(kind)
    fi, err := os.Stat(path)
    if err != nil {
        return Status{Kind: kind, Source: FromEmbed}
    }
    return Status{Kind: kind, Source: FromCache, Updated: fi.ModTime(), Size: int(fi.Size())}
}
```

`readCache` sí aplica el TTL (`time.Since(fi.ModTime()) > TTL`). O sea: el
mismo archivo puede ser "caché" según `Stat` y "caché viejo" según `Load`.

**Verificado** con un probe: una caché de dos horas da

```
Stat()  → source=caché        (Updated = hace 2 h)
Load()  → source=caché viejo  (mismos datos)
```

El consumidor de `Stat` es `cmd/b3l0w/sync.go:72` (`sync --check`), que
imprime "sincronizado <fecha>". O sea: `b3l0w sync --check` dice que la DB
está sincronizada cuando `Load` la va a tratar como vieja y va a intentar
descargarla de nuevo.

**Fix propuesto.** Que `Stat` use el mismo criterio que `readCache`: si la
copia pasó el TTL, devolver `FromStaleCache` (o un `Stale bool` en el
`Status`). Lo más limpio es que `Stat` reuse `readCache` y no duplique la
lógica de la fecha.

---

## BAJO

### A5 — `Sync()` devuelve la copia vieja sin actualizarla

`registry.go:111-125`: cuando la red falla y hay un caché viejo, `Sync`
devuelve `Status{Source: FromStaleCache, Updated: modTime(fi)}` y un error
con el texto "(se usa la copia local de <fecha>)". El `Status` que acompaña
al error es correcto (describe lo que hay), pero el nombre `Sync` sugiere
que sincronizó. El consumidor (`cmd/b3l0w/sync.go:54`) cuenta el error como
fallo y sigue con la siguiente categoría, así que el `Status` se descarta y
el usuario solo ve el warning. Aclarar en el docstring que `Sync` puede
devolver el estado de una copia que **no** actualizó, o renombrar el
`Source`.

### A6 — `defaultStateDir()` cae a `os.TempDir()`

`registry.go:172-183`: si `os.UserHomeDir()` falla (HOME no seteado en un
contenedor, usuario sin home), la caché va a `os.TempDir()/b3l0w/registry`.
`/tmp` es compartido y limpiable: otro usuario o un `tmpfiles` puede borrar
la caché, o un proceso con permisos puede escribirla antes que el primero.
Es un fallback para un caso raro, pero el directorio no se crea con modo
restringido (`MkdirAll` con `0755` en `registry.go:206`). Usar un directorio
por usuario (con el UID en el nombre) o `0700`.

### B1 — La respuesta de la red se acota a 4 MiB sin avisar

`registry.go:162`: `io.ReadAll(io.LimitReader(resp.Body, maxSize))`. Si el
servidor manda más de 4 MiB, `LimitReader` **trunca** y el resultado es un
JSON partido que se cachea (`registry.go:95`) y se sirve. El README
(README.md:98-100) dice que el límite "evita que un error del servidor con un
body gigante se coma la memoria", lo cual es cierto, pero truncar en
silencio produce una DB inválida en el caché en vez de un error. Comprobar el
tamaño con `Content-Length` o leer un byte de más para detectar el
desbordamiento, y devolver un error explícito.

---

## Lo que no encontré

- **La jerarquía.** `Load` sigue caché fresco → red → caché viejo → embebido
  (`registry.go:90-107`) exactamente como la documenta el README
  (README.md:14-16). Es correcta y el `TTL` de una hora es el criterio de
  pacman.
- **`Sync` ignora el TTL**, como debe ser el "pacman -Sy" del CLI
  (README.md:81-85). Correcto.
- **`All()`** devuelve las tres categorías sin repetir y en orden fijo
  (`registry.go:141`). Correcto.
- **El `User-Agent`** es `b3l0w` en toda request (`registry.go:153`).
  Correcto.
- **El `State` en vez de `Cache`** (README.md:62-64): el directorio es
  `~/.local/state/b3l0w/registry`, que respeta `XDG_STATE_HOME`
  (`registry.go:174-177`). Correcto.
- **Los paths del repo** (`system.json`, `services/services.json`,
  `plugins/plugins.json`) coinciden con lo que hay en `b3l0w`. Correcto.
- **`CachePath` es exportado** y lo usan `cmd/b3l0w/services.go:488` y
  `internal/update/update.go`. Correcto: el registry expone dónde está la
  copia para que otros paquetes puedan actualizarla.
- **No hay inyección de `http.Client`:** el `client` global (`registry.go:42`)
  tiene timeout de 5 s y el `baseURL` es una var de paquete que los tests
  cambian. Es justo lo que permite testear sin red. Bien.

## Cómo verifiqué

Tests temporales en `internal/registry/zz_probe_test.go` (package `registry`),
ejecutados con `go test ./internal/registry/ -run TestProbe -v` y borrados al
terminar. Cubrieron: `Kind` desconocido (A1), caché corrupta servida como
válida (A2) y `Stat` vs `Load` sobre la misma caché de dos horas (A4). El
estado final del worktree no tiene archivos de probe.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| B1 | `fetch` lee `maxSize+1` bytes y falla si llegó el byte de más: ya no trunca en silencio un JSON que después se cachea | `TestFetchRechazaUnaRespuestaGigante`, `TestFetchAceptaJustoElLimite` |
| A1 | `Load` valida el `Kind` contra `paths` antes de la cascada. Un Kind inexistente devuelve `categoría desconocida` en vez de caer al embebido y devolver otra categoría con `err nil` | `TestLoadRechazaUnaCategoriaDesconocida`, `TestLoadConCategoriaValidaSigueFuncionando` |
