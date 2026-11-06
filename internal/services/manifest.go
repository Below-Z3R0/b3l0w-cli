// Package services administra los servicios Docker de la distro.
//
// Un servicio es un stack de contenedores con su propio repo: el
// manifiesto de la distro dice qué existe, qué variantes hay y cómo
// se llama la imagen; el repo del servicio tiene el compose y el
// Dockerfile. El CLI consume las imágenes publicadas: no compila.
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Service es un servicio del manifiesto de distribución.
type Service struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Compose string `json:"compose,omitempty"`
	Image   string `json:"image,omitempty"`
	// Variants son las variantes de imagen que el servicio define.
	Variants []string `json:"variants,omitempty"`
	// Available son las que están publicadas en el registry. Se
	// recalcula contra el registry real; el manifiesto es una caché.
	Available []string `json:"available,omitempty"`
	// GPU marca servicios que pueden usar aceleración por hardware.
	GPU bool `json:"gpu,omitempty"`
	// EP es el execution provider por variante, para reportar.
	EP map[string]string `json:"ep,omitempty"`
	// PortFolds son los puertos que publica el stack.
	Ports []int `json:"ports,omitempty"`
	// Health es la URL que responde cuando el servicio está listo.
	Health string `json:"health,omitempty"`
	// State: ready, wip o planned.
	State string `json:"state"`
	// Models es el comando que instala los modelos, si aplica.
	Models string `json:"models,omitempty"`
	// Profiles son los perfiles de compose opcionales.
	Profiles []string `json:"profiles,omitempty"`
	// SizeGB es el disco que ocupa la imagen.
	SizeGB float64 `json:"size_gb,omitempty"`
}

// Manifest es services/services.json del repo de distribución.
type Manifest struct {
	Version  int       `json:"version"`
	Services []Service `json:"services"`
}

// LoadManifest lee el manifiesto.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version == 0 || len(m.Services) == 0 {
		return nil, fmt.Errorf("manifiesto de servicios vacío o sin versión")
	}
	return &m, nil
}

// SaveManifest lo escribe ordenado por id.
func SaveManifest(path string, m *Manifest) error {
	sort.Slice(m.Services, func(i, j int) bool { return m.Services[i].ID < m.Services[j].ID })
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Find devuelve el servicio por id.
func (m *Manifest) Find(id string) *Service {
	for i := range m.Services {
		if m.Services[i].ID == id {
			return &m.Services[i]
		}
	}
	return nil
}

// DefaultManifestPath es services/services.json del repo de distro.
func DefaultManifestPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "Github", "b3l0w", "services", "services.json")
}

// DefaultStateDir es donde vive el estado de cada servicio.
func DefaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "b3l0w", "services")
}

// State es el estado auditable de un servicio instalado.
type State struct {
	ID          string    `json:"id"`
	Variant     string    `json:"variant,omitempty"`
	Image       string    `json:"image,omitempty"`
	Digest      string    `json:"digest,omitempty"`
	Tag         string    `json:"tag,omitempty"`
	InstalledAt string    `json:"installed_at,omitempty"`
	UpdatedAt   string    `json:"updated_at,omitempty"`
	Health      string    `json:"health,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	Ports       []int     `json:"ports,omitempty"`
	Previous    string    `json:"previous_image,omitempty"`
	PreviousTag string    `json:"previous_tag,omitempty"`
	Services    []string  `json:"compose_services,omitempty"`
	Warnings    []string  `json:"warnings,omitempty"`
	Updated     time.Time `json:"-"`
}

// Save escribe el estado de un servicio.
func (s *State) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.ID+".json"), append(data, '\n'), 0o644)
}

// LoadState lee el estado de un servicio.
func LoadState(dir, id string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ListStates lee todos los estados guardados.
func ListStates(dir string) ([]*State, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*State
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		s, err := LoadState(dir, e.Name()[:len(e.Name())-len(".json")])
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
