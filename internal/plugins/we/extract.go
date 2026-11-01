package we

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Métodos de extract, tal como se reportan en el manifest.
const (
	MethodCopy      = "copy"
	MethodTranscode = "transcode"
)

// ExtractOptions controla el camino rápido de `we convert`.
type ExtractOptions struct {
	OutDir    string // directorio de salida; se crea si no existe
	Format    string // mp4 (default) | webm
	CRF       int    // calidad de x264/vp9; 0 = copia si el origen ya es mp4
	Overwrite bool   // sobreescribir el destino si ya existe
}

// Result describe el archivo que dejó Extract.
type Result struct {
	Path    string // ruta absoluta del archivo de salida
	Bytes   int64  // tamaño final en disco (o del existente si se saltó)
	Skipped bool   // el destino existía y no se pidió Overwrite
	Method  string // copy | transcode
}

// Extract saca el video de un wallpaper tipo `video` sin tocar el engine:
// copia el archivo si ya sirve tal cual, o lo transcodifica con ffmpeg. Nunca
// sobreescribe sin Overwrite; en ese caso devuelve Result{Skipped: true}.
func Extract(ctx context.Context, it Item, opt ExtractOptions) (Result, error) {
	var res Result
	if it.Type != "video" {
		return res, fmt.Errorf("el wallpaper %q es de tipo %q: extract sólo sirve para video, usá el método record", it.ID, it.Type)
	}
	if strings.TrimSpace(it.File) == "" {
		return res, fmt.Errorf("el wallpaper %q no tiene archivo reproducible en project.json", it.ID)
	}
	if strings.TrimSpace(opt.OutDir) == "" {
		return res, fmt.Errorf("falta el directorio de salida")
	}
	if opt.CRF < 0 {
		return res, fmt.Errorf("CRF inválido: %d", opt.CRF)
	}
	format, err := normalizeFormat(opt.Format)
	if err != nil {
		return res, err
	}
	outDir, err := filepath.Abs(opt.OutDir)
	if err != nil {
		return res, fmt.Errorf("resolver %s: %w", opt.OutDir, err)
	}
	src, err := filepath.Abs(it.File)
	if err != nil {
		return res, fmt.Errorf("resolver %s: %w", it.File, err)
	}
	st, err := os.Stat(src)
	if err != nil {
		return res, fmt.Errorf("stat %s: %w", src, err)
	}
	if st.IsDir() {
		return res, fmt.Errorf("%s es un directorio, no un archivo de video", src)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return res, fmt.Errorf("crear %s: %w", outDir, err)
	}

	dest := filepath.Join(outDir, OutputName(it, format))
	res.Path = dest
	if existing, err := os.Stat(dest); err == nil {
		if !opt.Overwrite {
			res.Skipped = true
			res.Bytes = existing.Size()
			return res, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return res, fmt.Errorf("stat %s: %w", dest, err)
	}

	if copyable(src, format, opt.CRF) {
		n, err := copyFile(src, dest, st)
		if err != nil {
			return res, err
		}
		res.Bytes = n
		res.Method = MethodCopy
		return res, nil
	}

	n, err := transcode(ctx, src, dest, format, opt.CRF)
	if err != nil {
		return res, err
	}
	res.Bytes = n
	res.Method = MethodTranscode
	return res, nil
}

// copyable decide si el origen se puede copiar tal cual: mismo contenedor y sin
// recodificar. Es la regla dura del camino rápido (0 GPU, ~1 s).
func copyable(src, format string, crf int) bool {
	return format == "mp4" && crf == 0 && strings.EqualFold(filepath.Ext(src), ".mp4")
}

func normalizeFormat(f string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(f, "."))) {
	case "", "mp4":
		return "mp4", nil
	case "webm":
		return "webm", nil
	default:
		return "", fmt.Errorf("formato %q no soportado: usá mp4 o webm", f)
	}
}

// copyFile copia byte a byte preservando modo y mtime. Escribe a un temporal y
// renombra, así el destino nunca queda a medias.
func copyFile(src, dest string, st os.FileInfo) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("abrir %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".b3l0w-*"+filepath.Ext(dest))
	if err != nil {
		return 0, fmt.Errorf("crear temporal en %s: %w", filepath.Dir(dest), err)
	}
	tmpName := tmp.Name()
	pending := true
	defer func() {
		if pending {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	n, err := io.Copy(tmp, in)
	if err != nil {
		return 0, fmt.Errorf("copiar %s: %w", src, err)
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("sincronizar copia de %s: %w", src, err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("cerrar copia de %s: %w", src, err)
	}
	if err := os.Chmod(tmpName, st.Mode().Perm()); err != nil {
		return 0, fmt.Errorf("permisos de %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return 0, fmt.Errorf("renombrar %s a %s: %w", tmpName, dest, err)
	}
	pending = false
	_ = os.Chtimes(dest, time.Now(), st.ModTime())
	return n, nil
}

// transcode recodifica con ffmpeg a un temporal con la extensión final y
// renombra. Sin audio (-an): un wallpaper no lo usa y evita copias muertas.
func transcode(ctx context.Context, src, dest, format string, crf int) (int64, error) {
	tool, err := Locate()
	if err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".b3l0w-*."+format)
	if err != nil {
		return 0, fmt.Errorf("crear temporal en %s: %w", filepath.Dir(dest), err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("cerrar temporal %s: %w", tmpName, err)
	}

	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-i", src,
	}
	switch format {
	case "webm":
		args = append(args, "-c:v", "libvpx-vp9", "-crf", strconv.Itoa(crf), "-b:v", "0", "-an")
	default:
		args = append(args, "-c:v", "libx264", "-crf", strconv.Itoa(crf), "-preset", "veryfast", "-pix_fmt", "yuv420p", "-an")
	}
	args = append(args, tmpName)

	if err := Run(ctx, tool.Path, args...); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("transcodificar %s: %w", filepath.Base(src), err)
	}
	out, err := os.Stat(tmpName)
	if err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("stat %s: %w", tmpName, err)
	}
	if out.Size() == 0 {
		os.Remove(tmpName)
		return 0, fmt.Errorf("ffmpeg no escribió nada para %s", filepath.Base(src))
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("permisos de %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("renombrar %s a %s: %w", tmpName, dest, err)
	}
	return out.Size(), nil
}
