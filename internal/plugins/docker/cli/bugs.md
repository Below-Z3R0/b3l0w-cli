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
