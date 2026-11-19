# BUGS — internal/releases

Revision de integridad sobre `internal/releases` (HEAD `fb76242`).
Se leyó `signature.go` (124 líneas), `signature_test.go` (77 líneas),
`README.md`, y los dos puntos de integración: `internal/doctor/doctor.go:281-291`
y `internal/update/update.go:543`. Los hallazgos marcados **verificado** se
reprodujeron con tests temporales en `t.TempDir()` (archivo `zz_probe_test.go`,
borrado después). No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/releases/ -count=1`
→ **ok** (1.809s), 2 tests verdes, cobertura **60.0 %**. `go vet` limpio.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| M1 | MEDIO | `githubRepo` acepta `github:owner/repo/extra`: la ruta extra se cuela en la URL de la API | `signature.go:118-123` |
| M2 | MEDIO | `client` es código muerto: el paquete nunca lo usa (las llamadas HTTP van por `kit/releases`) | `signature.go:23` |
| B1 | BAJO | Todos los fallos se reportan con el mismo texto: "repo sin releases" y "red caída" son indistinguibles | `signature.go:54-59,76-79` |
| B2 | BAJO | `Render` imprime con `fmt.Printf` y no con `output.*`: se pierde el formato del CLI | `signature.go:106` |

---

## MEDIO

### M1 — `githubRepo` acepta una ruta extra y la cuela en la URL

**Ubicación:** `signature.go:118-123`.

**Qué pasa.** `githubRepo` hace:

```go
repo := strings.TrimPrefix(source, "github:")
owner, name, ok := strings.Cut(repo, "/")
if !ok || owner == "" || name == "" {
    return "", false
}
return repo, true
```

`strings.Cut` parte por el **primer** `/`. Para `"github:owner/repo/extra"`,
`owner="owner"` y `name="repo/extra"`: los dos son no vacíos, así que la
función devuelve `("owner/repo/extra", true)`. El `repo` devuelto no es
`owner/repo` sino `owner/repo/extra`, que después se concatena en la URL de
la API (`kit/releases/signature.go:35`):
`{apiBase}/repos/owner/repo/extra/releases/latest` → un 404.

**Reproducción (verificado).**

```go
githubRepo("github:owner/repo/extra")
// ("owner/repo/extra", true)   ← debería ser ("", false)
```

El error es de validación: un `Source` mal escrito no se rechaza en el
borde, se convierte en una petición a una URL que no existe. El síntoma
visible es un warning genérico ("no se pudo consultar") en vez de un error
claro de manifiesto.

**Fix propuesto.** Exigir que el repo tenga exactamente un `/`:

```go
owner, name, ok := strings.Cut(repo, "/")
if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
    return "", false
}
```

El mismo patrón está en `internal/programs/install.go:25`, que produce
`https://github.com/owner/repo/extra/releases/latest/download/a` (verificado).

---

### M2 — `client` es código muerto

**Ubicación:** `signature.go:22-23`.

**Qué pasa.** El bloque de inyectables declara:

```go
var (
    client = &http.Client{Timeout: 8 * time.Second}
    ...
)
```

Pero `client` **no se usa en ninguna parte del paquete**. Todas las
llamadas HTTP van por `kit/releases` (`releases.LatestTag`,
`releases.HasSignature`), que tiene su propio cliente
(`kit/releases/signature.go:22`). Verificado: `grep -n client
internal/releases/*.go` devuelve **una sola** línea, la declaración.

Es código muerto que además **miente**: el comentario de arriba
(`signature.go:21`) dice "Inyectables para los tests", y un lector que
quiera apuntar las peticiones HTTP a un `httptest` cambiaría `client` y no
vería ningún efecto. De hecho, el test existente inyecta por otro lado:
`kitreleases.SetBases(srv.URL, srv.URL)` (`signature_test.go:26`), que
cambia las bases del paquete `kit`.

**Fix propuesto.** Borrar `client` (y los imports de `net/http` y `time` si
quedan sin uso), o mover la inyección al paquete `kit`, que es donde están
las bases. Si la intención era que este paquete hablara directo con la
API, eso es lo que debería documentar el README.

---

## BAJO

