# Bugs de `internal/plugins/we`

Revisión de integridad en contexto cero, hecha sobre la rama `mod/rev-we`
(base `68d4a8d`, igual que `main`).
**No se arregló nada**: este archivo sólo describe lo que se encontró.

Método: se leyó el módulo completo (producción + tests) y se contrastó cada
afirmación del `README.md` contra el código. Los probes son `_test.go`
temporales con nombre `zz_probe*_test.go` en el mismo paquete, con
`t.TempDir()`, `$HOME`/`$PATH` temporales y binarios falsos; **todos se
borraron al terminar** y el árbol quedó como estaba (`git status` muestra sólo
este `bugs.md` nuevo).

Los 5 bugs de la **capa de comandos** que ya están arreglados viven en
`internal/plugins/we/cli/bugs.md`. Ese archivo es el registro histórico de
arreglos: **no** es el inventario de este módulo y no se tocó.

## Resumen

| ID | Sev | Archivo:línea | Síntoma |
|---|---|---|---|
| C1 | CRÍTICO | `catalog.go:136-141` | `project.json` controla la ruta: un `file` con `../` saca el video de la carpeta del item y `we convert` lo copia a la salida |
| A1 | ALTO | `cli/still.go:125` | `we still` borra la miniatura previa **antes** de extraer: si ffmpeg falla, no queda miniatura |
| A2 | ALTO | `cli/solid.go:100` + `103` | El tope de `--size` se puede saltar con overflow de `int64`: `4611686018427387904x4611686018427387904` revienta con un panic |
| A3 | ALTO | `cli/solid.go:115` | `we solid` pisa el archivo de salida sin preguntar y sin `--overwrite` |
| M1 | MEDIO | `catalog.go:174` | `Filter` con un tipo que no existe (`--type videoz`) no filtra: devuelve el catálogo entero |
| M2 | MEDIO | `ffmpeg.go:85` | Las copias portables de ffmpeg se ordenan como texto: `ffmpeg-9.0` gana sobre `ffmpeg-10.0` |
| M3 | MEDIO | `cli/scan.go:151` | `we scan --kind` no valida el valor: `--kind videoz` se trata como `image` sin avisar |
| M4 | MEDIO | `cli/thumbs.go:65` + `86-96` | Una miniatura a medias (ffmpeg cortado) queda en disco y el caché la da por fresca en la corrida siguiente |
| M5 | MEDIO | `manifest.go:63` | `LoadManifest` acepta un `version` desconocido (99) y lo reescribe como si fuera válido |
| B1 | BAJO | `catalog.go:195-209` | `Slug` tira los acentos: "Café Münster Ñandú" → "caf-m-nster-and" |
| B2 | BAJO | `poster.go:58` vs `26` | `PosterOptions.Delay` dice "segundos" en el doc pero el flag del engine usa frames |
| B3 | BAJO | `deps.go:166` | `Summary()` muestra `workshop:N items` y nunca `workshop:falta`, al revés que el resto de los bloques |

**Totales:** 1 CRÍTICO · 3 ALTO · 5 MEDIO · 3 BAJO — 12 hallazgos.

---

## C1 — CRÍTICO · `we convert` copia un archivo de fuera del workshop

**Dónde.** `internal/plugins/we/catalog.go:136-141`.

```go
if proj.File != "" {
    it.File = filepath.Join(dir, proj.File)
}
if proj.Preview != "" {
    it.Preview = filepath.Join(dir, proj.Preview)
}
```

**Síntoma.** El `project.json` de cada item del workshop es un archivo que
**baja Steam**, no algo que wrote el usuario. Su campo `file` se mete en
`filepath.Join(dir, proj.File)` sin ninguna validación, y `filepath.Join`
**no** neutraliza `..`: sólo limpia la ruta. El resultado es que `it.File`
puede apuntar a cualquier archivo del disco.

Después, `we convert` copia ese archivo a la carpeta de salida y lo declara
`status: "ok"` en el manifiesto, como si fuera el video del wallpaper.

**Causa.** Falta la validación que el paquete padre sí tiene para el mismo
patrón: `internal/plugins/install.go:200` llama a `system.SafeName(name)`
antes de usar un nombre que viene de afuera, y el comentario de ahí explica
exactamente por qué ("un ID o un Path con `..` borra el directorio padre"). En
`internal/plugins/we/` no hay equivalente: `catalog.go` no llama a `SafeName`
ni valida el `file` contra la carpeta del item.

**Reproducción (paquete `we`, `t.TempDir()`).** Un `project.json` cuyo `file`
es `../secret.mp4`:

```go
proj := `{"title":"T","type":"Video","file":"../secret.mp4","preview":"../../preview.png"}`
os.WriteFile(filepath.Join(item, "project.json"), []byte(proj), 0o644)
items, _ := Scan(root)
```

```console
$ go test ./internal/plugins/we/ -run TestProbeTraversal -v
    zz_probe_test.go:29: Item.File   = /…/TestProbeTraversal2248282774/001/secret.mp4
    zz_probe_test.go:30: Item.Preview= /…/TestProbeTraversal2248282774/preview.png
    zz_probe_test.go:31: File fuera del dir del item: true
    zz_probe_test.go:38: Extract copió a /…/out/123-t.mp4 contenido="TOP-SECRET-CONTENT"
--- PASS: TestProbeTraversal (0.00s)
```

El `file` salió del directorio del item (`001/123/`) hacia `001/`. Con más
niveles de `..` sale de la raíz del workshop:

```console
$ go test ./internal/plugins/we/ -run TestProbeTraversalProfundo -v
    zz_probeB_test.go:21: Item.File    = /home/b3low-z3r0/etc/passwd
    zz_probeB_test.go:22: Item.Preview = /home/b3low-z3r0/.hermes/profiles/coding/cache/scratch/etc/hosts
    zz_probeB_test.go:23: File escapa la raíz del workshop: true
    zz_probeB_test.go:24: Preview escapa la raíz del workshop: true
--- PASS: TestProbeTraversalProfundo (0.00s)
```

**Reproducción (punta a punta, por la CLI).** El comando completo, con un
workshop falso y un `project.json` malicioso:

