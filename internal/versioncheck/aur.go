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

// Package es una versión publicada en AUR.
type Package struct {
	Name    string
	Version string
}

type aurResponse struct {
	Results []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
	} `json:"results"`
}

// aurEndpoint es variable para apuntar a un httptest en los tests.
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
	endpoint := aurEndpoint + strings.Join(args, "&")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(endpoint)
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
	out := make([]Package, 0, len(body.Results))
	for _, row := range body.Results {
		out = append(out, Package{Name: row.Name, Version: row.Version})
	}
	return out, nil
}
