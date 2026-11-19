# Bugs de `internal/plugins/anilist`

Revisión de integridad en contexto cero, hecha sobre `main` en `aebf878`.
**No se arregló nada**: este archivo sólo describe lo que se encontró.

Método: se leyó el módulo completo (producción + tests) y se contrastó cada
afirmación del `README.md` contra el código. Los hallazgos se reprodujeron
contra la **API real de AniList** (usuarios públicos 2, 5, 7, 9, 12, 25, 45,
55, 95, 110, 175) y contra una **copia aislada** del paquete en
`~/.hermes/profiles/coding/cache/scratch/probe` (el repo no se tocó: `go test`
y `go build` siguen verdes y el árbol quedó como estaba).

Los probes que se citan son de tres tipos:

- **API**: `curl` a `https://graphql.anilist.co` con la query exacta del módulo.
- **Go**: un `_test.go` temporario en la copia del paquete, con un
  `httptest.Server` que devuelve lo que hace fallar la cosa.
- **CLI**: un `main.go` temporario en `cmd/probe-cli-anilist/` dentro del
  repo (necesario para usar el paquete `internal/`), **borrado después**;
  intercepta `http.DefaultTransport` como hacen los tests de `cli/root_test.go`.

## Resumen

| ID | Sev | Archivo:línea | Síntoma |
|---|---|---|---|
| C1 | CRÍTICO | `library.go:193-201` + `covers.go:203-205` + `cli/root.go:144-149` | `sync --gc` **borra todas las carátulas** si la API falla, y sale con código 0 |
| A1 | ALTO | `covers.go:194,230` | Una descarga fallida (500/404) hace que el GC borre el JPG de esa entrada |
| A2 | ALTO | `covers.go:91,149` | El hardlink de `--reuse` + `os.WriteFile` del meta **corrompe el caché de origen**: la carátula vieja se presenta como nueva |
| A3 | ALTO | `schedule.go:37-38` | La agenda se corta en las **primeras 50** series CURRENT (119 reales → 50) |
| A4 | ALTO | `schedule.go:54-55` | La segunda query de la agenda se corta en **50 capítulos** (`hasNextPage: true` ignorado) |
| M1 | MEDIO | `schedule.go:38,136,156` | `mediaList` sin filtro de tipo mete manga en la lista y todo se etiqueta `ANIME` |
| M2 | MEDIO | `stats.go:96,234` | El fallback de stats usa la escala nativa del usuario; `statistics.scores` viene en 0-100 |
| M3 | MEDIO | `library.go:11-12,150` | `perChunk: 50` cuando la API acepta 500: 10× más consultas de las necesarias (y el comentario es falso) |
| M4 | MEDIO | `library.go:145-168` | `chunk: 0` y `chunk: 1` devuelven **la misma página**: una consulta de más por biblioteca |
| M5 | MEDIO | `library.go:159` | Empate de `updatedAt` en el merge: gana el primero, no el más completo |
| M6 | MEDIO | `covers.go:92,97` + `cli/root.go:219-220` | `adopt()` cuenta archivos, no carátulas: "reusadas" sale duplicado |
| M7 | MEDIO | `covers.go:139` | El tope de 16 MB **trunca** la imagen en silencio y guarda un JPEG corrupto como válido |
| M8 | MEDIO | `stats.go:343-351` + `libraryfile.go:32` | Sin `library.json`, el caché de stats de **otra cuenta** se sirve |
| M9 | MEDIO | `token.go:17-26` | `DataDir` ignora `XDG_STATE_HOME` y `NOCTALIA_STATE_HOME` |
| B1 | BAJO | `covers.go:149` vs `153` | El meta se escribe in situ y **antes** del rename del `.part` |
| B2 | BAJO | `discover.go:158-176` | `stripHTML` no decodifica entidades y un `<` suelto se come el resto |
| B3 | BAJO | `library.go:152-155` | `MediaListCollection: null` → biblioteca vacía **sin error** |
| B4 | BAJO | `cli/root.go:97` vs `304` | `--user` es `int` en `library`/`sync` y `string` en `stats` |
| B5 | BAJO | `README.md:43-44` | El README dice que el CLI exige token; `covers --library` funciona sin él |

**Totales:** 1 CRÍTICO · 4 ALTO · 9 MEDIO · 5 BAJO — 19 hallazgos.

---

## C1 — CRÍTICO · `sync --gc` borra todas las carátulas si la API falla

**Dónde.** `internal/plugins/anilist/library.go:193-201`, `covers.go:203-205`,
`cli/root.go:144-149`.

**Síntoma.** Si las dos listas de AniList fallan (429, 500, timeout), `sync --gc`
borra **todos** los JPG del directorio de carátulas y termina con **código de
salida 0**. El plugin cree que la sincronización fue un éxito y muestra la
biblioteca vacía.

