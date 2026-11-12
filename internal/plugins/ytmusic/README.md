# ytmusic — YouTube Music real para b3l0w

Backend embebido del plugin `b3l0w/ytmusic` (Noctalia). Habla con el innertube
de YouTube Music (cliente `WEB_REMIX`) y reproduce con `mpv`. Vive **fuera**
del service de Noctalia a propósito: parsear un JSON de 1 MB en el callback del
service revienta el presupuesto de 25 ms y el host deshabilita el plugin. El
service llama al binario y recibe JSON.

```
home [--continuation TOK]   charts [--country XX]   search <texto>
radio <videoId>             next|prev|queue|shuffle --session ID
play <videoId>              pause | resume | stop | seek <seg>   status
```

Paquete: `cli/root.go` (flags y salida), `player.go` (`mpv` detachado e IPC),
`parse.go` (parseo del JSON sin esquema), `radio.go` (la sesión), `client.go`
(el sobre de innertube), `model.go` (modelo y persistencia) y las pantallas:
`home.go`, `charts.go`, `thumbs.go`, `durations.go`, `search.go`.

## La radio y su cursor

La radio es una **cola persistida en disco**, no memoria: `next` corre en otro
proceso y a veces minutos después del `radio` que la creó. Bajo el `dataDir`
viven `sessions/<id>.json` (cola + cursor), `current.json` (el puntero a la
sesión activa), `player.json` (PID y metadata) y `mpv.sock` (el IPC).

`Session` tiene `Queue []Song` y `Cursor int` (0-based). El `Cursor` es **el
índice de la canción que está sonando**, no el de la próxima.

- **`next`** — `Cursor++`. Si `Cursor+1` pasa el final de la cola, pide la
  siguiente tanda del automix (`continuation`). Si la radio se agotó, **siembra
  otra** desde la última canción y sigue: rendirse deja el botón "siguiente"
  muerto a los 50 temas.
- **`prev`** — `Cursor--`. Error si ya está en la primera.
- **`shuffle`** — Fisher-Yates con `crypto/rand` sobre la cola **ya traída**,
  dejando la actual primero: un shuffle que cambia lo que está sonando es una
  patada, no un shuffle. Deja `Cursor = 0`.
- **`enqueue`** — agrega al final, ignorando repetidos.
- **`queue`** — devuelve la cola y el cursor tal como están.

`LoadSession` **rechaza un cursor fuera de rango** (`< 0` o `>= len(Queue)`)
con un error limpio. El cursor viene de un JSON en disco: si quedó fuera de la
cola (sesión editada a mano o guardado interrumpido a mitad de escritura),
indexar revienta con un **panic**. Se valida una sola vez, en la carga, y
cubre las cinco operaciones.

## Modelo de datos

`Song` lleva `VideoID`, `Title`, `Artist`, `Album`, `Duration`, `Seconds` (la
duración en segundos), `ThumbURL` (la URL remota), `Thumb` (la **ruta local**
de la portada) y `ChartRank` (el puesto en el ranking). `Feed` lleva `Source`,
`Title`, `Items`, `Continuation` y `FetchedAt`.

`Continuation` evita el bug original: sin él la lista muere en la primera
página. `Thumb` es una **ruta local** a propósito: el CLI baja la portada antes
de publicar el feed, porque `ui.image` no acepta URLs.

## Cliente HTTP y timeouts

| Timeout | Dónde | Por qué |
|---|---|---|
| 25 s | `http.Client` de innertube | Un browse de charts puede tardar |
| 12 s | descarga de portadas | Son 5 a la vez, no puede colgar la pantalla |
| 1,2 s | `net.DialTimeout` al socket de mpv | mpv puede no existir |
| 1,5 s | deadline de la conexión IPC | Un comando de mpv no puede colgar el panel |
| 45 s / 40 s | contexto del CLI | feeds 45 s, radio/next 40 s |

