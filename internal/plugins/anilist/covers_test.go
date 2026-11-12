package anilist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSyncCovers... cubre el ciclo completo del caché de carátulas: descarga,
// reuso por meta, GC de huérfanas y conteo de fallos.
func TestSyncCoversDescargaReutilizaYBorraHuerfanos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok.jpg" {
			w.Write([]byte("contenido-de-prueba"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	entry := Entry{MediaID: 7, CoverURL: srv.URL + "/ok.jpg"}

	stats, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{Workers: 2})
	if err != nil {
		t.Fatalf("SyncCovers: %v", err)
	}
	if stats.Downloaded != 1 || stats.Failed != 0 {
		t.Errorf("descargas = %d, fallos = %d; quería 1 y 0", stats.Downloaded, stats.Failed)
	}
	path, ok := stats.Covers["7"]
	if !ok {
		t.Fatalf("el mapa no tiene la entrada 7: %+v", stats.Covers)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no quedó el JPG: %v", err)
	}
	raw, err := os.ReadFile(path + ".meta.json")
	if err != nil {
		t.Fatalf("no quedó el meta: %v", err)
	}
	var meta coverMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("meta ilegible: %v", err)
	}
	if meta.Version != CoverVersion || meta.URL != entry.CoverURL {
		t.Errorf("meta = %+v; quería url %q y versión %d", meta, entry.CoverURL, CoverVersion)
	}

	// Segunda pasada: el meta coincide, no se vuelve a bajar nada.
	again, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{})
	if err != nil {
		t.Fatalf("segundo SyncCovers: %v", err)
	}
	if again.Downloaded != 0 {
		t.Errorf("descargó de nuevo %d carátulas que ya eran válidas", again.Downloaded)
	}
	if len(again.Covers) != 1 {
		t.Errorf("el mapa perdió la entrada válida: %+v", again.Covers)
	}

	// Con GC se borra lo que no está en la lista.
	huerfano := filepath.Join(dir, "999.jpg")
	if err := os.WriteFile(huerfano, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gc, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{GC: true})
	if err != nil {
		t.Fatalf("SyncCovers con GC: %v", err)
	}
	if gc.Removed != 1 {
		t.Errorf("borradas = %d; quería 1", gc.Removed)
	}
	if _, err := os.Stat(huerfano); !os.IsNotExist(err) {
		t.Errorf("el huérfano quedó: %v", err)
	}

	// Una 404 es un fallo, no una entrada.
	missing := Entry{MediaID: 8, CoverURL: srv.URL + "/no-esta.jpg"}
	bad, err := SyncCovers(context.Background(), []Entry{missing}, dir, CoverOptions{})
	if err != nil {
		t.Fatalf("SyncCovers con 404: %v", err)
	}
	if bad.Failed != 1 {
		t.Errorf("fallos = %d; quería 1", bad.Failed)
	}
	if _, ok := bad.Covers["8"]; ok {
		t.Error("una carátula fallida no puede estar en el mapa")
	}
}
