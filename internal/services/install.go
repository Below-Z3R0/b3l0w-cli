package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/confirm"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
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
	if len(published) == 0 {
		return "", "", fmt.Errorf("%s no tiene imagen publicada", svc.ID)
	}
	if want != "" {
		for _, v := range published {
			if v == want {
				return want, "elegida por el usuario", nil
			}
		}
		return "", "", fmt.Errorf("%s: la variante %q no está publicada (hay: %s)", svc.ID, want, strings.Join(published, ", "))
	}
	variant, reason, err = Recommend(hw, published)
	if err != nil {
		return "", "", err
	}
	// La recomendación tiene que estar publicada: si no, se cae a la
	// primera que sí lo esté.
	for _, v := range published {
		if v == variant {
			return variant, reason, nil
		}
	}
	return published[0], reason + "; uso " + published[0], nil
}

// TagFor devuelve el tag a usar para una variante.
func TagFor(svc Service, variant string, tags []string) string {
	if variant == "default" {
		for _, tag := range tags {
			if !strings.HasPrefix(tag, "arm64-") && !strings.HasPrefix(tag, "sha-") && tag != "latest" {
				return tag
			}
		}
		return "latest"
	}
	if tag := LatestTag(tags, variant); tag != "" {
		return tag
	}
	// Sin tag propio de la variante: el primero que no sea de caché.
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "buildcache-") {
			return tag
		}
	}
	return "latest"
}

// Install instala el servicio. dryRun solo muestra el plan.
//
// Los modelos no se instalan: son GB de descarga y los baja el CTL del
// servicio (argosctl models install). El CLI muestra el comando.
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

	output.Step(fmt.Sprintf("%s · %s", svc.ID, chosen))
	fmt.Printf("  %s\n", output.Dim(reason))
	fmt.Printf("  imagen: %s\n", image)
	if len(published) > 1 {
		fmt.Printf("  variantes: %s\n", strings.Join(published, ", "))
	}
	if svc.SizeGB > 0 {
		fmt.Printf("  disco:  ~%.1f GiB\n", svc.SizeGB)
	}
	if svc.Health != "" {
		fmt.Printf("  health: %s\n", svc.Health)
	}
	if files := ComposeFiles(svc, chosen); len(files) > 1 {
		fmt.Printf("  compose: %s\n", strings.Join(files, " + "))
	}
	if svc.Models != "" {
		fmt.Printf("  modelos: %s\n", output.Dim("los instala "+svc.Models))
	}

	if dryRun {
		fmt.Println("  → dry-run: no instalo")
		return nil, nil
	}

	// Puertos libres antes de levantar el stack. Si el servicio ya
	// está instalado, los puertos los ocupa su propio stack: el
	// install hace un recreate, no un arranque en frío.
	already, _ := LoadState(in.StateDir, svc.ID)
	for _, port := range svc.Ports {
		if PortInUse(port) {
			if already != nil {
				continue
			}
			return nil, fmt.Errorf("el puerto %d está ocupado", port)
		}
	}

	if err := in.Docker.Pull(svc.Image, tag); err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	digest, _ := in.Docker.ImageDigest(image)

	dir := in.repoDir(svc)
	files := ComposeFiles(svc, chosen)
	env := map[string]string{"IMAGE": tag}
	if err := in.Docker.Up(dir, files, env); err != nil {
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
	if err := in.verify(state, svc, dir, files); err != nil {
		return state, err
	}
	if err := state.Save(in.StateDir); err != nil {
		return state, err
	}
	return state, nil
}

// verify espera el healthcheck y lee el provider real.
func (in *Installer) verify(state *State, svc Service, dir string, files []string) error {
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
			} else if looksLikeArgos(body) {
				// El provider se llena cuando se carga un modelo. Sin
				// modelos no se puede verificar la aceleracion.
				state.Warnings = append(state.Warnings,
					"sin modelos cargados: el provider real se verifica cuando se carguen")
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

// looksLikeArgos dice si el status es el de Argos (el que trae el
// provider junto con los embedders cargados).
func looksLikeArgos(body string) bool {
	return strings.Contains(body, "embedders_loaded") && strings.Contains(body, "provider")
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
	localDigest := NormalizeDigest(state.Digest)
	remoteDigest = NormalizeDigest(remoteDigest)
	if remoteDigest == "" || remoteDigest == localDigest {
		output.Ok(fmt.Sprintf("%s: al día (%s)", svc.ID, tag))
		return state, nil
	}

	fmt.Printf("  %s: %s -> %s\n", svc.ID, shortDigest(localDigest), shortDigest(remoteDigest))
	if rebuild {
		fmt.Println("  → recompilo: pull de la imagen nueva y recreate del stack")
	} else {
		fmt.Println("  → solo actualizo: recreate con la imagen actual")
	}
	if !yes && !confirm.Confirm(fmt.Sprintf("Actualizar %s? [Y/n]", svc.ID), yes, true) {
		output.Warn("cancelado")
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
	files := ComposeFiles(svc, state.Variant)
	env := map[string]string{}
	if state.Tag != "" {
		env["IMAGE"] = state.Tag
	}
	if err := in.Docker.Up(dir, files, env); err != nil {
		return &previous, fmt.Errorf("up: %w", err)
	}
	if err := in.verify(state, svc, dir, files); err != nil {
		// Rollback: volver a la imagen anterior.
		output.Warn("el servicio no arrancó: vuelvo a la imagen anterior")
		rollbackEnv := map[string]string{}
		if previous.Tag != "" {
			rollbackEnv["IMAGE"] = previous.Tag
			_ = in.Docker.Pull(svc.Image, previous.Tag)
		}
		if err := in.Docker.Up(dir, files, rollbackEnv); err != nil {
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
