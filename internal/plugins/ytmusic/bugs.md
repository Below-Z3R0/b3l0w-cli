## Arreglados

Ola del 2026-11-19: los 5 hallazgos CRÍTICOS y ALTOS. Cada arreglo tiene un test
que se verificó **revirtiéndolo**: con el código viejo falla con el mensaje del
bug, no con un error de compilación. Los tests están en `fixes_test.go`.

| ID | Arreglo | Test |
|---|---|---|
| Y1 | `player.json` guarda `startTime` (campo 22 de `/proc/<pid>/stat`); `Stop` sólo manda señal si coincide, y `Status` sólo dice "reproduciendo" si coincide | `TestStopNoMataUnPIDQueNoEsNuestro` |
| Y2 | El socket se deriva siempre de `dataDir`; el campo `socket` del JSON no se usa para borrar ni para conectar | `TestNoSeBorraElArchivoQueDiceElSocket` |
| Y3 | `findRenderer` recorre las claves ordenadas, igual que `collectRenderers` | `TestFindRendererEsDeterminista` |
| Y4 | `Radio` se parte en `buildRadio` (no escribe) + guardado; el rebase de `next` usa `buildRadio` y escribe una sola vez, ya mergeado | `TestElRebaseNoTruncaLaColaEnDisco` |
| Y5 | `writeJSON` crea el temporal con `os.CreateTemp`: nombre único por escritor | `TestWriteJSONConcurrenteNoPierdeEscrituras` |

**Y1 — `player.go`.** `processStartTime` lee `/proc/<pid>/stat` cortando después
del último `)` (el campo `comm` puede traer espacios y paréntesis). `ours()`
exige PID vivo **y** starttime igual. Un `player.json` anterior a este cambio no
tiene `startTime`: no se puede validar, así que `stop` no mata (fail-closed) y se
limita a descartar el estado. Es el costo de no volver a matar un ajeno.

**Y2 — `player.go`.** `Pause`, `Resume`, `Seek`, `Status` y `Stop` usan
`socketPath(dataDir)` en lugar de `state.Socket`. El campo sigue en el JSON como
dato informativo, pero ya no decide sobre el filesystem.

**Y3 — `parse.go`.** El orden de un map de Go es aleatorio; con más de un
candidato, `continuationOf` devolvía tokens distintos para el mismo payload. Se
ordenan las claves antes de bajar.

**Y4 — `radio.go`.** `Radio` = `buildRadio` + `SaveSession`. El camino de rebase
llama a `buildRadio`, hace el merge en memoria y guarda una sola vez. Si ese
guardado falla, la sesión en disco queda como estaba en vez de quedar con la
cola corta.

**Y5 — `model.go`.** Un `.tmp` fijo hacía que dos escritores del mismo archivo
se pisaran: el primero renombraba y al segundo le daba ENOENT. Ahora cada uno
tiene su temporal y el rename final sigue siendo atómico.

Ola del 2026-11-20: los 15 hallazgos MEDIO y BAJO (Y6–Y20). Cada arreglo
tiene un test que se verificó **revirtiéndolo**: con el código viejo falla con
el mensaje del bug, no con un error de compilación.

| ID | Arreglo | Test |
|---|---|---|
| Y6 | `Play` recibe un contexto y `resolveMetadata` usa `exec.CommandContext` con `WaitDelay`: un `yt-dlp` colgado se corta en vez de colgar el panel | `TestPlayCortaElYTDLPColgado` |
| Y7 | Si el estado no se puede guardar, `Play` mata el grupo del proceso que arrancó (`killStarted`) | `TestPlayMataElProcesoSiNoGuardaElEstado` |
| Y8 | `EnsureDir` hace `chmod 0700` después de `MkdirAll`: un directorio que ya existía no queda en 0755 | `TestEnsureDirAjustaLosPermisosDeUnDirectorioExistente` |
| Y9 | `Thumbs` sanea el `videoId` (`safeThumbName`): un id con `/` o `..` se descarta y no se escribe fuera del directorio | `TestThumbsNoEscribeFueraDelDirectorio` |
| Y10 | `sessionPath` usa `filepath.Base` y `LoadSession`/`SaveSession` validan el id: la ruta nunca sale de `sessions/` | `TestSessionPathNoSaleDeSessions` |
| Y11 | `bestThumb` tiene red de seguridad: si todas las portadas son de más de 400 px, se queda con la más chica | `TestBestThumbUsaUnaPortadaGrandeSiNoHayChica` |
| Y12 | `durationToSeconds` tiene techo (`maxDurationSeconds`, un año) y acumula en `int64`: no hay overflow ni segundos negativos | `TestDurationToSecondsNoDesborda` |
| Y13 | `Status` distingue "no hay player.json" de "el estado está roto": el corrupto es error y se limpia el archivo | `TestStatusConEstadoCorruptoEsErrorYLimpiaElArchivo` |
| Y14 | `limit <= 0` significa "ninguna" en `Thumbs` y en `EnrichDurations`; el CLI no deja pasar un `--limit` negativo | `TestThumbsConLimiteNoPositivoNoBajaNinguna` |
| Y15 | El reloj del CLI usa `ytmusic.Clock`: una hora se muestra como `1:00:00`, no como `60:00` | `TestElRelojDelCLILlevaHoras` |
| Y16 | `seek` parsea con `strconv.Atoi` y falla si el argumento no es un número, en vez de saltar a 0 | `TestSeekConUnArgumentoQueNoEsNumeroFalla` |
| Y17 | Se borró la rama muerta `if song.Title == "" { song.Title = "" }` del comando `play` | `TestElComandoPlayNoTieneCodigoMuerto` |
| Y18 | La espera de 400 ms cae **entre** intentos: después del último no se espera (`waitBeforeRetry`) | `TestElReintentoNoEsperaDespuesDelUltimoIntento` |
| Y19 | El README declara la cobertura real, por paquete, y un test la mide en un subproceso y falla si deja de coincidir | `TestElREADMEDeclaraLaCoberturaReal` |
| Y20 | Lo que es de Unix (`setsid`, grupos de procesos, señales) vive en `player_unix.go` con build tag; `player_other.go` hace que el paquete compile en cualquier plataforma | `TestElPlayerCompilaFueraDeUnix` |