### B1 — Todos los fallos se reportan con el mismo texto

**Ubicación:** `signature.go:54-59` y `signature.go:76-79`.

**Qué pasa.** Todos los errores de `LatestTag` y `HasSignature` van al
mismo campo `Err` (`string`) y se muestran con el mismo prefijo "no se pudo
consultar" (`signature.go:101`). Un repo renombrado, un repo sin releases,
un 403 por rate limit y una red caída son indistinguibles para el usuario.

Verificado con un servidor que devuelve 404 en todo: el estado resultante
es `err="sin releases"`, `signed=false` — el mensaje crudo de
`kit/releases`, sin contexto de qué se intentó consultar.

**Fix propuesto.** Un error tipado, o al menos prefijar el mensaje con la
causa concreta (repo inexistente / sin releases / error de red).

---

### B2 — `Render` imprime con `fmt.Printf` y no con `output.*`

**Ubicación:** `signature.go:106`.

**Qué pasa.** Todo el resto del módulo usa `output.Step`, `output.Ok`,
`output.Warn`, `output.Fail` (`signature.go:96-105`). La línea que imprime
el comando a correr (`b3l0w-manteiner sign release ...`) usa `fmt.Printf`
directo. Se pierde el formato del CLI (indentación, color, ancho) en
exactamente la línea que el usuario va a copiar y pegar.

**Fix propuesto.** Usar `output.Step` o `output.Dim` (que ya se usa en
`internal/update/update.go:397`) para esa línea.

---

## Lo que no encontré

- **La lógica del `switch` de `Render` es correcta.** El orden
  `Err != ""` → `Signed` → default hace que un error de consulta NUNCA se
  reporte como "SIN FIRMA", y que solo se cuente como error un release
  realmente sin firmar (`signature.go:99-108`). Verificado: con un 404 en
  todo, el estado sale con `Err` seteado y `Render` lo muestra como warning
  (rama `st.Err != ""`), no como fallo. Es la decisión que documenta el
  README (`README.md:73-76`).
- **`Render` devuelve 0 con lista vacía** (`signature.go:93-95`): no
  imprime sección si no hay nada. Correcto.
- **El conteo de errores** (`signature.go:107`) solo suma los releases sin
  firma, no los no consultados. Coherente con el README (`README.md:73-76`).
- **El CLI se chequea aparte y al final** (`signature.go:69`): el README lo
  justifica (`README.md:53-55`). Correcto: el CLI no está en el manifiesto.
- **`cliTag`/`cliRepo` son inyectables y restaurables**
  (`signature.go:28-29`, `signature_test.go:30-33`): el test los restaura
  con `t.Cleanup`. Correcto.
- **`Check` ignora los programas que no son de GitHub** (`signature.go:49-52`):
  el README lo dice (`README.md:62-63`). No hay release API para ellos.
- **No hay escritura en disco ni ejecución de código** en este módulo: es
  consulta pura. El riesgo de daño es cero.

## Cómo verifiqué

Tests temporales en `internal/releases/zz_probe_test.go` (package
`releases`), ejecutados con `go test ./internal/releases/ -run TestProbe -v`
y borrados al terminar. Cubrieron: `githubRepo` con ruta extra (M1), la
existencia y el uso real de `client` (M2), y `Check` contra un servidor que
devuelve 404 en todo (confirma que el `switch` de `Render` reporta el 404
como warning, no como "SIN FIRMA"). El estado final del worktree no tiene
archivos de prueba nuevos.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| M1 | `githubRepo` rechaza una `/` extra en el repo: `github:owner/repo/extra` ya no arma una URL que no existe | `TestGitHubRepoRechazaRutaExtra` |
| M2 | Borrado `client`: declaración muerta que el comentario vendía como "inyectable para los tests". Las peticiones van por `kit/releases` | verificado con `grep -rn client internal/releases/*.go` (cero en producción) |

## Arreglados (BAJO)

| ID | Arreglo | Test |
|---|---|---|
| B1 | `Check` prefija el error con el paso (`tag:` o `firma:`) para que un repo sin releases no se confunda con un 403 | código |
| B2 | La línea del comando de firma usa `output.Dim` y la indentación del CLI | código |