**Causa.** `Sync` se come el error de `FetchList`: en `library.go:193-201` el
`err` se descarta y sólo se marca `Partial`. `library.go:203-208` convierte las
listas `nil` en listas vacías. Después `cli/root.go:144` arma `all` con esas
listas vacías y `cli/root.go:145-149` pasa ese `all` a `SyncCovers` con `GC:
true`. En `covers.go:204`, `gcOrphans` borra todo `.jpg` cuyo `mediaId` no esté
en `stats.Covers` — y `stats.Covers` está vacío porque no se descargó nada.

Es decir: **"no pude traer la lista" se confunde con "esa entrada ya no está en
la biblioteca"**, y lo que la biblioteca tiene se borra del disco.

**Reproducción (CLI, punta a punta).** Se intercepta el transporte HTTP con un
`RoundTripper` falso (como hace `cli/root_test.go:137-140`), se responde el
`Viewer` y se hace fallar cualquier otra query:

```go
// cmd/probe-cli-anilist/main.go (temporal, borrado después)
func (falso) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet {
		return resp(http.StatusOK, []byte("JPEG")), nil
	}
	raw, _ := io.ReadAll(req.Body)
	if strings.Contains(string(raw), "Viewer") {
		return resp(http.StatusOK, []byte(`{"data":{"Viewer":{"id":42,"name":"b3low"}}}`)), nil
	}
	return resp(http.StatusOK, []byte(`{"errors":[{"message":"Too Many Requests."}]}`)), nil
}
```

```console
$ go run ./cmd/probe-cli-anilist/
== comando: b3l0w anilist sync --gc --token X --data-dir DIR ==
usuario: b3low (#42)
anime:   0
manga:   0
carátulas: 0 válidas (0 nuevas, 0 fallidas)
aviso: una de las listas no se pudo traer
codigo de salida = 0
caratulas antes = 5, despues = 0
```

**Impacto.** En la máquina de desarrollo el directorio real tenía 453 carátulas
(93,7 MB). Con un 429 de AniList durante un `sync --gc`, se borran las 453 y
tienen que volver a bajarse. El aviso "una de las listas no se pudo traer" queda
en la salida humana, pero **no** en el JSON: el plugin lee `covers: {}` y no
`partial`.

**Variante de paquete (sólo falla MANGA).** Con `SyncCovers` y GC, la lista de
anime viene bien y la de manga falla: las carátulas de anime sobreviven, las de
manga se borran.

```
P1) Sync err=<nil> partial=true anime=2 manga=0
P1) Removed=4 quedan=[]  (3 y 4 son de MANGA: la lista fallo y sus caratulas se borraron)
```

---

## A1 — ALTO · El GC borra la carátula de una entrada cuya descarga falló

**Dónde.** `internal/plugins/anilist/covers.go:194` (el `stats.Failed++`) y
`covers.go:230` (el `os.Remove` del GC).

**Síntoma.** Una descarga que falla por un 500 del CDN deja la entrada **sin
carátula**: el `.jpg` anterior se borra y no queda nada.

**Causa.** `covers.go:190-198`: si `downloadCover` falla, la entrada **no** entra
en `stats.Covers`. Después `covers.go:204` llama a `gcOrphans`, que en
`covers.go:226-233` borra todo `.jpg` cuyo id no esté en ese mapa. El GC no
distingue "esta entrada no está en la biblioteca" de "esta entrada no se pudo
bajar".

**Reproducción.** Servidor que responde 500 en el CDN:

```
A) tras la 1a pasada la carátula existe=true
A) Failed=1 Removed=1  la carátula SIGUE en disco=false (mapa=map[])
```

Y con un `.jpg` sano al que sólo le falta el `.meta.json` (escritura
interrumpida), un 404 en el CDN:

```
P2) Failed=1 Removed=1 el .jpg bueno sigue=false (valido=false)
```

**Impacto.** Un 500 transitorio del CDN de AniList deja al panel sin carátula
hasta la próxima sincronización. Con C1 es el mismo mecanismo aplicado a toda la
biblioteca.

---

## A2 — ALTO · El hardlink de `--reuse` corrompe el caché de origen

**Dónde.** `internal/plugins/anilist/covers.go:91` (`os.Link`) y `covers.go:149`
(`os.WriteFile` del meta).

**Síntoma.** Con `--reuse DIR`, la primera sincronización **reescribe el
`meta.json` del directorio de origen**. Después, si se vuelve a adoptar ese
directorio, la carátula **vieja** se presenta como si fuera la nueva y no se
vuelve a baixar nunca.

