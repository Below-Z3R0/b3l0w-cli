package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// Installer instala un servicio: pull de la imagen, up del stack,
// verificación de health y del provider real.
type Installer struct {
	Docker   *DockerAdapter
	Registry *Registry
	StateDir string
	// RepoDir es donde vive el compose del servicio.
	RepoDir func(svc Service) string
}

// ResolveVariant decide la variante: la que pide el usuario, o la
// recomendada por hardware. Si la elegida no está publicada, avisa.
func ResolveVariant(svc Service, hw Hardware, published []string, want string) (variant, reason string, err error) {
	if want != "" {
		if len(published) == 0 {
			return "", "", fmt.Errorf("%s no tiene imagen publicada", svc.ID)
		}
		for _, v := range published {
			if v == want {
				return want, "elegida por el usuario", nil
			}
		}
		return "", "", fmt.Errorf("%s: la variante %q no está publicada (hay: %s)", svc.ID, want, strings.Join(published, ", "))
	}
	return Recommend(hw, published)
}

// TagFor devuelve el tag a usar para una variante.
func TagFor(svc Service, variant string, tags []string) string {
	if variant == "default" {
		if svc.State == "" {
			return "latest"
		}
		for _, tag := range tags {
			if !strings.HasPrefix(tag, "arm64-") && !strings.HasPrefix(tag, "sha-") && tag != "latest" {
				return tag
			}
		}
		return "latest"
	}
	return LatestTag(tags, variant)
}

// Install instala el servicio. dryRun solo muestra el plan.
func (in *Installer) Install(svc Service, variant string, dryRun bool) (*State, error) {
	tags, err := in.Registry.Tags(svc.Image)
	if err != nil {
		return nil, fmt.Errorf("%s: no pude leer el registry: %w", svc.ID, err)
	}
	published := AvailableVariants(tags, svc.Variants)
	if len(published) == 0 {
		return nil, fmt.Errorf("%s: no hay imagen publicada todavía", svc.ID)
	}
	hw := DetectHardware()
	chosen, reason, err := ResolveVariant(svc, hw, published, variant)
	if err != nil {
		return nil, err
	}
	tag := TagFor(svc, chosen, tags)
	image := svc.Image + ":" + tag

	cli.Step(fmt.Sprintf("%s · %s", svc.ID, chosen))
	fmt.Printf("  %s\n", cli.Dim(reason))
	fmt.Printf("  imagen: %s\n", image)
	if svc.SizeGB > 0 {
		fmt.Printf("  disco:  ~%.1f GiB\n", svc.SizeGB)
	}
	if svc.Health != "" {
		fmt.Printf("  health: %s\n", svc.Health)
	}
	if svc.Models != "" {
		fmt.Printf("  modelos: %s\n", cli.Dim("los instala "+svc.Models))
	}

	if dryRun {
		fmt.Println("  → dry-run: no instalo")
		return nil, nil
	}

	// Puertos libres antes de levantar el stack.
	for _, port := range svc.Ports {
		if PortInUse(port) {
			return nil, fmt.Errorf("el puerto %d está ocupado", port)
		}
	}

	if err := in.Docker.Pull(svc.Image, tag); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	digest, _ := in.Docker.ImageDigest(image)

	dir := in.repoDir(svc)
	file := svc.Compose
	if file == "" {
		file = "compose.yaml"
	}
	env := map[string]string{}
	if chosen != "default" {
		env["VARIANT"] = chosen
	}
	if err := in.Docker.Up(dir, file, env); err != nil {
		return nil, fmt.Errorf("up: %w", err)
	}

	state := &State{
		ID:          svc.ID,
		Variant:     chosen,
		Image:       image,
		Tag:         tag,
		Digest:      digest,
		InstalledAt: time.Now().Format(time.RFC3339),
		Health:      "starting",
		Ports:       svc.Ports,
	}
	if err := in.verify(state, svc, dir, file); err != nil {
		return state, err
	}
	if err := state.Save(in.StateDir); err != nil {
		return state, err
	}
	return state, nil
}

// verify espera el healthcheck y lee el provider real.
func (in *Installer) verify(state *State, svc Service, dir, file string) error {
	if svc.Health == "" {
		state.Health = "ok"
		return nil
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		body, err := HTTPGet(svc.Health, 5*time.Second)
		if err == nil {
			state.Health = "healthy"
			if provider := parseProvider(body); provider != "" {
				state.Provider = provider
			}
			return nil
		}
		if time.Now().After(deadline) {
			state.Health = "unhealthy"
			return fmt.Errorf("%s no respondió en %s", svc.Health, svc.Health)
		}
		time.Sleep(3 * time.Second)
	}
}

