package plugins

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-kit/system"
)

// registryTarball es el repo del registry comprimido. Se usa el tarball
// y no archivos sueltos porque un plugin tiene varios .luau y no hay
// listado de archivos en el manifiesto. Es el fallback cuando Noctalia
// no está instalado: con shell, el source es la vía principal.
const registryTarball = "https://codeload.github.com/Below-Z3R0/b3l0w-plugins/tar.gz/refs/heads/main"

// Install instala uno o todos los plugins. Con source de Noctalia no
// copia nada local: refresca el source, que es la única fuente. Copiar
// a ~/.local/share/noctalia/plugins/<slug> taparía el source con una
// versión vieja.
func Install(m *Manifest, id, pluginsDir string, dryRun bool) error {
	targets, err := selectPlugins(m, id)
	if err != nil {
		return err
	}
	if sourceReady() {
		return installFromSource(targets, dryRun)
	}
	if err := EnsureSource(); err == nil && sourceReady() {
		return installFromSource(targets, dryRun)
	}
	for _, entry := range targets {
		if err := installOne(entry, pluginsDir, dryRun); err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
	}
	return nil
}

// Update compara la versión instalada con la del manifiesto. Con
// source, el refresh lo hace Noctalia; sin shell, baja el tarball.
func Update(m *Manifest, id, pluginsDir string, dryRun bool) error {
	targets, err := selectPlugins(m, id)
	if err != nil {
		return err
	}
	if sourceReady() {
		return updateFromSource(targets, dryRun)
	}
	if err := EnsureSource(); err == nil && sourceReady() {
		return updateFromSource(targets, dryRun)
	}
	for _, entry := range targets {
		local := InstalledVersion(pluginsDir, entry)
		if local == "" {
			fmt.Printf("  ! %s: no instalado\n", entry.ID)
			continue
		}
		if local == entry.Version {
			fmt.Printf("  ✓ %s: al día (%s)\n", entry.ID, local)
			continue
		}
		fmt.Printf("  ▸ %s: %s -> %s\n", entry.ID, local, entry.Version)
		if err := installOne(entry, pluginsDir, dryRun); err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
	}
	return nil
}

func installFromSource(targets []Entry, dryRun bool) error {
	if dryRun {
		for _, entry := range targets {
			fmt.Printf("  → instalar %s %s (enable en Noctalia)\n", entry.ID, entry.Version)
		}
		return nil
	}
	if err := UpdateSource(SourceName); err != nil {
		return err
	}
	for _, entry := range targets {
		if !Enabled(entry.ID) {
			out, err := system.RunCombined("noctalia", "msg", "plugins", "enable", entry.ID)
			if err != nil {
				return fmt.Errorf("noctalia enable %s: %v: %s", entry.ID, err, out)
			}
			if !waitEnabled(entry.ID, 5*time.Second) {
				return fmt.Errorf("%s: Noctalia no lo habilitó; probá Browse Plugins", entry.ID)
			}
		}
		meta, ok := sourceMeta(entry)
		if !ok {
			return fmt.Errorf("%s: Noctalia no lo materializó; probá Browse Plugins", entry.ID)
		}
		if meta.Version != entry.Version {
			fmt.Printf("  ! %s: source %s, manifiesto %s\n", entry.ID, meta.Version, entry.Version)
		}
		fmt.Printf("  ✓ %s %s (source)\n", entry.ID, meta.Version)
	}
	return nil
}

// sourceMeta lee el plugin.toml materializado por Noctalia.
func sourceMeta(entry Entry) (pluginMeta, bool) {
	for _, dir := range materializedDirs(entry) {
		if meta, err := readPluginMeta(filepath.Join(dir, "plugin.toml")); err == nil {
			return meta, true
		}
	}
	return pluginMeta{}, false
}

// Enabled lee la salida de `noctalia msg plugins list`.
func Enabled(id string) bool {
	if !system.HaveCommand("noctalia") {
		return false
	}
	out, err := system.RunCombined("noctalia", "msg", "plugins", "list")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == id && fields[len(fields)-1] == "enabled" {
			return true
		}
	}
	return false
}

