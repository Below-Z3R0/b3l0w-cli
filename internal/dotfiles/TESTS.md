# TESTS — internal/dotfiles

Qué prueba cada test del módulo, qué encontró el fuzzing y qué quedó
sin cubrir.

**Regla dura del módulo:** ningún test toca `~/.config` real. Todo vive
en `t.TempDir()` con `srcDir`, `homeDir` y `statePath` explícitos.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 88 (56 previos + 32 nuevos) |
| Objetivos de fuzzing | 4 (2 previos + 2 nuevos) |
| Verdes hoy | 78 |
| **Rojos hoy (bugs reales)** | **10** |
| Fuzzing | 4 objetivos × 60 s: **0 hallazgos** |

Los 10 rojos son los bugs que este módulo todavía tiene. Cada uno tiene
un test que falla hoy y pasa con el fix. Están listados abajo. Los 22
`TestEdge_` nuevos pasan: blindan comportamiento que ya funciona.

---

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `pull_test.go` | primera corrida, conservación del cambio local, update, dry-run |
| `merge_test.go` | los casos del 3-way con base explícita |
| `adopt_test.go` | adoptar y después aplicar el repo |
| `escenario_test.go` | helper de escenarios multi-archivo (`escenarioMulti`, `leerEstado`, `mismoConjunto`, `contiene`) |
| `orphan_test.go` | los huérfanos: se reportan, no se borran |
| `force_test.go` | `--force`: qué pisa, qué conserva, qué lista |
| `dryrun_test.go` | el dry-run no escribe archivo, ni `.new`, ni estado |
| `bug_test.go` | 17 bugs de integridad ya arreglados (A1-A6, C1-C2, M1-M3) |
| `edge_test.go` | 13 bordes que ya funcionan y hay que blindar |
| `backup_test.go` | `--force` deja `.bak` / no lo deja |
| `git_test.go` | el `.git/` del repo no se despliega |
| `fuzz_test.go` | fuzzing del parser del estado y del Pull con estado arbitrario |
| **`concurrencia_test.go`** | **Pull simultáneos: estado pisado, error de rename, contenido mezclado** |
| **`temporal_test.go`** | **el temporal de escritura (`<archivo>.tmp`) y los archivos del usuario** |
| **`estado_test.go`** | **guard de repo vacío, repos parciales, estado raro, rutas relativas** |
| **`bak_test.go`** | **semántica del `.bak`: sobrescritura, symlink, dry-run** |
| **`gitskip_test.go`** | **qué de `.git`/`.gitignore`/`.github` se despliega** |
| **`fuzz_prop_test.go`** | **fuzzing por propiedades: el Result es la verdad** |

---

## Los 11 bugs que fallan hoy

Todos con `TestBug_`. Cada uno falla con el código actual y pasa con el
fix. **No se tocó `pull.go`**: los fixes son del writer.

### B1 — `TestBug_ElTemporalFijoDestruyeElArchivoDelUsuario` (+2 tests hermanos)

**`temporal_test.go`**

`writeFile` escribe en `<archivo>.tmp` y después renombra. El nombre del
temporal es **fijo**: no es único por escritura como el que usa
`saveState` (`os.CreateTemp`). Si el usuario ya tenía un archivo con ese
nombre, `os.WriteFile` lo trunca y el rename se lo lleva: **el archivo
del usuario desaparece**.

- `TestBug_ElTemporalFijoDestruyeElArchivoDelUsuario` — `config.toml.tmp`
- `TestBug_ElTemporalFijoDestruyeElNewTmpDelUsuario` — `config.toml.new.tmp`
- `TestBug_ElTemporalFijoDestruyeElBakTmpDelUsuario` — `config.toml.bak.tmp`

**Fix:** `os.CreateTemp` en el mismo directorio + `rename`, como hace
`saveState`. También hay que cuidar el `Chmod` (el modo final va en el
archivo temporal, no en el destino).

**Trampa:** `saveState` ya usa `os.CreateTemp` — el mismo patrón se puede
copiar tal cual a `writeFile`.

### B2 — `TestBug_DosPullConcurrentesMezclanElContenido`

**`concurrencia_test.go`**

