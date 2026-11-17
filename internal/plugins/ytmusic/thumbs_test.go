package ytmusic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// portadaFalsa devuelve bytes de imagen. El código descarta los archivos de
// menos de 512 bytes: un 404 con cuerpo de error no es una portada.
func portadaFalsa() []byte {
	return []byte(strings.Repeat("JPEG-DATA-", 100))
}

// TestThumbsBajaLaPortadaYLaDejaLocal: ui.image no acepta URLs, así que la
// portada tiene que estar en disco.
func TestThumbsBajaLaPortadaYLaDejaLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(portadaFalsa())
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{{VideoID: "abc", ThumbURL: server.URL + "/abc.jpg"}}
	got := Thumbs(context.Background(), dir, songs, 10)
	if got[0].Thumb == "" {
		t.Fatal("la portada no quedó en disco")
	}
	want := filepath.Join(dir, "abc.jpg")
	if got[0].Thumb != want {
		t.Errorf("thumb = %q, quería %q", got[0].Thumb, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("no se escribió la portada: %v", err)
	}
	if info.Size() == 0 {
		t.Error("la portada está vacía")
	}
}

// TestThumbsRespetaElLimite: no se bajan 50 portadas por pantalla.
func TestThumbsRespetaElLimite(t *testing.T) {
	var pedidas atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas.Add(1)
		_, _ = w.Write(portadaFalsa())
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{
		{VideoID: "a", ThumbURL: server.URL + "/a.jpg"},
		{VideoID: "b", ThumbURL: server.URL + "/b.jpg"},
		{VideoID: "c", ThumbURL: server.URL + "/c.jpg"},
	}
	Thumbs(context.Background(), dir, songs, 2)
	if got := pedidas.Load(); got != 2 {
		t.Errorf("pidió %d portadas, quería 2", got)
	}
	if songs[2].Thumb != "" {
		t.Errorf("bajó la tercera: %q", songs[2].Thumb)
	}
}

// TestThumbsCacheaPorVideoID: la segunda visita a Charts no baja nada.
func TestThumbsCacheaPorVideoID(t *testing.T) {
	var pedidas atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas.Add(1)
		_, _ = w.Write(portadaFalsa())
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{{VideoID: "abc", ThumbURL: server.URL + "/abc.jpg"}}
	Thumbs(context.Background(), dir, songs, 10)
	Thumbs(context.Background(), dir, songs, 10)
	if got := pedidas.Load(); got != 1 {
		t.Errorf("pidió %d veces, quería 1 (la segunda va al caché)", got)
	}
	if songs[0].Thumb == "" {
		t.Error("la portada cacheada no quedó apuntada")
	}
}

// TestThumbsConPuertoIguvenido: un 404 deja la canción sin portada pero sin
// romper la lista.
func TestThumbsConPuertoIguvenido(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{{VideoID: "abc", ThumbURL: server.URL + "/abc.jpg"}}
	got := Thumbs(context.Background(), dir, songs, 10)
	if got[0].Thumb != "" {
		t.Errorf("apuntó a una portada inexistente: %q", got[0].Thumb)
	}
	if _, err := os.Stat(filepath.Join(dir, "abc.jpg")); !os.IsNotExist(err) {
		t.Errorf("dejó el archivo roto: %v", err)
	}
}

// TestThumbsIgnoraLaPortadaChica: un cuerpo de menos de 512 bytes es un error
// de la red, no una imagen.
func TestThumbsIgnoraLaPortadaChica(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("chico"))
	}))
	defer server.Close()

	dir := t.TempDir()
	songs := []Song{{VideoID: "abc", ThumbURL: server.URL + "/abc.jpg"}}
	got := Thumbs(context.Background(), dir, songs, 10)
	if got[0].Thumb != "" {
		t.Errorf("apuntó a una portada demasiado chica: %q", got[0].Thumb)
	}
	// El temporal se limpia: no quedan .part por ahí.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".part") {
			t.Errorf("dejó el temporal %q", entry.Name())
		}
	}
}

// TestThumbsSinURLNiVideoIDNoHaceNada: sin dirección no hay a quién pedirle
// la portada.
func TestThumbsSinURLNiVideoIDNoHaceNada(t *testing.T) {
	dir := t.TempDir()
	songs := []Song{{VideoID: ""}}
	got := Thumbs(context.Background(), dir, songs, 10)
	if got[0].Thumb != "" {
		t.Errorf("se inventó una portada: %q", got[0].Thumb)
	}
}

// TestThumbsSinDirectorioNoHaceNada: sin destino no hay dónde guardar.
func TestThumbsSinDirectorioNoHaceNada(t *testing.T) {
	songs := []Song{{VideoID: "abc", ThumbURL: "https://example.test/abc.jpg"}}
	got := Thumbs(context.Background(), "", songs, 10)
	if got[0].Thumb != "" {
		t.Errorf("apuntó a una portada sin destino: %q", got[0].Thumb)
	}
}

// TestThumbsConListaVacia.
func TestThumbsConListaVacia(t *testing.T) {
	if got := Thumbs(context.Background(), t.TempDir(), nil, 10); len(got) != 0 {
		t.Errorf("devolvió %d canciones, quería 0", len(got))
	}
}
