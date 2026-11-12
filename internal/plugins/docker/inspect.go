package docker

import (
	"context"
	"encoding/json"
	"strings"
)

// Inspect es el detalle de un contenedor, ya aplanado para la UI.
type Inspect struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	StartedAt     string            `json:"startedAt"`
	FinishedAt    string            `json:"finishedAt"`
	RestartCount  int               `json:"restartCount"`
	RestartPolicy string            `json:"restartPolicy"`
	Platform      string            `json:"platform"`
	Command       []string          `json:"command"`
	Env           []string          `json:"env"`
	Labels        map[string]string `json:"labels"`
	Mounts        []Mount           `json:"mounts"`
	Networks      []string          `json:"networks"`
	Health        string            `json:"health"`
	HealthLog     []string          `json:"healthLog"`
}

// Mount es un bind o volumen montado.
type Mount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
	ReadOnly    bool   `json:"readOnly"`
}

// InspectContainer lee `docker inspect` y aplana los campos que el panel usa.
// Se pide por formato: el JSON completo trae cientos de KB y el runtime de Luau
// no los necesita.
func InspectContainer(ctx context.Context, run Runner, id string) (Inspect, error) {
	run = runnerDe(run)
	raw, err := run(ctx, "inspect", "--format", "{{json .}}", id)
	if err != nil {
		return Inspect{}, err
	}
	var payload struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status     string `json:"Status"`
			StartedAt  string `json:"StartedAt"`
			FinishedAt string `json:"FinishedAt"`
		} `json:"State"`
		RestartCount int `json:"RestartCount"`
		HostConfig   struct {
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`
		Platform string   `json:"Platform"`
		Path     string   `json:"Path"`
		Args     []string `json:"Args"`
		Config   struct {
			Image  string            `json:"Image"`
			Env    []string          `json:"Env"`
			Labels map[string]string `json:"Labels"`
			Cmd    []string          `json:"Cmd"`
			Health *struct {
				Log []struct {
					Output string `json:"Output"`
				} `json:"Log"`
			} `json:"Health"`
		} `json:"Config"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			Mode        string `json:"Mode"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return Inspect{}, err
	}
	out := Inspect{
		ID:            shortID(payload.ID),
		Name:          strings.TrimPrefix(payload.Name, "/"),
		Image:         payload.Config.Image,
		State:         payload.State.Status,
		StartedAt:     payload.State.StartedAt,
		FinishedAt:    payload.State.FinishedAt,
		RestartCount:  payload.RestartCount,
		RestartPolicy: payload.HostConfig.RestartPolicy.Name,
		Platform:      payload.Platform,
		Command:       append([]string{payload.Path}, payload.Args...),
		Env:           payload.Config.Env,
		Labels:        payload.Config.Labels,
		Networks:      make([]string, 0, len(payload.NetworkSettings.Networks)),
	}
	for nombre := range payload.NetworkSettings.Networks {
		out.Networks = append(out.Networks, nombre)
	}
	for _, mount := range payload.Mounts {
		out.Mounts = append(out.Mounts, Mount{
			Type:        mount.Type,
			Source:      mount.Source,
			Destination: mount.Destination,
			Mode:        mount.Mode,
			ReadOnly:    !mount.RW,
		})
	}
	if payload.Config.Health != nil {
		for _, entrada := range payload.Config.Health.Log {
			texto := strings.TrimSpace(entrada.Output)
			if texto != "" {
				out.HealthLog = append(out.HealthLog, texto)
			}
			if len(out.HealthLog) >= 5 {
				break
			}
		}
	}
	return out, nil
}
