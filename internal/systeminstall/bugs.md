# BUGS — systeminstall

Revisión de integridad del módulo en la rama `mod/systeminstall`
(commit `8a3b9f2`, "test(systeminstall): inyeccion de Runner y tests sin
tocar el sistema"). Leído contra el código, no contra la intención.

Severidad: **ALTO** (puede romper la máquina o perder datos), **MEDIO**
(comportamiento incorrecto o contrato roto), **BAJO** (limpieza, robustez o
documentación).

Verificado con: `go vet ./internal/systeminstall/...` (limpio),
`go test ./internal/systeminstall/...` (pasa), `go build ./...` (OK), y un
probe aparte de `ExpandHome` + `os.RemoveAll` sobre un directorio temporal.

---

## ALTO-1 — Apply reinstala dotfiles que ya están en su lugar

**Archivo:** `internal/systeminstall/apply.go:149-156` (con
`internal/systeminstall/plan.go:98-110`)

**Qué pasa.** `Build` mete en `plan.Files` **todos** los archivos del perfil,
los tenga o no la máquina (`plan.go:98-110` no filtra por `DestExists`).
`Apply` los recorre todos y llama a `installDotfile` sin mirar
`DestExists` (`apply.go:149-156`). O sea: cada corrida backs up y vuelve a
copiar encima los dotfiles que ya estaban.

Con el perfil `desktop` (4 dotfiles: `~/.config/fish`, `~/.config/noctalia`,
`~/.config/umbriel`, `~/.config/fastfetch`) cada
`b3l0w install system --apply` genera **cuatro backups nuevos** en el home
(`~/.config/fish.bak-20261111-031500`) y pisa las ediciones locales del
usuario sin avisar. Los backups nunca se limpian: se acumulan.

El contrato del módulo dice lo contrario en dos lugares: el doc de `Plan`
("lo que falta para que la máquina cumpla la definición", `plan.go:23`) y
`Plan.Empty()`/`filesMissing()` (`plan.go:35-48`), que solo cuentan un
archivo como pendiente si tiene origen y le falta destino. O sea: el plan
dice "no hay nada que hacer" y `Apply` hace cosas.

**Cómo se reproduce.** `Build` con un perfil de un dotfile cuyo destino ya
existe → `plan.Empty()` da `true` → `Apply` igual ejecuta `cp -a <dest>
<dest>.bak-<ts>` y `cp -a <src> <dest>`. Con el `fakeRunner` se ve en
`fake.calls`; el borrado real lo hace `os.RemoveAll` (`apply.go:195`).

**Fix propuesto.** En `Apply`, saltear los archivos que ya están:

```go
for _, file := range plan.Files {
	if !file.SrcExists {
		return fmt.Errorf("falta el origen del dotfile: %s", file.Src)
	}
	if file.DestExists {
		continue // ya está: no se toca
	}
	if err := installDotfile(run, file); err != nil {
		return err
	}
}
```

Si se quiere conservar la semántica de "el perfil es declarativo y se
re-aplica siempre", entonces `Empty()` y `filesMissing()` tienen que dejar
de decir que no hay trabajo, y hay que podar los backups viejos.

---

## ALTO-2 — `os.RemoveAll(dest)` sin validar el destino puede borrar el home

**Archivo:** `internal/systeminstall/apply.go:189-199` (destino en
`apply.go:103`, `kitsystem.ExpandHome` en `kit/system/system.go:134-143`)

**Qué pasa.** El destino del dotfile sale del perfil (definición remota de
GitHub o `--file`) y se usa tal cual, solo con `~` expandido. `ExpandHome`
con `~` pelado devuelve **exactamente `$HOME`** (verificado: `ExpandHome("~")`
→ `/home/b3low-z3r0`; `~` + `/` también, porque `filepath.Join` lo limpia).
Si el destino es `$HOME`, `installDotfile` hace:

1. `cp -a $HOME $HOME.bak-<ts>` (copia el home entero),
2. `os.RemoveAll($HOME)` → **borra el home del usuario** (verificado:
   `os.RemoveAll` sobre un directorio con contenido lo elimina),
3. `cp -a <src> $HOME`.

Lo mismo con `dest: "/"` (borra el sistema de archivos entero). No hay
ninguna validación: el `dest` del perfil actual es `~/.config/...`, pero
cualquier definición —remota o local— puede pedir otra cosa.

**Cómo se reproduce.** Definición local con `{"src":"x","dest":"~"}` →
`b3l0w install system --file=esa.json --apply`. En un sandbox: un
directorio temporal con un archivo adentro, `os.RemoveAll` lo deja vacío.
No se probó contra `$HOME` real (no se toca la máquina).

**Fix propuesto.** Validar el destino antes de borrar, y reemplazar el
`cp`+`RemoveAll` por un `os.Rename` (atómico y sin copia intermedia):

```go
func destinoSeguro(dest string) error {
	if dest == "" || dest == "/" || dest == os.Getenv("HOME") {
		return fmt.Errorf("destino de dotfile peligroso: %s", dest)
	}
	home, _ := os.UserHomeDir()
	if home != "" && !strings.HasPrefix(dest, home+string(os.PathSeparator)) {
		return fmt.Errorf("el destino está fuera del home: %s", dest)
	}
	return nil
}
```

y en `installDotfile`: `os.Rename(file.Dest, backup)` en vez de
`cp -a` + `RemoveAll`.

---

## MEDIO-1 — flatpak siempre va con `-y`, `Options.Yes` se ignora

**Archivo:** `internal/systeminstall/apply.go:115`

**Qué pasa.** `args := append([]string{"install", "-y", "flathub"}, ...)`:
el `-y` está fijo. El doc de `Options.Yes` (`apply.go:18-20`) dice "Yes pasa
`--noconfirm` a pacman y `-y` a flatpak (instalador desatendido). Sin esto,
pacman pregunta lo que tenga que preguntar" — o sea, el `-y` de flatpak
debería venir con `Yes`. En la práctica flatpak nunca pregunta, aunque el
usuario no haya pedido desatendido.

**Cómo se reproduce.** `Apply` con un plan de flatpak y `Options{Yes: false}`
→ el runner recibe `flatpak install -y flathub <app>`. Con `Yes: true` el
comando es idéntico.

**Fix propuesto.**

```go
args := []string{"install"}
if opts.Yes {
	args = append(args, "-y")
}
args = append(args, "flathub")
args = append(args, plan.Flatpak...)
```

---

## MEDIO-2 — el error de `flatpak remote-add` se descarta

**Archivo:** `internal/systeminstall/apply.go:113-114`

**Qué pasa.** `_ = run.Interactive("flatpak", "remote-add", ...)`: si el
remote-add falla (sin flatpak instalado, sin red, sin permisos), el error se
tira y el paso siguiente (`flatpak install ... flathub`) falla con
`flatpak falló: ...`, que no dice la causa real.

**Cómo se reproduce.** Con el `fakeRunner`, fallar `"flatpak remote-add
--if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo"` y
ver que el error que sale es el de `flatpak install`.

**Fix propuesto.** Guardar el error y devolverlo con contexto:

```go
if err := run.Interactive("flatpak", "remote-add", "--if-not-exists", "flathub", url); err != nil {
	return fmt.Errorf("no se pudo agregar el remote flathub: %w", err)
}
```

---

## MEDIO-3 — el paso de plugins no pasa por el Runner

**Archivo:** `internal/systeminstall/apply.go:159` (implementación en
`apply.go:52-62`, `internal/plugins/source.go:19-31,152-158`)

**Qué pasa.** La costura `Runner` cubre las consultas y todos los comandos
del plan, **excepto el paso 8**: `opts.syncPlugins()` cae en
`preparePlugins`, que llama a `plugins.EnsureSource/HasSource/UpdateSource`,
y esos van directo a `kit/system` → ejecutan `noctalia` de verdad (red +
escriben en `~/.config/noctalia`).

Consecuencia práctica: un test que inyecte un `Runner` y olvide setear
`Options.SyncPlugins` **igual toca la máquina** y sale a la red. El
`fakeRunner` no lo detecta. El mismo paso corre aunque el plan esté vacío
(instalación idempotente de la source), así que el contacto con la máquina
ocurre en la corrida más inocua.

**Cómo se reproduce.** `Apply(&Plan{}, Options{Runner: fake})` (sin
`SyncPlugins`) → el `fakeRunner` no registra nada, pero `noctalia` se
ejecutó. Los tests del módulo lo evitan(seteando `SyncPlugins`), lo que
delata el hole.

**Fix propuesto.** Que el paso de plugins use el `Runner` (agregar los
comandos `noctalia ...` a la interfaz), o que `Options.SyncPlugins` sea
obligatorio en el camino real y el default sea un no-op explícito:

```go
func (o Options) syncPlugins() func() {
	if o.SyncPlugins != nil {
		return o.SyncPlugins
	}
	return func() { /* no-op: la inyección es explícita */ }
}
```

---

## MEDIO-4 — la costura no cubre el filesystem

**Archivo:** `internal/systeminstall/plan.go:160-163` (`pathExists`),
`internal/systeminstall/apply.go:195` (`os.RemoveAll`),
`apply.go:201` (`os.MkdirAll`)

**Qué pasa.** `pathExists` usa `os.Stat` sobre el disco real, y
`installDotfile` borra y crea directorios reales. El doc de `Build`
(`plan.go:53-54`) afirma que "opts.Runner decide contra qué máquina se
consulta el estado": para los dotfiles eso no es cierto — se consulta el
filesystem del proceso, no el del runner. Consecuencia: un `Build` con
`Runner` inyectado sigue mirando el disco real, y un `Apply` con
`fakeRunner` borra directorios reales (los tests lo hacen a propósito con
`t.TempDir()`, y por eso `TestApplyDotfileHaceBackup` tiene que afirmar que
"el destino viejo tenía que quedar borrado").

**Cómo se reproduce.** `Build` con `Runner: fake` y un dotfile cuyo origen
existe solo en un tempdir → `SrcExists` sale del disco real.

**Fix propuesto.** Agregar `Stat(path string) bool` (o `Exists`) al
`Runner`, o una interfaz aparte de filesystem, y usar `run.Stat` en vez de
`pathExists` en los dos lugares.

---

## MEDIO-5 — Build no verifica que las herramientas existan

**Archivo:** `internal/systeminstall/plan.go:72-77` (flatpak),
`apply.go:111-119`

**Qué pasa.** Si `flatpak` no está instalado, `flatpak list` falla, el set
vuelve vacío y **todas** las apps flatpak del perfil entran al plan. Lo
mismo con `pacman -Qq` (pero pacman siempre está) . Después, en `Apply`, el
paso de flatpak falla → la instalación queda a medias, con los paquetes ya
instalados, y el error es `flatpak falló: ...` sin contexto.

**Cómo se reproduce.** `fake.fail["flatpak list --app --columns=application"]`
→ el plan pide todas las apps (`TestBuildComandoQueFallaNoRompe` lo
confirma: "con los comandos caídos tenía que pedir todo"). En `Apply`, el
mismo fake hace fallar el install y el flujo muere en el paso 3.

**Fix propuesto.** En `Build`, si `run.CommandPath("flatpak") == ""`, no
agregar las apps al plan y dejar que `Apply` avise ("flatpak no está
instalado: se omiten N apps") en vez de fallar a mitad.

---

## MEDIO-6 — los argumentos que vienen del perfil no se validan

**Archivo:** `internal/systeminstall/apply.go:84` (paquetes),
`apply.go:105` (AUR), `apply.go:115` (flatpak), `apply.go:144` (grupos)

**Qué pasa.** No hay shell (`exec.Command` con vector de argumentos), así
que no hay shell injection. Pero sí hay **inyección de argumentos**:

- `pacman -S --needed <pkg...>`: un nombre que empiece con `-` lo toma
  pacman como opción. No hay `--` separando la lista.
- `usermod -aG <g1,g2> <user>`: los grupos van **unidos con coma**
  (`strings.Join(plan.Groups, ",")`), así que un grupo con coma se parte en
  dos y un grupo con `-` al principio se interpreta como opción.

La definición del perfil viene de la API de GitHub del repo de la distro
(`internal/systemdef/remote.go:14`) o de un `--file` local: es dato
semiconsolidado, no entrada del usuario, pero tampoco código de confianza.

**Cómo se reproduce.** Con un perfil `{"repo":["--help"],"groups":["a,b"]}`
el comando queda `sudo pacman -S --needed --help` y
`sudo usermod -aG a,b <user>`.

**Fix propuesto.** Validar los nombres antes de usarlos (regexp
`^[A-Za-z0-9][A-Za-z0-9._+@-]*$` para paquetes, `^[a-z0-9_-]+$` para
grupos), poner `--` antes de la lista de paquetes, y pasar los grupos de a
uno (`usermod -aG g1 -aG g2 <user>`).

---

## MEDIO-7 — `Apply(nil, opts)` hace panic

**Archivo:** `internal/systeminstall/apply.go:71-79`

**Qué pasa.** Ni `Apply` ni `dryRun` validan el plan: `plan.Packages` sobre
un `*Plan` nil es un nil dereference → panic. `Build` siempre devuelve un
plan no nil, pero la API es pública y el llamador puede traer uno armado a
mano.

**Cómo se reproduce.** `Apply(nil, Options{})` → panic.

**Fix propuesto.** Al principio de `Apply`:

```go
if plan == nil {
	return fmt.Errorf("plan nil")
}
```

---

## MEDIO-8 — un grupo inexistente aborta el paso entero

**Archivo:** `internal/systeminstall/apply.go:139-147`

**Qué pasa.** `usermod -aG g1,g2 <user>` es atómico: si uno de los grupos
no existe en la máquina, no agrega **ninguno** y `Apply` corta con
`no se pudieron agregar los grupos`. El error no dice qué grupo faltaba.
Además, los grupos se agregan **después** de habilitar los servicios, así
que el error aparece tarde, con el sistema ya a medio camino. Y el grupo
nuevo rige recién en el próximo login: `docker` no habilita el socket
hasta que la sesión se renueva.

**Cómo se reproduce.** Plan con `Groups: []string{"docker", "grupo-que-no-existe"}`
→ el paso de grupos falla entero.

**Fix propuesto.** Validar los grupos contra `getent group` antes de
`usermod`, agregar los que existen y avisar de los que no, y mover el paso
de grupos **antes** de los servicios (el grupo habilita al usuario a usar
el servicio, no al revés).

---

## MEDIO-9 — `Apply` no pide confirmación antes de los pasos con sudo

**Archivo:** `internal/systeminstall/apply.go:79-147`

**Qué pasa.** No hay ninguna confirmación propia: `Options.Yes` solo
agrega `--noconfirm` a pacman y al ayudante AUR. El único control es el
prompt de `sudo`, y `sudo` cachea la credencial por su
`timestamp_timeout` (verificar el valor por defecto: 5 minutos en la
mayoría de las distros), así que los pasos 5 y 6 (`systemctl enable`,
`usermod`) pueden correr sin volver a preguntar nada.

**Cómo se reproduce.** Con `sudo` cacheado, `Apply` pasa los tres pasos con
sudo sin ninguna pregunta.

**Fix propuesto.** Una confirmación explícita antes del primer paso con
sudo, inyectable como el resto:

```go
// Options
Confirm func(prompt string) bool

// en Apply, antes del primer uso de sudo
if !opts.Yes && opts.Confirm != nil && !opts.Confirm("aplicar el plan de sistema") {
	return fmt.Errorf("cancelado por el usuario")
}
```

---

## BAJO-1 — `orReal` no detecta un Runner puntero nil

**Archivo:** `internal/systeminstall/runner.go:44-49`

**Qué pasa.** `orReal` compara la interfaz con `nil`. Un Runner guardado
como puntero nil tipado (`opts.Runner = (*fakeRunner)(nil)`) no es `nil`
como interfaz: `orReal` lo devuelve tal cual y el primer método que se
llame hace nil dereference → panic.

**Cómo se reproduce.** `var f *fakeRunner; Options{Runner: f}` → panic en
la primera llamada al runner.

**Fix propuesto.** Es la limitación clásica de las interfaces en Go:
documentarlo ("el Runner no puede ser un puntero nil") y/o pedir un
`Runner` no nil en el borde (`if opts.Runner == nil { ... }` antes de
usarlo).

---

## BAJO-2 — el paso de propios usa `os.Executable()`, que no es inyectable

**Archivo:** `internal/systeminstall/apply.go:121-131`

**Qué pasa.** Para instalar los binarios propios el módulo se invoca a sí
mismo: `run.Interactive(self, "install", nombre)`, con `self` =
`os.Executable()`. En producción es el b3l0w en ejecución (puede ser un
build de desarrollo o un binario en `/tmp`), y en los tests es el binario
de test, por eso `TestApplyOrdenDeEjecucion` compara contra
`os.Executable()` y no puede fijar el path.

**Cómo se reproduce.** `Apply` con `Own: []string{"b3l0w"}` → el runner
recibe `<path del binario actual> install b3l0w`.

**Fix propuesto.** `Options.SelfPath string` (vacío → `os.Executable()`),
para que el test fije el path y para poder instalar desde una ruta
concreta.

---

## BAJO-3 — `dryRun` imprime con `fmt.Printf` y no lo usa nadie

**Archivo:** `internal/systeminstall/apply.go:22-24`, `apply.go:72-75`,
`apply.go:164-185`

**Qué pasa.** Tres cosas:

1. `dryRun` imprime con `fmt.Printf` mientras el resto del módulo usa
   `kit/output` (`output.Ok`, `output.Warn`): el formato queda fuera del
   estilo del CLI.
2. El dry-run **no menciona el paso de plugins**, que sí corre en un apply
   real: la simulación miente sobre lo que pasa.
3. `Options.DryRun` **no lo usa ningún llamador del CLI**: la simulación de
   `b3l0w install system` se hace en `cmd/b3l0w/system.go:146-149` (imprime
   el plan y no llama a `Apply`). El código solo lo ejecutan los tests.

**Cómo se reproduce.** Grep de `DryRun`: los únicos usos de `systeminstall`
están en el propio módulo y sus tests.

**Fix propuesto.** Usar `kit/output`, mencionar el paso de plugins en la
salida, y decidir: o el CLI pasa `DryRun: true` cuando no hay `--apply`, o
se borra la opción.

---

## BAJO-4 — `systemctl is-enabled` no distingue "no habilitado" de "no habilitable"

**Archivo:** `internal/systeminstall/plan.go:142-146`

**Qué pasa.** `is-enabled` devuelve 0 también para `static`, `indirect` y
`generated`. Una unit estática **nunca** se puede habilitar (`systemctl
enable` falla con "unit has no installation config"), así que el plan la
marca como cumplida y el requisito queda incumplido para siempre, sin
aviso. El caso inverso: una unit `masked` devuelve != 0 → entra al plan →
`systemctl enable` falla → `Apply` aborta con
`no se pudieron habilitar los servicios`.

**Cómo se reproduce.** Con el `fakeRunner`, el resultado depende solo de si
hay error: `fail["systemctl is-enabled sddm"]` lo marca como pendiente.
Una unit estática real nunca llega al plan.

**Fix propuesto.** Parsear la salida de `is-enabled` y tratar `static`,
`indirect`, `generated` y `masked` como casos aparte (avisar, no fallar).

---

## BAJO-5 — los backups de dotfiles se pisan y nunca se limpian

**Archivo:** `internal/systeminstall/apply.go:190-192`

**Qué pasa.** El timestamp tiene granularidad de segundos
(`20060102-150405`): dos corridas en el mismo segundo escriben el mismo
nombre de backup y la segunda pisa la primera. Y no hay poda: los
`.bak-<ts>` se acumulan en el home para siempre (con ALTO-1, uno por
dotfile y por corrida).

**Cómo se reproducir.** Dos `Apply` del mismo dotfile dentro del mismo
segundo → el mismo `backup`.

**Fix propuesto.** Timestamp con nanosegundos o un sufijo incremental, y una
rutina de poda que conserve las últimas N versiones.

---

## BAJO-6 — Apply no devuelve qué se aplicó

**Archivo:** `internal/systeminstall/apply.go:71-162`

**Qué pasa.** Cuando un paso falla, el error dice qué falló pero no qué se
había hecho antes. El usuario tiene que leer la salida de los comandos
para saber en qué estado quedó la máquina. Con un plan de cientos de
paquetes, "pacman falló" no dice si los flatpak ya se instalaron.

**Cómo se reproduce.** Fallar el paso de grupos en un plan completo: el
error es `no se pudieron agregar los grupos: ...` y no hay forma de saber
programáticamente que los paquetes, AUR, flatpak, propios y servicios ya
estaban instalados.

**Fix propuesto.** Devolver un resultado con los pasos hechos
(`Apply(plan, opts) (Result, error)`, con `Result.Pasos []string`), o al
menos loguear cada paso con `output.Step` antes de ejecutarlo.

---

## Lo que NO encontré

- **Shell injection**: no hay. Todo se ejecuta con vector de argumentos
  (`kit/system/system.go:16-47` usa `exec.Command(nombre, args...)`, nunca
  `sh -c`), y en el módulo no hay concatenación de strings para armar
  comandos. Hay inyección de *argumentos* (MEDIO-6), que es otra cosa.
- **Rutas que se salten el `Runner`**: dentro de `plan.go` y `apply.go` no
  queda ninguna llamada directa a `kit/system`; el diff contra `main`
  reemplaza las seis. Lo que queda fuera del `Runner` es el filesystem
  (MEDIO-4), el paso de plugins (MEDIO-3), `os.Executable()` (BAJO-2) y
  `user.Current()`.
- **Errores tragados**: solo dos, y los dos están reportados
  (`MEDIO-2` por el remote-add de flatpak, `MEDIO-4` por el `cp` del
  backup: si el backup falla, el destino no se borra — eso está bien).
- **Falta de tests**: la cobertura de `Apply` es buena (orden, fallo por
  paso, dry-run, `--noconfirm`, dotfiles, plugins). Lo que no está
  cubierto y debería: `Apply` con plan nil (MEDIO-7), `Build`/`Apply` con
  `Runner` puntero nil (BAJO-1), `Build` cuando falta `flatpak`
  (MEDIO-5) y `Apply` con un destino de dotfile peligroso (ALTO-2).