El cliente manda `X-YouTube-Client-Name: 67`, `X-YouTube-Client-Version`,
`Origin`, `Referer` y un `User-Agent` de Chrome. Si YouTube rechaza la versión
del cliente, la respuesta viene **vacía, no con un 4xx**: por eso `post` mira
`playabilityStatus` y lo traduce a un error legible ("YouTube rechazó el
pedido (LOGIN_REQUIRED): ..."). Sin eso el llamador ve "0 canciones".

Reintentos: **2 intentos**, con 400 ms de espera. Un 5xx o un fallo de red se
reintenta; un 4xx no. El cuerpo se lee con un límite de 32 MB y los errores se
recortan a 160 caracteres: el service muestra ese texto en el panel, no puede
ser un volcado de 1 MB.

## Formato de duraciones

Dos funciones, dos sentidos, y conviene no confundirlas:

- **`durationToSeconds("3:45")` → `225`** — parseo tolerante. Da `0` si
  el texto no es puramente numérico con `:` (`"abc"`, `"3:4x"`, `":"`,
  `"3:45:"`). Acepta `"1:02:03"` → `3723`.
- **`clockOf(225)` → `"3:45"`** — formato para el panel. `""` para 0 o
  negativos. Con horas: `"1:02:03"`.

Las estanterías de home ("Quick picks") traen título, artista y portada pero
**no duración**. `EnrichDurations` la pide al endpoint `player` (que sí la
trae en `videoDetails.lengthSeconds`), en paralelo, hasta 6 a la vez, y corta
en `limit` para no hacer 50 llamadas por pantalla. Si el player falla, la
canción queda sin duración pero la lista sigue en pie.

## Decisiones

- **Sin structs fijos para innertube.** La forma cambia entre shelves y un
  struct roto se rompe en silencio (campos en cero). Se camina el árbol y se
  extraen los renderers conocidos; `collectRenderers` ordena las claves al
  recorrer un map, porque el orden de un map de Go no es estable.
- **`bestThumb` descarta las portadas de más de 400 px.** 544 px de carátula
  para una fila de 46 px es ancho de banda tirado.
- **Charts prefiere "Top 100" > "Top 50" > "Top" > "Trending"**, y suma +10 a
  las de "songs" y +5 a las "global". Las estanterías de videos vienen
  primero: quedarse con la primera tarjeta devuelve el chart equivocado.
- **Un chart vacío es un error, no un chart.** Igual que un feed sin canciones:
  exit 0 con lista vacía es el bug que hay que evitar.
- **Home tiene fallback a la primera playlist.** Una página de inicio anónima
  trae pocas estanterías de canciones y muchas de playlists; si no hay ni una
  canción, se abre la primera tarjeta con `browseEndpoint`.
- **`search` manda el filtro de canciones** (`EgWKAQIIAWoKEAkQBRAKEAMQBA%3D%3D`):
  sin él vienen "todos los resultados" y la mitad son videos de gente
  hablando.
- **El player es un proceso detachado (`setsid`)**, no un runAsync del service:
  si el service se recarga, la música no se corta. Se mata por **grupo**
  (`-pid`), porque el grupo es `yt-dlp | mpv` y matar sólo a mpv deja a
  yt-dlp escribiendo a un pipe sin lector.
- **La posición sale de mpv, no de un reloj nuestro**: una pausa o un buffer
  lento no mienten. Si mpv todavía no tiene `time-pos` se informa `starting`
  y la duración del catálogo sirve de referencia (antes daba "0:00 / 0:00").
- **EOF es la señal de "pasa a la siguiente"** y llega **antes** de que el
  proceso muera. Es lo que rompía el autoplay.
- **El estado se escribe de forma atómica** (temporal + `rename`): un `next`
  puede leer la sesión mientras otra llamada la reescribe. `DataDir` usa el
  flag explícito o el state dir de Noctalia, con permisos `0700`: ahí vive el
  socket de mpv.

## Tests

```
go test ./internal/plugins/ytmusic/...
```

**Ninguna prueba sale a la red.** Las URLs de innertube son constantes del
paquete, así que lo que se intercepta es el **transporte HTTP**: un
`httptest.Server` local con un `RoundTripper` que reescribe el host. El
cliente real (headers, reintentos, límite de 32 MB, `playability`) se ejercita
tal cual. Para mpv hay un **socket unix falso** que habla el protocolo IPC.
Cobertura: **83,7 %**.

Los archivos: `fakeserver_test.go` es la infra (servidor innertube falso y
fixtures); `durations_test.go` las duraciones; `parse_edge_test.go` los
helpers de JSON y los feeders; `radio_test.go` la máquina de estados y el
cursor inválido; `charts_test.go` el ranking; `home_search_test.go` el
fallback de home y la búsqueda; `client_test.go` los headers, los 4xx/5xx y
el `playabilityStatus`; `thumbs_test.go` la descarga y la caché;
`model_test.go` el `DataDir` y la persistencia; `player_ipc_test.go` el IPC
de mpv; `player_stop_test.go` el `Stop`; `cli/root_test.go` las flags.

**Lo que NO se testea (y por qué).** `Play` y `resolveMetadata` lanzan `yt-dlp`
y `mpv` reales contra la red; `Play` sí se cubre en parte por `Status`, `Stop`
y los tests de IPC. Los fixtures son recortes de respuestas reales: si YouTube
cambia la forma, estos tests fallan antes que el panel muestre una lista
vacía, pero no detectan un cambio fuera del recorte. Y `b3l0w plugin-exec` no
aplica: el backend es embebido.

**Lo que hay que verificar contra la API real.** Que `WEB_REMIX` con
`ClientVersion = "1.20250101.00.00"` siga siendo aceptado (si YouTube la
rechaza, la respuesta viene vacía y lo detecta `playabilityError`). Los
valores de `searchSongsParams` y del prefijo `RDAMVM` **(verificar)**: son
estables, pero innertube no publica contrato. El `params` por país de charts
es **(verificar)**: hoy el país se manda en el `GL` del cliente. Los shards
(`FEmusic_home`, `FEmusic_charts`, `VL*`) son internos de YouTube y pueden
cambiar, y el `plugin_api` del registry (43) tiene que ser el que espera el
plugin.

## Dependencias

`yt-dlp` y `mpv` (ambos en `internal/systemdef/system.json`). `mpv` además
necesita `--audio-client-name=b3l0w-ytmusic` para que YouTube no corte el
stream.