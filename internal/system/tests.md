# TESTS — internal/system

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 3 |
| Cobertura | 46.4 % |
| Verdes | 3 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `archupdate_test.go` | `Pending`: separación repo/AUR, lista vacía y que los encabezados no se cuenten como paquetes |

### Detalle

`TestPendingSeparaRepoYAUR` pasa una salida con las dos secciones y verifica
que el conteo dé `repo=2, aur=1`.

`TestPendingSinUpdates` verifica que una lista vacía devuelva `0, 0`.

`TestPendingIgnoraEncabezados` verifica que las líneas `==>` no se sumen.

Los tres son de función pura: no tocan el sistema, no necesitan el binario.

## Sin cubrir

| Qué | Por qué |
|---|---|
| `Binary`, `Available` | Dependen del PATH real: el test tendría que fabricar un `cachy-update` falso. Es lo que hace `snapshot` con `snapper`; acá no se hizo. |
| `List` | Igual: necesita el binario. Además tiene un timeout de 5 minutos, así que un test que lo ejercite sin stub esperaría. |
| `UpdateInteractive` | Hereda la terminal (`stdin`/`stdout`): no es testeable sin un PTY y no se quiso simular una confirmación que el usuario no dio. |
| El fallback entre los dos nombres | No hay test que ponga solo `arch-update` en el PATH y verifique que `Binary()` lo elija. |
