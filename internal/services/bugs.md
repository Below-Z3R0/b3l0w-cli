# BUGS — internal/services

Revisión de integridad sobre `internal/services` en el repo `b3l0w-cli`
(HEAD `fb76242`, rama `main`; en `internal/services/` los únicos archivos
sin trackear son los dos documentos de este encargo).

Se leyeron los 10 archivos de producción (`docker.go`, `doctor.go`,
`hardware.go`, `install.go`, `manifest.go`, `registry.go`, `report.go`,
`rollback.go`, `search.go`, `variant.go`), los 8 `*_test.go`, el `README.md`
del módulo y los puntos de integración en `cmd/b3l0w/services.go` y
`cmd/b3l0w/services_paths.go`. `variant.go` y `variant_test.go` son nuevos
(commit `ce79fed`) y se documentan como parte del módulo.

Los hallazgos marcados **verificado** se reprodujeron con archivos
temporales `zz_probe*_test.go` (package `services`, `t.TempDir()`, stub
de `docker` en el `PATH`), borrados después. Los marcados **verificar** son
análisis por inspección que no pude ejecutar de punta a punta. **No se
modificó código ni tests.**

Estado de la suite al cerrar la revisión:
`go test ./internal/services/ -count=1 -cover` → **ok** (0.054s),
**coverage: 46.9% of statements**. `go vet ./internal/services/` limpio.
Ningún test toca el Docker real: el adapter llama a `docker` por el `PATH`
y los tests ponen un stub delante. Ningún test lee el `/proc` ni el `/sys`
reales salvo `TestDetectHardwareAtNoSeComeElHost`, que solo comprueba que
no se caiga.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| C1 | CRÍTICO | `State.Save` escribe **fuera** del directorio de estado si el `id` del JSON trae `../` | `manifest.go:182` |
| A1 | ALTO | El health check acepta **cualquier** status HTTP: un 500 deja el servicio como `healthy` | `docker.go:243-254` |
| A2 | ALTO | `Update` nunca cambia el tag: guarda `Previous` == `Image`, así que el rollback posterior es un no-op | `install.go:265-267` |
| A3 | ALTO | `Install` que falla el health deja el stack **corriendo** sin estado en disco | `install.go:154-159` |
| A4 | ALTO | `SwitchVariant` no verifica el health ni el provider: deja los de la variante anterior | `variant.go:90-96` |
| A5 | ALTO | `Update` no pasa el `IMAGE` resuelto al `compose up` si `state.Tag` está vacío | `install.go:281-283` |
| M1 | MEDIO | `Update` persiste el digest **crudo** (`repo@sha256:...`) en vez del normalizado | `install.go:274-275` |
| M2 | MEDIO | `Sync` aborta con el **primer** servicio inalcanzable y tira lo ya sincronizado | `report.go:87-90` |
| M3 | MEDIO | `Registry` con `Client` nil o `BaseURL` corta hace **panic** | `registry.go:46,99` |
| M4 | BAJO | `doctor` distingue slice `nil` de slice vacía: dos criterios distintos para "no hay GPU" | `doctor.go:59` |
| M5 | MEDIO | El chequeo de disco del doctor **desaparece** si `DiskFree` falla: ni error ni aviso | `doctor.go:39` |
| M6 | MEDIO | `TagFor` en la variante `default` puede elegir un tag `buildcache-*` | `install.go:56` |
| M7 | MEDIO | `LoadState`/`Save` construyen la ruta con el `id` **del contenido del JSON**, no del archivo | `manifest.go:182,187` |
| B1 | BAJO | `State.Updated` es un campo muerto: no se lee ni se escribe | `manifest.go:170` |
| B2 | BAJO | `RegistryCacheTTL` no lo usa nadie | `report.go:14` |
| B3 | BAJO | `install_test.go` promete que el caso `buildcache` está "anotado en el README" y el README no lo anota | `install_test.go:267` |
| B4 | BAJO | `Recommend` devuelve una variante **no publicada** ("falta el build"), contra su propio comentario | `hardware.go:283,302-305` |

---

## CRÍTICO

### C1 — `State.Save` escribe fuera del directorio de estado

**Ubicación:** `manifest.go:182` (`State.Save`), `manifest.go:187`
(`LoadState`).

**Qué pasa.** `State.Save` escribe en `filepath.Join(dir, s.ID+".json")`.
El `s.ID` **viene del contenido del JSON que se acaba de leer**, no del
nombre del archivo del que se cargó. `parseManifest` valida los ids del
manifiesto con `system.SafeName` (`manifest.go:106`), pero el estado
guardado en disco no pasa por esa validación: `LoadState` lee lo que haya
en `<id>.json` y lo deserialize entero, `id` incluido.