// parseProvider busca el provider en un JSON de status.
func parseProvider(body string) string {
	for _, key := range []string{"\"provider\"", "\"execution_provider\"", "\"ep\"", "\"device\""} {
		idx := strings.Index(body, key)
		if idx < 0 {
			continue
		}
		rest := body[idx+len(key):]
		rest = strings.TrimLeft(rest, " :")
		if !strings.HasPrefix(rest, "\"") {
			continue
		}
		rest = rest[1:]
		if end := strings.Index(rest, "\""); end > 0 {
			return rest[:end]
		}
	}
	return ""
}

func (in *Installer) repoDir(svc Service) string {
	if in.RepoDir != nil {
		return in.RepoDir(svc)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "Github", filepath.Base(svc.Repo))
}

// Update comprueba si hay digest nuevo y, si lo hay y el usuario
// confirma, hace pull + recreate. Vuelve al digest anterior si falla.
func (in *Installer) Update(svc Service, state *State, rebuild bool, yes bool) (*State, error) {
	if state == nil || state.Image == "" {
		return nil, fmt.Errorf("%s no está instalado", svc.ID)
	}
	tags, err := in.Registry.Tags(svc.Image)
	if err != nil {
		return nil, fmt.Errorf("%s: no pude leer el registry: %w", svc.ID, err)
	}
	tag := state.Tag
	if tag == "" {
		tag = LatestTag(tags, state.Variant)
	}
	remoteDigest, err := in.Registry.Digest(svc.Image, tag)
	if err != nil {
		return nil, fmt.Errorf("%s: no pude pedir el digest remoto: %w", svc.ID, err)
	}
	localDigest := state.Digest
	if remoteDigest == "" || remoteDigest == localDigest {
		cli.Ok(fmt.Sprintf("%s: al día (%s)", svc.ID, tag))
		return state, nil
	}

	fmt.Printf("  %s: %s -> %s\n", svc.ID, shortDigest(localDigest), shortDigest(remoteDigest))
	if rebuild {
		fmt.Println("  → recompilo: pull de la imagen nueva y recreate del stack")
	} else {
		fmt.Println("  → solo actualizo: recreate con la imagen actual")
	}
	if !yes && !cli.Confirm(fmt.Sprintf("Actualizar %s? [Y/n]", svc.ID), yes, true) {
		cli.Warn("cancelado")
		return state, nil
	}

	previous := *state
	state.Previous = state.Image
	state.PreviousTag = state.Tag

	if rebuild {
		if err := in.Docker.Pull(svc.Image, tag); err != nil {
			return &previous, fmt.Errorf("pull: %w", err)
		}
	}
	digest, _ := in.Docker.ImageDigest(svc.Image + ":" + tag)
	state.Digest = digest
	state.UpdatedAt = time.Now().Format(time.RFC3339)

	dir := in.repoDir(svc)
	file := svc.Compose
	if file == "" {
		file = "compose.yaml"
	}
	env := map[string]string{}
	if state.Variant != "" && state.Variant != "default" {
		env["VARIANT"] = state.Variant
	}
	if err := in.Docker.Up(dir, file, env); err != nil {
		return &previous, fmt.Errorf("up: %w", err)
	}
	if err := in.verify(state, svc, dir, file); err != nil {
		// Rollback: volver a la imagen anterior.
		cli.Warn("el servicio no arrancó: vuelvo a la imagen anterior")
		rollbackEnv := map[string]string{}
		if previous.Variant != "" && previous.Variant != "default" {
			rollbackEnv["VARIANT"] = previous.Variant
		}
		if previous.Tag != "" {
			_ = in.Docker.Pull(svc.Image, previous.Tag)
		}
		if err := in.Docker.Up(dir, file, rollbackEnv); err != nil {
			return &previous, fmt.Errorf("rollback falló: %w", err)
		}
		*state = previous
		return &previous, fmt.Errorf("rollback a %s: %w", previous.Tag, err)
	}
	if err := state.Save(in.StateDir); err != nil {
		return state, err
	}
	// La imagen que quedó atrás se borra: la PC no acumula una imagen
	// por cada update.
	if previous.Image != "" && previous.Image != state.Image {
		if in.Docker.RemoveImage(previous.Image) {
			fmt.Printf("  ✓ borrada la imagen anterior: %s\n", previous.Image)
		}
	}
	return state, nil
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19]
	}
	if d == "" {
		return "?"
	}
	return d
}
