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
	"strconv"
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
	api   = "https://api.github.com/repos/" + repo + "/releases?per_page=30"
)

// Inyectables. En producción son los reales; los tests los sustituyen
// para no depender de la red ni del binario en ejecución.
var (
	apiURL         = api
	fetchAsset     = fetch.ToTemp
	replaceFile    = cli.ReplaceFile
	packageOwner   = cli.PackageOwner
	currentVersion = programs.Current
	verifyRelease  = programs.VerifyRelease
	checkRelease   = Check
	resolveExe     = func() (string, error) {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(exe)
	}
)

// Check devuelve el tag del último release del CLI.
//
// No usa /releases/latest a propósito: el repo también publica los
// paquetes del escritorio, y ese release queda como "latest" aunque no
// tenga el binario. Se busca el release más nuevo cuyo tag empiece con
// "v", que es la convención del CLI.
func Check() (string, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
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
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}
	for _, release := range releases {
		if release.Draft || release.Prerelease {
			continue
		}
		if strings.HasPrefix(release.TagName, "v") {
			return release.TagName, nil
		}
	}
	return "", fmt.Errorf("no hay releases del CLI con tag v* en %s", repo)
}

// Result cuenta qué pasó.
type Result struct {
	Before  string
	After   string
	Changed bool
	// Note explica por qué no se cambió nada cuando no fue por estar
	// al día (por ejemplo, un release más viejo que el binario).
	Note string
}

// compareVersions compara dos versiones numéricamente: -1, 0 o 1.
//
// Ignora los sufijos (-dev, +build): "v0.5.0-dev" y "v0.5.0" son la
// misma versión a los efectos de actualizar. Un release más viejo no
// debe degradar un binario de desarrollo.
func compareVersions(a, b string) int {
	na, nb := numeros(a), numeros(b)
	for i := 0; i < 3; i++ {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func numeros(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, parte := range strings.Split(v, ".") {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(parte)
	}
	return out
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

	exe, err := resolveExe()
	if err != nil {
		return result, err
	}

	if owner := packageOwner(exe); owner != "" {
		return result, fmt.Errorf("el binario es del paquete %s: se actualiza con pacman", owner)
	}

	result.Before = withV(version.Version)

	tag, err := checkRelease()
	if err != nil {
		return result, err
	}

	// Comparar el tag antes de descargar: si el release publicado es
	// más viejo (o el mismo), no hay nada que bajar.
	if cmp := compareVersions(tag, result.Before); cmp <= 0 {
		result.After = withV(tag)
		if cmp < 0 {
			result.Note = fmt.Sprintf("el release (%s) es más viejo que el binario (%s): no se toca",
				result.After, result.Before)
		}
		return result, nil
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, asset)
	tmp, err := fetchAsset(url)
	if err != nil {
		return result, err
	}
	defer os.Remove(tmp)

	if err := verifyRelease(url, asset, tmp); err != nil {
		return result, err
	}

	after, err := currentVersion(tmp)
	if err != nil {
		return result, fmt.Errorf("el binario bajado no responde: %w", err)
	}
	result.After = after

	switch compareVersions(after, result.Before) {
	case 0:
		return result, nil
	case -1:
		result.Note = fmt.Sprintf("el release (%s) es más viejo que el binario (%s): no se toca",
			after, result.Before)
		return result, nil
	}
	if err := replaceFile(tmp, exe); err != nil {
		return result, fmt.Errorf("no se pudo reemplazar %s: %w", exe, err)
	}
	result.Changed = true
	return result, nil
}
