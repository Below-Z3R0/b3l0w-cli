# BUGS — internal/snapshot

Revisión de integridad sobre `snapshot.go` (31 líneas) y su test (171 líneas,
con `fakeSnapper`). El módulo pide una snapshot antes de tocar configuración; no
copia archivos.

Estado de la suite al cerrar: `go test ./internal/snapshot/` → ok, cobertura
**100 %**.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Create` asume que existe la config `home` de snapper; si no existe, el error que devuelve snapper es indistinguible de un fallo real | `snapshot.go:26` |
| M1 | MEDIO | El error pierde el código de salida: solo viaja la salida combinada | `snapshot.go:26-29` |
| B1 | BAJO | El label no tiene tope de longitud ni se sanea | `snapshot.go:19-21` |

## A1 — la config `home` se asume, no se verifica

`Create` corre `snapper -c home create -d <label>` sin comprobar que la config
`home` exista. En un equipo donde solo hay config de root —el caso de este
equipo, donde `snapper -c home` no está creada— el comando falla y el módulo
devuelve el stderr de snapper envuelto. El llamador no puede distinguir "no hay
snapshots para home" de "snapper se rompió", que son dos decisiones distintas:
la primera es un aviso esperable, la segunda es un error.

Arreglo posible: `snapper -c home list` antes, o detectar el mensaje concreto
en el error. No se aplicó: es el módulo del que depende `dotfiles`, y cambiar
su contrato de error toca a ese consumidor.

## M1 — el código de salida se pierde

`system.RunCombined` devuelve `(output, err)`; el error se envuelve con la
salida pero sin el exit code. Para diagnosticar un fallo de snapper hace falta
el código (`1` es error genérico, `2` es uso incorrecto).

## B1 — el label no se acota

Un label de 10 KB se pasa tal cual a la línea de comandos. No es un vector de
ejecución (no hay shell), pero sí un argumento absurdo. `Create` ya rechaza el
vacío; el tope sería la otra mitad de la misma validación.

## Sin hallazgos en

- `Create` rechaza el label vacío **antes** de tocar el disco: la validación
  está en el orden correcto.
- `fakeSnapper` pone el binario falso primero en el PATH: el snapper real no se
  ejecuta nunca en los tests.
- No copia archivos ni hace `RemoveAll`: el módulo solo pide la snapshot.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| A1 | `Create` corre `snapper -c home list` antes del `create`: distingue "no existe la config `home`" (aviso esperable) de "snapper falló" | `TestCreateConSnapperQueFalla` (el fake ahora falla solo el `create` con `FAKE_FAIL_ON`), `TestCreateConSnapperInstalado` |

## Arreglados (BAJO)

| ID | Arreglo | Test |
|---|---|---|
| B1 | `Create` rechaza un label con saltos de línea o de más de 200 caracteres, además del vacío | `TestCreateRechazaUnLabelConSaltoDeLinea`, `TestCreateRechazaUnLabelMuyLargo` |
## Arreglados (MEDIO)

| ID | Arreglo | Test |
|---|---|---|
| M1 | El error de `snapper create` incluye el exit code (`exit 2` es uso incorrecto, `1` genérico) | código |
