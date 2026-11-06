package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// DoctorResult es el resultado de los chequeos.
type DoctorResult struct {
	Errors   int
	Warnings int
}

// Doctor chequea que la máquina pueda correr servicios.
func (d *DockerAdapter) Doctor(m *Manifest, hw Hardware, states []*State) DoctorResult {
	res := DoctorResult{}
	cli.Step("Chequeos de servicios")

	// Docker y el daemon.
	if !d.DockerOK() {
		cli.Fail("docker: el daemon no responde")
		res.Errors++
	} else {
		cli.Ok("docker: daemon responde")
	}

	// Compose v2: el v1 no soporta --project-directory ni perfiles.
	if !d.HaveComposeV2() {
		cli.Fail("docker compose: falta el plugin (docker-compose)")
		res.Errors++
	} else {
		cli.Ok("docker compose: " + d.ComposeVersion())
	}

	// Disco: las imágenes son grandes.
	if free, err := d.DiskFree(); err == nil {
		gb := float64(free) / (1 << 30)
		if gb < 20 {
			cli.Warn(fmt.Sprintf("disco: %.1f GiB libres; las imágenes de GPU necesitan 15-25 GiB", gb))
			res.Warnings++
		} else {
			cli.Ok(fmt.Sprintf("disco: %.1f GiB libres", gb))
		}
	}

	// Aceleración.
	if len(hw.GPUs) == 0 {
		cli.Warn("gpu: no hay GPU; los servicios usarán la variante cpu")
		res.Warnings++
	} else {
		for _, g := range hw.GPUs {
			cli.Ok("gpu: " + DescribeGPU(g))
		}
	}
	switch {
	case hw.GPUs == nil:
	case hasNvidia(hw.GPUs):
		if !NVIDIAToolkitInstalled() {
			cli.Fail("nvidia-container-toolkit: falta; la variante cuda no va a acelerar")
			res.Errors++
		} else {
			cli.Ok("nvidia-container-toolkit: instalado")
		}
	default:
		if !RenderDRI() {
			cli.Fail("/dev/dri: no existe; la variante intel/amd no va a acelerar")
			res.Errors++
		} else {
			cli.Ok("/dev/dri: presente")
		}
	}

	// Grupos: sin render/video no se puede usar /dev/dri.
	if len(hw.GPUs) > 0 && !hasNvidia(hw.GPUs) {
		for _, g := range []string{"render", "video"} {
			if !GroupExists(g) {
				cli.Warn(fmt.Sprintf("grupo %s: no existe", g))
				res.Warnings++
				continue
			}
			cli.Ok("grupo " + g + ": existe")
		}
	}

	// Puertos de los servicios instalados.
	for _, state := range states {
		for _, port := range state.Ports {
			if PortInUse(port) {
				// Ocupado puede ser el propio servicio.
				cli.Ok(fmt.Sprintf("puerto %d: en uso (%s)", port, state.ID))
				continue
			}
			cli.Warn(fmt.Sprintf("puerto %d: libre (%s no está corriendo)", port, state.ID))
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
		cli.Ok(fmt.Sprintf("%s: %s (%s) %s", svc.ID, variant, provider, reason))
	}

	if res.Errors > 0 {
		cli.Fail(fmt.Sprintf("%d chequeos fallaron", res.Errors))
	} else if res.Warnings > 0 {
		cli.Warn(fmt.Sprintf("%d avisos", res.Warnings))
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
func (d *DockerAdapter) Remove(dir, file string, purge bool) error {
	args := []string{"compose", "-f", file, "down", "--remove-orphans"}
	if purge {
		args = append(args, "--volumes")
	}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	_, err := d.run(args...)
	return err
}

// Restart reinicia el stack.
func (d *DockerAdapter) Restart(dir, file string) error {
	args := []string{"compose", "-f", file, "restart"}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	_, err := d.run(args...)
	return err
}

// Stop baja el stack sin borrarlo.
func (d *DockerAdapter) Stop(dir, file string) error {
	args := []string{"compose", "-f", file, "stop"}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	_, err := d.run(args...)
	return err
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
