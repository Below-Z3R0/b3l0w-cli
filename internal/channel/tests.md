# TESTS — internal/channel

Qué prueba cada test del módulo, qué queda sin cubierto y por qué.

**Regla dura del módulo:** los tests aíslan el home con `t.Setenv("HOME", ...)`,
así que nunca leen ni escriben el canal real del usuario.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Archivos de test | 1 (`channel_test.go`) |
| Tests | 2 |
| Verdes | 2 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **60.0 %** de las sentencias |

Medido con `go test ./internal/channel/ -count=1 -cover` → `ok ... coverage:
60.0 % of statements`.

La cobertura es "60 %" pero el número engaña: las dos funciones con más
lógica (`Current` y `Set`) están cubiertas, y lo que falta es `Detect()` y su
helper `installed()`, que dependen de pacman.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `channel_test.go` | `Set()` + `Current()` de ida y vuelta entre los dos canales, con `HOME` temporal; y el rechazo de un canal inválido |

---

## Qué prueba cada test

### `channel_test.go`

| Test | Qué fija |
|---|---|
| `TestSetAndCurrent` | `Set(Edge)` escribe el canal y `Current()` lo devuelve; después `Set(Stable)` y `Current()` devuelve `stable`. Es la ida y vuelta completa, con `t.Setenv("HOME", t.TempDir())` (`channel_test.go:6`) para que el archivo vaya a un home temporal y no al real. Fija que el archivo manda sobre la detección |
| `TestSetInvalid` | `Set("nightly")` devuelve **error** y no escribe nada. Es la validación del README (README.md:59-61): un canal que no sea `stable` ni `edge` es un error, no garbage en el archivo. También con `HOME` temporal (`channel_test.go:23`) |

---

## Cobertura por función

`go tool cover -func` sobre `-coverprofile`:

| Función | Cobertura | Nota |
|---|---|---|
| `Path` | 75 % | falta el caso "no hay home" (`channel.go:20-22`) |
| `Current` | 75 % | falta el caso "el archivo existe pero tiene basura" (BUGS A1) y el "no hay archivo" |
| `Set` | 75 % | falta el error de `MkdirAll`/`WriteFile` y el caso "no hay home" |
| **`Detect`** | **0 %** | lee los paquetes instalados |
| **`installed`** | **0 %** | corre `pacman -Q` |

---

## Sin cubrir

| Caso | Por qué no está |
|---|---|
| **`Detect()`** | Es la mitad de la lógica del módulo (README.md:26) y tiene 0 %. Se puede testear: `installed()` llama a `exec.Command("pacman", ...)` — con un `pacman` falso en el `PATH` (un script en `t.TempDir()` que sale con 0 o con 1) se puede fijar el comportamiento completo sin tocar el sistema. Hoy no hay seam, así que el test dependería de qué paquetes tiene la máquina |
| **`installed()`** | Mismo caso. Y es donde está el bug A5: cualquier error de pacman es "no instalado". Un test con un `pacman` falso que falle de otra manera expondría la diferencia |
| **`Current()` cuando el archivo no existe** | Debería caer a `Detect()`. Con `HOME` temporal y sin archivo, el test llamaría a `Detect()` y por lo tanto a pacman — se puede fijar con un `pacman` falso |
| **`Current()` con un canal inválido en el archivo** | El bug A1: el archivo con "nightly" o "EDGE" **debería** caer a `Detect()` o fallar, no devolverse tal cual. Con `HOME` temporal y un archivo escrito a mano, el test es de dos líneas. Hoy no hay test |
| **`Current()` con el archivo vacío o con solo espacios** | Caer a `Detect()`. Verificado con un probe (da `stable`), pero no hay test |
| **`Path()` sin home** | `os.UserHomeDir()` falla si `HOME` no está seteado (en Linux). Con `t.Setenv("HOME", "")` se puede fijar que `Path()` devuelve "" y que `Set()` falla con "no se pudo resolver el home" (`channel.go:51`) |
| **`Set()` con un error de escritura** | Permisos o disco lleno. `Set` debería devolver el error; hoy lo devuelve (`channel.go:56`), pero no está probado |
| **`Set()` es atómico** | El bug B1: un corte deja el archivo a medias. Difícil de testear sin un seam de filesystem, pero al menos se puede fijar que el archivo final tiene el contenido exacto |
| **El archivo que escribe `Set()`** | Ningún test comprueba el **contenido** del archivo: que sea `"stable\n"` o `"edge\n"`. El formato (con el salto de línea) es parte del contrato entre `Set` y `Current`, y nadie lo verifica |
| **Los permisos del archivo y del directorio** | `Set` crea el directorio con `0755` y el archivo con `0644` (README.md:63-64). Ningún test lo verifica. Es un detalle menor, pero los permisos de un archivo en `~/.config` son el tipo de cosa que un `umask` raro cambia en silencio |
| **`Detect()` con los dos paquetes instalados** | `umbriel-git` **y** `noctalia-git` instalados: por el cortocircuito de `||` (`channel.go:38`) solo se consulta el primero. Con un `pacman` falso se puede ver cuántas consultas se hacen |
| **Windows** | `os.UserHomeDir()` en Windows no usa `HOME`, así que `t.Setenv("HOME", ...)` no aísla el test. El repo es Linux-only, pero el test fallaría en Windows. Vale la pena un `t.Skip` si el proyecto alguna vez corre la suite allí |