```go
code := cmdConvert([]string{"--root", root, "--id", "999", "--out", out, "--crf", "0"})
```

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeTraversalCLI -v
  ✓ T                                        15 B
    zz_probe2_test.go:27: exit=0
    zz_probe2_test.go:31: salidó 999-t.mp4 contenido="CLAVE-DE-PRUEBA"
    zz_probe2_test.go:31: salidó manifest.json contenido="… \"status\": \"ok\", \"file\": \"…/out/999-t.mp4\", \"source\": \"…/001/secret.mp4\", \"method\": \"copy\" …"
--- PASS: TestProbeTraversalCLI (0.00s)
```

**Impacto.** `we convert --all` sobre un workshop con un item malicioso
copia a la carpeta de salida un archivo arbitrario legible por el usuario
(`~/.ssh/id_rsa`, `~/.aws/credentials`, un token de la API) y lo publica en el
manifiesto como `status: ok`. El plugin de Noctalia consume ese manifiesto:
el archivo ajeno queda expuesto como si fuera el wallpaper. El `preview` tiene
el mismo problema (queda como `source` en la entrada del manifiesto del poster).
El `README.md` promete en `catalog.go` que "`Properties` conserva el bloque
`general.properties` **tal cual**" pero no dice nada de que `file`/`preview`
se traten como rutas confiables: hoy son la excepción silenciosa.

**Nota.** El `file` no puede apuntar a un *directorio* (`extract.go:70` lo
rechaza) y el destino de la copia sí está bien armado (`OutputName` usa el
`id` numérico y el `Slug` saneado). El agujero es sólo la **entrada**.

---

## A1 — ALTO · `we still` borra la miniatura anterior antes de saber si va a poder

**Dónde.** `internal/plugins/we/cli/still.go:125`, dentro del `for _, seek := range seeks`:

```go
for _, seek := range seeks {
    _ = os.Remove(abs)
    cmdArgs := []string{"-y"}
```

**Síntoma.** `we still --file V --out J` borra `J` (la miniatura que ya
existed) y recién después corre ffmpeg. Si ffmpeg falla —el caso más común es
un video sin frame en ese segundo— el comando sale con 1 y **no queda
miniatura**: se perdió la anterior. El plugin usa `we still` para las
miniaturas de video (`SPEC-we.md`), así que un video que antes tenía cuadro
portada queda sin él.

**Causa.** Es el único comando del módulo que **borra el destino antes de
escribir**. `Extract` (`extract.go:134`) y `SaveManifest` (`manifest.go:95`)
escriben a un temporal y renombran al final, justamente para que el destino
nunca quede a medias. `still` rompe ese patrón.

**Reproducción.** Un `ffmpeg` falso en el `PATH` que sale con 1, y una
miniatura previa en el destino:

```go
os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
os.WriteFile(out, []byte("MINIATURA PREVIA"), 0o644)
code := cmdStill([]string{"--file", src, "--out", out})
```

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeStillBorraSalida -v
b3l0w we: no se pudo extraer un cuadro de /…/v.mp4: ffmpeg … : exit status 1 (stderr: sin stderr)
    zz_probe_test.go:32: exit=1 | la miniatura previa sigue en disco: false (err=stat /…/thumbs/t.jpg: no such file or directory)
--- PASS: TestProbeStillBorraSalida (0.01s)
```

**Impacto.** Pérdida de un archivo que el comando no pudo reemplazar. Con el
`-ss` alterno (el retry de `still.go:119`) el segundo intento también borra
antes de escribir, así que ambos intentos dejan el destino vacío.

---

## A2 — ALTO · El tope de `--size` de `we solid` se salta con overflow

**Dónde.** `internal/plugins/we/cli/solid.go:100` y `103`.

```go
if int64(width)*int64(height) > 1<<26 {
    return fail("--size demasiado grande: maximo 8192x8192")
}
img := image.NewRGBA(image.Rect(0, 0, width, height))
```

**Síntoma.** El `README.md` dice que este tope es "la defensa contra un input
hostil o un typo" y que "sin él, un valor absurdo revienta la máquina". Pero
`width*height` se calcula en `int64` **después** de multiplicar, y el
producto puede desbordar: `4611686018427387904 * 4611686018427387904` da un
resultado negativo, así que la comparación `> 1<<26` es falsa y el tope no
dispara. Después `image.NewRGBA` hace `panic: image: NewRGBA Rectangle has
huge or negative dimensions` y el proceso muere con un stack trace.

**Causa.** Falta la división antes de multiplicar (o un chequeo con `>` en vez
de `>=` sobre operandos ya acotados). El mismo patrón aparece en el hermano
`we scan --max`, pero ahí el valor es un `int` y el corte `max > 100000` se
aplica antes de cualquier asignación, así que aguanta.

**Reproducción (CLI real, sin tests).**

```console
$ go run ./cmd/b3l0w-we solid --color '#ffffff' --out /tmp/ov.png --size 4611686018427387904x4611686018427387904
panic: image: NewRGBA Rectangle has huge or negative dimensions

goroutine 1 [running]:
image.pixelBufferLength(0x6?, {{0x10?, 0x20?}, {0x4bf05d?, 0x7ffd73428d08?}}, {0x5b2347?, 0x555136?})
	/usr/lib/go/src/image/image.go:98 +0xb9
image.NewRGBA({{0x7ffd73428dca1?, 0x7?}, {0x5b2005?, 0x7?}})
exit=141
```

**Reproducción (test).**

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeSolidOverflow -v
    zz_probe2_test.go:41: PANIC: image: NewRGBA Rectangle has huge or negative dimensions
--- PASS: TestProbeSolidOverflow (0.00s)
```

**Impacto.** El comando no "falla con un mensaje": muere con un panic. Como
`we solid` es lo que el plugin llama para pintar un color, un `--size` con
basura (un typo de 20 dígitos) tumba el proceso en vez de dar un error
accionable. La nota del `README.md` ("Sin él, un valor absurdo revienta la
máquina (OOM kill del kernel)") describe un riesgo que **no** es el que
ocurre: aquí revienta antes, por el panic.

---

## A3 — ALTO · `we solid` pisa el archivo de salida sin preguntar

**Dónde.** `internal/plugins/we/cli/solid.go:115`:

```go
file, err := os.Create(out)
```

**Síntoma.** `we solid --color H --out P` **sobrescribe** `P` sin aviso y sin
que exista un `--overwrite`. Si `P` ya existe, se pierde su contenido. Los
otros dos comandos que escriben un archivo de destino **sí** tienen la
protección: `we convert` saltea lo existente salvo `--overwrite`
(`extract.go:79-87`) y `we poster` hace lo mismo con el PNG
(`poster.go:102-106`). `we solid` es la excepción.

**Causa.** `os.Create` trunca el archivo si existe. El README documenta el
"nunca sobreescribe sin `Overwrite`" como una regla del módulo
(`README.md:128`), pero la regla no se aplica acá.

**Reproducción.**

```go
os.WriteFile(out, []byte("CONTENIDO PREVIO"), 0o644)
code := cmdSolid([]string{"--color", "#ff0000", "--out", out})
```

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeSolidPisa -v
/home/b3low-z3r0/…/TestProbeSolidPisa1867856123/001/wall.png
    zz_probe_test.go:43: exit=0 | bytes=180126 | empieza con firma PNG=true
--- PASS: TestProbeSolidPisa (0.08s)
```

El archivo pasó de `CONTENIDO PREVIO` (15 bytes) a un PNG de 180126 bytes, y
el comando salió con 0.

**Impacto.** Pérdida de datos del archivo de salida. El caso realista es el
plugin apuntando `setWallpaper` a un archivo que el usuario tiene ahí: se
pisa sin aviso. Notar que `we solid` tampoco recibe `--out` con un
directorio (es un archivo, no una carpeta) ni valida la extensión.

---

## M1 — MEDIO · `we list --type` con un tipo inválido no filtra nada

**Dónde.** `internal/plugins/we/catalog.go:169-183`, en el filtro de `Filter`:

```go
typ = NormalizeType(typ)
…
if typ != "unknown" && it.Type != typ {
    continue
}
```

**Síntoma.** `NormalizeType` mapea cualquier tipo desconocido a `"unknown"`, y
después el filtro trata `"unknown"` como "sin filtro". O sea: `we list --type
videoz` (un typo) devuelve el catálogo **entero**, no cero items y no un
error. El usuario cree que filtró y ve 40 wallpapers.

Lo mismo pasa con `--type unknown`, que según el propio README es un tipo
legítimo ("lo unifica a `video | scene | web | unknown`"): pedir los items
unknown devuelve todos.

**Causa.** El valor centinela "sin filtro" y el tipo real "unknown" son el
mismo string. En `cli/list.go:41` el valor crudo de `--type` pasa sin
validar, así que no hay forma de distinguir "no pidió filtro" de "pidió un
tipo que no existe".

**Reproducción.**

```go
items := []Item{{ID:"1",Type:"video"},{ID:"2",Type:"scene"},{ID:"3",Type:"unknown"}}
Filter(items, "bogus", "")
```

```console
$ go test ./internal/plugins/we/ -run TestProbeFilter -v
    zz_probe_test.go:47: Filter(bogus)   -> 3 de 3
    zz_probe_test.go:48: Filter(unknown) -> 3 de 3
    zz_probe_test.go:49: Filter(video)   -> 1 de 3
--- PASS: TestProbeFilter (0.00s)
```

Y por la CLI, con un catálogo de 3 items (2 video, 1 scene):

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeListTypeUnknown -v
ID  TIPO   TAMAÑO  TÍTULO
1   video  29 B    T1
2   video  29 B    T2
3   scene  29 B    T3

3 de 3 wallpapers
    zz_probe3_test.go:36: exit=0
```

`--type unknown` devolvió los 3 items, incluidos los 2 video.

**Impacto.** El filtro por tipo no filtra cuando el tipo no existe. El
README dice que "`Filter(items, typ, query)` filtra por tipo"; hoy, para
`unknown` y para cualquier typo, no filtra.

---

## M2 — MEDIO · Las copias portables de ffmpeg se ordenan como texto

**Dónde.** `internal/plugins/we/ffmpeg.go:81-86`:

```go
matches, err := filepath.Glob(filepath.Join(home, hermesToolsSub, "ffmpeg-*", "bin"))
…
sort.Sort(sort.Reverse(sort.StringSlice(matches)))
```

**Síntoma.** El README dice que las copias portables se buscan "en orden
inverso para que gane la versión más nueva". Pero el orden inverso es
**alfabético**, no por versión: `"ffmpeg-9.0"` es mayor que `"ffmpeg-10.0"`
como texto (`'9' > '1'`), así que con ambas instaladas gana la **9.0**.

**Causa.** Es el patrón "comparaciones de versión como string" que el encargo
pedía buscar. El `sort` compara rutas completas, no números de versión.

**Reproducción.**

```go
for _, v := range []string{"ffmpeg-7.1", "ffmpeg-8.0", "ffmpeg-9.0", "ffmpeg-10.0"} {
    os.MkdirAll(filepath.Join(home, ".hermes", "tools", v, "bin"), 0o755)
}
toolDirs()
```

```console
$ go test ./internal/plugins/we/ -run TestProbeToolDirs -v
    zz_probe_test.go:61: toolDirs = [/…/.hermes/tools/ffmpeg-9.0/bin /…/.hermes/tools/ffmpeg-8.0/bin /…/.hermes/tools/ffmpeg-7.1/bin /…/.hermes/tools/ffmpeg-10.0/bin]
--- PASS: TestProbeToolDirs (0.00s)
```

La 10.0 quedó **última**. En esta máquina hay una sola copia
(`ffmpeg-9.0.1-linux-x64`), así que el bug está latente: aparece en cuanto
haya dos versiones con distinto número de dígitos.

**Impacto.** Se usa un ffmpeg viejo sin avisar. No es un crash, pero el
comportamiento no es el documentado.

---

## M3 — MEDIO · `we scan --kind` no valida el valor

**Dónde.** `internal/plugins/we/cli/scan.go:149-153`:

```go
exts := imageExts
itemType := "image"
if kind == "video" {
    exts, itemType = videoExts, "video"
}
```

**Síntoma.** `--kind` acepta cualquier cosa y sólo distingue exactamente
`"video"`. `--kind videoz`, `--kind VIDEO` o `--kind Video` caen todos en la
rama de imágenes **sin error**. El comando sale con 0 y lista imágenes
cuando el usuario pidió videos (o al revés).

El `README.md` documenta `--kind image|video` como si fuera un conjunto
cerrado, y el parser sí valida `--sort` (`scan.go:108-110`) y `--max`
(`scan.go:129`). `--kind` se quedó sin validar.

**Causa.** El `switch` de flags de `cmdScan` asigna `kind = v` sin más
(`scan.go:118-123`), y el `default` de esa rama es "cualquier valor".

**Reproducción.**

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeScanKind -v
[{"id":"/…/a.jpg","file":"/…/a.jpg","type":"image","size":1,"mod":1795177325,"name":"a","title":"a"}]
    zz_probe_test.go:52: exit=0 (--kind bogus no es imagen ni video)
--- PASS: TestProbeScanKind (0.00s)
```

**Impacto.** Un typo en `--kind` devuelve el conjunto equivocado con código
de salida 0: el plugin muestra imágenes donde el usuario pidió videos. Es
justo el caso que `--sort` y `--max` ya tratan con error.

---

## M4 — MEDIO · Una miniatura a medias queda cacheada como válida

**Dónde.** `internal/plugins/we/cli/thumbs.go:63-69` (la escritura) y
`thumbs.go:86-96` (el cache):

```go
args := []string{"-v", "error", "-y", …, "-vf", fmt.Sprintf("scale=%d:-2", width), "-q:v", "5", out}
if err := exec.CommandContext(ctx, ff.Path, args...).Run(); err != nil {
    items[i].Thumb = ""
    continue
}
…
func fresh(thumb, source string) bool {
    …
    return ti.ModTime().After(si.ModTime()) && ti.Size() > 0
}
```

**Síntoma.** ffmpeg escribe **directo** sobre el nombre final (no a un
temporal). Si ffmpeg falla a medio escribir, el archivo queda en disco con
contenido parcial. En la corrida siguiente, `fresh` lo acepta: mira que sea
más nuevo que el original y que tenga tamaño > 0. Un JPEG de 12 bytes
"fresco" pasa. El `thumb` del JSON apunta a un JPEG roto y **no se
regenera nunca más**, porque el original no cambió de fecha.

**Causa.** Falta el patrón de temporal+rename que sí usan `extract.go:134` y
`manifest.go:95`. Y el chequeo de frescura valida tamaño, no integridad.

**Reproducción.** Un `ffmpeg` falso que escribe un archivo parcial y sale
con 1:

```sh
#!/bin/sh
… (escribe 12 bytes en el archivo de salida)
exit 1
```

```console
$ go test ./internal/plugins/we/cli/ -run TestProbeThumbParcial -v
    zz_probe3_test.go:67: primer scan exit=0
    zz_probe3_test.go:71: quedó en disco v-2859c423.jpg (12 bytes) contenido="JPEG-PARCIAL"
    zz_probe3_test.go:75: segundo scan exit=0
--- PASS: TestProbeThumbParcial (0.00s)
```

El segundo scan volvió a reportar `"thumb":"…/v-2859c423.jpg"` sin volver a
llamar a ffmpeg: el archivo de 12 bytes se da por fresco.

**Impacto.** Una miniatura corrupta queda cacheada de forma permanente. El
plugin la muestra como si fuera una imagen válida. El `SPEC-we.md` promete
que "se regenera sólo si el original es más nuevo o la miniatura está vacía" —
una miniatura a medias no está vacía, así que el contrato se cumple al pie
de la letra y aun así el resultado es basura.

---

## M5 — MEDIO · `LoadManifest` acepta un `version` que no conoce

**Dónde.** `internal/plugins/we/manifest.go:63-65`:

```go
if m.Version == 0 {
    m.Version = ManifestVersion
}
```

**Síntoma.** Un manifiesto con `"version": 99` se carga **sin error**, y
`SaveManifest` lo reescribe tal cual. El campo `version` es "la versión del
contrato que lee el plugin" (`manifest.go:12`): un manifiesto de una versión
futura tiene una forma de `items` que este código no conoce, y al reescribirlo
se **pisan campos que el otro código esperaba**.

**Causa.** El chequeo sólo cubre el `0` (sin versionar). Falta un
`version > ManifestVersion → error`, que es lo que corresponde a un contrato
versionado.

**Reproducción.**

```console
$ go test ./internal/plugins/we/ -run TestProbeManifestVersion -v
    zz_probe_test.go:71: LoadManifest version=99 -> version=99 err=<nil>
--- PASS: TestProbeManifestVersion (0.00s)
```

**Impacto.** Medio: el `README.md` dice que los tags JSON del manifest "son
**el contrato con el plugin de Noctalia**: no se tocan sin coordinar con el
plugin", pero este código acepta y reescribe un contrato ajeno sin avisar.

---

## B1 — BAJO · `Slug` tira los acentos y las letras no-ASCII

**Dónde.** `internal/plugins/we/catalog.go:195` y `198-209`:

```go
var slugDrops = regexp.MustCompile(`[^a-z0-9]+`)
```

**Síntoma.** El regex reemplaza cualquier cosa que no sea `[a-z0-9]` por `-`.
Las letras acentuadas y la `ñ` **desaparecen** en vez de becomeorseguirse:
"Café Münster Ñandú" → "caf-m-nster-and". El `ñ` de "Ñandú" se va y el `ú`
también, dejando "and". El resultado no es legible ni reversible.

**Causa.** `Slug` normaliza a ASCII antes de aplicar el regex, así que todo
lo que no es ASCII se trata como separador. El README lo describe como
"kebab-case, 60 caracteres máximo" y no dice que se pierden caracteres.

**Reproducción.**

```console
$ go test ./internal/plugins/we/ -run TestProbeSlug -v
    zz_probe_test.go:75: Slug("Café Münster Ñandú") = "caf-m-nster-and"
--- PASS: TestProbeSlug (0.00s)
```

**Impacto.** Cosmético: los nombres de archivo quedan rareados para títulos
con acentos. Un `title` con sólo caracteres no-ASCII da slug vacío, que
`Slug` reemplaza por `"wallpaper"` (`catalog.go:205-207`) — sin colisión
porque el nombre final lleva el `id` adelante.

---

## B2 — BAJO · `PosterOptions.Delay` documentado como segundos, usado como frames

**Dónde.** `internal/plugins/we/poster.go:58` frente a `poster.go:23-26`.

```go
Delay      int    // segundos de espera antes de capturar     ← línea 58
…
// defaultScreenshotDelay es la cantidad de frames que espera el engine
// antes de capturar (el flag es en frames, no en segundos: a 30 fps, 30
// frames es un segundo). Abajo de eso suele salir un frame negro.   ← línea 23
```

**Síntoma.** El doc del campo dice "segundos" y el comentario de la
constante dice "frames". El valor se pasa tal cual a `--screenshot-delay`
(`poster.go:332`), que el engine cuenta en frames. El `README.md` no lo
menciona, así que el único documento del campo está mal.

**Causa.** El comentario del campo se copió de la versión en que el flag
era en segundos.

**Reproducción.** Lectura directa del código, más el test que fija el
valor por defecto:

```console
$ go test ./internal/plugins/we/ -run TestPosterOptionsNormalize -v
--- PASS: TestPosterOptionsNormalize (0.00s)
```

El test fija `Delay == defaultScreenshotDelay` (30), que son 30 frames = 1
segundo a 30 fps, no 30 segundos.

**Impacto.** Cosmético, pero el que lea el doc del campo y lo setee pensando
en segundos espera 30× más de lo que Cree.

---

## B3 — BAJO · `Summary()` no dice cuándo falta el workshop

**Dónde.** `internal/plugins/we/deps.go:163-171`:

```go
parts = append(parts, mark("engine", d.Engine.Ok))
parts = append(parts, mark("ffmpeg", d.Ffmpeg.Ok))
parts = append(parts, mark("ffprobe", d.Ffprobe.Ok))
parts = append(parts, fmt.Sprintf("workshop:%d items", d.Workshop.Items))
```

**Síntoma.** Los tres primeros bloques usan el par `ok`/`falta`. El cuarto
siempre muestra un número, así que un workshop **inexistente** y uno
**vacío** se ven igual (`workshop:0 items`). El `README.md:110` describe el
formato como "`engine:ok ffmpeg:falta …`", así que el bloque del workshop se
sale del patrón.

**Causa.** Al workshop se lo cuenta (items) en vez de marcarlo, y el `Ok`
del bloque (`deps.go:123`) no se usa.

**Reproducción.** El test que ya existe fija el caso vacío:

```console
$ go test ./internal/plugins/we/ -run TestDepsMissingYSummary -v
--- PASS: TestDepsMissingYSummary (0.00s)
```

Y `probeWorkshop` (`deps.go:105-125`) deja `Ok=false` con `Items=0` cuando
la raíz no existe, así que `Summary()` no puede distinguir los dos casos
con lo que imprime.

**Impacto.** Cosmético: `Summary()` es sólo texto para humanos. Nota: la
tabla del comando `we deps` (`cli/deps.go:71`) **sí** usa el par
`ok`/`FALTA` para el workshop, así que el problema es de `Summary()` y no
de la salida del comando.

---

## Afirmaciones del `README.md`: qué se sostiene y qué no

Cada fila es una afirmación del README (o de `doc.go`/`SPEC-we.md`, que son
el mismo contrato) con la línea que la sostiene.

| # | Afirmación | ¿Se sostiene? | Verificación |
|---|---|---|---|
| 1 | "La lógica … **no imprime nada**" | **Sí** | `grep -rn "fmt.Print" internal/plugins/we/*.go` → sin resultados (sólo `fmt.Errorf`/`Sprintf`). |
| 2 | "`we list` \| catálogo del workshop (tabla o JSON)" | **Sí** | `cli/list.go:13-88`; tabla con `tabwriter`, JSON con `json.Encoder`. |
| 3 | "`we info <id>` \| detalle de un item, con `properties`" | **Sí** | `cli/list.go:90-135`; el `Item` incluye `Properties` (`catalog.go:38`) y `info --json` lo emite. |
| 4 | "`we convert --out DIR` … `<id>-<slug>.<ext>` y `manifest.json`" | **Sí** | `catalog.go:212-214` arma `<id>-<slug>.<ext>`; `cli/convert.go:185` deja el manifest en `<out>/manifest.json`. |
| 5 | "`we poster --out DIR` … `<id>.png` y `manifest.json`" | **Sí** | `poster.go:322` → `<id>.png`; `cli/poster.go:125` → manifest. |
| 6 | "`we still --file V --out J`" | **Sí, pero pierde el destino previo** | `cli/still.go:18`; ver **A1**. |
| 7 | "`we scan --dir R` … con `--thumbs`: miniaturas JPEG" | **Sí** | `cli/scan.go:262-264` → `makeThumbs`. |
| 8 | "`we solid --color H --out P`" | **Sí, pero pisa el destino** | `cli/solid.go:115`; ver **A3**. |
| 9 | "`we deps` \| qué falta en el sistema" | **Sí** | `cli/deps.go:53-83`. |
| 10 | "`we solid --size` … se rechaza `width*height > 1<<26`" | **No** | El código es ese (`solid.go:100`) pero el overflow lo salta: **A2**. |
| 11 | "`we scan --max` … se rechaza `max > 100000`" | **Sí** | `cli/scan.go:171-173`, antes de `make([]scanItem, 0, max)`. |
| 12 | "Acepta `#rrggbb` y `#rgb` … `#f0a` → `#ff00aa`" | **Sí** | `cli/solid.go:15-29`; `TestParseHexColorAceptaLasFormasDocumentadas`. |
| 13 | "El PNG es opaco (alpha `255`), sin canal transparente" | **Sí** | `cli/solid.go:28` (`A: 255`). |
| 14 | "Tamaño default `3840x2160`" | **Sí** | `cli/solid.go:39`. |
| 15 | "Imprime por stdout la ruta del PNG" | **Sí** | `cli/solid.go:123`. |
| 16 | "El módulo **no maneja paletas**" | **Sí** | No hay `palette`/`theme` en el paquete; el color entra por `--color`. |
| 17 | "`--kind image|video`" | **No** | `cli/scan.go:151` no valida: **M3**. |
| 18 | "Extensiones: imágenes … y videos …" | **Sí** | `cli/scan.go:30-37`; coincide con `SPEC-we.md`. |
| 19 | "`--extensions` imprime la lista en JSON y sale" | **Sí** | `cli/scan.go:154-163`. |
| 20 | "El JSON trae `id`, `file`, `type`, `size`, `mod`, `name`, `title` y `thumb`" | **Sí** | `cli/scan.go:16-28` (struct `scanItem`). |
| 21 | "`title` es el nombre del archivo sin extensión con `_` y `-` como espacios" | **Sí** | `cli/scan.go:41-50` (`humanTitle`). |
| 22 | "`--thumbs DIR` … cuatro procesos en paralelo" | **Sí** | `cli/thumbs.go:45` (`workers = 4`). |
| 23 | "El nombre es `v-<djb2(ruta)>.jpg`" | **Sí** | `cli/thumbs.go:18-24`; `TestThumbNameUsaElHashAcordadoConElPlugin` fija los valores exactos. |
| 24 | "…reusado mientras el original no cambie de fecha" | **Parcial** | `fresh` (`thumbs.go:86-96`) sí mira la fecha, pero acepta un archivo a medias: **M4**. |
| 25 | "`DefaultRoot` … `~/.steam/steam/…`, `~/.local/share/Steam/…` y `~/.steam/root/…`" | **Sí** | `catalog.go:59-63`, en ese orden. |
| 26 | "Si ninguna existe devuelve un error que lista las rutas probadas" | **Sí** | `catalog.go:71`. |
| 27 | "`NormalizeType` … unifica a `video \| scene \| web \| unknown`" | **Sí** | `catalog.go:75-88`. |
| 28 | "`Filter(items, typ, query)` filtra por tipo y busca en el título o en el id" | **No** | Con tipo `unknown` o inválido no filtra: **M1**. La búsqueda por título/id sí funciona (`catalog.go:177`). |
| 29 | "`Find(items, id)` busca uno por id" | **Sí** | `catalog.go:186-193`. |
| 30 | "`Slug` … (kebab-case, 60 caracteres máximo)" | **Parcial** | El límite de 60 se aplica bien (`catalog.go:202-204`), pero pierde acentos: **B1**. |
| 31 | "`OutputName` arma `<id>-<slug>.<ext>`" | **Sí** | `catalog.go:212-214`. |
| 32 | "`Properties` conserva el bloque `general.properties` **tal cual**" | **Sí** | `catalog.go:154-165`: `map[string]json.RawMessage`, sin interpretar. |
| 33 | "**Un item roto no aborta el catálogo**: vuelve con `Err` cargado" | **Sí, salvo que falte `file`** | `catalog.go:122-128` pone `Err` si falla el `project.json`. Pero un item **con** `project.json` válido y **sin** `file` queda con `Err` vacío y `File` vacío: `probe` `TestProbeItemSinFile` → `Err vacío = true \| File vacío = true`. `doc.go:40` promete que "un item roto (sin project.json, **sin `file`**)" vuelve con `Err` cargado: **no se sostiene para el caso sin `file`**. |
| 34 | "`CheckDeps(ctx, root)` … cinco bloques" | **Sí** | `deps.go:38-44`. |
| 35 | "**Nunca falla por una dependencia ausente**" | **Sí** | `deps.go:49-57`; los que faltan quedan con `Ok=false`. |
| 36 | "`we deps` sale con 2 si falta `ffmpeg` o el workshop" | **Sí** | `cli/deps.go:80-82`. |
| 37 | "`MissingDeps()` devuelve las **obligatorias** que faltan: `ffmpeg` y `workshop`" | **Sí** | `deps.go:143-152`. |
| 38 | "`Summary()` … (`engine:ok ffmpeg:falta …`)" | **Parcial** | El formato es ese, pero el bloque del workshop no usa `ok`/`falta`: **B3**. |
| 39 | "El sondeo de versión tiene timeout de 3 s" | **Sí** | `deps.go:14` (`versionTimeout`), usado en `deps.go:93-95`. |
| 40 | "`Record` … hacen falta `grim` … y el motor, y se reporta el primer faltante" | **Sí** | `deps.go:130-138`. |
| 41 | "`Locate` … el `PATH`; después `~/.hermes/tools/ffmpeg-*/bin/`, en orden inverso para que gane la versión más nueva" | **No (el orden)** | `ffmpeg.go:81-86` ordena alfabéticamente al revés: **M2**. El resto del orden (PATH → portables → `B3L0W_FFMPEG_DIR`) sí se sostiene: `ffmpeg.go:46` → `toolDirs()`. |
| 42 | "`B3L0W_FFMPEG_DIR` … manda **sólo** ese directorio" | **Sí** | `ffmpeg.go:72-74`: si está seteado, `toolDirs` devuelve sólo ese dir y no mira las portables. |
| 43 | "Extract … **Nunca sobreescribe** sin `Overwrite`" | **Sí** | `extract.go:79-87`. |
| 44 | "…Escribe a un temporal en el mismo directorio y recién ahí renombra" | **Sí** | `extract.go:134-162` (copia) y `extract.go:175-217` (transcode). |
| 45 | "Sólo funciona para `type == \"video\"`" | **Sí** | `extract.go:42-44`, con el error que sugiere `record`. |
| 46 | "`Probe` … devuelve duración, ancho, alto y codec del primer stream de video" | **Sí** | `ffmpeg.go:195-216`. |
| 47 | "`Run` … devolviendo, si falla, un error con las últimas 20 líneas de su stderr" | **Sí** | `ffmpeg.go:25` (`stderrTail = 20`), `ffmpeg.go:114-125`. |
| 48 | "Gap conocido: `--method` se parsea y se valida, pero `convert` siempre llama a `Extract`" | **Sí, y además no se valida** | `cli/convert.go:75-80` asigna `o.method` sin validar: `--method banana` sale con 0 (`probe` `TestProbeMethodSinValidar` → `exit=0`). El README dice "se parsea y se valida"; la validación no existe. |
| 49 | "Un `Entry` por wallpaper con `id`, `title`, `type`, `status` (`ok \| skipped \| failed`), `file`, `source`, `method`, `bytes`, `duration`, `width`, `height` y `error`" | **Sí** | `manifest.go:33-46`. Nota: `duration`/`width`/`height` quedan siempre en 0 — `we.Probe` no lo llama nadie en producción (`grep 'we\.Probe'` → sólo el propio `ffmpeg.go`), así que el manifiesto nunca trae esas medidas. |
| 50 | "`SaveManifest` escribe de forma **atómica**" | **Sí** | `manifest.go:95-123`: temporal + `Sync` + `Rename`. `TestManifestSaveEsAtomico` lo fija. |
| 51 | "`LoadManifest` envuelve `os.ErrNotExist` si no está" | **Sí** | `manifest.go:53-56` con `%w`; `TestManifestLoadErrores` lo verifica. |
| 52 | "un manifest vacío o truncado es error y nunca se ignora en silencio" | **Sí, salvo la versión** | `manifest.go:57-62` da error ante vacío/truncado. Pero un `version` desconocido se acepta: **M5**. |
| 53 | "`Upsert` reemplaza el `Entry` con el mismo id o lo agrega al final" | **Sí** | `manifest.go:129-140`. |
| 54 | "`--method` … el valor nunca se lee" | **Sí** | `grep -n method internal/plugins/we/cli/convert.go` → sólo el parseo (línea 80); `o.method` no se usa nunca. |
| 55 | "Ningún proceso del motor queda vivo" | **Sí** | `poster.go:191-196` mata y cosecha; `TestPosterCancelaYNoDejaElEngineVivo` lo fija. |
| 56 | "El PNG se valida antes de aceptarlo — `pngComplete` exige firma PNG y chunk `IEND`" | **Sí** | `poster.go:200-219`. |
| 57 | "ffmpeg nunca se asume en el PATH" | **Sí** | `ffmpeg.go:45-66`. |
| 58 | "Todo comando externo tiene timeout" | **Sí** | Render 2 min (`poster.go:42`), sondeo 3 s (`deps.go:14`), thumbs 2 min (`cli/thumbs.go:40`), still 60 s (`cli/still.go:100`), convert 6 h (`cli/convert.go:134`), poster 4 h (`cli/poster.go:109`), deps 20 s (`cli/deps.go:51`). |
| 59 | "Sólo biblioteca estándar — cero dependencias externas de Go" | **Sí** | Todos los imports del paquete son de la stdlib. |
| 60 | "`we list --json \| jq` funciona sin filtrar logs" | **Sí** | `cli/list.go:66-71`: el JSON va a stdout, el conteo a stderr (`cli/list.go:86`). |
| 61 | "`b3l0w plugin-exec we` no funciona (sólo `igpu-power-menu.sh`)" | **Sí** | `ls internal/plugins/scripts/` → un solo archivo. |
| 62 | "`we convert` … `manifest.json` es `<out>/manifest.json` por defecto" | **Sí** | `cli/convert.go:183-186`. |
| 63 | "`we scan --max` … `--max` es la capacidad del slice" | **Sí** | `cli/scan.go:174`: `make([]scanItem, 0, max)`. |
| 64 | "`doc.go`: "El paquete no imprime nada"" | **Sí** | Ídem #1. |
| 65 | "`doc.go`: "Un item roto (sin project.json, sin `file`) … se devuelve con `Err` cargado" | **No** | Ver #33. |
| 66 | "`SPEC-we.md`: "`title` y `thumb` existen porque sin ellos la tarjeta mostraba la ruta absoluta"" | **Sí** | `cli/scan.go:22-28`; `TestScanEnJSONTraeLosCamposDelPlugin`. |
| 67 | "`SPEC-we.md`: "Regla de caché: se regenera sólo si el original es más nuevo o la miniatura está vacía"" | **Sí (literal), pero insuficiente** | La regla se cumple; el resultado es una miniatura corrupta cacheada: **M4**. |
| 68 | "`SPEC-we.md`: "Exit 0 si `ffmpeg` y `workshop` están OK"" | **Sí** | `cli/deps.go:80-83`. |

---

## Sin hallazgos

Se revisaron y **no** se encontró nada, con la evidencia:

- **Escrituras no atómicas de estado o caché** fuera de lo reportado: `manifest.go`
  y `extract.go` escriben a temporal y renombran (`manifest.go:95-123`,
  `extract.go:134-162`, `extract.go:175-217`); `poster.go:117-137` renderiza a
  temporal y renombra. `SaveManifest` deja permisos `0644` y no deja archivos
  sueltos (`TestManifestSaveEsAtomico`).
- **`os.RemoveAll` / `os.Rename` con nombre de afuera**: no hay ninguno en el
  módulo. `os.Rename` aparece en `extract.go:160`, `extract.go:214`,
  `manifest.go:120` y `poster.go:135`, y en todos los casos el origen es un
  temporal creado por el propio paquete y el destino está armado con
  `OutputName`/`posterTarget` (id numérico + slug saneado).
- **`exec.Command` con argumento sin validar**: los cuatro `exec.Command`
  del módulo (`ffmpeg.go:98`, `poster.go:146`, `deps.go:95`, `thumbs.go:65`)
  pasan `bin` de `Locate`/`resolveEngine` y argumentos construidos con
  `fmt.Sprintf` sobre valores ya validados. No hay shell detrás (ningún
  `sh -c`).
- **Parseo de un formato externo sin validar el caso vacío**: `LoadManifest`
  rechaza vacío (`manifest.go:57-59`), truncado (`manifest.go:60-62`) y
  version desconocida (**M5**, el único hueco). `parseProbe` rechaza JSON
  roto (`ffmpeg.go:197-199`) y devuelve error si no hay stream de video
  (`ffmpeg.go:215`); `probeNumber.UnmarshalJSON` (`ffmpeg.go:162-176`) trata
  `""`, `null`, `N/A` y `nan` como 0 y no invalida el resto.
- **Errores ignorados (`_ =`) en caminos que importan**: hay siete
  (`extract.go:164`, `catalog.go:221`, `cli/scan.go:195`, `poster.go:123`,
  `poster.go:130`, `poster.go:193`, `cli/still.go:125`). Seis son recuperables
  — `Chtimes` sobre un archivo ya copiado, el error de `WalkDir` al medir
  tamaño (dos veces), borrar un temporal (dos veces) y `Process.Kill` de un
  proceso que ya se fue. El séptimo, `cli/still.go:125`, **no** es inocuo: es el
  borrado del destino antes de extraer, y está reportado como **A1**.
- **El motor del plugin**: `poster.go:145-188` mata y cosecha siempre
  (`stopEngine`, `poster.go:191-196`), con `WaitDelay` para el caso del hijo
  que hereda stderr; `TestPosterCancelaYNoDejaElEngineVivo` lo fija.
- **`we scan` no sale de la carpeta**: `cli/scan.go:208-213` salta ocultos,
  `node_modules` y respeta `--depth`.
- **`we deps` nunca falla por una dependencia ausente**: `deps.go:49-57`
  devuelve el reporte con `Ok=false` y el CLI decide el exit code
  (`cli/deps.go:80-82`).

## No reproducidos (hipótesis, sin evidencia suficiente)

- **`we poster` y `we convert` comparten el `manifest.json` por defecto**
  (`<out>/manifest.json`) y cada uno hace `Upsert` de sus entradas. Con el
  mismo `--out`, el `poster` **agrega** el entry del poster y deja el del
  video (`probe` `TestProbeManifestClobber`: tras `convert` del id 1 y
  `poster` del id 2, el manifiesto tiene los dos). Pero `poster` pisa el
  campo `type` del entry con el tipo del wallpaper (`cli/poster.go:142`) y
  `method` con `"screenshot"`, así que un mismo id convertido **y** posteado
  queda con los datos del poster. No se pudo determinar si el plugin espera
  una entrada por wallpaper o una por artefacto: `SPEC-we.md` no lo aclara.
  Queda como pregunta abierta, no como bug.
- **`we scan` con una carpeta raíz sin permiso de lectura** sale con 0 y lista
  vacía sin avisar (`probe` `TestProbeScanRaizSinPermiso` → `exit=0`,
  `[]`). Es coherente con el comportamiento documentado de ignorar carpetas
  sin permisos (`SPEC-we.md`, "Una carpeta sin permisos no corta el
  escaneo"), así que no se reporta como bug: es una decisión, no un
  defecto.

## Sin revisar

- **`internal/plugins/we/cli/bugs.md`** (los 5 bugs de la capa de comandos ya
  arreglados) y **`internal/plugins/we/cli/tests.md`** se leyeron como
  referencia de formato y de cobertura. No se auditaron: son registros de
  arreglos ya cerrados, no parte del inventario.
- **`cmd/b3l0w-we/main.go`**: se leyó (4 líneas de wiring: `wecli.Run(os.Args[1:])`).
  Fuera del módulo.
- **`internal/plugins/luau-shared/`**: existe (`anim.luau`, `cli.luau`,
  `prefs.luau`, `store.luau`, `sync.sh`, `plugin.toml.tpl`) pero **no**
  contiene el plugin de wallpaper: `grep` de `b3l0w we`, `we scan`, `we list`,
  `video_thumb_path` y `djb2` sobre ese directorio no da resultados. El
  plugin de Noctalia que consume este CLI vive en otro repo (`b3l0w-plugins`),
  como el propio README dice. Por lo tanto **el contrato de nombres de
  miniaturas (`v-<djb2(ruta)>.jpg`) no se pudo verificar contra el código
  Luau real**: sólo se verificó que el lado Go usa djb2 de 32 bits, tal como
  dice el README.
- **`SPEC-we.md`** se leyó completo (contrato con el plugin). Las
  afirmaciones que de ahí se pudieron verificar están en la tabla de arriba
  (#66-68); el resto del archivo describe el plugin en Luau, que está fuera
  de este repo.
- **El motor real `linux-wallpaperengine`** no está instalado en esta
  máquina, así que todos los renders se probaron con un motor falso. El
  contrato de argumentos (`--bg`, `--window`, `--screenshot`,
  `--screenshot-delay`) se verificó contra lo que el motor recibe, no contra
  lo que el motor real acepta.

## Verificación

Los 12 hallazgos se **reprodujeron dos veces**: una en la primera pasada y otra
en una segunda vuelta con probes nuevos (mismo `t.TempDir()`, mismo `PATH`
falso, mismos stubs), para confirmar que cada salida citada abajo sigue
siendo la que da el código. La segunda pasada volvió a dar, entre otras
cosas:

```console
$ go test ./internal/plugins/we/ -run 'TestProbeRecheck(Traversal|Filter|Slug|ManifestVersion|ItemSinFile|ToolDirs)' -v
    Item.File   = /…/001/secret.mp4
    Item.Preview= /…/preview.png
    File fuera del dir del item: true
    Extract copió a /…/002/123-t.mp4 method=copy bytes=18
    contenido="TOP-SECRET-CONTENT"
    Filter(bogus)   -> 3 de 3
    Filter(unknown) -> 3 de 3
    Filter(video)   -> 1 de 3
    Slug("Café Münster Ñandú") = "caf-m-nster-and"
    LoadManifest version=99 err=<nil>
    Err vacío=true | File vacío=true
    toolDirs[0] = tools/ffmpeg-9.0
    toolDirs[1] = tools/ffmpeg-8.0
    toolDirs[2] = tools/ffmpeg-7.1
    toolDirs[3] = tools/ffmpeg-10.0
```

```console
$ go test ./internal/plugins/we/cli/ -run 'TestProbeRecheck' -v
    PANIC: image: NewRGBA Rectangle has huge or negative dimensions
    exit=0 | bytes=180126 | contenido="\x89PNG\r\n\x1a\n"
    exit=1 | la miniatura previa sigue en disco: false
    exit=0 (--kind videoz no es imagen ni video)
    quedó en disco v-c315d3a.jpg (12 bytes) contenido="JPEG-PARCIAL"
    exit=0 (--method banana)
```

Cierre de la ronda:

```console
$ cd /home/b3low-z3r0/Documents/Github/Cli/branches/rev-we
$ go test ./internal/plugins/we/... -count=1
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we	3.090s
ok  	github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we/cli	0.156s
$ go vet ./internal/plugins/we/...
$ go build ./...
$ git status --short
?? internal/plugins/we/bugs.md
?? internal/plugins/we/tests.md
```

Los dos verdes. El árbol tiene sólo los dos archivos nuevos: los
`zz_probe*_test.go` temporales se borraron todos.
