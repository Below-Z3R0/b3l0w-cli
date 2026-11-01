package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DiskRow es una fila de `docker system df`.
type DiskRow struct {
	Type        string `json:"type"`
	Total       int    `json:"total"`
	Active      int    `json:"active"`
	Size        string `json:"size"`
	Reclaimable string `json:"reclaimable"`
}

// DiskUsage devuelve el espacio que ocupa y el que se puede recuperar.
func DiskUsage(ctx context.Context, run Runner) ([]DiskRow, string, error) {
	raw, err := run(ctx, "system", "df", "--format", "{{json .}}")
	if err != nil {
		return nil, "", err
	}
	var filas []DiskRow
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		// Ojo: `docker system df --format '{{json .}}'` manda TotalCount y Active
		// como STRING ("4"), no como número: con int el unmarshal falla y la fila
		// se descarta en silencio.
		var row struct {
			Type        string `json:"Type"`
			TotalCount  string `json:"TotalCount"`
			Active      string `json:"Active"`
			Size        string `json:"Size"`
			Reclaimable string `json:"Reclaimable"`
		}
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		total, _ := strconv.Atoi(row.TotalCount)
		activos, _ := strconv.Atoi(row.Active)
		filas = append(filas, DiskRow{
			Type: row.Type, Total: total, Active: activos,
			Size: row.Size, Reclaimable: limpiarReclaimable(row.Reclaimable),
		})
	}
	// La última línea de `docker system df` sin --format es el total; con
	// --format no viene, así que se pide aparte en texto.
	total, _ := run(ctx, "system", "df")
	return filas, ultimaLinea(total), nil
}

// limpiarReclaimable quita el porcentaje entre paréntesis: "1.017MB (0%)".
func limpiarReclaimable(valor string) string {
	if idx := strings.Index(valor, " ("); idx > 0 {
		return valor[:idx]
	}
	return valor
}

func ultimaLinea(texto string) string {
	lineas := strings.Split(strings.TrimSpace(texto), "\n")
	if len(lineas) == 0 {
		return ""
	}
	return strings.TrimSpace(lineas[len(lineas)-1])
}

// PruneTargets son los prune que acepta el CLI.
var PruneTargets = map[string][]string{
	"containers": {"container", "prune", "-f"},
	"images":     {"image", "prune", "-f"},
	"volumes":    {"volume", "prune", "-f"},
	"builder":    {"builder", "prune", "-f"},
	"system":     {"system", "prune", "-f"},
}

// Prune borra lo que no está en uso. `what` tiene que estar en PruneTargets:
// un valor fuera de la lista no se traduce a un comando.
func Prune(ctx context.Context, run Runner, what string) (Action, error) {
	args, ok := PruneTargets[what]
	if !ok {
		return Action{OK: false}, fmt.Errorf("prune inválido %q: containers, images, volumes, builder o system", what)
	}
	return ejecutar(ctx, run, args...)
}