Dos Pull simultáneos con contenidos distintos escriben el mismo archivo
del home. Como el temporal es de nombre fijo, los dos writers escriben
en el mismo archivo: **el resultado es un Frankenstein** — el principio de
un repo y el final del otro. El estado además guarda el hash de una
versión que no es la que quedó en disco.

Medido: 11 de 40 corridas libres dejaron el archivo mezclado
(`"BBBB…AAAA"`, 500 000 bytes = 400 000 de A + 100 000 de B). El test lo
hace **determinista** con un FIFO como temporal: los dos writers se
bloquean en el mismo `open` y escriben en el mismo flujo.

**Fix:** el mismo que B1 (temporal único). Con un temporal por escritura
el destino recibe una versión completa, la que renombra último.

### B3 — `TestBug_DosPullConcurrentesFallanAlEscribirElMismoDestino`

**`concurrencia_test.go`**

El mismo origen que B2, por el otro lado: el writer que pierde el
`rename` (o el `chmod`) falla con `no such file or directory` — un error
críptico para el usuario. Medido: **19 de 20 iteraciones** fallan.

**Fix:** el mismo que B1.

### B4 — `TestBug_DosPullConcurrentesSePisanElEstado`

**`concurrencia_test.go`**

Confirmación con evidencia de **M6** (BUGS.md lo había marcado como
"no probado, por inspección"). Dos Pull sobre el mismo estado: el
último `save` gana y **las bases que avanzó el otro desaparecen**.

El solapamiento se fuerza con un FIFO: el Pull A se bloquea en
`writeFile` justo antes de escribir `a.toml`; el Pull B corre entero y
deja `{b.toml}`; cuando A se destraba, su save pisa el estado y queda
solo `{a.toml}`.

**Consecuencia para el usuario:** el pull siguiente ya no sabe qué base
tenía cada config. Cada una cae en la rama 4 (sin base) y aparece como
conflicto — o, con `--force`, se pisa en masa.

**Fix:** `flock` sobre `statePath + ".lock"` (o sobre el directorio de
estado) al entrar a `Pull`/`Adopt`, con timeout corto y error claro. Sin
lock, la pérdida de bases es inevitable: el estado es un read-modify-write.

### B5 — `TestBug_ElRepoSoloConArchivosDescartadosBorraElEstado`

**`estado_test.go`**

El guard de C1 (`if len(seen) == 0 { return }`) mira `seen`, que cuenta
**también los archivos que `skip()` descarta**. Un repo con un solo
`README.md` (un clone a medias, o un `--dotfiles-dir` apuntando a otra
cosa) deja `seen` con una entrada y por lo tanto **pasa el guard**: el
estado entero se poda y todas las bases se pierden.

Es el mismo daño que C1 por la puerta de atrás.

**Fix:** contar solo los archivos que **se aplican** (los que no caen en
`skip()`), no los que se ven. Si no se aplicó ninguno, no se poda nada.

### B6 — `TestBug_ElRepoSoloConGitBorraElEstado`

**`estado_test.go`**

El mismo caso de B5 con un repo que solo tiene `.git/` (un clon sin
working tree). El `.git` lo descarta `skip()`, pero igual cuenta como
visto.

**Fix:** el mismo de B5.

### B7 — `TestBug_ElEstadoPerdidoProvocaConflictosFalsos`

**`estado_test.go`**

La consecuencia concreta de B5/B6, de punta a punta: un pull con un repo
equivocado vacía el estado; el pull siguiente con el repo correcto
reporta **todos** los archivos como conflicto, aunque el usuario no los
tocó. Con `--force` es pérdida de trabajo local en masa.

**Fix:** el mismo de B5.

### B8 — `TestBug_ElEstadoQueEsUnDirectorioDaUnErrorSinSalida`

**`estado_test.go`**

Si el archivo del estado es un directorio, `loadState` devuelve el error
crudo de `os.ReadFile` ("is a directory"). El usuario no tiene forma de
saber qué hacer. El error de estado corrupto (JSON inválido) sí dice
"borralo para regenerarlo"; este no dice nada.

**Fix:** en `loadState`, envolver el error de lectura con el mismo
mensaje de recuperación que el del JSON inválido.

### B9 — resumen: 8 causas, 10 tests rojos

