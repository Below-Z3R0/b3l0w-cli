# TESTS — internal/plugins

Qué prueba cada test del módulo, qué queda sin cubrir y por qué.

Este paquete es la **capa transversal** de los plugins de Noctalia: catálogo,
manifiesto, install/update/remove, source de Noctalia y las extensiones
binarias del CLI. Los plugins individuales (`internal/plugins/we`,
`/docker`, `/anilist`, `/ytmusic`) tienen sus propias suites y **no** son
parte de este documento.

**Regla dura del módulo:** los tests no tocan el home real ni la red. Salvo
`TestDependencies` (ver B4 en `bugs.md`), todos usan `t.TempDir()`.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Archivos de test | 3 (`deps_test.go`, `exec_test.go`, `registry_test.go`) |
| Tests | 8 |
| Verdes | 8 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **28.8 %** de las sentencias |

Medido con `go test ./internal/plugins/ -count=1 -cover` → `ok ... coverage:
28.8 % of statements`. Con `-coverprofile` el total por función da 28.5 %
(la diferencia es el redondeo de `go test -cover`).

El número es bajo y **no es culpa de los tests**: el 71 % del paquete depende
de Noctalia (`noctalia msg plugins ...`), de la red (`codeload.github.com`,
`raw.githubusercontent.com`) o del home real. Casi ninguna de esas rutas es
alcanzable desde un test sin un seam de inyección.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `registry_test.go` | `slug()`, `Collect()` (orden, `Repo`, rechazo de slug/id distintos, API nueva), `Build()` (catalog.toml + plugins.json), `CheckCatalog()` (al día vs. viejo), round-trip del manifiesto con orden por id |
| `deps_test.go` | `Dependencies()`: que el manifiesto se lea y devuelva dependencias |
| `exec_test.go` | `ExecScript()` con un script inexistente: error, no panic |

---

## Qué prueba cada test

### `registry_test.go`

| Test | Qué fija |
|---|---|
| `TestSlug` | `slug()` sobre tres ids: `b3l0w/wallpaper` → `wallpaper`, `user/hover-zones` → `hover-zones`, y un id **sin barra** (`sinbarra`) → cae a `e.Path` (`registry.go:98`). Es el único test que toca `slug()`, y no cubre el caso `..` de BUGS C2 |
| `TestCollectAndBuild` | El camino completo del build: `Collect()` sobre un registry de dos plugins, el orden por id, que `Repo` sea `DistributionRepo`, que `Build()` escriba un `catalog.toml` con `id`, `version`, `plugin_api` y `tags` de los plugins, y que el `plugins.json` regenerado se pueda volver a leer con el orden correcto |
| `TestCollectRejectsSlugMismatch` | `Collect()` **falla** cuando el `id` del `plugin.toml` no termina en `/<carpeta>` (`registry.go:126`): el build se niega a publicar un plugin cuyo id no corresponde a la carpeta |
| `TestCollectAcceptsNewAPI` | Un `plugin_api` mayor que `SupportedPluginAPI` **no** rompe el build (`registry.go:129` solo rechaza `< 1`). Documenta la contradicción de BUGS A8: la constante dice "máximo" y el test dice lo contrario |
| `TestCheckCatalog` | `CheckCatalog()` da `true` sobre un catálogo recién generado y `false` después de escribir "viejo\n": el gate de `b3l0w plugins build --check` funciona en los dos sentidos |
| `TestManifestRoundTrip` | `SaveManifest()` ordena por id y `LoadManifest()` lo devuelve igual: el `plugins.json` sobrevive un viaje de ida y vuelta |

### `deps_test.go`

| Test | Qué fija |
|---|---|
| `TestDependencies` | `Dependencies()` lee el manifiesto (vía el registry) y devuelve un mapa no vacío de plugin → dependencias. **No es hermético**: depende de la red y de los plugins materializados en el home. Con `HOME` aislado **falla** (`no leyó ningún plugin`) — ver BUGS B4 |

### `exec_test.go`

| Test | Qué fija |
|---|---|
| `TestExecScriptMissing` | `ExecScript("no-existe-xyz", nil)` devuelve error en vez de panic. Es el único test del módulo que toca `exec.go`, y solo por el camino de error |

---

## Cobertura por función

Lo que sí se ejercita y lo que no (`go tool cover -func` sobre
`-coverprofile`):

| Función | Cobertura | Nota |
|---|---|---|
| `slug` | 100 % | pero solo el camino sin `..` |
| `readPluginMeta` | 100 % | |
| `parsePluginMeta` | 96.7 % | falta el caso de `id`/`version` ausentes |
| `quoteList` / `fileTimestamp` | 100 % | |
| `Collect` | 73.1 % | falta el caso "sin plugins" y "plugin.toml sin name" |
| `RenderCatalog` | 90.3 % | falta el id que no coincide con el del manifiesto |
| `Build` | 75 % | falta `manifestPath` vacío |
| `SaveManifest` | 83.3 % | |
| `LoadManifest` / `parse` | 66.7 % | falta el manifiesto corrupto |
| `CheckCatalog` | 66.7 % | falta el `catalog.toml` ilegible |
| `Dependencies` | 90.9 % | |
| `gitTimestamp` | 46.2 % | solo el camino sin repo git |
| `DefaultPluginsDir` | 75 % | |
| `LoadDefault` | 75 % | |
| **`install.go` (todo)** | **~0-3 %** | `Install`, `Update`, `installOne`, `downloadTarball`, `extractPrefix`, `Enabled`, `waitEnabled`, `Origin`, `InstalledVersion`, `Installed`, `sourceReady`, `candidateDirs` parcial |
| **`source.go` (todo)** | **0 %** | `EnsureSource`, `HasSource`, `AutoUpdate`, `Sources`, `InstalledBySource`, `UpdateSources`, `UpdateSource`, `repairSourceRepo` |
| **`remove.go` (todo)** | **0 %** | `Remove` completo |
| **`plugins.go` (todo)** | **0 %** | `Discover`, `Find`, `Run` |
| **`scripts.go`** | **0 %** | `Scripts` |

