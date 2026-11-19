package ytmusic

// Tests de los hallazgos CRÍTICOS y ALTOS. Cada uno tiene que FALLAR si se
// revierte su arreglo:
//
//   - Y1: `stop` valida la identidad del PID (starttime) antes de matar.
//   - Y2: el socket se deriva del dataDir; el del JSON no se usa ni se borra.
//   - Y3: `findRenderer` camina el map en orden estable.
//   - Y4: el rebase de `next` no escribe la cola truncada antes del merge.
//   - Y5: `writeJSON` usa un temporal único por escritor.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// TestStopNoMataUnPIDQueNoEsNuestro fija Y1: el PID del player.json puede
// haberse reciclado. Si el starttime no coincide, no se manda señal.
func TestStopNoMataUnPIDQueNoEsNuestro(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("/bin/sleep", "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("arrancando el proceso ajeno: %v", err)
	}
	pid := cmd.Process.Pid
	fin := make(chan struct{})
	go func() { _ = cmd.Wait(); close(fin) }()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-fin
	})
	if !alive(pid) {
		t.Fatal("el proceso ajeno no arrancó")
	}
	// starttime 1 no es el de ningún proceso vivo: simula el reciclado.
	if err := writeJSON(playerPath(dir), playerFile{
		PID:       pid,
		VideoID:   "abc",
		Socket:    socketPath(dir),
		StartTime: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Stop(dir); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !alive(pid) {
		t.Fatal("Stop mató un proceso que no es nuestro (Y1)")
	}
	if _, err := os.Stat(playerPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no limpió player.json: %v", err)
	}
}

// TestNoSeBorraElArchivoQueDiceElSocket fija Y2: ni `stop` ni `status` pueden
// borrar el path que trae el campo `socket` de player.json.
func TestNoSeBorraElArchivoQueDiceElSocket(t *testing.T) {
	casos := []struct {
		nombre string
		accion func(string)
	}{
		{"stop", func(dir string) { _ = Stop(dir) }},
		{"status", func(dir string) { _, _ = Status(dir) }},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			dir := t.TempDir()
			victima := filepath.Join(t.TempDir(), "importante.txt")
			if err := os.WriteFile(victima, []byte("no tocar"), 0o600); err != nil {
				t.Fatal(err)
			}
			// PID 0: nadie vivo, así que el comando limpia el estado.
			if err := writeJSON(playerPath(dir), playerFile{
				PID:     0,
				VideoID: "abc",
				Socket:  victima,
			}); err != nil {
				t.Fatal(err)
			}
			caso.accion(dir)
			if _, err := os.Stat(victima); err != nil {
				t.Fatalf("%s borró un archivo ajeno (Y2): %v", caso.nombre, err)
			}
		})
	}
}

// TestFindRendererEsDeterminista fija Y3: el mismo payload tiene que dar
// siempre el mismo token, no uno distinto por el orden aleatorio del map.
func TestFindRendererEsDeterminista(t *testing.T) {
	payload := map[string]any{
		"zzz": map[string]any{"nextRadioContinuationData": map[string]any{"continuation": "TOKEN-C"}},
		"aaa": map[string]any{"nextRadioContinuationData": map[string]any{"continuation": "TOKEN-A"}},
		"mmm": map[string]any{"nextRadioContinuationData": map[string]any{"continuation": "TOKEN-B"}},
	}
	primero := continuationOf(payload)
	if primero == "" {
		t.Fatal("no encontró ningún token")
	}
	for i := 0; i < 200; i++ {
		if got := continuationOf(payload); got != primero {
			t.Fatalf("iteración %d: %q != %q: findRenderer camina el map en orden aleatorio (Y3)", i, got, primero)
		}
	}
}

// TestElRebaseNoTruncaLaColaEnDisco fija Y4: si el guardado final falla, la
// sesión vieja tiene que quedar entera, no reemplazada por la tanda nueva.
func TestElRebaseNoTruncaLaColaEnDisco(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, radioPayload([]any{
			panelSong("nueva1", "Nueva 1", "Artista", "3:00"),
			panelSong("nueva2", "Nueva 2", "Artista", "3:00"),
		}, "TOKEN-NUEVO"))
	})
	dir := t.TempDir()
	sessionFixture(t, dir, Session{
		ID:     "s1",
		Seed:   "seed01",
		Queue:  []Song{{VideoID: "a"}, {VideoID: "b"}, {VideoID: "c"}},
		Cursor: 2,
	})
	// El guardado final no puede completarse: current.json pasa a ser un
	// directorio y el rename falla.
	current := filepath.Join(dir, "current.json")
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := Next(context.Background(), fake.client, dir, "s1"); err == nil {
		t.Fatal("Next tenía que fallar: el guardado final no puede escribir")
	}
	// La cola vieja sigue en disco: no se truncó con la tanda nueva.
	disco := loadRawSession(t, dir, "s1")
	if len(disco.Queue) < 3 {
		t.Fatalf("la cola en disco quedó en %d canciones: el rebase la truncó (Y4)", len(disco.Queue))
	}
	vistos := map[string]bool{}
	for _, song := range disco.Queue {
		vistos[song.VideoID] = true
	}
	for _, id := range []string{"a", "b", "c"} {
		if !vistos[id] {
			t.Errorf("la canción %q desapareció de la sesión en disco (Y4)", id)
		}
	}
}

// TestWriteJSONConcurrenteNoPierdeEscrituras fija Y5: con un `.tmp` de nombre
// fijo dos escritores se pisan y uno falla con ENOENT.
func TestWriteJSONConcurrenteNoPierdeEscrituras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "estado.json")
	relleno := strings.Repeat("x", 256*1024)

	const escritores, vueltas = 8, 20
	var wg sync.WaitGroup
	errores := make(chan error, escritores*vueltas)
	for w := 0; w < escritores; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < vueltas; i++ {
				value := map[string]string{
					"escritor": fmt.Sprint(w),
					"vuelta":   fmt.Sprint(i),
					"relleno":  relleno,
				}
				if err := writeJSON(path, value); err != nil {
					errores <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errores)
	for err := range errores {
		t.Fatalf("writeJSON concurrente falló: %v (Y5)", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("el archivo final no es JSON válido: %v", err)
	}
	if out["relleno"] != relleno {
		t.Error("el archivo final quedó truncado")
	}
}
