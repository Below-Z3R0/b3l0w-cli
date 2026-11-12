package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPullReportaHuerfanos: una ruta que estaba en el estado pero ya no
// está en el repo se reporta como huérfana. El prune saca la entrada del
// estado, pero el archivo local nunca se borra solo: que lo decida el
// usuario.
func TestPullReportaHuerfanos(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	}, "aurora/otro.toml", "noctalia/old.toml")

	// El huérfano existe en el home: no se debe tocar.
	oldPath := filepath.Join(home, ".config", "noctalia", "old.toml")
	if err := os.WriteFile(oldPath, []byte("mio"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}

	// Los dos huérfanos se reportan, ordenados.
	want := []string{"aurora/otro.toml", "noctalia/old.toml"}
	if !mismoConjunto(res.Orphaned, want...) {
		t.Fatalf("Orphaned = %v, quería %v", res.Orphaned, want)
	}
	for i := 1; i < len(res.Orphaned); i++ {
		if res.Orphaned[i-1] > res.Orphaned[i] {
			t.Fatalf("Orphaned no está ordenado: %v", res.Orphaned)
		}
	}

	// Un huérfano no es un archivo actualizado ni conservado.
	if contiene(res.Updated, "noctalia/old.toml") || contiene(res.Kept, "noctalia/old.toml") {
		t.Fatalf("el huérfano no debería estar en Updated/Kept: %+v", res)
	}

	// El archivo local del huérfano sigue ahí, con su contenido.
	if got, _ := os.ReadFile(oldPath); string(got) != "mio" {
		t.Fatalf("el huérfano local se pisó o se borró: %q", got)
	}

	// Del estado salen los huérfanos, pero el resto queda.
	st := leerEstado(t, statePath)
	if _, ok := st["noctalia/old.toml"]; ok {
		t.Fatalf("el huérfano sigue en el estado: %v", st)
	}
	if _, ok := st["aurora/otro.toml"]; ok {
		t.Fatalf("el huérfano sigue en el estado: %v", st)
	}
	if st["noctalia/config.toml"] != hash([]byte("nuevo")) {
		t.Fatalf("el estado no registró el archivo aplicado: %v", st)
	}
}

// TestPullSinHuerfanosNoReportaNada: si todas las rutas del estado siguen
// en el repo, Orphaned queda vacío y el estado no se toca.
func TestPullSinHuerfanosNoReportaNada(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("Orphaned = %v, quería vacío", res.Orphaned)
	}
}

// TestPullHuerfanoNoBorraElLocalNiConForce: aunque se pase --force, un
// huérfano solo se reporta: --force no borra archivos del home.
func TestPullHuerfanoNoBorraElLocalNiConForce(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {base: "base", local: "base", repo: "nuevo"},
	}, "noctalia/old.toml")

	oldPath := filepath.Join(home, ".config", "noctalia", "old.toml")
	if err := os.WriteFile(oldPath, []byte("mio"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(src, home, statePath, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !mismoConjunto(res.Orphaned, "noctalia/old.toml") {
		t.Fatalf("Orphaned = %v, quería el huérfano", res.Orphaned)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("--force borró el local del huérfano: %v", err)
	}
}
