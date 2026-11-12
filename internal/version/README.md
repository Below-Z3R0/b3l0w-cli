# version

Guarda la versión del binario de b3l0w.

## Qué hace y por qué existe

`b3l0w version` tiene que decir qué versión del binario está corriendo. El
valor no puede estar hardcodeado en el fuente porque cambia con cada
release, y no puede calcularse en runtime porque la info no está en ningún
archivo junto al binario.

Este módulo existe para tener **un solo lugar** donde vive ese valor y que
el build lo sobreescriba. Todo el resto del código lee `version.Version`:
`cmd/b3l0w/main.go` (el comando `version`), `internal/update` (el aviso de
"hay una versión nueva") y `internal/selfupdate` (la comparación antes /
después).

## API

| Símbolo | Qué hace |
|---|---|
| `Version` | Variable `string` con la versión del binario. La pisa el build. `"dev"` en una compilación local. |

### Uso

```go
import "github.com/Below-Z3R0/b3l0w-cli/internal/version"

fmt.Printf("b3l0w %s\n", version.Version)
```

Como es una `var` y no una `const`, un test puede cambiar su valor y
restaurarlo después. `internal/selfupdate/selfupdate_test.go` hace
exactamente eso para simular una actualización.

## Cómo se inyecta en el build

Con la flag `-ldflags` del linker de Go:

```bash
go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v0.1.0" -o b3l0w ./cmd/b3l0w
```

El `PKGBUILD` de `packaging/b3l0w-cli` lo hace con el valor del tag:

```bash
CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v$pkgver" \
    -o b3l0w ./cmd/b3l0w
```

Sin `-ldflags`, el valor por defecto `"dev"` sobrevive: un binario compilado
a mano se identifica como `dev`, y el código lo detecta
(`strings.Contains(version.Version, "dev")`) para no ir a buscar una release
que no corresponde.

## Decisiones

**`-X` solo funciona sobre variables de tipo string.** `Version` es una
`var` de string, inicializada con la constante `"dev"`, así que el linker
puede pisarla en tiempo de link. Si fuera una función, o un `const`, el
linker no la tocaría y el binario diría `"dev"` para siempre.

**Un solo paquete, un solo símbolo.** No hay `BuildDate`, `Commit` ni
`Channel` en este paquete. Si alguna vez hacen falta, van en el mismo
lugar, pero hoy no hay consumidor y una variable muerta es ruido.

**El valor por defecto es `"dev"`, no `""`.** Una cadena vacía se imprime
como nada y es indistinguible de un bug. `"dev"` dice explícitamente "esto
no salió de un release".

**El módulo no se importa desde `kit/`.** Vive en `internal/` porque es
detalle de la aplicación: el `kit/` es código compartido y no debe conocer
la versión de una CLI en particular.

## Archivos

| Archivo | Qué hace |
|---|---|
| `version.go` | Todo el módulo: la variable `Version` y el comentario que explica la inyección por `-ldflags`. |

Cobertura: no hay test propio. La variable se ejercita de forma indirecta:
`internal/selfupdate/selfupdate_test.go` la sobreescribe y la restaura para
simular el cambio de versión en una actualización.

## Qué NO hace

- **No consulta la red.** No sabe si hay una versión nueva; eso es
  `internal/selfupdate`.
- **No formatea.** Devuelve el string crudo; el prefijo `"b3l0w "` lo
  pone quien imprime.
- **No compara versiones.** La comparación con la release publicada la
  hace el llamador.
- **No es un changelog.** No sabe qué cambió entre versiones.
