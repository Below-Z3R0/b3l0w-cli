# update

Orquestador de `b3l0w update`: decide qué capas se actualizan, en qué
orden y qué se le pregunta. Cada capa vive en su paquete
(`internal/system`, `programs`, `plugins`, `services`, `dotfiles`) y
este módulo solo las coordina.

## Orden de ejecución

```
1. sync del registry  (solo las categorías que se van a tocar)
2. CLI                (selfupdate)
3. sistema base       (cachy-update / arch-update)
4. programas propios  (manifiesto)
5. plugins            (sources de Noctalia)
6. servicios          (digests Docker)
7. dotfiles           (merge 3-way)
8. snapshots Limine   (podar)
```

Por qué:

- **Registry primero:** sin catálogo fresco se compara contra la copia
  vieja. Es el `-Sy` que faltaba; `b3l0w update` es el `-Syu` completo.
- **CLI antes que todo:** si el update cambia el contrato de un programa,
  el que orquesta ya es el binario nuevo.
- **Sistema antes que programas:** un programa puede depender de una
  librería del sistema. Además, si el binario pertenece a un paquete,
  `programs.Update` lo detecta (`pacman -Qo`) y no lo toca: pacman manda.
- **Dotfiles y snapshots al final:** los TOML asumen binarios y plugins
  ya actualizados; la poda de Limine corre última para no borrar el
  snapshot recién creado.

Un error corta la cadena, salvo en dos casos: `updatePrograms` no se
detiene en el primer programa que falla, y `updateServices` sigue con el
resto de los servicios.

El **código de salida** refleja el resultado: si un servicio no se
actualizó, `updateServices` devuelve 1 y `Run` devuelve ese 1 (igual que
hace `updatePrograms`). Un script que automatiza el update puede así
saber que algo falló.

## Versiones del CLI

La comparación de versiones es **numérica**, no de strings: el `-ldflags`
puede inyectar la versión con o sin la `v` (`v0.5.0` o `0.5.0`) y las dos
son la misma versión. El dry-run solo anuncia una actualización cuando el
release es **más nuevo** que el binario, y nunca propone un downgrade.

## Escritorio: un solo camino por canal

`desktopPackages(canal)` es la única fuente de verdad de qué paquetes del
escritorio existen: `umbriel`/`noctalia` en `stable` y `umbriel-git`/
`noctalia-git` en `edge`. La usan las dos mitades del módulo: el `--check`
y el comando a mano. Si una preguntara por un canal distinto de la otra,
el check diría que lo que tenés instalado no está instalado.

## Flags

| Flag | Efecto |
|---|---|
| *(ninguno)* | CLI + sistema + programas + plugins + servicios + dotfiles |
| `--self` | Solo el binario (si va solo, corta ahí y el prompt solo lista el CLI) |
| `--system` | Solo el sistema base |
| `--programs` | Solo los programas del manifiesto |
| `--plugins` | Solo los sources de plugins de Noctalia |
| `--services` | Solo los servicios Docker instalados |
| `--dotfiles` | Solo los TOML del repo (merge 3-way) |
| `--adopt` | Registra los archivos actuales como base, sin escribir |
| `--check` | Reporta estado; no actualiza (sale con 1 si algo no se pudo consultar) |
| `--yes`, `-y` | No pregunta (también se pasa a servicios y snapshots) |
| `--dry-run` | Muestra el plan y no toca nada |
| `--force` | En dotfiles, el repo pisa los cambios locales |
| `--dotfiles-dir=<path>` | Repo de dotfiles explícito |
| `--keep=N` | Snapshots de Limine a conservar (default 20) |
| `--desktop` | Muestra el comando de pacman según el canal; no lo ejecuta |

Regla de selección: con **al menos un** flag de capa se ejecutan solo
esas capas; sin flags se ejecutan todas. `--self` cuenta como flag de
capa: con `--self` a secas solo se actualiza el binario. `--system` y
`--programs` juntos los rechaza `cmd/b3l0w/update.go` antes de llegar
acá.

## Confirmación

`confirmPrompt` arma la lista de capas **en orden de ejecución**, no en
el orden en que se pasaron los flags:

```
Update b3l0w, system, programs, plugins, services, dotfiles? [Y/n]
```

La lista es **lo que se va a tocar de verdad**: lo decide `capasPedidas`,
y cualquier flag de selección (`--system`, `--programs`, `--plugins`,
`--services`, `--self`) cuenta como selector. Con `--self` a secas el
prompt es "Update b3l0w? [Y/n] " y el update corta después del CLI: antes
prometía las seis capas y solo actualizaba el binario.

`confirm.Confirm(prompt, false, true)` usa `[Y/n]` con `Y` por defecto
y la respuesta vacía toma el default. Con `--yes` no pregunta. Sin
terminal (pipe, cron, CI) devuelve `false`: imprime `cancelled` y sale
con 0. Nunca se actualiza a ciegas.

## Sync previo del registry

`syncRegistry(systemWanted, servicesWanted, pluginsWanted)` baja las DB
de las categorías que se van a tocar e **ignora el TTL de 1 hora**
(`registry.Sync`, no `registry.Load`). Ante un fallo usa caché viejo →
copia embebida; un sync fallido solo avisa y el update sigue. `apps`
queda afuera: la administra pacman y el CLI no duplica su base.

## Dotfiles: merge 3-way y rollback

Antes de escribir, `snapshot.Create("b3l0w pre-dotfiles")` intenta una
snapshot de `home` con snapper. Si snapper no está o no hay config para
`home`, solo avisa: la protección real es el merge.

