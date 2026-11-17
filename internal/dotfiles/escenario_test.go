package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// casoArchivo describe un archivo del repo y su estado en el home.
type casoArchivo struct {
	// base: contenido registrado como base en el estado. Vacío
	// significa "sin entrada": nunca se aplicó ese archivo.
	base string
	// local: contenido en ~/.config.
	local string
	// repo: contenido en el repo.
	repo string
	// sinLocal: el archivo no existe en el home.
	sinLocal bool
}

// escenarioMulti arma repo, home y estado con varios archivos a la vez.
// Las rutas que se pasan en soloEstado quedan en el estado pero no en el
// repo: son los huérfanos que detecta Pull.
//
// Todos los archivos se cuelgan de ~/.config/<rel>, igual que en Pull.
func escenarioMulti(t *testing.T, casos map[string]casoArchivo, soloEstado ...string) (src, home, statePath string) {
	t.Helper()
	root := t.TempDir()
	src = filepath.Join(root, "repo")
	home = filepath.Join(root, "home")
	statePath = filepath.Join(root, "estado.json")

	st := state{}
	for rel, c := range casos {
		repoPath := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(repoPath, []byte(c.repo), 0o644); err != nil {
			t.Fatal(err)
		}
		if !c.sinLocal {
			localPath := filepath.Join(home, ".config", rel)
			if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(localPath, []byte(c.local), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if c.base != "" {
			st[rel] = hash([]byte(c.base))
		}
	}
	// Los huérfanos: en el estado, ausentes del repo.
	for _, rel := range soloEstado {
		st[rel] = hash([]byte("base"))
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return src, home, statePath
}

// leerEstado devuelve el estado guardado en disco.
func leerEstado(t *testing.T, path string) state {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer estado %s: %v", path, err)
	}
	st := state{}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("estado inválido: %v", err)
	}
	return st
}

// contiene dice si xs tiene a want.
func contiene(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// mismoConjunto compara sin importar el orden.
func mismoConjunto(xs []string, want ...string) bool {
	if len(xs) != len(want) {
		return false
	}
	for _, w := range want {
		if !contiene(xs, w) {
			return false
		}
	}
	return true
}
