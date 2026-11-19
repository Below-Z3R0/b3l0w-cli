# TESTS — internal/systemdef

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 18 |
| Cobertura | 74.1 % |
| Verdes | 18 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `systemdef_test.go` | `parse`, `Builtin`, `Names` y `Merge` sobre el `system.json` real y definiciones sintéticas |
| `remote_test.go` | `LoadRemote`: cache fresco, red, cache viejo y embebido, contra `httptest` |
| `remote_bytes_test.go` | `RemoteBytes`: la descarga sin cache |

### Detalle

`systemdef_test.go` cubre el parseo del archivo embebido (que es el contrato
real), el rechazo de una definición sin versión o sin perfiles, y el merge de
perfiles con deduplicación por categoría.

`remote_test.go` es el que ejercita la cascada de `LoadRemote` con un
`httptest.Server`: verifica que el cache fresco gane, que la red se use cuando no
hay cache, que el cache viejo gane si la red falla, y que el embebido sea el
último recurso.

`remote_bytes_test.go` cubre la descarga directa, que `systemdef` usa para
comparar la definición remota con la local.

## Sin cubrir

| Qué | Por qué |
|---|---|
| `writeCache` cuando falla la escritura | Necesita un filesystem sin permiso de escritura. Es el hallazgo B1 de `bugs.md`. |
| `Names` con más de un perfil | El test pasa con uno solo, así que el desorden (M1) no se ve. |
| `Merge` con un nombre inexistente | No hay test para esa rama. |
