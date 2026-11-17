# versioncheck

Consulta versiones publicadas en el AUR sin tocar el sistema.

## Qué hace y por qué existe

El CLI necesita saber si un programa del escritorio está más viejo que lo
publicado en el AUR. La fuente de verdad de "qué hay disponible" es el RPC
público del AUR, no una lista local ni un scrape de la web.

Este módulo existe para que esa consulta sea **de solo lectura y sin
efectos**: no invoca `pacman -Sy`, no actualiza bases de datos y no pide
confirmación. Si no hay red, devuelve error y el llamador decide si eso es
un warning o un error. La respuesta es un dato, no una acción.

## API

| Función / tipo | Qué hace |
|---|---|
| `Package` | Tipo con `Name string` y `Version string`. Una versión publicada en el AUR. |
| `AUR(names []string) ([]Package, error)` | Consulta el RPC público del AUR para los nombres dados y devuelve las versiones publicadas. |

### Uso

```go
remote, err := versioncheck.AUR([]string{"umbriel-git", "noctalia-git"})
if err != nil {
    output.Warn("escritorio: no se pudo consultar AUR")
    return
}
for _, pkg := range remote {
    local := localVersion(pkg.Name)
    // comparar local contra pkg.Version
}
```

Con `names` vacío devuelve `(nil, nil)`: no hay nada que consultar y no
hace falta tocar la red.

## El endpoint

Usa el RPC v5 de info:

```
https://aur.archlinux.org/rpc/v5/info?arg[]=umbriel-git&arg[]=noctalia-git
```

Cada nombre se agrega como `arg[]=<nombre escapado>` y los parámetros se
unen con `&`. El escape con `url.QueryEscape` no es decorativo: los nombres
de paquetes pueden llevar caracteres que romperían la query string.

La respuesta se decodifica con `encoding/json` sobre una estructura
interna `aurResponse`, que solo conserva `results` (y dentro de cada
resultado, `Name` y `Version`). El tipo `aurResponse` es privado: la API
pública expone `Package`, no el formato crudo del AUR.

## Decisiones

**Timeout de 8 segundos.** Es una consulta HTTP a un servidor externo. Sin
timeout, un AUR lento cuelga el comando que la dispara. 8 segundos es
suficiente para una respuesta normal y corta lo que se cuelgue.

**El error HTTP no es un warning del módulo.** Cualquier estado distinto de
200 devuelve `error` ("AUR: HTTP %d"). El módulo reporta; no decide si eso
es tolerable. El llamador (`internal/update`) lo degrada a warning.

**El endpoint es una variable.** `aurEndpoint` es una `var` de paquete, no
una constante, para que los tests la apunten a un `httptest.Server` local.
El tradeoff: como es una variable global mutable, los tests que la
cambian no pueden correr en paralelo entre sí dentro del paquete.

**Sin caché ni reintentos.** La consulta es barata, idempotente y de
solo lectura. Un TTL obligaría a inventar una política de expiración que
nadie pidió; un reintento transformaría un timeout de red en una espera
indefinida.

**Solo lectura.** No hay forma de instalar ni de actualizar nada desde acá.
`AUR` no llama a `pacman` ni a ningún helper del AUR.

## Archivos

| Archivo | Qué hace |
|---|---|
| `aur.go` | Todo el módulo: tipo `Package`, estructura interna de respuesta, endpoint y la función `AUR`. |
| `aur_test.go` | 2 tests: respuesta OK contra un `httptest` (2 paquetes, nombres y versiones) y error ante HTTP 500. |

Cobertura: el camino feliz (parseo del JSON y cantidad de resultados) y el
error de HTTP están cubiertos. No hay test para el JSON malformado ni para
`names` vacío: el primero lo cubre el error del decoder, el segundo es un
early return trivial.

## Qué NO hace

- **No instala ni actualiza nada.** Es consulta pura.
- **No consulta el repositorio oficial.** Solo el AUR; las versiones de los
  repos lo dice pacman.
- **No cachea.** Cada llamada es una petición nueva.
- **No reintenta** ante fallo de red ni de HTTP.
- **No valida que el paquete exista localmente.** Eso lo decide el llamador.
