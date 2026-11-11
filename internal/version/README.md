# internal/version

Guarda la versión del binario.

## Qué es

Una variable, no una función:

```go
var Version = "dev"
```

## Por qué una variable y no una función

El linker (`-ldflags -X`) solo puede pisar **variables de tipo string**, no
el resultado de una función. Si esto fuera:

```go
func Version() string { return "dev" }
```

el `-ldflags` apuntaría a un símbolo inexistente, el build seguiría
compilando sin quejarse, y el binario diría `"dev"` **para siempre**. El
fallo sería silencioso: ningún error, ningún warning, solo una versión
equivocada en cada `b3l0w version`.

Por eso es una variable.

## Cómo la pisa el build

```bash
go build -trimpath \
  -ldflags "-s -w -X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v$pkgver" \
  -o b3l0w ./cmd/b3l0w
```

La ruta del `-ldflags` tiene que ser `<módulo>/<paquete>.<variable>`:

| Parte | Valor |
|---|---|
| módulo | `github.com/Below-Z3R0/b3l0w-cli` |
| paquete | `internal/version` → `version` |
| variable | `Version` |

Si el archivo se mueve de `internal/version/` a otro lado, la ruta cambia y
el `-ldflags` deja de encontrar el símbolo. El build **no falla**: compila
igual y la versión queda en `"dev"`.

## Dónde se usa

- `b3l0w version` — muestra la versión del binario.
- `internal/versioncheck` — compara la versión local contra la remota.
- El PKGBUILD de `packaging/b3l0w-cli/PKGBUILD` inyecta `$pkgver`.

## Test

`version_test.go` verifica que la variable no esté vacía. Es una red de
seguridad mínima: el valor real solo existe después del link.

Para comprobar que la inyección funciona de verdad:

```bash
go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v9.9.9" \
  -o /tmp/b3l0w-test ./cmd/b3l0w
/tmp/b3l0w-test version
# debe decir v9.9.9
```

## Nota sobre tests que pegan a la red

`internal/systemdef` tiene un test que consulta GitHub. Se saltea con
`-short`:

```bash
go test ./... -short   # suite offline, la que corre en CI
go test ./...          # suite completa, incluye los que necesitan red
```

Si un test nuevo necesita internet, seguí el mismo patrón: `testing.Short()`
y un `t.Skip` con el motivo.
