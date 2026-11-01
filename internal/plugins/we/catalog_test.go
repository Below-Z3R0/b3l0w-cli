package we

import (
	"os"
	"path/filepath"
	"testing"
)

func writeItem(t *testing.T, root, id, project string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanNormalizaYOrdena(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "111", `{"title":"Zeta","type":"Scene","file":"a.mp4","preview":"p.jpg"}`)
	writeItem(t, root, "222", `{"title":"alfa","type":"Video","file":"b.mp4"}`)
	writeItem(t, root, "333", `{ roto`)
	writeItem(t, root, "no-numerico", `{"title":"ignorado","type":"video"}`)

	items, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("esperaba 3 items, obtuve %d", len(items))
	}
	alpha, err := Find(items, "222")
	if err != nil || alpha.Type != "video" {
		t.Errorf("222: %+v (%v)", alpha, err)
	}
	if filepath.Base(alpha.File) != "b.mp4" {
		t.Errorf("File mal resuelto: %q", alpha.File)
	}
	zeta, err := Find(items, "111")
	if err != nil || zeta.Type != "scene" {
		t.Errorf("111: %+v (%v)", zeta, err)
	}
	if filepath.Base(zeta.Preview) != "p.jpg" {
		t.Errorf("Preview mal resuelto: %q", zeta.Preview)
	}
	broken, err := Find(items, "333")
	if err != nil || broken.Err == "" {
		t.Errorf("el item roto debería traer Err: %+v", broken)
	}
	// Los dos válidos quedan en orden alfabético entre sí.
	if !(indexOf(items, "222") < indexOf(items, "111")) {
		t.Errorf("no quedó ordenado: %+v", titles(items))
	}
}

func TestFilterYFind(t *testing.T) {
	items := []Item{{ID: "1", Title: "Rainy Day", Type: "video"}, {ID: "2", Title: "Bosque", Type: "scene"}}
	if got := Filter(items, "scene", ""); len(got) != 1 || got[0].ID != "2" {
		t.Errorf("Filter por tipo: %+v", got)
	}
	if got := Filter(items, "", "rainy"); len(got) != 1 || got[0].ID != "1" {
		t.Errorf("Filter por query: %+v", got)
	}
	if _, err := Find(items, "2"); err != nil {
		t.Errorf("Find: %v", err)
	}
	if _, err := Find(items, "9"); err == nil {
		t.Error("Find de un id inexistente debería fallar")
	}
}

func TestSlugYOutputName(t *testing.T) {
	cases := map[string]string{
		"Campfire PC":                    "campfire-pc",
		"  The Winter Nothingness [4K] ": "the-winter-nothingness-4k",
		"":                               "wallpaper",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, quería %q", in, got, want)
		}
	}
	it := Item{ID: "42", Title: "Rainy Day"}
	if got := OutputName(it, "mp4"); got != "42-rainy-day.mp4" {
		t.Errorf("OutputName = %q", got)
	}
}

func indexOf(items []Item, id string) int {
	for i, it := range items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func titles(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}
