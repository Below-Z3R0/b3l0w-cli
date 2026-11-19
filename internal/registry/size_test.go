package registry

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// TestFetchRechazaUnaRespuestaGigante fija que un body que supera maxSize
// es un error, no un JSON truncado que después se cachea y se sirve.
func TestFetchRechazaUnaRespuestaGigante(t *testing.T) {
	preparar(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), maxSize+1))
	})

	_, err := fetch(System)
	if err == nil {
		t.Fatal("fetch aceptó una respuesta que supera maxSize")
	}
	if !strings.Contains(err.Error(), "supera") {
		t.Errorf("error = %q, tenía que decir que la respuesta se pasó del límite", err)
	}
}

// TestFetchAceptaJustoElLimite es el contraste: maxSize exacto es válido.
func TestFetchAceptaJustoElLimite(t *testing.T) {
	preparar(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), maxSize))
	})

	if _, err := fetch(System); err != nil {
		t.Fatalf("fetch rechazó un body de exactamente maxSize: %v", err)
	}
}
