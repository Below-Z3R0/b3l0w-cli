package systemdef

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// remoteURL es variable para poder apuntar a un httptest en los tests.
var remoteURL = "https://raw.githubusercontent.com/Below-Z3R0/b3l0w/main/system.json"

const (
	remoteTimeout = 5 * time.Second
	cacheTTL      = time.Hour
	maxRemoteSize = 1 << 20 // 1 MiB
)

// LoadRemote lee la definición del repo de distribución.
//
// Orden: cache fresco -> red -> cache viejo -> Builtin(). Así el CLI
// sigue funcionando sin red y no depende de un release para cambiar
// un perfil.
func LoadRemote() (*Definition, error) {
	cache := cachePath()

	if data, fresh := readCache(cache); fresh {
		if d, err := parse(data); err == nil {
			return d, nil
		}
	}

	client := &http.Client{Timeout: remoteTimeout}
	if resp, err := client.Get(remoteURL); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if data, rerr := io.ReadAll(io.LimitReader(resp.Body, maxRemoteSize)); rerr == nil {
				if d, perr := parse(data); perr == nil {
					writeCache(cache, data)
					return d, nil
				}
			}
		}
	}

	// Sin red o respuesta inválida: cache viejo, después el embed.
	if data, err := os.ReadFile(cache); err == nil {
		if d, perr := parse(data); perr == nil {
			return d, nil
		}
	}
	return Builtin()
}

func cachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "b3l0w", "system.json")
}

func readCache(path string) ([]byte, bool) {
	fi, err := os.Stat(path)
	if err != nil || time.Since(fi.ModTime()) > cacheTTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil
}

func writeCache(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}
