# programs

Consulta, instala y actualiza los programas propios del manifiesto.

## Qué hace y por qué existe

Un "programa propio" es un binario que la distro publica en GitHub
(`boot-preview`, `kitty-shapes`) y que el CLI sabe instalar, consultar
y actualizar.

El contrato con cada programa es chico y no se negocia:

```
prog version   -> una línea: prog vX.Y.Z
prog update    -> se actualiza solo, sin preguntar, exit 0/1
prog doctor    -> chequeos, exit != 0 si algo falta
```

El core no sabe de GitHub ni de assets: ejecuta y reporta. Si el
programa respeta el contrato, el CLI no necesita saber nada de él.

## API

### Consultar

| Función | Qué hace |
|---|---|
| `Statuses(m *config.Manifest) []Status` | Estado de todos los programas del manifiesto |
| `Current(bin string) (string, error)` | Versión de un binario, ejecutando `bin version` |
| `ParseVersion(out string) string` | Saca la versión de la salida de `version` |

```go
type Status struct {
    Name      string
    Bin       string
    Version   string
    Installed bool
    Err       string
}
```

### Instalar

| Función | Qué hace |
|---|---|
| `Install(p config.Program) (string, error)` | Instala: si es paquete, con pacman; si no, desde el release |
| `InstallVerified(p config.Program, requireSignature bool) (string, error)` | Instala desde el release, exigiendo firma salvo que se pida lo contrario |
| `AssetURL(p config.Program) (string, error)` | URL del asset del último release |
| `InstallPath(name string) (string, error)` | Ruta destino: `~/.local/bin/<nombre>` |
| `VerifyRelease(assetURL, assetName, assetPath string) error` | Valida el asset contra `SHA256SUMS` + firma |

### Actualizar

| Función | Qué hace |
|---|---|
| `Update(name string) (string, bool)` | Ejecuta `prog update`; devuelve el mensaje y si salió bien |

### Desinstalar

| Función | Qué hace |
|---|---|
| `Uninstall(name string, m *config.Manifest) (string, error)` | Borra el binario y saca el programa del manifiesto |

### Relación con pacman

| Función | Qué hace |
|---|---|
| `PackageName(name string) string` | Nombre del paquete en el repo `[b3l0w]` |
| `Packaged(name string) bool` | Si el programa ya se distribuye como paquete |
| `InstallPackage(name string) (string, error)` | Instala con `pacman -S --needed` |
| `RemovePackage(name string) error` | Quita con `pacman -Rns --noconfirm` |

## Decisiones

**El contrato es la integración.** `version`, `update` y `doctor`. El
core ejecuta y reporta: no conoce el lenguaje del programa, ni su
configuración, ni sus assets.

**`ParseVersion` es la única forma de leer una versión.** La regex
acepta `"v1.2.3"` y `"1.2.3"` y devuelve siempre con `v` adelante. Es
el único regex de versión en todo el CLI.

**Un binario viejo no rompe el listado.** `Current` corre
`bin version` con timeout de 10 s. Un binario que no conoce `version`
imprime su ayuda y sale con error: mostrar `"exit status 1"` no dice
nada, así que se usa su primera línea, que sí explica qué pasó.

**`Update` no aborta.** Devuelve la salida y si salió bien; el
llamador decide si sigue con el resto. Abortar en el primero esconde
el resto y deja el sistema a medio actualizar sin saberlo.

**Un paquete no se auto-actualiza.** Si el binario tiene dueño
(`pacman -Qo`), `Update` lo dice y no lo ejecuta: eso lo hace el
sistema, no el programa.

**`Install` elige el camino: paquete o release.** Si el programa ya
está empaquetado, `pacman` manda: no hay dos formas de instalar lo
mismo. El camino del asset suelto queda para lo que todavía no está
empaquetado.

**La firma es obligatoria salvo `--insecure`.** `InstallVerified` con
`requireSignature=false` existe para releases viejos, no para el
camino normal. Sin `SHA256SUMS.asc` válido, no se instala.

**Se valida el binario antes de pisar el que está.** El asset bajado
se prueba con `version` antes de reemplazar el destino: un asset
corrupto o de otra arquitectura no reemplaza algo que funciona.

**`Uninstall` respeta al dueño.** Si el binario pertenece a un paquete
de pacman, no se borra a mano. Si el paquete es del repo propio
(`b3l0w-*`, `boot-preview`, `kitty-shapes`), `Uninstall` lo quita con
`pacman -Rns`; si es de otro repo, devuelve un error que dice que se
quita con pacman y el programa sigue en el manifiesto. Si el binario
es suelto, se borra el archivo y el programa sale del manifiesto.

**`PackageName` solo renombra el CLI.** `b3l0w` y `b3l0w-cli` son
`b3l0w-cli`; el resto de los programas ya se llaman como su paquete.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `programs.go` | `Status`, `ParseVersion`, `Current`, `Statuses`, `Update` |
| `install.go` | `AssetURL`, `InstallPath`, `Install`, `InstallVerified`, `VerifyRelease` |
| `uninstall.go` | `Uninstall` |
| `pacman.go` | `PackageName`, `Packaged`, `InstallPackage`, `RemovePackage` |
| `programs_test.go` | Tests de versión, URL de asset y ruta de instalación |
| `pacman_test.go` | Tests del nombre de paquete y de la detección de paquetes propios |
| `verify_test.go` | Tests del rechazo cuando falta la firma |
| `verify_full_test.go` | Test end-to-end de la verificación con GPG real (se salta si no hay `gpg`) |

## Qué NO hace

- **No mantiene la DB de pacman.** Solo habla con pacman; el sync es de
  `internal/apps`.
- **No compila ni construye nada.** Baja binarios ya publicados.
- **No firma releases.** Firma el mantenedor, no el CLI de usuario.
- **No gestiona la configuración de los programas.** Instalar no toca
  la config: eso lo hace el propio programa.
- **No borra un binario que tiene dueño de paquete** por la fuerza.

## Tests

12 tests: parseo de versión (con y sin `v`, con ruido alrededor),
mensaje de error de un update, armado de la URL del asset, error sin
asset y con fuente no-GitHub, ruta de instalación bajo `~/.local/bin`,
mapeo de nombres de paquete, que un programa inexistente no se tome
por paquete, la pertenencia al repo propio, el rechazo sin firma, el
rechazo con una URL que no contiene el asset, y la verificación
completa con GPG real (asset alterado → falla).
