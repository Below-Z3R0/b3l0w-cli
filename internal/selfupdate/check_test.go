package selfupdate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCheckDevuelveElMayor es el bug A1: Check devolvía el primer tag
// v* por orden de la lista, no el de versión más alta.
func TestCheckDevuelveElMayor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v0.4.0","draft":false,"prerelease":false},
			{"tag_name":"v0.9.0","draft":false,"prerelease":false},
			{"tag_name":"v0.2.1","draft":false,"prerelease":false}
		]`)
	}))
	defer srv.Close()
	conInyectables(t, map[string]any{"apiURL": srv.URL})

	got, err := Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != "v0.9.0" {
		t.Errorf("Check = %q, quería v0.9.0 (el mayor, no el primero)", got)
	}
}

// TestCheckIgnoraDraftYPrerelease: un draft con versión más alta no
// puede ganar.
func TestCheckIgnoraDraftYPrerelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"tag_name":"v0.9.0","draft":false,"prerelease":false},
			{"tag_name":"v1.0.0","draft":true,"prerelease":false},
			{"tag_name":"v2.0.0","draft":false,"prerelease":true}
		]`)
	}))
	defer srv.Close()
	conInyectables(t, map[string]any{"apiURL": srv.URL})

	got, err := Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != "v0.9.0" {
		t.Errorf("Check = %q, quería v0.9.0: draft y prerelease no cuentan", got)
	}
}
