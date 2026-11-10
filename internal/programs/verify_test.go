package programs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Below-Z3R0/b3l0w-kit/verify"
)

// TestVerifyReleaseSinFirma cubre el release que no publica SHA256SUMS:
// el instalador tiene que negarse, no seguir de largo.
func TestVerifyReleaseSinFirma(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	err := VerifyRelease(srv.URL+"/b3l0w-linux-amd64", "b3l0w-linux-amd64", "/tmp/asset")
	if !errors.Is(err, verify.ErrSinFirma) {
		t.Fatalf("error = %v, quería ErrSinFirma", err)
	}
}

// TestVerifyReleaseURLExtrana no adivina la base del release.
func TestVerifyReleaseURLExtrana(t *testing.T) {
	err := VerifyRelease("https://example.com/otro", "b3l0w-linux-amd64", "/tmp/asset")
	if err == nil {
		t.Fatal("esperaba error con una URL que no contiene el asset")
	}
}
