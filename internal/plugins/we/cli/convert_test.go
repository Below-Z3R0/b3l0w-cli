package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func TestConvertSacaElVideoYEscribeElManifiesto(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, salida, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	manifiesto := strings.TrimSpace(salida)
	if filepath.Base(manifiesto) != "manifest.json" {
		t.Errorf("stdout = %q, quería la ruta del manifiesto", salida)
	}
	// El video extraído queda con el nombre del item.
	esperado := filepath.Join(out, "111-alfa.mp4")
	if _, err := os.Stat(esperado); err != nil {
		t.Fatalf("no escribió %s: %v", esperado, err)
	}
	contenido, err := os.ReadFile(esperado)
	if err != nil {
		t.Fatal(err)
	}
	if string(contenido) != "video falso" {
		t.Errorf("el video copiado = %q, quería el contenido original", contenido)
	}

	var m we.Manifest
	datos, err := os.ReadFile(manifiesto)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("el manifiesto no es JSON: %v", err)
	}
	if len(m.Items) != 1 || m.Items[0].ID != "111" || m.Items[0].Status != "ok" {
		t.Errorf("manifiesto = %+v, quería el item 111 con status ok", m.Items)
	}
	if m.Items[0].Method != we.MethodCopy {
		t.Errorf("method = %q, quería copy (crf 0 y origen mp4)", m.Items[0].Method)
	}
}

func TestConvertEnJSONEmiteElManifiesto(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, salida, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0", "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var m we.Manifest
	if err := json.Unmarshal([]byte(salida), &m); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(m.Items) != 1 || m.Items[0].ID != "111" {
		t.Errorf("manifiesto = %+v, quería el item 111", m.Items)
	}
}

func TestConvertFallaSiElWallpaperNoEsVideo(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"222": `{"title":"Beta","type":"scene"}`,
	})
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "222")
	if codigo != 2 {
		t.Errorf("código = %d, quería 2 (todo falló)", codigo)
	}
	if !strings.Contains(errSalida, "✗") {
		t.Errorf("stderr = %q, quería que marque el fallo", errSalida)
	}
	// El manifiesto se escribe igual: el panel necesita saber que falló.
	manifiesto := filepath.Join(out, "manifest.json")
	datos, err := os.ReadFile(manifiesto)
	if err != nil {
		t.Fatalf("no escribió el manifiesto: %v", err)
	}
	var m we.Manifest
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("el manifiesto no es JSON: %v", err)
	}
	if len(m.Items) != 1 || m.Items[0].Status != "failed" {
		t.Errorf("manifiesto = %+v, quería el item 222 con status failed", m.Items)
	}
}

func TestConvertSinOutFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "convert", "--root", t.TempDir())
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--out") {
		t.Errorf("stderr = %q, quería que pida --out", errSalida)
	}
}

func TestConvertSinIdNiAllFalla(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--root", root)
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--id") {
		t.Errorf("stderr = %q, quería que pida --id o --all", errSalida)
	}
}

func TestConvertConIdInexistenteFalla(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--root", root, "--id", "999")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "no existe el wallpaper") {
		t.Errorf("stderr = %q, quería que diga que no existe", errSalida)
	}
}

func TestConvertCrfInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--crf", "alto")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--crf") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestConvertJobsInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--jobs", "0")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--jobs") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestConvertOpcionDesconocidaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "convert", "--out", t.TempDir(), "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestConvertAceptaLosValoresConIgual(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	codigo, _, errSalida := correr(t, "convert", "--out="+out, "--root="+root, "--id=111", "--crf=0")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if _, err := os.Stat(filepath.Join(out, "111-alfa.mp4")); err != nil {
		t.Errorf("no extrajo el video: %v", err)
	}
}

func TestConvertSalteaLoQueYaExiste(t *testing.T) {
	root := raizWorkshop(t, map[string]string{
		"111": `{"title":"Alfa","type":"video","file":"a.mp4"}`,
	})
	escribirArchivo(t, filepath.Join(root, "111", "a.mp4"), "video falso")
	out := t.TempDir()

	if codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0"); codigo != 0 {
		t.Fatalf("primera corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	codigo, _, errSalida := correr(t, "convert", "--out", out, "--root", root, "--id", "111", "--crf", "0")
	if codigo != 0 {
		t.Fatalf("segunda corrida: código = %d (stderr: %s)", codigo, errSalida)
	}
	var m we.Manifest
	datos, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != 1 || m.Items[0].Status != "skipped" {
		t.Errorf("manifiesto = %+v, quería el item 111 con status skipped", m.Items)
	}
}
