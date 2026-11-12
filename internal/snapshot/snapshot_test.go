package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Banco de pruebas
//
// snapshot.go llama a `snapper` por nombre (system.CommandPath + system.RunCombined),
// así que alcanza con poner un binario falso adelante en el PATH para ver qué
// le pide el módulo sin ejecutar el snapper real y sin tocar btrfs.
//
//   - snapper → loguea sus argumentos en $FAKE_LOG, escribe $FAKE_STDERR en
//                stderr y sale con $FAKE_EXIT
//
// El fake va PRIMERO en el PATH, así que el binario real no se ejecuta jamás.
// ---------------------------------------------------------------------------

// fakeSnapper instala un "snapper" falso y devuelve el path de su log.
func fakeSnapper(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
for a in "$@"; do printf '%s\n' "$a" >> "$FAKE_LOG"; done
if [ -n "$FAKE_STDERR" ]; then printf '%s\n' "$FAKE_STDERR" >&2; fi
exit "${FAKE_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(dir, "snapper"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", log)
	t.Setenv("FAKE_EXIT", "0")
	t.Setenv("FAKE_STDERR", "")
	return log
}

// sinSnapper deja un PATH donde no hay ningún snapper.
func sinSnapper(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// argsDelFake devuelve los argumentos que recibió el snapper falso.
// nil significa que el fake nunca corrió.
func argsDelFake(t *testing.T, log string) []string {
	t.Helper()

	data, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	texto := strings.TrimRight(string(data), "\n")
	if texto == "" {
		return nil
	}
	return strings.Split(texto, "\n")
}

// TestCreateSinSnapperInstalado cubre el caso de una máquina donde snapper
// no está: el módulo avisa y no intenta nada más.
func TestCreateSinSnapperInstalado(t *testing.T) {
	sinSnapper(t)

	err := Create("b3l0w pre-dotfiles")
	if err == nil {
		t.Fatal("Create tenía que fallar sin snapper instalado")
	}
	if !strings.Contains(err.Error(), "snapper no está instalado") {
		t.Errorf("error = %q, quería que dijera que snapper no está instalado", err)
	}
}

// TestCreateConSnapperInstalado es el camino normal: snapper está, la
// snapshot se crea y no hay error.
func TestCreateConSnapperInstalado(t *testing.T) {
	log := fakeSnapper(t)

	if err := Create("b3l0w pre-dotfiles"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	esperados := []string{"-c", "home", "create", "-d", "b3l0w pre-dotfiles"}
	got := argsDelFake(t, log)
	if len(got) != len(esperados) {
		t.Fatalf("args = %v, quería %v", got, esperados)
	}
	for i := range esperados {
		if got[i] != esperados[i] {
			t.Errorf("arg %d = %q, quería %q", i, got[i], esperados[i])
		}
	}
}

// TestCreateConSnapperQueFalla cubre el error de snapper: el mensaje tiene
// que traer la salida del comando, si no el usuario no sabe qué pasó.
func TestCreateConSnapperQueFalla(t *testing.T) {
	fakeSnapper(t)
	t.Setenv("FAKE_EXIT", "1")
	t.Setenv("FAKE_STDERR", "Unknown config")

	err := Create("b3l0w pre-dotfiles")
	if err == nil {
		t.Fatal("Create tenía que fallar con snapper fallando")
	}
	if !strings.Contains(err.Error(), "snapper -c home") {
		t.Errorf("error = %q, tenía que decir qué comando falló", err)
	}
	if !strings.Contains(err.Error(), "Unknown config") {
		t.Errorf("error = %q, tenía que traer la salida de snapper", err)
	}
}

// TestCreateConLabelVacio define el comportamiento del label vacío: se
// rechaza antes de tocar el disco, porque una snapshot sin descripción no
// se puede identificar después.
func TestCreateConLabelVacio(t *testing.T) {
	log := fakeSnapper(t)

	err := Create("")
	if err == nil {
		t.Fatal("Create tenía que fallar con label vacío")
	}
	if !strings.Contains(err.Error(), "label") {
		t.Errorf("error = %q, tenía que decir que el label está vacío", err)
	}
	if args := argsDelFake(t, log); args != nil {
		t.Errorf("no tenía que llamar a snapper, lo llamó con %v", args)
	}
}

// TestCreateConLabelSoloEspacios: un label de puros espacios tampoco dice nada.
func TestCreateConLabelSoloEspacios(t *testing.T) {
	log := fakeSnapper(t)

	err := Create("   \t  ")
	if err == nil {
		t.Fatal("Create tenía que fallar con label de puros espacios")
	}
	if args := argsDelFake(t, log); args != nil {
		t.Errorf("no tenía que llamar a snapper, lo llamó con %v", args)
	}
}

// TestCreateConLabelConEspacios fija que el label viaja como un argumento
// solo: no hay shell que lo parta en palabras.
func TestCreateConLabelConEspacios(t *testing.T) {
	log := fakeSnapper(t)

	if err := Create("b3l0w pre-dotfiles 2026"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got := argsDelFake(t, log)
	if len(got) != 5 {
		t.Fatalf("args = %v, quería 5 argumentos", got)
	}
	if got[4] != "b3l0w pre-dotfiles 2026" {
		t.Errorf("label = %q, quería el label entero", got[4])
	}
}
