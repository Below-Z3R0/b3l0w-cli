package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Escenario de conflicto: el repo tiene un archivo, el home tiene OTRO
// contenido y el estado tiene la base del repo. El 3-way lo marca como
// conflicto real.
func escenarioConflicto(t *testing.T) (home, repo string) {
	t.Helper()

	home = t.TempDir()
	t.Setenv("HOME", home)

	repo = t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app", "config.toml"), []byte("del repo = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(home, ".config", "app", "config.toml")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("editado por el usuario\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El estado tiene una TERCERA version como base: la que estaba
	// antes de que el repo cambiara y antes de que el usuario
	// editara. Asi los dos cambiaron (rama 7) y es un conflicto real.
	//
	// Con la base igual al repo, la rama 6 lo conservaria ("kept") y
	// no habria conflicto: ese caso no es el que se quiere probar.
	estado := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	if err := os.MkdirAll(filepath.Dir(estado), 0o755); err != nil {
		t.Fatal(err)
	}
	base := hashDe("version anterior\n")
	if err := os.WriteFile(estado, []byte("{\n  \"app/config.toml\": \""+base+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return home, repo
}

// hashDe es el sha256 en hex de un contenido: el mismo formato que
// guarda el estado de dotfiles.
func hashDe(contenido string) string {
	suma := sha256.Sum256([]byte(contenido))
	return hex.EncodeToString(suma[:])
}

// El dry-run tiene que mostrar los conflictos. Antes no los mostraba:
// el preview estaba dentro de un `if !opts.DryRun`, asi que el dry-run
// no decia lo que pasaria y era inútil.
func TestBug_ElDryRunMuestraLosConflictos(t *testing.T) {
	home, repo := escenarioConflicto(t)

	dir := t.TempDir()
	fakeBin(t, dir, "snapper", "#!/bin/sh\nexit 0\n")
	fakeBin(t, dir, "noctalia", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	salida, _ := capturarSalida(t, func() {
		if code := updateDotfiles(Options{DryRun: true, DotfilesDir: repo}); code != 0 {
			t.Errorf("updateDotfiles(--dry-run) = %d, quería 0", code)
		}
	})

	if !strings.Contains(salida, "conflict") {
		t.Fatalf("el dry-run no mencionó el conflicto: %q", salida)
	}
	if !strings.Contains(salida, "app/config.toml") {
		t.Errorf("el dry-run no nombró el archivo en conflicto: %q", salida)
	}
	if !strings.Contains(salida, "dry-run") {
		t.Errorf("el dry-run no se anuncia: %q", salida)
	}

	// Y no escribió nada: ni el archivo ni el .new.
	dest := filepath.Join(home, ".config", "app", "config.toml")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "editado por el usuario\n" {
		t.Errorf("el dry-run toco el archivo: %q", string(data))
	}
	if _, err := os.Stat(dest + ".new"); !os.IsNotExist(err) {
		t.Error("el dry-run escribio un .new")
	}
}

// El dry-run tiene que decir cuantos archivos-conflicto se pisarian con
// --force. Sin ese numero el usuario no puede decidir.
func TestBug_ElDryRunAnunciaCuantosConflictosHay(t *testing.T) {
	_, repo := escenarioConflicto(t)

	dir := t.TempDir()
	fakeBin(t, dir, "snapper", "#!/bin/sh\nexit 0\n")
	fakeBin(t, dir, "noctalia", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	salida, _ := capturarSalida(t, func() {
		updateDotfiles(Options{DryRun: true, DotfilesDir: repo})
	})

	// "1 conflict" (o el plural) tiene que aparecer.
	if !strings.Contains(salida, "1 conflict") {
		t.Errorf("no Announces el número de conflictos: %q", salida)
	}
}

// Los huerfanos (en el estado pero no en el repo) se tienen que avisar.
// El campo existia en el modulo de dotfiles pero la CLI no lo imprimia.
func TestBug_LosHuerfanosSeAvisan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app", "config.toml"), []byte("del repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El estado tiene un archivo que el repo ya no tiene.
	estado := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	if err := os.MkdirAll(filepath.Dir(estado), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(estado, []byte("{\n  \"viejo/config.toml\": \""+hashDe("x")+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	fakeBin(t, dir, "snapper", "#!/bin/sh\nexit 0\n")
	fakeBin(t, dir, "noctalia", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	salida, _ := capturarSalida(t, func() {
		updateDotfiles(Options{DotfilesDir: repo})
	})

	if !strings.Contains(salida, "orphan") {
		t.Errorf("no avisó del huerfano: %q", salida)
	}
	if !strings.Contains(salida, "viejo/config.toml") {
		t.Errorf("no nombró el huerfano: %q", salida)
	}
}
