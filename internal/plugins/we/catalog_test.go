package we

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFilterConUnTipoQueNoExisteNoDevuelveElCatalogoEntero(t *testing.T) {
	items := []Item{
		{ID: "1", Title: "Rainy Day", Type: "video"},
		{ID: "2", Title: "Bosque", Type: "scene"},
		{ID: "3", Title: "Sin tipo", Type: "unknown"},
	}
	// Un typo no puede devolver el catálogo entero: no matchea con nada.
	if got := Filter(items, "videoz", ""); len(got) != 0 {
		t.Errorf("Filter con un tipo inexistente = %d items, quería 0", len(got))
	}
	// "unknown" es un tipo legítimo: filtra por los unknown, no por todo.
	got := Filter(items, "unknown", "")
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("Filter(unknown) = %+v, quería sólo el item 3", got)
	}
	// Los tipos de verdad siguen filtrando.
	if got := Filter(items, "video", ""); len(got) != 1 || got[0].ID != "1" {
		t.Errorf("Filter(video) = %+v, quería sólo el item 1", got)
	}
	// Sin tipo no se filtra por tipo, sólo por query.
	if got := Filter(items, "", "bosque"); len(got) != 1 || got[0].ID != "2" {
		t.Errorf("Filter sin tipo = %+v, quería sólo el item 2", got)
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

func TestSlugConservaLasLetrasAcentuadas(t *testing.T) {
	// Las letras acentuadas no se tiran: se transliteran.
	casos := map[string]string{
		"Café Münster Ñandú": "cafe-munster-nandu",
		"Piñata":              "pinata",
		"Über":                "uber",
	}
	for in, want := range casos {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestScanRechazaUnFileQueSaleDelItem(t *testing.T) {
	// El project.json lo baja Steam: el `file` no lo escribió el usuario y
	// puede traer `../` o una ruta absoluta.
	root := t.TempDir()
	writeItem(t, root, "111", `{"title":"T","type":"Video","file":"../../etc/passwd"}`)
	writeItem(t, root, "222", `{"title":"T","type":"Video","file":"/etc/passwd"}`)
	writeItem(t, root, "333", `{"title":"T","type":"Video","file":"../secret.mp4"}`)
	writeItem(t, root, "444", `{"title":"T","type":"Video","preview":"../../preview.png"}`)
	// El archivo real que un project.json malicioso intentaría copiar.
	escribir := func(path, contenido string) {
		if err := os.WriteFile(path, []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir(filepath.Join(root, "secret.mp4"), "CLAVE-DE-PRUEBA")

	items, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, it := range items {
		if it.File != "" {
			t.Errorf("item %s: File = %q, quería vacío (la ruta se sale del item)", it.ID, it.File)
		}
		if it.Preview != "" {
			t.Errorf("item %s: Preview = %q, quería vacío (la ruta se sale del item)", it.ID, it.Preview)
		}
		if it.Err == "" {
			t.Errorf("item %s: quería Err cargado (ruta fuera del item)", it.ID)
		}
	}
}

func TestScanAceptaUnFileDentroDelItem(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "111", `{"title":"T","type":"Video","file":"video.mp4","preview":"previews/p.jpg"}`)

	items, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, quería 1", len(items))
	}
	it := items[0]
	if it.Err != "" {
		t.Errorf("un file legítimo no debería dar Err: %q", it.Err)
	}
	if filepath.Base(it.File) != "video.mp4" {
		t.Errorf("File = %q, quería video.mp4", it.File)
	}
	if filepath.Base(it.Preview) != "p.jpg" {
		t.Errorf("Preview = %q, quería p.jpg", it.Preview)
	}
	// Un file en una subcarpeta del item también vale.
	if !strings.HasPrefix(it.Preview, filepath.Join(root, "111")) {
		t.Errorf("Preview = %q, tenía que quedar dentro del item", it.Preview)
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
