package systemdef

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadRemote(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"version":1,"profiles":{"desktop":{"repo":["htop"]}}}`)
	}))
	defer srv.Close()

	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	d, err := LoadRemote()
	if err != nil {
		t.Fatalf("LoadRemote: %v", err)
	}
	if got := len(d.Profiles["desktop"].Repo); got != 1 {
		t.Fatalf("esperaba 1 paquete, obtuve %d", got)
	}
}

func TestLoadRemoteFallback(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	d, err := LoadRemote()
	if err != nil {
		t.Fatalf("LoadRemote debía caer al embed: %v", err)
	}
	if len(d.Profiles) == 0 {
		t.Fatal("esperaba la definición embebida")
	}
}
