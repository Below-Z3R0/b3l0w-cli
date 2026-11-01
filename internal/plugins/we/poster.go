package we

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultPosterSize es el tamaño que se usa cuando PosterOptions.Size viene en
// cero. Coincide con el default del comando `we poster`.
var DefaultPosterSize = [2]int{1280, 720}

const (
	// defaultScreenshotDelay es la cantidad de frames que espera el engine
	// antes de capturar (el flag es en frames, no en segundos: a 30 fps, 30
	// frames es un segundo). Abajo de eso suele salir un frame negro.
	defaultScreenshotDelay = 30
	// maxDefaultJobs acota el paralelismo por defecto. Cada render ocupa la
	// GPU entera y tarda decenas de segundos, así que no conviene saturar.
	maxDefaultJobs = 4
	// engineFallback es la ruta medida en esta máquina para el paquete AUR,
	// que no siempre deja el symlink en /usr/bin.
	engineFallback = "/usr/sbin/linux-wallpaperengine"
	// maxErrOutput recorta la salida del engine que se embebe en los errores
	// para no volcar líneas enteras de stderr en el JSON.
	maxErrOutput = 300
)

var (
	// renderTimeout es el techo por imagen. El engine no termina solo después
	// de capturar, así que el corte lo pone este paquete: sin techo, un
	// wallpaper web que nunca renderiza colgaría el lote.
	renderTimeout = 2 * time.Minute
	// pollInterval es cada cuánto se mira si el PNG ya está escrito.
	pollInterval = 200 * time.Millisecond
	// engineWaitDelay acota cuánto se espera el cosechado después del kill.
	// Sin esto, un hijo del engine que herede stderr deja Wait colgado aunque
	// el proceso principal ya esté muerto.
	engineWaitDelay = time.Second
)

// pngSignature son los primeros bytes de cualquier PNG.
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// PosterOptions controla la generación de PNGs vía `--screenshot`.
type PosterOptions struct {
	OutDir     string // directorio de salida; se crea si no existe
	Size       [2]int // ancho y alto del render
	Delay      int    // segundos de espera antes de capturar
	Overwrite  bool   // pisa PNGs existentes
	Jobs       int    // renders en paralelo
	EnginePath string // binario del engine; vacío = autodetectar
}

