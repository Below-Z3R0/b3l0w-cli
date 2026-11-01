// Package wecli arma los comandos de `b3l0w we` sobre el paquete we.
// Acá vive solo el parseo de argumentos y la salida; la lógica está en we.
//
// # Contrato de salida
//
//   - stdout es sólo el resultado: tabla, lista de rutas o JSON completo.
//     Nunca logs ni progreso, para que `b3l0w we list --json | jq` funcione.
//   - stderr lleva el detalle cuando algo falla; el código de salida lo
//     distingue (ver fail).
//   - Los comandos con --json emiten exactamente un documento JSON.
//
// # Parseo de flags
//
// Sin dependencias: cada comando recorre args con un helper local que acepta
// tanto `--flag valor` como `--flag=valor`. Una ruta suelta se interpreta como
// la carpeta de trabajo del comando (por ejemplo en `scan`).
//
// # Archivos
//
//	root.go    dispatch, help y código de salida
//	list.go    `we list` / `we info` (tabla y JSON)
//	convert.go `we convert` (extraer o grabar a video + manifest)
//	poster.go  `we poster` (PNG por wallpaper con el motor)
//	still.go   `we still` (un cuadro de un video, para miniaturas)
//	scan.go    `we scan` (imágenes/videos de una carpeta, para el plugin)
//	solid.go   `we solid` (PNG de un solo color)
//	thumbs.go  miniaturas de imágenes (ffmpeg, en paralelo, cacheadas)
//	deps.go    `we deps` (qué falta y con qué versión)
package wecli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Below-Z3R0/b3l0w/internal/plugins/we"
)

const usage = `b3l0w we — wallpapers de Wallpaper Engine

uso: b3l0w we <comando> [opciones]

  list                     catálogo del workshop (tabla)
  list --json              lo mismo en JSON
  info <id>                detalle de un wallpaper
  convert --out DIR        convierte wallpapers a video (necesita ffmpeg)
  poster --out DIR         captura un PNG de cada wallpaper
  still --file V --out J   cuadro de un video (miniatura de videos locales)
  scan --dir R --kind image  lista imágenes o videos de una carpeta (JSON)
  solid --color H --out P  wallpaper de un solo color (PNG)
  deps                     qué falta en el sistema

Opciones de convert:
  --id ID            repetible; sin --all ni --id no hace nada
  --all              todos los del catálogo
  --method M         auto | extract | record   (default auto)
  --format F         mp4 | webm                (default mp4)
  --crf N            0 = copiar sin recodificar (default 18)
  --overwrite        pisa lo que ya existe
  --jobs N           conversiones en paralelo
  --manifest RUTA    default <out>/manifest.json
`

// Run despacha un subcomando y devuelve el código de salida.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "list":
		return cmdList(rest)
	case "info":
		return cmdInfo(rest)
	case "convert":
		return cmdConvert(rest)
	case "poster":
		return cmdPoster(rest)
	case "still":
		return cmdStill(rest)
	case "scan":
		return cmdScan(rest)
	case "solid":
		return cmdSolid(rest)
	case "deps":
		return cmdDeps(rest)
	}
	fmt.Fprintf(os.Stderr, "b3l0w we: comando desconocido %q\n", cmd)
	fmt.Print(usage)
	return 1
}

// fail imprime un error accionable y devuelve el código de salida.
func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "b3l0w we: "+format+"\n", a...)
	return 1
}

// catalog devuelve el catálogo del root pedido (o el default del sistema).
func catalog(root string) ([]we.Item, string, error) {
	if root == "" {
		var err error
		root, err = we.DefaultRoot()
		if err != nil {
			return nil, "", err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, "", fmt.Errorf("resolver %s: %w", root, err)
	}
	items, err := we.Scan(abs)
	if err != nil {
		return nil, abs, err
	}
	if len(items) == 0 {
		return nil, abs, we.ErrEmpty
	}
	return items, abs, nil
}

// pick resuelve los ids pedidos, o todo el catálogo con all.
func pick(items []we.Item, ids []string, all bool) ([]we.Item, error) {
	if all {
		return items, nil
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("falta --id <id> o --all")
	}
	chosen := make([]we.Item, 0, len(ids))
	for _, id := range ids {
		it, err := we.Find(items, id)
		if err != nil {
			return nil, err
		}
		chosen = append(chosen, it)
	}
	return chosen, nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
