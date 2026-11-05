# Contribuir

## Compilar y probar

```bash
gofmt -w .
go build ./...
go vet ./...
go test -race -count=1 ./...
bash scripts/smoke-test.sh ./b3l0w
```

La versión se inyecta al compilar:

```bash
go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v0.1.0" -o b3l0w ./cmd/b3l0w
```

## Reglas del proyecto

- **Sin dependencias.** Solo la biblioteca estándar. El binario tiene que
  compilar en cualquier máquina sin bajar nada, porque es lo que actualiza
  el sistema.
- **El core delega.** No implementa updates de programas ni de pacman:
  ejecuta el `update` de cada uno. Una segunda implementación es una
  segunda fuente de verdad que se separa en el primer release.
- **Un archivo, un tema**, y se parte al pasar ~230 líneas.
- **El manifiesto es JSON** (`~/.config/b3l0w/programs.json`): se escribe
  solo y se puede editar a mano, sin dependencias para parsear TOML.
- **Las extensiones no se buscan en el PATH.** Solo en
  `~/.local/lib/b3l0w/plugins/` (0700): este CLI pide sudo.
- **No se renombra ni se reemplaza un binario de un paquete.**
  `cachy-update` es un symlink a `arch-update` y pertenece a su paquete.
