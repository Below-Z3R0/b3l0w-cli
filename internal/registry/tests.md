# TESTS — internal/registry

Qué prueba cada test del módulo, qué queda sin cubrir y por qué.

**Regla dura del módulo:** ningún test toca la red real ni el home real. La
red se simula con un `httptest.NewServer` y el estado se aísla con
`t.TempDir()`.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Archivos de test | 1 (`registry_test.go`) |
| Tests | 7 |
| Verdes | 7 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **67.6 %** de las sentencias |

Medido con `go test ./internal/registry/ -count=1 -cover` → `ok ... coverage:
67.6 % of statements`.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `registry_test.go` | La jerarquía completa de `Load` (embebido, red, caché, caché viejo, error), `Sync` (ignora el TTL, red caída), `Stat` y el helper `preparar` que aísla estado y red |

---

## Qué prueba cada test

### `registry_test.go`

| Test | Qué fija |
|---|---|
| `TestLoadUsaElEmbebidoSinRed` | Sin caché y sin red, `Load` devuelve **exactamente** el embebido que le pasó el llamador y reporta `Source == FromEmbed`. Es el "último recurso" del README (README.md:76-79) |
| `TestLoadBajaYCachea` | El camino normal: la primera lectura baja de la red y reporta `FromRemote`; la **segunda sale del caché sin gastar red** (el contador de requests queda en 1); el `User-Agent` es `b3l0w`; el path pedido es `/services/services.json`; y el caché queda escrito en disco |
| `TestLoadUsaCacheViejoSiLaRedFalla` | Con red caída y un caché **envejecido** (el test usa `os.Chtimes` para retroceder el mtime más de un TTL), `Load` devuelve la copia vieja con `Source == FromStaleCache` y **no** cae al embebido |
| `TestLoadSinNadaEsError` | Sin caché, sin red y sin embebido, `Load` **falla**: es el único caso que devuelve error |
| `TestSyncIgnoraElTTL` | `Sync` ignora el TTL: con un caché **fresco** en disco, `Sync` igual descarga y **pisa** el caché con la versión nueva (`{"version":2}`). Es el "pacman -Sy" del CLI |
| `TestSyncConRedCaidaAvisaYUsaCache` | `Sync` con red caída devuelve **error** (no un éxito silencioso) y el `Status` que acompaña dice `FromStaleCache`: el usuario sabe que va a usar la copia local |
| `TestStat` | `Stat` sobre una categoría sin archivo devuelve `FromEmbed`; sobre un archivo existente devuelve `FromCache` con el `Size` correcto (7 bytes) |

### El helper `preparar`

`registry_test.go:13-27` es la pieza que hace posible todo lo demás:

- reemplaza `stateDir` por una función que devuelve un `t.TempDir()`, así que
  la caché vive aislada;
- con `h != nil` levanta un `httptest.NewServer` y apunta `baseURL` a él; con
  `h == nil` apunta a `http://127.0.0.1:1` (puerto cerrado) para simular
  "sin red" de forma determinista;
- restaura las dos variables en `t.Cleanup`.

Es un buen patrón: en vez de un `httptest` global, cada test decide si tiene
red o no.

---

## Cobertura por función

`go tool cover -func` sobre `-coverprofile`:

| Función | Cobertura | Nota |
|---|---|---|
| `Load` | 100 % | las cuatro ramas de la jerarquía |
| `Stat` | 100 % | |
| `cachePath` | 100 % | |
| `readCache` | 85.7 % | falta el caso "el archivo existe pero está vacío" |
| `fetch` | 75 % | falta el HTTP != 200, el body vacío y la categoría desconocida |
| `Sync` | 72.7 % | falta el camino "sin caché y sin embebido" |
| `writeCache` | 66.7 % | falta el caso "no se puede crear el directorio" |
| `modTime` | 66.7 % | falta el `fi == nil` |
| `String` (de `Source`) | **0 %** | los cuatro nombres ("caché", "red", "caché viejo", "embebido") no están probados |
| `All` | **0 %** | |
| `defaultStateDir` | **0 %** | |
| `CachePath` | **0 %** | |

---

## Sin cubrir

