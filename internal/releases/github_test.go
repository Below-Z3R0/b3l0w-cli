package releases

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// usarServidor apunta las bases a un servidor de prueba.
func usarServidor(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	viejoAPI, viejoDown := apiBase, downloadBase
	viejoDelay := retryDelay
	retryDelay = 0
	apiBase, downloadBase = srv.URL, srv.URL
	t.Cleanup(func() {
		apiBase, downloadBase = viejoAPI, viejoDown
		retryDelay = viejoDelay
	})
}

func TestLatestTag(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "b3l0w" {
			t.Error("falta User-Agent")
		}
		w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	})

	tag, err := LatestTag("owner/repo")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if tag != "v1.2.3" {
		t.Errorf("tag = %q", tag)
	}
}

func TestLatestTagSinReleases(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := LatestTag("owner/repo"); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestHasSignatureSi(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/repo/releases/download/v1.0.0/SHA256SUMS.asc" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	ok, err := HasSignature("owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("HasSignature: %v", err)
	}
	if !ok {
		t.Error("esperaba que encuentre la firma")
	}
}

func TestHasSignatureNo(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ok, err := HasSignature("owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("HasSignature: %v", err)
	}
	if ok {
		t.Error("esperaba que no encuentre la firma")
	}
}

// TestHasSignatureTolera404Transitorio verifica que un 404 que se
// resuelve al segundo intento no se reporte como "sin firma".
//
// El CDN de GitHub tarda unos segundos en ver un asset recién
// subido. Sin reintento, el doctor marca SIN FIRMA un release que sí
// está firmado.
func TestHasSignatureTolera404Transitorio(t *testing.T) {
	intentos := 0
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		intentos++
		if intentos < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ok, err := HasSignature("owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("HasSignature: %v", err)
	}
	if !ok {
		t.Error("esperaba que el reintento encontrara la firma")
	}
}

// TestHasSignatureFallaTrasLosReintentos verifica que un 404
// permanente se reporta como "no hay firma" y no como error.
func TestHasSignatureFallaTrasLosReintentos(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ok, _ := HasSignature("owner/repo", "v1.0.0")
	if ok {
		t.Error("esperaba que no encuentre la firma")
	}
}
