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

## Flags

| Flag | Efecto |
|---|---|
| *(ninguno)* | CLI + sistema + programas + plugins + servicios + dotfiles |
| `--self` | Solo el binario (si va solo, corta ahí) |
| `--system` | Solo el sistema base |
| `--programs` | Solo los programas del manifiesto |
| `--plugins` | Solo los sources de plugins de Noctalia |
| `--services` | Solo los servicios Docker instalados |
| `--dotfiles` | Solo los TOML del repo (merge 3-way) |
| `--adopt` | Registra los archivos actuales como base, sin escribir |
| `--check` | Reporta estado; no actualiza |
| `--yes`, `-y` | No pregunta (también se pasa a servicios y snapshots) |
| `--dry-run` | Muestra el plan y no toca nada |
| `--force` | En dotfiles, el repo pisa los cambios locales |
| `--dotfiles-dir=<path>` | Repo de dotfiles explícito |
| `--keep=N` | Snapshots de Limine a conservar (default 20) |
| `--desktop` | Muestra el comando de pacman según el canal; no lo ejecuta |

Regla de selección: con **al menos un** flag de capa se ejecutan solo
esas capas; sin flags se ejecutan todas. `--system` y `--programs`
juntos los rechaza `cmd/b3l0w/update.go` antes de llegar acá.

## Confirmación

`confirmPrompt` arma la lista de capas **en orden de ejecución**, no en
el orden en que se pasaron los flags:

```
Update b3l0w, system, programs, plugins, services, dotfiles? [Y/n]
```

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

Los tests acotan `PATH` y `HOME` a temporales y usan fakes de `vercmp`,
`pacman`, `snapper`, `noctalia` y `cachy-update` que dejan un marker si
se ejecutan: si el código toca el sistema, el test falla.

## Qué no está testeado y por qué

- `selfUpdate`: baja un release de GitHub y reemplaza el binario en
  ejecución.
- `updateSystem` (camino feliz): lanza `cachy-update` interactivo (news,
  confirmaciones, huérfanos, kernel, servicios a reiniciar).
- `updatePrograms` con programas instalados: ejecuta `<prog> update`.
- `updatePlugins`: corre `noctalia` y puede clonar el repo con `git`.
- `syncRegistry`: HTTP a raw.githubusercontent.com y escritura del caché.
- `check`, `checkDesktop`, `checkPlugins`: API de GitHub, AUR y
  `cachy-update -l`.
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