**Causa.** `adopt()` (covers.go:73-101) hace `os.Link` del `.jpg` **y** del
`.meta.json` del directorio de origen al destino: el `.meta.json` queda
**compartiendo inode**. Cuando la portada de AniList cambia de URL, el `.jpg`
adoptado queda viejo (URL distinta ⇒ `validCover` da false, covers.go:53-69) y
se re-descarga. Al escribir el meta nuevo, `os.WriteFile` (covers.go:149)
**trunca y escribe in situ**: como el inode es el mismo, el `meta.json` del
directorio de origen queda con la URL nueva, apuntando a un `.jpg` viejo. En la
siguiente adopción, `validCover` ve meta nuevo + `.jpg` viejo y lo acepta.

**Reproducción.**

```
E) meta del cache VIEJO ahora dice: {"url":"http://127.0.0.1:45311/NUEVA.jpg","version":2}
E) re-adoptado: Downloaded=0 (0 = no lo bajó) contenido="IMAGEN-VIEJA" -> la carátula VIEJA se muestra como NUEVA
```

**Impacto.** El caché que el usuario pasó con `--reuse` queda corrupto para
siempre: cualquier sincronización futura con ese directorio muestra carátulas
viejas como nuevas y no las vuelve a bajar (el `meta` dice que ya están al día).

---

## A3 — ALTO · La agenda se corta en las primeras 50 series CURRENT

**Dónde.** `internal/plugins/anilist/schedule.go:37-38`.

**Síntoma.** Un usuario con más de 50 series en curso ve en la agenda **sólo las
primeras 50**. Las demás no aparecen en ningún lado: no hay paginación.

**Causa.** `currentListQuery` pide `Page(page: 1, perPage: 50) { mediaList(...) }`.
El `Page` de AniList corta `mediaList` a 50 entradas y `schedule.go` no pide la
página siguiente ni mira `pageInfo`.

**Reproducción (API real).** Usuario 95, con la query exacta de `schedule.go:36-49`:

```
user 95:
   modulo (Page perPage=50) -> 50 entradas, 50 de ANIME
   verdad CURRENT ANIME   -> 119
```

Las 69 series que faltan son series que el usuario está viendo ahora mismo.

---

## A4 — ALTO · La segunda query de la agenda se corta en 50 capítulos

**Dónde.** `internal/plugins/anilist/schedule.go:54-55`.

**Síntoma.** La query de capítulos futuros devuelve **como máximo 50** capítulos
y el resto no aparece. `hasNextPage` viene en `true` y se ignora.

**Causa.** `scheduleQuery` mete `airingSchedules` dentro de
`Page(page: 1, perPage: 50)`. La página 2 de esa misma `Page` trae los
capítulos siguientes, pero `schedule.go:145-147` hace una sola consulta.

**Reproducción (API real).** 40 mediasTrending, `notYetAired: true`:

```
=== page 1 === {"n":50,"primeros":[180136,210482,195516],"pageInfo":{"hasNextPage":true,"total":5000}}
=== page 2 === {"n":43,"primeros":[206401,180136,195516],"pageInfo":{"hasNextPage":false,"total":93}}
```

93 capítulos posibles, 50 en la salida.

---

## M1 — MEDIO · `mediaList` mete manga en la lista y todo se etiqueta `ANIME`

**Dónde.** `internal/plugins/anilist/schedule.go:38` (la query) y
`schedule.go:136,156` (el `MediaType: "ANIME"` fijo).

**Síntoma.** La query de "series en curso" no filtra por tipo: devuelve anime **y**
manga. El módulo etiqueta `MediaType: "ANIME"` a mano en los dos lugares donde
arma un `AiringItem`, así que la etiqueta miente. Peor: los slots de 50 se
llenan con manga que no van a aparecer en la agenda, y las series de anime
quedan fuera.

**Causa.** Falta `type: ANIME` en el argumento `mediaList` de
`currentListQuery` (`schedule.go:38`), y el tipo se fija a mano en vez de leerse
de `media { type }`, que la query ya pide (`schedule.go:42`).

**Reproducción (API real).**

```
user 12 => {"n":17,"anime":14,"manga":3}
user 45 => {"n":33,"anime":20,"manga":13}
user 5  => {"n":43,"anime":3,"manga":40}
```

Para el usuario 45, 13 de los 33 slots los ocupa manga. Para el usuario 5, 40 de
43. Combinado con A3, la cuenta de anime que llega a la agenda baja a 20 de 20
(usuario 45) pero a 3 de 3 (usuario 5) con 40 manga耗材 el presupuesto.

---

## M2 — MEDIO · El fallback de stats cuenta las notas en otra escala

**Dónde.** `internal/plugins/anilist/stats.go:96` (la query del fallback) y
`stats.go:234` (el redondeo).