---

## Sin cubrir

| Caso | Por qué no está |
|---|---|
| **`installOne` / `extractPrefix`** | Es la función con más daño posible del módulo (borra directorios, escribe desde un tarball) y tiene **0 %**. Se puede testear: el tarball es un `.tar.gz` que se puede fabricar en `t.TempDir()` y `extractPrefix` (`install.go:249`) toma `(tarball, prefix, dest)` como strings — no necesita red ni Noctalia. Es el hueco más grande del módulo |
| **El guard de path traversal del tarball** (`install.go:277-280`) | Con el mismo tarball artificial se puede probar que un `hdr.Name` con `..` se rechaza. Hoy nadie lo verifica |
| **`Remove` completo** (`remove.go:13`) | `Remove` tiene un seam natural: recibe `manifestPath` y `pluginsDir` por parámetro. Se puede testear entero en `t.TempDir()`, incluyendo el rechazo de un directorio inválido (`remove.go:34`) y el caso `dryRun` |
| **`Discover` / `Find` / `Run`** (`plugins.go:23-67`) | Dependen de `paths.PluginsDir()`, que lee `XDG_DATA_HOME`/`HOME`. Se pueden testear con `t.Setenv` + un directorio temporal: es exactamente lo que hice en el probe que confirmó BUGS A2. Hoy `plugins.go` tiene 0 % |
| **`AutoUpdate`** (`source.go:53`) | Lee dos rutas del home; con `HOME` temporal y un `settings.toml` fabricated se puede probar. El probe que confirmó BUGS A6 lo hizo así |
| **`Enabled` / `waitEnabled` / `HasSource` / `Sources` / `InstalledBySource`** | Dependen de la salida de `noctalia msg plugins ...`. No hay seam: habría que inyectar el runner de `system.RunCombined`. Es deuda de diseño, no un test que falta |
| **`Install` / `Update` completos** | Mismo motivo: el camino "hay source" es talks con Noctalia. El camino "no hay source" (tarball) sí es testeable, pero `installOne` llama a `downloadTarball()` con la URL hardcodeada (`install.go:21`) — sin seam no hay forma de testearlo sin red |
| **`EnsureSource` con Noctalia ausente** | El caso `!system.HaveCommand("noctalia")` (`source.go:23`) es determinista en una máquina sin Noctalia, pero no en una que la tiene |
| **La red real (`codeload.github.com`)** | Deliberadamente no se testeada: sería un test que depende de internet. Lo que sí tiene sentido es un test del tarball **local** (ver la primera fila) |
| **`registry.Load` con el registry real** | `internal/registry` ya tiene su propia suite con servidor HTTP de prueba. `internal/plugins` lo usa como dependencia |
| **`repairSourceRepo`** | Hoy es código muerto (BUGS A7). Si se enchufa, necesita un repo git de prueba: se puede hacer con `git init` en `t.TempDir()` y un commit, sin red |
| **Windows** | El módulo usa `filepath.Separator` y `os.PathSeparator` de forma portable. No hay `t.Skip` por plataforma, pero tampoco hay cobertura en Windows |

---

## Bugs de esta revisión

Los 14 hallazgos de `bugs.md` **no tienen test**. El paquete no tiene tests
rojos: `go test ./internal/plugins/` pasa entero. Cada hallazgo está marcado
como **verificado** (reproducido con un `zz_probe_test.go` temporal, borrado
después) o **por inspección**.

Los que un test podría fijar de inmediato, sin refactor:

| Bug | Test que lo cubriría |
|---|---|
| A2 (symlinks) | `Discover` + `Find` con un symlink en un directorio temporal |
| A5 (arrays multilínea) | `parsePluginMeta` con un `dependencies` de tres líneas y un `#` adentro |
| A6 (`AutoUpdate`) | `settings.toml` temporal con `auto_update` en otra tabla |
| B1 (manifiesto vacío) | `LoadManifest` con `{"version":1,"plugins":[]}` |
| B4 (test no hermético) | El propio `TestDependencies` debería poder correr con `HOME` temporal |

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/plugins/ -count=1

# Con cobertura
go test ./internal/plugins/ -count=1 -cover

# Con el detector de carreras
go test ./internal/plugins/ -race -count=1

# Ver qué función está sin cubrir
go test ./internal/plugins/ -count=1 -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out
```

**Nota de hermeticidad:** `go test ./internal/plugins/` **toca la red**
(`TestDependencies` → `internal/registry` → `raw.githubusercontent.com`) y el
home real (`~/.local/share/noctalia/plugins`). En una máquina sin esos
plugins instalados, el test falla. Es el único punto no hermético del
paquete (BUGS B4).
