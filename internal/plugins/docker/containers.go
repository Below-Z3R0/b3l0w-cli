// Package docker habla con el daemon de Docker y devuelve un snapshot JSON que
// el plugin de Noctalia renderiza sin parsear nada.
//
// El plugin original hacía `docker ps --format '{{json .}}'` + `awk`/`cut` por
// contenedor y decodificaba de a 25 filas por tick; acá el trabajo pesado y el
// parseo de puertos viven en Go y el service sólo publica estado.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Container es una fila de `docker ps`.
type Container struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	State     string `json:"state"`  // running | exited | paused | created
	Status    string `json:"status"` // "Up 3 hours (healthy)"
	Health    string `json:"health"` // healthy | unhealthy | starting | ""
	Uptime    string `json:"uptime"` // "3 hours"
	Command   string `json:"command"`
	CreatedAt string `json:"createdAt"`
	Project   string `json:"project"` // label com.docker.compose.project
	Service   string `json:"service"` // label com.docker.compose.service
	Ports     []Port `json:"ports"`
	HasShell  bool   `json:"hasShell"` // el plugin puede ofrecer "abrir shell"
}

// Image es una fila de `docker images`.
type Image struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	CreatedAt  string `json:"createdAt"`
	Dangling   bool   `json:"dangling"`
}

// Volume es una fila de `docker volume ls`.
type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

// Network es una fila de `docker network ls`.
type Network struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Scope   string `json:"scope"`
	Builtin bool   `json:"builtin"`
}

// Snapshot es todo lo que la UI necesita en una sola pasada.
type Snapshot struct {
	Containers []Container `json:"containers"`
	Images     []Image     `json:"images"`
	Volumes    []Volume    `json:"volumes"`
	Networks   []Network   `json:"networks"`
	Projects   []Project   `json:"projects"`
	Running    int         `json:"running"`
	Stopped    int         `json:"stopped"`
	Unhealthy  int         `json:"unhealthy"`
	At         int64       `json:"at"`
	Error      string      `json:"error,omitempty"`
}

// Runner ejecuta un comando y devuelve stdout. Se inyecta para poder testear.
type Runner func(ctx context.Context, args ...string) (string, error)

// Exec es el runner real.
func Exec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s", primeraLinea(out.String()))
	}
	return out.String(), nil
}

func primeraLinea(texto string) string {
	texto = strings.TrimSpace(texto)
	if idx := strings.IndexByte(texto, '\n'); idx > 0 {
		texto = texto[:idx]
	}
	if len(texto) > 200 {
		texto = texto[:200]
	}
	return texto
}

type psRow struct {
	ID         string `json:"ID"`
	Names      string `json:"Names"`
	Image      string `json:"Image"`
	State      string `json:"State"`
	Status     string `json:"Status"`
	Command    string `json:"Command"`
	Ports      string `json:"Ports"`
	CreatedAt  string `json:"CreatedAt"`
	RunningFor string `json:"RunningFor"`
	Labels     string `json:"Labels"`
	HealthStat string `json:"HealthStatus"`
}

// HealthStatus no viene en `docker ps` estándar; se deduce del Status cuando
// trae el sufijo "(healthy)" / "(unhealthy)".
func healthFromStatus(status, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if idx := strings.LastIndex(status, "("); idx >= 0 {
		inner := strings.TrimSuffix(strings.TrimSpace(status[idx+1:]), ")")
		if inner == "healthy" || inner == "unhealthy" || inner == "starting" {
			return inner
		}
	}
	return ""
}

func labelValue(labels, key string) string {
	for _, par := range strings.Split(labels, ",") {
		if idx := strings.Index(par, "="); idx > 0 {
			if par[:idx] == key {
				return par[idx+1:]
			}
		}
	}
	return ""
}

// Containers lee `docker ps` (con -a si se piden detenidos).
func Containers(ctx context.Context, run Runner, all bool) ([]Container, error) {
	args := []string{"ps", "--no-trunc", "--format", "{{json .}}"}
	if all {
		args = []string{"ps", "-a", "--no-trunc", "--format", "{{json .}}"}
	}
	raw, err := run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var out []Container
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var row psRow
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		name := row.Names
		if idx := strings.IndexByte(name, ','); idx > 0 {
			name = name[:idx]
		}
		container := Container{
			ID:        shortID(row.ID),
			Name:      name,
			Image:     row.Image,
			State:     strings.ToLower(row.State),
			Status:    row.Status,
			Health:    healthFromStatus(row.Status, row.HealthStat),
			Uptime:    uptimeFromStatus(row.Status),
			Command:   row.Command,
			CreatedAt: row.CreatedAt,
			Project:   labelValue(row.Labels, "com.docker.compose.project"),
			Service:   labelValue(row.Labels, "com.docker.compose.service"),
			Ports:     ParsePorts(row.Ports),
		}
		if container.State == "" {
			if strings.HasPrefix(row.Status, "Up") {
				container.State = "running"
			} else {
				container.State = "exited"
			}
		}
		container.HasShell = container.State == "running"
		out = append(out, container)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].Name < out[j].Name
	})
	return out, scanner.Err()
}