**Y6 — `player.go`, `cli/root.go`.** `Play` ahora recibe un
`context.Context`. La metadata se pide con `exec.CommandContext` y un
`WaitDelay` de 2 s: si el `yt-dlp` deja hijos con los pipes abiertos, el
`Output()` se quedaba esperando el EOF de esos pipes. El CLI le pasa un
contexto de 45 s. El `playTimeout` (45 s) es el techo interno de la fase de
metadata, así que tampoco depende de que el llamador pase contexto.

**Y7 — `player.go`.** Si `writeJSON` del `player.json` falla, el reproductor
ya está corriendo pero sin estado: `killStarted` manda SIGTERM al grupo y,
al final, SIGKILL (el `sh` puede forkear a `yt-dlp` y `mpv` después del
SIGTERM). El PID es del proceso que este código acaba de lanzar, así que no
hace falta la validación de `startTime` que hace `Stop`.

**Y8 — `model.go`.** `EnsureDir` hace `chmod 0700` después de `MkdirAll`.
`MkdirAll` no toca los permisos de un directorio que ya existía: un `dataDir`
de una corrida anterior en 0755 dejaba el socket de mpv legible por otros.

**Y9 — `thumbs.go`.** `safeThumbName` acepta sólo `[A-Za-z0-9_-]` y hasta 64
caracteres. Un `videoId` con `/` o `..` (viene de la red, sin validar) se
descarta: la canción queda sin portada en vez de escribir donde le digan.

**Y10 — `radio.go`.** `sessionPath` usa `filepath.Base` (la ruta no puede
escapar del directorio) y `LoadSession`/`SaveSession` validan el id con
`sessionIDValido` antes de tocar el disco. Un id con `../` es un error
explícito, no una lectura de cualquier JSON del disco.

**Y11 — `parse.go`.** `pickThumb` separa la mejor candidata (la más grande de
hasta 400 px) del respaldo (la más chica de las que pasan). Si no hay ninguna
candidata chica, devuelve el respaldo: la fila deja de quedar sin imagen. La
preferencia por la portada chica no cambió.

**Y12 — `model.go`.** `durationToSeconds` acumula en `int64` y corta con
`maxDurationSeconds` (un año). Una duración absurda da `0`, como las demás
entradas raras: nunca un segundo negativo ni un número que desborda.

**Y13 — `player.go`.** `Status` separa los dos casos: si el `player.json` no
existe, "parado" con `exit 0`; si existe pero no se puede leer, es un error
("el estado del player está roto") y el archivo roto se limpia, para que el
siguiente `status` no vuelva a leerlo. `Stop` también limpia el estado
corrupto.

**Y14 — `thumbs.go`, `durations.go`, `cli/root.go`.** `limit <= 0` significa
"ninguna" en las dos funciones. En el CLI, `flags.number` cae al default con
un valor no positivo: `--limit -1` ya no significa "todas" en un comando y
"ninguna" en el otro.

**Y15 — `durations.go`, `cli/root.go`.** `clockOf` ahora delega en `Clock`,
que se exporta. El CLI usa `ytmusic.Clock`: los dos relojes son el mismo y una
hora se muestra como `1:00:00`.

**Y16 — `cli/root.go`.** `seek` parsea con `strconv.Atoi` y falla con un
error legible si el argumento no es un número. "12abc" también falla (antes
se aceptaba como 12).

**Y17 — `cli/root.go`.** La rama `if song.Title == "" { song.Title = "" }`
era un esqueleto: la decisión ya está implícita en `Play`, que resuelve la
metadata con yt-dlp cuando el título viene vacío. El arreglo no cambia el
comportamiento, así que su test mira el código: la rama muerta no puede
volver.

**Y18 — `client.go`.** `waitBeforeRetry` sólo espera si hay otro intento
después. Con un 5xx permanente el comando tardaba 800 ms de más.

**Y19 — `README.md`, `coverage_test.go`.** El README declara la cobertura
real por paquete. `TestElREADMEDeclaraLaCoberturaReal` mide la cobertura de
verdad en un subproceso (`go test -cover`) y falla si el README deja de
coincidir con ella, así que el número no puede volver a mentir.

