# TESTS — internal/pacman

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 2 |
| Cobertura | 36.4 % |
| Verdes | 2 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `repo_test.go` | `Configurado` con un archivo sin la sección, con la sección, y con un path que no existe |

### Detalle

`TestConfigurado` escribe un `pacman.conf` falso en `t.TempDir()` con dos casos:
primero `[options]\nColor` (sin el repo), después el mismo contenido más
`Section`. Verifica que antes sea `false` y después `true`.

`TestConfiguradoArchivoInexistente` pasa un path en un `t.TempDir()` vacío y
espera `false`. Cubre el contrato "un archivo que no se puede leer cuenta como
no configurado".

## Sin cubrir

| Qué | Por qué |
|---|---|
| `AgregarRepo` | Requiere `sudo` y un `pacman.conf` real. Su verificación es la corrida manual de `b3l0w setup repo`. |
| La sección comentada (`# [b3l0w]`) | Es el hallazgo M1 de `bugs.md`: el test que lo cubriría falla hoy, y arreglarlo no estaba en el alcance de esta documentación. |
| El archivo sin salto de línea final | B1 de `bugs.md`. Reproducirlo exige escribir un archivo sin `\n` final, que es posible, pero el caso no se da en un `pacman.conf` real. |
