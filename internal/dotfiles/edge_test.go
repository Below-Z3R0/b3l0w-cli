package dotfiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Tests de borde que NO son bugs: comportamiento que ya funciona y que
// hay que blindar para que un arreglo futuro no lo rompa.
//
// Nombres TestEdge_*. Todos en t.TempDir().

// TestEdge_ArchivoVacioSeInstalaYEsEstable: un archivo de 0 bytes tiene
// hash propio y se instala bien; el segundo pull no lo vuelve a tocar.
func TestEdge_ArchivoVacioSeInstalaYEsEstable(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "vacio.toml")
	dest := filepath.Join(home, ".config", "noctalia", "vacio.toml")
	bugEscribir(t, repo, "")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Missing, "noctalia/vacio.toml") {
		t.Fatalf("Missing = %v, quería el archivo vacío", res.Missing)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("no se instaló el archivo vacío: %v", err)
	}

	// Segundo pull: nada que hacer.
	res2, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	if res2.Changed {
		t.Fatalf("el segundo pull no debería cambiar nada: %+v", res2)
	}
	if len(res2.Updated)+len(res2.Missing)+len(res2.Conflicts) != 0 {
		t.Fatalf("el segundo pull reporta cambios: %+v", res2)
	}
}

// TestEdge_ArchivoVacioYArchivoAusenteNoSeConfunden: el hash de un
// archivo vacío es el de nil. El código tiene que distinguir "no existe"
// (Missing) de "existe y está vacío" (adopción).
func TestEdge_ArchivoVacioYArchivoAusenteNoSeConfunden(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "vacio.toml"), "")
	// El local existe pero está vacío.
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "vacio.toml"), "")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("el local vacío no está ausente: %+v", res)
	}
	if !mismoConjunto(res.Kept, "noctalia/vacio.toml") {
		t.Fatalf("Kept = %v, quería el archivo vacío adoptado", res.Kept)
	}
}

// TestEdge_ContenidoBinarioSeInstalaIntacto: bytes con NUL y UTF-8
// inválido. El contenido no va al estado (solo el hash), así que no
// debería romper ni el JSON ni la copia.
func TestEdge_ContenidoBinarioSeInstalaIntacto(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bin := []byte{0x00, 0xff, 0x01, 0x00, 0xfe, 'a', 0x00}
	repo := filepath.Join(src, "noctalia", "bin.dat")
	dest := filepath.Join(home, ".config", "noctalia", "bin.dat")
	if err := os.MkdirAll(filepath.Dir(repo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repo, bin, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(bin) {
		t.Fatalf("el binario no se copió intacto: %v", got)
	}
	// Y el estado tiene que seguir siendo JSON válido.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]string
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("estado inválido: %v", err)
	}
}

// TestEdge_RutasConEspaciosYUnicode: espacios y no-ASCII en la ruta
// relativa. El estado es JSON: las claves van escapadas.
func TestEdge_RutasConEspaciosYUnicode(t *testing.T) {
	src, home, statePath := bugSetup(t)
	rel := filepath.Join("mis configs", "ñoño", "archivo con espacios.toml")
	bugEscribir(t, filepath.Join(src, rel), "repo")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Missing) != 1 {
		t.Fatalf("Missing = %v, quería 1", res.Missing)
	}
	got, err := os.ReadFile(filepath.Join(home, ".config", rel))
	if err != nil {
		t.Fatalf("no se instaló: %v", err)
	}
	if string(got) != "repo" {
		t.Fatalf("contenido = %q", got)
	}
	// La clave del estado tiene que sobrevivir el round-trip.
	leido := leerEstado(t, statePath)
	if _, ok := leido[filepath.ToSlash(rel)]; !ok {
		t.Fatalf("la clave del estado no es la ruta relativa: %v", leido)
	}
}

// TestEdge_DirectoriosAnidadosProfundos: la ruta se crea entera.
func TestEdge_DirectoriosAnidadosProfundos(t *testing.T) {
	src, home, statePath := bugSetup(t)
	rel := filepath.Join("a", "b", "c", "d", "e", "f", "g.toml")
	bugEscribir(t, filepath.Join(src, rel), "repo")

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(home, ".config", rel))
	if err != nil {
		t.Fatalf("no se instaló: %v", err)
	}
	if string(got) != "repo" {
		t.Fatalf("contenido = %q", got)
	}
}

// TestEdge_DosArchivosConElMismoHashNoSeContaminan: dos archivos con
// contenido idéntico (mismo sha256) pero rutas distintas. La decisión
// 3-way es por ruta: tocar uno no puede afectar al otro.
func TestEdge_DosArchivosConElMismoHashNoSeContaminan(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "a", "x.toml"), "igual")
	bugEscribir(t, filepath.Join(src, "b", "y.toml"), "igual")
	// El local de "a" difiere; el de "b" es idéntico al repo.
	bugEscribir(t, filepath.Join(home, ".config", "a", "x.toml"), "diferente")
	bugEscribir(t, filepath.Join(home, ".config", "b", "y.toml"), "igual")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// "a/x.toml" entra en conflicto (sin base, difieren).
	if !mismoConjunto(res.Conflicts, "a/x.toml") {
		t.Fatalf("Conflicts = %v, quería a/x.toml", res.Conflicts)
	}
	// "b/y.toml" es idéntico: se adopta.
	if !mismoConjunto(res.Kept, "b/y.toml") {
		t.Fatalf("Kept = %v, quería b/y.toml", res.Kept)
	}
	// Y el estado guarda dos claves con el mismo valor, sin mezclarse.
	leido := leerEstado(t, statePath)
	if leido["a/x.toml"] != leido["b/y.toml"] {
		t.Fatalf("los hashes no coinciden: %v", leido)
	}
}

