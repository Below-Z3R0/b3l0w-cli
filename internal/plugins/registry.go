package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry es un plugin del manifiesto de distribución.
type Entry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	PluginAPI int    `json:"plugin_api"`
	SHA256    string `json:"sha256,omitempty"`
}

// Manifest es plugins/plugins.json del repo b3l0w.
type Manifest struct {
	Version int     `json:"version"`
	Plugins []Entry `json:"plugins"`
}

// LoadManifest lee el manifiesto.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version == 0 || len(m.Plugins) == 0 {
		return nil, fmt.Errorf("manifiesto de plugins vacío o sin versión")
	}
	return &m, nil
}

// SaveManifest lo escribe ordenado por id.
func SaveManifest(path string, m *Manifest) error {
	sort.Slice(m.Plugins, func(i, j int) bool { return m.Plugins[i].ID < m.Plugins[j].ID })
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// slug es el nombre de carpeta que espera Noctalia: la parte después
// de la barra del id ("b3l0w/wallpaper" -> "wallpaper").
func slug(e Entry) string {
	if i := strings.LastIndex(e.ID, "/"); i >= 0 && i+1 < len(e.ID) {
		return e.ID[i+1:]
	}
	return e.Path
}

// Sync copia cada plugin desde su clon local al repo del registry.
//
// El manifiesto manda: dice qué repo y qué carpeta. El sync refresca
// versión y plugin_api desde el plugin.toml copiado.
func Sync(srcRoot, registryDir, manifestPath string, m *Manifest) error {
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		return err
	}
	for i := range m.Plugins {
		entry := &m.Plugins[i]
		src := filepath.Join(srcRoot, entry.Path)
		dst := filepath.Join(registryDir, slug(*entry))
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("%s: no existe el clon local %s", entry.ID, src)
		}
		// Limpiar la carpeta nueva y la vieja (b3l0w-<name>).
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if entry.Path != slug(*entry) {
			if err := os.RemoveAll(filepath.Join(registryDir, entry.Path)); err != nil {
				return err
			}
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
		meta, err := readPluginMeta(filepath.Join(dst, "plugin.toml"))
		if err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
		entry.Version = meta.Version
		entry.PluginAPI = meta.PluginAPI
	}
	return SaveManifest(manifestPath, m)
}

// copyTree copia sin .git, tests, node_modules ni backups.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		base := filepath.Base(path)
		if info.IsDir() {
			switch base {
			case ".git", "tests", "node_modules", ".github", ".vscode", ".zed":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(base, ".bak") || strings.HasPrefix(base, ".bak-") {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(filepath.Join(dst, rel))
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

type pluginMeta struct {
	ID           string
	Name         string
	Version      string
	PluginAPI    int
	Author       string
	License      string
	Icon         string
	Description  string
	Tags         []string
	Dependencies []string
}

var (
	reString = func(key string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^` + key + `\s*=\s*"([^"]*)"`)
	}
	reInt = func(key string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^` + key + `\s*=\s*(\d+)`)
	}
	reTags = regexp.MustCompile(`(?m)^tags\s*=\s*\[([^\]]*)\]`)
	reDeps = regexp.MustCompile(`(?m)^dependencies\s*=\s*\[([^\]]*)\]`)
)

func readPluginMeta(path string) (pluginMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pluginMeta{}, err
	}
	text := string(data)
	meta := pluginMeta{}
	if mm := reString("id").FindStringSubmatch(text); mm != nil {
		meta.ID = mm[1]
	}
	if mm := reString("name").FindStringSubmatch(text); mm != nil {
		meta.Name = mm[1]
	}
	if mm := reString("version").FindStringSubmatch(text); mm != nil {
		meta.Version = mm[1]
	}
	if mm := reInt("plugin_api").FindStringSubmatch(text); mm != nil {
		meta.PluginAPI, _ = strconv.Atoi(mm[1])
	}
	if mm := reString("author").FindStringSubmatch(text); mm != nil {
		meta.Author = mm[1]
	}
	if mm := reString("license").FindStringSubmatch(text); mm != nil {
		meta.License = mm[1]
	}
	if mm := reString("icon").FindStringSubmatch(text); mm != nil {
		meta.Icon = mm[1]
	}
	if mm := reString("description").FindStringSubmatch(text); mm != nil {
		meta.Description = mm[1]
	}
	if mm := reTags.FindStringSubmatch(text); mm != nil {
		for _, tag := range strings.Split(mm[1], ",") {
			tag = strings.Trim(strings.TrimSpace(tag), `"`)
			if tag != "" {
				meta.Tags = append(meta.Tags, tag)
			}
		}
	}
	if mm := reDeps.FindStringSubmatch(text); mm != nil {
		for _, dep := range strings.Split(mm[1], ",") {
			dep = strings.Trim(strings.TrimSpace(dep), `"`)
			if dep != "" {
				meta.Dependencies = append(meta.Dependencies, dep)
			}
		}
	}
	if meta.ID == "" || meta.Version == "" {
		return meta, fmt.Errorf("plugin.toml sin id o version")
	}
	return meta, nil
}

// Build genera catalog.toml en el registry con el formato de Noctalia.
func Build(registryDir string, m *Manifest) error {
	var b strings.Builder
	b.WriteString("# This file is auto-generated by `b3l0w plugins build`.\n")
	b.WriteString("# Do not edit manually.\n\n")

	now := time.Now().Unix()
	for _, entry := range m.Plugins {
		meta, err := readPluginMeta(filepath.Join(registryDir, slug(entry), "plugin.toml"))
		if err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
		fmt.Fprintf(&b, "[[plugin]]\n")
		fmt.Fprintf(&b, "id = %q\n", meta.ID)
		fmt.Fprintf(&b, "name = %q\n", meta.Name)
		fmt.Fprintf(&b, "version = %q\n", meta.Version)
		fmt.Fprintf(&b, "updated_at = %d\n", now)
		fmt.Fprintf(&b, "added_at = %d\n", now)
		fmt.Fprintf(&b, "author = %q\n", meta.Author)
		fmt.Fprintf(&b, "license = %q\n", meta.License)
		fmt.Fprintf(&b, "icon = %q\n", meta.Icon)
		fmt.Fprintf(&b, "description = %q\n", meta.Description)
		fmt.Fprintf(&b, "plugin_api = %d\n", meta.PluginAPI)
		if len(meta.Tags) > 0 {
			fmt.Fprintf(&b, "tags = [%s]\n", quoteList(meta.Tags))
		} else {
			b.WriteString("tags = []\n")
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(registryDir, "catalog.toml"), []byte(b.String()), 0o644)
}

// DefaultPluginsDir es donde Noctalia guarda los plugins locales.
func DefaultPluginsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "noctalia", "plugins")
}

// DefaultManifestPath es plugins/plugins.json del repo de distribución.
func DefaultManifestPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "Github", "b3l0w", "plugins", "plugins.json")
}

// Dependencies devuelve las dependencias declaradas por cada plugin
// instalado o materializado.
func Dependencies() (map[string][]string, error) {
	m, err := LoadManifest(DefaultManifestPath())
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string)
	for _, entry := range m.Plugins {
		for _, dir := range candidateDirs(DefaultPluginsDir(), entry) {
			meta, err := readPluginMeta(filepath.Join(dir, "plugin.toml"))
			if err == nil {
				out[entry.ID] = meta.Dependencies
				break
			}
		}
	}
	return out, nil
}

func quoteList(items []string) string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = strconv.Quote(item)
	}
	return strings.Join(out, ", ")
}
