package we

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Tool es un binario externo ya resuelto en disco. Ok indica si se encontró y
// Path es su ruta absoluta.
type Tool struct {
	Path string
	Ok   bool
}

// stderrTail es cuántas líneas del stderr se arrastran al mensaje de error de un
// binario que falla. Más que esto no aporta y ensucia el log.
const stderrTail = 20

// hermesToolsSub es donde este sistema guarda las copias portables de ffmpeg
// (el directorio lleva la versión en el nombre, por eso se busca con glob).
const hermesToolsSub = ".hermes/tools"

// toolDirEnv permite apuntar a mano el directorio de ffmpeg/ffprobe.
const toolDirEnv = "B3L0W_FFMPEG_DIR"

// Locate resuelve ffmpeg: primero el PATH y después las copias portables de
// ~/.hermes/tools. Si no aparece devuelve un error con las rutas probadas.
func Locate() (Tool, error) {
	return locate("ffmpeg")
}

// LocateProbe resuelve ffprobe con la misma estrategia que Locate.
func LocateProbe() (Tool, error) {
	return locate("ffprobe")
}

func locate(name string) (Tool, error) {
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		return Tool{Path: p, Ok: true}, nil
	}
	var probed []string
	for _, dir := range toolDirs() {
		cand := filepath.Join(dir, name)
		probed = append(probed, cand)
		st, err := os.Stat(cand)
		if err != nil || st.IsDir() || st.Mode().Perm()&0o111 == 0 {
			continue
		}
		return Tool{Path: cand, Ok: true}, nil
	}
	if len(probed) == 0 {
		return Tool{}, fmt.Errorf("%s no está en el PATH y no hay copias en ~/%s; instalalo o exportá %s", name, hermesToolsSub, toolDirEnv)
	}
	return Tool{}, fmt.Errorf("%s no está en el PATH ni en las copias portables; probé: %s", name, strings.Join(probed, ", "))
}

// toolDirs lista los directorios candidatos, en orden de preferencia. Si
// B3L0W_FFMPEG_DIR está seteado manda solo ese directorio: el override explícito
// no se mezcla con el descubrimiento automático. Si no, se recorre el glob de
// ~/.hermes/tools de mayor a menor, así gana la versión más nueva.
func toolDirs() []string {
	if d := strings.TrimSpace(os.Getenv(toolDirEnv)); d != "" {
		return []string{d}
	}
	var dirs []string
	home, err := os.UserHomeDir()
	if err != nil {
		return dirs
	}
	matches, err := filepath.Glob(filepath.Join(home, hermesToolsSub, "ffmpeg-*", "bin"))
	if err != nil {
		return dirs
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return append(dirs, matches...)
}

// Run ejecuta un binario sin exponer su salida y devuelve un error con la cola
// del stderr cuando el proceso falla. El stdout se descarta a propósito: los
// comandos de este paquete escriben archivos, no datos por stdout.
func Run(ctx context.Context, bin string, args ...string) error {
	_, err := runCapture(ctx, bin, args...)
	return err
}

func runCapture(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := tailLines(errb.String(), stderrTail)
		if msg == "" {
			msg = "sin stderr"
		}
		return out.Bytes(), fmt.Errorf("%s %s: %w (stderr: %s)", filepath.Base(bin), strings.Join(args, " "), err, msg)
	}
	return out.Bytes(), nil
}

// tailLines devuelve las últimas n líneas no vacías de s. Normaliza \r porque
// ffmpeg reescribe la misma línea de progreso.
func tailLines(s string, n int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Meta son los datos del stream de video que necesita el manifest.
type Meta struct {
	Duration float64
	Width    int
	Height   int
	Codec    string
}

// Probe lee duración, dimensiones y codec de un medio con ffprobe.
func Probe(ctx context.Context, path string) (Meta, error) {
	tool, err := LocateProbe()
	if err != nil {
		return Meta{}, err
	}
	out, err := runCapture(ctx, tool.Path, probeArgs(path)...)
	if err != nil {
		return Meta{}, err
	}
	m, err := parseProbe(out)
	if err != nil {
		return Meta{}, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}
	return m, nil
}

func probeArgs(path string) []string {
	return []string{"-v", "error", "-print_format", "json", "-show_streams", "-show_format", path}
}

// probeNumber acepta el número como string o como número: ffprobe manda
// duraciones como string y no queremos depender de eso.
type probeNumber float64

// UnmarshalJSON acepta el número como tal o como string (ffprobe cambia de
// criterio según el campo) y lo deja en float64.
func (n *probeNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	switch s {
	case "", "null", "N/A", "nan":
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = 0 // un campo raro no invalida el resto del probe
		return nil
	}
	*n = probeNumber(f)
	return nil
}

type probeStream struct {
	CodecType string      `json:"codec_type"`
	CodecName string      `json:"codec_name"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	Duration  probeNumber `json:"duration"`
}

type probeDoc struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration probeNumber `json:"duration"`
	} `json:"format"`
}

// parseProbe interpreta la salida JSON de ffprobe y se queda con el primer
// stream de video.
func parseProbe(data []byte) (Meta, error) {
	var doc probeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Meta{}, fmt.Errorf("parsear salida de ffprobe: %w", err)
	}
	for _, s := range doc.Streams {
		if s.CodecType != "video" {
			continue
		}
		m := Meta{
			Duration: float64(doc.Format.Duration),
			Width:    s.Width,
			Height:   s.Height,
			Codec:    s.CodecName,
		}
		if m.Duration == 0 {
			m.Duration = float64(s.Duration)
		}
		return m, nil
	}
	return Meta{}, fmt.Errorf("el archivo no tiene stream de video")
}
