package wecli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func cmdDeps(args []string) int {
	asJSON := false
	root := ""
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
		case a == "--json":
			asJSON = true
		case a == "--root" || strings.HasPrefix(a, "--root="):
			v, ok := value()
			if !ok {
				return fail("--root necesita un valor")
			}
			root = v
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we deps [--json] [--root DIR]")
			return 0
		default:
			return fail("opción desconocida: %s", a)
		}
	}
	if root == "" {
		if found, err := we.DefaultRoot(); err == nil {
			root = found
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	deps := we.CheckDeps(ctx, root)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(deps); err != nil {
			return fail("serializar: %v", err)
		}
	} else {
		mark := func(ok bool) string {
			if ok {
				return "ok"
			}
			return "FALTA"
		}
		fmt.Printf("ffmpeg   %-6s %s\n", mark(deps.Ffmpeg.Ok), deps.Ffmpeg.Path)
		fmt.Printf("ffprobe  %-6s %s\n", mark(deps.Ffprobe.Ok), deps.Ffprobe.Path)
		fmt.Printf("engine   %-6s %s\n", mark(deps.Engine.Ok), deps.Engine.Path)
		fmt.Printf("workshop %-6s %s (%d items)\n", mark(deps.Workshop.Ok), deps.Workshop.Path, deps.Workshop.Items)
		if deps.Record.Supported {
			fmt.Printf("record   ok\n")
		} else {
			fmt.Printf("record   no     %s\n", deps.Record.Reason)
		}
	}

	// ffmpeg y el workshop son lo único imprescindible para convertir.
	if !deps.Ffmpeg.Ok || !deps.Workshop.Ok {
		return 2
	}
	return 0
}
