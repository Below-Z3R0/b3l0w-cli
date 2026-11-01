package we

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// versionTimeout acota el sondeo de versión: si un binario se cuelga, `b3l0w
// deps` no puede quedarse esperando.
const versionTimeout = 3 * time.Second

// ToolOK es el estado de un binario externo.
type ToolOK struct {
	Path    string `json:"path"`
	Ok      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
}

// WorkshopOK es el estado del catálogo de Wallpaper Engine.
type WorkshopOK struct {
	Path  string `json:"path"`
	Items int    `json:"items"`
	Ok    bool   `json:"ok"`
}

// RecordOK indica si se puede grabar video de un wallpaper renderizado.
type RecordOK struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

// Deps es el reporte de dependencias del entorno. Se serializa tal cual a JSON
// para el comando `b3l0w deps` y para el plugin de Noctalia.
type Deps struct {
	Engine   ToolOK     `json:"engine"`
	Ffmpeg   ToolOK     `json:"ffmpeg"`
	Ffprobe  ToolOK     `json:"ffprobe"`
	Workshop WorkshopOK `json:"workshop"`
	Record   RecordOK   `json:"record"`
}

// CheckDeps sondea el entorno y devuelve el reporte. Nunca falla por una
// dependencia ausente: lo que falta queda con Ok=false para que el CLI decida
// el exit code. root vacío dispara la autodetección del workshop.
func CheckDeps(ctx context.Context, root string) Deps {
	var d Deps
	d.Engine = probeEngine()
	d.Ffmpeg = probeFfmpeg(ctx)
	d.Ffprobe = probeFfprobe(ctx)
	d.Workshop = probeWorkshop(root)
	d.Record = probeRecord()
	return d
}

// probeEngine reporta el engine sin sondear versión: el binario no tiene un
// flag de versión estable y arrancarlo pide display.
func probeEngine() ToolOK {
	p, err := resolveEngine("")
	if err != nil {
		return ToolOK{}
	}
	return ToolOK{Path: p, Ok: true}
}

// probeFfmpeg resuelve ffmpeg con Locate (PATH, B3L0W_FFMPEG_DIR y las copias
// portables de ~/.hermes/tools) y le pide la versión. Si no aparece devuelve
// Path vacío y Ok=false: `deps` reporta, no falla.
func probeFfmpeg(ctx context.Context) ToolOK {
	t, err := Locate()
	if err != nil || !t.Ok {
		return ToolOK{}
	}
	return ToolOK{Path: t.Path, Ok: true, Version: toolVersion(ctx, t.Path, "-version")}
}

// probeFfprobe hace lo mismo con ffprobe, que el tarball trae al lado.
func probeFfprobe(ctx context.Context) ToolOK {
	t, err := LocateProbe()
	if err != nil || !t.Ok {
		return ToolOK{}
	}
	return ToolOK{Path: t.Path, Ok: true, Version: toolVersion(ctx, t.Path, "-version")}
}

// toolVersion devuelve la primera línea de `<tool> -version`. Si el binario
// falla o no contesta en versionTimeout, queda vacío: la versión es informativa
// y no cambia el Ok.
func toolVersion(ctx context.Context, path, arg string) string {
	cctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, arg).Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(line)
}

// probeWorkshop cuenta los items del catálogo. root vacío autodetecta; una raíz
// inexistente deja Ok=false con Items=0.
func probeWorkshop(root string) WorkshopOK {
	if strings.TrimSpace(root) == "" {
		r, err := DefaultRoot()
		if err != nil {
			return WorkshopOK{}
		}
		root = r
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	w := WorkshopOK{Path: abs}
	items, err := Scan(abs)
	if err != nil {
		return w
	}
	w.Items = len(items)
	w.Ok = true
	return w
}

// probeRecord decide si el método `record` puede correr. Faltan dos cosas en
// esta máquina: grim para capturar frames y el engine para renderizar el
// wallpaper; el primer faltante es el que se reporta.
func probeRecord() RecordOK {
	if p, err := exec.LookPath("grim"); err != nil || p == "" {
		return RecordOK{Reason: "requiere grim"}
	}
	if _, err := resolveEngine(""); err != nil {
		return RecordOK{Reason: "requiere linux-wallpaperengine"}
	}
	return RecordOK{Supported: true}
}

// MissingDeps devuelve los nombres de las dependencias obligatorias que no
// están: ffmpeg y workshop. El engine es opcional (los videos se extraen sin
// render), así que no entra en la lista.
func (d Deps) MissingDeps() []string {
	var missing []string
	if !d.Ffmpeg.Ok {
		missing = append(missing, "ffmpeg")
	}
	if !d.Workshop.Ok {
		missing = append(missing, "workshop")
	}
	return missing
}

// Summary arma una línea legible del estado, para el modo tabla de `deps`.
func (d Deps) Summary() string {
	var parts []string
	mark := func(name string, ok bool) string {
		if ok {
			return name + ":ok"
		}
		return name + ":falta"
	}
	parts = append(parts, mark("engine", d.Engine.Ok))
	parts = append(parts, mark("ffmpeg", d.Ffmpeg.Ok))
	parts = append(parts, mark("ffprobe", d.Ffprobe.Ok))
	parts = append(parts, fmt.Sprintf("workshop:%d items", d.Workshop.Items))
	rec := d.Record.Reason
	if d.Record.Supported {
		rec = "ok"
	}
	parts = append(parts, "record:"+rec)
	return strings.Join(parts, "  ")
}
