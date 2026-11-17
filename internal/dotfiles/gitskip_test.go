package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests de la política de `skip()`: qué del repo se despliega y qué no.
//
// `skip()` deja fuera: `.md` (y README.md), `outputs.toml`,
// `templates/hermes/` y el `.git` (directorio O archivo). Todo lo demás
// se despliega en ~/.config, sin importar qué sea.

// TestEdge_ElArchivoGitNoSeDespliega: un ".git" que es un ARCHIVO (no
// un directorio) es el caso de un worktree o un submódulo: el archivo
// apunta con "gitdir:" al repo real. No es una config del usuario y no
// puede terminar en ~/.config/.git.
func TestEdge_ElArchivoGitNoSeDespliega(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	// Un ".git" de archivo, como el de un worktree.
	bugEscribir(t, filepath.Join(src, ".git"), "gitdir: /home/usuario/repo/.git/worktrees/wt\n")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", ".git")); !os.IsNotExist(err) {
		t.Fatal("el archivo .git del repo se desplegó en ~/.config/.git")
	}
	// Y el resto del repo se aplica normal.
	if !mismoConjunto(res.Missing, "noctalia/config.toml") {
		t.Fatalf("Missing = %v", res.Missing)
	}
}

// TestEdge_ElGitignoreNoSeSaltea: un ".gitignore" NO es un ".git": es
// un archivo de configuración legítimo. `skip()` solo descarta el
// ".git" exacto y lo que empieza por ".git/". El ".gitignore" se
// despliega.
func TestEdge_ElGitignoreNoSeSaltea(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	bugEscribir(t, filepath.Join(src, ".gitignore"), "*.tmp\n")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !mismoConjunto(res.Missing, ".gitignore", "noctalia/config.toml") {
		t.Fatalf("Missing = %v, quería el .gitignore desplegado", res.Missing)
	}
	got, err := os.ReadFile(filepath.Join(home, ".config", ".gitignore"))
	if err != nil {
		t.Fatalf("el .gitignore no se desplegó: %v", err)
	}
	if string(got) != "*.tmp\n" {
		t.Fatalf("contenido del .gitignore = %q", got)
	}
}

// TestEdge_ElGithubYElLicenseSeDespliegan: `skip()` está cableado al
// layout del repo de b3l0w (BUGS.md M8): lo que no sea .md, outputs.toml
// o templates/hermes se despliega, including el CI y la licencia.
//
// Comportamiento ACTUAL, no bug: la política es explícita y el README la
// documenta. Lo que queda abierto es si la política debería cambiar
// (ver TESTS.md).
func TestEdge_ElGithubYElLicenseSeDespliegan(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	bugEscribir(t, filepath.Join(src, ".github", "workflows", "ci.yml"), "name: ci\n")
	bugEscribir(t, filepath.Join(src, "LICENSE"), "MIT\n")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".github/workflows/ci.yml", "LICENSE"} {
		if _, err := os.Stat(filepath.Join(home, ".config", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s no se desplegó: %v", rel, err)
		}
	}
}

// TestEdge_ElMdNoSeDespliegaPeroCuentaComoVisto: un ".md" del repo no
// se despliega, pero sí cuenta como "visto": si estaba en el estado, no
// se reporta como huérfano.
func TestEdge_ElMdNoSeDespliegaPeroCuentaComoVisto(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	bugEscribir(t, filepath.Join(src, "notas.md"), "# notas\n")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml": hash([]byte("repo")),
		"notas.md":             hash([]byte("viejo")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "notas.md")); !os.IsNotExist(err) {
		t.Fatal("el .md del repo se desplegó")
	}
	for _, h := range res.Orphaned {
		if h == "notas.md" {
			t.Fatalf("el .md se reportó como huérfano: %v", res.Orphaned)
		}
	}
}
