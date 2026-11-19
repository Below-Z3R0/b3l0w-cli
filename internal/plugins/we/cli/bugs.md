# Bugs de `internal/plugins/we/cli`

Bugs encontrados al escribir la suite de tests de este subpaquete (agosto 2026).
Cada uno tiene el test que lo cubre: si se revierte el arreglo, ese test falla.

Los arreglos son mínimos y **no cambian el contrato de salida**: stdout sigue
siendo sólo el resultado y stderr el detalle del error.

---

## 1. `list` no aceptaba `--type=valor` ni `--query=valor`

**Síntoma.** `b3l0w we list --type=video` fallaba con
`b3l0w we: opción desconocida: --type=video`, mientras que
`b3l0w we list --type video` sí funcionaba. Lo mismo con `--query`.

**Causa.** `cmdList` parseaba los flags a mano: sólo `--root` aceptaba la forma
con `=`. Los otros siete comandos del paquete (`convert`, `poster`, `still`,
`scan`, `solid`) usan un helper local que acepta las dos formas, y la
documentación del paquete (`root.go`, "Parseo de flags") dice que todos lo
hacen.

**Arreglo.** `cmdList` ahora usa el mismo helper `value()` que el resto del
paquete: acepta `--flag valor` y `--flag=valor`, y un flag sin valor da
`--flag necesita un valor` en vez de `opción desconocida`.

**Test.** `TestListaAceptaElTipoConIgual` (y `TestListaAceptaElValorConIgual`).

---

## 2. `deps` no aceptaba `--root=valor`

**Síntoma.** `b3l0w we deps --root=/ruta` fallaba con
`b3l0w we: opción desconocida: --root=/ruta`.

**Causa.** Misma causa que el bug 1: `cmdDeps` parseaba `--root` a mano y sólo
aceptaba la forma con espacio.

**Arreglo.** `cmdDeps` usa el helper `value()` como el resto del paquete.

**Test.** `TestDepsAceptaElRootConIgual`.

---

## 3. `scan` ignoraba las opciones desconocidas

**Síntoma.** `b3l0w we scan --dir R --no-existe` salía con código 0 y listaba
el contenido de `R` como si el flag no existiera. Un typo en un flag pasaba
desapercibido.

**Causa.** El `default` del switch de `cmdScan` sólo miraba si el argumento
empezaba con `-` para decidir si era una carpeta suelta; si empezaba con `-` lo
ignoraba en silencio, sin tomar el valor siguiente ni avisar.

**Arreglo.** Un argumento que empieza con `-` y no es un flag conocido ahora
da `opción desconocida: <arg>` y sale con código 1. Las carpetas sueltas
siguen funcionando igual.

**Test.** `TestScanOpcionDesconocidaFalla`.

---

## 4. `solid` ignoraba las opciones desconocidas

**Síntoma.** `b3l0w we solid --color '#fff' --out f.png --no-existe` salía con
código 0 y escribía el PNG, como si el flag desconocido no existiera.

**Causa.** El switch de `cmdSolid` no tenía rama `default`: cualquier
argumento que no fuera `--color`, `--out`, `--size` o `--help` caía fuera sin
avisar.

**Arreglo.** Se agrega la rama `default` con `opción desconocida: <arg>`.

**Test.** `TestSolidOpcionDesconocidaFalla`.

---

## 5. `list` contaba mal los wallpapers filtrados

**Síntoma.** `b3l0w we list --type video` sobre un catálogo de 3 items (2 de
tipo video) imprimía `2 de 2 wallpapers` en stderr. El número del total era
siempre igual al de la selección: el "de" no significaba nada.

**Causa.** `cmdList` hacía `fmt.Fprintf(os.Stderr, "\n%d de %d wallpapers\n",
len(items), len(items))` **después** de `we.Filter`, así que los dos `%d` veían
la lista ya filtrada.

**Arreglo.** El total se toma antes de filtrar y se usa en el resumen.

**Test.** `TestListaMuestraCuantosDeCuantos` (y `TestListaMuestraLaTabla` para
el caso sin filtro).

---

## Notas (no son bugs, son decisiones del código)

- **`poster` con todos los wallpapers fallando sale con 1, no con 2.** El
  código 2 de `poster.go` ("todo falló pero el lote siguió") es una rama
  **muerta**: cuando todos los renders fallan, `we.Posters` devuelve el error
  ("no se pudo generar ningún poster") y `cmdPoster` sale con 1 antes de
  llegar al conteo. Un fallo parcial sí sale con 0, con el item marcado
  `failed` en el manifiesto y el conteo `N fallidos` por stderr
  (`TestPosterMarcaElWallpaperQueFallo`). No se tocó el código: cambiar un
  código de salida es tocar el contrato con el panel.
- **`convert --method` se parsea pero no se usa.** El valor nunca se lee:
  `convert` siempre llama a `we.Extract`. El método `record` que anuncia el
  `--help` no está implementado en esta capa (ya está anotado como gap
  conocido en el README de `internal/plugins/we`). Los tests fijan el
  comportamiento actual, no la intention.
