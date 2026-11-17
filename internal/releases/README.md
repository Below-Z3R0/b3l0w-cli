# releases

Consulta el estado de firma de los releases de los programas.

## Qué hace y por qué existe

Existe para que "acordarse de firmar" no dependa de la memoria de
nadie. `b3l0w doctor` y `b3l0w update --check` lo verifican y fallan
si falta.

La regla de la distro es dura: **todo release se firma ANTES de
publicar**. Sin `SHA256SUMS.asc` no hay release — `b3l0w install` y
`b3l0w update --self` verifican la firma y se niegan a instalar
(fail-closed). Este módulo es el detector temprano: avisa antes de
que la instalación falle.

## API

| Función | Qué hace |
|---|---|
| `Check(m *config.Manifest) []Status` | Estado de firma de cada programa del manifiesto y del propio CLI |
| `Render(statuses []Status) int` | Imprime el estado y devuelve cuántos releases están sin firma |

```go
type Status struct {
    Name   string
    Repo   string
    Tag    string
    Signed bool
    Err    string
}
```

`Err` no es lo mismo que "sin firma": es que no se pudo consultar. Un
release sin firma y un release que no se pudo alcanzar son problemas
distintos y se muestran distinto.

## Uso

```go
// En doctor y en update --check.
if sinFirma := releases.Render(releases.Check(m)); sinFirma > 0 {
    output.Warn(fmt.Sprintf("%d release(s) sin firma", sinFirma))
}
```

`Render` devuelve el número de releases sin firma: eso es lo que
`doctor` suma a sus errores y lo que hace que `doctor` devuelva exit
code 1.

## Decisiones

**El CLI se chequea aparte.** No está en el manifiesto, así que `Check`
lo agrega al final con `checkCLI`. Si se publica una versión del CLI
sin firma, `doctor` tiene que verlo antes de que `update --self` falle.

**El CLI no se busca con `/releases/latest`.** El repo también publica
los paquetes del escritorio, y ese release queda como "latest" aunque
no traiga el binario. Para el CLI se usa la convención `v*`: eso hace
`selfupdate.Check`.

**Solo se miran los programas de GitHub.** Una fuente que no es
`github:owner/repo` se saltea: no hay API de releases para ella.

**Sin reintentos redundantes en este módulo.** El reintento ante 404
está en `kit/releases.HasSignature`, que sabe que un 404 del CDN
justo después de publicar es transitorio. Acá no se duplica.

**`Render` imprime el comando para arreglarlo.** Cuando un release no
está firmado, la línea siguiente dice exactamente qué correr:
`b3l0w-manteiner sign release <repo> <tag>`.

**El fallo de consulta no es un fallo de firma.** Si no se pudo
consultar, `Render` avisa con un warning y no suma al contador de
errores: no se puede afirmar que un release está sin firma si no se
llegó a verlo.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `signature.go` | Todo el módulo: `Status`, `Check`, `Render` y el helper `githubRepo` |
| `signature_test.go` | Tests del chequeo y del parseo de la fuente |

## Qué NO hace

- **No instala nada.** Solo informa el estado.
- **No verifica la firma criptográficamente.** La verificación real
  (SHA-256 + GPG) es de `kit/verify`, en el momento de instalar. Acá se
  comprueba que el asset `SHA256SUMS.asc` exista.
- **No firma.** Firmar es del mantenedor (`b3l0w-manteiner`), con la
  clave privada que nunca sale del equipo.
- **No consulta los releases de un repo que no sea GitHub.**

## Tests

2 tests: el chequeo de un manifiesto mixto (un repo firmado, uno sin
firma y una fuente no-GitHub, más el CLI propio) y el parseo de
`github:owner/repo` con sus casos inválidos (`github:owner`,
`github:/repo`, `file:///tmp`).
