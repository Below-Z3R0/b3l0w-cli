package ytmusic

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestStopMataElGrupoEntero: el grupo es `yt-dlp | mpv`; matar sólo a mpv
// deja a yt-dlp escribiendo a un pipe sin lector.
func TestStopMataElGrupoEntero(t *testing.T) {
	dir := t.TempDir()
	// Un proceso nuevo en su propio grupo de sesiones: el PID es el líder
	// del grupo, así que el -pid de Stop lo alcanza.
	cmd := exec.Command("/bin/sleep", "120")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("arrancando el proceso de prueba: %v", err)
	}
	pid := cmd.Process.Pid
	// Igual que en el player: un Wait en background recoge al proceso. Sin
	// esto queda zombie y kill(pid, 0) sigue respondiendo.
	terminado := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(terminado)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		select {
		case <-terminado:
		case <-time.After(2 * time.Second):
		}
	})
	// El líder del grupo tiene que estar vivo antes de probar.
	if !alive(pid) {
		t.Fatal("el proceso de prueba no arrancó")
	}
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     pid,
		VideoID: "abc",
		Title:   "Canción",
		Socket:  filepath.Join(dir, "mpv.sock"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := Stop(dir); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// El grupo tiene que estar muerto.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && alive(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if alive(pid) {
		t.Error("el proceso sigue vivo después de Stop")
	}
	// Y el estado y el socket se limpian.
	if _, err := os.Stat(playerPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no limpió player.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mpv.sock")); !os.IsNotExist(err) {
		t.Errorf("no limpió el socket: %v", err)
	}
}

// TestStopLimpiaElEstadoEnDisco: si el player queda con un PID muerto, el
// panel mostraría "reproduciendo" para siempre.
func TestStopLimpiaElEstadoEnDisco(t *testing.T) {
	dir := t.TempDir()
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     0,
		VideoID: "abc",
		Title:   "Canción",
		Socket:  filepath.Join(dir, "mpv.sock"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := Stop(dir); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(playerPath(dir)); !os.IsNotExist(err) {
		t.Errorf("no limpió player.json: %v", err)
	}
}
