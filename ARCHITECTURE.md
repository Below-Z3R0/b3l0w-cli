# Architecture

Mapa de alto nivel de `b3l0w-cli`. Si vas a modificar el código, empezá acá.

**Qué es este documento:** dónde está cada cosa y qué no se puede romper.
**Qué NO es:** un tutorial, una referencia de API ni la explicación de cómo
funciona cada módulo. Eso vive en el `README.md` de cada paquete.

---

## Bird's Eye View

`b3l0w-cli` es la capa de control de b3l0wz3r0-os. No instala paquetes: **coordina**
piezas que ya existen y que cada una tiene su propio dueño.

```
                    ┌──────────────┐
   el usuario  ───► │  cmd/b3l0w   │  dispatcher: interpreta el verbo
                    └──────┬───────┘
                           │
              ┌────────────┼─────────────┬──────────────┐
              ▼            ▼             ▼              ▼
        ┌──────────┐ ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ update   │ │ plugins  │  │ services │  │ programs │
        │ (hub)    │ │ 13.5k ln │  │ 2.2k ln  │  │ releases │
        └────┬─────┘ └────┬─────┘  └────┬─────┘  └────┬─────┘
             │            │             │              │
             └────────────┴──────┬──────┴──────────────┘
                                 ▼
                        ┌────────────────┐
                        │ internal/      │  el catálogo: qué existe
                        │ registry       │  y dónde se publica
                        └────────────────┘
                                 │
                        ┌────────┴────────┐
                        ▼                 ▼
                   ┌─────────┐      ┌──────────┐
                   │  kit/*  │      │ sistema  │
                   │ (base)  │      │ (pacman, │
                   │         │      │ docker)  │
                   └─────────┘      └──────────┘
```

El CLI **no reimplementa pacman**. Cuando algo pertenece a un paquete del
sistema, lo delega. Su trabajo es la coordinación: saber qué versión hay, qué
falta, en qué orden se actualiza y qué se puede deshacer.

**19.939 líneas de Go** (sin tests) en **23 módulos**: 19 paquetes de
`internal/` más los 4 plugins, sobre una base de 743 líneas en `kit/` y 2.201 en
`cmd/`.

El reparto no es parejo:

| Dónde | Líneas | Qué implica |
|---|---|---|
| Los 4 plugins | 11.047 (55 %) | cuatro módulos grandes, cada uno con su ciclo |
| Los otros 19 de `internal/` | 5.948 (30 %) | coordinación del sistema |
| `cmd/` | 2.201 (11 %) | el dispatcher y los verberos |
| `kit/` | 743 (4 %) | la base compartida |

Que el 58 % viva en plugins no significa que haya un módulo gigante: significa
que **cuatro módulos de 2.500–4.100 líneas** pesan más que los dieciocho de
coordinación juntos.

---

## Qué cuenta como módulo

Esta distinción ordena todo lo demás: **un módulo es una unidad con su propio
ciclo**, no una carpeta de código.

| | Un módulo tiene | Un paquete transversal no |
|---|---|---|
| Su `README.md` | sí | no (se documenta en el del proyecto) |
| Su PR propio | sí | entra en otro PR |
| Su worktree | sí | no |
| Su dueño claro | sí | lo usan todos |

**Consecuencia práctica:** `internal/plugins` **no es un módulo**. Son cuatro
módulos (`we`, `docker`, `anilist`, `ytmusic`) más una capa transversal que
comparten. Por eso existe un worktree `plugins-anilist` y no un
`plugins` genérico.

En total: **23 módulos** — 19 paquetes de `internal/` + 4 plugins. Más los 7 de
`kit/`, que son base compartida y no se trabajan como módulos.

---

## Entry Points

Según qué quieras hacer, empezá por acá:

| Si vas a... | Empezá en |
|---|---|
| Agregar o cambiar un verbo | `cmd/b3l0w/main.go` — el `switch` de comandos |
| Cambiar cómo se actualiza algo | `internal/update/update.go` — el orquestador |
| Tocar un plugin | `internal/plugins/<nombre>/` |
| Tocar un servicio Docker | `internal/services/install.go` |
| Cambiar el catálogo | `internal/registry/` |
| Cambiar qué instala la distro | `internal/systemdef/` + `system.json` embebido |
| Entender la salida al usuario | `kit/output/` |

`cmd/b3l0w/main.go` tiene 165 líneas y es solo un dispatcher: lee el primer
argumento y llama. Si un verbo crece, va a su propio archivo (`services.go`,
`plugins.go`, `programs.go`), no a `main.go`.

---

## Code Map

### `cmd/b3l0w` — el dispatcher

16 archivos. Interpreta el verbo, arma las flags y llama al paquete que
corresponde. Importa 18 paquetes internos: es el único que conoce a todos.

Los otros cuatro binarios (`b3l0w-we`, `b3l0w-anilist`, `b3l0w-docker`,
`b3l0w-ytmusic`) son envoltorios de 10-15 líneas que llaman a
`internal/plugins`. Existen para que cada plugin tenga su propio ejecutable.

