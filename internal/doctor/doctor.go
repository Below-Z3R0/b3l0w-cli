// Package doctor chequea que la máquina pueda hacer su trabajo.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins"
	"github.com/Below-Z3R0/b3l0w-cli/internal/programs"
	"github.com/Below-Z3R0/b3l0w-cli/internal/releases"
	"github.com/Below-Z3R0/b3l0w-cli/internal/system"

	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
	"github.com/Below-Z3R0/b3l0w-cli/kit/paths"
	kitsystem "github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// Level es la severidad del resultado de un chequeo.
type Level int

const (
	// LevelOk: el chequeo pasó.
	LevelOk Level = iota
	// LevelWarn: hay un aviso; la máquina sigue sirviendo.
	LevelWarn
	// LevelError: hay un error; el comando sale con código 1.
	LevelError
)

// Result es el resultado de un chequeo.
type Result struct {
	// Level dice si es un ok, un aviso o un error.
	Level Level
	// Message es la línea que se muestra.
	Message string
}

// Check es un chequeo del doctor.
//
// Run devuelve los resultados que hay que mostrar y cuántos de ellos
// son errores. Los avisos no cuentan: el comando sale con 0 igual.
type Check struct {
	// Name identifica el chequeo.
	Name string
	// Run hace el chequeo.
	Run func() ([]Result, int)
}

// Env son las dependencias del doctor: todo lo que mira la máquina
// entra por acá.
//
// Los tests inyectan un Env falso y así los chequeos corren sin tocar
// el sistema: sin pacman, sin noctalia y sin red.
type Env struct {
	// SystemBinary devuelve el actualizador del sistema, o "".
	SystemBinary func() string
	// CommandPath devuelve la ruta de un binario, o "" si no está.
	CommandPath func(name string) string
	// IsTTY dice si hay terminal interactiva.
	IsTTY func() bool
	// ProgramStatus devuelve el estado de los programas del manifiesto.
	ProgramStatus func(m *config.Manifest) []programs.Status
	// PluginScripts lista los backends embebidos.
	PluginScripts func() ([]string, error)
	// HasSource dice si Noctalia tiene ese source de plugins.
	HasSource func(name string) bool
	// AutoUpdate lee plugins.auto_update de la config de Noctalia.
	AutoUpdate func() string
	// Dependencies devuelve las dependencias declaradas por los plugins.
	Dependencies func() (map[string][]string, error)
	// ReleaseStatus devuelve el estado de firma de los releases.
	ReleaseStatus func(m *config.Manifest) []releases.Status
	// PluginsDir devuelve el directorio de extensiones.
	PluginsDir func() string
	// HasDependency dice si una dependencia está disponible.
	HasDependency func(name string) bool
}

// defaultEnv son las dependencias reales: la máquina de verdad.
func defaultEnv() *Env {
	return &Env{
		SystemBinary:  system.Binary,
		CommandPath:   kitsystem.CommandPath,
		IsTTY:         kitsystem.IsTTY,
		ProgramStatus: programs.Statuses,
		PluginScripts: plugins.Scripts,
		HasSource:     plugins.HasSource,
		AutoUpdate:    plugins.AutoUpdate,
		Dependencies:  plugins.Dependencies,
		ReleaseStatus: releases.Check,
		PluginsDir:    paths.PluginsDir,
		HasDependency: hasDependency,
	}
}

// Run devuelve 0 si no hay errores (los avisos no cuentan).
func Run(m *config.Manifest) int {
	return RunWith(m, defaultEnv())
}

// RunWith corre los chequeos contra el env dado.
//
// El env es la costura para los tests: con un env falso la suite
// corre entera sin tocar la máquina.
func RunWith(m *config.Manifest, env *Env) int {
	output.Step("Chequeos")

	errores := 0
	for _, c := range env.checks(m) {
		resultados, extra := c.Run()
		for _, r := range resultados {
			switch r.Level {
			case LevelError:
				output.Fail(r.Message)
				errores++
			case LevelWarn:
				output.Warn(r.Message)
			default:
				output.Ok(r.Message)
			}
		}
		errores += extra
	}

	if errores > 0 {
		output.Fail(fmt.Sprintf("%d chequeos fallaron", errores))
		return 1
	}
	return 0
}

