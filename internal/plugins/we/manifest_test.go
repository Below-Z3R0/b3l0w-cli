package we

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleEntry(id, status string) Entry {
	return Entry{
		ID:       id,
		Title:    "Campfire PC",
		Type:     "video",
		Status:   status,
		File:     "/out/" + id + ".mp4",
		Source:   "/in/Campfire PC.mp4",
		Method:   MethodCopy,
		Bytes:    12345678,
		Duration: 35.2,
		Width:    3840,
		Height:   2160,
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "manifest.json")

	in := Manifest{
		Version:   ManifestVersion,
		Generated: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		Root:      "/root/431960",
		Out:       "/out",
		Items:     []Entry{sampleEntry("1", StatusOK), sampleEntry("2", StatusSkipped)},
	}
	if err := SaveManifest(path, in); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	out, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if out.Version != ManifestVersion {
		t.Errorf("Version = %d, quería %d", out.Version, ManifestVersion)
	}
	if !out.Generated.Equal(in.Generated) {
		t.Errorf("Generated = %v, quería %v", out.Generated, in.Generated)
	}
	if out.Root != in.Root || out.Out != in.Out {
		t.Errorf("root/out = %q/%q, quería %q/%q", out.Root, out.Out, in.Root, in.Out)
	}
	if len(out.Items) != 2 {
		t.Fatalf("esperaba 2 items, obtuve %d", len(out.Items))
	}
	got := out.Items[0]
	if got != in.Items[0] {
		t.Errorf("el entry no sobrevivió el round-trip:\ngot  %+v\nwant %+v", got, in.Items[0])
	}

	// Los tags JSON son el contrato con el plugin: si cambian, el plugin rompe.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"version"`, `"generated"`, `"root"`, `"out"`, `"items"`,
		`"id"`, `"title"`, `"type"`, `"status"`, `"file"`, `"source"`, `"method"`,
		`"bytes"`, `"duration"`, `"width"`, `"height"`, `"error"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("falta la clave %s en el JSON", key)
		}
	}
	if strings.Contains(string(raw), `"Items"`) {
		t.Error("el JSON se está serializando con nombres Go en vez de los tags")
	}
}

func TestManifestUpsertReemplazaYAgrega(t *testing.T) {
	var m Manifest
	Upsert(&m, sampleEntry("1", StatusOK))
	Upsert(&m, sampleEntry("2", StatusFailed))
	if len(m.Items) != 2 {
		t.Fatalf("esperaba 2 items, obtuve %d", len(m.Items))
	}

	replaced := sampleEntry("1", StatusFailed)
	replaced.Err = "ffmpeg explotó"
	Upsert(&m, replaced)
	if len(m.Items) != 2 {
		t.Fatalf("Upsert agregó en vez de reemplazar: %d items", len(m.Items))
	}
	if m.Items[0].ID != "1" || m.Items[0].Status != StatusFailed || m.Items[0].Err == "" {
		t.Errorf("no reemplazó el entry 1: %+v", m.Items[0])
	}
	if m.Items[1].ID != "2" {
		t.Errorf("cambió el orden de los items: %+v", m.Items)
	}

	// nil-safe: no debe hacer panic.
	Upsert(nil, sampleEntry("3", StatusOK))
}

func TestManifestSaveEsAtomico(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	first := Manifest{Version: ManifestVersion, Out: dir, Items: []Entry{sampleEntry("1", StatusOK)}}
	if err := SaveManifest(path, first); err != nil {
		t.Fatalf("primer SaveManifest: %v", err)
	}
	second := Manifest{Version: ManifestVersion, Out: dir, Items: []Entry{sampleEntry("1", StatusFailed), sampleEntry("2", StatusOK)}}
	if err := SaveManifest(path, second); err != nil {
		t.Fatalf("segundo SaveManifest: %v", err)
	}

	// Después de cada escritura queda un único manifest válido y ningún temporal.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "manifest.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("quedaron archivos sueltos: %v", names)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o644 {
		t.Errorf("permisos = %v, quería 0644", perm)
	}

	out, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest tras el segundo guardado: %v", err)
	}
	if len(out.Items) != 2 || out.Items[0].Status != StatusFailed {
		t.Errorf("el archivo quedó con contenido viejo: %+v", out.Items)
	}
}

func TestManifestLoadErrores(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-existe.json")
	if _, err := LoadManifest(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("un manifest inexistente debería envolver os.ErrNotExist, obtuve %v", err)
	}

	empty := filepath.Join(dir, "vacio.json")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(empty); err == nil {
		t.Error("un manifest vacío debería fallar")
	}

	broken := filepath.Join(dir, "roto.json")
	if err := os.WriteFile(broken, []byte(`{"version":1,`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(broken); err == nil {
		t.Error("un manifest truncado debería fallar")
	}
}

func TestManifestSaveSinItemsSerializaArrayVacio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := SaveManifest(path, Manifest{}); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"items": []`) {
		t.Errorf("items debería serializarse como [] y no null:\n%s", raw)
	}
	out, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if out.Version != ManifestVersion {
		t.Errorf("Version = %d, quería el default %d", out.Version, ManifestVersion)
	}
}