**Y20 — `player_unix.go`, `player_other.go`.** `setsid`, `syscall.Kill` y el
`EPERM` de `alive` no existen fuera de Unix: viven en `player_unix.go` con
`//go:build unix`. `player_other.go` (`//go:build !unix`) da los stubs que
dicen que el reproductor sólo funciona en Unix. El resto del paquete compila
en cualquier plataforma.

### Verificación de los arreglos

```console
$ go build ./... && go vet ./internal/plugins/ytmusic/... && go test ./internal/plugins/ytmusic/... -count=2
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic/cli
```

Cada test se verificó **revirtiendo su arreglo**: el test falla con el
mensaje del bug, no con un error de compilación. Los 20 quedaron
verificados (Y1–Y5 en la ola del 2026-11-19, Y6–Y20 en la del 2026-11-20).

Los fixtures de `player_ipc_test.go` y `player_stop_test.go` se actualizaron al
contrato nuevo: el socket vive en `<dataDir>/mpv.sock` y el `playerFile` lleva el
starttime real del proceso de prueba. Sin eso, los tests viejos probaban un
contrato que ya no existe.

**Sigue abierto:** nada. Los 20 hallazgos de este reporte están cerrados.



---

## Y1 — `Stop` mata un grupo de procesos ajeno (CRÍTICO)

`Stop` lee el PID de `player.json` y mata ese grupo de procesos sin comprobar
que siga siendo nuestro:

```go
// player.go:214-230
if state.PID > 0 {
	if err := syscall.Kill(-state.PID, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(state.PID, syscall.SIGTERM)
	}
	...
	if alive(state.PID) {
		if err := syscall.Kill(-state.PID, syscall.SIGKILL); err != nil {
```

El PID es el de `/bin/sh` que hace de grupo (el `setsid` de `player.go:127`).
Cuando ese proceso muere, el kernel **reutiliza** su PID para otro proceso
del usuario. `player.json` sigue en disco con el PID viejo: el siguiente
`play` llama a `Stop` (`player.go:95`) y mata al proceso que recicló ese
número — con todo su grupo.

Reproduction (probe `TestProbe2StopMataProcesoAjeno`):

```go
cmd := exec.Command("sleep", "300")
cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
cmd.Start()
state := playerFile{PID: cmd.Process.Pid, Socket: filepath.Join(dir, "mpv.sock")}
writeJSON(playerPath(dir), state)
Stop(dir)
```

Salida:

```
P2 proceso ajeno vivo, pid=1969764, pgid=1969764
P2 el proceso ajeno MURIO: wait=signal: terminated
```

El proceso del probe (`sleep 300`) no tiene nada que ver con el reproductor y
murió igual.

