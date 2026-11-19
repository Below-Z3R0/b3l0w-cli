# TESTS — internal/apps

Qué prueba cada test del módulo, qué queda sin cubierto y por qué.

**Regla dura del módulo:** ningún test ejecuta pacman. Las funciones
puras de clasificación se prueban directamente; `Run()` y `exec()` (que sí
invocan pacman) no tienen test.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Archivos de test | 1 (`apps_test.go`) |
| Tests | 5 |
| Verdes | 5 |
| **Rojos** | **0** |
| Saltados | 0 (el de la DB real puede saltear si la máquina no tiene pacman) |
| Cobertura | **41.9 %** de las sentencias |

Medido con `go test ./internal/apps/ -count=1 -cover` → `ok ... coverage:
41.9 % of statements`.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `apps_test.go` | Las tres clasificaciones de banderas (`partialUpgrade`, `instala`, `needsRoot`), el formato de la edad y la lectura de la DB real de pacman |

---

## Qué prueba cada test

### `apps_test.go`

| Test | Qué fija |
|---|---|
| `TestPartialUpgrade` | `partialUpgrade()` sobre 8 banderas: `-Sy` y `-Syy` son partial upgrade (se rechazan); `-Syu` y `-Suy` no lo son (sincronizan **y** actualizan); `-S`, `-Ss`, `-Q` y `-Syu ` (con espacio final) no lo son. Es el núcleo de la regla que el módulo existe para aplicar |
| `TestInstala` | `instala()` sobre 6 banderas: `-S` y `-Syu` instalan; `-Ss` (busca), `-Si` (info), `-Q` (consulta) y `-R` no. Define qué/freno de DB vieja se aplica |
| `TestNeedsRoot` | `needsRoot()` sobre 8 banderas: `-S`, `-Syu`, `-R`, `-Rns` y `-U` piden sudo; `-Ss`, `-Si` y `-Q` no. Es la regla de "un `-Ss` no debería costar un password" (README.md:67-68) |
| `TestHumanizar` | `humanizar()` sobre 4 duraciones: 3 h → "3 h", 26 h → "1 día", 50 h → "2 días", 10 días → "10 días". Fija el texto que ve el usuario en el mensaje de DB vieja |
| `TestOldestSyncDB` | `OldestSyncDB()` contra las DB **reales** de la máquina: si hay pacman, devuelve una DB y una edad no negativa. **Se salta** si la máquina no tiene DB de pacman (`apps_test.go:81-83`), así que en un contenedor el test es verde sin probar nada |

---

## Cobertura por función

`go tool cover -func` sobre `-coverprofile`:

| Función | Cobertura | Nota |
|---|---|---|
| `instala` | 100 % | |
| `needsRoot` | 100 % | |
| `humanizar` | 100 % | |
| `OldestSyncDB` | 80 % | falta el caso "el glob no devuelve nada" y el de "todos los Stat fallan" |
| `partialUpgrade` | 75 % | falta el caso "el flag no empieza con `-`" (`apps.go:92-94`) |
| **`Run`** | **0 %** | la función pública del módulo |
| **`exec`** | **0 %** | la que invoca pacman |
| **`actualiza`** | **0 %** | usada por `Run` pero no testeada directamente |

El 41.9 % de cobertura es engañoso: `Run` y `exec` —la mitad del módulo en
líneas— están sin cubrir, y las funciones cubiertas son las **auxiliares**.
La lógica que decide qué hace el usuario (qué se rechaza, qué pide sudo) está
enteramente en las funciones puras, y esas sí están bien cubiertas.

---

## Sin cubrir