El TOML de Noctalia **y el de Umbriel** del repo se validan antes de
tocar nada, **pero solo si el validador está instalado**: sin `noctalia`
(o sin `umbriel`) en el PATH se avisa ("noctalia no está instalado: no se
valida el TOML del repo") y el update sigue. Un TOML que no valida sigue
abortando el update con código 1.

El estado (`~/.local/state/b3l0w/dotfiles.json`) guarda el sha256 de la
última versión aplicada por archivo:

| local | remoto | resultado |
|---|---|---|
| == base | != base | el repo manda, se actualiza |
| != base | == base | solo cambió el local: se conserva |
| != base | != base | conflicto: no se pisa, se escribe `<archivo>.new` |
| no existe | — | se instala |

**No hay rollback automático:** se restaura la snapshot de snapper a
mano (`snapper -c home undochange ...`). Los conflictos se resuelven
editando el `.new` y volviendo a correr; el comando lista al final los
que quedaron pendientes. Si hay conflictos pide dos confirmaciones para
que el repo gane (`--force`); si no, aplica el merge normal.

El **preview de conflictos se hace siempre, también en `--dry-run`**: el
dry-run tiene que decir qué archivos entrarían en conflicto y cuántos son,
si no no sirve para decidir. (El número del aviso es el de los conflictos:
`--force` resuelve solo esos, los archivos que el usuario editó y el repo
no tocó se conservan.)

**Si el preview falla, el update aborta con código 1** y avisa que no se
pudo previsualizar. No se aplica nada a ciegas: el preview es la única
forma de ver los conflictos antes de pisarlos.

Al final se avisa también de los **huérfanos**: rutas que estaban en el
estado pero ya no están en el repo. Se informa, pero el archivo local no
se borra.
`--adopt` corta antes de la snapshot: solo registra hashes y no toca ni
un archivo.

## Qué está testeado

`update_test.go` cubre lo puro y los caminos que no tocan el sistema:
selectores del prompt y su orden; `defaultDotfilesDir`, `fileExists`,
`localVersion` y `compareVersions` (parseo, fallback a 0 sin vercmp o
con salida inválida, y semántica real de epoch/pkgrel/vacío);
`updatePrograms` (manifiesto vacío y programa ausente), `updateServices`
(sin estado), `updateSystem` (sin actualizador y sin terminal),
`updateDotfiles` (`--adopt` no ejecuta snapper ni noctalia),
`desktopUpdate` (comando por canal, sin ejecutar pacman) y el despacho
de `Run` (cancelación sin `--yes`, programas ausentes y plan del
`--dry-run`).

`alto_test.go` cubre las regresiones de los cuatro hallazgos ALTO
(A1-A4): sin `noctalia` el update de dotfiles no aborta (y con un TOML
inválido sigue abortando), el error de un servicio llega al código de
salida (y `Run` lo propaga), el check del escritorio pregunta por los
paquetes del canal, y las versiones del CLI se comparan numéricamente.
Cada uno de esos tests **falla si se revierte su arreglo**: se comprobó
revirtiendo cada arreglo a mano.

`medio_test.go` cubre las regresiones de los seis hallazgos MEDIO (M1-M6):
`--self` no arrastra las demás capas al prompt, un preview que falla
aborta antes de aplicar, el TOML de Umbriel también se valida, sin
`$HOME` el error dice qué variable falta, un source de plugins ilegible es
un error y no un cambio de camino, y sin AUR el `check` sale con 1. Cada
uno **falla si se revierte su arreglo**: se comprobó revirtiendo cada
arreglo a mano.

Los tests acotan `PATH` y `HOME` a temporales y usan fakes de `vercmp`,
`pacman`, `snapper`, `noctalia` y `cachy-update` que dejan un marker si
se ejecutan: si el código toca el sistema, el test falla.

Para lo que depende de la red o de Docker hay **puntos de inyección**:
`selfCheck` (releases del CLI), `aurLookup` (AUR), `registrySync` (DB del
registry), `updateService` (update de un servicio), `releaseCheck`
(firmas de los releases) y `pluginSources` (sources de plugins) son
variables de paquete que los tests sustituyen por fakes.

## Qué no está testeado y por qué

- `selfUpdate`: baja un release de GitHub y reemplaza el binario en
  ejecución.
- `updateSystem` (camino feliz): lanza `cachy-update` interactivo (news,
  confirmaciones, huérfanos, kernel, servicios a reiniciar).
- `updatePrograms` con programas instalados: ejecuta `<prog> update`.
- `updatePlugins`: corre `noctalia` y puede clonar el repo con `git`.
- `syncRegistry`: HTTP a raw.githubusercontent.com y escritura del caché.
- `check`: la API de GitHub (releases sin firma) y `cachy-update -l`.
- `checkPlugins`: el registry de plugins y el loop de comparación.
- `updateDotfiles` sin `--adopt`: snapshot real con snapper antes de
  escribir en `~/.config`.

La causa es estructural: `update.go` llama funciones de paquete
(`selfupdate.Run`, `registry.Sync`, `system.UpdateInteractive`,
`dotfiles.Pull`, `snapshot.Create`) y `kitsystem.Run` directo. Sin punto
de inyección, el único test posible sería de integración contra la
máquina real.

## Qué haría falta para testearlo

1. **Runner inyectable:** interfaz con `Run`, `RunCombined`,
   `HaveCommand` e `IsTTY`, en `Options` o en una `var` de paquete
   (como ya hace `selfupdate` con `apiURL`/`replaceFile`).
2. **Separar decisión de IO:** `plan(opts, manifest) []step` puro y un
   ejecutor que reciba la lista. Hoy selectores, orden y kinds del
   registry están inline en `Run` y no se pueden probar sin ejecutar.
3. **Inyectar dotfiles y snapshot:** `dotfiles.Pull`/`Adopt` ya se
   testean con directorios temporales; falta que `updateDotfiles` los
   reciba para probar conflictos y `--force` sin snapper.
