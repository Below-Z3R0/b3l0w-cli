package selfupdate

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/version"
)

// conInyectables sustituye las dependencias del paquete y las restaura
// al terminar el test.
func conInyectables(t *testing.T, fns map[string]any) {
	t.Helper()
	oldAPI, oldFetch, oldReplace := apiURL, fetchAsset, replaceFile
	oldOwner, oldCurrent, oldExe := packageOwner, currentVersion, resolveExe
	oldVerify := verifyRelease
	oldVersion := version.Version

	t.Cleanup(func() {
		apiURL, fetchAsset, replaceFile = oldAPI, oldFetch, oldReplace
		packageOwner, currentVersion, resolveExe = oldOwner, oldCurrent, oldExe
		verifyRelease = oldVerify
		version.Version = oldVersion
	})

	// Sin firma explícita, los tests no salen a la red.
	verifyRelease = func(string, string, string) error { return nil }

	for nombre, fn := range fns {
		switch nombre {
		case "apiURL":
			apiURL = fn.(string)
		case "fetch":
			fetchAsset = fn.(func(string) (string, error))
		case "replace":
			replaceFile = fn.(func(string, string) error)
		case "owner":
			packageOwner = fn.(func(string) string)
		case "current":
			currentVersion = fn.(func(string) (string, error))
		case "exe":
			resolveExe = fn.(func() (string, error))
		case "verify":
			verifyRelease = fn.(func(string, string, string) error)
		case "version":
			version.Version = fn.(string)
		default:
			t.Fatalf("inyectable desconocido: %s", nombre)
		}
	}
}

func TestWithV(t *testing.T) {
	casos := map[string]string{
		"0.5.0":  "v0.5.0",
		"v0.5.0": "v0.5.0",
		"":       "v",
	}
	for entrada, quiere := range casos {
		if got := withV(entrada); got != quiere {
			t.Errorf("withV(%q) = %q, quería %q", entrada, got, quiere)
		}
	}
}

func TestCheckLeeElTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "b3l0w" {
			t.Errorf("User-Agent = %q, quería b3l0w", got)
		}
		w.Write([]byte(`[{"tag_name":"desktop-2026.11.06-r1","draft":false,"prerelease":false},{"tag_name":"v0.6.0","draft":false,"prerelease":false}]`))
	}))
	defer srv.Close()
	conInyectables(t, map[string]any{"apiURL": srv.URL})

	tag, err := Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if tag != "v0.6.0" {
		t.Errorf("tag = %q, quería v0.6.0", tag)
	}
}

func TestCheckFallaConHTTPNoOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	conInyectables(t, map[string]any{"apiURL": srv.URL})

	if _, err := Check(); err == nil {
		t.Fatal("esperaba error con HTTP 500")
	} else if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, quería que mencione HTTP 500", err)
	}
}