| Caso | Por qué no está |
|---|---|
| **`String()` de `Source`** | Es lo que se imprime en `b3l0w sync` (`cmd/b3l0w/sync.go:57`). Cuatro casos, un test de tabla. Trivial y hoy nadie lo cubre: un typo en "caché viejo" sale directo al usuario |
| **`All()`** | `cmd/b3l0w/sync.go:44` la usa para sincronizar todo. Un test de que devuelve exactamente `[System, Services, Plugins]` y que no repite ninguno |
| **`CachePath()`** | La exportan `cmd/b3l0w/services.go:488` y `internal/update/update.go`. Que apunte al mismo archivo que usa `cachePath` interno es una invariante que se puede fijar |
| **`defaultStateDir()`** | Decide dónde vive la caché. Con `XDG_STATE_HOME` seteado tiene que ir a `$XDG_STATE_HOME/b3l0w/registry`; sin home, al fallback de `/tmp` (BUGS A6). Es environmental: testeable con `t.Setenv` |
| **HTTP 404 / 500** | `fetch` devuelve "categoría desconocida: HTTP %d" (`registry.go:160`). Nadie verifica que un 500 no se cachee: es el caso donde un error del servidor se vuelve una DB rota en el caché |
| **Body vacío** | `fetch` lo rechaza con "respuesta vacía" (`registry.go:167`). Sin test: una respuesta 200 vacía es un caso real de un proxy mal configurado |
| **Una categoría desconocida** | `fetch` la rechaza (`registry.go:145-147`), pero **`Load` se come el error** (BUGS A1). El test que falta es el que lo demuestra: `Load(Kind("no-existe"), ...)` no debería devolver el embebido con `err == nil` |
| **La caché corrupta** | `readCache` no valida el contenido (BUGS A2). Un test que escriba `{roto` en el caché y exija que `Load` lo rechace |
| **`writeCache` con error** | El `MkdirAll` fallando y el `WriteFile` fallando (disco lleno, permisos) no tienen test. Hoy los errores se descartan (`_ =`), así que el test documentaría que un caché que no se pudo escribir no rompe la lectura |
| **Escritura atómica** | No hay writing atómico todavía (BUGS A3): el test que falta es el de "una caché truncada no se sirve" |
| **`Stat` vs `Load` en la misma caché** | Hoy contradicen el TTL (BUGS A4). Un test que los compare sobre una caché de dos horas lo fijaría |
| **`Sync` sin caché y sin embebido** | La rama `registry.go:124` no está cubierta |
| **Concurrencia** | `Load` y `Sync` escriben el caché sin lock. Dos procesos a la vez pueden pisarse el archivo. No hay test de concurrencia (el paquete no lo tiene y el módulo no lo pide) |
| **Un body de más de 4 MiB** | `maxSize` trunca en silencio (BUGS B1). Un `httptest` que mande 5 MiB y verifique que el error es explícito |
| **Windows** | El módulo usa `filepath.Join` y `os.Getenv`, portable. No hay cobertura en Windows, pero tampoco hay código específico |

---

## Bugs de esta revisión

Los 7 hallazgos de `bugs.md` **no tienen test**. El paquete no tiene tests
rojos: `go test ./internal/registry/` pasa entero.

Los que un test podría fijar sin refactor:

| Bug | Test que lo cubriría |
|---|---|
| A1 (categoría desconocida) | `Load(Kind("no-existe"), embedded)` no debe devolver `err == nil` |
| A2 (caché corrupta) | Escribir `{roto` en el caché y exigir que `Load` lo rechace |
| A4 (Stat vs Load) | Comparar ambos sobre una caché de 2 h |
| A5 (Sync no actualiza) | Verificar que el `Status` de `Sync` con red caída describe la copia que realmente se sirve |
| B1 (truncado silencioso) | Un handler que mande 5 MiB y esperar un error, no un JSON truncado |

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/registry/ -count=1

# Con cobertura
go test ./internal/registry/ -count=1 -cover

# Con el detector de carreras
go test ./internal/registry/ -race -count=1

# Ver qué función está sin cubrir
go test ./internal/registry/ -count=1 -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out
```

**Hermeticidad:** la suite no toca la red real ni el home. El servidor de
prueba es local (`httptest`) y el estado vive en un `t.TempDir()`. Corre
offline y en cualquier máquina.
