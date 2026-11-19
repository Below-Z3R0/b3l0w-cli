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
	// Un JSON válido pero sin campos (null, {}) no es un manifiesto: es
	// un archivo vacío. Normalizarlo evita un nil que después paniquea.
	if m.Programs == nil {
		m.Programs = []Program{}
	}
	if m.Version == 0 {
		m.Version = Schema
	}
	// Una versión futura trae campos que este binario no entiende:
	// cargarla y volver a guardarla los perdería sin avisar.
	if m.Version > Schema {
		return nil, fmt.Errorf("el manifiesto es de una versión más nueva (%d > %d): actualizá b3l0w", m.Version, Schema)
	}
	return &m, nil
}

// Save escribe el manifiesto creando el directorio si hace falta.
//
// Escribe a un temporal y renombra: un corte deja el manifiesto viejo
// entero, no uno a medias que el próximo Load no puede leer.
func (m *Manifest) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".programs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Get devuelve una copia del programa con ese nombre, o nil.
//
// Una copia, no un puntero dentro del slice: Remove compacta el slice y
// el puntero quedaría apuntando a otro programa.
func (m *Manifest) Get(name string) *Program {
	for i := range m.Programs {
		if m.Programs[i].Name == name {
			p := m.Programs[i]
			return &p
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
			// Limpiar la cola antes de truncar: el array subyacente
			// conservaría una copia del último elemento.
			copy(m.Programs[i:], m.Programs[i+1:])
			m.Programs[len(m.Programs)-1] = Program{}
			m.Programs = m.Programs[:len(m.Programs)-1]
			return true
		}
	}
	return false
}
