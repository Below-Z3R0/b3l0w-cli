package ytmusic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDataDirUsaElDirectorioExplicito: el flag gana al state dir.
func TestDataDirUsaElDirectorioExplicito(t *testing.T) {
	if got := DataDir("/tmp/explicito"); got != "/tmp/explicito" {
		t.Errorf("DataDir = %q", got)
	}
}

// TestDataDirRespetaElStateHomeDeNoctalia: mismo criterio que el resto de
// b3l0w.
func TestDataDirRespetaElStateHomeDeNoctalia(t *testing.T) {
	t.Setenv("NOCTALIA_STATE_HOME", "/state/prueba")
	got := DataDir("")
	want := filepath.Join("/state/prueba", "noctalia", "plugins", "data", "b3l0w", "ytmusic")
	if got != want {
		t.Errorf("DataDir = %q, quería %q", got, want)
	}
}

// TestDataDirCaeAlStateLocal: sin variable de entorno, el path estándar.
func TestDataDirCaeAlStateLocal(t *testing.T) {
	t.Setenv("NOCTALIA_STATE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("sin home: %v", err)
	}
	want := filepath.Join(home, ".local", "state", "noctalia", "plugins", "data", "b3l0w", "ytmusic")
	if got := DataDir(""); got != want {
		t.Errorf("DataDir = %q, quería %q", got, want)
	}
}

// TestEnsureDirCreaConPermisosDeDueño: el socket IPC de mpv vive acá, no
// puede quedar legible por otros.
func TestEnsureDirCreaConPermisosDeDueño(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ytmusic")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("no se creó el directorio")
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("permisos = %v, quería 0700", info.Mode().Perm())
	}
}

// TestEnsureDirConDirectorioVacioEsError.
func TestEnsureDirConDirectorioVacioEsError(t *testing.T) {
	if err := EnsureDir(""); err == nil {
		t.Fatal("un directorio vacío tiene que ser error")
	}
}

// TestWriteJSONEsAtomico: un `next` puede leer la sesión mientras otra
// llamada la reescribe.
func TestWriteJSONEsAtomico(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "estado.json")
	if err := writeJSON(path, map[string]string{"clave": "valor"}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if !strings.Contains(string(raw), `"clave":"valor"`) {
		t.Errorf("contenido = %s", raw)
	}
	// No queda ningún temporal por ahí.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("dejó el temporal %q", entry.Name())
		}
	}
}

// TestReadJSONDetectaElArchivoVacio: un JSON truncado por un corte de luz
// tiene que ser error, no un estado vacío.
func TestReadJSONDetectaElArchivoVacio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vacio.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	err := readJSON(path, &out)
	if err == nil {
		t.Fatal("un archivo vacío tiene que ser error")
	}
	if !strings.Contains(err.Error(), "vacío") {
		t.Errorf("el error no dice que está vacío: %v", err)
	}
}

// TestReadJSONDetectaElJSONRoto.
func TestReadJSONDetectaElJSONRoto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roto.json")
	if err := os.WriteFile(path, []byte(`{"a":`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := readJSON(path, &out); err == nil {
		t.Fatal("un JSON roto tiene que ser error")
	}
}

// TestReadJSONConArchivoInexistente.
func TestReadJSONConArchivoInexistente(t *testing.T) {
	var out map[string]any
	if err := readJSON(filepath.Join(t.TempDir(), "nope.json"), &out); err == nil {
		t.Fatal("un archivo inexistente tiene que ser error")
	}
}

// TestSongDedupConservaElOrden: las radios devuelven el seed al principio y
// otra vez más abajo.
func TestSongDedupConservaElOrden(t *testing.T) {
	songs := songDedup([]Song{
		{VideoID: "a", Title: "A"},
		{VideoID: "b", Title: "B"},
		{VideoID: "a", Title: "A repetida"},
	})
	if len(songs) != 2 {
		t.Fatalf("quedaron %d canciones, quería 2", len(songs))
	}
	if songs[0].VideoID != "a" || songs[1].VideoID != "b" {
		t.Errorf("orden = %+v", songs)
	}
	if songs[0].Title != "A" {
		t.Errorf("el dedupe no debe reemplazar la primera: %+v", songs[0])
	}
}

// TestSongDedupConListaVacia: no hay panic con nil.
func TestSongDedupConListaVacia(t *testing.T) {
	if got := songDedup(nil); len(got) != 0 {
		t.Errorf("quedaron %d canciones, quería 0", len(got))
	}
}
