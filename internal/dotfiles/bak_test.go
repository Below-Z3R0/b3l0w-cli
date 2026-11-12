package dotfiles

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests del backup ".bak".
//
// `backupLocal` deja una copia del contenido local al lado del destino
// antes de pisarlo con --force. Lo que NO está decidido (y por eso acá
// solo se documenta el comportamiento actual) es qué pasa cuando el
// backup ya existe: hoy se sobreescribe siempre.

// TestEdge_ElBakPrevioSeSobrescribeConElLocalMasReciente: si ya hay un
// ".bak" de una corrida anterior, el force lo pisa con el contenido
// local actual. Queda el más nuevo, no el más viejo.
func TestEdge_ElBakPrevioSeSobrescribeConElLocalMasReciente(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "local-nuevo", repo: "repo"},
	})
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	// Un .bak de una corrida anterior.
	bugEscribir(t, dest+".bak", "bak-viejo")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dest + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "local-nuevo" {
		t.Fatalf("el .bak quedó con %q, quería el local más reciente", got)
	}
}

// TestEdge_ElBakEditadoAManoSePierdeEnElProximoForce: si el usuario
// editó el ".bak" a mano (lo renombró, lo corrigió, lo dejó como
// referencia), el próximo --force lo pisa sin avisar.
//
// DECISIÓN DE DISEÑO PENDIENTE (ver TESTS.md): ¿el .bak debería rotar
// (.bak → .bak.1), debería negarse a pisar uno editado, o debería
// escribirse con timestamp? Hoy el último gana y el trabajo curado a
// mano se pierde en silencio.
func TestEdge_ElBakEditadoAManoSePierdeEnElProximoForce(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "local-v1", repo: "repo-v1"},
	})
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}
	// El usuario edita el backup a mano y sigue trabajando.
	bugEscribir(t, dest+".bak", "MI-BAK-CURADO-A-MANO")
	bugEscribir(t, dest, "local-v2")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo-v2")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dest + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	// Comportamiento ACTUAL: el .bak del usuario se perdió.
	if string(got) == "MI-BAK-CURADO-A-MANO" {
		t.Skip("el .bak editado a mano se conservó: la política cambió")
	}
	if string(got) != "local-v2" {
		t.Fatalf("el .bak quedó con %q, quería el local más reciente", got)
	}
}

// TestEdge_ElBakSeEscribeJuntoAlSymlinkYElTargetSeActualiza: si el
// destino es un symlink, el backup queda junto al symlink (en
// ~/.config) y el archivo real (el target) es el que se pisa. El symlink
// no se reemplaza.
func TestEdge_ElBakSeEscribeJuntoAlSymlinkYElTargetSeActualiza(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	target := filepath.Join(root, "target.toml")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo-nuevo")
	bugEscribir(t, target, "mio-en-el-target")
	if err := os.MkdirAll(filepath.Join(home, ".config", "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}

	// El target se actualizó con el repo.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "repo-nuevo" {
		t.Fatalf("el target no se actualizó: %q", got)
	}
	// El symlink sigue siendo symlink.
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("el symlink fue reemplazado por un archivo regular")
	}
	// El backup está junto al symlink, con el contenido viejo del target.
	backup, err := os.ReadFile(dest + ".bak")
	if err != nil {
		t.Fatalf("no hay backup junto al symlink: %v", err)
	}
	if string(backup) != "mio-en-el-target" {
		t.Fatalf("el backup no tiene el contenido anterior: %q", backup)
	}
}

// TestEdge_NoHayBakParaUnLocalVacio: un local vacío no genera backup:
// no hay nada que recuperar.
func TestEdge_NoHayBakParaUnLocalVacio(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "", repo: "repo"},
	})
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".bak"); !os.IsNotExist(err) {
		t.Fatal("un local vacío no debería dejar backup")
	}
	// El archivo sí se aplicó.
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "repo" {
		t.Fatalf("no se aplicó el repo: %q", got)
	}
}

// TestEdge_NoHayBakEnElDryRun: el dry-run no deja backup.
func TestEdge_NoHayBakEnElDryRun(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {local: "mio", repo: "repo"},
	})
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")

	if _, err := Pull(src, home, statePath, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".bak"); !os.IsNotExist(err) {
		t.Fatal("el dry-run dejó un backup")
	}
}

// TestEdge_NoHayBakSiElLocalNoExiste: si el archivo no estaba, no hay
// nada que respaldar.
func TestEdge_NoHayBakSiElLocalNoExiste(t *testing.T) {
	src, home, statePath := escenarioMulti(t, map[string]casoArchivo{
		"noctalia/config.toml": {repo: "repo", sinLocal: true},
	})
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")

	if _, err := Pull(src, home, statePath, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest + ".bak"); !os.IsNotExist(err) {
		t.Fatal("debería haber backup cuando el local no existe")
	}
}
