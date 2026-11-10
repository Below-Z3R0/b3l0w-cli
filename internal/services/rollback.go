package services

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// NormalizeDigest deja solo el "sha256:..." del digest.
//
// Del lado local, `docker image inspect` devuelve el RepoDigest
// ("ghcr.io/org/img@sha256:..."); del lado remoto, el header
// "Docker-Content-Digest" devuelve solo "sha256:...". Compararlos sin
// normalizar da "hay update" en cada corrida.
func NormalizeDigest(d string) string {
	if i := strings.Index(d, "@"); i >= 0 {
		d = d[i+1:]
	}
	return strings.TrimSpace(d)
}

// nowRFC3339 es la hora actual en el formato del estado.
func nowRFC3339() string { return time.Now().Format(time.RFC3339) }

// ImageInUse dice si algún container usa esa imagen.
func ImageInUse(image string) bool {
	out, err := exec.Command("docker", "ps", "--all", "--format", "{{.Image}}").Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == image {
			return true
		}
	}
	return false
}

// RemoveImage borra una imagen local. Devuelve false si no se pudo
// (no existe o la usa un container).
func (d *DockerAdapter) RemoveImage(image string) bool {
	if image == "" {
		return false
	}
	if ImageInUse(image) {
		return false
	}
	if _, err := d.run("docker", "image", "rm", image); err != nil {
		return false
	}
	return true
}

// PruneImages borra imágenes colgantes (dangling) del registry local.
// Es la limpieza de fondo: sin esto la máquina accumulates capas.
func (d *DockerAdapter) PruneImages() (string, error) {
	out, err := d.run("docker", "image", "prune", "--force", "--filter", "until=168h")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Rollback vuelve a la imagen anterior de un servicio. El estado
// guarda el tag previo, así que volver atrás es un pull de otro tag:
// no hace falta recompilar ni guardar imágenes localmente.
func (in *Installer) Rollback(svc Service, state *State) (*State, error) {
	if state == nil {
		return nil, fmt.Errorf("%s no está instalado", svc.ID)
	}
	if state.PreviousTag == "" || state.Previous == "" {
		return nil, fmt.Errorf("%s no tiene imagen anterior guardada", svc.ID)
	}
	output.Step(fmt.Sprintf("%s · rollback", svc.ID))
	fmt.Printf("  %s -> %s\n", state.Image, state.Previous)

	if err := in.Docker.Pull(svc.Image, state.PreviousTag); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	current := *state
	state.Image = state.Previous
	state.Tag = state.PreviousTag
	digest, _ := in.Docker.ImageDigest(state.Previous)
	state.Digest = digest
	state.Previous = current.Image
	state.PreviousTag = current.Tag
	state.UpdatedAt = nowRFC3339()

	dir := in.repoDir(svc)
	files := ComposeFiles(svc, state.Variant)
	env := map[string]string{"IMAGE": state.PreviousTag}
	if err := in.Docker.Up(dir, files, env); err != nil {
		return nil, fmt.Errorf("up: %w", err)
	}
	if err := in.verify(state, svc, dir, files); err != nil {
		return nil, fmt.Errorf("la imagen anterior tampoco arrancó: %w", err)
	}
	if err := state.Save(in.StateDir); err != nil {
		return nil, err
	}
	// La imagen que se acaba de dejar de usar se borra: la PC no
	// acumula imágenes de cada update.
	if current.Image != "" && current.Image != state.Image {
		if in.Docker.RemoveImage(current.Image) {
			fmt.Printf("  ✓ borrada la imagen anterior: %s\n", current.Image)
		}
	}
	return state, nil
}
