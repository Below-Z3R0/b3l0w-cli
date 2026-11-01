package we

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestVersion es la versión del contrato que lee el plugin de Noctalia.
const ManifestVersion = 1

// Estados posibles de un Entry.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Manifest es el índice de archivos generados. Es el contrato con el plugin de
// Noctalia: los tags JSON no se tocan sin coordinar con el plugin.
type Manifest struct {
	Version   int       `json:"version"`
	Generated time.Time `json:"generated"`
	Root      string    `json:"root"`
	Out       string    `json:"out"`
	Items     []Entry   `json:"items"`
}

// Entry es un wallpaper ya procesado.
type Entry struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Status   string  `json:"status"`
	File     string  `json:"file"`
	Source   string  `json:"source"`
	Method   string  `json:"method"`
	Bytes    int64   `json:"bytes"`
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Err      string  `json:"error"`
}

// LoadManifest lee el manifest de disco. Si el archivo no existe devuelve un
// error que envuelve os.ErrNotExist, para que el llamador decida si arranca uno
// nuevo. Un manifest vacío o truncado es error: nunca se ignora en silencio.
func LoadManifest(path string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("leer manifest %s: %w", path, err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return Manifest{}, fmt.Errorf("el manifest %s está vacío", path)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsear manifest %s: %w", path, err)
	}
	if m.Version == 0 {
		m.Version = ManifestVersion
	}
	if m.Items == nil {
		m.Items = []Entry{}
	}
	return m, nil
}

// SaveManifest escribe el manifest de forma atómica: serializa a un temporal en
// el mismo directorio, lo sincroniza y recién ahí lo renombra. Un corte a mitad
// de escritura deja el manifest anterior intacto.
func SaveManifest(path string, m Manifest) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("falta la ruta del manifest")
	}
	if m.Version == 0 {
		m.Version = ManifestVersion
	}
	if m.Items == nil {
		m.Items = []Entry{}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar manifest: %w", err)
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("crear %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("crear temporal en %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(b); err != nil {
		cleanup()
		return fmt.Errorf("escribir temporal del manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sincronizar temporal del manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cerrar temporal del manifest: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("permisos del manifest: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renombrar manifest a %s: %w", path, err)
	}
	return nil
}

// Upsert reemplaza el Entry con el mismo ID o lo agrega al final. Es nil-safe:
// un manifest sin puntero no hace nada en vez de romper.
func Upsert(m *Manifest, e Entry) {
	if m == nil {
		return
	}
	for i := range m.Items {
		if m.Items[i].ID == e.ID {
			m.Items[i] = e
			return
		}
	}
	m.Items = append(m.Items, e)
}
