# apps

Wrapper de pacman para el CLI.

## Qué hace y por qué existe

`b3l0w apps` no es un gestor de paquetes: es un traductor de banderas
que le agrega a pacman **una sola** cosa — impedir el partial upgrade.

El problema real que resuelve: `pacman -Sy <paquete>` sincroniza la base
de datos sin actualizar el resto del sistema. El paquete nuevo se
instala contra librerías viejas y rompe. Es el error clásico de Arch y
no se arregla con un warning: se tiene que negar el comando.

Por eso el wrapper existe. Todo lo demás (instalar, quitar, buscar,
consultar) lo sigue haciendo pacman.

## API

| Función | Qué hace |
|---|---|
| `Run(args []string) int` | Traduce las banderas y ejecuta pacman. Devuelve el exit code |
| `OldestSyncDB() (time.Duration, string, error)` | Edad de la DB de pacman más vieja y su nombre |
| `MaxAge` (const) | Antigüedad máxima tolerada de la DB: 7 días |

```go
// Consulta sin sudo: el wrapper decide si hace falta root.
code := apps.Run([]string{"-Ss", "firefox"})

// Instalar: si la DB tiene más de una semana, se niega y pide -Syu.
code := apps.Run([]string{"-S", "firefox"})
```

## Las reglas que traduce

| Entrada | Qué hace |
|---|---|
| `-Sy`, `-Syy` | **Rechazado**: es un partial upgrade. Mensaje: usá `b3l0w apps -Syu` |
| `-Syu` | Pasa: sincroniza y actualiza, el freno de DB vieja no aplica |
| `-S <pkg>` | Instala, solo si la DB tiene menos de `MaxAge` |
| `-S <pkg> --allow-stale` | Instala aunque la DB sea vieja (escape explícito) |
| `-R` | Se expande a `-Rns`: quita paquete, huérfanas y configuración |
| `-Ss`, `-Si`, `-Q` | Consulta pura, sin sudo |
| sin argumentos | Equivale a `-Q` |

## Decisiones

**El freno de DB vieja no aplica a `-Syu`.** `-Syu` refresca la DB
antes de actualizar: la freshness es justamente lo que arregla. Antes
el chequeo frenaba `-Syu` y el mensaje de escape decía "usá:
`b3l0w apps -Syu`" — un deadlock, porque el comando que el usuario
necesitaba era el que se rechazaba.

**La DB más vieja manda.** `OldestSyncDB` mira todos los
`/var/lib/pacman/sync/*.db` y se queda con el más antiguo. Si una sola
sección quedó atrás, instalar desde ella es igual de riesgoso que
instalar desde una DB vieja entera.

**Una semana, no un número de Arch.** `MaxAge` son 7 días. Arch no
fija un número; una semana es el punto donde un partial upgrade empieza
a romper librerías.

**`-R` se expande a `-Rns`.** Quitar el paquete y nada más deja
dependencias huérfanas y configuración dando vueltas. Es lo que la
gente espera cuando dice "desinstalar".

**Root solo cuando hace falta.** Consultas (`-Ss`, `-Si`, `-Q`) no
piden sudo: un `-Ss` no debería costar un password.

**`--allow-stale` es interno y explícito.** Se filtra antes de llegar
a pacman: no es una bandera de pacman, es la válvula de escape del
CLI.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `apps.go` | Todo el módulo: `Run`, `OldestSyncDB` y los helpers privados que clasifican banderas |
| `apps_test.go` | Tests de las reglas de traducción |

## Qué NO hace

- **No reimplementa pacman.** No resuelve dependencias, no consulta la
  DB, no firma nada: delega.
- **No administra el repo `[b3l0w]`.** Eso vive en el registro de la
  distro, no acá.
- **No maneja las DB de la distro.** `apps` habla con las de pacman.
- **No usa `pacman -Sy` nunca**, ni siquiera por error: la bandera se
  filtra antes de ejecutarse.

## Tests

5 tests: detección de partial upgrade (`-Sy`, `-Syy`, `-Syu`, `-Suy`),
clasificación de instalación (`-S`, `-Ss`, `-Si`, `-Q`), necesidad de
root por bandera, formato de la edad ("3 h", "1 día", "2 días") y la
lectura de la DB real de pacman (se salta si la máquina no tiene DB).
