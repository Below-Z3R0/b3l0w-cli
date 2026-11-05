package wecli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we"
)

// cmdStill saca un cuadro de un video cualquiera y lo guarda como JPEG.
// Sirve para las miniaturas de videos locales: `ui.image` necesita una ruta
// local y no hay forma de pedirle a mpvpaper un frame.
func cmdStill(args []string) int {
	file, out, sizeArg := "", "", "320x180"
	at := 1.0

	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, bool) {
			if _, after, found := strings.Cut(a, "="); found {
				return after, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch {
		case a == "-h" || a == "--help":
			fmt.Println("uso: b3l0w we still --file VIDEO --out IMAGEN.jpg [--size 320x180] [--at 1.0]")
			return 0
		case a == "--file" || strings.HasPrefix(a, "--file="):
			v, ok := value()
			if !ok {
				return fail("--file necesita un valor")
			}
			file = v
		case a == "--out" || strings.HasPrefix(a, "--out="):
			v, ok := value()
			if !ok {
				return fail("--out necesita un valor")
			}
			out = v
		case a == "--size" || strings.HasPrefix(a, "--size="):
			v, ok := value()
			if !ok {
				return fail("--size necesita un valor")
			}
			sizeArg = v
		case a == "--at" || strings.HasPrefix(a, "--at="):
			v, ok := value()
			if !ok {
				return fail("--at necesita un valor")
			}
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil || parsed < 0 {
				return fail("--at debe ser segundos >= 0, recibí %q", v)
			}
			at = parsed
		default:
			return fail("opción desconocida: %s", a)
		}
	}

	if file == "" || out == "" {
		return fail("faltan --file y --out")
	}
	if _, err := os.Stat(file); err != nil {
		return fail("no puedo leer %s: %v", file, err)
	}
	// Un GIF suele durar menos de 1 s: buscar el segundo 1 devuelve cero frames.
	if strings.HasSuffix(strings.ToLower(file), ".gif") {
		at = 0
	}

	var w, h int
	if _, err := fmt.Sscanf(sizeArg, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
		return fail("--size debe ser ANCHOxALTO (ej 320x180), recibí %q", sizeArg)
	}

	abs, err := filepath.Abs(out)
	if err != nil {
		return fail("resolver %s: %v", out, err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fail("crear %s: %v", filepath.Dir(abs), err)
	}

	ff, err := we.Locate()
	if err != nil {
		return fail("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// -ss antes de -i busca por índice: más rápido que decodificar desde el
	// principio. -2 mantiene la proporción con alto par.
	// Un JPG/PNG ya es un cuadro: pasarle -ss hace que ffmpeg no escriba nada
	// ("Output file is empty"). Para estos se decodifica directo.
	isStill := false
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".bmp", ".avif"} {
		if strings.HasSuffix(strings.ToLower(file), ext) {
			isStill = true
			break
		}
	}

	seeks := []string{""}
	if !isStill {
		seeks = []string{"0"}
		if at > 0 {
			seeks = []string{strconv.FormatFloat(at, 'f', 2, 64), "0"}
		}
	}

	var lastErr error
	for _, seek := range seeks {
		_ = os.Remove(abs)
		cmdArgs := []string{"-y"}
		if seek != "" {
			cmdArgs = append(cmdArgs, "-ss", seek)
		}
		cmdArgs = append(cmdArgs,
			"-i", file,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale=%d:-2", w),
			"-q:v", "3",
			abs,
		)
		if err := we.Run(ctx, ff.Path, cmdArgs...); err != nil {
			// Un JPEG/GIF sin frame en ese segundo sale con error: se reintenta
			// desde el principio antes de darlo por fallido.
			lastErr = err
			continue
		}
		if info, statErr := os.Stat(abs); statErr == nil && info.Size() > 0 {
			fmt.Println(abs)
			return 0
		}
		lastErr = fmt.Errorf("ffmpeg no escribió %s", abs)
	}
	return fail("no se pudo extraer un cuadro de %s: %v", file, lastErr)
}