### `internal/update` — el orquestador de actualización

652 líneas, **el hub del sistema**: importa 14 paquetes internos.

Es el que decide el orden de una actualización completa: canal → versión →
programas → plugins → servicios → dotfiles → snapshots. Si agregás una pieza
nueva al sistema, **es acá donde se conecta**.

**Invariante:** `update` puede importar cualquier módulo, pero ningún módulo
importa a `update`. Es la dirección de dependencia la que evita ciclos.

### `internal/plugins` — cuatro módulos, no un paquete

**No es un paquete: es un contenedor de módulos.** Cada plugin es un módulo
completo con dos mitades — backend Go acá y frontend Lua en Noctalia.

```
Noctalia (widget Lua)
   └── cli.luau (compartido)  ──►  b3l0w <ext>  ──►  internal/plugins/<ext>/  (Go)
                                                        └── cli/root.go  (registra el verbo)
```

Ese contrato está en `luau-shared/cli.luau`: *"el service es el único que llama
al CLI; el panel nunca lanza procesos"*. El widget no ejecuta nada: le pide al
CLI.

| Módulo | Go | Qué es |
|---|---|---|
| `we/` | 4.123 líneas, 22 archivos | wallpapers (con topes de 8192×8192 y 100000 contra OOM) |
| `docker/` | 2.783, 11 | servicios Docker desde el escritorio |
| `anilist/` | 2.526, 12 | listas de anime |
| `ytmusic/` | 2.459, 12 | YouTube Music |
| **Total** | **11.891** | cuatro módulos independientes |

Cada uno tiene su `cli/root.go`, que es donde se registra el verbo y se traduce
el pedido del frontend a una función del paquete.

**`luau-shared/`** (6 archivos) es el frontend compartido: `cli.luau` (el
puente), `anim.luau`, `prefs.luau`, `store.luau` y la plantilla
`plugin.toml.tpl`. Un cambio acá afecta a los cuatro plugins a la vez.

**Capa transversal** (1.625 líneas en la raíz):

| Archivo | Qué vive ahí |
|---|---|
| `plugins.go` (67) | descubrir y ejecutar extensiones |
| `registry.go` (373) | el manifiesto, `LoadDefault`, y `Dependencies()` |
| `install.go` (367) | instalar desde el tarball del registry |
| `release.go` (242) | publicar (hoy: desde `main`, sin merge de ramas) |
| `source.go` (181) | el source de Noctalia |
| `remove.go` (70) | borrar local; si viene del source, se deshabilita |
| `scripts.go` (27) | la lista de backends embebidos |
| `exec.go` (27) | correr un backend embebido con bash |

> **Cuidado:** `Dependencies()` **no** vive en un `deps.go` — está en
> `registry.go:247`. El archivo `deps_test.go` testea esa función.

**Invariante:** un plugin **no importa** a otro. Comparten `luau-shared` en el
frontend y `internal/registry` en el backend, y nada más.

**Invariante:** `plugins` no importa `update`, `services` ni `apps`. Solo
depende de `internal/registry` y de `kit/`.

**Invariante:** el binario del plugin nunca se ejecuta desde `PATH`. Se corre
como backend embebido o con `b3l0w plugin-exec`.

**Cómo se trabaja:** un PR por plugin, no uno por "plugins". Los worktrees
`plugins-ytmusic`, `plugins-docker`, `plugins-anilist` y `plugins-we` existen por
eso: son módulos separados.

### `internal/services` — servicios Docker

12 archivos, 2.209 líneas. Instalar, actualizar, rollback, diagnóstico de
hardware, manifiestos y reportes.

**Invariante:** `services` **no importa** `plugins`. Los dos comparten
`internal/registry` y nada más. Si necesitás que se hablen, el punto de contacto
es `registry`, no un import cruzado.

**Invariante:** una actualización de servicio es siempre por acción del usuario.
`-Syu` en services **verifica y pregunta**, no actualiza.

**Invariante:** el rollback revierte la imagen, **nunca** borra volúmenes.

### `internal/programs`, `releases`, `selfupdate` — los binarios propios

Tres paquetes que manejan el ciclo de vida de los ejecutables de la distro:

- `programs` (633 líneas): qué programas hay, versiones, descarga, verificación.
- `releases` (201): el catálogo de publicaciones.
- `selfupdate` (537): el CLI actualizándose a sí mismo.

Dependencia: `releases → selfupdate → programs`. En ese orden.

### `internal/dotfiles`, `snapshot`, `snapshots` — el estado del usuario

- `dotfiles` (555 líneas): copia configuración al home. Hace merge 3-way y deja
  `.new` si hay conflicto; **nunca sobrescribe en silencio**.
- `snapshots` (185 líneas): los snapshots del sistema (Btrfs/Snapper).
- `snapshot` (23 líneas): un envoltorio mínimo.

> **Cuidado:** `snapshot` y `snapshots` son **dos paquetes distintos**. Antes de
> tocar uno, mirá cuál importa el llamador.

### `internal/registry` — el punto de contacto