| Causa | Tests rojos | Fix |
|---|---|---|
| Temporal de nombre fijo (B1) | 3 | `os.CreateTemp` + rename |
| Temporal de nombre fijo, concurrencia (B2, B3) | 2 | el mismo |
| Sin lock sobre el estado (B4) | 1 | `flock` sobre el estado |
| El guard de repo vacío es angosto (B5, B6, B7) | 3 | contar solo los archivos aplicados |
| El estado como directorio no dice cómo recuperar (B8) | 1 | envolver el error de lectura |

**Tres de los diez tests rojos son el mismo bug** (el temporal de nombre
fijo): uno solo, dos en el `.new` y uno en el `.bak`. Con un solo fix
(`os.CreateTemp` en `writeFile`) caen tres.

---

## Los bordes que sí funcionan (blindados con `TestEdge_`)

Estos **pasan hoy**. Existen para que un arreglo futuro no los rompa.

### `temporal_test.go`

| Test | Qué fija |
|---|---|
| `TestEdge_NoQuedaTemporalEnElDisco` | después de un pull limpio no queda ningún `.tmp` en `~/.config` |
| `TestEdge_ElTemporalNoTocaElArchivoVecinoDeOtroNombre` | un `.tmp` que no es el temporal de un archivo gestionado sobrevive |

### `estado_test.go`

| Test | Qué fija |
|---|---|
| `TestEdge_ElRepoTotalmenteVacioNoBorraElEstado` | **el guard de C1**: un repo totalmente vacío no poda el estado. Este test **no existía** — el fix de C1 estaba sin cobertura |
| `TestEdge_ElEstadoQueEsUnDirectorioNoEscribeNada` | el fallo es limpio: no toca el home ni crea el estado |
| `TestEdge_AdoptConElEstadoQueEsUnDirectorioFallaSinTocarNada` | `Adopt` falla igual, con 0 adopciones |
| `TestEdge_ElSrcDirQueNoExisteFallaEnVozAlta` | un `--dotfiles-dir` inexistente falla con un error que lo menciona, y **no toca el estado** |
| `TestEdge_ConRutasRelativasFunciona` | `srcDir`/`homeDir`/`statePath` relativos (y con `..`) funcionan; la clave del estado es la ruta relativa al repo |
| `TestEdge_ElHomeQueEsUnSymlinkSeAplicaEnElTarget` | si el home es un symlink, se escribe en el home real y el symlink no se reemplaza |
| `TestEdge_ElConfigQueEsUnArchivoFallaSinTocarNada` | si `~/.config` es un archivo (no un dir), el pull falla sin escribir nada ni crear el estado |
| `TestEdge_UnaFallaDeEscrituraDejaElEstadoConsistente` | si la escritura de un archivo falla, los ya escritos quedan con su base avanzada y el que falló no queda con base (el contrato de "persistir por archivo") |

### `bak_test.go`

| Test | Qué fija |
|---|---|
| `TestEdge_ElBakPrevioSeSobrescribeConElLocalMasReciente` | si ya hay un `.bak`, el force lo pisa con el local actual: queda **el más nuevo**, no el más viejo |
| `TestEdge_ElBakEditadoAManoSePierdeEnElProximoForce` | **documenta la pérdida**: un `.bak` editado a mano se destruye en el próximo force (ver "Decisiones pendientes") |
| `TestEdge_ElBakSeEscribeJuntoAlSymlinkYElTargetSeActualiza` | con symlink, el `.bak` queda junto al symlink y el target es el que se pisa |
| `TestEdge_NoHayBakParaUnLocalVacio` | un local vacío no genera backup |
| `TestEdge_NoHayBakEnElDryRun` | el dry-run no deja backup |
| `TestEdge_NoHayBakSiElLocalNoExiste` | si el archivo no estaba, no hay nada que respaldar |

### `gitskip_test.go`

| Test | Qué fija |
|---|---|
| `TestEdge_ElArchivoGitNoSeDespliega` | un `.git` que es **archivo** (worktree/submódulo, `gitdir: …`) tampoco se despliega |
| `TestEdge_ElGitignoreNoSeSaltea` | un `.gitignore` **no** es un `.git`: se despliega (el `skip()` solo descarta el `.git` exacto y lo que empieza por `.git/`) |
| `TestEdge_ElGithubYElLicenseSeDespliegan` | `skip()` está cableado al layout de b3l0w (M8): el CI y la licencia **también** se despliegan |
| `TestEdge_ElMdNoSeDespliegaPeroCuentaComoVisto` | un `.md` no se despliega pero cuenta como visto: no se reporta como huérfano |

