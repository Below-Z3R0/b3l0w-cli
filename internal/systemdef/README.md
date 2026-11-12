# systemdef

Lee la definición del sistema: los perfiles que el instalador y
`b3l0w install system` aplican.

## Qué hace y por qué existe

La definición del sistema es un JSON que dice qué se instala en cada
perfil: `desktop`, `dev`, `gaming`, `tools`. Vive en el repo de
distribución (`Below-Z3R0/b3l0w`) como `system.json`.

El módulo tiene tres fuentes y una jerarquía:

```
caché fresco (1 h) -> red -> caché viejo -> embebido
```

El archivo va embebido en el binario (`go:embed`): así el CLI sabe
qué es "su" sistema sin depender de un archivo al lado. `--file`
permite usar otra definición.

## API

| Función | Qué hace |
|---|---|
| `Builtin() (*Definition, error)` | La definición embebida en el binario |
| `Load(path string) (*Definition, error)` | Lee una definición de un archivo |
| `LoadRemote() (*Definition, error)` | La definición del repo de distribución, con cache y fallback |
| `RemoteBytes() ([]byte, error)` | Baja el `system.json` remoto sin cache |
| `BuiltinBytes() []byte` | El `system.json` embebido, en bruto |
| `(*Definition) Names() []string` | Los nombres de los perfiles disponibles |
| `(*Definition) Merge(names []string) (Profile, error)` | Junta varios perfiles en uno, sin duplicados |

### Tipos

```go
type File struct {
    Src  string `json:"src"`
    Dest string `json:"dest"`
}

type Profile struct {
    Repo     []string `json:"repo,omitempty"`
    AUR      []string `json:"aur,omitempty"`
    Flatpak  []string `json:"flatpak,omitempty"`
    Own      []string `json:"own,omitempty"`
    Services []string `json:"services,omitempty"`
    Groups   []string `json:"groups,omitempty"`
    Files    []File   `json:"files,omitempty"`
}

type Definition struct {
    Version  int                `json:"version"`
    Profiles map[string]Profile `json:"profiles"`
}
```

## Decisiones

**Las cuatro fuentes están separadas a propósito.** Un paquete de
pacman, uno de AUR, una app de Flatpak y un binario propio no se
instalan igual, y mezclarlos obliga a adivinar después. `Repo`, `AUR`,
`Flatpak` y `Own` son categorías distintas de una sola vez.

**El embebido es el último recurso.** `LoadRemote` prueba la caché
fresca, después la red, después la caché vieja, y recién ahí cae al
`Builtin()`. El CLI sigue funcionando sin red y no depende de un
release para cambiar un perfil.

**La caché va a `os.UserCacheDir()`.** `~/.cache/b3l0w/system.json`
(respetando `XDG_CACHE_HOME`). Es una copia de un archivo remoto
descargable: es caché, no estado.

**Se usa la API de contenidos con `Accept: raw`, no `raw.githubusercontent`.**
El motivo está en el código: raw cachea y no acepta query strings, así
que no hay forma de pedirle una copia fresca. La API de contenidos con
`Accept: application/vnd.github.raw+json` devuelve el archivo crudo sin
ese cacheo.

**El parseo valida.** Una definición sin `version` o sin perfiles es un
error, no una definición vacía: `parse` la rechaza. Un `version: 0` es
señal de un archivo mal formado.

**`Merge` deduplica por categoría, no por nombre.** El mismo nombre
puede ser paquete en un perfil y binario propio en otro, y no son lo
mismo. Cada categoría tiene su propio conjunto de "vistos", así que
`docker` en `repo` y `docker` en `own` conviven sin pisarse.

**`Merge` falla con un perfil desconocido.** No lo ignora en
silencio: un perfil mal escrito que se ignora deja el sistema a medias
sin aviso.

**Los archivos (`Files`) no se deduplican.** Se anexan tal cual: son
copias de dotfiles con origen y destino, y dos perfiles pueden querer
el mismo archivo escrito en el mismo destino.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `systemdef.go` | Tipos `File`, `Profile`, `Definition` y `Builtin`, `Load`, `Names`, `Merge` |
| `remote.go` | `LoadRemote`, `RemoteBytes`, `BuiltinBytes` y el cache del remoto |
| `system.json` | La definición embebida (perfiles `desktop`, `dev`, `gaming`, `tools`) |
| `systemdef_test.go` | Tests de la definición embebida y del merge |
| `remote_test.go` | Tests del remoto y del fallback |
| `remote_bytes_test.go` | Test del `system.json` remoto contra GitHub |

## Qué NO hace

- **No instala nada.** Solo define qué hay que instalar. Instalar es de
  `internal/systeminstall`.
- **No resuelve conflictos entre perfiles.** Junta; no decide.
- **No valida que los paquetes existan.** Un paquete que ya no está en
  el repo es un problema del instalador, no de la definición.
- **No valida el `Src` ni el `Dest` de un `File`.** Solo los copia.
- **No publica la definición.** La publica el repo de distribución.

## Tests

8 tests: que la definición embebida tenga los cuatro perfiles
esperados, que `docker` esté en `desktop` y no duplicado en `dev`, que
`Merge` no duplique al pedir el mismo perfil dos veces, el error con un
perfil desconocido, que `Merge` junte los paquetes de dos perfiles, el
remoto con un servidor de prueba, el fallback al embed cuando el
remoto falla, y la descarga del `system.json` real de GitHub
(compara longitudes con el embebido).
