# TESTS — internal/systeminstall

Qué prueba cada test del módulo y qué quedó sin cubrir.

**Regla dura del módulo:** ningún test toca el sistema real. `fake_test.go`
define un `Runner` falso que registra cada comando en vez de ejecutarlo; los
tests de plan usan el `Runner` para simular qué está instalado.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 26 |
| Cobertura | 90.8 % |
| Verdes | 26 |
| Rojos | 0 |

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `apply_test.go` | `Apply`: orden de ejecución, corte ante el primer fallo, dry-run, `--yes`, back up de dotfiles y mensaje de cada paso |
| `plan_test.go` | `Build` y `Plan.Empty`: perfil vacío, completo, omitir lo instalado, servicios, dotfiles y comando que falla |
| `seguridad_test.go` | `destinoSeguro` y que `Apply` no pise un destino peligroso |
| `fake_test.go` | el `Runner` y el `Env` falsos; no tiene tests propios |

### Detalle

`apply_test.go` es el que más cubre:
- `TestApplyOrdenDeEjecucion`: los pasos corren en el orden del plan.
- `TestApplyUnPasoQueFallaCortaElFlujo`: un fallo no sigue con los siguientes.
- `TestApplyFallaEnServiciosNoTocaGruposNiDotfiles`: el corte deja lo de atrás
  intacto.
- `TestApplyDryRunNoEjecutaNada`: `--dry-run` no ejecuta ningún comando.
- `TestApplyYesPasaNoconfirm`: `--yes` agrega `--noconfirm` al comando.
- `TestApplyDotfileHaceBackup` y `TestApplyDotfileSinDestinoNoHaceBackup`: el
  backup solo se hace si había algo que pisar.
- `TestApplyCadaPasoFallaConSuMensaje` y `TestApplyErroresDeDotfile`: cada
  camino de error dice qué pasó.

`plan_test.go` cubre `Build` con el `Runner` falso: qué paquetes faltan, qué
servicios están habilitados, qué archivos de dotfiles existen, y que un comando
del `Runner` que falla no rompa el plan.

`seguridad_test.go` es el archivo de los bugs de seguridad ya arreglados:
`destinoSeguro` rechaza `/`, el home y las rutas que se escapan; y `Apply` no
pisa un archivo que ya está en su lugar.

## Sin cubrir

| Qué | Por qué |
|---|---|
| `Apply` con un perfil real completo | Los tests usan perfiles sintéticos y un `Runner` falso. Un apply real cambiaría la máquina. |
| El `Runner` real (`systemRunner`) | Solo `TestSystemRunnerNoEsNil` verifica que no sea nil. Ejecutarlo de verdad es `sudo pacman -S`. |
| Los caminos de error de `copyFile` | Necesitan un filesystem sin permiso. |