// PosterResult es el estado de un item dentro de un lote.
type PosterResult struct {
	ID      string `json:"id"`
	Path    string `json:"path,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	Err     string `json:"error,omitempty"`
}

// normalize completa los valores que vienen en cero. No valida OutDir: eso lo
// hace posterDir para poder devolver un error con contexto.
func (o PosterOptions) normalize() PosterOptions {
	if o.Size[0] <= 0 || o.Size[1] <= 0 {
		o.Size = DefaultPosterSize
	}
	if o.Delay <= 0 {
		o.Delay = defaultScreenshotDelay
	}
	if o.Jobs <= 0 {
		o.Jobs = runtime.NumCPU() / 2
		if o.Jobs > maxDefaultJobs {
			o.Jobs = maxDefaultJobs
		}
		if o.Jobs < 1 {
			o.Jobs = 1
		}
	}
	return o
}

// Poster renderiza el item a <OutDir>/<id>.png y devuelve la ruta. Si el PNG ya
// existe y no hay Overwrite lo saltea sin invocar al engine, así el comando es
// idempotente. El render corre offscreen: no mapea superficie en pantalla.
func Poster(ctx context.Context, it Item, opt PosterOptions) (string, error) {
	opt = opt.normalize()
	dir, path, err := posterTarget(it, opt)
	if err != nil {
		return "", err
	}
	if !opt.Overwrite {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path, nil
		}
	}
	engine, err := resolveEngine(opt.EnginePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("crear %s: %w", dir, err)
	}

	// Se renderiza a un temporal y se renombra al final: si el engine muere a
	// mitad de camino, el PNG bueno anterior no queda pisado a medias.
	tmp, err := os.CreateTemp(dir, "."+it.ID+".*.png")
	if err != nil {
		return "", fmt.Errorf("temporal en %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cerrar temporal %s: %w", tmpPath, err)
	}
	// El engine espera escribir un archivo nuevo, no pisar uno existente.
	if err := os.Remove(tmpPath); err != nil {
		return "", fmt.Errorf("preparar temporal %s: %w", tmpPath, err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := runEngine(ctx, engine, posterArgs(it, opt, tmpPath), tmpPath); err != nil {
		return "", fmt.Errorf("render de %s con %s: %w", it.ID, engine, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("mover poster de %s a %s: %w", it.ID, path, err)
	}
	return path, nil
}

// runEngine lanza el engine y espera a que el PNG esté escrito entero. El
// binario NO termina después de capturar: se queda renderizando el wallpaper,
// así que se lo mata apenas el archivo está completo. Ese kill es también lo
// que garantiza que no quede ningún linux-wallpaperengine vivo.
func runEngine(ctx context.Context, engine string, args []string, shotPath string) error {
	cmd := exec.CommandContext(ctx, engine, args...)
	cmd.Env = engineEnv()
	cmd.WaitDelay = engineWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("arrancar: %w", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	rctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	for {
		select {
		case err := <-waited:
			if pngComplete(shotPath) {
				return nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("abortado: %w (%s)", ctxErr, tailOutput(stderr.Bytes()))
			}
			if err != nil {
				return fmt.Errorf("el engine falló: %w (%s)", err, tailOutput(stderr.Bytes()))
			}
			return fmt.Errorf("el engine salió sin escribir el PNG (%s)", tailOutput(stderr.Bytes()))
		case <-rctx.Done():
			stopEngine(cmd, waited)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("abortado: %w (%s)", ctxErr, tailOutput(stderr.Bytes()))
			}
			return fmt.Errorf("timeout de %s sin PNG (%s)", renderTimeout, tailOutput(stderr.Bytes()))
		case <-tick.C:
			if pngComplete(shotPath) {
				stopEngine(cmd, waited)
				return nil
			}
		}
	}
}

// stopEngine mata el render y cosecha el proceso para no dejar zombies.
func stopEngine(cmd *exec.Cmd, waited chan error) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-waited
}

// pngComplete dice si el PNG ya se escribió entero: firma PNG al principio y
// chunk IEND al final. Así no se levanta un archivo a medio escribir.
func pngComplete(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < 32 {
		return false
	}
	head := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(f, head); err != nil || !bytes.Equal(head, pngSignature) {
		return false
	}
	tail := make([]byte, 12)
	if _, err := f.ReadAt(tail, st.Size()-12); err != nil {
		return false
	}
	return bytes.Contains(tail, []byte("IEND"))
}

// Posters renderiza un lote respetando Jobs. Un item que falla queda con Err
// seteado y no aborta al resto: el error de retorno es sólo para problemas de
// configuración, para una cancelación del context o cuando no se generó
// absolutamente nada. En los dos últimos casos los resultados parciales viajan
// igual, así el CLI puede reportar item por item.
func Posters(ctx context.Context, items []Item, opt PosterOptions) ([]PosterResult, error) {
	opt = opt.normalize()
	results := make([]PosterResult, len(items))
	if len(items) == 0 {
		return results, nil
	}
	dir, err := posterDir(opt)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("crear %s: %w", dir, err)
	}

	// Pre-pasada de skip: se resuelve antes de arrancar los workers para que
	// el reporte de Skipped no dependa de quién llegó primero al archivo.
	pending := make([]int, 0, len(items))
	for i, it := range items {
		path := filepath.Join(dir, it.ID+".png")
		if !opt.Overwrite {
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				results[i] = PosterResult{ID: it.ID, Path: path, Skipped: true}
				continue
			}
		}
		pending = append(pending, i)
	}
	if len(pending) == 0 {
		return results, nil
	}

	work := make(chan int)
	errc := make(chan error, len(pending))
	workers := opt.Jobs
	if workers > len(pending) {
		workers = len(pending)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				it := items[i]
				path, err := Poster(ctx, it, opt)
				if err != nil {
					results[i] = PosterResult{ID: it.ID, Err: err.Error()}
					errc <- err
					continue
				}
				results[i] = PosterResult{ID: it.ID, Path: path}
			}
		}()
	}
	for _, i := range pending {
		work <- i
	}
	close(work)
	wg.Wait()
	close(errc)

	failures := 0
	for range errc {
		failures++
	}
	if err := ctx.Err(); err != nil {
		return results, fmt.Errorf("posters interrumpidos: %w", err)
	}
	if failures == len(pending) {
		return results, fmt.Errorf("no se pudo generar ningún poster: fallaron los %d renders", failures)
	}
	return results, nil
}

// posterDir valida y absolutiza el directorio de salida.
func posterDir(opt PosterOptions) (string, error) {
	dir := strings.TrimSpace(opt.OutDir)
	if dir == "" {
		return "", fmt.Errorf("falta el directorio de salida de los posters")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolver %s: %w", dir, err)
	}
	return abs, nil
}

// posterTarget resuelve el directorio y la ruta final del PNG de un item.
func posterTarget(it Item, opt PosterOptions) (string, string, error) {
	if strings.TrimSpace(it.ID) == "" {
		return "", "", fmt.Errorf("item sin id: no puedo armar el nombre del poster")
	}
	dir, err := posterDir(opt)
	if err != nil {
		return "", "", err
	}
	return dir, filepath.Join(dir, it.ID+".png"), nil
}

// posterArgs arma la línea de comandos del engine. El formato de --window usa
// `x` como separador: con comas el binario rechaza la geometría.
func posterArgs(it Item, opt PosterOptions, shotPath string) []string {
	return []string{
		"--bg", it.ID,
		"--window", fmt.Sprintf("0x0x%dx%d", opt.Size[0], opt.Size[1]),
		"--screenshot", shotPath,
		"--screenshot-delay", strconv.Itoa(opt.Delay),
	}
}

// resolveEngine ubica el binario del engine. Un EnginePath explícito manda; si
// no, se prueba PATH y después la ruta medida en esta máquina.
func resolveEngine(explicit string) (string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if p, err := exec.LookPath(explicit); err == nil {
			return p, nil
		}
		if st, err := os.Stat(explicit); err == nil && !st.IsDir() {
			return explicit, nil
		}
		return "", fmt.Errorf("el engine indicado %q no existe o no es ejecutable", explicit)
	}
	if p, err := exec.LookPath("linux-wallpaperengine"); err == nil {
		return p, nil
	}
	if st, err := os.Stat(engineFallback); err == nil && !st.IsDir() {
		return engineFallback, nil
	}
	return "", fmt.Errorf("no encontré linux-wallpaperengine; probé PATH y %s", engineFallback)
}

// engineEnv arma el entorno del engine. Necesita un display: si no hay ninguno
// seteado se completa XDG_RUNTIME_DIR y WAYLAND_DISPLAY apuntando al compositor
// del usuario, que es lo que hay en esta máquina.
func engineEnv() []string {
	env := os.Environ()
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		rt = fmt.Sprintf("/run/user/%d", os.Getuid())
		if st, err := os.Stat(rt); err == nil && st.IsDir() {
			env = append(env, "XDG_RUNTIME_DIR="+rt)
		} else {
			return env
		}
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		if _, err := os.Stat(filepath.Join(rt, "wayland-0")); err == nil {
			env = append(env, "WAYLAND_DISPLAY=wayland-0")
		}
	}
	return env
}

// tailOutput deja la salida del engine en una sola línea corta para poder
// meterla en un mensaje de error.
func tailOutput(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if s == "" {
		return "sin salida"
	}
	if len(s) > maxErrOutput {
		s = s[:maxErrOutput] + "..."
	}
	return s
}
