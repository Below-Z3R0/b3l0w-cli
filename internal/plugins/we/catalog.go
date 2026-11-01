package we

// Este archivo cubre el descubrimiento del workshop y la normalización de cada
// project.json a Item. El paquete está documentado en doc.go.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// WorkshopSub es la ruta del workshop dentro de la biblioteca de Steam.
const WorkshopSub = "steamapps/workshop/content/431960"

// Item es una entrada del catálogo ya normalizada.
type Item struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"` // video | scene | web | unknown
	Dir      string   `json:"dir"`
	File     string   `json:"file,omitempty"`    // ruta absoluta al medio reproducible
	Preview  string   `json:"preview,omitempty"` // ruta absoluta al preview
	Bytes    int64    `json:"bytes"`
	Tags     []string `json:"tags,omitempty"`
	Content  string   `json:"content_rating,omitempty"`
	Duration float64  `json:"duration,omitempty"`
	Width    int      `json:"width,omitempty"`
	Height   int      `json:"height,omitempty"`
	Err      string   `json:"error,omitempty"`
	// Properties es el bloque `general.properties` del project.json tal cual:
	// el panel lo usa para armar la tarjeta de configuración por wallpaper.
	Properties map[string]json.RawMessage `json:"properties,omitempty"`
}

// Project es el subconjunto de project.json que usamos.
type Project struct {
	Title         string          `json:"title"`
	Type          string          `json:"type"`
	File          string          `json:"file"`
	Preview       string          `json:"preview"`
	ContentRating string          `json:"contentrating"`
	Tags          []string        `json:"tags"`
	General       json.RawMessage `json:"general"`
}

// DefaultRoot devuelve la primera raíz de workshop que exista, o un error con
// las rutas que probó para que el usuario sepa qué falta.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("directorio home: %w", err)
	}
	candidates := []string{
		filepath.Join(home, ".steam", "steam", WorkshopSub),
		filepath.Join(home, ".local", "share", "Steam", WorkshopSub),
		filepath.Join(home, ".steam", "root", WorkshopSub),
	}
	var probed []string
	for _, c := range candidates {
		probed = append(probed, c)
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("no encontré el workshop de Wallpaper Engine; probé: %s", strings.Join(probed, ", "))
}

// NormalizeType baja a minúsculas y unifica los tipos válidos.
func NormalizeType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "video":
		return "video"
	case "scene":
		return "scene"
	case "web", "website":
		return "web"
	case "":
		return "unknown"
	default:
		return "unknown"
	}
}

// LoadProject lee y valida el project.json de un directorio de item.
func LoadProject(dir string) (Project, error) {
	var p Project
	b, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		return p, fmt.Errorf("leer project.json de %s: %w", dir, err)
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, fmt.Errorf("parsear project.json de %s: %w", dir, err)
	}
	return p, nil
}

// Scan recorre la raíz y devuelve los items ordenados por título. Un item roto
// no rompe el scan: se devuelve con Err seteado.
func Scan(root string) ([]Item, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", root, err)
	}
	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			continue // no es un id de workshop
		}
		dir := filepath.Join(root, id)
		it := Item{ID: id, Dir: dir, Type: "unknown"}

		proj, err := LoadProject(dir)
		if err != nil {
			it.Title = id
			it.Err = err.Error()
			items = append(items, it)
			continue
		}
		it.Title = strings.TrimSpace(proj.Title)
		if it.Title == "" {
			it.Title = id
		}
		it.Type = NormalizeType(proj.Type)
		it.Content = proj.ContentRating
		it.Tags = proj.Tags
		if proj.File != "" {
			it.File = filepath.Join(dir, proj.File)
		}
		if proj.Preview != "" {
			it.Preview = filepath.Join(dir, proj.Preview)
		}
		it.Bytes = dirSize(dir)
		it.Properties = projectProperties(proj)
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})
	return items, nil
}

// projectProperties devuelve `general.properties` sin interpretarlo: la forma
// (type/value/text/options/order) es la que espera el plugin.
func projectProperties(proj Project) map[string]json.RawMessage {
	if len(proj.General) == 0 {
		return nil
	}
	var general struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(proj.General, &general); err != nil || len(general.Properties) == 0 {
		return nil
	}
	return general.Properties
}

// Filter aplica los filtros de `we list`. query busca en título sin distinguir
// mayúsculas; typ vacío no filtra.
func Filter(items []Item, typ, query string) []Item {
	typ = NormalizeType(typ)
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if typ != "unknown" && it.Type != typ {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Title), q) && !strings.Contains(it.ID, q) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// Find busca un item por id.
func Find(items []Item, id string) (Item, error) {
	for _, it := range items {
		if it.ID == id {
			return it, nil
		}
	}
	return Item{}, fmt.Errorf("no existe el wallpaper %q en el catálogo", id)
}

var slugDrops = regexp.MustCompile(`[^a-z0-9]+`)

// Slug convierte un título en un nombre de archivo seguro (kebab-case, <=60).
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugDrops.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	if s == "" {
		s = "wallpaper"
	}
	return s
}

// OutputName arma el nombre de archivo de salida para un item.
func OutputName(it Item, ext string) string {
	return it.ID + "-" + Slug(it.Title) + "." + strings.TrimPrefix(ext, ".")
}

// ErrEmpty identifica un catálogo sin items.
var ErrEmpty = errors.New("el catálogo está vacío")

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
