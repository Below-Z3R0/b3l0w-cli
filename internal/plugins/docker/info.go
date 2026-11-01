package docker

import (
	"context"
	"encoding/json"
	"time"
)

// Info es lo que hace falta para leer bien las métricas: sin NCPU un 100% de
// CPU en stats (que es relativo a un core) se interpreta mal.
type Info struct {
	ServerVersion string `json:"serverVersion"`
	Driver        string `json:"driver"`
	RootDir       string `json:"rootDir"`
	NCPU          int    `json:"ncpu"`
	MemTotal      int64  `json:"memTotal"`
	Containers    int    `json:"containers"`
	Running       int    `json:"running"`
	Paused        int    `json:"paused"`
	Stopped       int    `json:"stopped"`
	Images        int    `json:"images"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
}

type infoRaw struct {
	ServerVersion string `json:"ServerVersion"`
	Driver        string `json:"Driver"`
	DockerRootDir string `json:"DockerRootDir"`
	NCPU          int    `json:"NCPU"`
	MemTotal      int64  `json:"MemTotal"`
	Containers    int    `json:"Containers"`
	ContainersRun int    `json:"ContainersRunning"`
	ContainersPse int    `json:"ContainersPaused"`
	ContainersStp int    `json:"ContainersStopped"`
	Images        int    `json:"Images"`
	OSType        string `json:"OSType"`
	Architecture  string `json:"Architecture"`
}

// GetInfo lee `docker info --format json`.
func GetInfo(ctx context.Context, runner Runner) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if runner == nil {
		runner = Exec
	}
	out, err := runner(ctx, "info", "--format", "json")
	if err != nil {
		return Info{}, err
	}
	var raw infoRaw
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return Info{}, err
	}
	return Info{
		ServerVersion: raw.ServerVersion,
		Driver:        raw.Driver,
		RootDir:       raw.DockerRootDir,
		NCPU:          raw.NCPU,
		MemTotal:      raw.MemTotal,
		Containers:    raw.Containers,
		Running:       raw.ContainersRun,
		Paused:        raw.ContainersPse,
		Stopped:       raw.ContainersStp,
		Images:        raw.Images,
		OS:            raw.OSType,
		Arch:          raw.Architecture,
	}, nil
}