// checks arma la lista de chequeos, en el orden en que se corren.
func (e *Env) checks(m *config.Manifest) []Check {
	return []Check{
		{
			// El actualizador del sistema: sin él, media herramienta no sirve.
			Name: "actualizador",
			Run: func() ([]Result, int) {
				if bin := e.SystemBinary(); bin != "" {
					return []Result{{Level: LevelOk, Message: fmt.Sprintf("actualizador del sistema: %s", bin)}}, 0
				}
				return []Result{{Level: LevelError, Message: "no hay cachy-update ni arch-update"}}, 1
			},
		},
		{
			// pacman: lo necesita el chequeo de "es de un paquete".
			Name: "pacman",
			Run: func() ([]Result, int) {
				if path := e.CommandPath("pacman"); path != "" {
					return []Result{{Level: LevelOk, Message: fmt.Sprintf("pacman: %s", path)}}, 0
				}
				return []Result{{Level: LevelError, Message: "no hay pacman: esto no es un sistema Arch"}}, 1
			},
		},
		{
			// Terminal: el update del sistema la necesita.
			Name: "terminal",
			Run: func() ([]Result, int) {
				if e.IsTTY() {
					return []Result{{Level: LevelOk, Message: "terminal interactiva"}}, 0
				}
				return []Result{{Level: LevelWarn, Message: "sin terminal: b3l0w update solo podrá actualizar programas"}}, 0
			},
		},
		{
			// Programas del manifiesto.
			Name: "programas",
			Run: func() ([]Result, int) {
				statuses := e.ProgramStatus(m)
				if len(statuses) == 0 {
					return []Result{{Level: LevelWarn, Message: "el manifiesto no tiene programas"}}, 0
				}
				var out []Result
				for _, status := range statuses {
					switch {
					case !status.Installed:
						out = append(out, Result{Level: LevelWarn, Message: fmt.Sprintf("%s: no instalado", status.Name)})
					case status.Err != "":
						out = append(out, Result{Level: LevelError, Message: fmt.Sprintf("%s: %s", status.Name, status.Err)})
					default:
						out = append(out, Result{Level: LevelOk, Message: fmt.Sprintf("%s: %s", status.Name, status.Version)})
					}
				}
				return out, countErrors(out)
			},
		},
		{
			// Directorio de extensiones: si existe, tiene que ser privado.
			Name: "plugins",
			Run: func() ([]Result, int) {
				dir := e.PluginsDir()
				info, err := os.Stat(dir)
				switch {
				case os.IsNotExist(err):
					return []Result{{Level: LevelWarn, Message: fmt.Sprintf("plugins: %s todavía no existe", dir)}}, 0
				case err != nil:
					return []Result{{Level: LevelError, Message: fmt.Sprintf("plugins: %v", err)}}, 1
				case info.Mode().Perm()&0o077 != 0:
					return []Result{{Level: LevelError, Message: fmt.Sprintf("plugins: %s es accesible por otros (modo %o)", dir, info.Mode().Perm())}}, 1
				default:
					return []Result{{Level: LevelOk, Message: fmt.Sprintf("plugins: %s (privado)", dir)}}, 0
				}
			},
		},
		{
			// Backends embebidos: viven en el binario, no hay copia que chequear.
			Name: "backends",
			Run: func() ([]Result, int) {
				names, err := e.PluginScripts()
				if err != nil {
					return []Result{{Level: LevelWarn, Message: fmt.Sprintf("backends: no pude leer los scripts embebidos: %v", err)}}, 0
				}
				out := make([]Result, 0, len(names))
				for _, name := range names {
					out = append(out, Result{Level: LevelOk, Message: fmt.Sprintf("backend embebido: %s", name)})
				}
				return out, 0
			},
		},
		{
			// Noctalia y su source de plugins.
			Name: "noctalia",
			Run: func() ([]Result, int) {
				if e.CommandPath("noctalia") == "" {
					return []Result{{Level: LevelWarn, Message: "noctalia no está en PATH: los plugins no se actualizan"}}, 0
				}
				var out []Result
				if e.HasSource(plugins.SourceName) {
					out = append(out, Result{Level: LevelOk, Message: "noctalia: source " + plugins.SourceName + " presente"})
				} else {
					out = append(out, Result{Level: LevelWarn, Message: "noctalia: falta el source " + plugins.SourceName + " (b3l0w update --plugins)"})
				}
				switch v := e.AutoUpdate(); v {
				case "none":
					out = append(out, Result{Level: LevelOk, Message: "plugins: auto-update off (lo controla b3l0w)"})
				case "":
					out = append(out, Result{Level: LevelWarn, Message: "plugins: no pude leer auto_update"})
				default:
					out = append(out, Result{Level: LevelError, Message: fmt.Sprintf("plugins: auto_update=%q; poné \"none\" para que b3l0w controle las actualizaciones", v)})
				}
				return out, countErrors(out)
			},
		},
		{
			// Dependencias declaradas por los plugins (gh, mpv, etc.).
			Name: "dependencias",
			Run: func() ([]Result, int) {
				deps, err := e.Dependencies()
				if err != nil {
					return []Result{{Level: LevelWarn, Message: fmt.Sprintf("dependencias: no pude leerlas: %v", err)}}, 0
				}
				var ids []string
				for id := range deps {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				var out []Result
				for _, id := range ids {
					for _, dep := range deps[id] {
						if !e.HasDependency(dep) {
							out = append(out, Result{Level: LevelWarn, Message: fmt.Sprintf("%s: falta dependencia %s", id, dep)})
						}
					}
				}
				return out, 0
			},
		},
		{
			// Firmas de los releases: sin SHA256SUMS.asc, install y
			// update --self se niegan a instalar.
			//
			// releases.Render imprime su propia sección y devuelve
			// cuántos releases están sin firma.
			Name: "firmas",
			Run: func() ([]Result, int) {
				return nil, releases.Render(e.ReleaseStatus(m))
			},
		},
	}
}

// countErrors cuenta los resultados que son error.
func countErrors(resultados []Result) int {
	n := 0
	for _, r := range resultados {
		if r.Level == LevelError {
			n++
		}
	}
	return n
}

// hasDependency busca un comando en PATH o un paquete instalado.
func hasDependency(name string) bool {
	if kitsystem.CommandPath(name) != "" {
		return true
	}
	return exec.Command("pacman", "-Q", name).Run() == nil
}
