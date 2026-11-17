# anilist — backend de AniList del CLI

Backend de datos de la serie de anime: habla con la API GraphQL de
[AniList](https://anilist.co), mantiene una biblioteca local, sincroniza
carátulas y calcula estadísticas.

Acá vive **todo** el trabajo caro: la API, la paginación, la descarga de
imágenes y el caché. El plugin de Noctalia (`b3l0w/anilist`, del repo
`b3l0w-plugins`) es sólo la UI y llama a este binario.

## Cómo se invoca

No es un plugin de Noctalia: es una **extensión del CLI**. `cmd/b3l0w-anilist`
compila a `b3l0w-anilist` en `~/.local/lib/b3l0w/plugins/` y el CLI la resuelve
por nombre (`internal/plugins/plugins.go`, `Find` + `Run`):

```bash
b3l0w anilist <comando> [opciones]
```

`b3l0w plugin-exec` es otra cosa: ejecuta backends bash embebidos
(`internal/plugins/scripts/`). El plugin de Noctalia llama a esta extensión
con `b3l0w anilist … --json` y lee el JSON de stdout. El parsing y la salida
viven en `cli/root.go` (`anilistcli`): allá la lógica, acá el trabajo con la
API.

## La API de AniList

**GraphQL, no REST.** Un solo endpoint (`https://graphql.anilist.co`,
constante `Endpoint` en `client.go`), POST con `{query, variables}`. Cada
consulta arma el árbol de campos que necesita en una ida: la ficha de una
media, su carátula y su próximo capítulo salen juntos, y `MediaListCollection`
pagina con `chunk`/`perChunk` en vez de una consulta por entrada.

El cliente (`client.go`) reintenta hasta 4 veces con backoff exponencial
(800 ms base) y respeta `Retry-After` en 429 y 5xx: sin backoff una biblioteca
de 600 entradas falla a mitad de camino. Un 401 se corta de inmediato con un
mensaje que pide renovar la sesión.

**Autenticación.** `token.go` resuelve el token en este orden: flag explícito `--token`,
variable de entorno `ANILIST_TOKEN`, o `<data-dir>/token.json` con el campo
`access_token`. Nunca se imprime: el error dice sólo qué falta. Sin token el
cliente Go funciona igual (no manda `Authorization`, así que sólo alcanza la
data pública), pero el CLI exige token antes de correr cualquier comando. El
`data-dir` por defecto es `~/.local/state/noctalia/plugins/data/b3l0w/anilist`
(`DataDir`).

## Subcomandos

| Comando | Qué hace | Archivos | Qué escribe en disco |
|---|---|---|---|
| `sync` | biblioteca + carátulas en un paso | `library.go`, `covers.go` | `<covers-dir>/*.jpg` + `.meta.json` |
| `library` | sólo la biblioteca del usuario | `library.go` | nada |
| `covers` | sólo carátulas, desde `--library FILE` o desde la API | `covers.go`, `libraryfile.go` | `<covers-dir>/*.jpg` + `.meta.json` |
| `discover` | catálogo: trending/popular/top/new/seasonal | `discover.go` | nada (opcionalmente carátulas con `--covers-dir`) |
| `stats` | totales, géneros y notas | `stats.go` | `<data-dir>/stats.json` (caché) |
| `media` | ficha de una serie | `media.go` | nada |
| `schedule` | agenda de emisión | `schedule.go` | nada |
| `mutate` | guarda progreso/estado | `mutate.go` | nada |
| `status` | estado del token y del caché de carátulas | `cli/root.go`, `token.go` | nada |

`--json` saca el contrato en stdout (lo que consume el plugin de Noctalia);
sin el flag, texto humano.

## La biblioteca local

`library.go` pagina `MediaListCollection` de a `ChunkSize` (50) entradas,
mergea por `mediaId` —la misma obra puede estar en dos listas, gana la más
reciente por `updatedAt`— y ordena por título. `Sync` trae el `Viewer` y las
dos listas (anime y manga); si una falla, **no tira el trabajo a la basura**:
devuelve la otra con `Partial=true`.

`libraryfile.go` es el parser del `library.json` que escribe el plugin de
Noctalia. Decodifica a mano porque una tabla Lua vacía se serializa como `{}`
y no como `[]`, y `json.Unmarshal` a `[]Entry` explota con eso. En el data
dir, `library.json`, `covers.json` y `token.json` los escribe el plugin de
Noctalia (el primero es la entrada de `--library` y la pista del dueño del
token); `stats.json` lo escribe este módulo (caché).

## Las carátulas

`covers.go` deja un JPG por entrada en `<covers-dir>/<mediaId>.jpg` y al lado
un `<mediaId>.jpg.meta.json` con la URL de origen y `CoverVersion` (2). Una
portada es válida sólo si el archivo existe, el meta parsea y la URL
coincide: AniList cambia la imagen al anunciar temporada y sin esto el panel
mostraría la carátula vieja para siempre.

Las descargas corren en paralelo (`--workers`, 6 por defecto), escriben a
`.part` y recién ahí renombran —la UI nunca ve un JPG a medio escribir— con
un tope de 16 MB por imagen. `--reuse DIR` adopta un caché anterior con
hardlinks (68 MB de carátulas ya bajadas no se vuelven a bajar ni ocupan el
doble), con copia como fallback si el hardlink falla. `--gc` borra los JPG que
ya no corresponden a ninguna entrada.

El mapa `mediaId → ruta local` que devuelve `SyncCovers` es la única fuente de
verdad para la UI: el modelo de la lista no lleva rutas, así que un refresh no
puede borrarlas.

## Descubrimiento y agenda

`discover.go` trae una página del catálogo. `seasonal` no es un sort: es
popularidad filtrada por la temporada en curso, y se resuelve **antes** de
buscar en el mapa de sorts (si no, el fallback lo convierte en trending sin
avisar). La sinopsis pasa por `stripHTML` porque a veces viene con HTML pese
a pedir `asHtml: false`.

`schedule.go` arma la agenda de emisión en dos consultas: primero las series
en curso del usuario, después **una** consulta con `mediaId_in` para todos los
capítulos futuros —no una consulta por serie—.

## Estadísticas

`stats.go` calcula totales de anime (entradas, capítulos, minutos, días,
nota media) y de manga (entradas, capítulos, tomos, nota media), el top 5 de
géneros y la distribución de notas.

- **Fallback**: si AniList devuelve `count` pero géneros y notas vacíos —le
  pasa a muchas cuentas porque la API cachea los agregados por cuenta— se
  recalcula desde la lista (`statsBreakdown`, excluye `PLANNING`, deduplica
  por `mediaId`). Si el fallback falla, los totales se conservan igual.
- **Caché**: `LoadStats` sirve `stats.json` si tiene menos de `StatsCacheTTL`
  (30 minutos) y es del usuario pedido; si no, consulta y escribe el caché.
  El caché se consulta *antes* que el `Viewer`, así que el panel abre sin red
  si tiene stats frescas. Con `--refresh` se fuerza la consulta.

## Decisiones técnicas

- **GraphQL y no REST**: una consulta arma el árbol completo y
  `MediaListCollection` pagina con `chunk`/`perChunk`.
- **Caché local con TTL** en vez de golpear la API en cada apertura del panel.
- **Validación por URL + versión** en las carátulas y no por fecha de descarga:
  la URL es la que cambia cuando AniList renueva la imagen.
- **Escrituras atómicas** (`.part`/`.tmp` + rename) en carátulas y en
  `stats.json`: el plugin nunca lee un archivo a medio escribir.
- **Resultados parciales**: `Sync` prefiere devolver la mitad de la biblioteca
  a no devolver nada.
- **Nombres de campo cortos** en el JSON (`mediaId`, `coverUrl`, `airingAt`):
  cada byte viaja al runtime de Luau.
- **Nota**: `library.go` pagina de a 50 y `stats.go` de a 500 sobre la misma
  `MediaListCollection` (verificar por qué difieren).

## Archivos del módulo

| Archivo | Responsabilidad |
|---|---|
| `client.go` | cliente GraphQL, reintentos, `NewClient`, `Query` |
| `token.go` | `DataDir`, `Token` |
| `discover.go` | `Discover`, `DiscoverPage`, `DiscoverItem`, `stripHTML` |
| `library.go` | `Sync`, `FetchList`, `FetchViewer`, `Entry`, `Library` |
| `libraryfile.go` | `ReadLibraryFile` (tolerante a `{}` de Lua) |
| `media.go` | `FetchMedia`, `MediaDetail`, `AiringEpisode` |
| `covers.go` | `SyncCovers`, `CoverStats`, `CoverOptions`, meta, GC, hardlinks |
| `schedule.go` | `FetchSchedule`, `Schedule`, `AiringItem` |
| `stats.go` | `FetchStats`, `LoadStats`, `StatsPath`, breakdown, caché |
| `mutate.go` | `SaveProgress`, `Mutation` |
| `cli/root.go` | subcomandos, flags y salida (`anilistcli`) |

## Tests

`go test ./internal/plugins/anilist/` — los de GraphQL y de descarga corren
contra `httptest.Server`; ninguno toca la API real.

| Archivo | Qué cubre |
|---|---|
| `stats_test.go` | traducción de `statistics`, fallback desde la lista, slices vacíos, TTL del caché, usuario o cuenta ajena, error sin escribir caché |
| `covers_test.go` | descarga, reuso por meta, GC de huérfanas, conteo de fallos, escritura del `.meta.json` |
| `library_test.go` | paginación, merge por `mediaId`, orden por título, `Partial` cuando una lista falla |
| `libraryfile_test.go` | `{}` como lista vacía, entradas reales, JSON inválido |
| `token_test.go` | precedencia flag → env → `token.json`, error sin filtrar el secreto |
