package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// Stat es una fila de `docker stats --no-stream`.
type Stat struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	CPUPct   float64 `json:"cpuPct"`
	MemPct   float64 `json:"memPct"`
	MemUsage string  `json:"memUsage"` // "12.5MiB / 15.6GiB"
	NetIO    string  `json:"netIo"`
	BlockIO  string  `json:"blockIo"`
	PIDs     int     `json:"pids"`
}

// percent saca el número de "12.34%".
func percent(valor string) float64 {
	valor = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(valor), "%"))
	numero, err := strconv.ParseFloat(valor, 64)
	if err != nil {
		return 0
	}
	return numero
}

// Stats devuelve el consumo de cada contenedor vivo. `--no-stream` es una
// muestra instantánea: no deja ningún proceso colgado midiendo.
func Stats(ctx context.Context, run Runner) ([]Stat, error) {
	run = runnerDe(run)
	raw, err := run(ctx, "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var out []Stat
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var row struct {
			ID       string `json:"ID"`
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemPerc  string `json:"MemPerc"`
			MemUsage string `json:"MemUsage"`
			NetIO    string `json:"NetIO"`
			BlockIO  string `json:"BlockIO"`
			PIDs     string `json:"PIDs"`
		}
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		pids, _ := strconv.Atoi(strings.TrimSpace(row.PIDs))
		out = append(out, Stat{
			ID:       shortID(row.ID),
			Name:     row.Name,
			CPUPct:   percent(row.CPUPerc),
			MemPct:   percent(row.MemPerc),
			MemUsage: row.MemUsage,
			NetIO:    row.NetIO,
			BlockIO:  row.BlockIO,
			PIDs:     pids,
		})
	}
	return out, scanner.Err()
}
