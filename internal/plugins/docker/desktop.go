package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Las funciones que Docker Desktop expone y el plugin no tenía. Todas
// devuelven la misma forma (Action) para que el plugin Luau no tenga que
// distinguir.

// ExecResult es el resultado de `docker exec`.
type ExecResult struct {
	ID      int    `json:"id"`
	Running bool   `json:"running"`
	Exit    int    `json:"exit"`
	Output  string `json:"output"`
}

// ExecInContainer corre un comando dentro de un contenedor en ejecución. Es la función
// central de una terminal de contenedor (Docker Desktop la llama "Exec").
func ExecInContainer(ctx context.Context, run Runner, id string, argv []string) (ExecResult, error) {
	if len(argv) == 0 {
		return ExecResult{}, fmt.Errorf("exec: falta el comando")
	}
	run = runnerDe(run)
	args := append([]string{"exec", id}, argv...)
	salida, err := run(ctx, args...)
	if err != nil {
		return ExecResult{Output: salida}, err
	}
	return ExecResult{Running: true, Exit: 0, Output: strings.TrimSpace(salida)}, nil
}

// CopyResult es el resultado de copiar archivos entre el host y un contenedor.
type CopyResult struct {
	Bytes  int64  `json:"bytes"`
	Path   string `json:"path"`
	Output string `json:"output,omitempty"`
}

// Copy copia archivos de o hacia un contenedor (`docker cp`).
func Copy(ctx context.Context, run Runner, id string, origen string, destino string) (CopyResult, error) {
	run = runnerDe(run)
	salida, err := run(ctx, "cp", origen, id+":"+destino)
	if err != nil {
		return CopyResult{}, err
	}
	return CopyResult{Path: destino, Output: strings.TrimSpace(salida)}, nil
}

// CopyFrom copia archivos de un contenedor al host.
func CopyFrom(ctx context.Context, run Runner, id string, origen string, destino string) (CopyResult, error) {
	run = runnerDe(run)
	salida, err := run(ctx, "cp", id+":"+origen, destino)
	if err != nil {
		return CopyResult{}, err
	}
	return CopyResult{Path: destino, Output: strings.TrimSpace(salida)}, nil
}

// Commit crea una imagen a partir del estado actual de un contenedor
// (Docker Desktop: "Commit to image").
func Commit(ctx context.Context, run Runner, id string, repo string, tag string, mensaje string) (Action, error) {
	args := []string{"commit"}
	if mensaje != "" {
		args = append(args, "-m", mensaje)
	}
	args = append(args, id)
	if repo != "" {
		if tag == "" {
			tag = "latest"
		}
		args = append(args, repo+":"+tag)
	}
	return ejecutar(ctx, run, args...)
}

// Pause congela un contenedor (docker pause).
func Pause(ctx context.Context, run Runner, id string) (Action, error) {
	return ejecutar(ctx, run, "pause", id)
}

// Unpause reanuda un contenedor congelado.
func Unpause(ctx context.Context, run Runner, id string) (Action, error) {
	return ejecutar(ctx, run, "unpause", id)
}

// Rename cambia el nombre de un contenedor.
func Rename(ctx context.Context, run Runner, id string, nombre string) (Action, error) {
	return ejecutar(ctx, run, "rename", id, nombre)
}

// Tag etiqueta una imagen con un repositorio y tag nuevos.
func Tag(ctx context.Context, run Runner, id string, repo string, tag string) (Action, error) {
	if tag == "" {
		tag = "latest"
	}
	return ejecutar(ctx, run, "tag", id, repo+":"+tag)
}

// Top lista los procesos que corren dentro de un contenedor (`docker top`).
func Top(ctx context.Context, run Runner, id string) (Action, error) {
	return ejecutar(ctx, run, "top", id)
}

// History devuelve el historial de capas de una imagen.
func History(ctx context.Context, run Runner, id string) (Action, error) {
	return ejecutar(ctx, run, "history", "--no-trunc", id)
}

// PruneDetailed es el prune granular: se puede pedir sólo imágenes no
// usadas, sólo contenedores parados, sólo volúmenes o sólo la red.
func PruneDetailed(ctx context.Context, run Runner, what string, todas bool) (Action, error) {
	args := []string{"prune"}
	switch what {
	case "containers":
		args = append(args, "--filter", "until=24h")
	case "images":
		args = append(args, "--filter", "until=24h")
		args = append(args, "-a")
	case "volumes":
		args = append(args, "--volumes")
	case "networks":
		args = append(args, "--filter", "until=24h")
	case "builder":
		args = append(args, "--filter", "until=24h")
	case "system":
		if todas {
			args = append(args, "-a", "--volumes")
		}
	default:
		return Action{OK: false, Command: "docker prune"}, fmt.Errorf("destino inválido: %q", what)
	}
	args = append(args, "--force")
	return ejecutar(ctx, run, args...)
}

// SystemInfo agrega al snapshot la información del sistema que Docker
// Desktop muestra en su dashboard.
type SystemInfo struct {
	Containers int `json:"containers"`
	Running    int `json:"running"`
	Paused     int `json:"paused"`
	Stopped    int `json:"stopped"`
	Images     int `json:"images"`
	Volumes    int `json:"volumes"`
	Networks   int `json:"networks"`
}

// StatsMuestreo es una muestra de stats por contenedor.
type StatsMuestreo struct {
	Nombre  string  `json:"name"`
	CPU     float64 `json:"cpu"`
	Memoria int64   `json:"memoria"`
	MemLim  int64   `json:"memoriaLimite"`
	RedRX   int64   `json:"redRx"`
	RedTX   int64   `json:"redTx"`
	Bloques int64   `json:"bloques"`
	Pids    int     `json:"pids"`
	MemPct  float64 `json:"memoriaPct"`
	CPUPct  float64 `json:"cpuPct"`
	NetPct  float64 `json:"redPct"`
}

// PrettyBytes formatea bytes como texto legible ("1.2 GB").
func PrettyBytes(b int64) string {
	const unidad = 1024.0
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(1024), 0
	for n := b / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// MarshalIndent es un helper para las salidas JSON del CLI.
func MarshalIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// RunCommand ejecuta un comando de docker con varios argumentos y devuelve
// la salida. Se usa para los comandos de lectura sin forma propia.
func RunCommand(ctx context.Context, run Runner, comando string, argv []string) (Action, error) {
	args := append([]string{comando}, argv...)
	return ejecutar(ctx, run, args...)
}