---

## Bugs de esta revisión

Los 6 hallazgos de `bugs.md` **no tienen test**. El paquete no tiene tests
rojos: `go test ./internal/channel/` pasa entero.

Los dos bugs ALTO son los más cheaply testeables del módulo, porque el
archivo del canal se puede escribir a mano en un `HOME` temporal:

| Bug | Test que lo cubriría |
|---|---|
| A1 (el archivo manda sin validar) | Escribir "nightly" en el archivo del canal y exigir que `Current()` **no** lo devuelva tal cual |
| A2 (canal `stable` inventado) | Un `pacman` ausente o que falle: `Current()` no debería reportar `stable` sin saber |
| A3 (dos consultas de pacman, salida al stdout) | Un `pacman` falso que cuente las invocaciones y revise que nada se escape al stdout |
| A4 (ignora `XDG_CONFIG_HOME`) | `t.Setenv("XDG_CONFIG_HOME", ...)` y comprobar que `Path()` lo respeta |
| A5 ("no instalado" vs "pacman falló") | Un `pacman` falso que salga con 1 y con un stderr que no sea "was not found" |
| B1 (escritura no atómica) | Verificar el contenido exacto del archivo después de `Set` |

El patrón que falta en todos: **el módulo no tiene seam para el binario de
pacman**. Con una variable de paquete (o un parámetro) que apunte al ejecutable
—como `registry` hace con `baseURL` y `stateDir`, y como `internal/plugins`
hace con sus rutas— los seis tests se vuelven triviales y `Detect()` deja de
depender de qué hay instalado en la máquina.

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/b3l0w-cli

# Todo
go test ./internal/channel/ -count=1

# Con cobertura
go test ./internal/channel/ -count=1 -cover

# Con el detector de carreras
go test ./internal/channel/ -race -count=1

# Ver qué función está sin cubrir
go test ./internal/channel/ -count=1 -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out
```

**Hermeticidad:** los dos tests aíslan el home con `t.Setenv("HOME", ...)`, así
que no tocan `~/.config/b3l0w/channel`. **No** aíslan `XDG_CONFIG_HOME`, pero
el módulo tampoco lo usa (BUGS A4), así que hoy no hay contaminación posible.
Ninguno de los dos tests ejecuta pacman: `TestSetAndCurrent` siempre escribe
el archivo antes de leer, así que nunca cae a `Detect()`.
