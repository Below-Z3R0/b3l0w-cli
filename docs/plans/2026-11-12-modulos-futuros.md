# Módulos futuros del CLI

> Registro de módulos acordados pero **no implementados**. Cada uno tiene su
> alcance, su justificación y el criterio para saber cuándo arrancarlo. No son
> promesas: son decisiones tomadas en una sesión de diseño para no perderlas.

## `internal/device` — control de dispositivos por ADB

**Qué resuelve:** controlar teléfonos Android y Android TV de la LAN desde el
CLI: abrir apps, tocar, escribir, capturar pantalla, leer el árbol de UI.

**Por qué en el CLI y no como plugin/MCP:**
- El plugin `phone-harness` ya demuestra que la mecánica es ADB puro. No hay
  magia que reimplementar.
- Meterlo en Docker obliga a USB passthrough + `--network host` + udev: más
  frágil que invocar `adb` desde el host.
- El CLI es el punto natural: `b3l0w device list|tap|type|screen|open`.

**Alcance mínimo (núcleo):**

| Capacidad | Implementación |
|---|---|
| Resolver dispositivo | `adb devices -l`, `ANDROID_SERIAL`, wireless `adb connect` |
| Captura | `adb exec-out screencap -p` |
| Árbol de UI | `adb exec-out uiautomator dump /dev/tty` + parseo XML |
| Tap / swipe / texto | `adb shell input tap/swipe/text` |
| Apps | `pm list packages`, `am start -W` |
| Gate de bloqueo | `dumpsys power` antes de tocar |

**Tamaño estimado:** 600–800 líneas de Go con tests (el parseo de
`uiautomator` es la parte delicada).

**Criterio para arrancar:** cuando haga falta automatizar algo recurrente en el
TV o el teléfono (no por tenerlo).

**Antecedente:** plugin `phone-harness` (4.9k ★), auditado el 2026-11-12.
Su superficie de helpers es la referencia funcional.

---

## `internal/knowledge` — cliente de knowledge-resolver

**Qué resuelve:** que el CLI pueda consultar el grafo del vault (links
resueltos, huérfanos, backlinks, rutas) sin depender de Obsidian ni de Argos.

**Contexto:** existe `grafo-obsidian.ts` (905 líneas, Bun/Node) que replica el
resolver de links de Obsidian 1.13.7 y corre el vault completo en ~1,2 s
(6.717 nodos, 22.504 links). Ese script es la base del proyecto
`knowledge-resolver`.

**Qué expone el resolver:**

| Operación | Uso desde el CLI |
|---|---|
| `resolve(link, from)` | ¿a qué nota apunta este link? |
| `unresolved()` | links rotos, con archivo y línea |
| `orphans()` | notas sin backlinks |
| `backlinks(note)` | quién apunta a esta nota |
| `graph()` | nodos y aristas para exportar |

**Integración:** el CLI no reimplementa el resolver. Lo consume:
- como **librería** si el proyecto expone un paquete (preferido), o
- como **binario** con `--format=json` (fallback simple y estable).

**Criterio para arrancar:** cuando el proyecto `knowledge-resolver` tenga una
API estable y el CLI necesite validar el vault como parte de un comando
(`b3l0w vault check`, `b3l0w vault orphans`).

**Dependencia:** `Github/Knowledge-Resolver/` (hoy vacío).

---

## Orden sugerido

1. `knowledge-resolver` como proyecto independiente (el script ya existe).
2. `internal/knowledge` en el CLI, consumiéndolo.
3. `internal/device`, cuando aparezca la necesidad concreta.
