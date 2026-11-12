# internal/plugins

Administra los plugins de Noctalia: el catálogo que se publica, la
instalación, el alta/baja y el release. El código de los plugins vive
en un repo aparte y único:
<https://github.com/Below-Z3R0/b3l0w-plugins> (`DistributionRepo`).
Cada plugin es una carpeta en la raíz de ese repo con su `plugin.toml`.
El CLI no los guarda: los publica y Noctalia los materializa.

## Las dos cosas de este paquete

| | Qué es | Dónde vive |
|---|---|---|
| Extensión del CLI | binario `b3l0w-<nombre>` | dir privado (`paths.PluginsDir()`) |
| Plugin de Noctalia | carpeta Luau con `plugin.toml` | repo `b3l0w-plugins` |

`b3l0w-anilist` (el binario) y `b3l0w/anilist` (el plugin de Noctalia)
son cosas distintas: por eso las extensiones no se mezclan con el
catálogo ni se instalan desde el registry. `Discover()` las lista,
`Find()` busca una y `Run()` la ejecuta heredando la terminal.

El directorio de las extensiones es privado a propósito: cualquiera que
pueda escribir en el PATH podría poner un `b3l0w-update` y correrlo con
este usuario. Git lo permite y es un riesgo aceptado; acá no.

## El manifiesto: plugins.json

Es la lista de plugins que el CLI conoce, con dos copias:

- **embebida**: `internal/plugins/plugins.json` (10 plugins, API 43).
- **de red**: `plugins/plugins.json` del repo `b3l0w`, servido por
  `raw.githubusercontent.com/Below-Z3R0/b3l0w/main`.

Sin `--manifest`, `LoadDefault()` la saca del registry con la misma
jerarquía que el resto de la distro: caché fresco (1 h) → red → caché
viejo → embebido. El embebido es el último recurso: una instalación
limpia, sin red, igual tiene catálogo.

Cada entrada (`Entry`) lleva `id`, `name`, `repo`, `path`, `version`,
`plugin_api` y, opcional, `sha256`. El `id` es `autor/nombre`
(`b3l0w/anilist`) y el `path` es la carpeta en el repo: tiene que
llamarse igual que el slug del id, porque Noctalia indexa por nombre
de directorio. `plugin_api` es la API que el plugin necesita (el CLI
acepta hasta `SupportedPluginAPI`, 43).

## El catálogo: catalog.toml

`catalog.toml` está en la raíz del repo de plugins y lo genera
`b3l0w plugins build` (`Build()` → `RenderCatalog()`). No se edita a
mano: la primera línea del archivo lo dice.

Un bloque `[[plugin]]` por plugin, con `id`, `name`, `version`,
`updated_at`, `added_at`, `author`, `license`, `icon`, `description`,
`plugin_api` y `tags`. Las fechas salen del historial de git del repo,
por carpeta (`gitTimestamp()`), no del reloj del build.

`b3l0w plugins build --check` dice si el catálogo está al día
(`CheckCatalog()`) y sale con error si no: sirve de gate.

Del `plugin.toml` de cada plugin el CLI parsea lo mínimo: `id`, `name`,
`version`, `plugin_api`, `author`, `license`, `icon`, `description`,
`tags` y `dependencies`. `Collect()` valida que el id termine en
`/<carpeta>` y que el `plugin_api` sea válido; si no, el build falla.

## Instalar, remover, habilitar

| Comando | Qué hace |
|---|---|
| `b3l0w plugins install [id]` | instala uno o todos |
| `b3l0w plugins update [id]` | actualiza a la versión del manifiesto |
| `b3l0w plugins remove <id>` | borra el local; si viene del source, lo deshabilita |
| `b3l0w plugins enable/disable <id>` | `noctalia msg plugins enable/disable` |
| `b3l0w plugins list\|info\|search` | estado del catálogo |

Todos aceptan `--dry-run`.

