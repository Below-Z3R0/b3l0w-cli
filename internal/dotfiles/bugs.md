# BUGS — internal/dotfiles

Revisión de integridad en contexto cero sobre `mod/dotfiles` (HEAD `60816f9`).
Se leyó `pull.go`, `README.md`, los 7 `*_test.go`, y el punto de integración
`internal/update/update.go:323-410`. Los hallazgos marcados **verificado** se
reprodujeron con tests temporales en `t.TempDir()` (archivo `zz_probe_test.go`,
borrado después); los marcados **verificar** dependen de un layout o condición
que no pude confirmar en este equipo. No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/dotfiles/` → **ok**
(0.01s). `go vet` limpio. Ningún test toca el home real (`grep UserHomeDir` en el
paquete no devuelve nada).

## Cierre de los hallazgos CRÍTICO y ALTO (2026-11-19)

Los ocho hallazgos CRÍTICO/ALTO de este documento están **cerrados**. Cada uno
tiene un test canónico en `hallazgos_test.go` que **falla si se revierte el
arreglo** (verificado revirtiendo el arreglo uno por uno: los ocho tests
fallan, y el paquete sigue compilando en cada reversión).

| ID | Sev | Arreglo | Test que lo cubre |
|---|---|---|---|
| C1 | CRÍTICO | no se poda el estado si el repo no tenía nada aplicable | `TestHallazgo_C1_ElRepoEmptyNoBorraElEstado` |
| C2 | CRÍTICO | el VCS del repo (`.git`/`.hg`/`.svn`) no se despliega, por segmento y con `SkipDir` | `TestHallazgo_C2_ElVCSDelRepoNoSeDespliega` |
| A1 | ALTO | `Adopt` no pisa una base existente | `TestHallazgo_A1_AdoptNoPisaUnaBaseExistente` |
| A2 | ALTO | `--force --dry-run` reporta en `Updated` lo que se escribiría | `TestHallazgo_A2_ForceDryRunReportaLoQueSeEscribiria` |
| A3 | ALTO | un estado con JSON `null` se trata como estado vacío | `TestHallazgo_A3_ElEstadoNullNoPanica` |
| A4 | ALTO | el estado se guarda con temporal + `rename` (escritura atómica) | `TestHallazgo_A4_ElEstadoSeGuardaDeFormaAtomica` |
| A5 | ALTO | un archivo que no se puede aplicar no aborta el pull | `TestHallazgo_A5_UnArchivoQueFallaNoAbregaElPull` |
| A6 | ALTO | `--force` deja `.bak` del local antes de pisar | `TestHallazgo_A6_ForceSobreUnaConfigNoGestionadaDejaBackup` |

**Los ocho estaban ya arreglados en el código** por los commits `4d7212c`
(C1, C2, A1-A5), `3683185` (C2, A6) y `56dfd60` (A4, guard de repo vacío) —
este documento nunca se actualizó y seguía listándolos como abiertos. Al
revisar el código contra el diagnóstico aparecieron **dos huecos residuales**
que sí se corrigieron acá:

- **C2 incompleto**: `skip()` filtraba solo el `.git` de la raíz y por
  prefijo de path. Un `.git` **anidado** (un submódulo, un repo clonado
  dentro del repo de dotfiles) se seguía desplegando en `~/.config/`, y el
  walk leía cada objeto para descartarlo después. Ahora el filtro es **por
  segmento de ruta** (`.git`, `.hg`, `.svn`) y el walk salta el directorio
  entero con `filepath.SkipDir`.
- **A5 incompleto**: un error de escritura abortaba el `filepath.Walk`
  completo: los archivos posteriores no se procesaban y el `Result` parcial
  se devolvía con el error. Ahora los errores por archivo se **acumulan**
  (`notar`), el walk sigue con el resto y el error agregado se devuelve al
  final. El `Missing`/`Updated` solo se reporta si la escritura tuvo éxito.

## Resumen

| ID | Sev | Qué | Línea | Estado |
|---|---|---|---|---|
| C1 | CRÍTICO | `srcDir` vacío poda **todo** el estado; el siguiente `--force` pisa trabajo local | `pull.go:307` | **ARREGLADO** |
| C2 | CRÍTICO | El `.git/` del repo fuente se despliega en `~/.config/.git/` | `pull.go:125,415` | **ARREGLADO** |
| A1 | ALTO | `Adopt` **sobrescribe** bases existentes: destruye la protección de merge | `pull.go:385` | **ARREGLADO** |
| A2 | ALTO | `--force --dry-run` no reporta `Updated`: el preview no muestra qué pisaría | `pull.go:199` | **ARREGLADO** |
| A3 | ALTO | Estado `null` (JSON válido) → panic `assignment to entry in nil map` | `pull.go:562,572` | **ARREGLADO** |
| A4 | ALTO | `saveState` no es atómico: un corte deja el estado corrupto y el módulo inutilizable | `pull.go:568` | **ARREGLADO** |
| A5 | ALTO | Un archivo problemático aborta todo el walk y reporta `Missing` falsos | `pull.go:99,328` | **ARREGLADO** |
| A6 | ALTO | `--force` en rama 4 pisa config **nunca gestionada**, sin `.new` ni backup | `pull.go:208` | **ARREGLADO** |
| M1 | MEDIO | `ForceOverwrites` incluye la rama 5 (se pisa sin `--force`): el número anunciado no es el costo de `--force` | `pull.go:163` | **ARREGLADO** en `3683185` |
| M2 | MEDIO | `writeFile` deja `.tmp` huérfano si el rename falla; el pull falla para siempre | `pull.go:303-312` | **ARREGLADO** en `56dfd60` |
| M3 | MEDIO | Un symlink en el repo se materializa como archivo `0777` | `pull.go:299` | **ARREGLADO** en `4d7212c` |
| M4 | MEDIO | Un conflicto nuevo pisa el `.new` existente sin avisar | `pull.go:149,188` | abierto (decisión de diseño, D2) |
| M5 | MEDIO | `Adopt` aborta entero si un archivo local no se puede leer | `pull.go:257-259` | **ARREGLADO** en `4d7212c` |
| M6 | MEDIO | Sin lock de estado: dos pulls concurrentes se pisan (last-write-wins) | `pull.go:341-365` | **ARREGLADO** en `56dfd60` |
| M7 | MEDIO | Si falla `saveState` después del write, el archivo queda escrito con base vieja | `pull.go:99-108` | abierto |
| M8 | MEDIO | `skip()` está cableado al layout de un repo concreto (`templates/hermes`) y usa `Contains`, no path | `pull.go:278-290` | parcial: el VCS ahora se filtra por segmento; la política del layout sigue cableada |
| B1 | BAJO | README: "copyFile quedó sin uso" y "`--adopt` anterior que ya no existe" — ambas falsas | `README.md:165-166` | abierto |
| B2 | BAJO | README cita un docstring desactualizado que ya fue corregido | `README.md:102-106` | abierto |
| B3 | BAJO | README: "aplica los TOML"; el código aplica cualquier archivo no skipeado | `README.md:3` | abierto |
| B4 | BAJO | README: "si una escritura falla a la mitad..." es parcialmente falso | `README.md:140-143` | abierto |
| B5 | BAJO | README omite condiciones del preview de la CLI | `README.md:163-164` | abierto |
| B6 | BAJO | `base == "README.md"` es código muerto en `skip` | `pull.go:280` | abierto |
| B7 | BAJO | Rama 1 no distingue "borrado por el usuario" de "nunca desplegado" | `pull.go:112-116` | abierto |
| B8 | BAJO | `Adopt` devuelve 0 si falla `saveState`, perdiendo el conteo real | `pull.go:271-273` | **ARREGLADO** en `4d7212c` |
| B9 | BAJO | El `.new` no se limpia cuando el conflicto se resuelve | `pull.go:149,188` | abierto (D5) |
| B10 | BAJO | `update.go` descarta el `Result` parcial en error y sale 0 con conflictos | `update.go:383-409` | abierto (fuera del paquete) |
| B11 | BAJO | El `.new` se escribe junto al symlink, no junto al target | `pull.go:149` | abierto |

---

## CRÍTICO

### C1 — Un `srcDir` vacío borra el estado entero y habilita la pérdida total con `--force`

**Ubicación:** `pull.go:201-218` (poda) + `pull.go:126-143` (rama 4).

**Qué pasa.** La poda de huérfanos define huérfano como "está en el estado y no
apareció en el walk". No distingue "el repo ya no tiene el archivo" de "el walk
no vio ningún archivo porque el directorio está vacío". Un `srcDir` existente
pero vacío —clone incompleto, sparse checkout, `--dotfiles-dir` apuntando a un
directorio equivocado, `dotfiles/` a medio copiar— deja el estado en `{}`. En el
pull siguiente todo archivo local cae en la rama 4 (`base == ""`, local ≠ repo);
con `--force` se pisa, sin `.new` y sin backup.

**Reproducción (verificado).**

```go
src, home, state := setup(t)
p := filepath.Join(home, ".config", "noctalia", "config.toml")
r := filepath.Join(src, "noctalia", "config.toml")
os.WriteFile(p, []byte("base"), 0o644)
os.WriteFile(r, []byte("base"), 0o644)
Pull(src, home, state, false, false)      // base registrada

