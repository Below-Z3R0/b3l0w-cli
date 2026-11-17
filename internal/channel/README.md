# channel

Guarda si el escritorio sigue en `stable` o pasó a `edge`.

## Qué hace y por qué existe

El escritorio de b3l0w (umbriel, noctalia) se distribuye en dos canales:
el estable, que es lo que instala el instalador, y el de desarrollo,
que se mueve todos los días. El CLI necesita saber en cuál está el
usuario para:

- Mostrar el canal en `b3l0w channel` y en los chequeos.
- Sugerir los paquetes correctos al actualizar el escritorio: en
  `stable` son `umbriel` y `noctalia`; en `edge`, `umbriel-git` y
  `noctalia-git`.

Un archivo de texto con el nombre del canal alcanza. Todo lo demás
sería complejidad gratis.

## API

| Función | Qué hace |
|---|---|
| `Path() string` | Ruta del archivo: `~/.config/b3l0w/channel` |
| `Current() string` | Canal vigente: el del archivo, o el detectado |
| `Detect() string` | Canal real según los paquetes instalados |
| `Set(name string) error` | Escribe el canal pedido |
| `Stable`, `Edge` (consts) | Los dos nombres válidos: `"stable"`, `"edge"` |

```go
// Mostrar el canal actual.
fmt.Println(channel.Current())

// Cambiarlo.
if err := channel.Set(channel.Edge); err != nil {
    return err
}
```

## Dónde se persiste

`~/.config/b3l0w/channel`: un archivo con el nombre del canal y un
salto de línea. Vive en el home del usuario, no en `/etc`: el canal es
una preferencia por máquina, y `b3l0w` corre como el usuario aunque
algunos de sus comandos pidan sudo.

## Decisiones

**El archivo manda, la detección es el respaldo.** `Current()` lee el
archivo; si no existe o está vacío, cae a `Detect()`. Así el CLI
funciona recién instalado, sin un paso de configuración, y todavía
sabe en qué canal está.

**La detección mira los paquetes instalados.** `Detect()` consulta con
`pacman -Q`: si está `umbriel-git` o `noctalia-git`, el escritorio es
`edge`; si no, es `stable`. Es el estado real de la máquina, no una
preferencia que alguien pueda olvidar.

**`Set` valida antes de escribir.** Un canal que no sea `stable` ni
`edge` es un error, no un archivo con garbage adentro. El mensaje dice
exactamente qué usar: `canal inválido: %s (usá stable o edge)`.

**`Set` crea el directorio.** `~/.config/b3l0w/` puede no existir: se
crea con `0755` y el archivo se escribe con `0644`.

**Detectar y leer son separados.** `Detect()` es consultable por sí
mismo (el chequeo del escritorio necesita saber el canal real aunque
haya archivo). `Current()` es la combinación de ambos.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `channel.go` | Todo el módulo: `Path`, `Current`, `Detect`, `Set` |
| `channel_test.go` | Tests de set/get y de validación |

## Qué NO hace

- **No instala ni quita paquetes.** El canal es una etiqueta; el
  `update` del escritorio muestra el comando de pacman y el usuario lo
  corre.
- **No cambia la config de pacman.** No toca mirrors ni `pacman.conf`.
- **No migra entre canales.** Guardar `edge` no desinstala el paquete
  estable: eso es del instalador.
- **No consulta la red.** El canal es local.

## Tests

2 tests: `Set` + `Current` ida y vuelta entre los dos canales (con
`HOME` temporal) y el rechazo de un canal inválido como `"nightly"`.
