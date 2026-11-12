# snapshot

Pide una snapshot de `home` antes de que el CLI toque la configuración.

## Qué hace y por qué existe

Antes de aplicar dotfiles sobre el home del usuario, el CLI crea una
snapshot del subvolume `home` con `snapper`. Si algo sale mal, hay un punto
de retorno del filesystem, no solo el 3-way merge de archivos.

Este módulo existe para eso y nada más: **un solo comando, una sola
responsabilidad**. No hace copias de archivos, no implementa backups ni
mantiene un historial propio. Si `snapper` no está instalado, o no tiene
configuración para `home`, el módulo devuelve un error y **el llamador
decide**: en `internal/update` es un warning, no un bloqueo, porque el
3-way merge de dotfiles sigue protegiendo los cambios locales.

## API

| Función | Qué hace |
|---|---|
| `Create(label string) error` | Intenta crear una snapshot del subvolume `home` con la etiqueta dada. |

### Uso

```go
if err := snapshot.Create("b3l0w pre-dotfiles"); err != nil {
    output.Warn("no home snapshot: " + err.Error())
}
```

El error nunca detiene la operación: la snapshot es una red de seguridad,
no un requisito.

## Qué corre por debajo

```
snapper -c home create -d <label>
```

- `system.CommandPath("snapper")` verifica que el binario esté en el
  `PATH`. Si no está, el error es `"snapper no está instalado"`.
- `system.RunCombined` ejecuta el comando con stderr mezclado en stdout.
  El error no distingue el código de salida del mensaje: lo que ve el
  usuario es el output combinado.
- El subvolume se fija en `home`: es el que contiene la configuración que el
  CLI va a sobreescribir.

## Diferencia con `internal/snapshots`

Son dos módulos distintos y no se pisan:

| | `internal/snapshot` (este) | `internal/snapshots` |
|---|---|---|
| Objeto | Snapshots de **snapper** (filesystem) | **Entradas de arranque** de Limine |
| Herramienta | `snapper -c home create` | `limine-snapper-info`, `limine-snapper-remove` |
| API | `Create(label)` | `Entries()`, `IDs()`, `Prune(keep)`, `Report(keep, yes)` |
| Para qué | Punto de retorno antes de tocar el home | Contar y podar las entradas de Limine que apuntan a snapshots |
| Momento | Antes de aplicar dotfiles | Routine de mantenimiento |

`snapshots` no crea snapshots: cuenta y borra **entradas de arranque**, que
en Limine son punteros a snapshots de snapper. Son cosas distintas con
nombres parecidos; de ahí las dos carpetas.

## Decisiones

**Un solo archivo, una sola función.** El módulo tiene 23 líneas. No hace
falta una capa de abstracción alrededor de un comando: la abstracción útil
está en "pedir una snapshot" y en "que el fallo no sea bloqueante".

**El error se propaga, la decisión no.** `Create` devuelve error y no
decide si eso interrumpe o no. Quien llama (`internal/update`) sabe que la
actualización de dotfiles tiene un 3-way merge de respaldo y por eso
degrada el fallo a warning.

**No se consulta si snapper tiene config `home`.** La comprobación
explícita de la configuración de snapper agregaría un segundo comando y un
punto de falla más. Si la config no está, `snapper` falla y el error se
reporta igual.

**No se hace copia manual de archivos.** Copiar el home a mano duplicaría
lo que snapper ya hace con subvolumes y no sería atómico.

## Archivos

| Archivo | Qué hace |
|---|---|
| `snapshot.go` | Todo el módulo: la función `Create` y su verificación de `snapper` en el `PATH`. |

Cobertura: este módulo **no tiene archivo de test propio**. No es posible
probar `Create` sin `snapper` instalado y con un subvolume `home`
configurado, así que la cobertura es manual. La lógica de decisión
(que el fallo sea warning y no bloqueo) vive en `internal/update`.

## Qué NO hace

- **No copia archivos.** Delega en `snapper`.
- **No bloquea la operación** si falla. Solo informa.
- **No lista, no borra y no poda snapshots.** Eso es `internal/snapshots`.
- **No toca otros subvolumes.** Solo `home`.
- **No restaura.** Solo crea.
