package fetch

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const contenido = "#!/bin/sh\necho v1.0.0\n"

// TestToTempBajaElArchivo comprueba el camino feliz: el asset queda en un
// temporal ejecutable con el contenido servido.
func TestToTempBajaElArchivo(t *testing.T) {
	var vistoCacheBuster bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// El cache-buster es la razón de que el CDN no sirva el asset
		// anterior justo después de publicar.
		if r.URL.Query().Get("cb") != "" {
			vistoCacheBuster = true
		}
		w.Write([]byte(contenido))
	}))
	defer srv.Close()

	path, err := ToTemp(srv.URL + "/asset")
	if err != nil {
		t.Fatalf("ToTemp: %v", err)
	}
	defer os.Remove(path)

	if !vistoCacheBuster {
		t.Error("no llegó el parámetro cb")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if string(data) != contenido {
		t.Errorf("contenido = %q, quería %q", data, contenido)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Errorf("permisos = %o, quería 0755", perm)
	}
}

// TestToTempFallaConHTTPNoOK cubre el 404: curl -f falla y no queda
// basura en /tmp.
func TestToTempFallaConHTTPNoOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	path, err := ToTemp(srv.URL + "/no-existe")
	if err == nil {
		os.Remove(path)
		t.Fatal("esperaba error con HTTP 404")
	}
	if path != "" {
		t.Errorf("path = %q, quería vacío", path)
	}
	if !strings.Contains(err.Error(), "no se pudo bajar") {
		t.Errorf("error = %q, quería que mencione la descarga", err)
	}
}

// TestToTempFallaConServidorCaido cubre el error de conexión.
func TestToTempFallaConServidorCaido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // el puerto queda cerrado

	path, err := ToTemp(url + "/asset")
	if err == nil {
		os.Remove(path)
		t.Fatal("esperaba error con el servidor caído")
	}
}

// TestFirstLine prefiere la primera línea con contenido y cae al error
// cuando no hay salida.
func TestFirstLine(t *testing.T) {
	casos := []struct {
		nombre string
		out    string
		err    error
		quiere string
	}{
		{"una línea", "curl: (6) no resolve host", nil, "curl: (6) no resolve host"},
		{"varias líneas", "linea1\nlinea2\nlinea3", nil, "linea1"},
		{"líneas vacías al final", "linea1\n\n\n", nil, "linea1"},
		{"sin salida ni error", "  \n ", nil, "sin salida"},
		{"sin salida con error", "", os.ErrNotExist, os.ErrNotExist.Error()},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := firstLine(c.out, c.err); got != c.quiere {
				t.Errorf("firstLine = %q, quería %q", got, c.quiere)
			}
		})
	}
}
