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

	"github.com/Below-Z3R0/b3l0w-kit/cli"
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

// composeFiles devuelve los compose con path completo. El `-f` de
// compose es relativo al directorio de trabajo, no al
// --project-directory: sin el path absoluto, compose busca el archivo
// en el cwd del CLI.
func composeFiles(dir string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		if dir != "" && !filepath.IsAbs(file) {
			out = append(out, filepath.Join(dir, file))
		} else {
			out = append(out, file)
		}
	}
	return out
}

// runEnv corre un comando con variables de entorno extra.
//
// Docker Compose no tiene un flag para pasar variables: `${IMAGE}` se
// resuelve desde el entorno del proceso. Por eso el tag de la imagen
// viaja así, y no con `--env` (que no existe: es `--env-file`).
func (d *DockerAdapter) runEnv(env map[string]string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("comando vacío")
	}
	if d.Runner != nil {
		return d.Runner(args[0], args[1:]...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
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
func (d *DockerAdapter) PS(dir string, files []string) ([]ComposeService, error) {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "ps", "--format", "{{.Service}}	{{.State}}	{{.Health}}	{{.Image}}	{{.Ports}}")
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
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

// Up levanta el stack. files es la lista de compose a usar: el
// principal y el override de la variante.
func (d *DockerAdapter) Up(dir string, files []string, env map[string]string) error {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "up", "-d", "--remove-orphans")
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
	}
	out, err := d.runEnv(env, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Down baja el stack sin borrar volúmenes.
func (d *DockerAdapter) Down(dir string, files []string) error {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "down", "--remove-orphans")
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
	}
	out, err := d.run(args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// Logs devuelve las últimas líneas de un servicio.
func (d *DockerAdapter) Logs(dir string, files []string, service string, tail int) (string, error) {
	args := []string{"docker", "compose"}
	for _, file := range composeFiles(dir, files) {
		args = append(args, "-f", file)
	}
	args = append(args, "logs", "--tail", strconv.Itoa(tail))
	if service != "" {
		args = append(args, service)
	}
	if dir != "" {
		args = append([]string{"docker", "compose", "--project-directory", dir}, args[2:]...)
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
