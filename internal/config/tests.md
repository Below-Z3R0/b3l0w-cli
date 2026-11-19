# TESTS — internal/config

Qué prueba cada test del módulo, qué queda sin cubrir y por qué.

**Regla dura del módulo:** ningún test toca `~/.config/b3l0w/programs.json`
real. Todo vive en `t.TempDir()`.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Archivos de test | 1 (`manifest_test.go`) |
| Tests | 4 |
| Verdes | 4 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **84.4 %** de las sentencias |

Medido con `go test ./internal/config/ -count=1 -cover` → `ok ... coverage:
84.4 % of statements`.

Es la mejor cobertura de los cinco módulos de esta revisión: el módulo es
chico, sin estado global y sin dependencias externas.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `manifest_test.go` | El default cuando no hay archivo, el round-trip de save/load, la idempotencia de `Add` y el comportamiento de `Remove` |

---

## Qué prueba cada test

### `manifest_test.go`

| Test | Qué fija |
|---|---|
| `TestLoadSinArchivoDevuelveDefault` | `Load()` de un path que no existe devuelve el **default** con error `nil`: trae programas (`boot-preview` y `kitty-shapes` están, se comprueba uno por uno con `Get`). Es la regla del README (README.md:74-76) y la que hace que el CLI funcione recién instalado |
| `TestSaveLoadRoundtrip` | `Save()` + `Load()` de ida y vuelta: el archivo se crea (en un subdirectorio que **tampoco** existe, para verificar el `MkdirAll`), la lista de programas conserva la longitud, y el programa agregado con `Add` sigue ahí después de releer |
| `TestAddEsIdempotente` | `Add()` de un nombre existente devuelve `false`; de uno nuevo devuelve `true`; de uno ya agregado devuelve `false` la segunda vez. Es el patrón "agregá y avisá si algo cambió" del README (README.md:78-81) |
| `TestRemove` | `Remove()` de un programa existente devuelve `true` y el programa desaparece (`Get` devuelve `nil`); `Remove()` de uno que ya no está devuelve `false` |

---

## Cobertura por función

`go tool cover -func` sobre `-coverprofile`:

| Función | Cobertura | Nota |
|---|---|---|
| `Default` | 100 % | |
| `Get` | 100 % | |
| `Add` | 100 % | |
| `AddProgram` | 100 % | |
| `Remove` | 100 % | |
| `Load` | 72.7 % | falta el JSON corrupto, el `version` ausente y el `version` futura |
| `Save` | 66.7 % | falta el error de `MkdirAll` y el de `MarshalIndent` |

Las dos funciones sin cubrir al 100 % son `Load` y `Save`, y sus huecos son
justo los bugs del módulo (ver más abajo).

---

## Sin cubrir

| Caso | Por qué no está |
|---|---|
| **Un manifiesto con JSON inválido** | `Load()` debería devolver el error de parseo (`manifest.go:55-57`). No hay test. Es el caso más importante de todos: es el que le dice al usuario "tu archivo está roto" |
| **Un manifiesto con `version` ausente** | `Load()` tiene que poner `Schema` (`manifest.go:58-59`). No hay test: hoy el default y el archivo escrito sí llevan `version`, así que el camino no se ejercita |
| **Un manifiesto con `version` futura (99)** | `Load()` **no debería** aceptarlo (BUGS A1). No hay test, y el bug está precisely ahí: hoy lo acepta en silencio |
| **Un manifiesto `null`, `{}` o `{"programs": null}`** | `Load()` devuelve un manifiesto vacío sin error (BUGS A2). No hay test. Es el caso que puede hacer perder la lista de programas del usuario |
| **Un manifiesto con `programs: []`** | El caso "lista vacía pero explícita" es distinto de "sin la lista". No está testeado |
| **`Save()` fallando** | Ni el `MkdirAll` (un path cuyo padre es un archivo) ni el `WriteFile` (permisos). `Save` no es atómico (BUGS A4): tampoco hay test de que un manifiesto truncado sea recuperable |
| **`Get()` con un nombre que no existe** | Lo cubre `TestRemove` de rebote (`Get` después de borrar devuelve `nil`), pero no hay un caso explícito de "un nombre que nunca existió" |
| **`Get()` y el puntero que caduca** | El bug A3 (un `Remove` posterior deja el puntero apuntando a otro programa) **no** se puede fijar con la API actual sin escribir el test en el orden exacto: obtener el puntero, remover otro, y mirar qué pasó. Es el test que documentaría el bug |
| **`Add()` con nombre vacío o fuente inválida** | `AddProgram` no valida nada (BUGS B1). No hay test de que un nombre vacío se rechace |
| **`AddProgram()` con el `Asset` puesto** | `Add()` (sin asset) sí está probado; `AddProgram()` (con asset) es lo que usa el CLI real (`cmd/b3l0w/programs.go:54`) y solo se ejercita de rebote |
| **`Remove()` sobre el primer y el último elemento** | `TestRemove` borra el segundo de dos. Los casos de los extremos (y el de un slice de uno solo) tocan el `append` de compactación de forma distinta (BUGS A5) |
| **Round-trip con un manifiesto grande** | No hay test de que el orden y el contenido se preserven con más de dos programas. `TestSaveLoadRoundtrip` usa dos |
| **Concurrencia** | El módulo no tiene estado global: no aplica. Dos procesos editando el mismo manifiesto sí pueden pisarse, pero eso es del filesystem, no del módulo |
| **Windows** | `filepath.Dir` y `MkdirAll` son portables. No hay cobertura en Windows, pero tampoco hay código específico |

---

## Bugs de esta revisión

Los 6 hallazgos de `bugs.md` **no tienen test**. El paquete no tiene tests
rojos: `go test ./internal/config/` pasa entero.

Todos los bugs de este módulo son **de `Load`**, y `Load` es justamente la
función con la cobertura más baja (72.7 %). Los tres que más importan:

| Bug | Test que lo cubriría |
|---|---|
| A1 (versión futura aceptada) | `Load` de `{"version":99,...}` debe fallar, no devolver `version=99` |
| A2 (manifiesto semiválido → vacío) | `Load` de `null` / `{}` / `{"programs":null}` debe fallar, no devolver una lista vacía |
| A3 (puntero que caduca) | Obtener el puntero, hacer `Remove` de otro, y comprobar que el puntero sigue apuntando a su programa (o que la API cambió) |

Y los dos de `Save`:

| Bug | Test que lo cubriría |
|---|---|
| A4 (guardado no atómico) | Escribir un manifiesto, simular un corte, y comprobar que el archivo old sigue siendo válido |
| A5 (slice sin compactar) | Tras un `Remove`, comprobar que la cola del array subyacente quedó vaciada |

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/config/ -count=1

# Con cobertura
go test ./internal/config/ -count=1 -cover

# Con el detector de carreras
go test ./internal/config/ -race -count=1

# Ver qué función está sin cubrir
go test ./internal/config/ -count=1 -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out
```

**Hermeticidad:** la suite no toca el manifiesto real. Todos los paths son de
`t.TempDir()`, incluido el subdirectorio que `Save` tiene que crear. Corre
offline y en cualquier máquina.