Con source de Noctalia, instalar no copia nada local: refresca el source
y deja que Noctalia habilite, porque copiar a
`~/.local/share/noctalia/plugins/<slug>` taparía el source con una versión
vieja. Sin Noctalia, `Install()` cae al tarball del repo (`main`). Un
plugin del source no se puede borrar: `Remove()` lo deshabilita.

## El source de Noctalia

El source se llama `b3l0w` y apunta al repo de plugins
(`RegistryURL`). `EnsureSource()` lo agrega si falta; `HasSource()` y
`Sources()` leen la lista; `UpdateSource()` lo refresca.

`repairSourceRepo()` arregla un clon con el working tree vacío (todo el
índice como "D"), que ya pasó: lo restaura desde HEAD. `AutoUpdate()`
lee `plugins.auto_update` del estado de Noctalia: mientras Noctalia esté
en "none", las actualizaciones las controla b3l0w.

## Backends embebidos

`b3l0w plugin-exec <script> [args...]` corre un script bash embebido
(`scripts/*.sh`) heredando la terminal. Hoy hay uno: `igpu-power-menu.sh`.

## Release

`b3l0w plugins release [--plugin=<slug>] [--api=N] [--push] [--yes]`

### Qué hace

1. Verifica que el repo esté en `main` y con el working tree limpio
   (excepto `catalog.toml`).
2. Elige los plugins: todos, o los que matchean `--plugin`.
3. Con `--api=N` rechaza los plugins que pidan una API mayor que N.
4. Prunea cada carpeta elegida: saca `tests/`, `docs/`, `fixtures/`,
   `node_modules/`, `.github/`, `.vscode/`, `.zed/`, `package.json`,
   los lockfiles, los `.py` y los `.md` que no sean LICENSE. Un
   `.releaseignore` opcional agrega rutas a borrar.
5. Regenera `catalog.toml` (y `plugins.json` si se pasó `--manifest`)
   con lo que quedó en el repo.
6. Commitea `release: <plugins>` en `main` y, con `--push`, lo sube a
   `origin main`.

### Cuándo se usa

Después de mergear el trabajo de un plugin a `main` y antes de que
Noctalia lo instale: deja la carpeta en producción y el catálogo al día.

### Por qué ya no hay rama dev

El repo de plugins tuvo dos ramas: `dev` (desarrollo) y `main`
(producción); el release copiaba de una a la otra pruneando la copia.
La `dev` se borró después de mergearla a `main` (merge `75abdee`), así
que hoy el repo tiene una sola rama.

Por eso el release ahora corre sobre `main`, no hace checkout de otra
rama (no hay rama de trabajo a la que volver) y no copia nada: prunea
en el lugar y commitea en `main`.

Efecto: los tests y docs que estaban en la `dev` ahora viven en `main`
y el release los borra del árbol (quedan en el historial). Volver a
desarrollar encima de un plugin publicado exige recuperarlos de ahí.

### Rutas

- `--registry`: repo de plugins. Por defecto `~/Documents/Github/b3l0w-plugins`
  (verificar: el real está en `~/Documents/Github/Plugins/b3l0w-plugins`).
- `--manifest`: `plugins.json` local. Sin el, va al registry.
- `--plugins-dir`: donde busca los plugins instalados.

## Tests

- `release_test.go`: `pruneProduction` y el release sobre un repo git de
  una sola rama: publica desde `main`, rechaza otra rama o el working
  tree sucio, y en dry-run no toca nada.
- `registry_test.go`: slug, `Collect`, `Build`, `CheckCatalog`,
  `selectRelease` y el round-trip del manifiesto.
- `exec_test.go`: `ExecScript` con un script inexistente.

## Pendiente (verificar)

- `release.go` menciona filas `[[plugin.release]]` del catálogo para la
  compatibilidad de API, pero `RenderCatalog()` solo escribe `[[plugin]]`.
- La fecha exacta de borrado de la rama `dev`: el merge `75abdee` se ve
  en el repo, el borrado no.