func TestCheckFallaConJSONInvalido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{no es json`))
	}))
	defer srv.Close()
	conInyectables(t, map[string]any{"apiURL": srv.URL})

	if _, err := Check(); err == nil {
		t.Fatal("esperaba error con JSON inválido")
	}
}

func TestRunRechazaBinarioDePaquete(t *testing.T) {
	conInyectables(t, map[string]any{
		"exe":   func() (string, error) { return "/usr/bin/b3l0w", nil },
		"owner": func(string) string { return "b3l0w-cli" },
		"fetch": func(string) (string, error) {
			t.Fatal("no debería descargar si el binario es de pacman")
			return "", nil
		},
	})

	_, err := Run()
	if err == nil {
		t.Fatal("esperaba error")
	}
	if !strings.Contains(err.Error(), "pacman") {
		t.Errorf("error = %q, quería que mencione pacman", err)
	}
}

func TestRunFallaSiElBinarioNoResponde(t *testing.T) {
	conInyectables(t, map[string]any{
		"exe":     func() (string, error) { return "/tmp/b3l0w", nil },
		"owner":   func(string) string { return "" },
		"fetch":   func(string) (string, error) { return "/tmp/b3l0w-nuevo", nil },
		"current": func(string) (string, error) { return "", errors.New("no responde") },
	})

	_, err := Run()
	if err == nil {
		t.Fatal("esperaba error")
	}
	if !strings.Contains(err.Error(), "no responde") {
		t.Errorf("error = %q, quería que mencione el binario", err)
	}
}

func TestRunNoCambiaSiLaVersionEsIgual(t *testing.T) {
	reemplazos := 0
	conInyectables(t, map[string]any{
		"version": "0.5.0",
		"exe":     func() (string, error) { return "/tmp/b3l0w", nil },
		"owner":   func(string) string { return "" },
		"fetch":   func(string) (string, error) { return "/tmp/b3l0w-nuevo", nil },
		"current": func(string) (string, error) { return "v0.5.0", nil },
		"replace": func(string, string) error {
			reemplazos++
			return nil
		},
	})

	result, err := Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, quería false")
	}
	if result.Before != "v0.5.0" || result.After != "v0.5.0" {
		t.Errorf("Before/After = %q/%q, quería v0.5.0/v0.5.0", result.Before, result.After)
	}
	if reemplazos != 0 {
		t.Errorf("reemplazos = %d, quería 0", reemplazos)
	}
}

func TestRunReemplazaSiLaVersionEsDistinta(t *testing.T) {
	var origen, destino string
	conInyectables(t, map[string]any{
		"version": "0.5.0",
		"exe":     func() (string, error) { return "/tmp/b3l0w", nil },
		"owner":   func(string) string { return "" },
		"fetch":   func(string) (string, error) { return "/tmp/b3l0w-nuevo", nil },
		"current": func(string) (string, error) { return "v0.6.0", nil },
		"replace": func(src, dst string) error {
			origen, destino = src, dst
			return nil
		},
	})

	result, err := Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Changed {
		t.Error("Changed = false, quería true")
	}
	if result.After != "v0.6.0" {
		t.Errorf("After = %q, quería v0.6.0", result.After)
	}
	if origen != "/tmp/b3l0w-nuevo" || destino != "/tmp/b3l0w" {
		t.Errorf("replace(%q, %q), quería (/tmp/b3l0w-nuevo, /tmp/b3l0w)", origen, destino)
	}
}

func TestRunNoReemplazaSiLaFirmaFalla(t *testing.T) {
	reemplazos := 0
	conInyectables(t, map[string]any{
		"version": "0.5.0",
		"exe":     func() (string, error) { return "/tmp/b3l0w", nil },
		"owner":   func(string) string { return "" },
		"fetch":   func(string) (string, error) { return "/tmp/b3l0w-nuevo", nil },
		"current": func(string) (string, error) { return "v0.6.0", nil },
		"verify": func(string, string, string) error {
			return errors.New("firma inválida")
		},
		"replace": func(string, string) error {
			reemplazos++
			return nil
		},
	})

	_, err := Run()
	if err == nil {
		t.Fatal("esperaba error de firma")
	}
	if reemplazos != 0 {
		t.Errorf("reemplazos = %d, quería 0: no se toca el binario sin firma válida", reemplazos)
	}
}

func TestRunPropagaElErrorDeReemplazo(t *testing.T) {
	conInyectables(t, map[string]any{
		"version": "0.5.0",
		"exe":     func() (string, error) { return "/tmp/b3l0w", nil },
		"owner":   func(string) string { return "" },
		"fetch":   func(string) (string, error) { return "/tmp/b3l0w-nuevo", nil },
		"current": func(string) (string, error) { return "v0.6.0", nil },
		"replace": func(string, string) error { return errors.New("permiso denegado") },
	})

	_, err := Run()
	if err == nil {
		t.Fatal("esperaba error")
	}
	if !strings.Contains(err.Error(), "permiso denegado") {
		t.Errorf("error = %q, quería que mencione el reemplazo", err)
	}
}
