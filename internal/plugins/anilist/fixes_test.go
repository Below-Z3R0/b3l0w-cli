package anilist

// Tests de los hallazgos CRÍTICOS y ALTOS de la revisión de integridad.
// Cada uno tiene que FALLAR si se revierte su arreglo:
//
//   - C1: `Sync` devuelve error si las dos listas fallan (antes devolvía una
//     biblioteca vacía con err=nil, y el GC borraba todas las carátulas).
//   - A1: una descarga fallida no vuelve borrable la carátula que ya estaba.
//   - A2: reescribir el meta del destino no toca el `.meta.json` del origen
//     adoptado con `--reuse`.
//   - A3/A4: las dos queries de la agenda paginan hasta agotar.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyncDevuelveErrorSiLasDosListasFallan fija C1: con las dos listas
// caídas, "no pude leer la biblioteca" no puede llegar al caller como una
// biblioteca vacía y sin error.
func TestSyncDevuelveErrorSiLasDosListasFallan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, _ := requestBody(t, r)
		if strings.Contains(query, "Viewer") {
			w.Write([]byte(`{"data":{"Viewer":{"id":42,"name":"b3low"}}}`))
			return
		}
		w.Write([]byte(`{"errors":[{"message":"Too Many Requests."}]}`))
	}))
	defer srv.Close()

	lib, err := Sync(context.Background(), serverClient(t, srv), 0)
	if err == nil {
		t.Fatalf("Sync devolvió err=nil con las dos listas falladas: el GC borraría las carátulas (C1)")
	}
	if lib.Partial {
		t.Errorf("con error no importa Partial; lo que importa es el error")
	}
}

// TestFetchSchedulePaginaLaListaYLosCapitulos fija A3 y A4: la lista de series
// en curso y la query de capítulos tienen que seguir pageInfo hasta agotar.
func TestFetchSchedulePaginaLaListaYLosCapitulos(t *testing.T) {
	const perPage = 50
	const seriesTotal = 119
	const capsTotal = 93

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query, vars := requestBody(t, r)
		page := int(vars["page"].(float64))

		if strings.Contains(query, "mediaList") {
			from := (page - 1) * perPage
			to := from + perPage
			if to > seriesTotal {
				to = seriesTotal
			}
			var entries []string
			for i := from; i < to; i++ {
				entries = append(entries, fmt.Sprintf(
					`{"mediaId":%d,"status":"CURRENT","media":{"id":%d,"title":{"userPreferred":"Serie %d"},"coverImage":{"extraLarge":"http://c/%d.jpg","color":"#abc"},"nextAiringEpisode":{"episode":1,"airingAt":1000}}}`,
					i, i, i, i))
			}
			fmt.Fprintf(w, `{"data":{"Page":{"pageInfo":{"hasNextPage":%t},"mediaList":[%s]}}}`,
				to < seriesTotal, strings.Join(entries, ","))
			return
		}

		if strings.Contains(query, "airingSchedules") {
			from := (page - 1) * perPage
			to := from + perPage
			if to > capsTotal {
				to = capsTotal
			}
			var nodes []string
			for i := from; i < to; i++ {
				nodes = append(nodes, fmt.Sprintf(
					`{"mediaId":%d,"episode":2,"airingAt":2000,"media":{"id":%d,"title":{"userPreferred":"Serie %d"},"coverImage":{"extraLarge":"http://c/%d.jpg","color":"#abc"}}}`,
					i, i, i, i))
			}
			fmt.Fprintf(w, `{"data":{"Page":{"pageInfo":{"hasNextPage":%t},"airingSchedules":[%s]}}}`,
				to < capsTotal, strings.Join(nodes, ","))
			return
		}

		t.Errorf("query inesperada: %.80s", query)
	}))
	defer srv.Close()

	sched, err := FetchSchedule(context.Background(), serverClient(t, srv), 42)
	if err != nil {
		t.Fatalf("FetchSchedule: %v", err)
	}
	if len(sched.Items) != capsTotal {
		t.Fatalf("items = %d; quería %d (la agenda se cortaba en la primera página, A3/A4)", len(sched.Items), capsTotal)
	}
	// Las 119 series tienen que haber entrado en el pedido de capítulos.
	seen := map[int]bool{}
	for _, item := range sched.Items {
		seen[item.MediaID] = true
	}
	if len(seen) != capsTotal {
		t.Errorf("mediaIds distintos = %d; quería %d", len(seen), capsTotal)
	}
}

