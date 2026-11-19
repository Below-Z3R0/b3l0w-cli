package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPackageOwnerDetectaArchivoDePacman comprueba la regla que impide
// que el CLI pise un binario del sistema.
func TestPackageOwnerDetectaArchivoDePacman(t *testing.T) {
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman no está instalado")
	}
	if _, err := os.Stat("/usr/bin/pacman"); err != nil {
		t.Skip("/usr/bin/pacman no existe")
	}

	if owner := PackageOwner("/usr/bin/pacman"); owner != "pacman" {
		t.Errorf("PackageOwner(/usr/bin/pacman) = %q, quería pacman", owner)
	}
}

// TestPackageOwnerIgnoraArchivoPropio cubre el caso normal: un binario
// en ~/.local/bin no pertenece a ningún paquete.
func TestPackageOwnerIgnoraArchivoPropio(t *testing.T) {
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman no está instalado")
	}

	path := filepath.Join(t.TempDir(), "programa-propio")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho v1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if owner := PackageOwner(path); owner != "" {
		t.Errorf("PackageOwner(%s) = %q, quería vacío", path, owner)
	}
}

// TestPackageOwnerIgnoraInexistente no debe romper con rutas que no
// existen.
func TestPackageOwnerIgnoraInexistente(t *testing.T) {
	if owner := PackageOwner(filepath.Join(t.TempDir(), "no-existe")); owner != "" {
		t.Errorf("PackageOwner(inexistente) = %q, quería vacío", owner)
	}
}

// TestReplaceFileConservaElModoDelDestino verifica que reemplazar un
// archivo no le cambie los permisos.
//
// Un config 0644 que termina 0755 por reemplazarlo es un bug de
// permisos: cualquier usuario lo podría escribir.
func TestReplaceFileConservaElModoDelDestino(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "nuevo")
	dest := filepath.Join(dir, "instalado")

	if err := os.WriteFile(src, []byte("nuevo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("viejo"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceFile(src, dest); err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("modo = %o, quiero 644 (el del destino)", got)
	}
}

// TestReplaceFileResuelveSymlink verifica que se reemplaza el archivo
// real y no el symlink.
//
// Si el destino es un symlink y se reemplaza el symlink, queda
// convertido en una copia del binario: el original no se actualiza y
// quedan dos copias.
func TestReplaceFileResuelveSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	src := filepath.Join(dir, "nuevo")

	if err := os.WriteFile(real, []byte("viejo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("nuevo"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceFile(src, link); err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}

	data, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nuevo" {
		t.Errorf("el archivo real = %q, quiero %q", data, "nuevo")
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("el symlink se convirtió en copia: queda duplicado el binario")
	}
}

// TestReplaceBinaryDejaEjecutable es el bug M2 de internal/programs: un
// binario nuevo escrito con ReplaceFile queda 0644 y no se puede correr.
// ReplaceBinary tiene que dejarlo ejecutable.
func TestReplaceBinaryDejaEjecutable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "dest")

	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceBinary(src, dest); err != nil {
		t.Fatalf("ReplaceBinary: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("el binario quedó %o, sin permiso de ejecución", info.Mode().Perm())
	}

	// Y un binario que ya existía conserva su modo: no se fuerza 0755
	// sobre uno que el usuario dejó 0700.
	if err := os.Chmod(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceBinary(src, dest); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(dest)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("ReplaceBinary pisó el modo: quedó %o, quería 0700", info.Mode().Perm())
	}
}
