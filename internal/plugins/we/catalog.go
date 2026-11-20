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
			resolved, err := resolveInDir(dir, proj.File)
			if err != nil {
				it.Err = fmt.Sprintf("file del project.json: %v", err)
			} else {
				it.File = resolved
			}
		}
		if proj.Preview != "" {
			resolved, err := resolveInDir(dir, proj.Preview)
			if err != nil {
				if it.Err == "" {
					it.Err = fmt.Sprintf("preview del project.json: %v", err)
				}
			} else {
				it.Preview = resolved
			}
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

// resolveInDir resuelve un nombre de archivo que viene de afuera (el
// project.json lo baja Steam) y lo deja dentro del directorio del item.
//
// filepath.Join no neutraliza los `..`: sólo limpia la ruta. Un `file` con
// `../` sale del item y `we convert` copia a la salida un archivo cualquiera
// del disco. Por eso la ruta se resuelve y después se comprueba que siga
// dentro: si se sale, el item vuelve roto en vez de apuntar afuera.
func resolveInDir(dir, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("ruta vacía")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("la ruta %q es absoluta: se espera un archivo dentro del item", name)
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolver %s: %w", dir, err)
	}
	target := filepath.Join(base, name)
	if !dentroDe(base, target) {
		return "", fmt.Errorf("la ruta %q sale del directorio del item", name)
	}
	// Si el archivo existe, se resuelve el symlink: un enlace dentro del item
	// tampoco puede apuntar afuera.
	if real, err := filepath.EvalSymlinks(target); err == nil {
		realBase := base
		if r, err := filepath.EvalSymlinks(base); err == nil {
			realBase = r
		}
		if !dentroDe(realBase, real) {
			return "", fmt.Errorf("la ruta %q apunta afuera del item", name)
		}
	}
	return target, nil
}

// dentroDe dice si target está dentro de base (o es base).
func dentroDe(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
//
// El tipo se valida contra los que el catálogo conoce: un tipo que no existe
// (un typo) no puede devolver el catálogo entero, y "unknown" es un tipo
// legítimo que filtra por los items cuyo project.json no dice nada.
func Filter(items []Item, typ, query string) []Item {
	q := strings.ToLower(strings.TrimSpace(query))
	filtraTipo := strings.TrimSpace(typ) != ""
	norm := ""
	if filtraTipo {
		if !ValidType(typ) {
			// Un tipo inexistente no matchea nada: devolver todo haría
			// creer al usuario que filtró.
			return []Item{}
		}
		norm = NormalizeType(typ)
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if filtraTipo && it.Type != norm {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Title), q) && !strings.Contains(it.ID, q) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// ValidType dice si el tipo pedido existe en el catálogo. "unknown" cuenta:
// es el tipo de los items cuyo project.json no declara un tipo conocido.
func ValidType(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "video", "scene", "web", "website", "unknown":
		return true
	}
	return false
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

// transliteraciones son las letras acentuadas (y las que no son ASCII) con su
// equivalente ASCII. Antes el slug las borraba: "Café Münster Ñandú" quedaba
// "caf-m-nster-and". Ahora se transliteran: "cafe-munster-nandu".
var transliteraciones = map[rune]string{
	'á': "a", 'à': "a", 'ä': "a", 'â': "a", 'ã': "a", 'å': "a",
	'é': "e", 'è': "e", 'ë': "e", 'ê': "e",
	'í': "i", 'ì': "i", 'ï': "i", 'î': "i",
	'ó': "o", 'ò': "o", 'ö': "o", 'ô': "o", 'õ': "o", 'ø': "o",
	'ú': "u", 'ù': "u", 'ü': "u", 'û': "u",
	'ñ': "n", 'ç': "c", 'ý': "y", 'ÿ': "y", 'š': "s", 'ž': "z", 'ł': "l",
	'ß': "ss", 'æ': "ae", 'œ': "oe",
}

// Slug convierte un título en un nombre de archivo seguro (kebab-case, <=60).
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = transliterar(s)
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

// transliterar reemplaza las letras acentuadas por su equivalente ASCII, para
// que el slug no pierda letras: "ñ" es "n", no un hueco.
func transliterar(s string) string {
	var b strings.Builder
	for _, r := range s {
		if repl, ok := transliteraciones[r]; ok {
			b.WriteString(repl)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
