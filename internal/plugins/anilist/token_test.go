package anilist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const envAusente = "B3L0W_TEST_ANILIST_TOKEN_AUSENTE"

func TestTokenPrecedenciaYDegradacion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(`{"access_token":"del-archivo"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. El flag explícito manda sobre todo lo demás.
	got, err := Token("del-flag", "B3L0W_TEST_ANILIST_TOKEN", dir)
	if err != nil || got != "del-flag" {
		t.Errorf("token = %q (%v); quería del-flag", got, err)
	}

	// 2. Sin flag, la variable de entorno.
	t.Setenv("B3L0W_TEST_ANILIST_TOKEN", "del-entorno")
	got, err = Token("", "B3L0W_TEST_ANILIST_TOKEN", dir)
	if err != nil || got != "del-entorno" {
		t.Errorf("token = %q (%v); quería del-entorno", got, err)
	}

	// 3. Sin flag ni variable, el token.json del data dir.
	t.Setenv(envAusente, "")
	got, err = Token("", envAusente, dir)
	if err != nil || got != "del-archivo" {
		t.Errorf("token = %q (%v); quería del-archivo", got, err)
	}

	// 4. Sin nada, el error dice qué falta y no filtra secretos.
	_, err = Token("", envAusente, "")
	if err == nil {
		t.Fatal("sin token debería fallar")
	}
	if !strings.Contains(err.Error(), envAusente) {
		t.Errorf("el error no nombra la variable: %v", err)
	}

	// 5. Un token.json sin access_token no es un token válido.
	vacio := t.TempDir()
	if err := os.WriteFile(filepath.Join(vacio, "token.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Token("", envAusente, vacio); err == nil {
		t.Error("un token.json sin access_token tiene que ser error")
	}
}
