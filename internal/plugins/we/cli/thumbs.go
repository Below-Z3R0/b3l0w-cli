package wecli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Below-Z3R0/b3l0w/internal/plugins/we"
)

// thumbName reproduce el nombre que arma el plugin en Luau
// (`video_thumb_path`): djb2 de 32 bits de la ruta, en hex, con prefijo `v-`.
// Si cambia acá, el plugin deja de encontrar las miniaturas ya generadas.
func thumbName(path string) string {
	var hash uint32 = 5381
	for i := 0; i < len(path); i++ {
		hash = hash*33 + uint32(path[i])
	}
	return fmt.Sprintf("v-%x.jpg", hash)
}

// makeThumbs genera una miniatura por imagen y la deja en `dir`.
//
// Un tile con la imagen original obliga al host a decodificar un PNG 4K por
// tarjeta; con 55 fondos eso son cientos de MB. ffmpeg baja cada una a 320 px
// de ancho en milisegundos y el resultado se reusa mientras el original no
// cambie de fecha.
func makeThumbs(items []scanItem, dir string, width int) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0
	}
	ff, err := we.Locate()
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Cuatro ffmpeg en paralelo: decodificar un PNG 4K tarda ~250 ms y en serie
	// una carpeta de 55 fondos bloquea el catálogo 14 s.
	const workers = 4
	pendientes := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	hechas := 0

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range pendientes {
				out := items[i].Thumb
				if fresh(out, items[i].File) {
					mu.Lock()
					hechas++
					mu.Unlock()
					continue
				}
				args := []string{"-v", "error", "-y", "-threads", "1", "-i", items[i].File,
					"-vf", fmt.Sprintf("scale=%d:-2", width), "-q:v", "5", out}
				if err := exec.CommandContext(ctx, ff.Path, args...).Run(); err != nil {
					// Sin miniatura la tarjeta muestra el placeholder.
					items[i].Thumb = ""
					continue
				}
				mu.Lock()
				hechas++
				mu.Unlock()
			}
		}()
	}
	for i := range items {
		items[i].Thumb = filepath.Join(dir, thumbName(items[i].File))
		pendientes <- i
	}
	close(pendientes)
	wg.Wait()
	return hechas
}

// fresh indica si la miniatura existe y es más nueva que el original.
func fresh(thumb, source string) bool {
	ti, err := os.Stat(thumb)
	if err != nil {
		return false
	}
	si, err := os.Stat(source)
	if err != nil {
		return false
	}
	return ti.ModTime().After(si.ModTime()) && ti.Size() > 0
}
