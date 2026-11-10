package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-kit/cli"
)

// RegistryCache remembers the last registry check so a stale manifest
// is detected without hammering GHCR on every command.
const RegistryCacheTTL = 6 * time.Hour

// RegistryStatus is the result of consulting the registry.
type RegistryStatus struct {
	CheckedAt   time.Time
	Available   map[string][]string
	Stale       []string
	Unreachable []string
}

// CheckRegistry consults the registry for each service and compares
// the published variants with what the manifest declares. A service
// whose manifest lists a variant that is not published is stale, like
// a pacman database that has not been synced.
func CheckRegistry(m *Manifest, reg *Registry) *RegistryStatus {
	st := &RegistryStatus{
		CheckedAt: time.Now(),
		Available: make(map[string][]string),
	}
	for i := range m.Services {
		svc := &m.Services[i]
		if svc.Image == "" {
			continue
		}
		tags, err := reg.Tags(svc.Image)
		if err != nil {
			st.Unreachable = append(st.Unreachable, svc.ID)
			st.Available[svc.ID] = svc.Available
			continue
		}
		published := AvailableVariants(tags, svc.Variants)
		if len(svc.Variants) == 0 {
			published = nil
			if len(tags) > 0 {
				published = []string{"default"}
			}
		}
		st.Available[svc.ID] = published
		for _, v := range svc.Available {
			if !contains(published, v) {
				st.Stale = append(st.Stale, fmt.Sprintf("%s: %s no está publicada", svc.ID, v))
			}
		}
		for _, v := range published {
			if len(svc.Available) > 0 && !contains(svc.Available, v) {
				st.Stale = append(st.Stale, fmt.Sprintf("%s: %s publicada y no declarada", svc.ID, v))
			}
		}
	}
	sort.Strings(st.Stale)
	sort.Strings(st.Unreachable)
	return st
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Sync actualiza el manifiesto con las variantes realmente publicadas
// en el registry. Es el equivalente a "pacman -Sy": sincroniza la
// caché, no instala nada.
func Sync(m *Manifest, manifestPath string, reg *Registry) (int, error) {
	changed := 0
	for i := range m.Services {
		svc := &m.Services[i]
		if svc.Image == "" {
			continue
		}
		tags, err := reg.Tags(svc.Image)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", svc.ID, err)
		}
		var published []string
		if len(svc.Variants) == 0 {
			if len(tags) > 0 {
				published = []string{"default"}
			}
		} else {
			published = AvailableVariants(tags, svc.Variants)
		}
		if !sameList(published, svc.Available) {
			fmt.Printf("  %s: %v -> %v\n", svc.ID, svc.Available, published)
			svc.Available = published
			changed++
		}
	}
	if changed == 0 {
		return 0, nil
	}
	if err := SaveManifest(manifestPath, m); err != nil {
		return changed, err
	}
	return changed, nil
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func RenderList(m *Manifest, states map[string]*State, st *RegistryStatus, hw Hardware) {
	cli.Step("Servicios")
	fmt.Printf("  %-10s %-9s %-8s %-10s %s\n", "ID", "ESTADO", "VAR", "IMAGEN", "NOTAS")
	for i := range m.Services {
		svc := &m.Services[i]
		state := states[svc.ID]
		status := svc.State
		variant := "-"
		notes := ""

		switch {
		case state != nil:
			status = "instalado"
			variant = state.Variant
			if variant == "" {
				variant = "default"
			}
			notes = state.Health
			if state.Provider != "" {
				notes += " " + state.Provider
			}
			if st != nil {
				if published := st.Available[svc.ID]; len(published) > 0 {
					if !contains(published, variant) {
						notes += " (variante no publicada)"
					}
				}
			}
		case svc.State == "planned":
			status = "planned"
			notes = "sin imagen publicada"
		case svc.State == "wip":
			status = "wip"
			notes = "en desarrollo"
		default:
			status = "disponible"
			if st != nil {
				if published := st.Available[svc.ID]; len(published) > 0 {
					notes = "variantes: " + strings.Join(published, ", ")
				}
			}
		}
		fmt.Printf("  %-10s %-9s %-8s %-10s %s\n", svc.ID, status, variant, svc.Image, cli.Dim(notes))
	}

	if st != nil && len(st.Stale) > 0 {
		cli.Warn("el manifiesto quedó desactualizado respecto del registry:")
		for _, s := range st.Stale {
			fmt.Printf("    %s\n", s)
		}
		fmt.Println("    (b3l0w services sync actualiza el manifiesto)")
	}
	if st != nil && len(st.Unreachable) > 0 {
		cli.Warn("no pude consultar el registry para: " + strings.Join(st.Unreachable, ", "))
	}
}

// RenderHardware imprime el hardware y la recomendación.
func RenderHardware(hw Hardware, m *Manifest, st *RegistryStatus) {
	cli.Step("Hardware")
	fmt.Printf("  CPU        %s", hw.CPUVendor)
	if len(hw.CPUFlags) > 0 {
		fmt.Printf(" (%s)", strings.Join(hw.CPUFlags, ", "))
	}
	fmt.Println()
	if len(hw.GPUs) == 0 {
		fmt.Println("  GPU        ninguna detectada")
	} else {
		for _, g := range hw.GPUs {
			fmt.Printf("  GPU        %s\n", DescribeGPU(g))
		}
	}
	for i := range m.Services {
		svc := &m.Services[i]
		if !svc.GPU {
			continue
		}
		available := svc.Available
		if st != nil {
			if published := st.Available[svc.ID]; len(published) > 0 {
				available = published
			}
		}
		if len(available) == 0 {
			continue
		}
		variant, reason, err := Recommend(hw, available)
		if err != nil {
			continue
		}
		provider := svc.EP[variant]
		fmt.Printf("  %-10s %s  %s\n", svc.ID, variant, cli.Dim(fmt.Sprintf("(%s) %s", provider, reason)))
	}
}