// TestEdge_HomeInexistenteSeCrea: Pull sobre un home que no existe
// crea ~/.config y el archivo. No es un error.
func TestEdge_HomeInexistenteSeCrea(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home-que-no-existe")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Missing, "noctalia/config.toml") {
		t.Fatalf("Missing = %v", res.Missing)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se creó el archivo: %v", err)
	}
}

// TestEdge_AdoptDosVecesEsIdempotente: Adoptar dos veces sin cambios no
// mueve la base: la segunda corrida no adopta nada nuevo y el estado
// queda idéntico.
func TestEdge_AdoptDosVecesEsIdempotente(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")

	n1, err := Adopt(src, home, statePath)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 1 {
		t.Fatalf("la primera corrida adoptó %d, quería 1", n1)
	}
	estado1 := leerEstado(t, statePath)

	n2, err := Adopt(src, home, statePath)
	if err != nil {
		t.Fatal(err)
	}
	estado2 := leerEstado(t, statePath)

	// "Solo agrega": la segunda corrida no tiene nada nuevo que adoptar.
	if n2 != 0 {
		t.Fatalf("la segunda corrida adoptó %d, quería 0 (ya había base)", n2)
	}
	if len(estado1) != len(estado2) {
		t.Fatalf("el estado cambió: %v vs %v", estado1, estado2)
	}
	for k, v := range estado1 {
		if estado2[k] != v {
			t.Fatalf("la base de %s cambió: %s vs %s", k, v, estado2[k])
		}
	}
}

// TestEdge_AdoptDryRunNoEscribeElEstado: el dryRun opcional de Adopt
// informa el conteo pero no toca el estado en disco.
func TestEdge_AdoptDryRunNoEscribeElEstado(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "mio")

	n, err := Adopt(src, home, statePath, true)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if n != 1 {
		t.Fatalf("adoptó %d, quería 1", n)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("el dry-run de Adopt escribió el estado")
	}
}

// TestEdge_EstadoConRutasQueNoExistenSeReportanComoHuerfanas: el
// estado puede apuntar a rutas que ya no están en el repo. Se reportan
// y se podan, sin tocar el disco.
func TestEdge_EstadoConRutasQueNoExistenSeReportanComoHuerfanas(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEstadoBase(t, statePath, map[string]string{
		"noctalia/config.toml":    hash([]byte("repo")),
		"viejo/desaparecido.toml": hash([]byte("algo")),
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !mismoConjunto(res.Orphaned, "viejo/desaparecido.toml") {
		t.Fatalf("Orphaned = %v", res.Orphaned)
	}
	leido := leerEstado(t, statePath)
	if _, ok := leido["viejo/desaparecido.toml"]; ok {
		t.Fatalf("el huérfano sigue en el estado: %v", leido)
	}
}

// TestEdge_ElEstadoNoAceptaRutasAbsolutasNiConPuntos: una clave del
// estado con ".." o ruta absoluta nunca se usa como nombre de archivo:
// solo se comparan contra lo que hay en el repo.
func TestEdge_ElEstadoNoAceptaRutasAbsolutasNiConPuntos(t *testing.T) {
	src, home, statePath := bugSetup(t)
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "repo")
	bugEstadoBase(t, statePath, map[string]string{
		"../../../../etc/passwd": "aa",
		"/etc/shadow":            "bb",
	})

	res, err := Pull(src, home, statePath, false, false)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// Se reportan como huérfanos y se podan; no se crea nada fuera del home.
	for _, huerfano := range res.Orphaned {
		if huerfano != "../../../../etc/passwd" && huerfano != "/etc/shadow" {
			t.Fatalf("Orphaned inesperado: %v", res.Orphaned)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "noctalia", "config.toml")); err != nil {
		t.Fatalf("no se instaló el archivo del repo: %v", err)
	}
	// Nada se escribió fuera del home.
	for _, fuera := range []string{
		filepath.Join(home, "..", "..", "..", "..", "etc", "passwd"),
		"/etc/shadow.b3l0w",
	} {
		if _, err := os.Stat(fuera); err == nil {
			t.Fatalf("se escribió fuera del home: %s", fuera)
		}
	}
}

// TestEdge_ModoReadOnlySeConserva: un 0444 en el destino se mantiene
// después del update (no se abre a 0644).
func TestEdge_ModoReadOnlySeConserva(t *testing.T) {
	src, home, statePath := bugSetup(t)
	repo := filepath.Join(src, "noctalia", "config.toml")
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	bugEscribir(t, repo, "v1")
	bugEscribir(t, dest, "v1")
	if err := os.Chmod(dest, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dest, 0o644) })

	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// El repo cambió: se actualiza.
	bugEscribir(t, repo, "v2")
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o444 {
		t.Fatalf("el modo read-only no se conservó: %v", fi.Mode().Perm())
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "v2" {
		t.Fatalf("no se actualizó: %q", got)
	}
}

// TestEdge_DestinoQueEsSymlinkSeEscribeEnElTarget: el symlink del home
// se conserva como symlink y el contenido va a su target.
func TestEdge_DestinoQueEsSymlinkSeEscribeEnElTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "v1")
	bugEscribir(t, filepath.Join(home, ".config", "noctalia", "config.toml"), "v1")
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Ahora el destino es un symlink a otro lado.
	target := filepath.Join(root, "target.toml")
	bugEscribir(t, target, "v1")
	dest := filepath.Join(home, ".config", "noctalia", "config.toml")
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("no se puede crear symlink: %v", err)
	}

	bugEscribir(t, filepath.Join(src, "noctalia", "config.toml"), "v2")
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("el symlink fue reemplazado por un archivo regular")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v2" {
		t.Fatalf("el target no se actualizó: %q", got)
	}
}
