package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Fuzzing del módulo internal/dotfiles.
//
// Dos objetivos:
//
//	FuzzParseEstado  — el parser del estado (loadState/saveState): nunca
//	                   debe panicar, nunca debe devolver un map nil, y
//	                   un round-trip tiene que conservar las claves.
//
//	FuzzPullConEstado — Pull con un estado arbitrario y archivos del repo
//	                   arbitrarios: nunca debe panicar, nunca debe
//	                   escribir fuera del home, y el estado que queda en
//	                   disco siempre tiene que ser JSON válido.
//
// Los seeds viven en testdata/fuzz/<FuzzX>/. El corpus de fallos que
// genera el fuzzer se guarda en el directorio del fuzzing;

// ---------------------------------------------------------------------------
// FuzzParseEstado
// ---------------------------------------------------------------------------

func FuzzParseEstado(f *testing.F) {
	// Seeds: los casos que un usuario o una herramienta externa pueden
	// dejar en el archivo de estado.
	seeds := []string{
		`{}`,
		`null`,
		`{"noctalia/config.toml":"abc123"}`,
		`{"a":"b","c":"d"}`,
		`{"a":null}`,
		`[]`,
		`[1,2,3]`,
		`"string"`,
		`123`,
		`true`,
		``,
		`{`,
		`}`,
		`{"a":`,
		`{"":""}`,
		`{"../../etc/passwd":"x"}`,
		`{"a":"b","a":"c"}`,
		`{"ñ":"á"}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "estado.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("escribir estado: %v", err)
		}

		st, err := loadState(path)
		if err != nil {
			// Un estado inválido es un error, no un panic: eso está bien.
			return
		}
		// Invariante 1: un estado cargado sin error nunca es nil.
		// Un map nil revienta en la primera asignación.
		if st == nil {
			t.Fatalf("loadState devolvió un map nil sin error (input: %q)", data)
		}
		// Invariante 2: el map se puede usar.
		st["__fuzz__"] = "x"
		if st["__fuzz__"] != "x" {
			t.Fatalf("el map no es usable (input: %q)", data)
		}

		// Invariante 3: round-trip. Guardar y volver a cargar tiene que
		// dar el mismo mapa.
		out := filepath.Join(dir, "out.json")
		if err := saveState(out, st); err != nil {
			t.Fatalf("saveState: %v", err)
		}
		st2, err := loadState(out)
		if err != nil {
			t.Fatalf("loadState del round-trip: %v (input: %q)", err, data)
		}
		if len(st2) != len(st) {
			t.Fatalf("el round-trip cambió el tamaño: %d vs %d (input: %q)",
				len(st2), len(st), data)
		}
		for k, v := range st {
			if st2[k] != v {
				t.Fatalf("el round-trip cambió %q: %q vs %q (input: %q)",
					k, v, st2[k], data)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// FuzzPullConEstado
// ---------------------------------------------------------------------------

func FuzzPullConEstado(f *testing.F) {
	// Semilla: (estado, contenido del repo, contenido del local).
	f.Add([]byte(`{"noctalia/config.toml":"0000"}`), []byte("repo"), []byte("local"))
	f.Add([]byte(`null`), []byte("repo"), []byte("local"))
	f.Add([]byte(`{}`), []byte(""), []byte(""))
	f.Add([]byte(`{"noctalia/config.toml":""}`), []byte("repo"), []byte("repo"))

	f.Fuzz(func(t *testing.T, estado, repoBytes, localBytes []byte) {
		dir := t.TempDir()
		src := filepath.Join(dir, "repo")
		home := filepath.Join(dir, "home")
		statePath := filepath.Join(dir, "estado.json")

		repoFile := filepath.Join(src, "noctalia", "config.toml")
		if err := os.MkdirAll(filepath.Dir(repoFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(repoFile, repoBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		localFile := filepath.Join(home, ".config", "noctalia", "config.toml")
		if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(localFile, localBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(statePath, estado, 0o644); err != nil {
			t.Fatal(err)
		}

		// Pull en dry-run: no puede escribir nada, pero tampoco puede
		// panicar.
		if _, err := Pull(src, home, statePath, false, true); err != nil {
			// Un estado inválido da error: aceptable.
			return
		}

		// Pull real: no puede panicar.
		if _, err := Pull(src, home, statePath, false, false); err != nil {
			return
		}

		// Invariante: el estado en disco tiene que seguir siendo JSON
		// válido después de cualquier corrida.
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("leer estado: %v", err)
		}
		var st map[string]string
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatalf("el estado quedó corrupto: %v (estado=%q)", err, estado)
		}

		// Invariante: nada se escribe fuera del home.
		entradas, err := os.ReadDir(home)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entradas {
			if e.Name() != ".config" {
				t.Fatalf("se creó %q en el home", e.Name())
			}
		}
	})
}
