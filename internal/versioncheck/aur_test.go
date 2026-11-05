package versioncheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAUR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"resultcount":2,"results":[
			{"Name":"umbriel-git","Version":"1:0.1.0-1"},
			{"Name":"noctalia-git","Version":"1:5.4.1-1"}
		]}`))
	}))
	defer srv.Close()

	old := aurEndpoint
	aurEndpoint = srv.URL + "?"
	defer func() { aurEndpoint = old }()

	pkgs, err := AUR([]string{"umbriel-git", "noctalia-git"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("esperaba 2, obtuve %d", len(pkgs))
	}
	if pkgs[0].Name != "umbriel-git" || pkgs[0].Version == "" {
		t.Fatalf("paquete inesperado: %+v", pkgs[0])
	}
}

func TestAURError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	old := aurEndpoint
	aurEndpoint = srv.URL + "?"
	defer func() { aurEndpoint = old }()

	if _, err := AUR([]string{"x"}); err == nil {
		t.Fatal("esperaba error con HTTP 500")
	}
}
