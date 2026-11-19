package versioncheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAURConErrorEnElJSON fija que un 200 con type:"error" es un fallo.
func TestAURConErrorEnElJSON(t *testing.T) {
	viejo := aurEndpoint
	t.Cleanup(func() { aurEndpoint = viejo })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"No request type/data specified.","resultcount":0,"results":[],"type":"error","version":5}`))
	}))
	defer srv.Close()
	aurEndpoint = srv.URL + "?"

	if _, err := AUR([]string{"x"}); err == nil {
		t.Fatal("AUR aceptó un 200 con error")
	} else if !strings.Contains(err.Error(), "No request type") {
		t.Errorf("error = %q", err)
	}
}

// TestAURMandaUserAgent fija que el AUR recibe un UA identificable.
func TestAURMandaUserAgent(t *testing.T) {
	viejo := aurEndpoint
	t.Cleanup(func() { aurEndpoint = viejo })
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.Write([]byte(`{"resultcount":1,"results":[{"Name":"x","Version":"1.0"}]}`))
	}))
	defer srv.Close()
	aurEndpoint = srv.URL + "?"

	if _, err := AUR([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if got != UserAgent {
		t.Errorf("User-Agent = %q, quería %q", got, UserAgent)
	}
}

// TestAURReportaElPaqueteQueFalta fija que un nombre inexistente no
// desaparece de la lista sin aviso.
func TestAURReportaElPaqueteQueFalta(t *testing.T) {
	viejo := aurEndpoint
	t.Cleanup(func() { aurEndpoint = viejo })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"resultcount":1,"results":[{"Name":"umbriel-git","Version":"1.0"}]}`))
	}))
	defer srv.Close()
	aurEndpoint = srv.URL + "?"

	_, err := AUR([]string{"umbriel-git", "no-existe-xyz"})
	if err == nil {
		t.Fatal("AUR no avisó del paquete que falta")
	}
	if !strings.Contains(err.Error(), "no-existe-xyz") {
		t.Errorf("error = %q, tenía que nombrar el paquete", err)
	}
}