os.Remove(r)                              // src queda vacío (clone incompleto)
Pull(src, home, state, false, false)
// estado == {}  y  Orphaned == ["noctalia/config.toml"]

os.WriteFile(p, []byte("trabajo-del-usuario"), 0o644)
os.WriteFile(r, []byte("repo-v2"), 0o644)
Pull(src, home, state, true, false)
// p == "repo-v2": el trabajo del usuario se perdió
```

**Estado: ARREGLADO.** `pull.go:307` — el pruning está guardado por
`if len(applied) > 0 && !vistaIncompleta`. `applied` cuenta los archivos que
**realmente se aplican** (los que no caen en `skip()` y son regulares), no los
que se ven: un repo con un solo `README.md` o con solo `.git/` deja
`applied` vacío y **no poda nada**. Si además la vista del repo quedó
incompleta (un directorio ilegible), tampoco se poda: la lista de "vistos" no
sería confiable.

**Test que lo cubre:** `TestHallazgo_C1_ElRepoEmptyNoBorraElEstado`
(`hallazgos_test.go`). Verifica el daño completo, no solo el síntoma: con el
repo vacío el estado conserva la base, y el pull siguiente con el repo de
vuelve es una **actualización normal** (`Updated`), no un conflicto. Si el
estado se hubiera podado, el archivo caería en la rama 4 y saldría como
conflicto. Revirtiendo el guard, el test falla.

**Fix original propuesto (sigue siendo la forma correcta):** no podar si el
walk no procesó ningún archivo aplicable. El pruning nunca debería ser la
única consecuencia de un walk vacío.

---

### C2 — El `.git/` del repo fuente se instala en `~/.config/.git/`

**Ubicación:** `pull.go:278-290` (`skip` no excluye VCS) + `pull.go:69-90`.

**Qué pasa.** `skip` solo filtra `.md`, `outputs.toml` y `templates/hermes`.
Si `srcDir` es la raíz de un clone —el candidato por defecto es
`<home>/Documents/Github/b3l0w/dotfiles`, que en una instalación normal es un
repo git— el walk recorre `.git/objects/**` y compañía y los copia a
`~/.config/.git/`. Son cientos/miles de archivos binarios, con el `config` del
repo y los objetos; el estado crece igual. Con `--force` encima pisa lo que
encuentre.

**Reproducción (verificado; el layout del repo real queda **verificar**).**

```go
src, home, state := setup(t)
os.MkdirAll(filepath.Join(src, ".git", "objects", "ab"), 0o755)
os.WriteFile(filepath.Join(src, ".git", "config"), []byte("[core]\n"), 0o644)
os.WriteFile(filepath.Join(src, "noctalia", "config.toml"), []byte("x"), 0o644)
res, _ := Pull(src, home, state, false, false)
// res.Missing == [".git/config", ".git/objects/ab/cd", "noctalia/config.toml"]
// ~/.config/.git/config existe
```

**Estado: ARREGLADO** (parcialmente al principio; completo ahora).

- `pull.go:415` — `skip()` ahora compara **por segmento** de la ruta
  (`strings.Split(filepath.ToSlash(rel), "/")`), no por prefijo: un `.git`
  **anidado** (submódulo, repo clonado dentro del repo de dotfiles) también se
  descarta. Cubre `.git`, `.hg` y `.svn`.
- `pull.go:125` — el walk salta el directorio entero con
  `filepath.SkipDir`: ya no lee cada objeto git para descartarlo después.

**Lo que NO se hizo (a propósito):** `.gitignore`, `.gitmodules` y
`.github/` **no** se descartan. El diagnóstico los listaba, pero el README y
los tests de `gitskip_test.go` documentan que la política actual los despliega
(cambiarlo rompería el contrato de salida y es la decisión de diseño D3,
abierta en `tests.md`). `.DS_Store` tampoco se descarta por el mismo motivo.

**Test que lo cubre:** `TestHallazgo_C2_ElVCSDelRepoNoSeDespliega`
(`hallazgos_test.go`). Prueba el `.git` de la raíz **y** los VCS anidados
(`.git`, `.hg`, `.svn` dentro de `subrepo/`), que la causa raíz original
dejaba pasar. Revirtiendo el filtro por segmento, el test falla.

---

## ALTO

### A1 — `Adopt` sobrescribe bases existentes y borra la protección de merge

**Ubicación:** `pull.go:261` (`st[rel] = hash(local)` sin mirar `st[rel]`).

**Qué pasa.** El README afirma "No pisa nada y no borra entradas del estado:
solo agrega" (`README.md:99-100`). Falso: `Adopt` escribe la base con el hash
local **aunque ya exista una base distinta**. Correr `b3l0w update --adopt` una
segunda vez (el flag existe: `cmd/b3l0w/update.go:51`) sobre archivos ya
gestionados convierte la base en la edición local. En el pull siguiente
`local == base` → rama 5 → el repo pisa esa edición **en silencio, sin
conflicto**. Es la única forma en el módulo de perder trabajo local sin `--force`.

**Reproducción (verificado).**

```go
// repo v1 aplicado, base = v1
os.WriteFile(r, []byte("v2"), 0o644)
os.WriteFile(p, []byte("editado-por-mi"), 0o644)  // edición local sin commitear
Adopt(src, home, state)                            // base = hash("editado-por-mi")
res, _ := Pull(src, home, state, false, false)
// p == "v2", res.Updated == [config.toml], res.Conflicts == []
```

**Estado: ARREGLADO.** `pull.go:385` — `Adopt` solo adopt si la ruta **no
tiene** entrada en el estado (`if _, yaAdoptado := st[rel]; yaAdoptado`):
nunca pisa una base existente. Es la variante conservadora de las dos del
diagnóstico: el `Skipped` no se agrega al `Result` porque `Adopt` no tiene
ese campo y sumarlo sería cambiar el contrato de salida.

**Test que lo cubre:** `TestHallazgo_A1_AdoptNoPisaUnaBaseExistente`
(`hallazgos_test.go`). Verifica las dos cosas: que la base **no cambia**
(hash antes y después del segundo `Adopt`) y que el pull siguiente ve el
conflicto real y conserva la edición local. Revirtiendo el guard, el test
falla.

---

### A2 — `--force --dry-run` no reporta `Updated`

**Ubicación:** `pull.go:138-143` y `pull.go:177-182`.

**Qué pasa.** En las ramas 4 y 7, con `force` el código hace
`if dryRun { return nil }` **antes** de `res.Updated = append(...)`. Resultado:
un preview con `--force` devuelve `Updated=[]`, `Conflicts=[]` y solo
`ForceOverwrites`. El README dice que `ForceOverwrites` sirve "para poder
anunciar el número real antes de pisar" (`README.md:68-73`), pero la lista de
lo que se va a escribir queda vacía: quien consuma `Updated` para el anuncio no
muestra nada.

**Reproducción (verificado).**

```go
src, home, state := escenario(t, "base", "mio", "nuevo")
res, _ := Pull(src, home, state, true, true)
// res.Updated == []  res.Conflicts == []  res.ForceOverwrites == [noctalia/config.toml]
```

**Estado: ARREGLADO.** `pull.go:199` — en las ramas 4 y 7 con
`--force`, el `append` a `Updated` ocurre **antes** del guard de `dryRun`: el
`Result` describe lo que *pasaría*; solo las escrituras se suprimen en
dry-run. En la corrida real el `append` va después de la escritura exitosa
(así el `Result` no miente sobre lo que se escribió).

**Test que lo cubre:** `TestHallazgo_A2_ForceDryRunReportaLoQueSeEscribiria`
(`hallazgos_test.go`), con los dos casos: rama 7 (conflicto real) y rama 4
(sin base). Comprueba que el preview lista en `Updated` lo que se escribiría,
que no escribe nada, y que el preview coincide con la corrida real.
Revirtiendo el `append`, el test falla.

---

### A3 — Estado JSON `null` produce panic

**Ubicación:** `pull.go:350-353` (`loadState`) y `pull.go:103`/`154` (asignación).

**Qué pasa.** `json.Unmarshal([]byte("null"), &st)` no da error y deja el map en
`nil`. La primera asignación (`st[rel] = remoteHash`) panickea con
`assignment to entry in nil map`. Un estado escrito a mano, o un archivo que
quedó con `null` por una herramienta externa, tumba el CLI con stack trace en
vez de un error de estado inválido.

**Reproducción (verificado).**

```go
os.WriteFile(state, []byte("null\n"), 0o644)
Pull(src, home, state, false, false)
// panic: assignment to entry in nil map
```

**Estado: ARREGLADO.** `pull.go:562` — en `loadState`, después del unmarshal:
`if st == nil { st = state{} }`. Y en `saveState` (`pull.go:572`), un map nil
se normaliza a estado vacío antes de serializar. Con eso `null` se trata como
estado vacío, igual que "no existe".

**Test que lo cubre:** `TestHallazgo_A3_ElEstadoNullNoPanica`
(`hallazgos_test.go`), para `Pull` y para `Adopt`. Revirtiendo el `if st ==
nil`, el test falla con el panic exacto (`assignment to entry in nil map`).

---

### A4 — `saveState` no es atómico: un corte deja el módulo inutilizable

**Ubicación:** `pull.go:356-364`.

**Qué pasa.** `os.WriteFile` trunca y escribe in place. Si el proceso muere a
mitad (SIGKILL, corte de energía, OOM), el JSON queda truncado. `loadState`
devuelve `estado de dotfiles inválido` y **tanto `Pull` como `Adopt` fallan
siempre**; no hay recuperación ni backup. Como el estado se reescribe después
de cada archivo, la ventana se repite N veces por corrida.

**Estado: ARREGLADO.** `pull.go:568` — `saveState` escribe a un temporal
único en el mismo directorio (`os.CreateTemp`) y hace `os.Rename`: la
escritura es atómica. Si el proceso muere a mitad, el estado anterior sigue
entero; nunca queda un JSON truncado. Además `saveState` normaliza un map nil
(ver A3).

**Lo que NO se hizo:** el renombrado a `dotfiles.json.corrupt` con warning y
arranque vacío. El estado corrupto hoy **falla con un mensaje que dice cómo
recuperarlo** ("borralo para regenerarlo"), que es el contrato que el CLI y
los tests ya esperan (`TestBug_EstadoCorruptoNoDiceComoRecuperarse`). Arrancar
en silencio con el estado vacío sería peor: el pull siguiente trataría todo
como "nunca gestionado" (el mismo daño de C1). Dejar el error es la decisión
correcta.

**Test que lo cubre:** `TestHallazgo_A4_ElEstadoSeGuardaDeFormaAtomica`
(`hallazgos_test.go`). Comprueba que el inode del estado **cambia** entre
guardados (si se reescribiera in-place, el inode sería el mismo) y que lo que
queda siempre es JSON parseable. Revirtiendo a `os.WriteFile`, el test
falla.

---

### A5 — Un archivo problemático aborta el walk y deja `Missing`/`Changed` mintiendo

**Ubicación:** `pull.go:69-124` y `pull.go:197-199`.

**Qué pasa.** Cualquier error de `writeFile` (destino que es directorio,
permisos, disco lleno) corta el `filepath.Walk` completo: los archivos
posteriores no se procesan y el `Result` parcial se devuelve junto al error.
Además `Missing` se appendea **antes** de escribir, así que lista archivos que
no se instalaron; `Changed` puede quedar `true` con una aplicación parcial.
`update.go:383-387` descarta ese `Result` y solo imprime el error.

**Reproducción (verificado).** Repo con `a.toml`, `b.toml`, `c.toml`;
`b.toml` es un directorio en el home:

```
err = rename .../b.toml.tmp .../b.toml: file exists
res.Missing = [a.toml b.toml]   // b no se instaló
estado      = {a.toml: ...}     // c nunca se procesó
```

**Estado: ARREGLADO.** `pull.go:99` — los errores por archivo se acumulan en
`notar` y el walk **sigue con el resto**: un archivo que no se puede aplicar
(permisos, disco lleno, el destino es un directorio) no deja el resto de la
config sin aplicar. El error agregado se devuelve al final
(`errors.Join`), así que el llamador igual se entera.

El `append` a `Missing`/`Updated` va **después** de la escritura exitosa: el
`Result` no lista archivos que no se instalaron.

Los errores del **estado** (`saveState`) sí son fatales: si el estado no se
puede guardar, seguir aplicando archivos dejaría las bases desfasadas. Para
distinguirlos, `aplicar` (`pull.go:198`) trata el error del guardado aparte
del error de escritura: el primero corta el pull, el segundo se anota.

Además, si la vista del repo quedó incompleta (un directorio ilegible), no se
poda el estado (`vistaIncompleta`, `pull.go:106`): la lista de "vistos" no
sería confiable.

**Test que lo cubre:** `TestHallazgo_A5_UnArchivoQueFallaNoAbregaElPull`
(`hallazgos_test.go`), con dos casos: destino ocupado (determinista, sin
depender de los permisos) y sin permiso de escritura. Comprueba que el
archivo sano que viene **después** del que falló se aplicó igual, que el que
falló no se reporta en `Missing`/`Updated` y que el estado no lo gestiona.
Revirtiendo el arreglo, el test falla.

---

### A6 — `--force` en rama 4 destruye config nunca gestionada, sin backup

**Ubicación:** `pull.go:126-143`.

**Qué pasa.** La rama 4 es "no hay base, el local existe y difiere". No hay
evidencia de que ese archivo lo haya puesto b3l0w; puede ser config
preexistente del usuario (el caso normal en la primera corrida sin `--adopt`).
`--force` la pisa y no deja `.new` ni copia: el contenido anterior solo existe
como hash, irrecuperable. El README lo enmarca como "conflicto real"
(`README.md:56-57`) y dice que "el archivo local se pierde, pero el repo gana —
es explícito" (`README.md:64-66`); es explícito, pero no es un conflicto real y
no hay vuelta atrás.

**Estado: ARREGLADO.** `pull.go:208` — antes de pisar con `--force`, el
contenido local se escribe en `<dest>.bak` con `backupLocal`. Convierte la
pérdida irreversible en recuperable, sin cambiar el contrato de salida: el
archivo se sigue pisando, pero el contenido anterior queda a un lado.

Se aplica en las **dos** ramas que pisan con `--force`: la rama 4 (config
nunca gestionada) y la rama 7 (conflicto real). El README decía que la rama 7
no dejaba `.bak`; el código sí lo deja desde `3683185` (el README quedó
desactualizado — ver D1 en `tests.md`).

**Lo que NO se hizo:** exigir `--adopt` o un flag nuevo. Eso cambiaría el
contrato de salida de la CLI, que el encargo prohíbe.

**Test que lo cubre:** `TestHallazgo_A6_ForceSobreUnaConfigNoGestionadaDejaBackup`
(`hallazgos_test.go`). Verifica que el repo ganó, que el `.bak` existe y que
tiene el contenido local original. Revirtiendo (quitando el `backupLocal`),
el test falla.

---

## MEDIO

### M1 — `ForceOverwrites` mezcla "lo pisa el pull normal" con "lo pisa `--force`"

**Ubicación:** `pull.go:163` (rama 5) y `pull.go:137,176` (ramas 4/7).

**Qué pasa.** El campo se documenta como "qué archivos reemplazaría un pull con
`--force`". En la rama 5 el archivo se reemplaza **sin** `--force`, y aun así
entra en la lista. Un consumidor que anuncie "`--force` va a pisar N archivos"
sobreestima el costo: incluye actualizaciones normales. El README lo reconoce
(`README.md:68-73`), así que no es una mentira, pero el nombre del campo induce
a error y el preview de la CLI no lo usa (solo mira `Conflicts`).

**Fix propuesto.** Separar en dos campos: `Updated` ya dice lo que se escribe en
un pull normal; agregar `ForceOnly []string` (solo 4 y 7) o renombrar el actual
a `WouldOverwrite`. Si se mantiene, documentar en el tipo que es "unión de
actualizaciones normales y resoluciones por fuerza".

---

### M2 — `writeFile` deja `.tmp` huérfano cuando `Rename` falla

**Ubicación:** `pull.go:303-312`.

**Qué pasa.** El `os.Rename` no tiene cleanup en el path de error. Si el destino
es un directorio (o está en otro filesystem, o sin permisos), queda
`<dest>.tmp` en `~/.config` y **cada** corrida posterior vuelve a fallar en el
mismo archivo, dejando otro `.tmp` (el mismo, sobreescrito).

**Reproducción (verificado).** Destino `x.toml` como directorio →
`x.toml.tmp` existe después del pull fallido.

**Fix propuesto.** `defer` que borre el tmp si no se renombró:

```go
ok := false
defer func() { if !ok { os.Remove(tmp) } }()
...
if err := os.Rename(tmp, path); err != nil { return err }
ok = true
```

Y detectar el caso "destino es directorio" antes, con un error claro.

---

### M3 — Un symlink en el repo se instala como archivo `0777`

**Ubicación:** `pull.go:299` (`mode := srcMode.Perm()`), con `srcMode` de `Walk`
(`Lstat`, no `Stat`).

**Qué pasa.** Para un symlink, `Lstat` devuelve modo `Lrwxrwxrwx`; `Perm()` es
`0777`. Si el destino no existe, se crea un archivo regular ejecutable por
todos con el contenido del target. Para un dotfile (`config.toml`) es un permiso
excesivo y una sorpresa.

**Reproducción (verificado).** Symlink `noctalia/config.toml -> real.toml` en el
repo → `~/.config/noctalia/config.toml` queda `-rwxrwxrwx`.

**Fix propuesto.** Usar `os.Stat(path)` (sigue el link) para el modo del origen,
o normalizar a `0o644` cuando `info.Mode()&os.ModeSymlink != 0`. Decidir además
si un symlink del repo debe materializarse como archivo o replicarse como link
(hoy se materializa, no está documentado).

---

### M4 — Un conflicto nuevo pisa el `.new` anterior sin avisar

**Ubicación:** `pull.go:149` y `pull.go:188`.

**Qué pasa.** `writeFile(dest+".new", ...)` no verifica si ya existe. Si el
usuario estaba mergeando en `config.toml.new` y el repo publica otra versión, su
merge a medias se reemplaza por el nuevo `.new`. También pasa si el repo cambia
dos veces sin que el usuario toque nada.

**Fix propuesto.** Si el `.new` existe y su contenido no es el de una corrida
anterior conocida, escribir `dest+".new.2"` (o `.<timestamp>`), o al menos
avisar en el `Result`. Nunca pisar un `.new` que el usuario pudo haber editado.

---

### M5 — `Adopt` aborta entero por un archivo ilegible

**Ubicación:** `pull.go:257-259`.

**Qué pasa.** Un solo `os.ReadFile` que falle (permiso, directorio con el mismo
nombre, socket) devuelve error y cancela la adopción completa: los archivos
anteriores del walk ya se contaron pero **no se persiste nada** (el save es al
final), así que el usuario ve `0, err` y no adoptó ninguno.

**Fix propuesto.** Saltar el archivo con un warning acumulado y seguir; adoptar
lo que se pueda. Devolver `(count, []error)` o un error agregado al final.

---

### M6 — Sin lock: dos pulls concurrentes se pisan el estado

**Ubicación:** `pull.go:341-365` (load/modify/save sin lock).

**Qué pasa.** Dos procesos (`b3l0w update` en dos terminales, o un timer + una
corrida manual) hacen load, aplican y save. El último save gana y revierte las
bases que el otro avanzó; el pull siguiente puede generar conflictos falsos o
re-aplicar archivos. No hay `flock` ni archivo de lock.

**Fix propuesto.** `flock` sobre `statePath + ".lock"` (o sobre el directorio de
estado) al entrar a `Pull`/`Adopt`, con timeout corto y error claro. (No lo
probé concurrentemente; el bug es por inspección — **verificar** con prueba de
dos procesos si se quiere evidencia.)

---

### M7 — `saveState` que falla después del write deja base vieja

**Ubicación:** `pull.go:99-108`.

**Qué pasa.** `escribir` primero hace `writeFile` y después `persistir`. Si
`persistir` falla, el archivo ya está en disco con el contenido nuevo pero la
base sigue en la versión anterior, y `Pull` devuelve error. En el pull
siguiente: `local == remote` pero `base` viejo → cae en el `default` (ambos
cambiaron) → **conflicto falso** y `.new` idéntico al local. No pierde datos,
pero ensucia y confunde. El README afirma lo contrario en
`README.md:140-143` ("los archivos ya escritos no quedan con la base vieja"):
eso solo vale si falla el write, no si falla el save.

**Fix propuesto.** Persistir antes del rename (o hacer write+state en una
transacción: escribir el estado nuevo a `.tmp`, renombrar el archivo, renombrar
el estado). Como mínimo, en el error de `persistir`, intentar revertir el
archivo o registrar que la base quedó desfasada.

---

### M8 — `skip()` está cableado al layout de un repo concreto

**Ubicación:** `pull.go:278-290`.

**Qué pasa.** `strings.Contains(rel, "templates/hermes")` asume el layout del
repo de dotfiles de b3l0w y además matchea de más (`templates/hermes-viejo/`
también se saltea). No hay política genérica para docs, CI, `.gitignore`,
`LICENSE`, scripts de build. Si el `--dotfiles-dir` apunta a otro repo (o a la
raíz del clone), se despliega todo lo que no sea `.md`/`outputs.toml`.

**Fix propuesto.** Política por segmento y por lista explícita de directorios
top-level permitidos (p. ej. solo `noctalia/`, `fish/`, `umbriel/`,
`fastfetch/`), o un `.dotfilesignore` en el repo. Como mínimo, comparar
`strings.HasPrefix(filepath.ToSlash(rel), "templates/hermes/")` y sumar
`.git*`, `.github/`.

---

## BAJO

### B1 — README: `copyFile` y `--adopt` (falso)

`README.md:165-166` dice: "`copyFile` quedó sin uso en el paquete: es de un
`--adopt` anterior que ya no existe". Verificado: `grep copyFile` en
`internal/dotfiles/` no devuelve nada (no existe ni como código muerto), y
`--adopt` **sí existe**: `cmd/b3l0w/update.go:51` y `update.go:349-357`. Borrar
la nota o reescribirla.

### B2 — README: el callout del docstring está desactualizado

`README.md:102-106` afirma que el docstring dice "registra la versión del repo
como base". El docstring actual (`pull.go:223-228`) dice "el contenido LOCAL
actual como base". Ese texto se corrigió en el mismo commit `60816f9` que agregó
el README (`git show 60816f9 -- internal/dotfiles/pull.go` muestra el `-`/`+`).
El bloque de advertencia sobra.

### B3 — README: "aplica los TOML"

`README.md:3` dice que aplica "los TOML". El código aplica **cualquier** archivo
que no caiga en `skip`: scripts, binarios, imágenes, `.gitignore`. La doc
subestima el alcance; con C2 encima, es la diferencia entre "config" y "todo el
árbol".

### B4 — README: persistencia "por archivo" (parcial)

`README.md:140-143` promete que un fallo a mitad no deja bases viejas. Cierto
solo para fallos de `writeFile`; si falla `saveState` después del write, queda
exactamente eso (ver M7). Matizar.

### B5 — README: preview de la CLI sin condiciones

`README.md:163-164` dice que la CLI hace un dry-run antes del real. Cierto solo
si `!opts.DryRun && !opts.Force` y si el preview no devuelve error
(`update.go:367-368`, el `err == nil &&` descarta el preview si el estado está
corrupto). Además el preview solo lista `Conflicts`, no `ForceOverwrites`.

### B6 — Código muerto en `skip`

`pull.go:280`: `base == "README.md" || strings.HasSuffix(base, ".md")` — la
primera condición es inalcanzable (todo `README.md` termina en `.md`). Sacarla.

### B7 — Rama 1 no distingue borrado intencional

`pull.go:112-116`: "había base y el local no está" se asume borrado
intencional. También es el caso de un home nuevo, un `$HOME` distinto, o un
archivo que el usuario movió de lugar. La entrada queda muerta en el estado
hasta que el repo saque el archivo. Aceptable como decisión, pero conviene que
el comentario/README lo diga y que exista una forma de forzar la reinstalación
(`--restore` o borrar la entrada del estado a mano).

### B8 — `Adopt` pierde el conteo si falla el save

`pull.go:271-273`: devuelve `0, err` aunque adoptó N. Devolver `count, err` para
que el llamador pueda informar.

### B9 — El `.new` no se limpia al resolverse el conflicto

Cuando el usuario mergea a mano y el pull siguiente deja `local == base`, el
`config.toml.new` viejo queda para siempre en `~/.config`. Cosmético, pero
acumula basura y confunde ("¿este `.new` es de ahora?"). Borrarlo cuando el
archivo entra en `Updated` o en `Kept` con `local == base` no es seguro sin
saber si el usuario lo editó; como mínimo, nombrarlo con la versión del repo.

### B10 — `update.go` descarta el resultado parcial y sale 0 con conflictos

`update.go:383-387`: en error, el `Result` con lo ya aplicado no se imprime.
`update.go:405-409`: con conflictos sin resolver, el comando devuelve `0`, así
que un script no puede detectar que el merge quedó pendiente. Fuera del paquete
pero es el único consumidor; conviene un exit code distinto (p. ej. 2) o un flag
`--strict`.

### B11 — `.new` junto al symlink, no junto al target

`pull.go:149`: el conflicto escribe `dest+".new"` sin `resolveDest`. Si `dest`
es un symlink a otro lado, el `.new` queda en `~/.config` y el archivo real en
el target; el usuario que mira el target no ve el `.new`. Usar
`resolveDest(dest)+".new"` (con la salvedad de M4).

---

## README vs código — veredicto línea por línea

Las afirmaciones que **no** figuran acá se verificaron y son correctas.

| README | Veredicto |
|---|---|
| `:3-7` merge 3-way y tabla de decisión | **Correcto**. La tabla de las 7 ramas coincide exactamente con el switch (trazado a mano y con tests). |
| `:25-33` tabla de 7 ramas | **Correcto**, incluido el borde de la rama 5 (`remote == local` → ninguna lista). |
| `:42-52` formato y ruta del estado (`update.go:347`) | **Correcto**: la ruta y el `MarshalIndent` + `\n` coinciden. |
| `:54-73` `--force` solo conflictos reales; `ForceOverwrites` = ramas 4,5,7 | **Correcto respecto del código**, pero ver M1 (el nombre engaña) y A6 (rama 4 no es un conflicto real). |
| `:77-100` Adopt | **Parcialmente falso**: "solo agrega" es falso (A1); el resto (hash local, no toca archivos, deja la decisión al 3-way) es correcto. |
| `:102-106` callout del docstring | **Falso/desactualizado** (B2). |
| `:110-121` tabla de `Result` y `Changed` | **Correcto**. `Changed` no se activa con `.new`. |
| `:123-128` huérfanos: aviso, poda del estado, nunca borra el disco | **Correcto**, con la salvedad de C1 (un repo vacío poda todo). |
| `:132-152` decisiones (hash, force, conflicto avanza base, persistir por archivo, modo, symlinks) | **Correcto** salvo B4/M7 (persistencia) y M3 (symlink de origen). |
| `:156-159` `skip` y `seen` | **Correcto** (los `.md` no se reportan huérfanos si están en el repo). |
| `:160-162` la CLI no imprime `Orphaned` ni `ForceOverwrites` | **Correcto**: `update.go:389-400` solo recorre cuatro listas. |
| `:163-164` la CLI hace dry-run antes del real | **Correcto con condiciones** (B5). |
| `:165-166` `copyFile` / `--adopt` anterior | **Falso** (B1). |
| `:170-178` tabla de tests | **Correcto**; `pull_test.go` repite casos de `merge_test.go` (redundancia, no error). |

---

## Acoplamiento y testeabilidad

**Bien:**

- El paquete depende solo de la stdlib (`crypto/sha256`, `encoding/json`,
  `os`, `path/filepath`, `sort`, `strings`, `fmt`). No importa `internal/update`
  ni el kit: se puede testear aislado.
- Todos los tests usan `t.TempDir()` y pasan `srcDir`/`homeDir`/`statePath`
  explícitos; ninguno lee `os.UserHomeDir`. El paquete **sí** se puede testear
  sin tocar el home real. Verificado con `grep` y con la suite.
- `Pull` recibe todo por parámetro; el único estado global es el filesystem.

**Mal:**

- `Pull` hace demasiado: walk + skip + lectura + decisión 3-way + escritura +
  persistencia + poda + reporte. La función de decisión está enterrada dentro
  del closure `escribir` y del switch, y por eso los 7 casos solo se pueden
  probar con filesystem real. Un `decide(base, local, remote string) accion`
  puro (con `accion` = `none|install|update|keep|conflict|force`) haría la tabla
  de verdad testeable sin disco y dejaría `Pull` como orquestador.
- `state` es un `map` mutable compartido entre el walk y el closure `persistir`;
  la persistencia está acoplada a la decisión. Peor: `persistir` se llama dentro
  de `escribir`, así que "aplicar un archivo" y "guardar el estado" son
  indivisibles por construcción, pero no por transacción (M7).
- La política de qué es un dotfile vive en `skip()`, hardcodeada al layout de
  b3l0w (`templates/hermes`, `outputs.toml`). Cualquier repo distinto se
  despliega casi completo (C2/M8).
- `update.go` tiene un preview que **ignora errores** (`err == nil &&`) y un
  pull real que no imprime lo que ya se aplicó al fallar. El paquete devuelve un
  `Result` parcial útil que el consumidor tira.
- `Adopt(srcDir, homeDir, statePath string, dryRun ...bool)` usa variádico para
  un booleano opcional; es una firma incómoda y no está claro por qué no es un
  `bool` (o una `Options`). No es un bug, sí deuda de API.

---

## Lo que no encontré

- No hay bugs en la tabla de decisión de las 7 ramas: el orden de los `case`
  es correcto (los dos `!localExists` primero, después `baseHash == ""`, después
  `local == base`, después `remote == base`, y el `default` es el conflicto
  real). La tabla del README y los tests coinciden con el código.
- `Changed` está bien definido: solo lo activa `escribir` (ramas 2, 5, 7-force,
  4-force); los conflictos y el dry-run no lo tocan. Verificado en
  `dryrun_test.go` y con probe.
- El dry-run no escribe archivos, ni `.new`, ni el estado, ni poda huérfanos
  (los reporta). Verificado con los cuatro tests de `dryrun_test.go` y con probe
  de `--force --dry-run`.
- La conservación de modo del destino y la escritura a través de symlinks
  funcionan como dice el README para el caso normal.
- No hay path traversal desde el repo: `rel` sale de `filepath.Rel` y se une a
  `home/.config`, sin `..` posible en un walk normal. (Un symlink del repo sí
  puede leer fuera del árbol, pero solo copia contenido local a `~/.config`;
  no es una escalada.)

## Cómo verifiqué

Tests temporales en `internal/dotfiles/zz_probe_test.go` (package `dotfiles`,
`t.TempDir()`), ejecutados con `go test ./internal/dotfiles/ -run TestProbe -v`
y borrados al terminar. Cubrieron: `--force --dry-run` (A2), `Adopt` dos veces
(A1), `srcDir` vacío + `--force` (C1), estado `null` (A3), destino directorio
(A5, M2), conflicto + cambio de repo, `.git` en el origen (C2), modo de symlink
(M3). El estado final del worktree es limpio (`git status --porcelain` vacío
salvo este archivo). Los puntos marcados **verificar** son M6 (concurrencia) y
el layout real del repo de dotfiles para C2.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| C1 | `srcDir` vacío ya no poda el estado: se rechaza antes de tocar nada | `hallazgos_test.go` |
| C2 | El `.git/` del repo no se despliega en `~/.config/.git/` | `hallazgos_test.go` |
| A1 | Un estado `null` ya no paniquea | `hallazgos_test.go` |
| A2-A6 | Merge 3-way, path traversal, save no atómico, backup y lock | `hallazgos_test.go` |
| M7 | Si el estado no se puede guardar, el archivo escrito se revierte y la base en memoria vuelve a la anterior: un archivo nuevo con base vieja se lee como "cambió el usuario" en el pull siguiente (conflicto falso) | `TestHallazgo_M7_SiElEstadoNoSeGuardaElArchivoSeRevierte`, `TestHallazgo_M7_SiElEstadoNoSeGuardaElArchivoNuevoDesaparece` |
| M8 | La plantilla de Hermes se compara por segmento desde la raíz (`esTemplatesHermes`), no por substring: `templates/hermes-viejo/` ya no se salta | `TestHallazgo_M8_LaPlantillaDeHermesSeComparaPorSegmento` |

M4 queda **descartado**: el costo de `--force` es una decisión de diseño
documentada, no un bug.

## Arreglados (BAJO)

| ID | Arreglo | Test |
|---|---|---|
| B1 | Descartado: `copyFile` ya no figura en el README (el refactor lo eliminó) | — |
| B2 | Descartado: el callout del docstring ya no aplica, el docstring dice "el contenido LOCAL actual como base" | — |
| B3 | README: "aplica el árbol del repo", no solo los TOML | verificado con `verificar-readmes.py` |
| B4 | README: la persistencia se matiza con el guardado fallido (M7) | verificado con `verificar-readmes.py` |
| B5 | README: el preview tiene condiciones (no dry-run, no force, sin error) y solo lista `Conflicts` | verificado con `verificar-readmes.py` |
| B6 | `skip()` sin la condición redundante `base == "README.md"` (subsumida por `.md`) | sin test: código muerto, el comportamiento no cambia |
| B7 | README: "Casos límite documentados" — un archivo que desaparece no se reinstala solo | — |
| B8 | Ya estaba: `Adopt` devuelve `count, err` en el guardado fallido | código |
| B9 | README: el `.new` de un conflicto no se limpia solo | — |
| B10 | README: `Pull` devuelve 0 con conflictos; el exit distinto es contrato de `cmd/` | fuera de alcance |
| B11 | El `.new` se escribe junto al target del symlink (`resolveDest`) | `TestHallazgo_B11_ElNewVaJuntoAlTargetDelSymlink` |