**Síntoma.** Cuando la API devuelve los agregados vacíos y entra el fallback, la
distribución de notas del panel sale en la **escala del usuario** (0-10 para
quien usa POINT_10). Cuando la responde la API, sale en **0-100**. El mismo
campo `scores` cambia de escala según el camino.

**Causa.** `statsListQuery` (`stats.go:96`) pide `score` sin `format`, y
AniList lo devuelve en la escala de la lista del usuario. El comentario de
`stats.go:88-90` afirma que "es la misma que usaría `statistics.scores`", y no
lo es: `statistics.anime.scores` siempre está en 0-100. `stats.go:234` redondea
el valor nativo, así que los cubos quedan en 1-10 en vez de 10-100.

**Reproducción (API real).** Usuario 55 (lista en POINT_10):

```
user 55: statistics.scores=[5,10,20,30,40,50,60,65,70,75,80,85,90,100]
         score nativo de la lista=[0.5,1,2,3,4,5,6,6.5,7,7.5,8,8.5,9,10]
```

Los valores 20, 30, … 100 de `statistics.scores` son imposibles en la escala
nativa: la agregación y el fallback no hablan la misma lengua.

---

## M3 — MEDIO · `perChunk: 50` cuando la API acepta 500

**Dónde.** `internal/plugins/anilist/library.go:11-12` (el comentario) y
`library.go:150` (el valor).

**Síntoma.** La biblioteca se pagina de a 50 entradas cuando la API acepta
hasta 500: una biblioteca de 600 entradas son **13 consultas** en vez de 2. Cada
consulta es un chances de 429, que es justo lo que el `README.md:36-38` dice
que hay que evitar.

**Causa.** `ChunkSize = 50` (library.go:12) con el comentario "50 es el máximo
que acepta la API para MediaListCollection". **El comentario es falso**: el
máximo real es 500.

**Reproducción (API real).** Mismo usuario, tres `perChunk`:

```
perChunk=50   => {"n":50,  "hasNext":true}
perChunk=100  => {"n":100, "hasNext":true}
perChunk=500  => {"n":500, "hasNext":true}
perChunk=1000 => {"n":500, "hasNext":true}
```

Con `perChunk: 500` la biblioteca entera del usuario 5 (172 entradas) cabe en
**una** consulta.

