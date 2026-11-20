package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Action es el resultado de una operación sobre un recurso.
type Action struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
}

func ejecutar(ctx context.Context, run Runner, args ...string) (Action, error) {
	run = runnerDe(run)
	comando := "docker " + strings.Join(args, " ")
	salida, err := run(ctx, args...)
	if err != nil {
		return Action{OK: false, Command: comando, Output: err.Error()}, err
	}
	return Action{OK: true, Command: comando, Output: strings.TrimSpace(salida)}, nil
}

// AllowedActions es la lista de acciones que el CLI puede ejecutar.
//
// El CLI la consulta antes de correr una acción: lo que no está en esta
// lista tiene que fallar en vez de ejecutar algo no previsto (D17).
var AllowedActions = map[string]bool{
	"start": true, "stop": true, "restart": true, "remove": true,
	"rmi": true, "volume-rm": true, "network-rm": true, "run": true,
	"pause": true, "unpause": true, "pull": true, "push": true,
	"volume-create": true, "network-create": true,
	"save": true, "load": true, "export": true, "diff": true,
	"build": true, "commit": true, "rename": true, "tag": true,
	"cp": true, "exec": true, "update": true, "prune": true,
	"volume-export": true, "volume-import": true, "compose": true,
}

// idInvalido dice si un identificador no se puede pasar a docker: uno que
// empieza con "-" lo tomaría docker como un flag suyo (--privileged,
// --user root) y no como el nombre de un recurso (D12).
func idInvalido(id string) bool {
	return strings.HasPrefix(id, "-")
}

// idRechazado arma el error de una acción con un id no válido.
func idRechazado(accion string, id string) (Action, error) {
	msg := fmt.Sprintf("%s: el id no puede empezar con '-': %q", accion, id)
	return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
}

// Start arranca un contenedor detenido y devuelve la salida de docker.
func Start(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("start", id)
	}
	return ejecutar(ctx, run, "start", id)
}

// Stop detiene un contenedor en ejecución (SIGTERM, con el timeout de docker).
func Stop(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("stop", id)
	}
	return ejecutar(ctx, run, "stop", id)
}

// Restart reinicia un contenedor sin recrearlo.
func Restart(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("restart", id)
	}
	return ejecutar(ctx, run, "restart", id)
}

// Remove acepta -f porque un contenedor parado a veces tiene red en uso.
func Remove(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("remove", id)
	}
	return ejecutar(ctx, run, "rm", "-f", id)
}

// RemoveImage borra una imagen; falla si algún contenedor la usa.
func RemoveImage(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("rmi", id)
	}
	return ejecutar(ctx, run, "rmi", id)
}

// RemoveVolume borra un volumen; falla si está montado.
func RemoveVolume(ctx context.Context, run Runner, name string) (Action, error) {
	if idInvalido(name) {
		return idRechazado("volume-rm", name)
	}
	return ejecutar(ctx, run, "volume", "rm", name)
}

// RemoveNetwork se niega con las redes nativas: docker también, pero el mensaje
// de acá dice por qué.
func RemoveNetwork(ctx context.Context, run Runner, name string) (Action, error) {
	if idInvalido(name) {
		return idRechazado("network-rm", name)
	}
	switch name {
	case "bridge", "host", "none":
		msg := fmt.Sprintf("la red %s es nativa de Docker", name)
		return Action{OK: false, Command: "docker network rm " + name, Output: msg}, fmt.Errorf("%s", msg)
	}
	return ejecutar(ctx, run, "network", "rm", name)
}

// RunOptions son los datos del formulario "correr imagen".
// Pull descarga una imagen del registro.
func Pull(ctx context.Context, run Runner, imagen string) (Action, error) {
	if idInvalido(imagen) {
		return idRechazado("pull", imagen)
	}
	return ejecutar(ctx, run, "pull", imagen)
}

// Push sube una imagen al registro.
func Push(ctx context.Context, run Runner, imagen string) (Action, error) {
	if idInvalido(imagen) {
		return idRechazado("push", imagen)
	}
	return ejecutar(ctx, run, "push", imagen)
}

// CreateVolume crea un volumen nuevo.
func CreateVolume(ctx context.Context, run Runner, nombre string) (Action, error) {
	if idInvalido(nombre) {
		return idRechazado("volume-create", nombre)
	}
	return ejecutar(ctx, run, "volume", "create", nombre)
}

// CreateNetwork crea una red nueva.
func CreateNetwork(ctx context.Context, run Runner, nombre string) (Action, error) {
	if idInvalido(nombre) {
		return idRechazado("network-create", nombre)
	}
	return ejecutar(ctx, run, "network", "create", nombre)
}

// SaveImage exporta una imagen a un tar.
func SaveImage(ctx context.Context, run Runner, imagen string, destino string) (Action, error) {
	if idInvalido(imagen) {
		return idRechazado("save", imagen)
	}
	return ejecutar(ctx, run, "save", "-o", destino, imagen)
}

