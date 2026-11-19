# BUGS — internal/doctor

Revisión de integridad sobre `doctor.go` (301 líneas) y los 2 archivos de test.
El módulo corre los chequeos de "¿esta máquina puede usar el sistema?".

Estado de la suite al cerrar: `go test ./internal/doctor/` → ok, cobertura
**97.5 %**.

## Resumen

| ID | Sev | Qué | Línea | Estado |
|---|---|---|---|---|
| C1 | CRÍTICO | `RunWith` contaba cada fallo **dos veces**: sumaba el `LevelError` del `Result` y además el segundo valor de `Check.Run`, que en seis chequeos era el mismo número | `doctor.go:112-126` | **arreglado** |
| M1 | MEDIO | `hasDependency` llama a `exec.Command("pacman", "-Q")` directo, fuera del `Env` inyectable | `doctor.go:299` | abierto |
| B1 | BAJO | El chequeo de firmas devuelve `nil` resultados y cuenta en el segundo valor: el único que usa ese contrato | `doctor.go:288-290` | por diseño |

## C1 — el conteo mentía (arreglado)

`RunWith` hacía:

```go
for _, r := range resultados {
    case LevelError:
        errores++
}
errores += extra
```

Los seis chequeos que devolvían `extra != 0` ya traían un `Result` con
`LevelError`, así que el mismo fallo sumaba dos. El exit code era correcto (1);
lo que mentía era el número: "2 chequeos fallaron" con un solo chequeo roto.

Arreglado en el PR #23: los chequeos devuelven `extra=0` cuando el error ya va
en el `Result`, `countErrors` se eliminó, y el conteo quedó en `runChecks` con
`TestRunChecksCuentaCadaFalloUnaVez`, que cubre seis casos (uno falla si el
conteo vuelve a duplicar).

El único `extra` legítimo que queda es el del chequeo de firmas, que devuelve
`nil` resultados y cuenta en el segundo valor.

## M1 — `hasDependency` fuera de la costura

El módulo tiene un `Env` con funciones inyectables para que los tests corran sin
tocar la máquina. `hasDependency` es una función libre que llama a
`exec.Command("pacman", "-Q", name)` directo: los tests que la ejercitan corren
el pacman real. No rompe nada, pero es la excepción a la regla del paquete.

## Por diseño

El chequeo de firmas imprime su propia sección y devuelve cuántos releases
están sin firma en el segundo valor, sin `Result`. Es el contrato que usa
`runChecks` para el `extra`.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| C1 | Arreglado en `ac95ce8` (PR #23): `runChecks` cuenta cada fallo una sola vez; los 4 chequeos con `LevelError` + `extra=1` y los 2 con `countErrors` quedaron con `extra=0` | `TestRunChecksCuentaCadaFalloUnaVez` |
| M1 | `hasDependency` deja de ser una función libre y pasa a `Env.disponible` con `PackageInstalled` inyectable: la consulta a pacman (-Q) se puede simular sin correr el pacman real | `TestHasDependency` |

## Arreglados (BAJO)

| ID | Arreglo | Test |
|---|---|---|
| B1 | Descartado por diseño: el chequeo de firmas devuelve `nil` y cuenta en el segundo valor porque no produce `Result` | `TestRunChecksCuentaCadaFalloUnaVez` |
