package wecli

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parseHexColor acepta "#rrggbb", "rrggbb", "#rgb" y devuelve RGBA opaco.
func parseHexColor(value string) (color.RGBA, error) {
	v := strings.TrimPrefix(strings.TrimSpace(value), "#")
	switch len(v) {
	case 3:
		v = string([]byte{v[0], v[0], v[1], v[1], v[2], v[2]})
	case 6:
	default:
		return color.RGBA{}, fmt.Errorf("color inválido %q (esperaba #rrggbb)", value)
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("color inválido %q", value)
	}
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
}

// cmdSolid genera un PNG de un solo color. Noctalia no expone un "wallpaper de
// color" en la API de plugins, así que el plugin apunta setWallpaper a este
// archivo: es lo mismo que hace el panel built-in al aplicar un color.
//
//	uso: b3l0w we solid --color '#1e1e2e' --out archivo.png [--size 3840x2160]
func cmdSolid(args []string) int {
	hex := ""
	out := ""
	width, height := 3840, 2160

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
			fmt.Println("uso: b3l0w we solid --color '#rrggbb' --out archivo.png [--size 3840x2160]")
			return 0
		case a == "--color" || strings.HasPrefix(a, "--color="):
			v, ok := value()
			if !ok {
				return fail("--color necesita un valor")
			}
			hex = v
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
			w, h, found := strings.Cut(strings.ToLower(v), "x")
			if !found {
				return fail("--size inválido: %s", v)
			}
			pw, err1 := strconv.Atoi(w)
			ph, err2 := strconv.Atoi(h)
			if err1 != nil || err2 != nil || pw <= 0 || ph <= 0 {
				return fail("--size inválido: %s", v)
			}
			width, height = pw, ph
		}
	}

	if hex == "" || out == "" {
		return fail("hacen falta --color y --out")
	}
	rgba, err := parseHexColor(hex)
	if err != nil {
		return fail("%v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, rgba)
		}
	}

	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail("no se pudo crear %s: %v", dir, err)
		}
	}
	file, err := os.Create(out)
	if err != nil {
		return fail("no se pudo escribir %s: %v", out, err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		return fail("png: %v", err)
	}
	fmt.Println(out)
	return 0
}
