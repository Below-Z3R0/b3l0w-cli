# BUGS — internal/plugins/ytmusic

Revisión de integridad en contexto cero sobre `internal/plugins/ytmusic`:
**2.286 líneas de producción** (1.896 del paquete + 390 de `cli/root.go`) y
**3.967 de tests** (13 archivos de test + `cli/root_test.go`). Base: `main`
en `aebf878`, working tree limpio. **No se corrigió nada**: este archivo es el
reporte.

El módulo nunca pasó por una revisión de integridad: los 151 hallazgos de la
auditoría anterior cubrieron `internal/*`, no los plugins.

Estado de la suite al cerrar:

```
$ cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli
$ go test ./internal/plugins/ytmusic/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic	1.706s	coverage: 85.0% of statements
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic/cli	0.003s	coverage: 32.5% of statements
$ go build ./...
(sin salida)
```

Los dos verdes: no se tocó código de producción ni de test.

## Cómo se corrieron las reproducciones

Cada hallazgo va con un probe que se ejecutó **contra una copia del paquete**,
para no dejar un archivo de test en el repo:

```bash
mkdir -p /tmp/ytprobe/ytmusic
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli
cp internal/plugins/ytmusic/*.go /tmp/ytprobe/ytmusic/
printf 'module ytprobe\n\ngo 1.27.3\n' > /tmp/ytprobe/go.mod
# + el probe del hallazgo (van abajo, en package ytmusic)
cd /tmp/ytprobe && go test ./ytmusic/ -run TestProbe -v -count=1
```

Los nombres de los probes son `TestProbe<N>` y el prefijo de cada línea de
salida (`P<N>`) corresponde al hallazgo. Las salidas citadas son literales.

## Resumen

| ID | Sev | Qué | Línea | Estado |
|---|---|---|---|---|
| Y1 | CRÍTICO | `Stop` mata el grupo de procesos del PID que dice `player.json`, **sin validar que sea el nuestro**: mata procesos ajenos | `player.go:209-235` | abierto |
| Y2 | ALTO | `Status` y `Stop` borran el archivo que dice el campo `socket` de `player.json`: borrado de archivo arbitrario | `player.go:232,257` | abierto |
| Y3 | ALTO | `findRenderer` camina el map en el orden **aleatorio** de Go: el token de continuación (y el `videoId`) cambian entre llamadas al mismo payload | `parse.go:75-94,170-188` | abierto |
| Y4 | ALTO | El rebase de `next` escribe la cola **truncada** en disco antes del guardado final: si ese guardado falla, se pierde la cola | `radio.go:140` + `radio.go:58,65-73` | abierto |
| Y5 | ALTO | `writeJSON` usa un `.tmp` de **nombre fijo**: dos escritores del mismo archivo se pisan, uno falla con ENOENT y la actualización del otro se pierde | `model.go:142-152` | abierto |
| Y6 | MEDIO | `Play` no tiene timeout en ningún punto: un `yt-dlp` colgado cuelga el comando para siempre | `player.go:51,88` + `cli/root.go:244,305` | abierto |
| Y7 | MEDIO | `Play` deja el proceso de reproducción **huérfano** si el estado no se puede guardar | `player.go:148` | abierto |
| Y8 | MEDIO | `EnsureDir` no ajusta los permisos de un directorio que ya existe: el socket de mpv puede quedar en un dir `0755` | `model.go:90-95` | abierto |
| Y9 | MEDIO | `Thumbs` arma la ruta con el `videoId` que viene de la red, sin sanear: puede escribir **fuera** del directorio de portadas | `thumbs.go:32,57` | abierto |
| Y10 | MEDIO | `sessionPath` arma la ruta con el valor de `--session`: puede leer/escribir **fuera** de `sessions/` | `radio.go:20-22` | abierto |
| Y11 | MEDIO | `bestThumb` se queda **sin portada** si todas las que trae la página son de más de 400 px | `parse.go:149-166` | abierto |
| Y12 | MEDIO | `durationToSeconds` **desborda** con un número enorme: devuelve segundos negativos o absurdos | `model.go:115-138` | abierto |
| Y13 | MEDIO | Un `player.json` corrupto hace que `status` diga "parado" para siempre y **nunca** limpia el archivo roto | `player.go:239-243` | abierto |
| Y14 | BAJO | `limit <= 0` significa cosas distintas: `Thumbs` baja **todas** las portadas, `EnrichDurations` no hace nada | `thumbs.go:26` vs `durations.go:20` | abierto |
| Y15 | BAJO | El reloj del CLI no lleva horas: 3.723 s se muestran como `62:03` | `cli/root.go:361-366` | abierto |
| Y16 | BAJO | `seek abc` no falla: el error de `Sscanf` se ignora y salta a 0 | `cli/root.go:330` | abierto |
| Y17 | BAJO | Código muerto: `if song.Title == "" { song.Title = "" }` | `cli/root.go:302-304` | abierto |
| Y18 | BAJO | El reintento espera 400 ms **también después del último intento** | `client.go:103` | abierto |
| Y19 | BAJO | La cobertura que promete el README (83,7 %) no es la real (85,0 % del paquete, 32,5 % de `cli/`) | `README.md:139` | abierto |
| Y20 | BAJO | El paquete no compila fuera de Unix (`syscall.Kill`, `Setsid`) y no tiene build tag | `player.go:127,216` | abierto |

