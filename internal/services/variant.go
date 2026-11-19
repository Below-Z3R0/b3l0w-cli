package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
)

// VariantOptions describe un cambio de variante de un servicio.
type VariantOptions struct {
	ManifestPath string
	StateDir     string
	ID           string
	Variant      string

	// Registry y Docker son para los tests: nil usa los reales.
	Registry *Registry
	Docker   *DockerAdapter
	// HealthTimeout es cuanto se espera al healthcheck. Cero = 2 minutos.
	HealthTimeout time.Duration
}

// healthTimeout es cuanto se espera al healthcheck.
func (o VariantOptions) healthTimeout() time.Duration {
	if o.HealthTimeout > 0 {
		return o.HealthTimeout
	}
	return defaultHealthTimeout
}

// VariantResult es lo que el cambio produjo, para que el llamador lo
// presente sin volver a consultar nada.
type VariantResult struct {
	ID      string
	Variant string
	Image   string
	Tag     string
}

// SwitchVariant cambia la variante de un servicio instalado.
//
// Hace el pull, recrea el stack y guarda el estado. Recrear es lo que
// hace que la variante se aplique: antes solo se hacía el pull y se
// guardaba el estado, y el contenedor seguía corriendo la imagen
// anterior mientras el estado decía otra cosa. El env con IMAGE es lo
// que le dice a compose qué imagen levantar.
//
// Vive acá y no en cmd/ porque es un caso de uso: valida contra el
// catálogo, consulta el registry y mueve el estado.
func SwitchVariant(opts VariantOptions) (*VariantResult, error) {
	m, err := LoadManifest(opts.ManifestPath)
	if err != nil {
		return nil, err
	}
	svc := m.Find(opts.ID)
	if svc == nil {
		return nil, fmt.Errorf("servicio desconocido: %s", opts.ID)
	}

	state, err := LoadState(opts.StateDir, svc.ID)
	if err != nil {
		return nil, fmt.Errorf("%s no está instalado", svc.ID)
	}

	reg := opts.Registry
	if reg == nil {
		reg = NewRegistry("")
	}
	tags, err := reg.Tags(svc.Image)
	if err != nil {
		return nil, err
	}
	published := AvailableVariants(tags, svc.Variants)
	if !contiene(published, opts.Variant) {
		return nil, fmt.Errorf("%s: la variante %q no está publicada (hay: %s)",
			svc.ID, opts.Variant, strings.Join(published, ", "))
	}

	tag := TagFor(*svc, opts.Variant, tags)
	image := svc.Image + ":" + tag

	docker := opts.Docker
	if docker == nil {
		docker = NewDocker()
	}
	if err := docker.Pull(svc.Image, tag); err != nil {
		return nil, err
	}
	digest, _ := docker.ImageDigest(image)

	dir := ServiceDir(*svc)
	files := ComposeFiles(*svc, opts.Variant)
	if err := docker.Up(dir, files, map[string]string{"IMAGE": tag}); err != nil {
		return nil, err
	}

	state.Previous = state.Image
	state.PreviousTag = state.Tag
	state.PreviousDigest = NormalizeDigest(state.Digest)
	state.Variant = opts.Variant
	state.Image = image
	state.Tag = tag
	state.Digest = digest
	state.UpdatedAt = time.Now().Format(time.RFC3339)

	// La salud y el provider son los de la variante nueva: los de la
	// anterior no dicen nada del stack que se acaba de levantar.
	state.Health = "starting"
	state.Provider = ""
	if err := verifyHealth(state, *svc, opts.healthTimeout()); err != nil {
		state.Warnings = append(state.Warnings,
			fmt.Sprintf("la variante %s no arrancó: %s", opts.Variant, err))
		_ = state.Save(opts.StateDir)
		return nil, fmt.Errorf("%s: la variante %q no arrancó: %w", svc.ID, opts.Variant, err)
	}
	if err := state.Save(opts.StateDir); err != nil {
		return nil, err
	}

	return &VariantResult{ID: svc.ID, Variant: opts.Variant, Image: image, Tag: tag}, nil
}

// ServiceDir es donde vive el stack del servicio: el repo de la distro
// clonado en el home.
func ServiceDir(svc Service) string {
	home := paths.Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Documents", "Github", repoName(svc.Repo))
}

func repoName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

func contiene(lista []string, valor string) bool {
	for _, v := range lista {
		if v == valor {
			return true
		}
	}
	return false
}
