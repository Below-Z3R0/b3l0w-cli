package anilist

import "testing"

// TestReadLibraryFile... el JSON lo escribe el plugin de Luau: una tabla
// vacía se serializa como {} y no como [], así que el parser tiene que
// tolerarlo.
func TestReadLibraryFileToleraListasVacias(t *testing.T) {
	raw := []byte(`{"anime":{},"manga":[],"viewer":{"id":3,"name":"b3low"},"fetchedAt":42}`)
	lib, err := ReadLibraryFile(raw)
	if err != nil {
		t.Fatalf("ReadLibraryFile: %v", err)
	}
	if lib.Anime == nil || len(lib.Anime) != 0 {
		t.Errorf("anime = %+v; quería una lista vacía (no nil)", lib.Anime)
	}
	if lib.Manga == nil || len(lib.Manga) != 0 {
		t.Errorf("manga = %+v; quería una lista vacía (no nil)", lib.Manga)
	}
	if lib.Viewer.ID != 3 || lib.Viewer.Name != "b3low" {
		t.Errorf("viewer = %+v; quería b3low #3", lib.Viewer)
	}
	if lib.FetchedAt != 42 {
		t.Errorf("fetchedAt = %d; quería 42", lib.FetchedAt)
	}

	conEntradas := []byte(`{"anime":[{"mediaId":7,"title":"Frieren","progress":12}]}`)
	lib2, err := ReadLibraryFile(conEntradas)
	if err != nil {
		t.Fatalf("ReadLibraryFile con entradas: %v", err)
	}
	if len(lib2.Anime) != 1 || lib2.Anime[0].MediaID != 7 || lib2.Anime[0].Title != "Frieren" {
		t.Errorf("anime = %+v; quería la entrada 7", lib2.Anime)
	}
	if lib2.Manga == nil || len(lib2.Manga) != 0 {
		t.Errorf("manga ausente = %+v; quería una lista vacía (no nil)", lib2.Manga)
	}

	if _, err := ReadLibraryFile([]byte("{roto")); err == nil {
		t.Error("un JSON inválido tiene que ser error")
	}
}
