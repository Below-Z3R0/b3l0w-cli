// Package config lee y escribe el manifiesto de programas.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Schema es la versión del formato del manifiesto.
const Schema = 1

// Program es un programa propio que el core sabe consultar, instalar y
// actualizar.
//
// Asset es el nombre del archivo en el release de GitHub: cada repo lo
// publica con el suyo (boot-preview-linux-amd64, kitty-shapes-linux-x64),
// así que no se puede adivinar desde el nombre del programa.
type Program struct {
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	Asset  string `json:"asset,omitempty"`
}

// Manifest es la lista de programas registrados.
type Manifest struct {
	Version  int       `json:"version"`
	Programs []Program `json:"programs"`
}

// Default es el manifiesto que se usa la primera vez.
func Default() *Manifest {
	return &Manifest{
		Version: Schema,
		Programs: []Program{
			{Name: "boot-preview", Source: "github:Below-Z3R0/boot-preview", Asset: "boot-preview-linux-amd64"},
			{Name: "kitty-shapes", Source: "github:Below-Z3R0/kitty-shapes", Asset: "kitty-shapes-linux-x64"},
		},
	}
}

// Load lee el manifiesto. Si no existe devuelve el default: el CLI
// tiene que funcionar recién instalado, sin un paso de configuración.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version == 0 {
		m.Version = Schema
	}
	return &m, nil
}

// Save escribe el manifiesto creando el directorio si hace falta.
func (m *Manifest) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Get devuelve el programa con ese nombre, o nil.
func (m *Manifest) Get(name string) *Program {
	for i := range m.Programs {
		if m.Programs[i].Name == name {
			return &m.Programs[i]
		}
	}
	return nil
}

// Add registra un programa con su fuente. Devuelve false si ya estaba.
func (m *Manifest) Add(name, source string) (bool, error) {
	return m.AddProgram(Program{Name: name, Source: source})
}

// AddProgram registra un programa completo. Devuelve false si ya estaba:
// es idempotente.
//
// Valida el nombre y la fuente al registrar, no al instalar: un programa
// mal cargado falla tres comandos después, cuando el usuario ya se olvidó
// de lo que escribió.
func (m *Manifest) AddProgram(p Program) (bool, error) {
	if err := validarPrograma(p); err != nil {
		return false, err
	}
	if m.Get(p.Name) != nil {
		return false, nil
	}
	m.Programs = append(m.Programs, p)
	return true, nil
}

// validarPrograma comprueba lo mínimo para que el programa se pueda
// instalar: un nombre que sea un componente de ruta y una fuente
// github:owner/repo.
func validarPrograma(p Program) error {
	if p.Name == "" || p.Name == "." || p.Name == ".." || strings.ContainsAny(p.Name, `/\`) {
		return fmt.Errorf("nombre de programa inválido: %q", p.Name)
	}
	if p.Source == "" {
		return nil
	}
	resto, ok := strings.CutPrefix(p.Source, "github:")
	if !ok || strings.Count(resto, "/") != 1 ||
		strings.HasPrefix(resto, "/") || strings.HasSuffix(resto, "/") {
		return fmt.Errorf("fuente no soportada para %s: %q (usá github:owner/repo)", p.Name, p.Source)
	}
	return nil
}

// Remove borra un programa. Devuelve false si no estaba.
func (m *Manifest) Remove(name string) bool {
	for i := range m.Programs {
		if m.Programs[i].Name == name {
			m.Programs = append(m.Programs[:i], m.Programs[i+1:]...)
			return true
		}
	}
	return false
}