Totales: **1 CRÍTICO, 4 ALTO, 8 MEDIO, 7 BAJO** (20 hallazgos, todos reproducidos).

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

`README.md:139` dice "Cobertura: **83,7 %**". La medida de hoy:

```
$ go test ./internal/plugins/ytmusic/... -count=1 -cover
ok  	.../internal/plugins/ytmusic	1.706s	coverage: 85.0% of statements
ok  	.../internal/plugins/ytmusic/cli	0.003s	coverage: 32.5% of statements
```

El 83,7 % no corresponde ni al paquete (85,0 %) ni a `cli/` (32,5 %). Si la
cifra era del paquete, hay que actualizarla; si era de las dos juntas, hay
que aclarar de qué se mide.

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
| Contexto de 45 s para feeds y 40 s para radio/next | `cli/root.go:156,199,218` | se sostiene para esos tres; **`play` no tiene contexto** (Y6) |
| Headers `X-YouTube-Client-Name: 67`, versión, Origin, Referer y User-Agent | `client.go:92-98` | se sostiene |
| `post` mira `playabilityStatus` y lo traduce a error legible | `client.go:129-133`, `client.go:142-156` | se sostiene |
| 2 intentos con 400 ms de espera; 5xx y red se reintentan, 4xx no | `client.go:87-119` | se sostiene (con Y18: espera de más) |
| Cuerpo limitado a 32 MB | `client.go:106` | se sostiene |
| Errores recortados a 160 caracteres | `client.go:166-175` | se sostiene |
| `durationToSeconds("3:45")` → 225, tolera `"1:02:03"`, da 0 en las entradas raras | `model.go:115-138` | se sostiene (el caso `"5"` → 5 es intencional y está testeado en `durations_test.go:41`) |
| `clockOf(225)` → `"3:45"`, `""` para 0 o negativos, `"1:02:03"` con horas | `durations.go:75-85` | se sostiene (el CLI tiene su propio reloj: Y15) |
| `EnrichDurations` en paralelo, hasta 6 a la vez, corta en `limit` | `durations.go:26-47` | se sostiene |
| `bestThumb` descarta las portadas de más de 400 px | `parse.go:161` | se sostiene, pero sin fallback (Y11) |
| Charts prefiere Top 100 > Top 50 > Top > Trending, +10 "songs", +5 "global" | `charts.go:49-72` | se sostiene |
| Un chart vacío es un error | `charts.go:44` | se sostiene |
| Home tiene fallback a la primera playlist | `home.go:28-34`, `home.go:49-77` | se sostiene |
| `search` manda el filtro de canciones | `client.go:194-196` | se sostiene |
| El player es detachado (`setsid`) y se mata por grupo (`-pid`) | `player.go:127`, `player.go:216` | se sostiene el mecanismo; **no valida el PID** (Y1) |
| La posición sale de mpv; si no hay `time-pos` se informa `starting` con la duración de catálogo | `player.go:278-285` | se sostiene |
| EOF es la señal de "pasa a la siguiente" y llega antes de que muera el proceso | `player.go:260-272` | se sostiene |
| El estado se escribe de forma atómica (temporal + rename) | `model.go:142-152` | **no se sostiene contra otro escritor** (Y5) |
| `DataDir` con permisos 0700 | `model.go:90-95` | **no se sostiene si el directorio ya existía** (Y8) |
| `LoadSession` rechaza un cursor fuera de rango y cubre las cinco operaciones | `radio.go:96-100` | se sostiene |
| `next` avanza el cursor, pide la tanda siguiente y siembra otra si se agotó | `radio.go:128-162` | se sostiene el flujo; el rebase deja la cola truncada si el guardado final falla (Y4) |
| `prev` decrece y falla en la primera | `radio.go:182-185` | se sostiene |
| `shuffle` es Fisher-Yates con `crypto/rand` y deja la actual primero | `radio.go:232-248` | se sostiene (el bucle `index > 0` con `rand.Int(0..index)` es el Fisher-Yates correcto) |
| `enqueue` agrega al final e ignora repetidos | `radio.go:262-267` | se sostiene |
| `queue` devuelve la cola y el cursor tal como están | `radio.go:283-295` | se sostiene |
| Ninguna prueba sale a la red | `fakeserver_test.go:34-69` (transporte reescrito) | se sostiene |
| Cobertura 83,7 % | — | **no se sostiene** (Y19) |

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
