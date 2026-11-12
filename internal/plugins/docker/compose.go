package docker

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Project agrupa los contenedores de un mismo stack de compose. Se arma desde
// las etiquetas del snapshot: `docker compose ls` no dice el estado agregado y
// obligaría a un proceso extra por refresco.
type Project struct {
	Name       string   `json:"name"`
	Services   []string `json:"services"`
	Containers int      `json:"containers"`
	Running    int      `json:"running"`
	Stopped    int      `json:"stopped"`
	Unhealthy  int      `json:"unhealthy"`
	State      string   `json:"state"` // running | partial | stopped
	ConfigFile string   `json:"configFile,omitempty"`
}

// GroupProjects arma los stacks a partir de los contenedores ya listados.
func GroupProjects(containers []Container) []Project {
	porNombre := map[string]*Project{}
	for _, c := range containers {
		nombre := c.Project
		if nombre == "" {
			continue
		}
		p := porNombre[nombre]
		if p == nil {
			p = &Project{Name: nombre, Services: []string{}, State: "stopped"}
			porNombre[nombre] = p
		}
		p.Containers++
		if c.State == "running" {
			p.Running++
		} else {
			p.Stopped++
		}
		if c.Health == "unhealthy" {
			p.Unhealthy++
		}
		if c.Service != "" {
			p.Services = append(p.Services, c.Service)
		}
	}
	out := make([]Project, 0, len(porNombre))
	for _, p := range porNombre {
		switch {
		case p.Running == p.Containers && p.Containers > 0:
			p.State = "running"
		case p.Running > 0:
			p.State = "partial"
		default:
			p.State = "stopped"
		}
		sort.Strings(p.Services)
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ComposeAction corre `docker compose -p <proyecto> <accion>` resolviendo el
// archivo desde `docker compose ls`.
func ComposeAction(ctx context.Context, run Runner, project, action string) (Action, error) {
	run = runnerDe(run)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	args := []string{"compose", "-p", project, action}
	if action == "up" {
		args = append(args, "-d")
	}
	res := Action{Command: "docker " + strings.Join(args, " ")}
	out, err := run(ctx, args...)
	res.Output = strings.TrimSpace(out)
	if err != nil {
		res.OK = false
		return res, err
	}
	res.OK = true
	if len(res.Output) > 400 {
		res.Output = res.Output[:400]
	}
	return res, nil
}

// Event es una línea de `docker events` normalizada.
type Event struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Name   string `json:"name"`
	ID     string `json:"id"`
	Image  string `json:"image,omitempty"`
	Health string `json:"health,omitempty"`
	Exit   string `json:"exit,omitempty"`
}

type eventRaw struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Time   int64  `json:"time"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

// Events devuelve los últimos eventos de contenedor. `docker events` sin
// `--until` no termina nunca: se acota la ventana para que el comando retorne.
func Events(ctx context.Context, run Runner, since string) ([]Event, error) {
	run = runnerDe(run)
	if since == "" {
		since = "6h"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := run(ctx, "events",
		"--since", since,
		"--until", "0s",
		"--filter", "type=container",
		"--format", "{{json .}}")
	if err != nil && strings.TrimSpace(out) == "" {
		return []Event{}, err
	}
	eventos := []Event{}
	for _, linea := range strings.Split(out, "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" || !strings.HasPrefix(linea, "{") {
			continue
		}
		var raw eventRaw
		if json.Unmarshal([]byte(linea), &raw) != nil {
			continue
		}
		accion := raw.Action
		if accion == "" {
			continue
		}
		ev := Event{
			Action: accion,
			Name:   raw.Actor.Attributes["name"],
			Image:  raw.Actor.Attributes["image"],
			Health: raw.Actor.Attributes["health_status"],
			Exit:   raw.Actor.Attributes["exitCode"],
			ID:     shortID(raw.Actor.ID),
		}
		if raw.Time > 0 {
			ev.Time = time.Unix(raw.Time, 0).Format("15:04:05")
		}
		eventos = append(eventos, ev)
	}
	// lo más nuevo primero, y sin crecer sin techo
	for i, j := 0, len(eventos)-1; i < j; i, j = i+1, j-1 {
		eventos[i], eventos[j] = eventos[j], eventos[i]
	}
	if len(eventos) > 60 {
		eventos = eventos[:60]
	}
	return eventos, nil
}
