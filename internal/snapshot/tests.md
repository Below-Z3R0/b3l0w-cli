# TESTS — internal/snapshot

Qué prueba cada test del módulo. Es el módulo con mejor relación
test/código del repo: 171 líneas de test para 31 de producción, 100 % de
cobertura.

**Regla dura del módulo:** ningún test ejecuta `snapper` real. `fakeSnapper`
instala un binario falso primero en el PATH que loguea sus argumentos en
`$FAKE_LOG`, escribe `$FAKE_STDERR` y sale con `$FAKE_EXIT`.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 6 |
| Cobertura | 100.0 % |
| Verdes | 6 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `snapshot_test.go` | los 6 caminos de `Create` con un `snapper` falso: sin binario, con binario, con fallo, y los tres casos de label |

### Detalle

| Test | Qué fija |
|---|---|
| `TestCreateSinSnapperInstalado` | con un PATH vacío devuelve el error "snapper no está instalado" y **no** intenta ejecutar nada |
| `TestCreateConSnapperInstalado` | con el falso, corre y los argumentos son `-c home create -d <label>` |
| `TestCreateConSnapperQueFalla` | con `FAKE_EXIT=1` y un stderr, el error lo incluye |
| `TestCreateConLabelVacio` | label `""` → error, sin tocar el binario |
| `TestCreateConLabelSoloEspacios` | label `"   "` → error (usa `TrimSpace`, no `== ""`) |
| `TestCreateConLabelConEspacios` | un label con espacios internos se pasa como **un** argumento, no se parte |

## Sin cubrir

Nada del código de producción: la cobertura es 100 %.

Lo que no se prueba es el comportamiento de `snapper` real, que está fuera del
módulo. En particular, el hallazgo A1 de `bugs.md` (la config `home` puede no
existir) no tiene test porque el falso siempre responde lo que el test le dice.
