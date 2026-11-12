# doctor

Chequeos de salud de la máquina: el comando `b3l0w doctor` dice si el
sistema está en condiciones de hacer su trabajo.

## Qué es

`doctor` no arregla nada. Mira y reporta. Recorre un conjunto de
chequeos y, si alguno falla, sale con código 1. Se usa como diagnóstico:
después de una actualización, de un `setup`, o cuando algo no anda.

Se invoca desde el CLI en dos lugares (verificar):

- `b3l0w doctor` — a secas.
- `b3l0w setup` — al final, después de source + plugins + backends.

## Los chequeos

Cada chequeo devuelve una lista de `Result` (nivel + mensaje) y un
contador de errores. Los avisos no cuentan para el código de salida.

| Chequeo | Qué verifica | Aviso (warning) | Error |
|---|---|---|---|
| `actualizador` | que exista `cachy-update` o `arch-update` | — | no hay ninguno de los dos |
| `pacman` | que `pacman` esté en el PATH | — | no está: no es un sistema Arch |
| `terminal` | que haya terminal interactiva | sin terminal: `b3l0w update` solo podrá actualizar programas | — |
| `programas` | el estado de cada programa del manifiesto | el manifiesto no tiene programas / el programa no está instalado | el programa falló al consultarse |
| `plugins` | que el directorio de extensiones sea privado | el directorio todavía no existe | no se pudo statear / es accesible por otros (modo `0o077`+) |
| `backends` | los backends embebidos en el binario | no se pudieron leer los scripts embebidos | — |
| `noctalia` | que Noctalia esté, con su source y `auto_update` | no está en el PATH / falta el source `b3l0w` / no se pudo leer `auto_update` | `auto_update` con un valor distinto de `"none"` |
| `dependencias` | las dependencias declaradas por los plugins | falta una dependencia / no se pudieron leer | — |
| `firmas` | que los releases tengan firma GPG | no se pudo consultar un release | release SIN FIRMA |

### Aviso vs error

- **Aviso** (`LevelWarn`): la máquina sigue sirviendo. Imprime `!` en
  stdout. No cambia el código de salida.
- **Error** (`LevelError`): algo que el doctor considera roto. Imprime
  `✗` en **stderr** y suma al contador de errores.

El código de salida es 0 si no hay ningún error, 1 si hay al menos uno.
Los avisos solos dejan el comando en 0.

## La capa `Env`

Todo lo que el doctor mira en la máquina entra por `Env`. `Env` es una
struct de funciones: cada chequeo pide lo que necesita y no toca el
sistema por su cuenta.

```go
type Env struct {
    SystemBinary  func() string
    CommandPath   func(name string) string
    IsTTY         func() bool
    ProgramStatus func(m *config.Manifest) []programs.Status
    PluginScripts func() ([]string, error)
    HasSource     func(name string) bool
    AutoUpdate    func() string
    Dependencies  func() (map[string][]string, error)
    ReleaseStatus func(m *config.Manifest) []releases.Status
    PluginsDir    func() string
    HasDependency func(name string) bool
}
```

`defaultEnv()` arma el `Env` real (la máquina de verdad). `RunWith(m,
env)` corre los chequeos contra el `Env` que le pases.

**Por qué existe:** es la costura para los tests. Con un `Env` falso la
suite corre entera sin pacman, sin Noctalia, sin red y sin tocar el
home del usuario. `Run(m)` es la API pública y no cambia: usa
`defaultEnv()`.

`Run(m)` sigue siendo el punto de entrada del CLI; `RunWith` existe
para poder testear y para correr la suite sin una máquina real. Ojo:
hoy ningún workflow de CI corre `go test` (verificar: solo hay
`desktop-packages.yml`, que arma paquetes), así que esta suite se
corre localmente.

### `Check`

```go
type Check struct {
    Name string
    Run  func() ([]Result, int)
}
```

`Run` devuelve los resultados que hay que mostrar y cuántos de ellos
son errores.

## Archivos del módulo

| Archivo | Qué tiene |
|---|---|
| `doctor.go` | los tipos (`Level`, `Result`, `Check`, `Env`), `checks()` con los nueve chequeos, `Run`, `RunWith`, `defaultEnv`, `countErrors`, `hasDependency` |
| `doctor_test.go` | los tests: cada chequeo en su estado ok, aviso y error, más los códigos de salida |
| `helpers_test.go` | helpers de test: `capturarSalida` (atrapa stdout y stderr y saca los códigos ANSI) y `contiene` |

## Tests

```bash
go test ./internal/doctor/
```

Qué cubren:

- Cada chequeo en su estado **ok**: no suma errores.
- Cada chequeo en su estado **aviso**: no cuenta como error.
- Cada chequeo en su estado **error**: cuenta como error.
- El código de salida: 0 con solo avisos, 1 con al menos un error.
- Casos borde: manifiesto `nil`, manifiesto vacío, `auto_update`
  distinto de `"none"`, dependencia ausente, directorio de plugins con
  permisos `0777`.

Ningún test toca el host: todo va por `t.TempDir()` y por `Env` falso.

## Notas

- El conteo del resumen ("N chequeos fallaron") sale inflado:
  `RunWith` suma los resultados con nivel error **y** el contador `extra`
  que devuelve el chequeo, así que un solo fallo se cuenta dos veces
  (verificar: no hay test que fije el número exacto). El código de
  salida no se ve afectado.
- `releases.Render` imprime su propia sección ("Firmas de releases") y
  devuelve cuántos releases están sin firma; el chequeo `firmas` no
  devuelve `Result`, solo ese número.
