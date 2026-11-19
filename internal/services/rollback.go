package services

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
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

// restorePrevious deja el stack con la imagen anterior.
//
// Con tags mutables, volver atrás es volver al digest: el tag anterior
// ya apunta a la imagen que se acaba de instalar, así que un pull de
// ese tag baja lo nuevo y el rollback no vuelve atrás. Se baja el
// digest anterior y se reetiqueta con el tag, que es lo que compose
// levanta.
func (in *Installer) restorePrevious(svc Service, prevDigest, prevTag, curDigest string) error {
	d := NormalizeDigest(prevDigest)
	if d == "" || d == NormalizeDigest(curDigest) {
		if prevTag == "" {
			return nil
		}
		return in.Docker.Pull(svc.Image, prevTag)
	}
	if err := in.Docker.PullDigest(svc.Image, d); err != nil {
		return err
	}
	if prevTag == "" {
		return nil
	}
	return in.Docker.TagImage(svc.Image+"@"+d, svc.Image+":"+prevTag)
}

// Rollback vuelve a la imagen anterior de un servicio. El estado
// guarda el digest previo, así que volver atrás es un pull de ese
// digest: no hace falta recompilar ni guardar imágenes localmente.
func (in *Installer) Rollback(svc Service, state *State) (*State, error) {
	if state == nil {
		return nil, fmt.Errorf("%s no está instalado", svc.ID)
	}
	if state.PreviousTag == "" || state.Previous == "" {
		return nil, fmt.Errorf("%s no tiene imagen anterior guardada", svc.ID)
	}
	output.Step(fmt.Sprintf("%s · rollback", svc.ID))
	fmt.Printf("  %s -> %s\n", state.Image, state.Previous)

	prevDigest := NormalizeDigest(state.PreviousDigest)
	if err := in.restorePrevious(svc, state.PreviousDigest, state.PreviousTag, state.Digest); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	current := *state
	// El tag al que se vuelve se guarda aparte: despues PreviousTag
	// pasa a ser el tag nuevo (para poder volver adelante), y el env
	// del compose tiene que llevar el viejo.
	rollbackTag := state.PreviousTag
	state.Image = state.Previous
	state.Tag = rollbackTag
	if prevDigest != "" {
		// El digest es lo que se sabe con certeza: el inspect local
		// puede devolver otra cosa.
		state.Digest = svc.Image + "@" + prevDigest
	} else {
		digest, _ := in.Docker.ImageDigest(state.Previous)
		state.Digest = digest
	}
	state.Previous = current.Image
	state.PreviousTag = current.Tag
	state.PreviousDigest = NormalizeDigest(current.Digest)
	state.UpdatedAt = nowRFC3339()

	dir := in.repoDir(svc)
	files := ComposeFiles(svc, state.Variant)
	env := map[string]string{"IMAGE": rollbackTag}
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
	if prevDigest != "" && NormalizeDigest(current.Digest) != prevDigest {
		// Con tags mutables el tag ya apunta a la imagen nueva: la
		// que quedó atrás se borra por su digest.
		old := svc.Image + "@" + NormalizeDigest(current.Digest)
		if in.Docker.RemoveImage(old) {
			fmt.Printf("  ✓ borrada la imagen anterior: %s\n", old)
		}
	} else if current.Image != "" && current.Image != state.Image {
		if in.Docker.RemoveImage(current.Image) {
			fmt.Printf("  ✓ borrada la imagen anterior: %s\n", current.Image)
		}
	}
	return state, nil
}
