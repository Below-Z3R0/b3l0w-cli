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
data pública), pero el CLI pide token antes de hablar con la API: la única
excepción es `covers --library FILE`, que trabaja sobre el archivo de
biblioteca y corre sin token. El `data-dir` por defecto sale de
`NOCTALIA_STATE_HOME`, después de `XDG_STATE_HOME` y por último de
`~/.local/state` (`DataDir`).

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

El flag `--user` significa lo mismo en todos los subcomandos: el **id numérico**
de AniList o el **nombre** del usuario. Un id numérico se usa tal cual; un
nombre se resuelve contra la API antes de consultar la biblioteca. `0` (o
vacío) significa "el del token", como en el flag anterior.

## La biblioteca local

`library.go` pagina `MediaListCollection` de a `ChunkSize` (500) entradas y
arranca en el **chunk 1**: la API devuelve la misma página para el `chunk: 0`
que para el `chunk: 1`, así que pedir el 0 es una consulta de más. Mergea por
`mediaId` —la misma obra puede estar en dos listas: gana la más reciente por
`updatedAt` y, cuando hay empate (el reloj de AniList tiene resolución de
segundos), la más completa: más progreso, después nota, después la que no está
en `PLANNING`— y ordena por título. `Sync` trae el `Viewer` y las dos listas
(anime y manga); si una falla, **no tira el trabajo a la basura**: devuelve la
otra con `Partial=true`. Si fallan las dos, devuelve error.

`libraryfile.go` es el parser del `library.json` que escribe el plugin de
Noctalia. Decodifica a mano porque una tabla Lua vacía se serializa como `{}`
y no como `[]`, y `json.Unmarshal` a `[]Entry` explota con eso. Un `viewer`
mal formado es un error, no un cero silencioso. En el data dir, `library.json`,
`covers.json` y `token.json` los escribe el plugin de Noctalia (el primero es
la entrada de `--library` y la pista del dueño del token); `stats.json` lo
escribe este módulo (caché).

## Las carátulas

`covers.go` deja un JPG por entrada en `<covers-dir>/<mediaId>.jpg` y al lado
un `<mediaId>.jpg.meta.json` con la URL de origen y `CoverVersion` (2). Una
portada es válida sólo si el archivo existe, el meta parsea y la URL
coincide: AniList cambia la imagen al anunciar temporada y sin esto el panel
mostraría la carátula vieja para siempre.

Las descargas corren en paralelo (`--workers`, 6 por defecto), escriben a
`.part` y recién ahí renombran —la UI nunca ve un JPG a medio escribir—. El
tope de 16 MB por imagen es un **límite, no un truncamiento**: una imagen más
grande se rechaza y no se cachea. El `.meta.json` se escribe también de forma
atómica (`.part` + rename) y **después** del rename del JPG: un lector
concurrente nunca ve un meta nuevo con la imagen vieja.

`--reuse DIR` adopta un caché anterior con hardlinks (68 MB de carátulas ya
bajadas no se vuelven a bajar ni ocupan el doble), con copia como fallback si
el hardlink falla. El `.meta.json` se copia, nunca se hardlinkea: compartir el
inode hacía que reescribir el meta del destino corrompiera el caché de origen.
El contador `reused` cuenta **carátulas**, no archivos. `--gc` borra los JPG
que ya no corresponden a ninguna entrada (una descarga fallida no vuelve
borrable la carátula que ya estaba).

El mapa `mediaId → ruta local` que devuelve `SyncCovers` es la única fuente de
verdad para la UI: el modelo de la lista no lleva rutas, así que un refresh no
puede borrarlas.

## Descubrimiento y agenda

`discover.go` trae una página del catálogo. `seasonal` no es un sort: es
popularidad filtrada por la temporada en curso, y se resuelve **antes** de
buscar en el mapa de sorts (si no, el fallback lo convierte en trending sin
avisar). La sinopsis pasa por `stripHTML` porque a veces viene con HTML pese
a pedir `asHtml: false`: el filtro quita las etiquetas, decodifica las
entidades (`&mdash;` → `—`) y no se come el texto que sigue a un `<` suelto.

