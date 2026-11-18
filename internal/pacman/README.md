# internal/pacman

La configuración del repo `[b3l0w]` en `pacman.conf`.

## Qué hace

| Función | Para qué |
|---|---|
| `Configurado(path)` | dice si la sección `[b3l0w]` ya está en el archivo |
| `AgregarRepo(path, section)` | la agrega al final con `sudo tee -a` |
| `Section` | el bloque que se escribe |

## Por qué vive acá y no en `cmd/`

Escribir en `/etc` es acceso al sistema, no presentación. El comando
`b3l0w setup repo` parsea el flag, confirma con el usuario y muestra el
resultado; el archivo lo toca este paquete.

La regla del perfil es que `cmd/` no tiene `exec.Command` ni `os.RemoveAll`.
Este paquete es lo que hace que esa regla se pueda cumplir.

## Detalles

- **`Configurado` devuelve `false` si el archivo no se puede leer.** El
  llamador decide si eso es un error o una razón para intentar el append.
- **`AgregarRepo` usa `sudo` y no `os.OpenFile`.** La redirección la haría
  el shell como usuario sin privilegios; `tee` como root es el camino que
  funciona sin pedir una shell interactiva.
- **`tee` escribe de una sola vez.** Un corte a mitad no deja el archivo
  parcialmente modificado.

## Tests

`repo_test.go` cubre el archivo sin la sección, con la sección y el archivo
inexistente. `AgregarRepo` no se testea: requiere sudo y un `pacman.conf`
real, así que su verificación es la corrida manual de `b3l0w setup repo`.
