# systeminstall

Compara la definición del sistema contra la máquina y aplica lo que falta.

Es el módulo más peligroso del CLI: instala con root, habilita servicios y
escribe en el home. Por eso todo lo que le pide al sistema pasa por una
interfaz inyectable (`Runner`) y se testea sin pacman, flatpak ni systemctl.

| Archivo | Qué hace |
|---|---|
| `plan.go` | `Build`: arma el `Plan` con lo que falta contra la máquina. Solo lee. |
| `apply.go` | `Apply`: ejecuta el plan en orden. Escribe. |
| `runner.go` | La interfaz `Runner` y su implementación real (`kit/system`). |

## Qué es un Plan

`Build(profile, dotfiles, opts)` devuelve un `Plan`: siete listas, una por
categoría de la definición (`internal/systemdef`):

| Campo | Qué guarda | Cómo se decide que falta |
|---|---|---|
| `Packages` | paquetes de pacman | no están en `pacman -Qq` |
| `AUR` | paquetes de AUR | no están en `pacman -Qq` (los foráneos también salen ahí) |
| `Flatpak` | apps de flatpak | no salen en `flatpak list --app` |
| `Own` | binarios propios | `CommandPath` no los encuentra en el `PATH` |
| `Services` | units de systemd | `systemctl is-enabled` no devuelve 0 |
| `Groups` | grupos del usuario | no salen en `id -nG` |
| `Files` | dotfiles | `FileStatus{Src, Dest, SrcExists, DestExists}` |

`Plan.Empty()` dice si no hay nada pendiente. Ojo: `Files` lista **todos** los
archivos del perfil, no solo los que faltan; `Empty()` mira que no haya
ninguno con origen presente y destino ausente (ver BUGS.md, ALTO-1).

`Build` es de lectura: `Options.Yes` y `Options.DryRun` no aplican ahí.

## El orden de Apply

`Apply(plan, opts)` ejecuta en este orden, y el orden es el contrato
(`TestApplyOrdenDeEjecucion` lo fija):

1. **Paquetes** — `sudo pacman -S --needed ...`
2. **AUR** — `paru` (o `yay`) `-S --needed ...`
3. **Flatpak** — `flatpak remote-add --if-not-exists flathub ...` y
   `flatpak install -y flathub ...`
4. **Propios** — el binario actual con `install <nombre>`
5. **Servicios** — `sudo systemctl enable ...`
6. **Grupos** — `sudo usermod -aG g1,g2 <usuario>`
7. **Dotfiles** — `cp -a` con backup del destino
8. **Plugins** — source de Noctalia (`noctalia msg plugins ...`)

Por qué ese orden:

- **Paquetes primero** porque todo lo demás necesita binarios que no están:
  `paru`, `flatpak` y las units vienen de los paquetes. Sin este paso, el
  paso 2 no tiene con qué correr y el 5 no tiene qué habilitar.
- **AUR después de los paquetes** porque el ayudante (`paru`) se instala
  desde el repo: el perfil `desktop` lo mete en `repo`.
- **Flatpak después** por lo mismo: el binario `flatpak` es del repo.
- **Propios después** porque `b3l0w install <propio>` usa el CLI ya instalado.
- **Servicios después de los paquetes** porque la unit la trae el paquete:
  habilitar `docker.socket` sin `docker` instalado falla.
- **Grupos después de los servicios** (verificar): el código justifica el
  orden con "lo que habilita a lo que viene después se instala primero",
  pero habilitar una unit no depende del grupo. Lo cierto es que el grupo
  **no** habilita al servicio: rige recién en el próximo login.
- **Dotfiles al final** porque pisan la configuración que trae el sistema:
  si se copiaran antes, un paquete podría sobrescribirlos.
- **Plugins al final** porque la source de Noctalia escribe dentro de
  `~/.config/noctalia`, que es un dotfile del perfil: los dotfiles se copian
  primero y el source se agrega después, así no se pierde.

## Si un paso falla

**Corta en el primero.** `Apply` devuelve el error del paso que falló y no
ejecuta nada más. No hay rollback: lo aplicado queda aplicado (el error
dice qué falló, no qué se había hecho) ni resumen de lo hecho (BUGS.md,
BAJO-6).

