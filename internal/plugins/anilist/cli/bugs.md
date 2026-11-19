# Bugs de `internal/plugins/anilist/cli`

La capa de parseo y salida de `b3l0w anilist`. Acá se anotan los bugs que
encontraron los tests de `root_test.go`: qué se veía, por qué pasaba, cómo se
arregló y qué test lo cubre.

## `status` no aceptaba `--token`

**Síntoma.** `b3l0w anilist status --token <token>` moría con código 2 y el error
del flag en stderr:

```
flag provided but not defined: -token
```

El texto de ayuda anuncia la opción `--token` bajo «Opciones comunes», pero `status`
era el único comando que no lo declaraba: todos los demás arman su flagset
con `--token`, y `status` lo armaba a mano con dos flags (`--json` y
`--data-dir`) y se olvidó del token.

**Causa.** En `runStatus` el flagset se crea a mano en vez de con
`newFlags`, y el chequeo del token usaba siempre el valor por defecto:

```go
if _, err := anilist.Token("", "ANILIST_TOKEN", dir); err != nil {
```

El `""` del primer argumento es el token explícito: con el flag
`--token` no definido no había forma de pasarlo, así que la comprobación
sólo miraba la variable de entorno y `token.json`. Un token válido pasado
por flag se reportaba como ausente (`hasToken: false`).

**Arreglo.** Se declaró el flag y se pasó su valor al chequeo:

```go
token := fs.String("token", "", "token de AniList")
...
if _, err := anilist.Token(*token, "ANILIST_TOKEN", dir); err != nil {
```

El cambio no altera el contrato de salida: `status` sigue emitiendo el
mismo JSON (`dataDir`, `hasToken`, `coversDir`, `covers`, `coversBytes`) y
el mismo texto humano. Sólo cambia que ahora `--token` es una opción
válida y que un token por flag cuenta como token presente.

**Test que lo cubre.** `TestStatusAceptaElTokenExplicito` (corre el comando
en un subproceso porque los flagsets usan `flag.ExitOnError`).
`TestStatusDetectaElTokenEnElDisco` cubre el camino del `token.json` y
`TestStatusReportaElTokenYElCache` el de «no hay token».

## Nota: `stats` manda sus errores sólo a stderr

No es un bug: es una decisión deliberada y el test la fija. `stats` es el
único comando cuyo error **no** va a stdout, porque el plugin lee stdout
como JSON y un objeto de error ahí lo rompe (`statsFail` en `root.go`).
Los demás comandos sí escriben `{"error": "..."}` en stdout cuando van con
`--json`, y el plugin distingue el error por el código de salida, no por el
cuerpo. Lo cubren `TestStatsConErrorDejaStdoutVacio` y
`TestComandoQueConsultaLaApiFallaConCodigoUno`.