// LoadImage importa una imagen desde un tar.
func LoadImage(ctx context.Context, run Runner, origen string) (Action, error) {
	if idInvalido(origen) {
		return idRechazado("load", origen)
	}
	return ejecutar(ctx, run, "load", "-i", origen)
}

// ExportContainer exporta el sistema de archivos de un contenedor a un tar.
func ExportContainer(ctx context.Context, run Runner, id string, destino string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("export", id)
	}
	return ejecutar(ctx, run, "export", "-o", destino, id)
}

// Diff muestra qué cambió en el sistema de archivos del contenedor.
func Diff(ctx context.Context, run Runner, id string) (Action, error) {
	if idInvalido(id) {
		return idRechazado("diff", id)
	}
	return ejecutar(ctx, run, "diff", id)
}

type RunOptions struct {
	Image   string   `json:"image"`
	Name    string   `json:"name"`
	Network string   `json:"network"`
	Ports   []string `json:"ports"` // "8080:80"
	Env     []string `json:"env"`   // "KEY=value"
	Detach  bool     `json:"detach"`
}

// RunImage arma `docker run` sólo con lo que el usuario cargó.
//
// El original forzaba `-p P:P` (mismo puerto adentro y afuera) e ignoraba la
// sintaxis host:contenedor; acá se pasa tal cual y se valida el formato.
func RunImage(ctx context.Context, run Runner, opts RunOptions) (Action, error) {
	if strings.TrimSpace(opts.Image) == "" {
		return Action{OK: false, Output: "falta la imagen"}, fmt.Errorf("falta la imagen")
	}
	args := []string{"run"}
	// `Detach` se respeta: la condición era `opts.Detach || true`, que
	// siempre era verdadera y dejaba el campo decorativo (D8).
	if opts.Detach {
		args = append(args, "-d")
	}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}
	for _, puerto := range opts.Ports {
		if !puertoValido(puerto) {
			msg := fmt.Sprintf("puerto inválido %q: usá host:contenedor", puerto)
			return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
		}
		args = append(args, "-p", puerto)
	}
	for _, variable := range opts.Env {
		if !strings.Contains(variable, "=") {
			msg := fmt.Sprintf("variable inválida %q: usá CLAVE=valor", variable)
			return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
		}
		args = append(args, "-e", variable)
	}
	args = append(args, opts.Image)
	return ejecutar(ctx, run, args...)
}

// puertoValido acepta 8080:80, 127.0.0.1:8080:80/tcp y 80.
func puertoValido(valor string) bool {
	if valor == "" {
		return false
	}
	if idx := strings.LastIndex(valor, "/"); idx >= 0 {
		valor = valor[:idx]
	}
	partes := strings.Split(valor, ":")
	if len(partes) == 1 {
		_, err := strconv.Atoi(partes[0])
		return err == nil
	}
	if len(partes) < 2 {
		return false
	}
	for _, parte := range partes[len(partes)-2:] {
		if _, err := strconv.Atoi(parte); err != nil {
			return false
		}
	}
	return true
}

// Logs devuelve las últimas líneas de un contenedor (texto, no JSON).
func Logs(ctx context.Context, run Runner, id string, tail int) (string, error) {
	run = runnerDe(run)
	if tail <= 0 {
		tail = 200
	}
	return run(ctx, "logs", "--tail", strconv.Itoa(tail), id)
}

// Build construye una imagen desde un Dockerfile.
//
// Es la única acción que escribe en disco (deja la imagen en el daemon),
// así que valida lo que puede: la etiqueta tiene que tener forma de
// referencia de imagen, y el contexto y el Dockerfile son rutas, no
// argumentos sueltos que se puedan colar como opciones de docker.
func Build(ctx context.Context, run Runner, etiqueta string, contexto string, dockerfile string) (Action, error) {
	etiqueta = strings.TrimSpace(etiqueta)
	if etiqueta == "" {
		return Action{OK: false, Output: "build: falta la etiqueta de la imagen"}, fmt.Errorf("build: falta la etiqueta de la imagen")
	}
	if strings.HasPrefix(etiqueta, "-") {
		msg := fmt.Sprintf("build: la etiqueta no puede empezar con '-': %q", etiqueta)
		return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
	}
	contexto = strings.TrimSpace(contexto)
	if contexto == "" {
		contexto = "."
	}
	if strings.HasPrefix(contexto, "-") {
		msg := fmt.Sprintf("build: el contexto no puede empezar con '-': %q", contexto)
		return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
	}
	args := []string{"build", "-t", etiqueta}
	dockerfile = strings.TrimSpace(dockerfile)
	if dockerfile != "" {
		if strings.HasPrefix(dockerfile, "-") {
			msg := fmt.Sprintf("build: el Dockerfile no puede empezar con '-': %q", dockerfile)
			return Action{OK: false, Output: msg}, fmt.Errorf("%s", msg)
		}
		args = append(args, "-f", dockerfile)
	}
	args = append(args, contexto)
	return ejecutar(ctx, run, args...)
}