Esto también responde la nota del propio `README.md:139-140` ("library.go pagina
de a 50 y stats.go de a 500 ... verificar por qué difieren"): no hay razón
técnica, `stats.go:27` ya usa 500.

---

## M4 — MEDIO · `chunk: 0` y `chunk: 1` devuelven la misma página

**Dónde.** `internal/plugins/anilist/library.go:145-168`.

**Síntoma.** La paginación hace **una consulta de más** por biblioteca: pide la
página 1 y recibe la página 0.

**Causa.** No está en el código: la API de AniList responde igual para `chunk:
0` y `chunk: 1` sobre `MediaListCollection` con `lists`. El módulo no lo
detecta y lo pide igual. El `union` de los chunks sigue siendo completo (no se
pierde nada), así que el daño es el costo en consultas y en cuota de API.

**Reproducción (API real).** Mismo usuario, tres chunks consecutivos:

```
user 5 chunk 0 => {"n":50,"ids":[17,20,27,47,53]}
user 5 chunk 1 => {"n":50,"ids":[17,20,27,47,53]}     <-- misma página
user 5 chunk 2 => {"n":50,"ids":[469,478,570,576,590]}
user 95 chunk 0 => {"n":50,"ids":[26,10588,14283,14833,16157]}
user 95 chunk 1 => {"n":50,"ids":[26,10588,14283,14833,16157]}  <-- misma página
```

Y la unión sigue completa (172 = 172), comprobado sobre 4 usuarios en ANIME y
2 en MANGA.

---

## M5 — MEDIO · Empate de `updatedAt` en el merge: gana el primero

**Dónde.** `internal/plugins/anilist/library.go:159`.

**Síntoma.** Cuando la misma obra aparece en dos listas con el **mismo**
`updatedAt` (segundos), el merge se queda con la primera que vio, que puede ser
la de menos progreso. El `README.md:68` promete que "gana la más reciente por
`updatedAt`", pero con empate no hay más reciente: gana la primera.

**Causa.** `if prev, ok := byID[entry.MediaID]; ok && prev.UpdatedAt >= entry.UpdatedAt { continue }`.
Con `>=` el empate se resuelve a favor de la entrada ya guardada. Debería ser
`>` (o desempatar por progreso/estado) para que el empate no arbitrarily froze
la primera.

**Reproducción.** Dos entradas del mismo `mediaId` con `updatedAt: 100` ambas,
la primera `CURRENT` con `progress: 5` y la segunda `COMPLETED` con
`progress: 12`:

```
P3) entradas=1 progress=5 score=80  (updatedAt empatado en 100: gana el primero, no el mas reciente)
```

El test existente (`library_test.go:88`) sólo cubre `updatedAt` distintos
(100 vs 300), así que no lo detecta.

---

## M6 — MEDIO · `adopt()` cuenta archivos, no carátulas

**Dónde.** `internal/plugins/anilist/covers.go:92,97` y el aviso en
`cli/root.go:219-220`.

**Síntoma.** Con `--reuse`, el contador `Reused` (y el texto "carátulas: N
reusadas") vale el doble de carátulas realmente adoptadas: por cada portada se
adoptan **dos** archivos (el `.jpg` y el `.meta.json`) y los dos cuentan.

**Causa.** `adopt()` incrementa `adopted` una vez por archivo,nameRating
adoptado (covers.go:92 para el hardlink, covers.go:97 para la copia). El
contador se llama "reusadas" y se muestra como carátulas.

**Reproducción.**

```
D) Reused=2 (una sola carátula) Covers=map[7:.../7.jpg] Downloaded=0
```

**Impacto.** El número que ve el usuario y el campo `reused` del contrato JSON
mienten por el factor 2. `cli/root_test.go:1569` sólo comprueba `reused > 0`, así
que no lo detecta.

---

## M7 — MEDIO · El tope de 16 MB trunca la imagen en silencio

**Dónde.** `internal/plugins/anilist/covers.go:139`.

**Síntoma.** Una imagen de más de 16 MB se guarda **cortada** a 16 MB, se
renombra a `.jpg`, se le escribe su meta y queda **cacheada como válida para
siempre**: `validCover` sólo mira que el archivo exista y no esté vacío.

**Causa.** `io.Copy(out, io.LimitReader(resp.Body, 16<<20))`. `LimitReader` no
avisa que truncó: `io.Copy` devuelve `nil` y el código sigue como si el
archivo estuviera completo. `covers.go:207-209` sólo mira `info.Size() == 0`.

**Reproducción.** Servidor que manda 17 MB:

```
C) Failed=0 Downloaded=1 tamano=16777216 (origen 17MB) validCover=true
```

`Failed=0`: el módulo cree que la descarga fue un éxito.

**Impacto.** Un JPEG truncado se sirve al panel y no se vuelve a bajar nunca (el
meta coincide). El `README.md:89-90` habla de "un tope de 16 MB por imagen",
que suena a límite, no a truncamiento silencioso.

---

## M8 — MEDIO · Sin `library.json`, el caché de stats de otra cuenta se sirve

**Dónde.** `internal/plugins/anilist/stats.go:343-351` y
`libraryfile.go:32`.

**Síntoma.** Si el directorio de datos no tiene `library.json` (o lo tiene con
la forma equivocada), el `stats.json` cacheado se sirve **sin comprobar de qué
cuenta es**. El panel muestra los totales, géneros y notas de otra cuenta de
AniList.

**Causa.** `readStatsCache` (stats.go:343-351) deriva el dueño esperado de
`libraryViewerName(dataDir)` (stats.go:347). Si no hay `library.json`, `name`
queda `""` y la guarda `if name != "" && cached.User != "" && ...` (stats.go:349)
se cumple sin comparar nada. Lo mismo pasa si `library.json` tiene un `viewer`
mal formado: `libraryfile.go:32` descarta el error de `json.Unmarshal` y deja
`Viewer` en cero, así que `libraryViewerName` (stats.go:357-367) devuelve `""`.

**Reproducción.** `stats.json` de otra cuenta en un directorio sin
`library.json`:

```
I) err=<nil> user="cuentaAjena" count=999 (el cache de otra cuenta se sirvio)
```

Y el eslabón intermedio, con el `viewer` mal formado:

```
G) err=<nil> viewer={ID:0 Name:} fetchedAt=0 (el error de unmarshal se perdio)
```

**Impacto.** El `README.md:121-122` promete que el caché se sirve "si ... es del
usuario pedido". Con el `library.json` ausente —el caso normal después de
cambiar de cuenta o antes de la primera sincronización— esa condición no se
puede evaluar y se sirve igual. El test `stats_test.go:363` cubre el caso con
`library.json` presente, no el ausente.

---

## M9 — MEDIO · `DataDir` ignora `XDG_STATE_HOME` y `NOCTALIA_STATE_HOME`

**Dónde.** `internal/plugins/anilist/token.go:17-26`.

**Síntoma.** El directorio de datos del plugin se resuelve siempre contra
`~/.local/state`, aunque el sistema (o el resto del repo) lo configure por
variables de entorno. Con `NOCTALIA_STATE_HOME` seteado, el CLI no encuentra el
`token.json` que el plugin de Noctalia escribió.

**Causa.** `DataDir` (token.go:17-26) usa `os.UserHomeDir()` + `.local/state`
cuando `stateHome` viene vacío, y `cli/root.go:104,406` siempre lo pasan
vacío. En el mismo repo, `internal/registry/registry.go:188` sí respeta
`XDG_STATE_HOME` y `internal/plugins/ytmusic/model.go:81` sí respeta
`NOCTALIA_STATE_HOME`.

**Reproducción.**

```
M) DataDir("", "b3l0w/anilist") = /home/b3low-z3r0/.local/state/noctalia/plugins/data/b3l0w/anilist
M) XDG_STATE_HOME=/tmp/xdg-state NOCTALIA_STATE_HOME=/tmp/noctalia-state
```

**Impacto.** Depende de dónde escriba el plugin de Noctalia, que está fuera de
este repo; lo verificable es la inconsistencia interna con los dos módulos
hermanos citados arriba.

---

## B1 — BAJO · El meta se escribe in situ y antes del rename del `.part`

**Dónde.** `internal/plugins/anilist/covers.go:149` (escritura) vs `covers.go:153`
(rename).

**Síntoma.** Hay una ventana en la que el `meta.json` de la portada nueva ya
está escrito pero el `.jpg` todavía es el viejo (el rename del `.part` va
después). Un lector concurrente —el plugin— puede ver meta nuevo + imagen vieja
y `validCover` la da por buena.

**Causa.** El `.jpg` se renombra (atómico), pero el meta se escribe con
`os.WriteFile` al path final: **trunca in situ**, sin `.tmp` + rename. El
`README.md:133-134` afirma "Escrituras atómicas (`.part`/`.tmp` + rename) en
carátulas y en `stats.json`: el plugin nunca lee un archivo a medio escribir";
para el `.meta.json` de las carátulas eso no se cumple.

**Reproducción.** Es el mecanismo que hace posible A2: el `os.WriteFile` de
`covers.go:149` sobre un inode compartido con el caché de origen lo reescribe.

---

## B2 — BAJO · `stripHTML` no decodifica entidades y se traga el texto tras un `<`

**Dónde.** `internal/plugins/anilist/discover.go:158-176` (usado también en
`media.go:117`).

**Síntoma.** Dos problemas en la sinopsis:

- Las entidades HTML quedan literales: la API manda `&mdash;` y el panel muestra
  `&mdash;` en vez de `—`.
- Un `<` que no es una etiqueta se come todo lo que viene después:
  `stripHTML("5 < 6 y 7")` devuelve `"5"`.

**Causa.** `stripHTML` quita etiquetas pero no decodifica entidades, y el
contador de profundidad (`depth`) nunca se cierra si el `<` no tiene `>`.

**Reproducción.**

```
Q) stripHTML("a &mdash; b") = "a &mdash; b"
Q) stripHTML("5 < 6 y 7") = "5" (el '<' sucio se come el resto)
```

Y en la API real, 1 descripción de 30 trae `&mdash;`.

---

## B3 — BAJO · `MediaListCollection: null` → biblioteca vacía sin error

**Dónde.** `internal/plugins/anilist/library.go:152-155`.

**Síntoma.** Si la API responde `{"data":{"MediaListCollection":null}}` sin
`errors`, `FetchList` devuelve una lista vacía **sin error**. El usuario ve
"anime: 0" como si no tuviera nada.

**Causa.** `client.go:104` hace `json.Unmarshal(parsed.Data, out)` sobre
`null`, lo que no falla y deja la struct en cero; `library.go:153-155` no
distingue "colección vacía" de "colección nula".

**Reproducción.** Con un servidor falso que devuelve esa respuesta:

```
P) FetchList err=<nil> entradas=0 (biblioteca vacia sin error: indistinguishable de una lista vacia)
P) Sync err=<nil> partial=false anime=0
```

**Nota.** En la API real esta forma no se observó: los usuarios privados
devuelven `{"errors":[{"message":"Private User"}],...}`, que el módulo sí
convierte en error. Queda como borde sinuchar contra el servidor real.

---

## B4 — BAJO · `--user` es `int` en `library`/`sync` y `string` en `stats`

**Dónde.** `internal/plugins/anilist/cli/root.go:97` vs `cli/root.go:304`.

**Síntoma.** El mismo flag significa dos cosas según el subcomando, y el texto
de uso (`cli/root.go:50`) sólo lo documenta para `stats`:

- `b3l0w anilist library --user b3low` → `invalid value "b3low" for flag
  -user: parse error`, salida 2.
- `b3l0w anilist stats --user 123` → lo toma como **nombre de usuario** y
  busca el usuario literal "123": `HTTP 404: {"errors":[{"message":"Not Found."}]}`.

**Causa.** `newFlags` (root.go:97) declara `--user` como `fs.Int` ("user id") y
`runStats` (root.go:304) como `fs.String` ("usuario de AniList").

**Reproducción.**

```console
$ b3l0w-anilist library --user b3low --json
invalid value "b3low" for flag -user: parse error
$ b3l0w-anilist stats --user 123 --json
error: HTTP 404: {"errors":[{"message":"Not Found.","status":404,...}],"data":{"User":null}}
```

---

## B5 — BAJO · El README dice que el CLI exige token; `covers --library` no lo pide

**Dónde.** `internal/plugins/anilist/README.md:43-44`.

**Síntoma.** El README afirma: "Sin token el cliente Go funciona igual (no manda
`Authorization`, así que sólo alcanza la data pública), pero **el CLI exige token
antes de correr cualquier comando**". `covers --library FILE` corre sin token y
devuelve su JSON.

**Causa.** `cli/root.go:182` descarta el error de `client()` a propósito
(`_, dir, _ := client(...)`) porque con `--library` no hace falta hablar con la
API. El comportamiento es intencional y está fijo en
`cli/root_test.go:540` (`TestCoversDesdeElArchivoDeBibliotecaNoPideToken`); lo
que está mal es el README.

**Reproducción.**

```console
$ env -u ANILIST_TOKEN b3l0w-anilist covers --library /tmp/lib.json --json
{"dir":".../covers","covers":{},"downloaded":0,"reused":0,"failed":0,"removed":0,"bytes":0}
$ echo $?
0
```

---

## Afirmaciones del README verificadas

Contrastadas una por una contra el código. Las que se sostienen:

| Afirmación | Dónde se sostiene |
|---|---|
| "GraphQL, no REST. Un solo endpoint ... constante `Endpoint` en `client.go`" | `client.go:15,46` |
| "POST con `{query, variables}`" | `client.go:46` |
| "reintenta hasta 4 veces con backoff exponencial (800 ms base)" | `client.go:50-60` (esperas 800 ms / 1,6 s / 3,2 s) |
| "respeta `Retry-After` en 429 y 5xx" | `client.go:85-87` |
| "Un 401 se corta de inmediato" | `client.go:89-90` |
| "flag explícito `--token`, variable de entorno `ANILIST_TOKEN`, o `<data-dir>/token.json`" | `token.go:30-55` |
| "Nunca se imprime: el error dice sólo qué falta" | `token.go:40-53` |
| "pagina `MediaListCollection` de a `ChunkSize` (50) entradas" | `library.go:150` (pero M3) |
| "mergea por `mediaId`" | `library.go:159` (pero M5) |
| "ordena por título" | `library.go:173-175` |
| "`Sync` trae el `Viewer` y las dos listas" | `library.go:181-202` (pero C1) |
| "si una falla ... devuelve la otra con `Partial=true`" | `library.go:193-201` |
| "Decodifica a mano porque una tabla Lua vacía se serializa como `{}`" | `libraryfile.go:40-49` |
| "`CoverVersion` (2)" | `covers.go:18` |
| "escriben a `.part` y recién ahí renombran" | `covers.go:135,153` (sólo el `.jpg`, ver B1) |
| "`--reuse DIR` adopta un caché anterior con hardlinks ... con copia como fallback" | `covers.go:91-98` (pero A2) |
| "`--gc` borra los JPG que ya no corresponden a ninguna entrada" | `covers.go:215-236` (pero C1, A1) |
| "`seasonal` no es un sort ... se resuelve antes de buscar en el mapa de sorts" | `discover.go:89-98` |
| "La sinopsis pasa por `stripHTML`" | `discover.go:150`, `media.go:117` (pero B2) |
| "`schedule.go` arma la agenda en dos consultas ... una consulta con `mediaId_in`" | `schedule.go:36-66` (pero A3, A4) |
| "totales de anime ... y de manga ... el top 5 de géneros y la distribución de notas" | `stats.go:146-163,261-282` |
| "se recalcula desde la lista (`statsBreakdown`, excluye `PLANNING`, deduplica por `mediaId`)" | `stats.go:189-247` — el "excluye `PLANNING` igual que `statistics.anime.count`" **se confirma**: usuario 5 tiene 172 entradas (119 COMPLETED, 3 CURRENT, 13 DROPPED, 7 PAUSED, 30 PLANNING) y `statistics.anime.count` = 142 = 172 − 30 |
| "Si el fallback falla, los totales se conservan igual" | `stats.go:168-172` |
| "`LoadStats` sirve `stats.json` si tiene menos de `StatsCacheTTL` (30 minutos)" | `stats.go:18,340` |
| "El caché se consulta *antes* que el `Viewer`" | `stats.go:295-299` |
| "Con `--refresh` se fuerza la consulta" | `stats.go:295` |
| "las descargas corren en paralelo (`--workers`, 6 por defecto)" | `covers.go:176-201` |
| "El mapa `mediaId → ruta local` ... es la única fuente de verdad para la UI" | `covers.go:170,198` |
| "Nadie toca la API real en los tests" | los 5 `*_test.go` del paquete y `cli/root_test.go` |

Las que **no** se sostienen: `README.md:43-44` (B5), `README.md:68` (M5),
`README.md:89-90` (M7), `README.md:121-122` (M8), `README.md:133-134` (B1),
`README.md:139-140` (M3, y esta nota queda respondida), y `library.go:11-12`
(M3).

---

## Sin hallazgos

- **La trigonometría del token.** `token.go` resuelve bien la precedencia
  flag → env → `token.json`, no filtra el secreto en el error y maneja el
  `access_token` vacío. Cubierto por `token_test.go:12`.
- **La Query de GraphQL.** `client.go:45-107` reintenta 429/5xx, corta el 401,
  trunca la respuesta a 8 MB, maneja `errors` y `data: null` sin romper. El
  manejo de `Retry-After` es correcto salvo el detalle menor de que el valor
  también se duplica en el siguiente ciclo.
- **`mutate.go`.** La query GraphQL de `SaveMediaListEntry` es válida: se
  confirmó contra la API real que omitir la variable nullable `$status` no
  dispara un error de validación GraphQL (llega a la autorización), o sea que
  `mutate.go:26-28` está bien. La lectura de `MediaList` en `media.go:62,136`
  (que es la del usuario autenticado) coincide con la API.
- **Los slices nulos.** `stats.go:261-282` y `library.go:126-128` garantizan
  arrays vacíos en el JSON, que es lo que el runtime de Luau necesita.
- **`stripHTML` como filtro de etiquetas.** Para HTML bien formado (que es lo
  que trae AniList) funciona; lo que falla son las entidades y el `<` suelto
  (B2).
- **El parseo de `library.json`.** La tolerancia a `{}` de Lua está bien
  resuelta en `libraryfile.go:40-49`; lo que falla es el `viewer` mal formado
  (M8).

---

## No reproducidos (hipótesis)

- **`stats.go:143` — `statistics: null` con `User` no nulo.** Si la API
  devolviera `{"data":{"User":{"name":"x","statistics":null}}}`,
  `FetchStats` devolvería ceros sin error (el `data.User == nil` de la línea
  143 no cubre el caso). Reproducido **sólo** con un servidor falso; contra la
  API real los perfiles privados devuelven `{"errors":[{"message":"Not Found."}],
  "data":{"User":null}}`, que el módulo maneja como error. Falta una cuenta
  real con lista privada y perfil público.
- **`client.go:85` — `Retry-After` en formato HTTP-date.** El código sólo
  parsea el formato entero (`strconv.Atoi`); una cabecera con fecha se ignora
  silenciosamente y se usa el backoff exponencial. No se comprobó contra
  AniList porque la API manda el formato entero.

## Sin revisar

- **`internal/plugins/anilist/cli/root_test.go`** (1.700 líneas): se leyeron los
  80 tests (nombres y aserciones de los que aplican) pero no línea por línea.
  En particular no se auditó el `capturar()`/`afirmarUnSoloJSON()` contra
  rutas no cubiertas.
- **El plugin de Noctalia** (`b3l0w-plugins`, repo externo): no se revisó de qué
  forma escribe `library.json`, `covers.json` y `token.json`. Es la fuente de
  M1, M8 y M9 y若干 de las formas que `libraryfile.go` tolera.
- **Cuentas con token real**: sin token no se pudo probar el camino autenticado
  (`mutate`, `MediaList` del usuario autenticado, `SaveMediaListEntry`).
- **El `README.md` de la raíz del repo** (`README.md:543-550`) y
  `ARCHITECTURE.md`: se miró la sección de `b3l0w anilist` de paso; no se
  auditó línea por línea contra este módulo.
- **Los otros tres plugins** (`docker`, `we`, `ytmusic`): fuera de alcance.

## Verificación

```console
$ go test ./internal/plugins/anilist/... -count=1
ok  github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist
ok  github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist/cli

$ go build ./...
(sin salida)
```

Los dos verdes: no se tocó código de producción ni tests. El árbol de trabajo
del módulo quedó como estaba (`bugs.md` nuevo, nada más).