// waitEnabled espera a que Noctalia termine de exportar el estado; el
// enable contesta "ok (exporting in background)".
func waitEnabled(id string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if Enabled(id) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func updateFromSource(targets []Entry, dryRun bool) error {
	if dryRun {
		for _, entry := range targets {
			fmt.Printf("  → refrescar %s desde el source\n", entry.ID)
		}
		return nil
	}
	if err := UpdateSource(SourceName); err != nil {
		return err
	}
	for _, entry := range targets {
		meta, ok := sourceMeta(entry)
		if !ok {
			fmt.Printf("  ! %s: no instalado\n", entry.ID)
			continue
		}
		if meta.Version == entry.Version {
			fmt.Printf("  ✓ %s: al día (%s)\n", entry.ID, meta.Version)
			continue
		}
		fmt.Printf("  ▸ %s: %s -> %s (source)\n", entry.ID, entry.Version, meta.Version)
	}
	return nil
}

func selectPlugins(m *Manifest, id string) ([]Entry, error) {
	if id == "" {
		return m.Plugins, nil
	}
	for _, entry := range m.Plugins {
		if entry.ID == id {
			return []Entry{entry}, nil
		}
	}
	return nil, fmt.Errorf("plugin %s no está en el manifiesto", id)
}

func installOne(entry Entry, pluginsDir string, dryRun bool) error {
	dest := filepath.Join(pluginsDir, slug(entry))
	if dryRun {
		fmt.Printf("  → %s %s en %s\n", entry.ID, entry.Version, dest)
		return nil
	}
	// Borrar la carpeta nueva y la vieja (b3l0w-<name>) si existen.
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if entry.Path != slug(entry) {
		if err := os.RemoveAll(filepath.Join(pluginsDir, entry.Path)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	tmp, err := downloadTarball()
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	prefix := "b3l0w-plugins-main/" + slug(entry) + "/"
	if err := extractPrefix(tmp, prefix, dest); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dest, "plugin.toml")); err != nil {
		return fmt.Errorf("el plugin no trae plugin.toml")
	}
	fmt.Printf("  ✓ %s %s\n", entry.ID, entry.Version)
	return nil
}

func downloadTarball() (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(registryTarball)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "b3l0w-plugins-*.tar.gz")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), tmp.Close()
}

// extractPrefix extrae solo las entradas que empiezan con prefix,
// quitándolo. Rechaza rutas que se escapan del destino.
func extractPrefix(tarball, prefix, dest string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, prefix)
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("ruta insegura en el tar: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

// candidateDirs son los lugares donde puede vivir un plugin instalado:
// el directorio local, la carpeta legacy y el source materializado.
func candidateDirs(pluginsDir string, entry Entry) []string {
	dirs := []string{
		filepath.Join(pluginsDir, slug(entry)),
		filepath.Join(pluginsDir, entry.Path),
	}
	dirs = append(dirs, materializedDirs(entry)...)
	return dirs
}

func materializedDirs(entry Entry) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	pattern := filepath.Join(home, ".local", "state", "noctalia", "plugins", "materialized", "*", slug(entry))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

// sourceVersion lee la versión materializada por Noctalia, o "".
func sourceVersion(entry Entry) string {
	for _, dir := range materializedDirs(entry) {
		if meta, err := readPluginMeta(filepath.Join(dir, "plugin.toml")); err == nil {
			return meta.Version
		}
	}
	return ""
}

// Origin dice de dónde sale el plugin instalado: "local", "source" o "".
func Origin(pluginsDir string, entry Entry) string {
	if _, err := os.Stat(filepath.Join(pluginsDir, slug(entry), "plugin.toml")); err == nil {
		return "local"
	}
	if sourceVersion(entry) != "" {
		return "source"
	}
	return ""
}

// InstalledVersion lee la versión del plugin instalado, o "".
func InstalledVersion(pluginsDir string, entry Entry) string {
	for _, dir := range candidateDirs(pluginsDir, entry) {
		if meta, err := readPluginMeta(filepath.Join(dir, "plugin.toml")); err == nil {
			return meta.Version
		}
	}
	return ""
}

// Installed dice si el plugin está instalado (local o desde un source).
func Installed(pluginsDir string, entry Entry) bool {
	return InstalledVersion(pluginsDir, entry) != ""
}

func sourceReady() bool {
	return HasSource(SourceName)
}
