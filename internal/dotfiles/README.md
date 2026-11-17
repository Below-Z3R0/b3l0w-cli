# dotfiles

Aplica los TOML del repo de distribución sobre `~/.config` **sin pisar los
cambios locales**. Merge 3-way por hash: el estado guarda el sha256 de la
última versión aplicada por archivo, y de ahí sale la decisión (`local ==
base` → el repo manda; `remote == base` → solo cambió el local, se
conserva; ambos cambian → conflicto y se escribe `<archivo>.new`).

## API

```go
res, err := dotfiles.Pull(srcDir, homeDir, statePath, force, dryRun)
n, err := dotfiles.Adopt(srcDir, homeDir, statePath)   // dryRun opcional
```

`srcDir` es el repo de dotfiles; `homeDir` es el home del usuario (los
archivos van a `<home>/.config/<ruta relativa>`); `statePath` es el JSON
de estado.

`Pull` recorre el repo con `filepath.Walk` y decide por archivo. El orden
de las ramas importa: la primera que matchea gana.

## Las 7 ramas del switch

| # | Base | Local | Repo | Resultado | Con `--force` |
|---|---|---|---|---|---|
| 1 | hay | no existe | cualquiera | `Kept` — borrado intencional, no se resucita | igual: se conserva |
| 2 | no hay | no existe | cualquiera | `Missing` — se instala | igual: se instala |
| 3 | no hay | existe | **idéntico** | `Kept` — se adopta, la base pasa a ser el remoto | igual |
| 4 | no hay | existe | **distinto** | `Conflicts` + se escribe `<archivo>.new` | `Updated` — el repo gana |
| 5 | hay | **igual a la base** | distinto | `Updated` — el repo manda | igual: ya se pisa |
| 6 | hay | distinto | **igual a la base** | `Kept` — solo cambió el local | **igual: se conserva** |
| 7 | hay | distinto | distinto | `Conflicts` + se escribe `<archivo>.new` | `Updated` — el repo gana |

Bordes: en la rama 5, si el repo tampoco cambió (`remote == local`) el
archivo no entra en ninguna lista — no hay nada que hacer. La rama 3
evita que la primera corrida sea una bola de conflictos: si el local ya
es idéntico al repo, se adopta como base y queda registrado.

## El estado

JSON plano, ruta relativa (con `/`) → sha256 en hex del contenido aplicado:

```json
{"noctalia/config.toml": "27edeab5088a973a...", "fish/config.fish": "5b1c01f2e62989c5..."}
```

Lo usa la CLI en `~/.local/state/b3l0w/dotfiles.json`
(`internal/update/update.go:347`). Se escribe con `MarshalIndent` y un
`\n` al final; si no existe, `Pull` arranca con el estado vacío. Es la
**única** memoria del merge: no guarda copias del contenido anterior,
alcanza con el hash para saber si el local cambió.

## Qué significa `--force` ahora

Antes pisaba todo. Ahora **no cambia la lógica 3-way**: solo resuelve
los conflictos reales (ramas 4 y 7) a favor del repo.

Lo que `--force` **no** hace: no pisa un archivo que solo editó el
usuario (rama 6 — ese cambio es trabajo del usuario, no un conflicto), no
resucita un archivo que el usuario borró (rama 1) y no toca los
huérfanos (ver abajo).

Lo que sí hace: en las ramas 4 y 7 escribe el remoto sobre el local en
lugar de dejar un `.new` al lado. El archivo local se pierde, pero el
repo gana — es explícito y el usuario lo pidió.

`Result.ForceOverwrites` lista **el costo de `--force`**: qué archivos
pisaría `--force` y que sin él no se pisarían. Para anunciar el número
real antes de decidir.

Entra en esa lista solo la **rama 7** (ambos cambiaron: conflicto real).
**No** entran las ramas 4 y 5:

- La rama 5 (el usuario no editó el archivo) se pisa igual con y sin
  `--force`: el repo manda siempre. Incluirla inflaría el número.
- La rama 4 (sin base en el estado) tiene conflicto y `--force` la
  resuelve, pero antes deja un `.bak` del contenido local.

La lista se arma igual con y sin `--force`, así que se puede calcular el
costo antes de decidir.

## Adopt

`Adopt` registra el **hash del contenido local** como base de cada
archivo que ya existe en el home, sin tocar ningún archivo. Devuelve
cuántos adoptó.

Sirve para la primera corrida: sin historial, todo sería conflicto
(rama 4) y el usuario tendría que mergear N archivos a mano. Adoptando,
cada archivo existente cae en la rama 5 (su local *es* la base): si el
repo difiere, se aplica sin generar conflicto; si es idéntico, no pasa
nada.

Por qué la base es el **local** y no el repo: la base representa "lo que
el usuario tenía cuando el sistema lo aplicó". Si fuera el hash del
repo, un archivo que el usuario ya editó caería en la rama 6 (solo
cambió el local) y el repo no podría volver a tocarlo: cada cambio
futuro del repo sobre ese archivo sería un conflicto en vez de una
actualización. Con la base local, el merge 3-way normal decide desde el
primer pull. Es el criterio de git y rsync: la base es el ancestro
común, no la versión del servidor.

