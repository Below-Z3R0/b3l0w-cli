package registry

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// preparar aísla el estado y apunta la red a un servidor de prueba.
func preparar(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	dir := t.TempDir()
	viejoDir, viejoURL := stateDir, baseURL
	stateDir = func() string { return dir }
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		baseURL = srv.URL
	} else {
		baseURL = "http://127.0.0.1:1" // puerto cerrado: sin red
	}
	t.Cleanup(func() { stateDir, baseURL = viejoDir, viejoURL })
	return dir
}

func TestLoadUsaElEmbebidoSinRed(t *testing.T) {
	preparar(t, nil)
	embedded := []byte(`{"version":1}`)

	data, src, err := Load(Services, embedded)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if src != FromEmbed {
		t.Errorf("source = %v, quería embebido", src)
	}
	if string(data) != string(embedded) {
		t.Errorf("data = %s", data)
	}
}

func TestLoadBajaYCachea(t *testing.T) {
	pedidos := 0
	dir := preparar(t, func(w http.ResponseWriter, r *http.Request) {
		pedidos++
		if r.URL.Path != "/services/services.json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "b3l0w" {
			t.Error("falta User-Agent")
		}
		w.Write([]byte(`{"version":1,"services":[]}`))
	})

	data, src, err := Load(Services, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if src != FromRemote {
		t.Errorf("source = %v, quería red", src)
	}
	if pedidos != 1 {
		t.Errorf("pedidos = %d", pedidos)
	}

	// La segunda lectura sale del caché: cero red.
	if _, src, _ := Load(Services, nil); src != FromCache {
		t.Errorf("segunda lectura: source = %v, quería caché", src)
	}
	if pedidos != 1 {
		t.Errorf("pedidos = %d tras el caché, quería 1", pedidos)
	}

	// El caché quedó escrito.
	if _, err := os.Stat(filepath.Join(dir, "services.json")); err != nil {
		t.Errorf("no se escribió el caché: %v", err)
	}
	if string(data) == "" {
		t.Error("data vacía")
	}
}

func TestLoadUsaCacheViejoSiLaRedFalla(t *testing.T) {
	dir := preparar(t, nil)
	path := filepath.Join(dir, "services.json")
	if err := os.WriteFile(path, []byte(`{"viejo":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Envejecer el archivo para que no sea "fresco".
	old := time.Now().Add(-2 * TTL)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	data, src, err := Load(Services, []byte(`{"embed":true}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if src != FromStaleCache {
		t.Errorf("source = %v, quería caché viejo", src)
	}
	if string(data) != `{"viejo":true}` {
		t.Errorf("data = %s", data)
	}
}

func TestLoadSinNadaEsError(t *testing.T) {
	preparar(t, nil)
	if _, _, err := Load(Plugins, nil); err == nil {
		t.Fatal("esperaba error sin caché, red ni embebido")
	}
}

func TestSyncIgnoraElTTL(t *testing.T) {
	pedidos := 0
	dir := preparar(t, func(w http.ResponseWriter, r *http.Request) {
		pedidos++
		w.Write([]byte(`{"version":2}`))
	})

	// Cache fresco que Sync tiene que pisar igual.
	path := filepath.Join(dir, "services.json")
	os.WriteFile(path, []byte(`{"version":1}`), 0o644)

	st, err := Sync(Services, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if st.Source != FromRemote || pedidos != 1 {
		t.Errorf("source = %v, pedidos = %d", st.Source, pedidos)
	}

	data, _ := os.ReadFile(path)
	if string(data) != `{"version":2}` {
		t.Errorf("caché = %s, quería la nueva", data)
	}
}

func TestSyncConRedCaidaAvisaYUsaCache(t *testing.T) {
	dir := preparar(t, nil)
	path := filepath.Join(dir, "plugins.json")
	os.WriteFile(path, []byte(`{"local":true}`), 0o644)

	st, err := Sync(Plugins, nil)
	if err == nil {
		t.Fatal("esperaba error de red")
	}
	if st.Source != FromStaleCache {
		t.Errorf("source = %v, quería caché viejo", st.Source)
	}
}

func TestStat(t *testing.T) {
	dir := preparar(t, nil)
	if st := Stat(System); st.Source != FromEmbed {
		t.Errorf("sin archivo: source = %v", st.Source)
	}
	os.WriteFile(filepath.Join(dir, "system.json"), []byte(`{"x":1}`), 0o644)
	if st := Stat(System); st.Source != FromCache || st.Size != 7 {
		t.Errorf("stat = %+v", st)
	}
}
