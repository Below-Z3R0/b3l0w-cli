// Package selfupdate actualiza el propio binario de b3l0w.
//
// Es el caso especial del contrato: el core no puede actualizarse con
// `b3l0w update` sin bajarse a sí mismo. Si el binario pertenece a un
// paquete, no se toca: eso lo hace pacman.
package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
	"github.com/Below-Z3R0/b3l0w-cli/internal/fetch"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
)

const (
	repo  = "Below-Z3R0/b3l0w-cli"
	asset = "b3l0w-linux-amd64"
	api   = "https://api.github.com/repos/" + repo + "/releases/latest"
)

// Check devuelve el tag del último release, o "" si no hay red.
func Check() (string, error) {
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "b3l0w")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.TagName, nil
}

// Result cuenta qué pasó.
type Result struct {
	Before  string
	After   string
	Changed bool
}

// withV deja la versión con una sola "v": el ldflags puede venir con
// ella (v0.2.0) o sin ella (0.2.0).
func withV(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// Run baja el último release y reemplaza el binario en ejecución.
func Run() (Result, error) {
	var result Result

	exe, err := os.Executable()
	if err != nil {
		return result, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return result, err
	}

	if owner := cli.PackageOwner(exe); owner != "" {
		return result, fmt.Errorf("el binario es del paquete %s: se actualiza con pacman", owner)
	}

	result.Before = withV(version.Version)

	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset)
	tmp, err := fetch.ToTemp(url)
	if err != nil {
		return result, err
	}
	defer os.Remove(tmp)

	after, err := programs.Current(tmp)
	if err != nil {
		return result, fmt.Errorf("el binario bajado no responde: %w", err)
	}
	result.After = after

	if after == result.Before {
		return result, nil
	}
	if err := cli.ReplaceFile(tmp, exe); err != nil {
		return result, fmt.Errorf("no se pudo reemplazar %s: %w", exe, err)
	}
	result.Changed = true
	return result, nil
}