Consecuencia: como la base es el local, el siguiente pull trata ese
archivo como "no editado por el usuario" y **el repo gana** sobre él
(rama 5). `Adopt` no congela la config del usuario: la delega al repo
desde la primera corrida. No pisa nada y no borra entradas del estado:
solo agrega. Un `dryRun` opcional evita escribir el estado.

> El docstring de `Adopt` en `pull.go` dice "registra la versión del repo
> como base", pero el código registra el hash local (`hash(local)`). El
> comportamiento no cambió en el commit `2bb6eaf` — el docstring anterior
> ("el hash actual de cada archivo local") era el correcto y este lo
> dejó desactualizado.

## Result

| Campo | Qué significa |
|---|---|
| `Updated` | el repo se aplicó sobre el local |
| `Kept` | se conservó el local (no lo editaste, o solo cambió el local) |
| `Conflicts` | hubo conflicto: el local quedó intacto y se escribió `.new` |
| `Missing` | no existía en el home: se instaló |
| `Orphaned` | estaba en el estado pero ya no está en el repo |
| `ForceOverwrites` | qué archivos reemplazaría un pull con `--force` |
| `Changed` | se escribió algún archivo |

`Changed` solo se activa cuando se escribe un archivo del repo (`Updated`
o `Missing`). Un conflicto **no** lo activa: escribe un `.new` aparte.

`Orphaned` es un aviso, no una acción. El archivo local **nunca** se
borra solo — ni con `--force`. Se poda la entrada del estado (para que
no crezca sin control) y se reporta el resto, pero decidir qué hacer con
los archivos que quedaron atrás es del usuario. Un huérfano se detecta
comparando el estado contra lo que hay en el repo: una ruta que estaba
en el estado y ya no aparece en el repo se reporta.

## Decisiones

- **Hash en vez de copia del base.** El estado es un JSON plano en vez de
  un árbol de archivos: el contenido anterior no hace falta para decidir,
  alcanza con saber si cambió.
- **`--force` solo para conflictos reales.** Pisar un cambio local que no
  es conflicto es perder trabajo del usuario sin motivo.
- **Conflicto avanza la base.** Cuando hay conflicto se escribe el
  `.new` y la base avanza al hash del repo. Si no avanzara, el mismo
  conflicto reaparecería en cada pull.
- **El estado se persiste por archivo.** Se guarda después de cada
  archivo aplicado, no al final. Si una escritura falla a la mitad, los
  archivos ya escritos no quedan con la base vieja (que dispararía
  conflictos falsos en el pull siguiente).
- **Los huérfanos se podan del estado, no del disco.** El estado tiene
  que reflejar lo que el repo maneja hoy. Los archivos del home son del
  usuario: avisar, no borrar.
- **Se conserva el modo del destino.** Un `0600` no puede quedar `0644`
  después de un update. Si el destino no existe, se usa el modo del
  origen.
- **No se rompen symlinks.** `resolveDest` sigue el link antes de
  escribir: escribir encima reemplazaría el symlink por un archivo
  regular y dejaría el target huérfano.

## Notas

- `skip()` deja fuera lo que no es configuración de usuario: cualquier
  `.md`, `outputs.toml` y `templates/hermes/`. Pero la ruta se marca
  como vista igual, así que un `.md` que está en el repo no se reporta
  como huérfano aunque figure en el estado.
- La CLI todavía **no imprime** `Orphaned` ni `ForceOverwrites`
  (`internal/update/update.go` solo recorre `Updated`, `Kept`, `Missing`
  y `Conflicts`). Los campos están listos para usar; falta el `output`.
- La CLI hace un `Pull` en dry-run antes del real para detectar
  conflictos y preguntar antes de pisar.
- Cuando `--force` pisa una config que **nunca estuvo gestionada**
  (rama 4), deja un `.bak` del contenido local al lado. Sin ese backup
  ese trabajo se perdía en silencio: el `.new` no alcanza porque el
  conflicto no se reporta cuando `--force` lo resuelve.
- Cuando `--force` pisa un **conflicto real** (rama 7), no deja `.bak`:
  el `.new` ya documenta el estado anterior y el usuario fue avisado.

## Tests

| Archivo | Qué prueba |
|---|---|
| `pull_test.go` | primera corrida, conservación del cambio local, update, dry-run |
| `merge_test.go` | los casos del 3-way con base explícita |
| `adopt_test.go` | adoptar y después aplicar el repo |
| `escenario_test.go` | helper de escenarios multi-archivo |
| `orphan_test.go` | los huérfanos: se reportan, no se borran |
| `force_test.go` | `--force`: qué pisa, qué conserva, qué lista |
| `dryrun_test.go` | el dry-run no escribe archivo, ni `.new`, ni estado |
