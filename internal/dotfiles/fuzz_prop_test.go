package dotfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Fuzzing por propiedades del módulo internal/dotfiles.
//
// Los objetivos de fuzz_test.go (FuzzParseEstado, FuzzPullConEstado)
// verifican que no haya panic y que el estado quede parseable. Estos
// verifican la PROPIEDAD central del módulo: la decisión 3-way.
//
//	FuzzPullRespetaElLocal — lo que el Result dice es lo que pasó en el
//	                       disco: un archivo reportado como Kept o
//	                       Conflicts no cambió un byte; uno reportado
//	                       como Updated o Missing tiene EXACTAMENTE el
//	                       contenido del repo.
//
//	FuzzAdoptNoTocaElHome — Adopt solo lee: no modifica ni crea ningún
//	                       archivo del home, y la base que registra es
//	                       el hash del contenido local.

// FuzzPullRespetaElLocal: el Result es la verdad.
func FuzzPullRespetaElLocal(f *testing.F) {
	rel := "noctalia/config.toml"

	// Semillas: una por rama del switch. Los estados llevan el hash real
	// de "repo" y de "local" para que el fuzzer pueda mutar desde las
	// bases válidas.
	estadoRepo := fmt.Sprintf(`{%q:%q}`, rel, hash([]byte("repo")))
	estadoLocal := fmt.Sprintf(`{%q:%q}`, rel, hash([]byte("local")))
	estadoViejo := fmt.Sprintf(`{%q:%q}`, rel, hash([]byte("base-viejo")))

	f.Add([]byte(estadoRepo), []byte("repo"), []byte("local"), false)
	f.Add([]byte(estadoLocal), []byte("repo"), []byte("local"), false)
	f.Add([]byte(estadoViejo), []byte("repo"), []byte("local"), false)
	f.Add([]byte(`{}`), []byte("repo"), []byte("local"), false)
	f.Add([]byte(estadoViejo), []byte("repo"), []byte("local"), true)
	f.Add([]byte(`{}`), []byte("repo"), []byte("repo"), false)
	f.Add([]byte(`{}`), []byte(""), []byte(""), false)
	f.Add([]byte(estadoViejo), []byte("repo"), []byte(""), false)

	f.Fuzz(func(t *testing.T, estado, repoBytes, localBytes []byte, force bool) {
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

		// Contenido local antes del pull.
		localAntes, errAntes := os.ReadFile(localFile)
		existiaAntes := errAntes == nil

		res, err := Pull(src, home, statePath, force, false)
		if err != nil {
			// Un estado inválido o un fs raro: error, no panic.
			return
		}

		localDespues, errDespues := os.ReadFile(localFile)
		existeDespues := errDespues == nil

		// Invariante 1: lo que el Result no dice que se escribió, no se
		// escribió. Kept y Conflicts dejan el local intacto.
		conservado := contiene(res.Kept, rel) || contiene(res.Conflicts, rel)
		if conservado {
			if existiaAntes != existeDespues {
				t.Fatalf("Kept/Conflicts pero el archivo %s (existía=%v, existe=%v)",
					rel, existiaAntes, existeDespues)
			}
			if existiaAntes && !bytes.Equal(localAntes, localDespues) {
				t.Fatalf("Kept/Conflicts pero el local cambió:\n antes=%q\ndespués=%q",
					localAntes, localDespues)
			}
		}

		// Invariante 2: lo que el Result dice que se escribió, se
		// escribió con EXACTAMENTE el contenido del repo.
		escrito := contiene(res.Updated, rel) || contiene(res.Missing, rel)
		if escrito {
			if !existeDespues {
				t.Fatalf("Updated/Missing pero %s no existe", rel)
			}
			if !bytes.Equal(localDespues, repoBytes) {
				t.Fatalf("Updated/Missing pero el contenido no es el del repo:\n repo=%q\n disco=%q",
					repoBytes, localDespues)
			}
		}

		// Invariante 3: el estado en disco siempre es JSON válido.
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("leer el estado: %v", err)
		}
		var st map[string]string
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatalf("el estado quedó corrupto: %v (%s)", err, data)
		}

		// Invariante 4: nada se escribe fuera del home.
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

// FuzzAdoptNoTocaElHome: Adopt solo lee.
func FuzzAdoptNoTocaElHome(f *testing.F) {
	rel := "noctalia/config.toml"

	f.Add([]byte(`{}`), []byte("repo"), []byte("local"))
	f.Add([]byte(fmt.Sprintf(`{%q:%q}`, rel, hash([]byte("repo")))), []byte("repo"), []byte("local"))
	f.Add([]byte(`null`), []byte("repo"), []byte(""))
	f.Add([]byte(`{}`), []byte(""), []byte("local"))

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

		// Todo el home antes, para comparar después.
		var recorrer func(string, map[string][]byte) error
		recorrer = func(base string, dst map[string][]byte) error {
			entradas, err := os.ReadDir(base)
			if err != nil {
				return err
			}
			for _, e := range entradas {
				p := filepath.Join(base, e.Name())
				if e.IsDir() {
					if err := recorrer(p, dst); err != nil {
						return err
					}
					continue
				}
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				dst[p] = data
			}
			return nil
		}
		antes := map[string][]byte{}
		if err := recorrer(home, antes); err != nil {
			t.Fatal(err)
		}

		if _, err := Adopt(src, home, statePath); err != nil {
			// Un estado inválido: error, no panic.
			return
		}

		// Invariante: Adopt no creó ni modificó nada en el home.
		despues := map[string][]byte{}
		if err := recorrer(home, despues); err != nil {
			t.Fatal(err)
		}
		if len(despues) != len(antes) {
			t.Fatalf("Adopt cambió la cantidad de archivos del home: %d → %d", len(antes), len(despues))
		}
		for p, data := range antes {
			otro, ok := despues[p]
			if !ok {
				t.Fatalf("Adopt borró %s", p)
			}
			if !bytes.Equal(data, otro) {
				t.Fatalf("Adopt cambió el contenido de %s", p)
			}
		}

		// Invariante: la base registrada es el hash del LOCAL, no el del
		// repo. Solo para las claves nuevas: Adopt "solo agrega" y no
		// pisa una base que ya estaba.
		stAntes := map[string]string{}
		if dataAntes, err := os.ReadFile(statePath); err == nil {
			json.Unmarshal(dataAntes, &stAntes)
		}
		data, err := os.ReadFile(statePath)
		if err != nil {
			return // el estado no se escribió (dry-run o error)
		}
		var st map[string]string
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatalf("el estado quedó corrupto: %v (%s)", err, data)
		}
		for clave, base := range st {
			if antes, ok := stAntes[clave]; ok {
				if antes != base {
					t.Fatalf("Adopt pisó la base de %s: %s → %s", clave, antes, base)
				}
				continue
			}
			if base != hash(localBytes) {
				t.Fatalf("la base nueva de %s no es el hash del local: %s", clave, base)
			}
		}
	})
}
