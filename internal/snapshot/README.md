# Snapshot de home

## Qué es

Pide una snapshot del subvolume `home` antes de que el CLI toque la
configuración del usuario: si el pull de dotfiles sale mal, hay una snapshot a
la que volver. No copia archivos ni restaura: solo llama a `snapper`.

## API

```go
func Create(label string) error
```

- `label`: descripción de la snapshot, lo que permite saber después cuál era
  (`b3l0w pre-dotfiles`). Vacío o solo espacios es un error: se rechaza
  **antes** de tocar el disco, porque una snapshot sin descripción no se puede
  identificar después.
- `nil` si la snapshot se creó. Error si el label está vacío, si snapper no está
  instalado, o si el comando falla; en el último caso el error trae la salida.

Único llamador: `internal/update/update.go`, antes del pull de dotfiles:

```go
if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
    output.Warn("no home snapshot: " + err.Error())
}
```

La firma `Create(label string) error` no cambió.

## Por qué el error NO es bloqueante

El módulo devuelve el error y **decide el llamador**. Acá decide
`internal/update`, y decide avisar y seguir, por tres razones:

1. La snapshot es una protección extra, no la única: el pull de dotfiles tiene
   su propio 3-way merge, no pisa cambios locales igual.
2. Sin snapper, o sin config `home`, bloquear sería castigar al usuario por una
   protección que no pidió.
3. El error dice qué pasó (`snapper -c home: <salida>`): se ve si falta el
   paquete, la config, o el permiso.

## Qué pasa cuando snapper no está

| Situación | Qué devuelve |
|---|---|
| snapper no está instalado | `snapper no está instalado` |
| snapper está pero no tiene config `home` | `snapper -c home: <salida de snapper>` |

En el segundo caso la salida de snapper dice qué falta (por ejemplo
`Unknown config`).

## La diferencia con `internal/snapshots`

| | `internal/snapshot` (singular) | `internal/snapshots` (plural) |
|---|---|---|
| Qué hace | **Crea** una snapshot de `home` | **Cuenta y poda** entradas de arranque de Limine |
| Herramienta | `snapper` | `limine-snapper-info` / `-list` / `-remove` / `-sync` |
| Toca el disco | Sí: crea la snapshot | No: solo borra entradas de la ESP |
| Rol | Red de seguridad antes de tocar config | Higiene del menú de arranque |

Ninguno depende del otro: el primero crea snapshots de snapper, el segundo
administra las entradas de Limine que apuntan a ellas.

## Estado real de la config `home` en CachyOS

En esta máquina **hoy `Create` siempre falla**, y no es un bug del módulo:

- snapper **sí está instalado**: `/usr/sbin/snapper`.
- El subvolume `home` **sí existe**: `/etc/fstab` lo monta como
  `subvol=/@home` sobre el mismo btrfs que `@`.
- Pero `/etc/snapper/configs/` contiene **solo `root`**: no hay config `home`,
  así que `snapper -c home` responde `Unknown config` y el módulo devuelve ese
  error.

Falta crear la config: es trabajo del sistema, no del módulo. Lo natural es que
`internal/systemdef` declare `snapper-config create home` en el perfil,
(verificar) porque el módulo no debería decidir políticas de config.

Sobre permisos: `snapper` habla con `snapperd` por DBus (`BusName=org.opensuse.Snapper`)
y la política deja que el contexto por defecto envíe mensajes, pero sin ser
root hace falta `ALLOW_USERS` o `ALLOW_GROUPS` en el config, (verificar) cuál
aplicaría en la distro.

## Decisiones

- **Sin runner inyectado.** `Create` llama a `system.CommandPath` y
  `system.RunCombined`, que resuelven el binario por `PATH`. Alcanza con un
  `snapper` falso adelante en el `PATH`: **no hubo que tocar la firma ni la
  implementación** para testearlo y la API queda intacta. Es el patrón que usa
  `internal/snapshots` con sus tools.
- **El label vacío se rechaza antes de tocar el disco.** Una snapshot sin
  descripción no se puede identificar después, así que llamarla sería dejar
  basura en el timeline. Se valida el argumento primero
  (`strings.TrimSpace(label) == ""`). El label viaja como **un** argumento (no
  hay shell que lo parta), así que admite espacios.
- **El error de snapper trae la salida.** `fmt.Errorf("snapper -c home: %s",
  out)` en vez de solo el error: la salida dice *qué* falló (config inexistente,
  permiso, disco lleno).

## Archivos

| Archivo | Qué hace |
|---|---|
| `snapshot.go` | `Create`: valida el label, chequea snapper, pide la snapshot. |
| `snapshot_test.go` | 6 tests contra un `snapper` falso en el `PATH`. |