func shortID(id string) string {
	if idx := strings.IndexByte(id, ':'); idx > 0 {
		return id[:idx]
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// uptimeFromStatus saca "3 hours" de "Up 3 hours (healthy)".
func uptimeFromStatus(status string) string {
	if !strings.HasPrefix(status, "Up ") {
		return ""
	}
	resto := strings.TrimPrefix(status, "Up ")
	if idx := strings.LastIndex(resto, " ("); idx > 0 {
		resto = resto[:idx]
	}
	return strings.TrimSpace(resto)
}

// Images lee `docker images`.
func Images(ctx context.Context, run Runner) ([]Image, error) {
	raw, err := run(ctx, "images", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var out []Image
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var row struct {
			ID         string `json:"ID"`
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			Size       string `json:"Size"`
			CreatedAt  string `json:"CreatedAt"`
		}
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		out = append(out, Image{
			ID:         shortID(row.ID),
			Repository: row.Repository,
			Tag:        row.Tag,
			Size:       row.Size,
			CreatedAt:  row.CreatedAt,
			Dangling:   row.Repository == "<none>",
		})
	}
	return out, scanner.Err()
}

// Volumes lee `docker volume ls`.
func Volumes(ctx context.Context, run Runner) ([]Volume, error) {
	raw, err := run(ctx, "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	var out []Volume
	for _, linea := range strings.Split(raw, "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var row struct {
			Name       string `json:"Name"`
			Driver     string `json:"Driver"`
			Mountpoint string `json:"Mountpoint"`
		}
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		out = append(out, Volume{Name: row.Name, Driver: row.Driver, Mountpoint: row.Mountpoint})
	}
	return out, nil
}

// Networks lee `docker network ls`.
func Networks(ctx context.Context, run Runner) ([]Network, error) {
	raw, err := run(ctx, "network", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	builtin := map[string]bool{"bridge": true, "host": true, "none": true}
	var out []Network
	for _, linea := range strings.Split(raw, "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var row struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
			Scope  string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(linea), &row); err != nil {
			continue
		}
		out = append(out, Network{
			ID: row.ID, Name: row.Name, Driver: row.Driver, Scope: row.Scope,
			Builtin: builtin[row.Name],
		})
	}
	return out, nil
}

// Snapshot junta todo. Las imágenes/volúmenes/redes se piden en paralelo: en
// serie son cuatro procesos que el panel esperaba uno atrás del otro.
func Collect(ctx context.Context, run Runner, includeStopped bool) Snapshot {
	// Slices vacíos, no nil: en JSON un nil sale `null` y el plugin recibe nil
	// en vez de una lista (cualquier `#payload.images` explota).
	snap := Snapshot{
		At:         time.Now().Unix(),
		Containers: []Container{},
		Images:     []Image{},
		Volumes:    []Volume{},
		Networks:   []Network{},
		Projects:   []Project{},
	}
	// Cada goroutine devuelve su resultado por canal. Escribir el struct
	// compartido desde tres goroutines es una carrera aunque los campos sean
	// distintos (lo detecta `go test -race`).
	type imagenesRes struct {
		lista []Image
		err   error
	}
	type volumenesRes struct {
		lista []Volume
		err   error
	}
	type redesRes struct {
		lista []Network
		err   error
	}
	canalImg := make(chan imagenesRes, 1)
	canalVol := make(chan volumenesRes, 1)
	canalRed := make(chan redesRes, 1)
	go func() {
		lista, err := Images(ctx, run)
		canalImg <- imagenesRes{lista, err}
	}()
	go func() {
		lista, err := Volumes(ctx, run)
		canalVol <- volumenesRes{lista, err}
	}()
	go func() {
		lista, err := Networks(ctx, run)
		canalRed <- redesRes{lista, err}
	}()

	containers, err := Containers(ctx, run, includeStopped)
	if err != nil {
		snap.Error = err.Error()
	}
	snap.Containers = containers

	if img := <-canalImg; img.err != nil {
		if snap.Error == "" {
			snap.Error = img.err.Error()
		}
	} else if img.lista != nil {
		snap.Images = img.lista
	}
	if vol := <-canalVol; vol.err != nil {
		if snap.Error == "" {
			snap.Error = vol.err.Error()
		}
	} else if vol.lista != nil {
		snap.Volumes = vol.lista
	}
	if red := <-canalRed; red.err != nil {
		if snap.Error == "" {
			snap.Error = red.err.Error()
		}
	} else if red.lista != nil {
		snap.Networks = red.lista
	}
	for _, c := range snap.Containers {
		if c.State == "running" {
			snap.Running++
		} else {
			snap.Stopped++
		}
		if c.Health == "unhealthy" {
			snap.Unhealthy++
		}
	}
	snap.Projects = GroupProjects(snap.Containers)
	return snap
}
