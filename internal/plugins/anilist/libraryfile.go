package anilist

import "encoding/json"

// libraryFile es la biblioteca que escribe el plugin en disco para que la fase
// de carátulas no repita el fetch.
//
// Se decodifica a mano porque el JSON viene de Luau: una tabla Lua vacía se
// serializa como `{}` y no como `[]`, y `json.Unmarshal` a `[]Entry` explota
// con "cannot unmarshal object into Go struct field".
type libraryFile struct {
	Viewer    Viewer  `json:"viewer"`
	Anime     []Entry `json:"anime"`
	Manga     []Entry `json:"manga"`
	FetchedAt int64   `json:"fetchedAt"`
}

// ReadLibraryFile parsea el archivo tolerando `{}` como lista vacía.
func ReadLibraryFile(raw []byte) (Library, error) {
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(raw, &campos); err != nil {
		return Library{}, err
	}
	out := Library{Anime: []Entry{}, Manga: []Entry{}}
	if value, ok := campos["anime"]; ok {
		out.Anime = decodeEntries(value)
	}
	if value, ok := campos["manga"]; ok {
		out.Manga = decodeEntries(value)
	}
	if value, ok := campos["viewer"]; ok {
		json.Unmarshal(value, &out.Viewer)
	}
	if value, ok := campos["fetchedAt"]; ok {
		json.Unmarshal(value, &out.FetchedAt)
	}
	return out, nil
}

func decodeEntries(raw json.RawMessage) []Entry {
	trim := string(raw)
	if trim == "" || trim == "null" || trim == "{}" {
		return []Entry{}
	}
	var lista []Entry
	if err := json.Unmarshal(raw, &lista); err != nil || lista == nil {
		return []Entry{}
	}
	return lista
}