**Agravante (hipótesis, no ejecutada por seguridad).** Si el PID guardado es
`1`, `syscall.Kill(-1, SIGTERM)` señaliza **todos** los procesos que el
usuario puede señalar (`man 2 kill`: "If pid is -1, all processes except
init and process 1 itself are signaled"). No se ejecutó porque mataría la
sesión de pruebas. Es alcanzable con un `player.json` editado a mano — el
propio README acepta ese escenario como amenaza para el cursor de la sesión
(`README.md:41-45`). Lo que faltó: correrlo en un namespace de PID propio
(`unshare --pid --fork`) para no tocar la máquina.

## Y2 — `Status` y `Stop` borran el archivo que dice `player.json` (ALTO)

El campo `socket` de `player.json` se usa tal cual como ruta para borrar:

```go
// player.go:232-233 (Stop) y player.go:256-257 (Status)
_ = os.Remove(state.Socket)
_ = os.Remove(playerPath(dataDir))
```

`state.Socket` viene del JSON en disco, no del código. Con un `player.json`
editado (o corrupto), `status` borra cualquier archivo que el usuario pueda
escribir. `Status` no necesita ni que el proceso esté vivo: basta con un PID
muerto.

Reproduction (probe `TestProbe3StatusBorraArchivoAjeno`):

```go
victim := filepath.Join(t.TempDir(), "importante.txt")
os.WriteFile(victim, []byte("no me borres"), 0o600)
state := playerFile{PID: 999999, VideoID: "x", Socket: victim}
writeJSON(playerPath(dir), state)
Status(dir)
```

Salida:

```
P3 Status borró /tmp/.../importante.txt (ruta sacada de player.json)
```

El mismo `os.Remove` se repite en `Stop` (`player.go:232`).

## Y3 — `findRenderer` depende del orden aleatorio del map (ALTO)

El README dice, con razón, que `collectRenderers` ordena las claves "porque el
orden de un map de Go no es estable" (`README.md:98-100`). Pero `findRenderer`
no lo hace:

```go
// parse.go:81-85
for _, value := range typed {
	if found := findRenderer(value, key); found != nil {
		return found
	}
}
```

`findRenderer` es la base de `continuationOf` (`parse.go:178`), `bestThumb`
(`parse.go:128,131`), `songFromResponsive` (`parse.go:212`), `chartsTitle`
(`charts.go:96,101`) y `shelfTitles` (`home.go:85`). Con dos subárboles que
tienen la misma clave, el winner depende del orden de iteración del map: es
aleatorio en Go y **cambia entre llamadas**.

El caso que duele es el de la paginación: una página de home con dos
estanterías, cada una con su continuación, devuelve un token distinto en cada
llamada. El panel pagina a una estantería y a otra.

Reproduction (probe `TestProbe1ContinuationOfNoDeterminista`):

```go
payload := map[string]any{
	"a": map[string]any{"nextContinuationData": map[string]any{"continuation": "TOKEN_A"}},
	"b": map[string]any{"nextContinuationData": map[string]any{"continuation": "TOKEN_B"}},
}
for i := 0; i < 500; i++ { seen[continuationOf(payload)]++ }
```

Salida:

```
P1 continuationOf sobre el MISMO payload -> map[TOKEN_A:441 TOKEN_B:59]
P1b findRenderer el mismo payload -> map[VID_A:437 VID_B:63]
```

El mismo payload, dos respuestas distintas. En un test esto es un flake
intermitente; en producción, una página que "salta" de estantería.

## Y4 — el rebase de `next` deja la cola truncada en disco (ALTO)

`Next` siembra una radio nueva cuando la cola se agotó, y para eso llama a
`Radio`:

```go
// radio.go:140
rebased, rebaseErr := Radio(ctx, c, dataDir, last.VideoID, session.ID)
```

`Radio` guarda la sesión por su cuenta (`radio.go:58-60` → `SaveSession`), y
`SaveSession` **escribe primero el archivo de la sesión y después
`current.json`**:

```go
// radio.go:69-72
if err := writeJSON(sessionPath(dataDir, session.ID), session); err != nil { ... }
return writeJSON(filepath.Join(dataDir, "current.json"), ...)
```

O sea: el `SaveSession` interno del rebase deja en disco la cola **rebased**
(la cola corta de la radio nueva, con cursor 0) **antes** de que `Next` arme
la cola final y la vuelva a guardar (`radio.go:163`). Si ese segundo guardado
falla —o el proceso muere entre los dos— la cola original no está más en
disco.

Reproduction (probe `TestProbe16NextRebaseTruncaLaCola`): cola de 3
canciones (A B C) con cursor en la última, `current.json` como directorio (no
se puede escribir) y la innertube falsa respondiendo con una sola canción:

```
P16 Next -> err=la radio se agotó y no se pudo sembrar otra desde c: rename .../current.json.tmp .../current.json: file exists
P16 sesión en disco: queue=1 cursor=0 titles=[A]
P16 (la original tenía 3 canciones: A B C)
```

La cola pasó de 3 canciones a 1. El disparador real de la segunda escritura
que falla es Y5 (dos `next` a la vez), que está en el mismo archivo.

## Y5 — `writeJSON` no aguanta dos escritores (ALTO)

El "atomic write" usa un temporal de **nombre fijo**:

```go
// model.go:147-151
tmp := path + ".tmp"
if err := os.WriteFile(tmp, raw, 0o600); err != nil { return err }
return os.Rename(tmp, path)
```

Dos procesos que guardan la misma sesión comparten `s1.json.tmp`: uno lo
renombra y el otro recibe `ENOENT` al renombrar. El primero que escribió
pierde su actualización y el segundo ve un error que no es del mundo.

Eso es exactamente el caso que el README promete cubrir (`README.md:123-124`:
"El estado se escribe de forma atómica (temporal + rename): un `next` puede
leer la sesión mientras otra llamada la reescribe"). Contra un **lector**
sí aguanta; contra otro **escritor**, no.

Reproduction (probe `TestProbe4bWriteJSONConcurrenteErrores`): dos goroutines
escribiendo el mismo path, 300 vueltas:

```
P4b errores de writeJSON entre dos escritores: 296/600; contenido ni A ni B: 0/300
```

El contenido nunca queda corrupto (el `rename` es atómico para el lector),
pero **casi la mitad de las escrituras falla** y una de las dos
actualizaciones se pierde.

Y a nivel de comando, dos `next` en paralelo (probe `TestProbe15NextConcurrente`):

```
P15 next#1 err=sesiones: rename .../sessions/s1.json.tmp .../sessions/s1.json: no such file or directory cursor=0
P15 next#2 err=<nil> cursor=1
P15 sesión en disco: cursor=1 queue=2 err=<nil>
```

El primer `next` vuelve al panel con `sesiones: rename ...: no such file or
directory` y su avance del cursor no queda guardado.

## Y6 — `Play` no tiene timeout (MEDIO)

Ni `Play` ni `resolveMetadata` reciben contexto ni tienen deadline:

```go
// player.go:51
out, err := exec.Command("yt-dlp", args...).Output()
```

`cli/root.go:244` y `cli/root.go:305` llaman a `Play` sin contexto (el
`ctx` de 40 s de `runStep` no se le pasa), así que nada acota la espera. Un
`yt-dlp` que se queda esperando la red deja el comando colgado indefinidamente
—el panel nunca recibe respuesta.

Reproduction (probe `TestProbe17PlaySinTimeout`): un `yt-dlp` falso que
duerme 6 s en el `PATH`:

```
P17 Play tardó 6.244s (err=<nil>)
```

El tiempo de `Play` es el del `yt-dlp` falso, sin techo. El README promete
"45 s / 40 s contexto del CLI" (`README.md:66`) y eso no alcanza a `play`.

## Y7 — `Play` deja el reproductor huérfano (MEDIO)

`Play` arranca el proceso **antes** de guardar el estado:

```go
// player.go:131-150
if err := cmd.Start(); err != nil { ... }
...
if err := writeJSON(playerPath(dataDir), state); err != nil {
	return PlayerState{}, fmt.Errorf("play: %w", err)
}
```

Si el guardado falla, el comando devuelve error pero `yt-dlp | mpv` sigue
corriendo: como no hay `player.json`, `stop` no lo encuentra y la música
sigue sonando sin control.

Reproduction (probe `TestProbe12PlayDejaProcesoHuerfano`): `player.json` como
directorio (hace fallar el `rename`) y `yt-dlp`/`mpv` falsos en el `PATH`:

```
P12 Play -> err=play: rename .../player.json.tmp .../player.json: file exists state={PID:0 ...}
P12 procesos del probe vivos después del error: 1
```

Queda vivo el `sh` del pipeline (el grupo que creó el `setsid`) después de que
`Play` ya había devuelto el error: sin `player.json` no hay forma de
referenciarlo, así que `stop` no lo mata.

## Y8 — `EnsureDir` no ajusta los permisos de un directorio existente (MEDIO)

```go
// model.go:90-95
func EnsureDir(dir string) error {
	if dir == "" { return fmt.Errorf("directorio de datos vacío") }
	return os.MkdirAll(dir, 0o700)
}
```

`MkdirAll` no cambia los permisos de un directorio que ya existe. El README
dice que el `dataDir` va con permisos `0700` "ahí vive el socket de mpv"
(`README.md:124-126`): si el directorio quedó en `0755` de una corrida
anterior, `EnsureDir` lo deja como está y el socket queda legible por otros.

Reproduction (probe `TestProbe6EnsureDirNoAjustaPermisos`):

```
P6 permisos finales: -rwxr-xr-x (pedidos 0700)
P6 REPRODUCIDO: sigue 0755
```

## Y9 — `Thumbs` escribe fuera del directorio con un `videoId` de la red (MEDIO)

```go
// thumbs.go:32 y thumbs.go:57
path := filepath.Join(dir, songs[index].VideoID+".jpg")
```

El `videoId` sale del JSON de innertube (`parse.go:197` y `parse.go:213`) y no
se valida: si trae `../`, `filepath.Join` lo limpia y la escritura sale del
directorio de portadas. Con `--thumbs` apuntando a un directorio del
proyecto, el plugin puede escribir un `.jpg` donde le digan.

Reproduction (probe `TestProbe5bThumbsPathTraversal`), con
`VideoID: "../../escaped"`:

```
P5b Thumb calculado por el código = "/tmp/.../escaped.jpg"
P5b REPRODUCIDO: se escribió fuera de /tmp/.../001/thumbs -> /tmp/.../escaped.jpg
```

## Y10 — `sessionPath` sale de `sessions/` con el valor de `--session` (MEDIO)

```go
// radio.go:20-22
func sessionPath(dataDir, id string) string {
	return filepath.Join(sessionsDir(dataDir), id+".json")
}
```

El `id` viene del flag `--session` (`cli/root.go:217`, `cli/root.go:258`) o
de `current.json`. Con `../` se lee o se escribe fuera de `sessions/`.

Reproduction (probe `TestProbe9SessionPathTraversal`):

```
P9 sessionPath(dataDir, "../../escaped") = "/tmp/.../escaped.json"
P9 REPRODUCIDO: sale de sessions/
```

Mismo camino que Y9: el `videoId`/el `id` que llegan de afuera entran al
sistema de archivos sin sanear.

## Y11 — `bestThumb` se queda sin portada (MEDIO)

`pickThumb` descarta todo lo que pase de 400 px y **no tiene fallback**:

```go
// parse.go:149-166
if width <= 400 && width >= chosenWidth {
	chosen, chosenWidth = url, width
}
```

Si la página sólo trae la versión de 544 px, el resultado es la cadena vacía
y la fila queda sin imagen. El README dice que las portadas grandes se
descartan por ancho de banda (`README.md:101-102`), lo que no dice es que no
haya red de seguridad.

Reproduction (probe `TestProbe8BestThumbSinGrande`):

```
P8 bestThumb con una sola portada de 544px -> ""
```

## Y12 — `durationToSeconds` desborda (MEDIO)

```go
// model.go:125-135
n = n*10 + int(r-'0')
...
total = total*60 + n
```

No hay control de overflow: un `lengthText` enorme (viene de la red) produce
segundos negativos o absurdos, que después se imprimen en el panel.

Reproduction (probe `TestProbe7DurationOverflow`):

```
P7 durationToSeconds("99999999999999999999") = 7766279631452241919
P7 durationToSeconds("9223372036854775808") = -9223372036854775808
P7 durationToSeconds("1:99999999999999999999") = 7766279631452241979
```

Un segundo negativo hace que `clockOf` (`durations.go:76`) devuelva `""` y la
fila quede sin duración.

## Y13 — un `player.json` corrupto deja el `status` mudo para siempre (MEDIO)

```go
// player.go:239-243
state, err := loadPlayer(dataDir)
if err != nil {
	return PlayerState{Playing: false}, nil
}
```

Cualquier error de lectura —no sólo "no hay player"— se traduce en "parado"
con `exit 0`. Y como el `os.Remove` de la limpieza está **después** de ese
`return` (`player.go:256-257`), el archivo roto no se limpia nunca: cada
`status` posterior vuelve a leer el mismo JSON inválido y vuelve a decir
"parado". El panel no puede distinguir "no suena nada" de "el estado está
roto".

Reproduction (probe `TestProbe19StatusConEstadoCorrupto`):

```
P19 Status con player.json roto -> state={... Playing:false ...} err=<nil>
P19 el archivo roto sigue en disco: true
```

## Y14 — `limit <= 0` significa dos cosas distintas (BAJO)

```go
// thumbs.go:26
if limit <= 0 || limit > len(songs) { limit = len(songs) }   // <= 0 => TODAS
```

```go
// durations.go:20
if len(songs) == 0 || limit <= 0 { return songs }             // <= 0 => NINGUNA
```

`cli/root.go:91-102` deja pasar un valor negativo (`--limit -1`): el fallback
solo corre cuando el número parseado es 0.

Reproduction (probe `TestProbe10LimitesDistintos`):

```
P10 Thumbs(limit=-1) bajó 5 de 5 portadas
```

Con `--limit -1` se bajan todas las portadas de la pantalla y no se pide
ninguna duración.

## Y15 — el reloj del CLI no lleva horas (BAJO)

```go
// cli/root.go:361-366
return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
```

El paquete tiene `clockOf` (`durations.go:75-85`) que sí hace
`"1:02:03"`, pero el CLI usa su propio reloj y no puede usar el otro porque
no está exportado. Una pista de una hora se muestra como `62:03`.

Reproduction (probe `TestProbe13ClockCLI`):

```
P13 clock(3723) del CLI = "62:03"  (clockOf(3723) = "1:02:03")
```

## Y16 — `seek abc` salta a 0 en vez de fallar (BAJO)

```go
// cli/root.go:329-330
if len(f.pos) > 0 {
	fmt.Sscanf(f.pos[0], "%d", &seconds)   // el error se ignora
}
```

`Sscanf` devuelve el error y deja `seconds` en 0; el código no lo mira y
manda `seek 0` a mpv: la pista vuelve al principio en vez de decir que el
argumento no es un número.

Reproduction:

```
$ cd /tmp/sscanf && go run .
Sscanf("abc") -> n=0 err=expected integer seconds=0
Sscanf("12abc") -> n=1 err=<nil> seconds=12
```

O sea: `"abc"` → 0 silencioso, y `"12abc"` → 12 (aceptado sin quejarse).

## Y17 — código muerto en el comando `play` (BAJO)

```go
// cli/root.go:302-304
if song.Title == "" {
	song.Title = ""
}
```

La rama no hace nada: el comentario que la acompaña (`cli/root.go:299-301`)
explica que el título **no** debe rellenarse con el `videoId`, y la decisión
ya está implícita en `Play` (`player.go:102`, que llama a `resolveMetadata`
cuando el título viene vacío). Quedó el esqueleto de una versión anterior.

## Y18 — el reintento espera 400 ms de más (BAJO)

```go
// client.go:87-104
for attempt := 0; attempt < 2; attempt++ {
	...
	if err != nil {
		lastErr = ...
		time.Sleep(400 * time.Millisecond)
		continue
	}
```

La espera va **dentro** del `for`, así que también ocurre después del
último intento, cuando el resultado ya no se usa. Con un 5xx permanente el
comando tarda 800 ms de más en la red.

Reproduction (probe `TestProbe21SleepDespuesDelUltimoIntento`): un servidor
que siempre responde 500:

```
P21 un 5xx siempre: tardó 800ms err=innertube browse: HTTP 500: sin cuerpo
```

Dos intentos, dos esperas: la segunda sobra.

## Y19 — la cobertura del README no es la real (BAJO)

`README.md:139` decía "Cobertura: **83,7 %**". La medida de hoy:

```
$ go test ./internal/plugins/ytmusic/... -count=1 -cover
ok  	.../internal/plugins/ytmusic	1.706s	coverage: 85.0% of statements
ok  	.../internal/plugins/ytmusic/cli	0.003s	coverage: 32.5% of statements
```

El 83,7 % no corresponde ni al paquete (85,0 %) ni a `cli/` (32,5 %). Si la
cifra era del paquete, hay que actualizarla; si era de las dos juntas, hay
que aclarar de qué se mide.

**Arreglado.** El README declara la cobertura real, por paquete, y un test
(`TestElREADMEDeclaraLaCoberturaReal`) la mide en un subproceso y falla si
deja de coincidir. La medida después de esta ola de arreglos:

```
$ go test ./internal/plugins/ytmusic/... -count=1 -cover
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic	15.119s	coverage: 87.9% of statements
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic/cli	0.003s	coverage: 38.4% of statements
```

Los números del README son estos, no los de arriba (que eran los de la
auditoría).

## Y20 — el paquete no compila fuera de Unix (BAJO)

`player.go` usa `syscall.Kill` y `SysProcAttr.Setsid`, que no existen fuera
de Unix, y no hay build tag que lo marque como Linux-only:

```
$ GOOS=windows GOARCH=amd64 go build ./internal/plugins/ytmusic/...
internal/plugins/ytmusic/player.go:127:41: unknown field Setsid in struct literal of type syscall.SysProcAttr
internal/plugins/ytmusic/player.go:216:21: undefined: syscall.Kill
internal/plugins/ytmusic/player.go:217:16: undefined: syscall.Kill
internal/plugins/ytmusic/player.go:227:22: undefined: syscall.Kill
internal/plugins/ytmusic/player.go:228:17: undefined: syscall.Kill
internal/plugins/ytmusic/player.go:309:17: undefined: syscall.Kill
```

b3l0w es una CLI de CachyOS, así que el objetivo es Linux; pero un
`go build ./...` de otra plataforma revienta sin explanation. Con un
`//go:build linux` el error aparece en el lugar correcto.

---

## Afirmaciones del README: qué la sostiene y qué no

Cada afirmación del README se abrió en el código. Las que no se sostienen
están arriba; las que sí, acá:

| Afirmación | Dónde la sostiene | Veredicto |
|---|---|---|
| Timeout de 25 s del cliente de innertube | `client.go:40` | se sostiene |
| 12 s para la descarga de portadas | `thumbs.go:40` | se sostiene |
| `net.DialTimeout` de 1,2 s al socket de mpv | `player.go:316` | se sostiene |
| Deadline de 1,5 s en la conexión IPC | `player.go:321` | se sostiene, pero el error del `SetDeadline` se ignora (`player.go:321`) |
| Contexto de 45 s para feeds y 40 s para radio/next | `cli/root.go:156,199,218` | se sostiene; `play` también tiene contexto (45 s) desde Y6 |
| Headers `X-YouTube-Client-Name: 67`, versión, Origin, Referer y User-Agent | `client.go:92-98` | se sostiene |
| `post` mira `playabilityStatus` y lo traduce a error legible | `client.go:129-133`, `client.go:142-156` | se sostiene |
| 2 intentos con 400 ms de espera entre ellos; 5xx y red se reintentan, 4xx no | `client.go:87-119` | se sostiene (Y18 lo arregló: después del último intento no espera) |
| Cuerpo limitado a 32 MB | `client.go:106` | se sostiene |
| Errores recortados a 160 caracteres | `client.go:166-175` | se sostiene |
| `durationToSeconds("3:45")` → 225, tolera `"1:02:03"`, da 0 en las entradas raras | `model.go:115-138` | se sostiene (el caso `"5"` → 5 es intencional y está testeado en `durations_test.go:41`) |
| `clockOf(225)` → `"3:45"`, `""` para 0 o negativos, `"1:02:03"` con horas | `durations.go:75-85` | se sostiene (Y15: el CLI ahora usa el mismo `Clock`) |
| `EnrichDurations` en paralelo, hasta 6 a la vez, corta en `limit` | `durations.go:26-47` | se sostiene |
| `bestThumb` descarta las portadas de más de 400 px | `parse.go:161` | se sostiene (Y11 le agregó el fallback: si sólo hay grandes, usa la más chica) |
| Charts prefiere Top 100 > Top 50 > Top > Trending, +10 "songs", +5 "global" | `charts.go:49-72` | se sostiene |
| Un chart vacío es un error | `charts.go:44` | se sostiene |
| Home tiene fallback a la primera playlist | `home.go:28-34`, `home.go:49-77` | se sostiene |
| `search` manda el filtro de canciones | `client.go:194-196` | se sostiene |
| El player es detachado (`setsid`) y se mata por grupo (`-pid`) | `player_unix.go`, `player.go:216` | se sostiene el mecanismo; **no valida el PID** (Y1) |
| La posición sale de mpv; si no hay `time-pos` se informa `starting` con la duración de catálogo | `player.go:278-285` | se sostiene |
| EOF es la señal de "pasa a la siguiente" y llega antes de que muera el proceso | `player.go:260-272` | se sostiene |
| El estado se escribe de forma atómica (temporal + rename) | `model.go:142-152` | se sostiene, también contra otro escritor (Y5) |
| `DataDir` con permisos 0700 | `model.go:90-95` | se sostiene (Y8: los permisos se ajustan aunque el directorio ya exista) |
| `LoadSession` rechaza un cursor fuera de rango y cubre las cinco operaciones | `radio.go:96-100` | se sostiene |
| `next` avanza el cursor, pide la tanda siguiente y siembra otra si se agotó | `radio.go:128-162` | se sostiene el flujo; el rebase deja la cola truncada si el guardado final falla (Y4) |
| `prev` decrece y falla en la primera | `radio.go:182-185` | se sostiene |
| `shuffle` es Fisher-Yates con `crypto/rand` y deja la actual primero | `radio.go:232-248` | se sostiene (el bucle `index > 0` con `rand.Int(0..index)` es el Fisher-Yates correcto) |
| `enqueue` agrega al final e ignora repetidos | `radio.go:262-267` | se sostiene |
| `queue` devuelve la cola y el cursor tal como están | `radio.go:283-295` | se sostiene |
| Ninguna prueba sale a la red | `fakeserver_test.go:34-69` (transporte reescrito) | se sostiene |
| Cobertura por paquete | `README.md` | se sostiene (Y19: el README declara la real y un test la mide) |

## Revisado y limpio (lo que se buscó y no se encontró)

- **Inyección de shell**: `shellQuote` (`player.go:382-384`) resiste un
  `videoId` hostil. Probe `TestProbe20SinInyeccionShell`: con
  `abc'; touch /tmp/ytmusic-pwned; echo '` el script queda
  `'https://music.youtube.com/watch?v=abc'\''; touch /tmp/ytmusic-pwned; echo '\'''`
  y el `touch` **no** se ejecuta (no se creó el marcador). El `exec.Command`
  de `resolveMetadata` (`player.go:51`) no usa shell, así que el `videoId` no
  puede inyectar argumentos.
- **Caracteres de control en la respuesta del servidor**: `snippet`
  (`client.go:166-175`) puede cortar un UTF-8 a la mitad en el byte 160; el
  `json.Encoder` de la CLI reemplaza el rune inválido, así que no rompe la
  salida. Anotado, no es un bug.
- **`os.RemoveAll` / `os.Rename` con nombre de afuera**: no hay `RemoveAll` en
  el módulo. Los `Rename` son los de `writeJSON` (`model.go:151`) y
  `downloadThumb` (`thumbs.go:102`), ambos con nombre derivado (Y5, Y9).
- **Comparaciones de versión como string**: no hay. `ClientVersion`
  (`client.go:16`) es una constante que se manda, no se compara.
- **Escrituras no atómicas del caché de portadas**: `downloadThumb` escribe
  en `.part` y renombra (`thumbs.go:84-102`); contra otro proceso que baje
  la misma portada el `.part` se comparte. No se llegó a producir un JPEG
  corrupto (probe `TestProbe5cThumbCacheCorrupta`: el sha de lo guardado
  coincidió con el de origen), pero sí se midió que la carrera puede
  dejar **sin archivo** (uno de los pares se lleva el `.part` mientras el
  otro escribe). Queda como riesgo teórico, no como hallazgo.
- **`Thumbs` con el mismo `videoId` en paralelo**: sin corrupción medible
  (mismo probe).
- **Fallo de `EnrichDurations`**: si el `player` falla, la canción queda sin
  duración y la lista sigue (`durations.go:39-41`), como dice el README.
- **`Shuffle`**: Fisher-Yates correcto, la canción actual queda primera y el
  cursor en 0.
- **Orden de `collectRenderers`**: el README tiene razón, las claves se
  ordenan (`parse.go:106-110`); el problema está en `findRenderer` (Y3), que
  es otra función.

## Hipótesis (no reproducidas)

- **`player.go:216` con PID 1** → `syscall.Kill(-1, SIGTERM)` señaliza todos
  los procesos del usuario. No se ejecutó (mataría la sesión de pruebas).
  Lo que faltó: correrlo dentro de un namespace de PID propio
  (`unshare --pid --fork`) para limitar el alcance.
- **`player.go:321`** (`_ = conn.SetDeadline(...)`): si el `SetDeadline`
  fallara, la lectura del IPC se queda sin deadline y el comando de mpv
  cuelga el panel. No se pudo producir un `SetDeadline` fallido sobre un
  socket unix: no hay forma de forzarlo desde el paquete.
- **`player.go:97`** (`_ = os.Remove(socket)`): si el socket viejo no se
  puede borrar, `mpv --input-ipc-server` no puede crear el suyo y el
  reproductor no arranca, pero `Play` devuelve el estado como si hubiera
  arrancado. No se reprodujo: haría falta un socket que no se pueda borrar.

## Sin revisar

- El **lado Noctalia** que llama a este binario: qué le pasa al panel cuando
  el comando devuelve `{"error": ...}` con exit 1, y si reintenta.
- El **cableado** de `cmd/b3l0w-ytmusic/main.go` (12 líneas) y del router de
  `b3l0w` que lo invoca: no se revisó cómo se despacha `b3l0w ytmusic ...`.
- El **contrato real de innertube**: ningún test sale a la red, así que los
  shards, los `params` por país y el `ClientVersion` siguen sin verificar
  contra YouTube (el propio README lo marca como "verificar").
- **`Play` y `resolveMetadata` contra el `yt-dlp` y el `mpv` reales**: los
  probes usaron dobles en el `PATH`. El comportamiento con un `yt-dlp` de
  verdad (formato de salida, flags, versiones) no se comprobó.
- **El comportamiento en el arranque en frío**: `Play` espera 2,5 s a que
  aparezca el socket (`player.go:154-160`) y devuelve el estado aunque el
  socket no haya aparecido; no se midió contra la red real.
