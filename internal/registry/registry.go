// Package registry carga las DB de la distro con una sola jerarquía de
// fuentes, igual que pacman:
//
//	caché fresco -> red -> caché viejo -> embebido
//
// El embebido es el último recurso: en una instalación limpia, sin el
// repo clonado y sin red, el CLI igual tiene catálogo.
package registry

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Kind identifica una DB.
type Kind string

const (
	System   Kind = "system"
	Services Kind = "services"
	Plugins  Kind = "plugins"
)

// paths es dónde vive cada DB en el repo de distribución.
//
// No se mueven los archivos: el registry cambia cómo se leen, no la
// estructura del repo.
var paths = map[Kind]string{
	System:   "system.json",
	Services: "services/services.json",
	Plugins:  "plugins/plugins.json",
}

// Inyectables para los tests.
var (
	baseURL  = "https://raw.githubusercontent.com/Below-Z3R0/b3l0w/main"
	stateDir = defaultStateDir
	client   = &http.Client{Timeout: 5 * time.Second}
)

const (
	// TTL es cuánto vale una copia local antes de volver a preguntar.
	TTL = time.Hour
	// maxSize acota la respuesta: ninguna DB pasa de unos KB.
	maxSize = 4 << 20
)

// Source dice de dónde salió la DB.
type Source int

const (
	FromEmbed Source = iota
	FromCache
	FromRemote
	FromStaleCache
)

func (s Source) String() string {
	switch s {
	case FromCache:
		return "caché"
	case FromRemote:
		return "red"
	case FromStaleCache:
		return "caché viejo"
	default:
		return "embebido"
	}
}

// Status describe el estado de una DB.
type Status struct {
	Kind    Kind
	Source  Source
	Updated time.Time
	Size    int
}

// Load devuelve la DB de una categoría.
//
// embedded es el contenido compilado en el binario; se usa solo si no
// hay caché ni red.
func Load(kind Kind, embedded []byte) ([]byte, Source, error) {
	cache := cachePath(kind)

	if data, fresh := readCache(cache); fresh {
		return data, FromCache, nil
	}

	if data, err := fetch(kind); err == nil {
		writeCache(cache, data)
		return data, FromRemote, nil
	}

	// Sin red: sirve la copia vieja, que es mejor que nada.
	if data, err := os.ReadFile(cache); err == nil && len(data) > 0 {
		return data, FromStaleCache, nil
	}

	if len(embedded) > 0 {
		return embedded, FromEmbed, nil
	}
	return nil, FromEmbed, fmt.Errorf("%s: sin caché, sin red y sin copia embebida", kind)
}

// Sync fuerza la descarga e ignora el TTL. Es el "pacman -Sy" del CLI.
func Sync(kind Kind, embedded []byte) (Status, error) {
	cache := cachePath(kind)
	data, err := fetch(kind)
	if err != nil {
		if old, rerr := os.ReadFile(cache); rerr == nil && len(old) > 0 {
			fi, _ := os.Stat(cache)
			return Status{Kind: kind, Source: FromStaleCache, Updated: modTime(fi), Size: len(old)},
				fmt.Errorf("%w (se usa la copia local de %s)", err, modTime(fi).Format("2006-01-02 15:04"))
		}
		if len(embedded) > 0 {
			return Status{Kind: kind, Source: FromEmbed, Size: len(embedded)},
				fmt.Errorf("%w (se usa la copia embebida)", err)
		}
		return Status{Kind: kind}, err
	}
	writeCache(cache, data)
	return Status{Kind: kind, Source: FromRemote, Updated: time.Now(), Size: len(data)}, nil
}

// Stat describe la copia local sin tocar la red.
func Stat(kind Kind) Status {
	path := cachePath(kind)
	fi, err := os.Stat(path)
	if err != nil {
		return Status{Kind: kind, Source: FromEmbed}
	}
	return Status{Kind: kind, Source: FromCache, Updated: fi.ModTime(), Size: int(fi.Size())}
}

// All son todas las DB del registry.
func All() []Kind { return []Kind{System, Services, Plugins} }

func fetch(kind Kind) ([]byte, error) {
	path, ok := paths[kind]
	if !ok {
		return nil, fmt.Errorf("categoría desconocida: %s", kind)
	}
	url := fmt.Sprintf("%s/%s", baseURL, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "b3l0w")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", kind, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: respuesta vacía", kind)
	}
	return data, nil
}

func defaultStateDir() string {
	// Go no tiene UserStateDir: XDG dice que el estado va en
	// XDG_STATE_HOME o ~/.local/state.
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "b3l0w", "registry")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "b3l0w", "registry")
	}
	return filepath.Join(home, ".local", "state", "b3l0w", "registry")
}

// CachePath es dónde vive la copia local de una DB. Lo usan los
// paquetes que actualizan la DB además de leerla.
func CachePath(kind Kind) string { return cachePath(kind) }

func cachePath(kind Kind) string {
	return filepath.Join(stateDir(), string(kind)+".json")
}

func readCache(path string) ([]byte, bool) {
	fi, err := os.Stat(path)
	if err != nil || time.Since(fi.ModTime()) > TTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func writeCache(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func modTime(fi os.FileInfo) time.Time {
	if fi == nil {
		return time.Time{}
	}
	return fi.ModTime()
}
