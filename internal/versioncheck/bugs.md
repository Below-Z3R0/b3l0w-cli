# BUGS — internal/versioncheck

Revision de integridad sobre `internal/versioncheck` (HEAD `fb76242`).
Se leyó `aur.go` (60 líneas), `aur_test.go` (48 líneas) y `README.md`.
Los hallazgos marcados **verificado** se reprodujeron con tests
temporarios en `t.TempDir()` (archivo `zz_probe_test.go`, borrado después);
los marcados **verificar** dependen de una condición que no pude confirmar
en este equipo. No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/versioncheck/ -count=1`
→ **ok** (0.005s), 2 tests verdes, cobertura **85.0 %**. `go vet` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | El RPC v5 responde `type:error` con HTTP 200: el módulo lo toma como "sin resultados" y **no hay error** | `aur.go:19-24,52-54` |
| A2 | MEDIO | Un paquete que no existe en el AUR **desaparece** de la lista sin aviso: el llamador no puede distinguir "no existe" de "consultado" | `aur.go:55-58` |
| A3 | MEDIO | No se manda `User-Agent`: el AUR puede rechazar o clasificar la petición como bot | `aur.go:41-42` |
| A4 | BAJO | `aurEndpoint` es una `var` global mutable: dos consultas concurrentes se pisan | `aur.go:27` |

---

## ALTO

### A1 — El RPC v5 reporta el error dentro del JSON, con HTTP 200

**Ubicación:** `aur.go:19-24` (la estructura `aurResponse`) y `aur.go:52-54`
(el decode).

**Qué pasa.** El RPC v5 del AUR no usa el código HTTP para señalar errores.
Cuando la petición está mal, devuelve **200** con un cuerpo de error:

```
$ curl -s "https://aur.archlinux.org/rpc/v5/info"
{"error":"No request type/data specified.","resultcount":0,"results":[],"type":"error","version":5}
```

`aurResponse` solo declara `Results`. Los campos `error`, `type` y
`resultcount` se ignoran: el decode no falla (es JSON válido), `body.Results`
queda vacío y `AUR` devuelve `([]Package{}, nil)` — **cero resultados y
ningún error**. Para el llamador, "el AUR no tiene ese paquete" y "el AUR
me dijo que no" son indistinguibles.

**Reproducción (verificado).** Con un servidor que devuelve el cuerpo de
error real del AUR:

```go
// el handler devuelve el cuerpo exacto que devuelve aur.archlinux.org
// para una petición inválida
pkgs, err := AUR([]string{"x"})
// err == nil, len(pkgs) == 0
```

El `checkDesktop` de `internal/update` (`internal/update/update.go:576-596`) recorre
`remote` y no encuentra nada: no avisa que hubo un error, simplemente no
imprime nada sobre los paquetes del escritorio. El usuario ve un `--check`
que "pasó" cuando en realidad la consulta falló.

**Fix propuesto.** Agregar el campo `Error` a `aurResponse` y tratarlo:

```go
type aurResponse struct {
    Error  string `json:"error"`
    Type   string `json:"type"`
    Result []struct {
        Name    string `json:"Name"`
        Version string `json:"Version"`
    } `json:"results"`
}
...
if body.Error != "" {
    return nil, fmt.Errorf("AUR: %s", body.Error)
}
```

Un `type` distinto de `"multiinfo"` debería además ser un error: es la
forma en que el RPC señala que la respuesta no es la esperada.

---

## MEDIO

### A2 — Un paquete inexistente desaparece de la lista sin aviso

**Ubicación:** `aur.go:55-58`.

**Qué pasa.** `AUR` mapea `body.Results` a `[]Package`. Si se piden dos
nombres y solo uno existe en el AUR, el que no existe simplemente no está
en la lista. No hay forma de que el llamador sepa que se pidió y no se
encontró.

**Reproducción (verificado).**

```go
// el servidor devuelve resultcount 1 con un solo paquete
pkgs, err := AUR([]string{"umbriel-git", "no-existe-xyz"})
// err == nil, len(pkgs) == 1  -> "no-existe-xyz" desapareció
```

El README (`README.md:37-38`) documenta el caso de `names` vacío
(`(nil, nil)`), pero no este: **pedir un paquete que no existe no es un
error, es una ausencia silenciosa**. En `internal/update`, `checkDesktop`
(`internal/update/update.go:583`) itera sobre `remote`, así que un paquete reticente del
escritorio no genera ni un warning.

**Fix propuesto.** Devolver la lista pedida con un marcador para los que no
aparecieron, o devolver un `map[string]Package` con los nombres pedidos como
claves (los ausentes con `Version == ""`). Lo mínimo: que el llamador pueda
distinguir "no existe" de "no vine".

---

### A3 — No se manda `User-Agent`

**Ubicación:** `aur.go:41-42`.

**Qué pasa.** `client.Get(endpoint)` no setea ningún header. El `User-Agent`
por defecto de Go es `Go-http-client/1.1`, que se identifica como cliente
genérico. El AUR es la misma API que usa `curl` y que documenta el uso de
un `User-Agent` identificable; un UA de bot es exactamente lo que un
servicio público filtra antes.

**Reproducción (verificado).** El header capturado por el servidor de
prueba:

```
User-Agent enviado: "Go-http-client/1.1"
```

El módulo hermano `internal/selfupdate` **sí** lo manda
(`internal/selfupdate/selfupdate.go:64`: `req.Header.Set("User-Agent", "b3l0w")`), y
`kit/releases` también (`kit/releases/signature.go:40`). La inconsistencia
dentro del propio repo es el argumento: el resto del CLI se identifica.

**Nota (verificar).** En esta máquina el AUR respondió 200 tanto con el UA
de Go como con `b3l0w`, así que hoy no hay un fallo observable. El hallazgo
es la inconsistencia y la falta de identificación, no un 403 reproducido.

**Fix propuesto.** `req, err := http.NewRequest(...)` +
`req.Header.Set("User-Agent", "b3l0w")`, igual que en `internal/selfupdate/selfupdate.go:64`.

---

## BAJO

### A4 — `aurEndpoint` es una `var` global mutable

**Ubicación:** `aur.go:27`.

**Qué pasa.** `aurEndpoint` es una variable de paquete, no una constante, para
que los tests la apunten a un `httptest.Server` (`aur_test.go:19-21`). Como
es global y mutable, dos llamadas concurrentes a `AUR` leen y escriben el
mismo string sin sincronización: una corrida de tests que change el endpoint
mientras otra consulta lee produce una URL mezclada o un data race.

El README lo reconoce (`README.md:67-70`: "como es una variable global
mutable, los tests que la cambian no pueden correr en paralelo entre sí
dentro del paquete"). Es una deuda **documentada y aceptada**, no un descuido:
se lista porque es el tipo de trampa que un `t.Parallel()` futuro
despierta.

**Fix propuesto.** Si alguna vez hace falta concurrencia, que `AUR` acepte
un `*http.Client` y un endpoint por llamada, o que el endpoint se pase por
parámetro. Alternativa mínima: un `sync.RWMutex` alrededor de la lectura.

---

## Lo que no encontré

- **El armado de la query es correcto.** `arg[]=nombre` con
  `url.QueryEscape` por paquete y unido con `&` (`aur.go:36-39`). Es
  exactamente el formato del RPC v5. Verificado contra el AUR real:
  `?arg[]=umbriel-git` responde con el paquete.
- **El `early return` de `names` vacío** (`aur.go:32-34`) devuelve
  `(nil, nil)` como documenta el README (`README.md:37-38`). No toca la
  red. Correcto.
- **El timeout de 8 s** (`aur.go:41`) está bien puesto: es una consulta
  HTTP a un servidor externo.
- **El status HTTP distinto de 200** (`aur.go:47-49`) produce un error
  con el código: `"AUR: HTTP %d"`. Correcto.
- **El `Body` se cierra siempre** (`aur.go:46`), incluso en el camino de
  error de status: el `defer` está antes del `if`.
- **El JSON malformado** (`aur.go:52-54`) sí produce error: el decoder
  falla. Ese camino está bien (el README lo reconoce en `README.md:88-90`).
- **No hay inyección de query**: los nombres se escapan con
  `url.QueryEscape` antes de concatenarlos (`aur.go:37`). Un nombre con
  `&` o `#` no puede romper la query.

## Cómo verifiqué

Tests temporales en `internal/versioncheck/zz_probe_test.go` (package
`versioncheck`), ejecutados con `go test ./internal/versioncheck/ -run
TestProbe -v` y borrados al terminar. Cubrieron: paquete inexistente en la
respuesta (A2), `User-Agent` ausente (A3), y la query con nombres
duplicados. El cuerpo de error `type:error` (A1) se comprobó contra el AUR
real con `curl` y se reprodujo con un `httptest` que devuelve ese mismo
cuerpo. El estado final del worktree no tiene archivos de prueba nuevos.
