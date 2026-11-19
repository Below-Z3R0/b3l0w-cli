package anilistcli

// Test del contrato de C1 en la capa que lo sufre: `sync --gc` con una lista
// caída no puede borrar nada y tiene que salir con código != 0.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncConGcNoBorraSiLaBibliotecaEstaIncompleta(t *testing.T) {
	falso := instalarAniListFalso(t)
	// Sólo falla MANGA: ANIME viene bien y la biblioteca queda parcial, que es
	// exactamente el caso donde el GC antes borraba todo lo que no estuviera
	// en la lista vacía.
	falso.responder = func(query string, variables map[string]any) (string, int) {
		if strings.Contains(query, "MediaListCollection(userId:") && variables["type"] == "MANGA" {
			return `{"errors":[{"message":"Not Found."}]}`, 0
		}
		return responderPorDefecto(query, variables)
	}
	sinTokenDeEntorno(t)
	dir := t.TempDir()
	escribirToken(t, dir)

	// Una carátula que ya estaba en disco. Sin la lista de manga no hay forma
	// de saber si es huérfana: se conserva.
	cubiertas := filepath.Join(dir, "covers")
	if err := os.MkdirAll(cubiertas, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(cubiertas, "999.jpg"), "JPEG-bueno")
	escribir(t, filepath.Join(cubiertas, "999.jpg.meta.json"), `{"url":"https://cdn.test/999.jpg","version":2}`)

	codigo, salida, errores := capturar(t, func() int {
		return Run([]string{"sync", "--json", "--data-dir", dir, "--gc"})
	})
	if codigo == 0 {
		t.Fatalf("código = 0; quería != 0: pidió --gc y no se pudo hacer, el caller tiene que enterarse (C1)")
	}
	doc := afirmarUnSoloJSON(t, salida)
	if doc["partial"] != true {
		t.Errorf("partial = %v; quería true (manga falló)", doc["partial"])
	}
	if _, err := os.Stat(filepath.Join(cubiertas, "999.jpg")); err != nil {
		t.Fatalf("borró una carátula con la biblioteca incompleta (C1): %v", err)
	}
	if !strings.Contains(errores, "no se borró ninguna carátula") {
		t.Errorf("stderr = %q; quería el aviso de que no se borró nada", errores)
	}
}
