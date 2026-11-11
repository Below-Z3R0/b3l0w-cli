package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// TestForceOverwritesListaLosQueSePisanYNoLosQueSeConservan:
// ForceOverwrites dice qué archivos "--force" pisaría. Los que se
// conservan (solo cambió el local) no van en la lista: --force no los
// toca.
func TestForceOverwritesListaLosQueSePisanYNoLosQueSeConservan(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		// No lo tocó el usuario: el repo manda, se pisa igual.
		"a/pisa.toml": {base: "base", local: "base", repo: "nuevo"},
		// Editó el usuario y cambió el repo: conflicto real.
		"b/conflicto.toml": {base: "base", local: "mio", repo: "nuevo"},
		// Editó el usuario y el repo no cambió: se conserva.
		"c/conserva.toml": {base: "base", local: "mio", repo: "base"},
		// Nunca existió: se instala, no hay local que pisar.
		"d/nuevo.toml": {repo: "nuevo", sinLocal: true},
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}

	// Los que --force pisaría: el que el repo actualiza y el conflicto.
	want := []string{"a/pisa.toml", "b/conflicto.toml"}
	if !mismoConjunto(res.ForceOverwrites, want...) {
		t.Fatalf("ForceOverwrites = %v, quería %v", res.ForceOverwrites, want)
	}

	// El conservado y el instalado no van en la lista.
	if contiene(res.ForceOverwrites, "c/conserva.toml") {
		t.Fatalf("el conservado no debería ir en ForceOverwrites: %v", res.ForceOverwrites)
	}
	if contiene(res.ForceOverwrites, "d/nuevo.toml") {
		t.Fatalf("el instalado no debería ir en ForceOverwrites: %v", res.ForceOverwrites)
	}
}

// TestForceOverwritesIncluyeElConflictoSinBase: sin historial no se sabe
// quién cambió, así que --force lo pisaría igual. Va en la lista.
func TestForceOverwritesIncluyeElConflictoSinBase(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "mio", repo: "nuevo"},
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !mismoConjunto(res.ForceOverwrites, "noctalia/config.toml") {
		t.Fatalf("ForceOverwrites = %v, quería el archivo sin base", res.ForceOverwrites)
	}
	if !mismoConjunto(res.Conflicts, "noctalia/config.toml") {
		t.Fatalf("Conflicts = %v, quería el archivo sin base", res.Conflicts)
	}
}

// TestForceConservaCuandoSoloCambioElLocal: --force NO pisa un archivo
// que solo editó el usuario. Follows the rule: --force resuelve
// conflictos reales, no borra trabajo local.
func TestForceConservaCuandoSoloCambioElLocal(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "mio", repo: "base"},
	})
	path := filepath.Join(home, ".config", "noctalia", "config.toml")

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(path); string(got) != "mio" {
		t.Fatalf("--force pisó el cambio local: %q", got)
	}
	if !mismoConjunto(res.Kept, "noctalia/config.toml") {
		t.Fatalf("Kept = %v, quería el archivo conservado", res.Kept)
	}
	if len(res.Updated) != 0 {
		t.Fatalf("Updated = %v, quería vacío", res.Updated)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, quería vacío", res.Conflicts)
	}
	if len(res.ForceOverwrites) != 0 {
		t.Fatalf("ForceOverwrites = %v, quería vacío", res.ForceOverwrites)
	}
	// Ni .new: no hubo conflicto.
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Fatal("no debía escribir el .new")
	}
}

// TestForceAplicaElRepoEnConflictoReal: --force sí resuelve el conflicto
// real a favor del repo, sin dejar .new.
func TestForceAplicaElRepoEnConflictoReal(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "mio", repo: "nuevo"},
	})
	path := filepath.Join(home, ".config", "noctalia", "config.toml")

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(path); string(got) != "nuevo" {
		t.Fatalf("--force no aplicó el repo: %q", got)
	}
	if !mismoConjunto(res.Updated, "noctalia/config.toml") {
		t.Fatalf("Updated = %v, quería el archivo", res.Updated)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, quería vacío", res.Conflicts)
	}
	if !mismoConjunto(res.ForceOverwrites, "noctalia/config.toml") {
		t.Fatalf("ForceOverwrites = %v, quería el archivo", res.ForceOverwrites)
	}
	// Resuelto: no queda el .new del conflicto.
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Fatal("no debía quedar el .new")
	}
	// Y la base queda en la versión del repo.
	st := leerEstado(t, statePath)
	if st["noctalia/config.toml"] != hash([]byte("nuevo")) {
		t.Fatalf("el estado no avanzó: %v", st)
	}
}

// TestForcePisaElQueNoSeToco: --force no cambia el resultado del caso
// "el repo manda": el repo se aplica igual.
func TestForcePisaElQueNoSeToco(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	})
	path := filepath.Join(home, ".config", "noctalia", "config.toml")

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "nuevo" {
		t.Fatalf("no aplicó el repo: %q", got)
	}
	if !mismoConjunto(res.Updated, "noctalia/config.toml") {
		t.Fatalf("Updated = %v, quería el archivo", res.Updated)
	}
}

// TestForceNoBorraElLocalQueElUsuarioBorro: si el usuario borró el
// archivo y hay base, es borrado intencional: ni --force lo resucita.
func TestForceNoBorraElLocalQueElUsuarioBorro(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", repo: "nuevo", sinLocal: true},
	})
	path := filepath.Join(home, ".config", "noctalia", "config.toml")

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("--force resucitó el archivo borrado")
	}
	if !mismoConjunto(res.Kept, "noctalia/config.toml") {
		t.Fatalf("Kept = %v, quería el archivo borrado", res.Kept)
	}
	if len(res.Updated) != 0 {
		t.Fatalf("Updated = %v, quería vacío", res.Updated)
	}
}
