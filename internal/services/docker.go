package services

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// DockerAdapter habla con Docker. Todo pasa por compose v2.
type DockerAdapter struct {
	Runner func(name string, args ...string) (string, error)
}

// NewDocker devuelve un adapter con los defaults del sistema.
func NewDocker() *DockerAdapter {
	return &DockerAdapter{Runner: cli.RunCombined}
}

func (d *DockerAdapter) run(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("comando vacío")
	}
	if d.Runner != nil {
		return d.Runner(args[0], args[1:]...)
	}
	return cli.RunCombined(args[0], args[1:]...)
}

// HaveComposeV2 dice si está el plugin de compose v2 (o posterior).
//
// La numeración cambió: compose v2 se announces como "v2.x.x" y las
// versiones nuevas como "5.x.x". Lo que importa es que el subcomando
// `docker compose` exista: el v1 (`docker-compose`) no soporta
// --project-directory ni los perfiles que usa el CLI.
func (d *DockerAdapter) HaveComposeV2() bool {
	out, err := d.run("docker", "compose", "version")
	if err != nil {
		return false
	}
	return strings.Contains(out, "Docker Compose version")
}

// ComposeVersion devuelve la versión de compose, o "" si no está.
func (d *DockerAdapter) ComposeVersion() string {
	out, err := d.run("docker", "compose", "version")
	if err != nil {
		return ""
	}
	idx := strings.Index(out, "version")
	if idx < 0 {
		return ""
	}
	fields := strings.Fields(out[idx+len("version"):])
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// DockerOK dice si el daemon responde.
func (d *DockerAdapter) DockerOK() bool {
	_, err := d.run("docker", "info", "--format", "{{.ServerVersion}}")
	return err == nil
}

// DiskFree devuelve los bytes libres del filesystem donde está el
// directorio de Docker.
func (d *DockerAdapter) DiskFree() (uint64, error) {
	out, err := d.run("docker", "info", "--format", "{{.DockerRootDir}}")
	if err != nil {
		return 0, err
	}
	root := strings.TrimSpace(out)
	if root == "" {
		root = "/var/lib/docker"
	}
	return freeSpace(filepath.Dir(root))
}

// freeSpace usa statfs para leer el espacio libre.
func freeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// ComposeService es un contenedor del stack.
type ComposeService struct {
	Name    string
	State   string
	Health  string
	Image   string
	Ports   string
	RawLine string
}

// PS lista los contenedores del stack.
func (d *DockerAdapter) PS(dir string, file string) ([]ComposeService, error) {
	args := []string{"compose", "-f", file, "ps", "--format", "{{.Service}}\t{{.State}}\t{{.Health}}\t{{.Image}}\t{{.Ports}}"}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir, "-f", file}, args[2:]...)
	}
	out, err := d.run(args...)
	if err != nil {
		return nil, err
	}
	var out2 []ComposeService
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		for len(fields) < 5 {
			fields = append(fields, "")
		}
		out2 = append(out2, ComposeService{
			Name: fields[0], State: fields[1], Health: fields[2],
			Image: fields[3], Ports: fields[4], RawLine: line,
		})
	}
	return out2, nil
}

// ImageDigest devuelve el digest local de una imagen.
func (d *DockerAdapter) ImageDigest(image string) (string, error) {
	out, err := d.run("docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", image)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Pull baja una imagen.
func (d *DockerAdapter) Pull(image, tag string) error {
	_, err := d.run("docker", "pull", image+":"+tag)
	return err
}

// Up levanta el stack.
func (d *DockerAdapter) Up(dir, file string, env map[string]string) error {
	args := []string{"compose", "-f", file}
	for k, v := range env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, "up", "-d", "--remove-orphans")
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	_, err := d.run(args...)
	return err
}

// Down baja el stack sin borrar volúmenes.
func (d *DockerAdapter) Down(dir, file string) error {
	args := []string{"compose", "-f", file, "down", "--remove-orphans"}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	_, err := d.run(args...)
	return err
}

// Logs devuelve las últimas líneas de un servicio.
func (d *DockerAdapter) Logs(dir, file, service string, tail int) (string, error) {
	args := []string{"compose", "-f", file, "logs", "--tail", strconv.Itoa(tail)}
	if service != "" {
		args = append(args, service)
	}
	if dir != "" {
		args = append([]string{"compose", "--project-directory", dir}, args[1:]...)
	}
	return d.run(args...)
}

// HTTPGet consulta una URL y devuelve el cuerpo.
func HTTPGet(url string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PortInUse dice si algo escucha en el puerto.
func PortInUse(port int) bool {
	out, err := exec.Command("ss", "-ltn").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), fmt.Sprintf(":%d ", port)) ||
		strings.Contains(string(out), fmt.Sprintf(":%d\n", port))
}

// GroupExists dice si el grupo existe en el sistema.
func GroupExists(name string) bool {
	out, err := exec.Command("getent", "group", name).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// NvidiaToolkitInstalled dice si está el toolkit de NVIDIA.
func NVIDIAToolkitInstalled() bool {
	_, err := os.Stat("/usr/bin/nvidia-container-runtime")
	return err == nil
}

// RenderDRI dice si el host expone /dev/dri.
func RenderDRI() bool {
	entries, err := os.ReadDir("/dev/dri")
	return err == nil && len(entries) > 0
}
