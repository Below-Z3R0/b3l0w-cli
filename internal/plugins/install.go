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
)

// registryTarball es el repo del registry comprimido. Se usa el tarball
// y no archivos sueltos porque un plugin tiene varios .luau y no hay
// listado de archivos en el manifiesto.
const registryTarball = "https://codeload.github.com/Below-Z3R0/b3l0w-plugins/tar.gz/refs/heads/main"

// Install baja e instala uno o todos los plugins del manifiesto.
func Install(m *Manifest, id, pluginsDir string, dryRun bool) error {
	if err := EnsureSource(); err != nil {
		fmt.Printf("  ! source: %v\n", err)
	}
	targets, err := selectPlugins(m, id)
	if err != nil {
		return err
	}
	for _, entry := range targets {
		if err := installOne(entry, pluginsDir, dryRun); err != nil {
			return fmt.Errorf("%s: %w", entry.ID, err)
		}
	}
	return nil
}

// Update compara la versión instalada con la del manifiesto.
func Update(m *Manifest, id, pluginsDir string, dryRun bool) error {
	if err := EnsureSource(); err != nil {
		fmt.Printf("  ! source: %v\n", err)
	}
	targets, err := selectPlugins(m, id)
	if err != nil {
		return err
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
	if home, err := os.UserHomeDir(); err == nil {
		pattern := filepath.Join(home, ".local", "state", "noctalia", "plugins", "materialized", "*", slug(entry))
		if matches, err := filepath.Glob(pattern); err == nil {
			dirs = append(dirs, matches...)
		}
	}
	return dirs
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