| Caso | Por qué no está |
|---|---|
| **`Run()` completo** | Es la API pública (README.md:22) y tiene 0 %. Es testeable, pero requiere un seam: hoy llama a `exec()` → `system.Interactive` → pacman de verdad. Con un `exec` inyectable (o una variable de paquete con el nombre del binario) se podría probar toda la lógica de decisión sin tocar pacman. Es la deuda de testeabilidad más grande del módulo |
| **`exec()`** | Mismo motivo. Además debería tener un test que fije la propagación del exit code (BUGS A2): hoy `exec` devuelve 1 para cualquier fallo |
| **`actualiza()`** | Se usa en `Run` (`apps.go:59`) pero no tiene test directo. `TestPartialUpgrade` cubre `-Syu` de rebote, pero `actualiza` no está verificada por separado |
| **`partialUpgrade` con un flag que no empieza con `-`** | `apps.go:92-94` devuelve `false` para un argumento que no es flag (por ejemplo `b3l0w apps firefox`). No está en la tabla de `TestPartialUpgrade` |
| **`instala()` con `-S` y letras combinadas** | La tabla de `TestInstala` tiene `-S`, `-Ss`, `-Si`, `-Syu`, `-Q`, `-R`. Faltan las combinaciones rarer: `-Syy`, `-Suy`, `-Su`, `-Qu`, `-Sp`. Con la lógica por substrings (BUGS A3, A4) cada combinación es un caso distinto |
| **`needsRoot()` con `-U`** | Está en la tabla ✓, pero falta `-D` (debase) y `-Qq`, que tampoco piden sudo |
| **El freno de DB vieja en `Run`** | La condición `instala(flag) && !allowStale && !actualiza(flag)` (`apps.go:59`) es la política central del módulo (README.md:48-52) y no tiene ningún test: no hay forma de comprobar que `-Syu` **no** frena y que `-S` **sí**. Es el hueco más importante del módulo |
| **`--allow-stale` como escape** | El flag se filtra en `apps.go:33-39` y README.md:41-42 lo documenta, pero no hay test de que llegue a `pacman` sin el flag ni de que se quite antes |
| **La traducción de `-R` → `-Rns`** | `apps.go:70-72`. Con `Run` sin seam no hay test. Se podría extraer a una función pura `traducir(args []string) []string` y testearla aparte |
| **La DB real de pacman** | `TestOldestSyncDB` lee `/var/lib/pacman/sync/*.db` de verdad. Es el único test que depende del estado de la máquina: en un contenedor sin pacman **se salta** y no prueba nada. Con un directorio inyectable se podría testear con DBs falsas (una vieja, una nueva) |
| **Qué DB es la más vieja** | El test solo comprueba que devuelve un nombre no vacío y una edad ≥ 0. No verifica que elija la más vieja entre varias |
| **Un `pacman` que no existe** | `exec` con un binario inexistente: el error de `Interactive` no es un `ExitError` y `exec` devuelve 1 igual (BUGS A2). Sin seam no hay test |
| **`sudo` fallando** | Con `needsRoot` verdadero, `system.Interactive("sudo", "pacman", ...)` puede fallar por sudo no instalado o contraseña incorrecta. No hay test |
| **Concurrencia** | El módulo no tiene estado propio: no aplica |
| **Windows** | No hay código específico de plataforma. `OldestSyncDB` mira un path de Arch Linux, así que en Windows siempre falla: es correcto que el módulo sea de Arch |

---

## Bugs de esta revisión

Los 8 hallazgos de `bugs.md` **no tienen test**. El paquete no tiene tests
rojos: `go test ./internal/apps/` pasa entero.

El hallazgo **A1** (el freno de partial upgrade se esquiva con un flag global
adelante) es el más grave y el más fácil de cubrir: la decisión es de
`partialUpgrade(flag)` sobre un string, así que un test de tabla con
`["--noconfirm", "-Sy"]` lo demuestra sin ejecutar pacman. Lo que falta para
que sea un test **rojo** (y no solo una documentación del bug) es refactorizar
`Run` para que la clasificación de la operación se pueda pedir por separado.

| Bug | Test que lo cubriría |
|---|---|
| A1 (flag global adelante) | Tabla de `partialUpgrade`/`instala`/`needsRoot` con el flag de operación **al final** del vector |
| A2 (exit code) | `exec` con un binario falso que sale con 42: el wrapper debe devolver 42 |
| A3 (substrings) | Tabla con las combinaciones `-Syy`, `-Suy`, `-Su`, `-Sy` en distintos órdenes |
| A4 (orden de letras) | `actualiza("-uSy")` debe dar true |
| A5 (glob de DBs) | `OldestSyncDB` con un directorio inyectable que tenga una DB de la distro y una de un repo |
| A6 (Stat ignorado) | `OldestSyncDB` con una DB que no se puede leer |
| B1 (humanizar) | `humanizar(30*time.Minute)` no debería decir "0 h" |

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/apps/ -count=1

# Con cobertura
go test ./internal/apps/ -count=1 -cover

# Con el detector de carreras
go test ./internal/apps/ -race -count=1

# Ver qué función está sin cubrir
go test ./internal/apps/ -count=1 -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out
```

**Hermeticidad:** la suite no ejecuta pacman. El único test que lee el
estado de la máquina es `TestOldestSyncDB`, que lee
`/var/lib/pacman/sync/*.db` y **se salta** si no hay (README.md:96 lo
confirma). En una máquina con pacman, ese test depende de que haya al menos
una DB.
