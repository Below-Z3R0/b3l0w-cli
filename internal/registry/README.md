# registry

Carga las DB de la distro con una sola jerarquía de fuentes.

## Qué hace y por qué existe

La distro tiene tres catálogos: el del sistema, el de servicios y el
de plugins. Viven como JSON en el repo de distribución
(`Below-Z3R0/b3l0w`) y el CLI los baja y los cachea localmente.

El módulo replica la jerarquía que ya usa pacman, y con la misma
forma:

```
caché fresco (1 h) -> red -> caché viejo -> embebido
```

El embebido es el último recurso, no la fuente normal: una instalación
limpia, sin el repo clonado y sin red, igual tiene catálogo.

## API

| Función | Qué hace |
|---|---|
| `Load(kind Kind, embedded []byte) ([]byte, Source, error)` | Devuelve la DB de una categoría siguiendo la jerarquía |
| `Sync(kind Kind, embedded []byte) (Status, error)` | Fuerza la descarga, ignorando el TTL |
| `Stat(kind Kind) Status` | Describe la copia local, sin tocar la red |
| `All() []Kind` | Las tres categorías del registry |
| `CachePath(kind Kind) string` | Ruta de la copia local de una DB |

### Tipos

```go
type Kind string
const (
    System   Kind = "system"
    Services Kind = "services"
    Plugins  Kind = "plugins"
)

type Source int
const (
    FromEmbed Source = iota
    FromCache
    FromRemote
    FromStaleCache
)

type Status struct {
    Kind    Kind
    Source  Source
    Updated time.Time
    Size    int
}
```

`Source` tiene `String()`: `"caché"`, `"red"`, `"caché viejo"`,
`"embebido"`. Es lo que se imprime en `b3l0w sync`.

## Dónde vive la copia local

`~/.local/state/b3l0w/registry/<categoría>.json`, respetando
`XDG_STATE_HOME` si está definido. Se llama *state* y no *cache* a
propósito: es el estado del registry, no un archivo de derivación.

## Decisiones

**Una sola jerarquía para las tres categorías.** `system`, `services` y
`plugins` se resuelven igual. Un cuarto camino para una categoría
nada más sería una excepción que después se copia a otra.

**El TTL es de una hora.** `TTL = time.Hour`: cuánto vale una copia
local antes de volver a preguntar. Es el mismo criterio que usa pacman
con su DB.

**El embebido es el último recurso.** `Load` devuelve la copia
compilada en el binario solo si no hay caché fresca, no hay red y no
hay caché viejo. El argumento `embedded` lo pasa el llamador: el
registry no sabe qué hay embebido en cada categoría.

**`Sync` es el "pacman -Sy" del CLI.** Ignora el TTL y fuerza la
descarga. Si la red falla, devuelve el error **y** el estado de lo que
va a servir: el error dice "(se usa la copia local de ...)" o "(se usa
la copia embebida)". Un fallo de red no es motivo para quedarse sin
catálogo.

**`Stat` no toca la red.** Describe lo que hay en disco. `b3l0w sync
--check` lo usa para mostrar el estado sin gastar una consulta.

**La red es `raw.githubusercontent.com`.** Sin GitHub Pages y sin
workflow de publicación: los JSON son la fuente, el repo es el
repositorio.

**Las rutas dentro del repo no se mueven.** `system.json`,
`services/services.json` y `plugins/plugins.json` son las que hay. El
registry cambia cómo se leen, no la estructura del repo.

**Respuesta acotada a 4 MiB.** `maxSize` recorta el body: ninguna DB
pasa de unos KB, y un límite evita que un error del servidor con un
body gigante se coma la memoria.

**User-Agent propio.** Todas las requests van como `b3l0w`.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `registry.go` | Todo el módulo: `Kind`, `Source`, `Status`, `Load`, `Sync`, `Stat`, `All`, `CachePath` |
| `registry_test.go` | Tests de la jerarquía completa con servidor HTTP de prueba |

## Qué NO hace

- **No interpreta el contenido de las DB.** Devuelve bytes: parsearlos
  es de los paquetes que las consumen.
- **No firma ni publica nada.** Solo lee.
- **No decide qué hacer con los datos.** No instala servicios, no
  actualiza plugins.
- **No usa GitHub Pages.** La fuente es `raw.githubusercontent.com`.
- **No es un gestor de paquetes.** No reemplaza al repo de pacman.

## Tests

7 tests: el embebido cuando no hay red, la descarga con cacheo (y que
la segunda lectura no toque la red), el caché viejo cuando la red
falla, el error cuando no hay nada de nada, que `Sync` ignore el TTL,
que `Sync` con red caída devuelva error pero use el caché viejo, y el
estado local de `Stat`.