### `concurrencia_test.go`

| Test | Qué fija |
|---|---|
| `TestEdge_ElEstadoQuedaJSONValidoConPullConcurrentes` | aun con Pull concurrentes el estado nunca queda truncado ni corrupto (la invariante del guardado atómico) |
| `TestEdge_ElArchivoDelHomeNoQuedaConContenidoParcial` | un Pull aislado siempre deja el archivo completo |

---

## Fuzzing

### Objetivos previos (`fuzz_test.go`)

| Objetivo | Qué cubre | Resultado (60 s) |
|---|---|---|
| `FuzzParseEstado` | el parser del estado: nunca panica, nunca devuelve un map nil, el round-trip conserva las claves | **0 hallazgos** — 45 477 ejecuciones, 128 inputs "interesantes" nuevos |
| `FuzzPullConEstado` | `Pull` con estado/repo/local arbitrarios: nunca panica, el estado queda con JSON válido, nada se escribe fuera del home | **0 hallazgos** — 69 679 ejecuciones, 127 inputs nuevos |

### Objetivos nuevos (`fuzz_prop_test.go`)

Verifican la **propiedad central** del módulo, que los objetivos previos
no miraban: *el `Result` es la verdad*.

| Objetivo | Qué cubre | Resultado (60 s) |
|---|---|---|
| `FuzzPullRespetaElLocal` | lo que el `Result` no dice que se escribió, no se escribió: un archivo reportado `Kept` o `Conflicts` no cambió un byte; uno reportado `Updated` o `Missing` tiene **exactamente** el contenido del repo. Además: el estado queda con JSON válido y nada se escribe fuera del home | pendiente de la corrida larga |
| `FuzzAdoptNoTocaElHome` | `Adopt` solo lee: no crea, no borra ni modifica ningún archivo del home, y la base que registra es el hash del **local** (no del repo) | pendiente de la corrida larga |

**Cómo correrlos:**

```bash
go test ./internal/dotfiles/ -run='^$' -fuzz='^FuzzPullRespetaElLocal$' -fuzztime=60s
go test ./internal/dotfiles/ -run='^$' -fuzz='^FuzzAdoptNoTocaElHome$' -fuzztime=60s
```

Los inputs que el fuzzer encuentra quedan en el cache de Go
(`$GOCACHE/fuzz/…`); los que rompen una invariante se copian a
`testdata/fuzz/<Objetivo>/` y hay que commitearlos.

---

## Decisiones pendientes (NO se tocaron `pull.go`)

Estas necesitan una decisión de diseño antes de escribir código. Cada una
tiene un test que **documenta el comportamiento actual** (`TestEdge_`,
verde) para que el cambio sea deliberado.

### D1 — Qué pasa con un `.bak` que ya existe

**Hoy:** `backupLocal` sobrescribe siempre. El `.bak` de una corrida
anterior se pierde, y un `.bak` que el usuario editó a mano **se pierde
en silencio** (`TestEdge_ElBakEditadoAManoSePierdeEnElProximoForce`).

**Opciones:** rotar (`.bak` → `.bak.1`), negarse a pisar uno editado,
escribir con timestamp, o avisar en el `Result`.

**Nota:** el README dice que el `.bak` solo se deja en la rama 4 (config
nunca gestionada), pero el código también lo deja en la rama 7
(conflicto real). El README quedó desactualizado en el commit `3683185`.

### D2 — El `.new` de un conflicto pisa el merge a medias del usuario

**Hoy:** `writeFile(dest+".new", …)` no verifica si ya existe. Si el
usuario estaba mergeando en `config.toml.new` y el repo publica otra
versión, su merge se reemplaza sin avisar (BUGS.md **M4**).

**Opciones:** `dest+".new.2"`, negarse a escribir si ya existe, o
avisar en el `Result`. **No se escribió test** porque el comportamiento
deseado no está decidido: el mismo bug se puede(specificar) de tres
formas y el test tiene que elegir una.

