package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// DoctorResult es el resultado de los chequeos.
type DoctorResult struct {
	Errors   int
	Warnings int
}

// Doctor chequea que la máquina pueda correr servicios.
func (d *DockerAdapter) Doctor(m *Manifest, hw Hardware, states []*State) DoctorResult {
	res := DoctorResult{}
	output.Step("Chequeos de servicios")

	// Docker y el daemon.
	if !d.DockerOK() {
		output.Fail("docker: el daemon no responde")
		res.Errors++
	} else {
		output.Ok("docker: daemon responde")
	}

	// Compose v2: el v1 no soporta --project-directory ni perfiles.
	if !d.HaveComposeV2() {
		output.Fail("docker compose: falta el plugin (docker-compose)")
		res.Errors++
	} else {
		output.Ok("docker compose: " + d.ComposeVersion())
	}

	// Disco: las imágenes son grandes.
	if free, err := d.DiskFree(); err == nil {
		gb := float64(free) / (1 << 30)
		if gb < 20 {
			output.Warn(fmt.Sprintf("disco: %.1f GiB libres; las imágenes de GPU necesitan 15-25 GiB", gb))
			res.Warnings++
		} else {
			output.Ok(fmt.Sprintf("disco: %.1f GiB libres", gb))
		}
	}

	// Aceleración.
	if len(hw.GPUs) == 0 {
		output.Warn("gpu: no hay GPU; los servicios usarán la variante cpu")
		res.Warnings++
	} else {
		for _, g := range hw.GPUs {
			output.Ok("gpu: " + DescribeGPU(g))
		}
	}
	switch {
	case hw.GPUs == nil:
	case hasNvidia(hw.GPUs):
		if !NVIDIAToolkitInstalled() {
			output.Fail("nvidia-container-toolkit: falta; la variante cuda no va a acelerar")
			res.Errors++
		} else {
			output.Ok("nvidia-container-toolkit: instalado")
		}
	default:
		if !RenderDRI() {
			output.Fail("/dev/dri: no existe; la variante intel/amd no va a acelerar")
			res.Errors++
		} else {
			output.Ok("/dev/dri: presente")
		}
	}

	// Grupos: sin render/video no se puede usar /dev/dri.
	if len(hw.GPUs) > 0 && !hasNvidia(hw.GPUs) {
		for _, g := range []string{"render", "video"} {
			if !GroupExists(g) {
				output.Warn(fmt.Sprintf("grupo %s: no existe", g))
				res.Warnings++
				continue
			}
			output.Ok("grupo " + g + ": existe")
		}
	}

	// Puertos de los servicios instalados.
	for _, state := range states {
		for _, port := range state.Ports {
			if PortInUse(port) {
				// Ocupado puede ser el propio servicio.
				output.Ok(fmt.Sprintf("puerto %d: en uso (%s)", port, state.ID))
				continue
			}
			output.Warn(fmt.Sprintf("puerto %d: libre (lo declara %s)", port, state.ID))
			res.Warnings++
		}
	}

	// Variantes publicadas vs hardware.
	for i := range m.Services {
		svc := &m.Services[i]
		if !svc.GPU {
			continue
		}
		published := svc.Available
		if len(published) == 0 {
			continue
		}
		variant, reason, err := Recommend(hw, published)
		if err != nil {
			continue
		}
		provider := svc.EP[variant]
		output.Ok(fmt.Sprintf("%s: %s (%s) %s", svc.ID, variant, provider, reason))
	}

	if res.Errors > 0 {
		output.Fail(fmt.Sprintf("%d chequeos fallaron", res.Errors))
	} else if res.Warnings > 0 {
		output.Warn(fmt.Sprintf("%d avisos", res.Warnings))
	}
	return res
}

func hasNvidia(gpus []GPU) bool {
	for _, g := range gpus {
		if g.Vendor == "nvidia" {
			return true
		}
	}
	return false
}

// Remove baja el stack sin borrar volúmenes. purge borra los volúmenes.
func (d *DockerAdapter) Remove(dir string, files []string, purge bool) error {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "down", "--remove-orphans")
	if purge {
		args = append(args, "--volumes")
	}
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
	}
	out, err := d.run(args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Restart reinicia el stack.
func (d *DockerAdapter) Restart(dir string, files []string) error {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "restart")
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
	}
	out, err := d.run(args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Stop baja el stack sin borrarlo.
func (d *DockerAdapter) Stop(dir string, files []string) error {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "stop")
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
	}
	out, err := d.run(args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// ComposeFiles devuelve los compose a usar para una variante: el
// principal y el override que le corresponde.
func ComposeFiles(svc Service, variant string) []string {
	files := []string{ComposeFile(svc)}
	if svc.Overrides != nil {
		if override, ok := svc.Overrides[variant]; ok && override != "" {
			files = append(files, override)
		}
	}
	return files
}

// ComposeFile devuelve la ruta del compose de un servicio dentro de su
// repo.
func ComposeFile(svc Service) string {
	if svc.Compose != "" {
		return svc.Compose
	}
	return "compose.yaml"
}

// StateDirReady dice si el directorio de estado existe.
func StateDirReady(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// VariantForTag deduce la variante a partir del tag de una imagen.
func VariantForTag(tag string) string {
	parts := strings.Split(tag, "-")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
