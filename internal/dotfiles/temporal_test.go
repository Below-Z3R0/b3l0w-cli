package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests del temporal de escritura.
//
// `writeFile` escribe primero en "<archivo>.tmp" y después renombra.
// El nombre del temporal es FIJO: no es un archivo único por escritura
// (como el que usa `saveState` con os.CreateTemp). Dos consecuencias:
//
//  1. Si el usuario ya tenía un archivo con ese nombre, el pull lo
//     destruye: os.WriteFile lo trunca y el rename se lo lleva.
//  2. Si dos escritores apuntan al mismo destino, se pisan entre ellos
//     (ver concurrencia_test.go).

// TestBug_ElTemporalFijoDestruyeElArchivoDelUsuario: el usuario tiene un
// "config.toml.tmp" al lado de su config (un temporal suyo, un editor que
// lo dejó, un script). El pull de "config.toml" trunca ese archivo y
// renombra el temporal encima del destino: el archivo del usuario
// desaparece sin aviso.
func TestBug_ElTemporalFijoDestruyeElArchivoDelUsuario(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	// Un archivo del usuario que termina en .tmp, al lado del destino.
	userTmp := filepath.Join(home, ".config", "noctalia", "config.toml.tmp")
	bugEscribir(t, userTmp, "MI-TEMPORAL-DEL-USUARIO")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(userTmp); os.IsNotExist(err) {
		t.Fatalf("el pull destruyó el archivo del usuario %s", userTmp)
	}
	got, err := os.ReadFile(userTmp)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MI-TEMPORAL-DEL-USUARIO" {
		t.Fatalf("el archivo del usuario cambió de contenido: %q", got)
	}
}

// TestBug_ElTemporalFijoDestruyeElNewTmpDelUsuario: mismo problema con
// el temporal del `.new` de un conflicto ("config.toml.new.tmp").
func TestBug_ElTemporalFijoDestruyeElNewTmpDelUsuario(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "mio", repo: "nuevo"},
	})
	userTmp := filepath.Join(home, ".config", "noctalia", "config.toml.new.tmp")
	bugEscribir(t, userTmp, "MI-TEMPORAL-DEL-USUARIO")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(userTmp); os.IsNotExist(err) {
		t.Fatalf("el pull destruyó el archivo del usuario %s", userTmp)
	}
}

// TestBug_ElTemporalFijoDestruyeElBakTmpDelUsuario: y con el temporal
// del backup ("config.toml.bak.tmp").
func TestBug_ElTemporalFijoDestruyeElBakTmpDelUsuario(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "mio", repo: "nuevo"},
	})
	userTmp := filepath.Join(home, ".config", "noctalia", "config.toml.bak.tmp")
	bugEscribir(t, userTmp, "MI-TEMPORAL-DEL-USUARIO")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(userTmp); os.IsNotExist(err) {
		t.Fatalf("el pull destruyó el archivo del usuario %s", userTmp)
	}
}

// TestEdge_NoQuedaTemporalEnElDisco: después de un pull limpio no
// queda ningún ".tmp" tirado en ~/.config.
func TestEdge_NoQuedaTemporalEnElDisco(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
		"noctalia/otro.toml":   {base: "base", local: "mio", repo: "nuevo"},
	})

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}

	var basuras []string
	err := filepath.Walk(home, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if info != nil && !info.IsDir() && filepath.Ext(path) == ".tmp" {
			basuras = append(basuras, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(basuras) != 0 {
		t.Fatalf("quedaron temporales tirados: %v", basuras)
	}
}

// TestEdge_ElTemporalNoTocaElArchivoVecinoDeOtroNombre: un archivo que
// termina en ".tmp" pero NO es el temporal de ningún archivo gestionado
// sobrevive. El pull solo pisa el temporal del archivo que está
// instalando.
func TestEdge_ElTemporalNoTocaElArchivoVecinoDeOtroNombre(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	// Un ".tmp" de otro archivo gestionado: sí es su temporal.
	otro := filepath.Join(home, ".config", "noctalia", "otro.toml.tmp")
	bugEscribir(t, otro, "TEMP-DE-OTRO")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(otro); os.IsNotExist(err) {
		t.Fatalf("el pull destruyó un archivo que no era su temporal: %s", otro)
	}
}
