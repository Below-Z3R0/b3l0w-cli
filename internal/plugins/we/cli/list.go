package wecli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

func cmdList(args []string) int {
	asJSON, root, typ, query := false, "", "", ""
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
		case a == "--type" || strings.HasPrefix(a, "--type="):
			v, ok := value()
			if !ok {
				return fail("--type necesita un valor")
			}
			if !we.ValidType(v) {
				return fail("--type inválido: %s (video|scene|web|unknown)", v)
			}
			typ = v
		case a == "--query" || strings.HasPrefix(a, "--query="):
			v, ok := value()
			if !ok {
				return fail("--query necesita un valor")
			}
			query = v
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we list [--json] [--root DIR] [--type video|scene|web] [--query TXT]")
			return 0
		default:
			return fail("opción desconocida: %s", a)
		}
	}

	items, _, err := catalog(root)
	if err != nil {
		return fail("%v", err)
	}
	// El total va aparte: el resumen dice "X de Y" y con filtro Y es el
	// tamaño del catálogo, no el de la selección.
	total := len(items)
	items = we.Filter(items, typ, query)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			return fail("serializar: %v", err)
		}
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIPO\tTAMAÑO\tTÍTULO")
	for _, it := range items {
		title := it.Title
		if it.Err != "" {
			title = "(roto) " + it.Err
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.ID, it.Type, humanBytes(it.Bytes), title)
	}
	if err := tw.Flush(); err != nil {
		return fail("escribir tabla: %v", err)
	}
	fmt.Fprintf(os.Stderr, "\n%d de %d wallpapers\n", len(items), total)
	return 0
}

func cmdInfo(args []string) int {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we info <id> [--json]")
			return 0
		case strings.HasPrefix(a, "-"):
			return fail("opción desconocida: %s", a)
		case id == "":
			id = a
		}
	}
	if id == "" {
		return fail("falta el id")
	}
	items, _, err := catalog("")
	if err != nil {
		return fail("%v", err)
	}
	item, err := we.Find(items, id)
	if err != nil {
		return fail("%v", err)
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(item); err != nil {
			return fail("serializar: %v", err)
		}
		return 0
	}
	fmt.Printf("id:      %s\ntítulo:  %s\ntipo:    %s\ntamaño:  %s\n", item.ID, item.Title, item.Type, humanBytes(item.Bytes))
	if item.File != "" {
		fmt.Printf("archivo: %s\n", item.File)
	}
	if item.Preview != "" {
		fmt.Printf("preview: %s\n", item.Preview)
	}
	if item.Err != "" {
		fmt.Printf("error:   %s\n", item.Err)
	}
	return 0
}