El error de cada paso nombra la categoría: `pacman falló: ...`,
`paru falló: ...`, `flatpak falló: ...`, `no se pudo instalar <x>`,
`no se pudieron habilitar los servicios`, `no se pudieron agregar los
grupos`, `backup de <dest>` / `copiar <src>`.

El paso de plugins es la excepción: sus fallos son **avisos**, no errores.
Los plugins son un extra, no el sistema: un source que no se puede agregar
no tira la instalación.

## Qué NO hace

- **No particiona ni toca el bootloader.** No hay `sgdisk`, `mkfs` ni
  `grub-install` en el módulo: `limine` y los hooks de kernel se instalan
  como paquetes, nada más.
- **No arranca los servicios ni recarga systemd.** Hace `systemctl enable`,
  no `enable --now`: un servicio habilitado queda para el próximo arranque.
- **No actualiza la base de datos de pacman.** No hace `-Sy` ni `-Syu`.
- **No borra ni desinstala.** El plan solo contiene lo que falta; para
  quitar cosas están los comandos de cada categoría.
- **No reinicia ni recarga systemd.** Los grupos nuevos y los servicios
  habilitados rigen en el próximo login/arranque.
- **No hace rollback** si un paso falla a mitad, ni valida el destino de
  los dotfiles (ver BUGS.md, ALTO-2).

## La costura Runner

`Runner` (en `runner.go`) tiene tres métodos:

```go
type Runner interface {
	Run(name string, args ...string) (string, error) // consulta, devuelve stdout
	Interactive(name string, args ...string) error    // con terminal: sudo pregunta
	CommandPath(name string) string                  // dónde está un binario
}
```

Por qué existe: este módulo muta el sistema con root. Sin una costura
inyectable, probar el orden de `Apply` implicaría instalar paquetes de
verdad. Con `Runner`, los tests usan un `fakeRunner` que registra lo que se
le pidió. `Options.Runner` nil cae en el runner real (`orReal`).

Qué cubre: **todas las consultas** (`pacman -Qq`, `flatpak list`,
`systemctl is-enabled`, `id -nG`, `CommandPath`) y **todos los comandos que
se ejecutan** (`pacman`, `paru`, `flatpak`, `systemctl`, `usermod`, `cp`).

Qué **no** cubre la costura (importante para los tests):

- El **filesystem**: `pathExists` usa `os.Stat` y `installDotfile` borra y
  crea directorios con `os.RemoveAll`/`os.MkdirAll` sobre el disco real.
- El **paso de plugins**: va por `Options.SyncPlugins` (una función), no por
  el `Runner`. Si el llamador no la setea, `Apply` corre `noctalia` de
  verdad.
- `os.Executable()` (el binario actual, paso 4) y `user.Current()` (el
  usuario, paso 6): no son inyectables.

Por eso un test que use el `Runner` tiene que setear **también**
`Options.SyncPlugins` si no quiere que se toque la máquina.

## Seguridad

- **No hay shell.** Todo se ejecuta con `exec.Command(nombre, args...)`
  (vector de argumentos), nunca `sh -c`. No hay concatenación de strings
  para armar comandos, así que no hay shell injection.
- **Sudo sin filtro**: los pasos con sudo son `pacman`, `systemctl enable` y
  `usermod`. `Apply` no pide confirmación propia: el único control es el
  prompt de sudo, que queda cacheado por su `timestamp_timeout` (verificar
  el valor por defecto de la máquina), así que los pasos 5 y 6 pueden pasar
  sin volver a preguntar.
- **`Options.Yes`** agrega `--noconfirm` a pacman y al ayudante AUR. El
  flatpak siempre va con `-y` (ver BUGS.md, MEDIO-1).
- Los nombres de paquetes, grupos y apps vienen de la definición (remota o
  de `--file`) y no se validan (ver BUGS.md, MEDIO-6).

## Cómo se usa

```go
plan := systeminstall.Build(profile, dotfiles, systeminstall.Options{})
err := systeminstall.Apply(plan, systeminstall.Options{Yes: yes})
```

Desde el CLI: `b3l0w install system [--profile=desktop] [--apply] [--yes]`.
Sin `--apply` el CLI imprime el plan y no llama a `Apply`.
