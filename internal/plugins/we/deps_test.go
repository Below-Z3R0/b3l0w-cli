package we

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sinHerramientas deja PATH y HOME apuntando a directorios vacíos para que el
// sondeo no encuentre binarios reales de la máquina.
func sinHerramientas(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	return home
}

// fakeTool escribe un script ejecutable que responde a -version como ffmpeg.
func fakeTool(t *testing.T, dir, name, firstLine string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	script := "#!/bin/sh\nprintf '" + firstLine + "\\nsegunda-linea\\n'\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("escribir %s: %v", name, err)
	}
	return p
}

func TestCheckDepsRootVacio(t *testing.T) {
	sinHerramientas(t)
	root := t.TempDir()

	d := CheckDeps(context.Background(), root)

	if !d.Workshop.Ok || d.Workshop.Items != 0 {
		t.Errorf("workshop vacío = %+v", d.Workshop)
	}
	if d.Workshop.Path != root {
		t.Errorf("workshop.Path = %q, quería %q", d.Workshop.Path, root)
	}
	if d.Ffmpeg.Ok || d.Ffmpeg.Path != "" {
		t.Errorf("ffmpeg no debería estar disponible: %+v", d.Ffmpeg)
	}
	if d.Ffprobe.Ok {
		t.Errorf("ffprobe no debería estar disponible: %+v", d.Ffprobe)
	}
	if d.Record.Supported || d.Record.Reason != "requiere grim" {
		t.Errorf("record sin grim = %+v", d.Record)
	}
	// El engine puede o no estar: no debe paniquear ni inventar un Path falso.
	if !d.Engine.Ok && d.Engine.Path != "" {
		t.Errorf("engine con Ok=false no debería traer Path: %+v", d.Engine)
	}
}

func TestCheckDepsWorkshopInexistente(t *testing.T) {
	sinHerramientas(t)
	root := filepath.Join(t.TempDir(), "no-existe")

	d := CheckDeps(context.Background(), root)
	if d.Workshop.Ok {
		t.Errorf("una raíz inexistente no debería quedar Ok: %+v", d.Workshop)
	}
	if d.Workshop.Items != 0 {
		t.Errorf("Items = %d, quería 0", d.Workshop.Items)
	}
	if d.Workshop.Path != root {
		t.Errorf("Path = %q, quería %q", d.Workshop.Path, root)
	}
}

func TestCheckDepsCuentaItems(t *testing.T) {
	sinHerramientas(t)
	root := t.TempDir()
	for _, id := range []string{"111", "222"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"title":"x","type":"Video"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d := CheckDeps(context.Background(), root)
	if !d.Workshop.Ok || d.Workshop.Items != 2 {
		t.Errorf("workshop = %+v, quería 2 items", d.Workshop)
	}
}

func TestCheckDepsHerramientasEnPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	ff := fakeTool(t, bin, "ffmpeg", "ffmpeg version 9.9.9-fake")
	fp := fakeTool(t, bin, "ffprobe", "ffprobe version 9.9.9-fake")

	d := CheckDeps(context.Background(), t.TempDir())
	if !d.Ffmpeg.Ok || d.Ffmpeg.Path != ff {
		t.Errorf("ffmpeg = %+v, quería %q", d.Ffmpeg, ff)
	}
	if d.Ffmpeg.Version != "ffmpeg version 9.9.9-fake" {
		t.Errorf("versión de ffmpeg = %q", d.Ffmpeg.Version)
	}
	if !d.Ffprobe.Ok || d.Ffprobe.Path != fp {
		t.Errorf("ffprobe = %+v, quería %q", d.Ffprobe, fp)
	}
	if d.Ffprobe.Version != "ffprobe version 9.9.9-fake" {
		t.Errorf("versión de ffprobe = %q", d.Ffprobe.Version)
	}
}

func TestCheckDepsFfmpegFueraDePATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	bin := filepath.Join(home, ".hermes", "tools", "ffmpeg-9.0.1-linux-x64", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	ff := fakeTool(t, bin, "ffmpeg", "ffmpeg version 9.0.1-fake")

	// Sin ffprobe al lado: se reporta ffmpeg pero no ffprobe.
	d := CheckDeps(context.Background(), t.TempDir())
	if !d.Ffmpeg.Ok || d.Ffmpeg.Path != ff {
		t.Fatalf("no encontró ffmpeg en ~/.hermes/tools: %+v", d.Ffmpeg)
	}
	if d.Ffprobe.Ok {
		t.Errorf("no hay ffprobe, no debería quedar Ok: %+v", d.Ffprobe)
	}

	// El tarball trae los dos juntos: se resuelve por hermano del ffmpeg.
	fp := fakeTool(t, bin, "ffprobe", "ffprobe version 9.0.1-fake")
	d = CheckDeps(context.Background(), t.TempDir())
	if !d.Ffprobe.Ok || d.Ffprobe.Path != fp {
		t.Errorf("ffprobe hermano = %+v, quería %q", d.Ffprobe, fp)
	}
}

func TestDepsJSONTags(t *testing.T) {
	sinHerramientas(t)
	d := CheckDeps(context.Background(), t.TempDir())
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"engine", "ffmpeg", "ffprobe", "workshop", "record"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("falta la clave %q en %s", k, string(b))
		}
	}
	var w struct {
		Path  string `json:"path"`
		Items int    `json:"items"`
		Ok    bool   `json:"ok"`
	}
	if err := json.Unmarshal(raw["workshop"], &w); err != nil || !w.Ok {
		t.Errorf("workshop JSON = %s (%v)", string(raw["workshop"]), err)
	}
	var r struct {
		Supported bool   `json:"supported"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(raw["record"], &r); err != nil || r.Supported || r.Reason == "" {
		t.Errorf("record JSON = %s (%v)", string(raw["record"]), err)
	}
}

func TestDepsMissingYSummary(t *testing.T) {
	var vacio Deps
	missing := vacio.MissingDeps()
	if len(missing) != 2 || missing[0] != "ffmpeg" || missing[1] != "workshop" {
		t.Errorf("MissingDeps vacío = %v", missing)
	}
	if s := vacio.Summary(); !strings.Contains(s, "ffmpeg:falta") || !strings.Contains(s, "record:") {
		t.Errorf("Summary = %q", s)
	}

	ok := Deps{Ffmpeg: ToolOK{Ok: true}, Workshop: WorkshopOK{Ok: true, Items: 41}, Record: RecordOK{Supported: true}}
	if got := ok.MissingDeps(); len(got) != 0 {
		t.Errorf("no debería faltar nada: %v", got)
	}
	if s := ok.Summary(); !strings.Contains(s, "workshop:41 items") || !strings.Contains(s, "record:ok") {
		t.Errorf("Summary = %q", s)
	}
}
