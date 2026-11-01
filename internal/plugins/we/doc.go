// Package we implementa el soporte de Wallpaper Engine para el CLI b3l0w:
// catálogo del Workshop de Steam, extracción a video, posters, miniaturas y
// manifiestos.
//
// # Qué resuelve
//
// Un wallpaper de Wallpaper Engine vive en
// ~/.steam/steam/steamapps/workshop/content/431960/<id>/ con un project.json
// adentro. Según su `type`:
//
//   - video: el campo `file` es un mp4/webm real. Se copia o remuxea con
//     ffmpeg; no hace falta el motor.
//   - scene / web: no hay media reproducible, hay que renderizarlo. El motor
//     (linux-wallpaperengine) puede escribir un PNG offscreen con
//     --screenshot, que es lo que usa Poster.
//
// El paquete no imprime nada: devuelve structs y errores. Toda salida (tabla o
// JSON) la arma el subpaquete cli, para que stdout quede limpio y un consumidor
// pueda hacer `b3l0w we list --json | jq` sin filtrar líneas de log.
//
// # Archivos
//
//	catalog.go   Scan/Filter/Find: recorre el workshop y normaliza cada
//	             project.json a Item (incluye `properties` tal cual, que es lo
//	             que el plugin de Noctalia usa para configurar cada fondo).
//	extract.go   remux o recodificación del `file` con ffmpeg.
//	poster.go    --screenshot del motor para los tipos sin media.
//	ffmpeg.go    localización de ffmpeg/ffprobe (PATH y rutas de Hermes) y
//	             helpers de ejecución con captura de stderr.
//	manifest.go  manifest.json: id → archivo generado, con estado por item.
//	deps.go      qué falta en el sistema, con la ruta y la versión de cada
//	             binario.
//
// # Invariantes
//
//  1. Nunca asumir ffmpeg en PATH: se resuelve con Locate/LocateProbe, que
//     también miran el directorio de herramientas de Hermes.
//  2. El `type` del project.json viene con capitalización inconsistente
//     ("Video", "video"); NormalizeType lo baja a minúsculas.
//  3. Un item roto (sin project.json, sin `file`) no aborta el catálogo: se
//     devuelve con Err cargado y el resto sigue.
//  4. Ningún proceso del motor queda vivo: cada corrida cierra el suyo.
//
// # Contrato con el plugin de Noctalia
//
// El plugin `b3l0w/wallpaper` consume este paquete a través del CLI, no de la
// biblioteca (son lenguajes distintos). Los comandos `we list --json`,
// `we scan --json` y `we solid` existen para eso: el host de plugins da 25 ms
// de CPU por callback y recorrer un workshop en Luau medía 26-30 ms.
// Ver SPEC-we.md en la raíz del repo.
package we