// TestGCMantieneLaCaratulaSiLaDescargaFalla fija A1: el CDN responde 500, pero
// la carátula que ya estaba en disco no es un huérfano y no se borra.
func TestGCMantieneLaCaratulaSiLaDescargaFalla(t *testing.T) {
	dir := t.TempDir()
	// Una carátula vieja con su meta: la URL cambió, así que hay que bajarla.
	if err := os.WriteFile(filepath.Join(dir, "7.jpg"), []byte("JPEG-VIEJO"), 0o600); err != nil {
		t.Fatal(err)
	}
	metaViejo := `{"url":"http://viejo/7.jpg","version":2}`
	if err := os.WriteFile(filepath.Join(dir, "7.jpg.meta.json"), []byte(metaViejo), 0o600); err != nil {
		t.Fatal(err)
	}

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer cdn.Close()

	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	stats, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{Workers: 1, GC: true})
	if err != nil {
		t.Fatalf("SyncCovers: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d; quería 1", stats.Failed)
	}
	if stats.Removed != 0 {
		t.Errorf("removed = %d; quería 0: la descarga falló, no es un huérfano (A1)", stats.Removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "7.jpg")); err != nil {
		t.Fatalf("el .jpg previo desapareció: %v", err)
	}
}

// TestReuseNoCorrompeElMetaDelOrigen fija A2: adoptar un caché con --reuse no
// puede reescribir el `.meta.json` del directorio de origen.
func TestReuseNoCorrompeElMetaDelOrigen(t *testing.T) {
	reuse := t.TempDir()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(reuse, "7.jpg"), []byte("JPEG-VIEJO"), 0o600); err != nil {
		t.Fatal(err)
	}
	metaOrigen := `{"url":"http://viejo/7.jpg","version":2}`
	if err := os.WriteFile(filepath.Join(reuse, "7.jpg.meta.json"), []byte(metaOrigen), 0o600); err != nil {
		t.Fatal(err)
	}

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("JPEG-NUEVO"))
	}))
	defer cdn.Close()

	entry := Entry{MediaID: 7, Title: "Serie", CoverURL: cdn.URL + "/7.jpg"}
	stats, err := SyncCovers(context.Background(), []Entry{entry}, dir, CoverOptions{
		Workers: 1, ReuseDir: reuse, GC: false,
	})
	if err != nil {
		t.Fatalf("SyncCovers: %v", err)
	}
	if stats.Downloaded != 1 {
		t.Fatalf("downloaded = %d; quería 1 (la URL cambió, hay que bajarla)", stats.Downloaded)
	}

	raw, err := os.ReadFile(filepath.Join(reuse, "7.jpg.meta.json"))
	if err != nil {
		t.Fatalf("leer el meta del origen: %v", err)
	}
	if string(raw) != metaOrigen {
		t.Fatalf("el meta del ORIGEN cambió a %s: el --reuse corrompió el caché (A2)", string(raw))
	}
	var nuevo coverMeta
	rawDest, err := os.ReadFile(filepath.Join(dir, "7.jpg.meta.json"))
	if err != nil {
		t.Fatalf("leer el meta del destino: %v", err)
	}
	if err := json.Unmarshal(rawDest, &nuevo); err != nil {
		t.Fatalf("el meta del destino no es JSON: %v", err)
	}
	if nuevo.URL != entry.CoverURL {
		t.Errorf("meta del destino = %q; quería la URL nueva %q", nuevo.URL, entry.CoverURL)
	}
}