Un archivo de estado con `{"id":"../../pwned"}` hace que el siguiente
`Save` escriba **fuera** del directorio de estado. El `id` del archivo
(`argos.json`) y el `id` dentro (`../../pwned`) son independientes: el
nombre del archivo no restringe nada.

**Reproducción (verificado).**

```go
dir := t.TempDir()
os.WriteFile(filepath.Join(dir, "argos.json"),
    []byte(`{"id":"../../pwned","image":"x"}`), 0o644)
s, _ := LoadState(dir, "argos")   // s.ID == "../../pwned"
s.Save(dir)
// → escribe <dir>/../../pwned.json, fuera del directorio
```

**Alcance.** El vector de entrada es un archivo de estado manipulado. No
es remoto: el atacante tiene que poder escribir en
`~/.local/state/b3l0w/services/`. Pero el módulo se protégés
específicamente contra el traversal de ids (`manifest.go:103-109` lo
dice: *"el ID termina en el nombre del archivo de estado: si trae `../`
el CLI escribe fuera del directorio de estado"*), y esa protección no
cubre la otra mitad del módulo.

**Fix propuesto.** Validar el `id` en el punto de uso, no solo en el
parseo del manifiesto: en `State.Save`, `if err := system.SafeName(s.ID);
err != nil { return err }`. En `LoadState`, validar el `id` recibido y
después usar **ese** id (el del argumento) al serializar, no el del
contenido — o normalizar `s.ID = id` después del unmarshal, para que
nombre de archivo y contenido no puedan divergir.

---

## ALTO

### A1 — El health check acepta cualquier status HTTP

**Ubicación:** `docker.go:243-254` (`HTTPGet`), consumido en
`install.go:171` (`verify`).

**Qué pasa.** `HTTPGet` no mira `resp.StatusCode`: si el GET no falla a
nivel de red, devuelve el cuerpo con `err == nil` sea cual sea el status.
`verify` (`install.go:171-182`) trata `err == nil` como "el servicio
está sano" y pone `state.Health = "healthy"`.

Un servicio que responde **500** queda registrado como `healthy`. Peor:
el body de un error suele traer campos con nombre propio, y
`parseProvider` (`install.go:199-216`) busca `"provider"` en cualquier
parte del texto — así que un cuerpo de error con `"provider":"CUDA"`
**inyecta el provider en el estado** de un servicio que no arrancó bien.

**Reproducción (verificado).**

```go
srv := httptest.NewServer(...)   // responde 500 con {"status":"error","provider":"CUDA"}
body, err := HTTPGet(srv.URL, 2*time.Second)
// err=<nil>  body="{\"status\":\"error\",\"provider\":\"CUDA\"}"

err = in.verify(&State{ID:"argos"}, Service{ID:"argos", Health: srv.URL}, "", nil)
// err=<nil>  Health="healthy"  Provider="CUDA"
```

**Consecuencia.** El gate de "el servicio arrancó bien" no gatea nada.
`Install` persiste `healthy`; `Update` no entra al rollback automático
(`install.go:287`) porque `verify` no falló.

**Fix propuesto.** En `HTTPGet`, si `resp.StatusCode` no está en 2xx,
devolver el status como error: `return "", fmt.Errorf("%s: HTTP %d",
url, resp.StatusCode)`. Y en `verify`, parsear el provider solo si el
status fue 2xx.

---

### A2 — `Update` nunca cambia el tag: el rollback posterior es un no-op

**Ubicación:** `install.go:265-267` (`state.Previous = state.Image`,
`state.PreviousTag = state.Tag`), `install.go:274-276`.

**Qué pasa.** `Update` calcula el `tag` local (`install.go:239-242`) y lo
usa para el pull y para el digest, pero **nunca lo escribe en
`state.Tag` ni en `state.Image`**. Las líneas 266-267 copian el valor
*anterior* a `Previous`/`PreviousTag` — y como `state.Tag` nunca
cambió, `PreviousTag == Tag` y `Previous == Image`.

El resultado es que el estado dice que hay una imagen anterior cuando en
realidad es la misma. Y el `rollback` manual posterior
(`rollback.go:80`) hace `Pull` de `state.PreviousTag`, que es el tag
actual: descarga la imagen que se acaba de actualizar, no la anterior.

**Reproducción (verificado).**

```go
state := &State{Image: "host/repo:cpu-latest", Tag: "cpu-latest", Digest: "sha256:1111..."}
in.Update(svc, state, true, true)
// tras el update:
//   Image="host/repo:cpu-latest"  Previous="host/repo:cpu-latest"
//   Tag="cpu-latest"              PreviousTag="cpu-latest"
//   ← Previous es la MISMA imagen

in.Rollback(svc, &saved)
// pulls: "host/repo:cpu-latest"  (dos veces, el mismo tag)
// Image="host/repo:cpu-latest"    ← no volvió atrás
```

**Consecuencia.** `b3l0w services rollback <id>` después de un update
no vuelve atrás: el estado queda con `previous_image` == `image`, y el
README (`README.md:60-61`) promete que `previous_*` es lo que hace
posible el rollback.

Y hay una segunda consecuencia, en la misma línea: como `state.Image`
tampoco se actualiza, la comparación de `install.go:306`
(`previous.Image != state.Image`) **nunca** da true, así que la imagen
que quedó atrás **no se borra** (`install.go:306-310`). El README
(`README.md:164`) promete que "se borra la imagen que quedó atrás" para
que la PC no acumule una capa de GiB por update.

**Nota de diseño.** El módulo decide por digest, no por tag
(`README.md:162`), y el CI publica tags mutables. Con tags mutables,
"volver atrás" **no es un pull de otro tag**: es volver al digest
anterior, que no está en ningún registry. Por eso el `rollback` manual
del README no puede funcionar tal como está escrito.

**Fix propuesto.** Guardar el digest anterior, no solo el tag:
`state.PreviousDigest = previous.Digest` antes de pisar
`state.Digest`. Y hacer que el rollback apunte al digest
(`docker pull host/repo@sha256:...`) en vez de al tag. Como
alternativa de menor alcance: que `Update` registre explícitamente
`state.PreviousDigest` y que `Rollback` avise por stderr cuando el
digest anterior y el actual son el mismo y el rollback no puede hacer
nada.

---

### A3 — `Install` que falla el health deja el stack corriendo sin estado

**Ubicación:** `install.go:154-159`.

**Qué pasa.** El orden es: `Pull` → `Up` → `verify` → `Save`. Si
`verify` falla (el health no responde en 120 s), la función devuelve
`state, err` en `install.go:155` **antes** de `state.Save`. El stack
quedó levantado en `install.go:140` (`in.Docker.Up`), pero no hay
archivo de estado.

**Reproducción (verificado).**

```go
svc := Service{ID: "argos", Image: host+"/repo", Variants: []string{"cpu"},
               Health: "http://127.0.0.1:1/readyz"}   // nadie escucha
_, err := in.Install(svc, "", false)
// err = "http://127.0.0.1:1/readyz no respondió en http://127.0.0.1:1/readyz"
// calls: pull → image inspect → compose up -d        ← el stack quedó arriba
// NO se guardó el estado: <stateDir>/argos.json no existe
```

**Consecuencia.** El servicio está corriendo y el CLI no lo sabe:
`b3l0w services list` no lo muestra, `status` dice que no está
instalado, y el `install` siguiente vuelve a intentarlo desde cero. Y
como los puertos quedaron ocupados por el stack que nadie recorded, el
`install` siguiente falla con *"el puerto 7711 está ocupado"*
(`install.go:128`) sin explicar por qué — el usuario no tiene forma de
saber que el stack es suyo.

**Fix propuesto.** Guardar el estado **antes** de `verify` (con
`Health: "starting"`, que es lo que el struct ya tiene prepared para eso
en `install.go:151`), y actualizarlo después con el resultado real. Si
`verify` falla, el estado queda en disco y el servicio aparece como
instalado-con-problemas, que es la verdad.

---

### A4 — `SwitchVariant` no verifica el health ni el provider

**Ubicación:** `variant.go:90-96`.

**Qué pasa.** `SwitchVariant` hace pull, `Up`, y actualiza el estado —
pero **no llama a `verify`**. Las líneas 90-96 pisan `Variant`, `Image`,
`Tag` y `Digest`, y **dejan intactos** `Health` y `Provider`, que
siguen siendo los de la variante anterior.

**Reproducción (verificado).**

```go
prev := &State{ID:"app", Variant:"cuda", Image: host+"/app:cuda", Tag:"cuda",
               Health:"healthy", Provider:"CUDA", Ports: []int{9101}}
prev.Save(stateDir)

SwitchVariant(VariantOptions{... Variant: "rocm", Registry: reg})
// estado en disco:
//   Variant="rocm"   ← la nueva
//   Health="healthy" ← el de la variante anterior
//   Provider="CUDA"  ← el de la variante anterior
```

**Consecuencia.** El estado miente: dice que el servicio está sano y que
usa CUDA cuando acaba de cambiar a rocm. `RenderList`
(`report.go:142-145`) imprime `state.Health` y `state.Provider` en la
columna NOTAS, así que elCLI muestra la provider equivocada. Y si el
stack nuevo no arrancó, el estado dice `healthy` igual.

Además, a diferencia de `Install` y `Update`, `SwitchVariant` no deja
rastro de una verificación fallida: no hay `Warnings` que lo digan.

**Fix propuesto.** Después del `Up`, llamar a la misma verificación que
usan `Install`/`Update` y actualizar `Health` y `Provider` con el
resultado. Si la variante nueva no responde, dejar `Health:
"unhealthy"` y un warning explícito, y **no** devolver éxito.

---

### A5 — `Update` no pasa el `IMAGE` resuelto al `compose up`

**Ubicación:** `install.go:281-283`.

**Qué pasa.** `Update` resuelve el tag en `install.go:239-242`:

```go
tag := state.Tag
if tag == "" {
    tag = LatestTag(tags, state.Variant)
}
```

Pero el env que se pasa al `Up` se arma desde `state.Tag`, no desde
`tag` (`install.go:280-283`):

```go
env := map[string]string{}
if state.Tag != "" {
    env["IMAGE"] = state.Tag
}
```

Si `state.Tag` está vacío —un estado viejo, o escrito por otra
herramienta— el `up` se lanza **sin `IMAGE`**, y compose aplica su
default (`${IMAGE:-intel-latest}`). El `pull` de la línea 270 sí fue del
tag correcto, así que la imagen correcta está descargada y el stack
arranca con otra.

**Reproducción (verificado).**

```go
state := &State{ID:"argos", Variant:"cpu", Image: host+"/repo:cpu-latest",
                Digest: "sha256:1111..."}   // sin Tag
in.Update(svc, state, true, true)
// pull: host/repo:cpu-latest        ← el tag resuelto, correcto
// up:   ... image=                   ← SIN IMAGE: compose usa su default
// estado en disco: Tag=""  Image="host/repo:cpu-latest"
```

El `Digest` en disco queda con la forma cruda del `docker image
inspect` (ver M1).

Dos problemas en uno: el env sin `IMAGE`, y que `state.Tag` sigue
vacío en disco después del update (el tag resuelto no se persiste).

**Fix propuesto.** Usar `tag` (el local) en vez de `state.Tag` al armar
el env, y asignar `state.Tag = tag` y `state.Image = svc.Image + ":" + tag`
antes de `Save`. Así el estado en disco refleja lo que realmente corre.

---

## MEDIO

### M1 — `Update` persiste el digest crudo en vez del normalizado

**Ubicación:** `install.go:274-275` (`digest, _ := in.Docker.ImageDigest(...)`,
`state.Digest = digest`), mismo patrón en `install.go:135` y
`rollback.go:90`.

**Qué pasa.** `ImageDigest` devuelve el **RepoDigest** tal cual lo da
docker: `ghcr.io/org/img@sha256:abc`. `NormalizeDigest`
(`rollback.go:18-23`) existe justamente para reducir las dos formas a
`sha256:...`, pero `Update` guarda la forma larga.

**Reproducción (verificado).** El estado en disco queda con
`Digest="host/repo@sha256:nuevo"` — la forma con el repo delante.

**Consecuencia.** El digest en disco no es comparable con el que
devuelve el registry (`sha256:...`) sin pasar por `NormalizeDigest`. Hoy
la comparación de `install.go:247-248` normaliza los dos lados, así que
el update funciona. Pero cualquier consumidor del estado que compare el
digest crudo (un script, otro comando, un `jq`) ve dos formatos
distintos para el mismo valor.

**Fix propuesto.** `state.Digest = NormalizeDigest(digest)` en los tres
sitios (`install.go:135`, `install.go:275`, `rollback.go:90`). Eso hace
que el estado en disco y el header del registry digan exactamente lo
mismo, y elimina la necesidad de normalizar en el punto de comparación.

### M2 — `Sync` aborta con el primer servicio inalcanzable

**Ubicación:** `report.go:87-90`.

**Qué pasa.** `Sync` recorre los servicios y, en el primero que falla,
devuelve `0, err` — abortando todo. Los servicios ya sincronizados
antes del fallo **no se guardan**: el `SaveManifest` de `report.go:108`
nunca llega a ejecutarse.

**Reproducción (verificado).** Con un servicio en `127.0.0.1:1`
(inalcanzable), `Sync` devuelve `changed=0` y el error del primer
servicio.

**Consecuencia.** Una sola imagen caída en el registry impide
sincronizar las demás. `CheckRegistry` (`report.go:38-42`), que hace lo
mismo, sí es tolerante: acumula el id en `st.Unreachable` y sigue. Los
dos caminos al registry no coinciden.

**Fix propuesto.** Acumular los ids inalcanzables y seguir, como hace
`CheckRegistry`. Si al menos un servicio cambió, guardar el manifiesto y
devolver `changed` con un warning aparte por los que no se pudieron
consultar. El error duro solo si **todos** fallaron.

### M3 — `Registry` con `Client` nil o `BaseURL` corta hace panic

**Ubicación:** `registry.go:46` (`r.Client.Get`), `registry.go:99`
(`r.BaseURL[len("https://"):]`).

**Qué pasa.** Dos formas de que `Registry` reviente:

1. `Registry{}` (valor cero) → `r.Client` es nil → panic en
   `r.Client.Get` (`registry.go:46`).
2. `BaseURL` más corto que 8 caracteres → `r.BaseURL[len("https://"):]`
   corta fuera de rango → panic en `registry.go:99`.

**Reproducción (verificado).**

```
Registry{}.Tags("ghcr.io/x/y")   → panic: invalid memory address or nil pointer dereference
Registry{BaseURL: "x"}.BaseURLFor("ghcr.io") → panic: slice bounds out of range [8:1]
```

**Consecuencia.** `Registry` es un struct exportado con campos
exportados: cualquier consumidor puede construirlo mal y el primer
error es un panic con stack trace, no un mensaje. `NewRegistry`
(`registry.go:20`) sí setea ambos campos — el problema es que el struct
puede construirse a mano y el código no lo valida.

**Fix propuesto.** En `NewRegistry` no alcanza: validar en el uso.
`BaseURLFor`: `if len(r.BaseURL) > len("https://") && r.BaseURL[:len("https://")] == "https://" { ... }`
en vez de cortar a ciegas. `Tags`/`Digest`: devolver
`fmt.Errorf("registry sin cliente HTTP configurado")` si `r.Client == nil`.

### M4 — El doctor distingue `nil` de slice vacía

**Ubicación:** `doctor.go:59` (`case hw.GPUs == nil:`), comparado con
`doctor.go:50` (`len(hw.GPUs) == 0`).

**Qué pasa.** El `switch` de `doctor.go:58-74` tiene tres ramas:

```go
switch {
case hw.GPUs == nil:      // ← slice nil
case hasNvidia(hw.GPUs):
    ...
default:
    if !RenderDRI() { output.Fail("/dev/dri: no existe; ...") }
}
```

En Go un `case` con el cuerpo vacío **no** cae al `default`: corta el
switch. O sea, con `hw.GPUs == nil` la rama vacía se lleva el caso y el
`default` no se ejecuta — el doctor no chequea `/dev/dri`. Con una slice
**no-nil vacía** (`[]GPU{}`) la primera condición es falsa, `hasNvidia`
también, y el `default` sí corre: el doctor chequea `/dev/dri`. El
problema es la asimetría con `doctor.go:50`, que usa `len(hw.GPUs) == 0`
para el aviso de "no hay GPU": la slice vacía no-nil recibe ese aviso
igual que la nil, pero después se la trata como si tuviera GPU.

**Reproducción (verificado).** Con `Hardware{}` (nil) el doctor no
imprime la línea de `/dev/dri`; con `Hardware{GPUs: []GPU{}}` imprime
`✓ /dev/dri: presente`. En una máquina sin `/dev/dri` el segundo caso
daría un **error** ("/dev/dri: no existe") por una máquina que
simplemente no tiene GPU.

**Alcance.** En producción el caso no se alcanza: `detectGPUsAt`
(`hardware.go:114-149`) declara `var out []GPU` (`hardware.go:119`) y
solo le hace `append` cuando encuentra una GPU real, así que sin GPUs
devuelve **nil**, nunca una slice vacía. El único `Hardware` que llega
al doctor viene de `DetectHardware()` (`cmd/b3l0w/services.go:296`).
Queda como trampa latente: si alguien cambia `detectGPUsAt` para
devolver `[]GPU{}` en vez de nil, el doctor pasa a exigir `/dev/dri` en
una máquina sin GPU.

**Fix propuesto.** Cambiar `case hw.GPUs == nil:` por
`case len(hw.GPUs) == 0:` y dejar el `default` para el caso "hay GPU
pero no es NVIDIA". Así las dos ramas usan el mismo criterio.

### M5 — El chequeo de disco del doctor desaparece si `DiskFree` falla

**Ubicación:** `doctor.go:39` (`if free, err := d.DiskFree(); err == nil {`).

**Qué pasa.** Todo el bloque de disco está dentro del `if err == nil`.
Si `DiskFree` falla (el `docker info` no responde, `statfs` da error),
no se imprime **nada**: ni el `Ok`, ni el `Warn` de disco bajo, ni un
error. El chequeo desaparece en silencio del reporte.

**Reproducción (verificado).** Con el stub de `docker` con
`STUB_FAIL=info` (el `docker info` falla, así que `DiskFree` devuelve
error), la salida del doctor es:

```
▸ Chequeos de servicios
  ✗ docker: el daemon no responde
  ✗ docker compose: falta el plugin (docker-compose)
  ! gpu: no hay GPU; los servicios usarán la variante cpu
  ✗ 2 chequeos fallaron
```

No hay ninguna línea de disco: ni el `Ok` con los GiB libres, ni el
`Warn` de disco bajo, ni un error. El bloque de `doctor.go:39-47` no
tiene rama `else`.

**Consecuencia.** El reporte no menciona el disco: no hay ni dato ni
aviso, y el resumen de abajo ("N chequeos fallaron" / "N avisos") no lo
cuenta. El README (`README.md:137-138`) explica que cada stack de
servicio consume 15-25 GiB de disco, así que el chequeo que falta es el
que avisa antes de que el espacio se vaya.

**Fix propuesto.** Agregar la rama `else` que reporte el fallo:
`output.Warn("disco: no pude medir el espacio libre")` y `res.Warnings++`.
Un chequeo que no se pudo hacer es un aviso, no un silencio.

### M6 — `TagFor` en la variante `default` puede elegir un tag `buildcache-*`

**Ubicación:** `install.go:54-60` (rama `default` de `TagFor`),
comparar con `registry.go:113` (`AvailableVariants`).

**Qué pasa.** La rama `default` de `TagFor` salta los tags `arm64-`, los
`sha-` y el `latest` — pero **no** salta los `buildcache-`. La función
`AvailableVariants` sí los salta (`registry.go:113`). Las dos funciones
no coinciden.

**Reproducción (verificado).** Con `tags = ["buildcache-3.10.4", "3.10.4"]`
y variante `default`, `TagFor` devuelve `"buildcache-3.10.4"` mientras
`AvailableVariants` devuelve `[default]`.

**Consecuencia.** El CLI instalaría la imagen de caché en lugar de la
publicada. Hoy no muerde: ningún servicio con variante `default`
publica tags `buildcache` (solo `seanime`, que solo tiene tags de
versión). Es una bomba de tiempo, no un bug activo.

El propio test lo admite (`install_test.go:264-267`): *"Ojo: esta rama
no excluye los tags buildcache-*, a diferencia de AvailableVariants. No
muerde hoy... pero queda anotado en el README"*. El README **no** lo
anota — no hay ninguna mención de `buildcache` en `README.md` (ver B3).

**Fix propuesto.** Agregar `&& !strings.HasPrefix(tag, "buildcache-")`
a la condición de `install.go:56`, alineando las dos funciones.

### M7 — `LoadState`/`Save` construyen la ruta con el `id` del contenido

**Ubicación:** `manifest.go:182` (`State.Save`), `manifest.go:187`
(`LoadState`).

**Qué pasa.** Es la otra mitad de C1, vista desde el nombre del archivo.
`LoadState(dir, id)` lee `dir/<id>.json` y devuelve el `State` con el
`id` **del JSON**, que puede no ser el `id` pedido. `Save` usa ese
`id` del JSON para escribir. Nombre de archivo y contenido pueden
divergir sin que nada lo note.

**Reproducción (verificado).** Con `argos.json` conteniendo
`{"id":"../../pwned"}`, `LoadState(dir, "argos")` devuelve un `State` con
`ID == "../../pwned"`, y `Save` escribe fuera del directorio (ver C1).

**Fix propuesto.** El mismo de C1: `s.ID = id` después del unmarshal en
`LoadState`, y validar con `system.SafeName` en `Save`. Con las dos
cosas, el `id` del archivo y el del contenido no pueden divergir y no
hay traversal posible.

---

## BAJO

### B1 — `State.Updated` es un campo muerto

`manifest.go:170`: `Updated time.Time \`json:"-"\``. No lo lee ni lo
escribe nadie en el paquete (verificado con `grep -rn "\.Updated\b"`
sobre `internal/services/`: sin resultados; los `.Updated` que aparecen
en `cmd/b3l0w/sync.go:78` y en `internal/plugins/anilist/library.go:121`
son de otros tipos, `registry.Stat` y `anilist.Record`). El tag
`json:"-"` lo excluye de la serialización, así que ni siquiera sobrevive
al round-trip. O se usa o se borra: un campo que promete un orden
temporal y nunca lo cumple envenena el debug.

### B2 — `RegistryCacheTTL` no lo usa nadie

`report.go:14`: `const RegistryCacheTTL = 6 * time.Hour`. Declarado,
documentado ("*remembers the last registry check so a stale manifest is
detected without hammering GHCR on every command*") y sin una sola
referencia en el repo (verificado con `grep -rn RegistryCacheTTL`).
El comentario además describe un comportamiento que no existe: no hay
ningún caché de registry en el módulo — `CheckRegistry` consulta GHCR en
cada `list`.

### B3 — El README no anota el caso `buildcache` que el test promete

`install_test.go:267`: *"pero queda anotado en el README"*. No lo está:
`grep -n buildcache internal/services/README.md` no devuelve nada. El
README §5 (variantes por hardware) y §7 (decisiones) no mencionan los
tags de caché en ningún lado. O se agrega la nota al README, o se
borra la promesa del test.

### B4 — `Recommend` puede devolver una variante que no existe en el registry

`hardware.go:283-284` dice: *"La variante recomendada nunca es una que no
exista en el registry: recomendar una imagen inexistente rompe la
instalación"*. Pero el paso 2 de `hardware.go:300-305` devuelve
exactamente eso:

```go
for _, v := range supported {
    if v.Supported {
        return v.Name, v.Reason + " (no publicada: falta el build)", nil
    }
}
```

El `README.md:127` repite la misma regla ("*Regla dura: la variante
recomendada nunca sea una que no exista en el registry*") mientras
documenta en las líneas 125-126 el paso que la viola: *"(2) si no está
publicada, la que el host soporta, avisando `(no publicada: falta el
build)`"*.

**Consecuencia.** El que protects el install es `ResolveVariant`
(`install.go:44-49`), que vuelve a chequear contra `published` y cae a
`published[0]`. La regla se cumple **por el consumidor**, no por la
función que dice cumplirla. `Recommend` es exportada: cualquier otro
llamador (o el `doctor`, `doctor.go:111`) recibe la variante no publicada
como si fuera válida.

No lo reproduje como bug observable en el flujo del CLI porque
`ResolveVariant` lo neutraliza — es una inconsistencia entre el
comentario y el código.

---

## Acoplamiento y testeabilidad

**Bien:**

- El paquete no usa `exec.Command` directamente salvo en los cuatro
  lugares donde no hay alternativa real (`docker.go:64` para inyectar el
  env, `docker.go:259` y `docker.go:269` para `ss` y `getent`, y
  `rollback.go:30` para el `docker ps` de `ImageInUse`). Todo lo demás
  pasa por `system.RunCombined` (`kit/system`), que tiene timeout y
  combined output.
- El `DockerAdapter` no tiene `Runner` inyectable: el stub de `docker`
  en el `PATH` es la seam de test y funciona bien (4 de los 8 archivos
  de test lo usan: `docker_env_test.go`, `install_test.go`,
  `rollback_test.go` y `variant_test.go`; los otros 4 no tocan docker).
- El hardware se detecta por rutas inyectables
  (`detectHardwareAt(cpuinfo, drmRoot)`, `hardware.go:64`): los tests
  usan `t.TempDir()` y `/proc` y `/sys` reales quedan fuera salvo un
  test explícito que solo chequea que no se caiga.
- `registry.go` separa el `token` del `tags` del `digest` y devuelve
  errores con el host y el status en el mensaje.

**Mal:**

- `report.go` tiene **0 % de cobertura**: `CheckRegistry`, `Sync`,
  `RenderList`, `RenderHardware`, `sameList` y `contains` no tienen ni
  un test. Es la capa que decide qué variante se ofrece y qué se
  considera "desactualizado" — y `CheckRegistry` es exactamente el
  camino tolerante que a `Sync` le falta (M2).
- `doctor.go` tiene **4.5 %**: `Doctor` entero no se prueba, y es una
  función de 100 líneas con entradas de error (daemon caído, compose
  v1, disco lleno, sin GPU, con NVIDIA).
- `install.go:78` (`Installer.Install`) tiene **0 %**: el camino
  completo del install —el más importante del módulo— no tiene ni un
  test. El `verify` de `install.go:164` tiene 17.6 %.
- `docker.go` tiene 27.1 %: `HaveComposeV2`, `ComposeVersion`,
  `DockerOK`, `DiskFree`, `freeSpace`, `PS`, `Down`, `Logs`, `HTTPGet`,
  `PortInUse`, `GroupExists`, `NVIDIAToolkitInstalled` y `RenderDRI` no
  se ejercitan.
- `Installer` no tiene forma de inyectar el hardware ni el health: las
  dos llamadas a `DetectHardware()` y `HTTPGet()` son globales
  (`install.go:87`, `install.go:171`). Por eso `Install` no se puede
  testear sin refactorizar. Los `Docker`/`Registry` sí son inyectables
  — la falta es inconsistente.
- `State.Save` y `LoadState` no validan el `id` (C1/M7) mientras
  `parseManifest` sí lo hace. La validación existe en un punto y falta
  en el otro.
- `report.go` mezcla inglés y español en los mismos comentarios
  (`RegistryCache remembers the last registry check...` sobre un
  paquete que por lo demás está en español). Lo mismo en `doctor.go`
  (`ComposeService`, `PS`) y en `search.go`.

---

## Lo que no encontré

- **El cálculo de la variante recomendada es correcto.** `Recommend`
  (`hardware.go:285-309`) recorre en el orden correcto: la acelerada
  que el host soporta y está publicada, después la que el host soporta
  aunque no esté publicada, después la primera publicada. La suite de
  `hardware_paths_test.go` cubre las cinco matrices de GPU más el
  fallback, y `TestRecommendPrefersPublishedAcceleration` y
  `TestRecommendFallsBackToPublished` cubren los dos caminos de la
  función.
- **La normalización del digest funciona.** `NormalizeDigest`
  (`rollback.go:18-23`) quita el prefijo `repo@` y el espacio, y
  `TestNormalizeDigest` (7 casos) más
  `TestNormalizeDigestComparaLocalYRemoto` cubren las dos formas. El
  bug de M1 es que no se usa al *guardar*, no que la función esté mal.
- **La detección de GPU es sólida.** `detectGPUsAt`
  (`hardware.go:114-149`) descarta los conectores `cardN-DP`, los
  vendors desconocidos y los `card` sin `device`, y ordena por
  ranking. `hardware_paths_test.go` arma un árbol de DRM falso y
  comprueba los tres vendors, el orden y el descarte.
- **`AvailableVariants` y `LatestTag` son consistentes entre sí.**
  `AvailableVariants` acepta la forma exacta (`tag == v`) y
  `LatestTag` también (`registry.go:168`); el comentario de
  `registry.go:163-167` explica por qué.
- **El rollback manual funciona cuando el estado es coherente.**
  `TestRollbackVuelveALaImagenAnterior` (`rollback_test.go:158`)
  comprueba el ciclo completo: pull del tag anterior, recreate con ese
  tag en el env, estado guardado con el `previous` nuevo, y la imagen
  que quedó atrás borrada. El bug de A2 es que `Update` nunca deja el
  estado en esa condición.
- **La validación de ids del manifiesto funciona.**
  `TestParseManifestRechazaLoInvalido` (`manifest_test.go:90`) cubre
  `../escape`, `a/b` y el id vacío, y el test del manifiesto embebido
  comprueba que el JSON compilado parsea.
- **`ListStates` es tolerante**: saltea los `.json` rotos, los
  subdirectorios y los que no son `.json` (`manifest.go:208-215`), y
  ordena por id. `TestListStatesIgnoraLoQueNoEsEstado` lo cubre.
- **No hay path traversal desde el manifiesto.** `parseManifest` valida
  cada id con `system.SafeName` antes de devolverlo
  (`manifest.go:105-109`), y `AvailableVariants`/`TagFor` no tocan el
  filesystem.
- **`SwitchVariant` no toca el estado en un rechazo.** Verificado: con
  un id desconocido o una variante no publicada, no llama a docker y no
  toca el archivo de estado
  (`variant_test.go:121-160`).

---

## Cómo verifiqué

Tests temporales en `internal/services/zz_probe*_test.go` (package
`services`, `t.TempDir()`, stub de `docker` en el `PATH`), ejecutados
con `go test ./internal/services/ -count=1 -run TestProbe -v` y
**borrados al terminar** (`git status --porcelain internal/services/`
solo muestra los dos `.md` de este encargo).
Cubrieron: el traversal de `State.Save` (C1/M7), el 500 tomado como
health sano y el provider inyectado desde el body de error (A1), el
`Previous == Image` y el `PreviousTag == Tag` tras un update (A2), el
install que deja el stack arriba sin guardar estado (A3), el
health/provider de la variante anterior tras un `SwitchVariant` (A4),
el `up` sin `IMAGE` cuando `state.Tag` está vacío (A5), el digest crudo
en disco (M1), el `Sync` que aborta con el primer servicio
inalcanzable (M2), los dos panics de `Registry` (M3), la diferencia
nil vs slice vacía en el doctor (M4), la línea de disco que desaparece
cuando `DiskFree` falla (M5) y el tag `buildcache` de `TagFor` (M6).

Cada fila de la tabla se verificó con `sed -n '<linea>p' <archivo>`
contra el working tree de HEAD `fb76242`. El estado final del worktree
está limpio salvo estos dos archivos.
