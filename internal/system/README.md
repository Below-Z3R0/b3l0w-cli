# system

Wrapper del actualizador del sistema base (`cachy-update` / `arch-update`).

## Qué hace y por qué existe

En CachyOS el actualizador es `cachy-update`, que es un symlink a
`arch-update` (el upstream de Arch). Es **el mismo programa** y pertenece a
un paquete del sistema: no se renombra, no se reescribe y no se reimplementa.

Este módulo existe para que el resto del CLI no tenga que saber cómo se
llama el actualizador en cada distro. Encapsula tres cosas:

1. **El nombre del binario.** La resolución es por nombre, con fallback:
   primero `cachy-update`, después `arch-update`. Si no hay ninguno, el
   módulo lo dice y el llamador decide qué hacer.
2. **El modo de ejecución.** Listar pendientes es no interactivo (`-l` con
   timeout). Actualizar es interactivo y necesita heredar la terminal.
3. **El formato de salida.** `Pending` convierte la salida cruda de `-l` en
   dos números: cuántos paquetes del repo y cuántos de AUR.

## API

| Función | Qué hace |
|---|---|
| `Binary() string` | Ruta del actualizador, o `""` si no hay ninguno. Prioriza `cachy-update`, cae a `arch-update`. |
| `Available() bool` | `Binary() != ""`. Atajo para "¿tenemos actualizador?". |
| `List() (string, error)` | Salida cruda de `-l` (la opción no interactiva). Timeout de 5 minutos. |
| `Pending(list string) (repo, aur int)` | Cuenta los paquetes pendientes separando repo y AUR. |
| `UpdateInteractive() error` | Corre el actualizador heredando stdin/stdout/stderr. |

### Uso

```go
if !system.Available() {
    output.Warn("no hay cachy-update ni arch-update: se omite el sistema")
    return 0
}

list, err := system.List()
if err != nil && list == "" {
    return 1
}
repo, aur := system.Pending(list)
// repo: paquetes del repo, aur: paquetes de AUR
```

`UpdateInteractive` no se automatiza: el programa muestra las news, pide
confirmación y después ofrece huérfanos, `pacnew`, kernel pendiente y
servicios a reiniciar. No hay bandera `--yes` y fingir que sí sería mentir.

## Decisiones

**Wrapper, no reimplementación.** El CLI no decide qué paquetes actualizar ni cómo
se aplican: llama al actualizador del sistema. Duplicar esa lógica
significa mantener dos Politics más dos hooks y terminar con un sistema
distinto al de la distro.

**Fallback por nombre, no por paquete.** Se busca `cachy-update` y después
`arch-update` en el `PATH`. No se consulta la base de datos de pacman para
saber qué paquete trae el binario: el `PATH` ya dice la verdad de qué
actualizador se puede usar.

**`List` con timeout de 5 minutos.** `List` lanza un comando externo que
puede quedarse esperando. Sin deadline, el comando hereda esa espera y
`b3l0w status` queda colgado; el timeout de `kit/system.RunTimeout` corta
la espera.

**`Pending` cuenta líneas, no parsea nombres.** El formato de `-l` es una
sección por encabezado (`==> Packages:`, `==> AUR packages:`) y una línea
por paquete. El encabezado define a qué sección pertenece cada línea, así
que alcanza con contar líneas por sección: no hace falta extraer ni validar
el nombre del paquete. Una línea que empieza con `==>` cambia la sección
vigente (y no se cuenta); las demás se cuentan según esa sección: si el
encabezado contiene `aur` van a AUR, si contiene `package` van al repo.

**`UpdateInteractive` hereda la terminal.** `kit/system.Interactive` conecta
stdin, stdout y stderr del hijo con los del proceso padre, que es lo que
necesita un programa que hace preguntas.

## Archivos

| Archivo | Qué hace |
|---|---|
| `archupdate.go` | Todo el módulo: resolución del binario, listado, conteo de pendientes y actualización interactiva. |
| `archupdate_test.go` | 3 tests de `Pending`: separa repo y AUR, salida sin actualizaciones, y que los encabezados no se cuenten como paquetes. |

Cobertura: `Pending` está cubierto con fixtures del formato real de
`cachy-update -l`. `Binary`, `Available`, `List` y `UpdateInteractive`
dependen del sistema y no tienen tests propios: se ejercitan a través de
`internal/update` y `internal/doctor`.

## Qué NO hace

- **No decide si hay que actualizar.** `List` solo consulta y
  `UpdateInteractive` corre lo que el llamador le pide.
- **No automatiza la actualización.** No hay `--yes`, ni respuesta
  automática a las preguntas del actualizador.
- **No reimplementa pacman.** No calcula dependencias, no resuelve orphans,
  no decide qué se actualiza.
- **No distingue más de dos categorías.** `Pending` separa repo y AUR; no
  distingue oficial de no-oficial, ni kernels, ni huérfanos.
- **No chequea la edad de la base de datos.** Eso es territorio de pacman
  y de la categoría `apps`, no de este wrapper.
