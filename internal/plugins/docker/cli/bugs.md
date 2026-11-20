# Bugs de `internal/plugins/docker/cli`

La capa de comandos de `b3l0w docker`. Acá se anotan los bugs que
encontraron los tests del subpaquete: qué se veía, por qué pasaba, cómo se
arregló y qué test lo cubre.

## `logs-stream` dejaba el `docker logs -f` huérfano si fallaba el pidfile

**Síntoma.** `b3l0w docker logs-stream web` devolvía la ruta del log con
código 0 y el `docker logs -f` seguía corriendo para siempre, sin forma de
pararlo: `b3l0w docker logs-stop web` contestaba `sin seguimiento` porque no
encontraba ningún pid.

**Causa.** En `logsStreamCmd` (`root.go`) el pid se guardaba sin mirar el
error:

```go
pid := cmd.Process.Pid
_ = os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o644)
```

Para cuando se escribía el pidfile, el proceso ya estaba arriba: se había
arrancado con `setsid` (para que sobreviva a la salida del CLI) y se había
hecho `Release()` (para que el CLI no lo espere). Si el `WriteFile` fallaba
—disco lleno, permisos, el directorio no existe— el error se descartaba con
`_ =` y el comando salía con 0 como si nada. El pid se perdía y nadie podía
matar al proceso.

El comentario que precedía esas líneas («el pid se guarda ANTES del
Release») era cierto y a la vez engañoso: explicaba el orden de las dos
operaciones, no que el error del `WriteFile` fuera irrecuperable.

**Arreglo.** El `WriteFile` ahora se chequea, y si falla se mata al proceso
que se acaba de arrancar antes de salir:

```go
if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
    _ = syscall.Kill(-pid, syscall.SIGTERM)
    _ = syscall.Kill(-pid, syscall.SIGKILL)
    _ = cmd.Process.Kill()
    _ = cmd.Wait()
    if info, err := os.Stat(pidFile); err == nil && info.Mode().IsRegular() {
        _ = os.Remove(pidFile)
    }
    fmt.Fprintf(os.Stderr, "no pude guardar el pidfile %s: %v\n", pidFile, err)
    return 1
}
```

Detalles que importan:

- **Se mata el grupo, no sólo el líder.** El proceso es líder de sesión
  (`setsid`), así que su pid es también el del grupo: `Kill(-pid, …)` lleva
  la señal a los hijos también. Es lo mismo que hace `logs-stop`.
- **`SIGKILL` además del `SIGTERM`.** Si `docker` ignorara el `SIGTERM`, el
  `cmd.Wait()` de abajo se quedaría esperando para siempre y el CLI se
  colgaría en vez de devolver el error.
- **Se hace `Wait()`.`Release()` deja al hijo zombi hasta que el padre
  muera; con `Wait()` el proceso se recoge y no queda ni vivo ni zombi.
- **Se borra el pidfile.** Si el `WriteFile` falló a medias, el archivo puede
  quedar con un pid truncado: `logs-stop` leería un número que apunta a otro
  proceso y lo mataría. Sólo se borra si es un archivo común, nunca un
  directorio.
- **El comando sale con 1** y explica el error por stderr. El contrato de
  salida no cambia para el camino feliz: `logs-stream` sigue imprimiendo
  sólo la ruta del log.

**Test que lo cubre.** `TestLogsStreamMataElProcesoSiNoPuedeGuardarElPid`
(`commands_test.go`). Arma un `dirLogs` con un **directorio** en lugar del
pidfile (así el `WriteFile` falla con `EISDIR`, sin depender de permisos ni
de ser root), corre `logs-stream` con un `docker` falso en el `PATH` que se
queda vivo, y comprueba tres cosas: que el comando devuelve 1, que stderr
nombra el pidfile, y que no queda ningún proceso vivo —ni el del script, ni
los de su grupo—. Todo se mira en `/proc`.

Revirtiendo el arreglo (volviendo al `_ = os.WriteFile(...)`), el test falla
con las tres aserciones: `código = 0, quería 1`, `stderr = ""` y
`quedó vivo el docker logs -f (pid [...])`.

---

## `logs-stream` ejecutaba comandos con el id del contenedor

**Síntoma.** `b3l0w docker logs-stream 'web$(touch /tmp/PWNED)'` **ejecutaba**
`touch /tmp/PWNED` y devolvía 0 como si nada. Cualquier cosa que el shell
interprete se ejecutaba con los permisos del usuario que corrió el comando.

**Causa.** El comando se armaba con `fmt.Sprintf` y se pasaba a `sh -c`:

```go
cmd := exec.Command("sh", "-c",
	fmt.Sprintf("exec docker logs -f --tail 200 %q >> %q 2>&1", id, archivo))
```

`%q` es el **quoting de Go**, no el del shell: escapa `"` y `\`, pero deja
intactos `$`, `` ` `` y `$( )`. El resultado es una cadena entre comillas
dobles, y en `sh` las comillas dobles **no** impiden la sustitución de
comandos: `"web$(touch X)"` sigue ejecutando `touch X`.

**Arreglo.** Se dejó de usar `sh -c`. El comando se arma con los argumentos
separados y la redirección al archivo la hace Go con un `*os.File`:

```go
salida, err := os.OpenFile(archivo, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
cmd := exec.Command("docker", "logs", "-f", "--tail", "200", id)
cmd.Stdout = salida
cmd.Stderr = salida
```