382 líneas. El catálogo: qué existe, en qué versión, dónde se publica. Es el
único punto donde `plugins`, `services` y `update` se tocan.

**Caché en cuatro niveles:** caché fresco (1 h) → red → caché viejo → embebido.
El embebido es el último recurso, no la fuente normal.

### `internal/systemdef`, `systeminstall` — la definición de la distro

`systemdef` (373 líneas) lee `system.json`: qué perfiles existen y qué instala
cada uno. Va **embebido con `go:embed`**, así el CLI sabe qué es "su" sistema sin
depender de un archivo al lado.

`systeminstall` (300 líneas) aplica esa definición. Es el paquete con más
cuidado: borra y crea rutas.

**Invariante:** toda escritura pasa por `destinoSeguro`, que aborta si la ruta
se sale del home o cae en `/`. Un destino que viene de una definición remota
nunca se usa sin pasar por ahí.

### `internal/system`, `channel`, `version`, `versioncheck`, `apps`, `config`, `pacman`

Los chicos: estado del sistema (121 líneas), canal `stable`/`edge` (88),
la versión (12), chequeo remoto (108), apps del escritorio (253), la
configuración (179) y la escritura del repo en `pacman.conf` (48).

`pacman` escribe en `/etc` con `sudo tee -a`: es acceso al sistema, así que vive
acá y no en `cmd/`. El comando `setup repo` solo parsea, confirma y muestra.

### `kit/` — la base compartida

Siete paquetes sin dependencias del dominio: `system` (ejecutar comandos),
`output` (imprimir), `paths` (dónde van las cosas), `confirm` (preguntar),
`fetch` (HTTP), `verify` (firmas), `releases`.

**Invariante:** `kit` **no importa `internal`**. La dirección es
`cmd → internal → kit`, nunca al revés. Si un helper de `kit` necesita saber de
plugins, el helper está mal ubicado.

`kit/system` es el nodo más importado del repo: **14 paquetes** dependen de él.
Un cambio ahí se siente en todos lados.

---

## Grafo de dependencias

```
cmd/b3l0w ──► internal/* (19 paquetes)
cmd/b3l0w-{we,anilist,docker,ytmusic} ──► internal/plugins

internal/update ──► 14 paquetes internos        (el hub)
internal/plugins ──► registry, kit/*            (aislado)
internal/services ──► registry, kit/*           (aislado)
internal/plugins ✗→ services   internal/services ✗→ plugins
internal/* ──► kit/*          kit/* ✗→ internal/*
```

---

## Cross-cutting concerns

### Cómo se ejecutan los comandos

Todo pasa por `kit/system`. No hay `exec.Command` suelto fuera de ese paquete
(salvo casos justificados). Eso permite inyectar un runner falso en los tests.

### Cómo se imprime

Todo pasa por `kit/output`. Un módulo no hace `fmt.Println` directo si el
resultado lo va a leer otro. Eso hace testeable la salida.

### Cómo se pregunta

`kit/confirm`. Toda operación destructiva pregunta. En un entorno no interactivo
(`!IsTTY`) la respuesta por defecto es **no**.

### Cómo se manejan los errores

Un módulo devuelve `error`; el `cmd/` decide cómo mostrarlo y con qué código de
salida. Los errores no se imprimen dos veces.

### Testing

Cada paquete tiene su `_test.go` al lado. Los que tocan el sistema inyectan un
runner o una capa `Env` en vez de llamar a `exec`. Los tests no necesitan red ni
root.

---

## Glosario

| Término | Qué es |
|---|---|
| **programa** | Un binario propio de la distro (`b3l0w-we`, `b3l0w-anilist`, …) |
| **plugin** | Una extensión del CLI, con su manifiesto y su binario |
| **servicio** | Un contenedor Docker administrado por el CLI |
| **set** | Un conjunto transaccional de paquetes, en vez de un meta paquete |
| **canal** | `stable` o `edge`: qué versión del escritorio sigue el usuario |
| **release** | Una publicación firmada de un programa o del CLI |
| **perfil** | Un conjunto de paquetes y dotfiles que define una instalación |

---

## Invariantes del repo

1. **La dirección de dependencias es una sola:** `cmd → internal → kit`.
2. **`update` es el único hub.** Ningún módulo lo importa.
3. **Ningún binario se ejecuta desde `PATH`.** Siempre backend embebido.
4. **Toda ruta destructiva se valida antes.** Si no coincide, se aborta.
5. **Todo release se firma antes de publicar.** Sin firma, `install` se niega.
6. **No se reimplementa pacman.** `-S` sin DB fresca se rechaza.
7. **Los servicios se actualizan solo por acción del usuario.**
8. **`main` es la única rama.** Un worktree por módulo, PR a `main`.

---

## Qué no está acá

Este documento no explica **cómo** funciona cada paquete. Para eso:

- `internal/<paquete>/README.md` — el mapa local: archivos, funciones, tests.
- `b3l0w-cli/README.md` — el proyecto para usuarios: instalación y uso.
- `docs/` — decisiones, planes y deuda técnica.