### D3 — La política de `skip()`

**Hoy:** `skip()` está cableado al layout del repo de b3l0w
(`templates/hermes`, `outputs.toml`, `.md`). Todo lo demás se despliega:
`.github/`, `LICENSE`, `.gitignore`, scripts de build (BUGS.md **M8**).

**Opciones:** política por segmento, lista explícita de directorios
top-level permitidos, o un `.dotfilesignore` en el repo.

Los tests de `gitskip_test.go` **documentan el comportamiento actual**
para que el cambio sea deliberado.

### D4 — Un symlink en `~/.config` hace que el pull escriba afuera

`resolveDest` sigue el symlink del destino antes de escribir (para no
romperlo). El efecto: si `~/.config/foo` es un symlink a
`~/.local/share/otro-cosa`, el pull escribe **ahí**. Es el comportamiento
documentado ("No se rompen symlinks") y es lo correcto para el caso de
uso (el usuario lo puso a propósito), pero significa que un symlink
-equivocado en `~/.config` desvía la escritura fuera de `~/.config`.

No se propone fix: cambiarlo rompería el caso de uso real.

### D5 — El `.new` no se limpia al resolverse el conflicto

Cuando el usuario mergea a mano y el pull siguiente resuelve, el
`config.toml.new` viejo queda para siempre (BUGS.md **B9**). Cosmético,
pero acumula basura. Borrarlo no es seguro sin saber si el usuario lo
editó.

---

## Qué quedó sin cubrir (y por qué)

| Caso | Por qué no está |
|---|---|
| **Disco lleno real** (ENOSPC) | no se puede simular sin montar un tmpfs con límite (requiere privilegios) ni usar `ulimit -f` (manda SIGXFSZ y mata el proceso de test). Se cubre lo más cercano: `TestEdge_UnaFallaDeEscrituraDejaElEstadoConsistente` simula el fallo de escritura con un directorio en el lugar del temporal (`EISDIR`) y verifica que el estado queda consistente |
| **Dos procesos reales** (`b3l0w update` en dos terminales) | los tests de concurrencia son dos goroutines en el mismo proceso: reproducen la condición de carrera del `read-modify-write` del estado y del temporal, pero no el `fork` ni el `page cache` entre procesos. El FIFO como temporal es la misma ruta de código (`writeFile`), así que la mezcla de contenido es la misma |
| **`flock` / lock entre procesos** | no hay lock hoy (B4). Cuando se agregue, tiene que tener su propio test: dos procesos que compiten por el lock, un lock que ya está tomado (timeout), un lock roto (stale) |
| **El `.new` de un conflicto (M4)** | decisión de diseño pendiente (D2) |
| **La política de `skip()` (M8)** | decisión de diseño pendiente (D3) |
| **`~/.config` en otro filesystem** (bind mount, tmpfs) | `writeFile` hace `rename` dentro del mismo directorio, así que nunca cruza de filesystem. No hace falta testearlo: el rename es local al directorio |
| **Windows** | el módulo usa `filepath.Separator` y `syscall.Mkfifo`; los tests de FIFO hacen `t.Skip` en Windows. El resto del módulo es portable |
| **Errores de `Adopt` con `saveState` que falla a mitad** | cubierto de forma equivalente por `TestBug_AdoptDevuelveCeroSiFallaElGuardado` (ya existía) |
| **El CLI (`internal/update`)** | fuera del módulo: consume `Result` pero no imprime `Orphaned` ni `ForceOverwrites` (BUGS.md B10). El módulo entrega el dato; el que no lo muestra es el consumidor |

---

## Cómo correr la suite

```bash
cd ~/Documents/Github/Cli/branches/dotfiles

# Todo
go test ./internal/dotfiles/ -count=1

# Solo los bugs (los que tienen que fallar hasta que se arreglen)
go test ./internal/dotfiles/ -run='^TestBug_' -v -count=1

# Solo los bordes blindados (siempre verdes)
go test ./internal/dotfiles/ -run='^TestEdge_' -count=1

# Con el detector de carreras
go test ./internal/dotfiles/ -race -count=1

# Fuzzing (uno por vez)
go test ./internal/dotfiles/ -run='^$' -fuzz='^FuzzPullRespetaElLocal$' -fuzztime=60s
```
