package wecli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func cmdPoster(args []string) int {
	out, root, sizeArg, manifest := "", "", "1280x720", ""
	ids := []string{}
	all, overwrite := false, false
	jobs := max(1, min(runtime.NumCPU()/2, 4))

	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, bool) {
			if _, after, found := strings.Cut(a, "="); found {
				return after, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case a == "--all":
			all = true
		case a == "--overwrite":
			overwrite = true
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we poster --out DIR [--id ID]... [--all] [--size 1280x720] [--jobs N] [--overwrite]")
			return 0
		case a == "--out" || strings.HasPrefix(a, "--out="):
			v, ok := value()
			if !ok {
				return fail("--out necesita un valor")
			}
			out = v
		case a == "--root" || strings.HasPrefix(a, "--root="):
			v, ok := value()
			if !ok {
				return fail("--root necesita un valor")
			}
			root = v
		case a == "--id" || strings.HasPrefix(a, "--id="):
			v, ok := value()
			if !ok {
				return fail("--id necesita un valor")
			}
			ids = append(ids, v)
		case a == "--size" || strings.HasPrefix(a, "--size="):
			v, ok := value()
			if !ok {
				return fail("--size necesita un valor")
			}
			sizeArg = v
		case a == "--jobs" || strings.HasPrefix(a, "--jobs="):
			v, ok := value()
			if !ok {
				return fail("--jobs necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &jobs); err != nil || jobs < 1 {
				return fail("--jobs no es un entero positivo: %s", v)
			}
		case a == "--manifest" || strings.HasPrefix(a, "--manifest="):
			v, ok := value()
			if !ok {
				return fail("--manifest necesita un valor")
			}
			manifest = v
		default:
			return fail("opción desconocida: %s", a)
		}
	}
	if out == "" {
		return fail("falta --out DIR")
	}
	var w, h int
	if _, err := fmt.Sscanf(sizeArg, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
		return fail("--size debe ser ANCHOxALTO (ej 1280x720), recibí %q", sizeArg)
	}
	outDir, err := filepath.Abs(out)
	if err != nil {
		return fail("resolver %s: %v", out, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fail("crear %s: %v", outDir, err)
	}

	items, rootPath, err := catalog(root)
	if err != nil {
		return fail("%v", err)
	}
	chosen, err := pick(items, ids, all)
	if err != nil {
		return fail("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()

	fmt.Fprintf(os.Stderr, "generando %d posters con %d jobs (es lento: ~30s por imagen)\n", len(chosen), jobs)
	results, err := we.Posters(ctx, chosen, we.PosterOptions{
		OutDir:    outDir,
		Size:      [2]int{w, h},
		Overwrite: overwrite,
		Jobs:      jobs,
	})
	if err != nil {
		return fail("%v", err)
	}

	manifestPath := manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(outDir, "manifest.json")
	}
	m, loadErr := we.LoadManifest(manifestPath)
	if loadErr != nil && !errors.Is(loadErr, fs.ErrNotExist) {
		m = we.Manifest{Version: 1}
	}
	if m.Version == 0 {
		m.Version = 1
	}
	m.Root, m.Out = rootPath, outDir
	m.Generated = time.Now().UTC()

	ok, failed := 0, 0
	for _, r := range results {
		entry := we.Entry{ID: r.ID, Type: "poster", File: r.Path, Method: "screenshot"}
		if it, findErr := we.Find(items, r.ID); findErr == nil {
			entry.Title = it.Title
			entry.Type = it.Type
			entry.Source = it.Preview
		}
		if r.Err != "" {
			entry.Status = "failed"
			entry.Err = r.Err
			failed++
			fmt.Fprintf(os.Stderr, "  ✗ %s: %s\n", r.ID, r.Err)
		} else {
			entry.Status = "ok"
			ok++
		}
		we.Upsert(&m, entry)
	}
	if err := we.SaveManifest(manifestPath, m); err != nil {
		return fail("guardar manifiesto: %v", err)
	}
	fmt.Printf("%s\n", manifestPath)
	fmt.Fprintf(os.Stderr, "\n%d posters · %d fallidos\n", ok, failed)
	if failed == len(results) && len(results) > 0 {
		return 2
	}
	return 0
}