El id nunca pasa por un shell. Como efecto lateral, el pid guardado es el de
`docker` (antes lo era el del `sh -c`, que hacía `exec`).

**Test que lo cubre.** `TestLogsStreamNoEjecutaComandosDelId`
(`cli/fixes_test.go`): un `docker` falso en el `PATH` anota sus argumentos, el
id lleva `$(touch ...)` y el archivo marcador **no** existe después del
comando; además se comprueba que el id llegó a docker como un argumento
literal. `TestLogsStreamAceptaIdsConCaracteresRaros` cubre el `;`.

## `logs-stop` mataba un proceso que no era el seguimiento

**Síntoma.** `b3l0w docker logs-stop <contenedor>` mandaba `SIGTERM` al grupo
del pid que estaba en el pidfile sin comprobar que ese pid fuera el del
seguimiento. Si el pid fue reciclado, se mataba un proceso ajeno.

**Causa.** El pidfile guardaba sólo un número y `logsStopCmd` lo usaba sin
validar: ni el nombre del ejecutable, ni la línea de comandos, ni la hora de
arranque.

**Arreglo.** El pidfile ahora guarda **pid + starttime** (el campo 22 de
`/proc/<pid>/stat`, los ticks desde el arranque del sistema). Antes de
mandar la señal, `logs-stop` comprueba que el proceso vivo tenga ese
starttime: si el número fue reciclado, el starttime no coincide y **no se
manda ninguna señal**; el comando contesta "sin seguimiento". Es el mismo
criterio que ya usa `ytmusic` (Y1).

**Test que lo cubre.** `TestLogsStopNoMataUnProcesoAjeno`
(`cli/fixes_test.go`): un proceso propio en su propio grupo, con un pidfile
cuyo starttime no es el suyo ⇒ el proceso sigue vivo y el comando no dice
"detenido". `TestLogsStopMataElSeguimientoValido` comprueba el camino bueno.

**Test ajustado.** `TestLogsStopMataElProceso` (`cli/commands_test.go`)
escribía el pidfile con sólo el pid; ahora escribe el par pid + starttime.

## `volume-export` con ruta relativa montaba la raíz del host

**Síntoma.** `b3l0w docker volume-export --name datos --out backup.tar.gz`
armaba un `docker run` que **montaba la raíz del sistema de archivos del
host** (`-v /:/respaldo`) y escribía el tarball en `/backup.tar.gz` del host.

**Causa.** `splitPath` devolvía `("/", nombre)` cuando la ruta no tenía
ninguna barra, y esa carpeta se usaba como bind mount del host.

**Arreglo.** `splitPath` fue reemplazada por `rutaRespaldo`: la ruta se
resuelve a absoluta dentro del directorio actual y se rechaza lo que queda
fuera (`../x`) y lo que queda en la raíz del sistema. `volume-import` usa el
mismo camino.

**Test que lo cubre.** `TestVolumeExportConRutaRelativaNoMontaLaRaiz` y
`TestVolumeImportConRutaRelativaNoMontaLaRaiz` (`cli/fixes_test.go`): el
comando lleva `-v <cwd>:/respaldo` y **no** `-v /:/respaldo`.
`TestVolumeExportRechazaRutaFueraDelDirectorio` cubre el rechazo.

**Test ajustado.** `TestSplitPath` (`cli/root_test.go`) fijaba el contrato
viejo (`"backup.tar.gz"` ⇒ `("/", "backup.tar.gz")`): se reemplazó por los
tests de `rutaRespaldo`.

## `compose --action` se perdía cuando el flag va después del proyecto

**Síntoma.** `b3l0w docker compose shop --action up` **no** hacía `up`: corría
`docker compose -p shop restart`. Y en el orden inverso, `b3l0w docker compose
--action up shop` armaba `docker compose -p --action up -d`: el proyecto se
llamaba `--action`.

**Causa.** `composeCmd` usaba el `flag` de Go, que **deja de parsear en el
primer argumento posicional**. Con `args = ["shop", "--action", "up"]` el
flag se perdía y `args[0]` (el proyecto) tampoco se separaba bien.

**Arreglo.** `composeCmd` saca el flag con `extraerValor` —que ya existía en
el módulo justamente para esto— y el proyecto es el primer argumento que
sobra.

**Test que lo cubre.** `TestComposeAceptaElActionDespuesDelProyecto`
(`cli/fixes_test.go`): las tres formas arman `compose -p shop up -d`.

## `logs-stream` con un contenedor inexistente devolvía 0

**Síntoma.** `b3l0w docker logs-stream nope` (contenedor inexistente)
devolvía **código 0** e imprimía la ruta del log, como si el seguimiento
hubiera arrancado. El pidfile quedaba apuntando a un proceso ya muerto.

**Causa.** El comando arrancaba `sh -c "exec docker logs ..."` y `cmd.Start()`
tenía éxito (arrancaba el shell, no docker). El error de docker aparecía
después, dentro del log, y nadie lo miraba.

**Arreglo.** El `Wait` del proceso corre en una goroutine: si docker termina
enseguida, el comando falla con el error que docker escribió en el log y
**no deja pidfile de un proceso muerto**.

**Test que lo cubre.** `TestLogsStreamConContenedorInexistenteFalla`
(`cli/fixes_test.go`): con un `docker` falso que falla, el comando sale con
código distinto de 0, el error de docker aparece en stderr y no queda
pidfile.
