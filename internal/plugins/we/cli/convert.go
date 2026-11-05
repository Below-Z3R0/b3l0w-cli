package wecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

type convertOpts struct {
	out       string
	root      string
	ids       []string
	all       bool
	method    string
	format    string
	crf       int
	overwrite bool
	jobs      int
	manifest  string
	asJSON    bool
}

func cmdConvert(args []string) int {
	o := convertOpts{method: "auto", format: "mp4", crf: 18, jobs: max(1, runtime.NumCPU()/2)}
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
			o.all = true
		case a == "--overwrite":
			o.overwrite = true
		case a == "--json":
			o.asJSON = true
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we convert --out DIR [--id ID]... [--all] [--method auto|extract|record] [--format mp4|webm] [--crf N] [--jobs N] [--overwrite] [--manifest RUTA]")
			return 0
		case a == "--out" || strings.HasPrefix(a, "--out="):
			v, ok := value()
			if !ok {
				return fail("--out necesita un valor")
			}
			o.out = v
		case a == "--root" || strings.HasPrefix(a, "--root="):
			v, ok := value()
			if !ok {
				return fail("--root necesita un valor")
			}
			o.root = v
		case a == "--id" || strings.HasPrefix(a, "--id="):
			v, ok := value()
			if !ok {
				return fail("--id necesita un valor")
			}
			o.ids = append(o.ids, v)
		case a == "--method" || strings.HasPrefix(a, "--method="):
			v, ok := value()
			if !ok {
				return fail("--method necesita un valor")
			}
			o.method = v
		case a == "--format" || strings.HasPrefix(a, "--format="):
			v, ok := value()
			if !ok {
				return fail("--format necesita un valor")
			}
			o.format = v
		case a == "--crf" || strings.HasPrefix(a, "--crf="):
			v, ok := value()
			if !ok {
				return fail("--crf necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &o.crf); err != nil {
				return fail("--crf no es un número: %s", v)
			}
		case a == "--jobs" || strings.HasPrefix(a, "--jobs="):
			v, ok := value()
			if !ok {
				return fail("--jobs necesita un valor")
			}
			if _, err := fmt.Sscanf(v, "%d", &o.jobs); err != nil || o.jobs < 1 {
				return fail("--jobs no es un entero positivo: %s", v)
			}
		case a == "--manifest" || strings.HasPrefix(a, "--manifest="):
			v, ok := value()
			if !ok {
				return fail("--manifest necesita un valor")
			}
			o.manifest = v
		default:
			return fail("opción desconocida: %s", a)
		}
	}

	if o.out == "" {
		return fail("falta --out DIR")
	}
	outDir, err := filepath.Abs(o.out)
	if err != nil {
		return fail("resolver %s: %v", o.out, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fail("crear %s: %v", outDir, err)
	}

	items, root, err := catalog(o.root)
	if err != nil {
		return fail("%v", err)
	}
	chosen, err := pick(items, o.ids, o.all)
	if err != nil {
		return fail("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	entries := make([]we.Entry, len(chosen))
	sem := make(chan struct{}, o.jobs)
	var wg sync.WaitGroup
	var done, failed int
	var mu sync.Mutex

	for i, it := range chosen {
		wg.Add(1)
		go func(i int, it we.Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := we.Extract(ctx, it, we.ExtractOptions{
				OutDir:    outDir,
				Format:    o.format,
				CRF:       o.crf,
				Overwrite: o.overwrite,
			})
			mu.Lock()
			defer mu.Unlock()
			entry := we.Entry{ID: it.ID, Title: it.Title, Type: it.Type, Source: it.File}
			switch {
			case err != nil:
				entry.Status = "failed"
				entry.Err = err.Error()
				failed++
				fmt.Fprintf(os.Stderr, "  ✗ %-40s %v\n", it.Title, err)
			case res.Skipped:
				entry.Status = "skipped"
				entry.File = res.Path
				entry.Method = res.Method
				done++
			default:
				entry.Status = "ok"
				entry.File = res.Path
				entry.Method = res.Method
				entry.Bytes = res.Bytes
				done++
				fmt.Fprintf(os.Stderr, "  ✓ %-40s %s\n", it.Title, humanBytes(res.Bytes))
			}
			entries[i] = entry
		}(i, it)
	}
	wg.Wait()

	manifestPath := o.manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(outDir, "manifest.json")
	}
	manifest, err := we.LoadManifest(manifestPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "  aviso: manifiesto previo ilegible, se rehace (%v)\n", err)
		manifest = we.Manifest{Version: 1}
	}
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	manifest.Root = root
	manifest.Out = outDir
	for _, e := range entries {
		we.Upsert(&manifest, e)
	}
	manifest.Generated = time.Now().UTC()
	if err := we.SaveManifest(manifestPath, manifest); err != nil {
		return fail("guardar manifiesto: %v", err)
	}

	if o.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(manifest); err != nil {
			return fail("serializar: %v", err)
		}
	} else {
		fmt.Printf("%s\n", manifestPath)
	}

	fmt.Fprintf(os.Stderr, "\n%d ok · %d fallidos · %d totales\n", done, failed, len(chosen))
	if failed == len(chosen) {
		return 2
	}
	return 0
}
