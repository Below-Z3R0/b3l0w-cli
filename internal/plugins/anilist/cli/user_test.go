package anilistcli

// Tests del hallazgo B4: el flag `--user` era un int en library/sync y un
// string en stats, así que el mismo flag significaba dos cosas según el
// subcomando.
//
//   - `library --user b3low` → "invalid value ... parse error", salida 2.
//   - `stats --user 123` → lo tomaba como nombre de usuario y buscaba el
//     usuario literal "123".

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestElFlagUserAceptaUnNombreEnLibrary comprueba que `library --user b3low`
// no es un error de parseo. El flagset usa ExitOnError: un flag inválido
// mata el proceso, así que el comando corre en un subproceso.
func TestElFlagUserAceptaUnNombreEnLibrary(t *testing.T) {
	sinTokenDeEntorno(t)
	dir := t.TempDir()

	codigo, _, errores := ejecutarEnSubproceso(t, "library", "--json", "--data-dir", dir, "--user", "b3low")
	if strings.Contains(errores, "invalid value") {
		t.Fatalf("--user no acepta un nombre de usuario: %s", errores)
	}
	// Sin token el comando falla al pedirlo, pero no al parsear: el código 2
	// es el del error de parseo.
	if codigo != 1 {
		t.Errorf("código = %d, quería 1 (fallo por falta de token, no por parseo)", codigo)
	}
}

// TestElFlagUserAceptaUnIdEnStats comprueba que `stats --user 123` resuelve
// el id numérico contra la API en vez de buscar el usuario literal "123".
func TestElFlagUserAceptaUnIdEnStats(t *testing.T) {
	falso := instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	falso.responder = func(query string, variables map[string]any) (string, int) {
		switch {
		case strings.Contains(query, "User(id: $id)"):
			if variables["id"] != float64(123) {
				t.Errorf("id = %v; quería 123", variables["id"])
			}
			return `{"data":{"User":{"id":123,"name":"b3low"}}}`, 0
		case strings.Contains(query, "statistics {"):
			// La API real responde 404 para un usuario que no existe.
			if variables["name"] == "123" {
				return `{"errors":[{"message":"Not Found.","status":404}],"data":{"User":null}}`, 404
			}
			return cuerpoStats, 0
		case strings.Contains(query, "MediaListCollection(userName:"):
			return `{"data":{"MediaListCollection":{"hasNextChunk":false,"lists":[]}}}`, 0
		case strings.Contains(query, "Viewer { id name }"):
			return cuerpoViewer, 0
		}
		return `{"errors":[{"message":"consulta no simulada"}]}`, 0
	}

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"stats", "--json", "--data-dir", dir, "--user", "123"})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["user"] != "b3low" {
		t.Errorf("user = %v; quería b3low: el id 123 tiene que resolverse a su nombre (B4)", doc["user"])
	}
}

// TestElFlagUserEsElMismoEnTodosLosSubcomandos comprueba que los subcomandos
// que comparten el flagset lo declaran igual: un string que acepta el id
// numérico o el nombre.
func TestElFlagUserEsElMismoEnTodosLosSubcomandos(t *testing.T) {
	fs, _, _, _, user := newFlags("library")
	if user == nil {
		t.Fatal("newFlags tiene que devolver el flag --user")
	}
	if err := fs.Parse([]string{"--user", "b3low"}); err != nil {
		t.Fatalf("--user tiene que aceptar un nombre: %v", err)
	}
	// Con `%v` el test compila con el flag viejo (int) también: si el flag
	// vuelve a ser un int, el parseo falla arriba y el fallo dice por qué.
	if valor := fmt.Sprintf("%v", *user); valor != "b3low" {
		t.Errorf("--user = %q; quería %q", valor, "b3low")
	}
}

// TestElFlagUserCeroEsElDelToken comprueba que `--user 0` conserva el
// significado del flag viejo (0 = el usuario del token) y no busca un
// usuario literal "0".
func TestElFlagUserCeroEsElDelToken(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)
	c, _, err := client("token-de-prueba", dir)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	id, nombre, err := resolverUsuarioID(context.Background(), c, "0")
	if err != nil {
		t.Fatalf("resolverUsuarioID: %v", err)
	}
	if id != 0 || nombre != "" {
		t.Errorf("resolverUsuarioID(\"0\") = (%d, %q); quería (0, \"\")", id, nombre)
	}
	nombre, err = resolverUsuarioNombre(context.Background(), c, "0")
	if err != nil {
		t.Fatalf("resolverUsuarioNombre: %v", err)
	}
	if nombre != "" {
		t.Errorf("resolverUsuarioNombre(\"0\") = %q; quería \"\"", nombre)
	}
}

// TestElArchivoDeBibliotecaNoPideToken comprueba que `covers --library` corre
// sin token: es la excepción que el README tiene que documentar (B5).
func TestElArchivoDeBibliotecaNoPideToken(t *testing.T) {
	instalarAniListFalso(t)
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	lib := filepath.Join(dir, "library.json")
	raw, err := json.Marshal(map[string]any{
		"viewer": map[string]any{"id": 1, "name": "b3low"},
		"anime":  []any{},
		"manga":  []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"covers", "--json", "--data-dir", dir, "--library", lib, "--covers-dir", filepath.Join(dir, "covers")})
	})
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errores)
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["dir"] == nil {
		t.Errorf("la salida no trae el directorio de carátulas: %v", doc)
	}
}
