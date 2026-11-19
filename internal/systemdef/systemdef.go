// Package systemdef lee la definición del sistema: los perfiles que el
// instalador y `b3l0w install system` aplican.
//
// El archivo va embebido en el binario (go:embed): así el CLI sabe qué
// es "su" sistema sin depender de un archivo al lado. `--file` permite
// usar otra definición.
package systemdef

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

//go:embed system.json
var builtin []byte

// File es un dotfile que se copia al home del usuario.
type File struct {
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

// Profile es un conjunto de cosas que se instalan juntas.
//
// Las cuatro fuentes están separadas a propósito: un paquete de pacman,
// uno de AUR, una app de Flatpak y un binario propio no se instalan
// igual, y mezclarlos obliga a adivinar después.
type Profile struct {
	Repo     []string `json:"repo,omitempty"`
	AUR      []string `json:"aur,omitempty"`
	Flatpak  []string `json:"flatpak,omitempty"`
	Own      []string `json:"own,omitempty"`
	Services []string `json:"services,omitempty"`
	Groups   []string `json:"groups,omitempty"`
	Files    []File   `json:"files,omitempty"`
}

// Definition es el archivo completo.
type Definition struct {
	Version  int                `json:"version"`
	Profiles map[string]Profile `json:"profiles"`
}

// Builtin devuelve la definición embebida.
func Builtin() (*Definition, error) {
	return parse(builtin)
}

// Load lee una definición de un archivo.
func Load(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(data)
}

func parse(data []byte) (*Definition, error) {
	var d Definition
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if d.Version == 0 {
		return nil, fmt.Errorf("definición sin versión")
	}
	if len(d.Profiles) == 0 {
		return nil, fmt.Errorf("la definición no tiene perfiles")
	}
	return &d, nil
}

// Names devuelve los perfiles disponibles, ordenados.
func (d *Definition) Names() []string {
	names := make([]string, 0, len(d.Profiles))
	for name := range d.Profiles {
		names = append(names, name)
	}
	// El recorrido de un mapa no es determinista: sin ordenar, dos
	// ejecuciones del mismo comando listan los perfiles distinto.
	sort.Strings(names)
	return names
}

// Merge junta los perfiles pedidos en uno solo, sin duplicados.
func (d *Definition) Merge(names []string) (Profile, error) {
	if len(names) == 0 {
		return Profile{}, fmt.Errorf("falta el perfil")
	}

	var out Profile

	// Un "visto" por categoría: el mismo nombre puede ser paquete en un
	// perfil y binario propio en otro, y no son lo mismo.
	repo := map[string]bool{}
	aur := map[string]bool{}
	flatpak := map[string]bool{}
	own := map[string]bool{}
	services := map[string]bool{}
	groups := map[string]bool{}

	add := func(dst *[]string, items []string, seen map[string]bool) {
		for _, item := range items {
			if seen[item] {
				continue
			}
			seen[item] = true
			*dst = append(*dst, item)
		}
	}

	for _, name := range names {
		profile, ok := d.Profiles[name]
		if !ok {
			return out, fmt.Errorf("perfil desconocido: %s", name)
		}
		add(&out.Repo, profile.Repo, repo)
		add(&out.AUR, profile.AUR, aur)
		add(&out.Flatpak, profile.Flatpak, flatpak)
		add(&out.Own, profile.Own, own)
		add(&out.Services, profile.Services, services)
		add(&out.Groups, profile.Groups, groups)
		out.Files = append(out.Files, profile.Files...)
	}
	return out, nil
}
