# TESTS — internal/doctor

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 21 |
| Cobertura | 97.5 % |
| Verdes | 21 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `doctor_test.go` | los nueve chequeos uno por uno, el conteo de `runChecks`, y `RunWith` con manifiestos y env falsos |
| `helpers_test.go` | el `Env` de prueba: funciones falsas para cada chequeo y el capturador de salida |

### Detalle

`doctor_test.go` tiene tres bloques:

1. **Estructura**: `TestDefaultEnvEstaCompleto` recorre el `Env` por reflexión y
   falla si algún campo quedó nil; `TestChecksCubrenTodosLosChequeos` verifica
   que no falte ninguno.
2. **Chequeo por chequeo**: un test por cada uno (actualizador, pacman, terminal,
   programas, plugins, backends, noctalia, dependencias, firmas), con el caso ok
   y el caso de fallo.
3. **Integración**: `runChecks` con checks sintéticos y `RunWith` con manifiestos
   vacíos, nil, con un programa y con auto-update distinto de `none`.

`TestRunChecksCuentaCadaFalloUnaVez` es el que cubre el bug C1: seis casos,
incluido "dos errores del mismo chequeo" que debe dar 2 y no 4.

`helpers_test.go` define el `Env` falso y `capturarSalida`, que redirige stdout
y stderr a buffers.

## Sin cubrir

| Qué | Por qué |
|---|---|
| `hasDependency` contra pacman real | El test lo ejecuta de verdad (es el hallazgo M1 de `bugs.md`). |
| El chequeo de firmas contra GitHub | Se prueba con un `Env` falso; la consulta real no se hace en tests. |
| `defaultEnv` en todas sus ramas | Se verifica que esté completo, no que cada función haga lo suyo. |
