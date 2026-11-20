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
			// El healthcheck NO vive en Config: `docker inspect` lo pone
			// en State.Health (con Status y Log). Leerlo de Config hacía
			// que la salud saliera siempre vacía (D1).
			Health *struct {
				Status string `json:"Status"`
				Log    []struct {
					Output string `json:"Output"`
				} `json:"Log"`
			} `json:"Health"`
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
	if err := json.Unmarshal([]byte(extraerJSON(raw)), &payload); err != nil {
		return Inspect{}, err
	}
	// Las listas y el mapa salen vacíos, nunca nil: en JSON un nil sale
	// `null` y el plugin Luau hace `#payload.mounts` sobre null (D14).
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
		Env:           []string{},
		Labels:        map[string]string{},
		Mounts:        []Mount{},
		Networks:      make([]string, 0, len(payload.NetworkSettings.Networks)),
		HealthLog:     []string{},
	}
	if len(payload.Config.Env) > 0 {
		out.Env = payload.Config.Env
	}
	for clave, valor := range payload.Config.Labels {
		out.Labels[clave] = valor
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
	if payload.State.Health != nil {
		out.Health = payload.State.Health.Status
		// El log de health va de más antiguo a más reciente: las últimas
		// entradas son las que explican el fallo actual (D13).
		entradas := payload.State.Health.Log
		if len(entradas) > 5 {
			entradas = entradas[len(entradas)-5:]
		}
		for _, entrada := range entradas {
			texto := strings.TrimSpace(entrada.Output)
			if texto != "" {
				out.HealthLog = append(out.HealthLog, texto)
			}
		}
	}
	return out, nil
}