`schedule.go` arma la agenda de emisión en dos consultas: primero las series
**de anime** en curso del usuario (el filtro `type: ANIME` evita que la manga
ocupe los slots de la agenda), después **una** consulta con `mediaId_in` para
todos los capítulos futuros —no una consulta por serie—. La etiqueta
`mediaType` sale del `type` que trae la media, no de un valor fijo.

## Estadísticas

`stats.go` calcula totales de anime (entradas, capítulos, minutos, días,
nota media) y de manga (entradas, capítulos, tomos, nota media), el top 5 de
géneros y la distribución de notas.

- **Fallback**: si AniList devuelve `count` pero géneros y notas vacíos —le
  pasa a muchas cuentas porque la API cachea los agregados por cuenta— se
  recalcula desde la lista (`statsBreakdown`, excluye `PLANNING`, deduplica
  por `mediaId`). Las notas se piden en `POINT_100`, la misma escala de
  `statistics.scores`, así que la distribución no cambia de escala según el
  camino. Si el fallback falla, los totales se conservan igual.
- **Caché**: `LoadStats` sirve `stats.json` si tiene menos de `StatsCacheTTL`
  (30 minutos) **y su dueño se puede verificar**: con `--user` se compara
  contra ese nombre, y sin `--user` contra el `viewer` de `library.json`. Si
  no hay `library.json` o el `viewer` está mal formado, el caché **no** se
  sirve: servirlo sería mostrarle al usuario los totales de otra cuenta. El
  caché se consulta *antes* que el `Viewer`, así que el panel abre sin red si
  tiene stats frescas y la biblioteca sincronizada. Con `--refresh` se fuerza
  la consulta.

## Decisiones técnicas

- **GraphQL y no REST**: una consulta arma el árbol completo y
  `MediaListCollection` pagina con `chunk`/`perChunk`.
- **Caché local con TTL** en vez de golpear la API en cada apertura del panel.
- **Validación por URL + versión** en las carátulas y no por fecha de descarga:
  la URL es la que cambia cuando AniList renueva la imagen.
- **Escrituras atómicas** (`.part`/`.tmp` + rename) en carátulas —imagen y
  meta— y en `stats.json`: el plugin nunca lee un archivo a medio escribir.
- **Resultados parciales**: `Sync` prefiere devolver la mitad de la biblioteca
  a no devolver nada.
- **Nombres de campo cortos** en el JSON (`mediaId`, `coverUrl`, `airingAt`):
  cada byte viaja al runtime de Luau.
- **Un `--user`, un significado**: id numérico o nombre, en todos los
  subcomandos.

## Archivos del módulo

| Archivo | Responsabilidad |
|---|---|
| `client.go` | cliente GraphQL, reintentos, `NewClient`, `Query` |
| `token.go` | `DataDir`, `Token` |
| `users.go` | `FetchUserByName`, `FetchUserByID` (resolver `--user`) |
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

`go test ./internal/plugins/anilist/cli/` — los de la capa de parseo y salida
usan un `RoundTripper` falso; tampoco tocan la API real.

El detalle de qué prueba cada test está en [`tests.md`](tests.md).

| Archivo | Qué cubre |
|---|---|
| `stats_test.go` | traducción de `statistics`, fallback desde la lista, slices vacíos, TTL del caché, usuario o cuenta ajena, error sin escribir caché |
| `covers_test.go` | descarga, reuso por meta, GC de huérfanas, conteo de fallos, escritura del `.meta.json` |
| `library_test.go` | paginación, merge por `mediaId`, orden por título, `Partial` cuando una lista falla |
| `libraryfile_test.go` | `{}` como lista vacía, entradas reales, JSON inválido |
| `token_test.go` | precedencia flag → env → `token.json`, error sin filtrar el secreto |
| `fixes_test.go` | hallazgos CRÍTICOS y ALTOS (C1, A1–A4) |
| `medio_test.go` | hallazgos MEDIO y BAJO (M1–M9, B1–B3, B5) |
| `cli/root_test.go` | despacho y códigos de salida, flags de cada comando, stdout con `--json` como **un solo** documento JSON, errores a stderr, `status` y el caché de carátulas |
| `cli/gc_test.go` | el GC con biblioteca parcial (C1) |
| `cli/user_test.go` | el flag `--user` en todos los subcomandos (B4, B5) |
