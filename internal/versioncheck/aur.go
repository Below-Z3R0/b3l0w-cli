// Package versioncheck consulta versiones upstream sin tocar el sistema.
package versioncheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UserAgent identifica al cliente ante el AUR.
//
// Go manda "Go-http-client/1.1" por defecto, que un servicio público
// filtra como bot. El AUR documenta usar un UA identificable.
const UserAgent = "b3l0w-cli (+https://github.com/Below-Z3R0/b3l0w-cli)"

// Package es una versión publicada en AUR.
type Package struct {
	Name    string
	Version string
}

type aurResponse struct {
	// El RPC v5 no usa el código HTTP para los errores: devuelve 200 con
	// {"error":"...","type":"error"}.
	Error   string `json:"error"`
	Type    string `json:"type"`
	Results []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
	} `json:"results"`
}

// aurEndpoint es variable para apuntar a un httptest en los tests.
//
// Es de solo lectura después del arranque: un test la cambia antes de
// correr y no vuelve a tocarla. Concurrentemente no hay escrituras.
var aurEndpoint = "https://aur.archlinux.org/rpc/v5/info?"

// AUR consulta el RPC público de AUR. Sin red devuelve error: el
// llamador decide si es warning o no.
func AUR(names []string) ([]Package, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var args []string
	for _, name := range names {
		args = append(args, "arg[]="+url.QueryEscape(name))
	}

	req, err := http.NewRequest(http.MethodGet, aurEndpoint+strings.Join(args, "&"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AUR: HTTP %d", resp.StatusCode)
	}

	var body aurResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	// Un 200 con type:"error" es un fallo, no una lista vacía.
	if body.Error != "" {
		return nil, fmt.Errorf("AUR: %s", body.Error)
	}

	out := make([]Package, 0, len(body.Results))
	for _, row := range body.Results {
		out = append(out, Package{Name: row.Name, Version: row.Version})
	}
	// Un paquete pedido que no vino no se descarta en silencio: el
	// llamador no puede distinguir "no existe" de "no lo pedí".
	found := map[string]bool{}
	for _, p := range out {
		found[p.Name] = true
	}
	var faltan []string
	for _, name := range names {
		if !found[name] {
			faltan = append(faltan, name)
		}
	}
	if len(faltan) > 0 {
		return out, fmt.Errorf("AUR: no encontrados: %s", strings.Join(faltan, ", "))
	}
	return out, nil
}
