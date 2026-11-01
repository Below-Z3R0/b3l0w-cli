package wecli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scanItem es la forma que consume el plugin: exactamente los campos que usa
// una tarjeta (id, título, ruta y tipo), sin nada del motor.
type scanItem struct {
	ID   string `json:"id"`
	File string `json:"file"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	Mod  int64  `json:"mod"`
	Name string `json:"name"`
	// Title es lo que muestra la tarjeta: sin esto el panel cae al id, que es
	// la ruta absoluta completa.
	Title string `json:"title"`
	// Thumb sólo viene con --thumbs: la miniatura ya generada.
	Thumb string `json:"thumb,omitempty"`
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".bmp": true, ".avif": true, ".gif": true, ".tif": true, ".tiff": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".webm": true, ".mkv": true, ".mov": true, ".avi": true, ".m4v": true,
}

// humanTitle convierte "2026-10-23_07-47-12" o "mi_fondo_bonito.jpg" en algo
// legible: sin extensión, guiones y guiones bajos a espacios.
func humanTitle(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	base = strings.Join(strings.Fields(base), " ")
	if base == "" {
		return name
	}
	return base
}

// cmdScan lista imágenes o videos de una carpeta para que el plugin no tenga
// que recorrer el árbol en Luau: el host da 25 ms por callback y un ~/Pictures
// grande lo hacía caer (y con el service muerto ningún wallpaper se aplicaba).
//
//	uso: b3l0w we scan [--dir RUTA]... [--kind image|video] [--max 400] [--depth 2]
//	     b3l0w we scan --extensions
func cmdScan(args []string) int {
	dirs := []string{}
	kind := "image"
	max := 400
	depth := 2
	jsonOut := false
	listExts := false
	sortMode := "name"
	thumbDir := ""
	thumbSize := 320

	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, bool) {
			if _, after, found := strings.Cut(a, "="); found {
				return after, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we scan [--dir RUTA]... [--kind image|video] [--max 400] [--depth 2] [--sort name|date|random] [--thumbs DIR [--thumb-size 320]] [--json]")
			return 0
		case a == "--extensions":
			listExts = true
		case a == "--json":
			jsonOut = true
		case a == "--thumbs" || strings.HasPrefix(a, "--thumbs="):
			v, ok := value()
			if !ok {
				return fail("--thumbs necesita una carpeta de salida")
			}
			thumbDir = v
		case a == "--thumb-size" || strings.HasPrefix(a, "--thumb-size="):
			v, ok := value()
			if !ok {
				return fail("--thumb-size necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &thumbSize); err != nil || thumbSize < 64 {
				return fail("--thumb-size inválido: %s", v)
			}
		case a == "--sort" || strings.HasPrefix(a, "--sort="):
			v, ok := value()
			if !ok {
				return fail("--sort necesita un valor")
			}
			if v != "name" && v != "date" && v != "random" {
				return fail("--sort inválido: %s (name|date|random)", v)
			}
			sortMode = v
		case a == "--dir" || strings.HasPrefix(a, "--dir="):
			v, ok := value()
			if !ok {
				return fail("--dir necesita un valor")
			}
			dirs = append(dirs, v)
		case a == "--kind" || strings.HasPrefix(a, "--kind="):
			v, ok := value()
			if !ok {
				return fail("--kind necesita un valor")
			}
			kind = v
		case a == "--max" || strings.HasPrefix(a, "--max="):
			v, ok := value()
			if !ok {
				return fail("--max necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &max); err != nil || max <= 0 {
				return fail("--max inválido: %s", v)
			}
		case a == "--depth" || strings.HasPrefix(a, "--depth="):
			v, ok := value()
			if !ok {
				return fail("--depth necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &depth); err != nil || depth < 0 {
				return fail("--depth inválido: %s", v)
			}
		default:
			// Una ruta suelta se toma como carpeta.
			if !strings.HasPrefix(a, "-") {
				dirs = append(dirs, a)
			}
		}
	}

	exts := imageExts
	itemType := "image"
	if kind == "video" {
		exts, itemType = videoExts, "video"
	}
	if listExts {
		out := make([]string, 0, len(exts))
		for e := range exts {
			out = append(out, e)
		}
		sort.Strings(out)
		encoded, _ := json.Marshal(out)
		fmt.Println(string(encoded))
		return 0
	}

	if len(dirs) == 0 {
		return fail("indicá al menos una carpeta con --dir")
	}

	items := make([]scanItem, 0, max)
	seen := map[string]bool{}

	for _, dir := range dirs {
		root := dir
		if strings.HasPrefix(root, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				root = filepath.Join(home, root[2:])
			}
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}

		rootDepth := strings.Count(abs, string(filepath.Separator))
		_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// Una carpeta sin permisos no corta el escaneo.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if len(items) >= max {
				return fs.SkipAll
			}
			name := d.Name()
			if d.IsDir() {
				if path != abs && (strings.HasPrefix(name, ".") || name == "node_modules") {
					return fs.SkipDir
				}
				if strings.Count(path, string(filepath.Separator))-rootDepth > depth {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(name, ".") {
				return nil
			}
			if !exts[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			size, mod := int64(0), int64(0)
			if fi, statErr := d.Info(); statErr == nil {
				size = fi.Size()
				mod = fi.ModTime().Unix()
			}
			items = append(items, scanItem{
				ID:    path,
				File:  path,
				Type:  itemType,
				Size:  size,
				Mod:   mod,
				Name:  humanTitle(name),
				Title: humanTitle(name),
			})
			return nil
		})
		if len(items) >= max {
			break
		}
	}

	switch sortMode {
	case "date":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Mod > items[j].Mod
		})
	case "random":
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	default:
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].File) < strings.ToLower(items[j].File)
		})
	}

	// Miniaturas sólo para imágenes: un video necesita extraer un cuadro y para
	// eso el plugin ya tiene su propio camino.
	if thumbDir != "" && itemType == "image" && len(items) > 0 {
		makeThumbs(items, thumbDir, thumbSize)
	}

	if jsonOut {
		encoded, err := json.Marshal(items)
		if err != nil {
			return fail("no se pudo serializar: %v", err)
		}
		fmt.Println(string(encoded))
		return 0
	}

	for _, item := range items {
		fmt.Println(item.File)
	}
	return 0
}
