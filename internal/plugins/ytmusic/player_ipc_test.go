package ytmusic

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// socketDir crea un directorio corto para el socket unix. El path del socket
// tiene un límite del kernel (~108 caracteres) y el directorio de tests es
// más largo que eso.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ytm")
	if err != nil {
		t.Fatalf("creando el directorio del socket: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeMPV levanta un socket unix que habla como mpv: cada comando recibe un
// `{"command": [...]}` y contesta `{"error":"success","data":...}`. Los eventos
// (`{"event":...}`) se descartan, como en el player real.
func fakeMPV(t *testing.T, respond func(command []any) (any, bool)) string {
	t.Helper()
	dir := socketDir(t)
	socket := filepath.Join(dir, "mpv.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("escuchando en el socket: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				decoder := json.NewDecoder(conn)
				for {
					var payload struct {
						Command []any `json:"command"`
					}
					if err := decoder.Decode(&payload); err != nil {
						return
					}
					data, hasEvent := respond(payload.Command)
					if hasEvent {
						// Un evento no es una respuesta: el player lo ignora.
						event := map[string]any{"event": "property-change"}
						raw, _ := json.Marshal(event)
						if _, err := conn.Write(append(raw, '\n')); err != nil {
							return
						}
						continue
					}
					reply := map[string]any{"error": "success", "data": data}
					raw, _ := json.Marshal(reply)
					if _, err := conn.Write(append(raw, '\n')); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return socket
}

// TestIPCCommandPreguntaUnaPropiedad: el estado del player sale de mpv, no de
// un reloj nuestro.
func TestIPCCommandPreguntaUnaPropiedad(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		if len(command) != 2 || command[0] != "get_property" {
			t.Errorf("comando inesperado: %v", command)
		}
		return 42.5, false
	})
	response, err := ipcCommand(socket, []any{"get_property", "time-pos"})
	if err != nil {
		t.Fatalf("ipcCommand: %v", err)
	}
	if response["error"] != "success" {
		t.Errorf("response = %+v", response)
	}
}

// TestIPCCommandIgnoraLosEventos: mpv manda eventos por el mismo socket; si
// el player no los descarta, toma un evento por respuesta.
func TestIPCCommandIgnoraLosEventos(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		// El primer comando devuelve un evento; el segundo, la respuesta.
		if command[0] == "get_property" && command[1] == "time-pos" {
			return nil, true
		}
		return "ok", false
	})
	response, err := ipcCommand(socket, []any{"get_property", "duration"})
	if err != nil {
		t.Fatalf("ipcCommand: %v", err)
	}
	if response["data"] != "ok" {
		t.Errorf("response = %+v, quería la respuesta, no el evento", response)
	}
}

// TestIPCCommandPropagaElErrorDeMPV: un `{"error":"..."}` es un fallo.
func TestIPCCommandPropagaElErrorDeMPV(t *testing.T) {
	socket := filepath.Join(socketDir(t), "mpv.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("escuchando: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		decoder := json.NewDecoder(conn)
		var payload map[string]any
		if err := decoder.Decode(&payload); err != nil {
			return
		}
		raw, _ := json.Marshal(map[string]any{"error": "property unavailable"})
		_, _ = conn.Write(append(raw, '\n'))
	}()
	_, err = ipcCommand(socket, []any{"get_property", "time-pos"})
	if err == nil {
		t.Fatal("un error de mpv tiene que ser error")
	}
	if !strings.Contains(err.Error(), "property unavailable") {
		t.Errorf("el error no trae el motivo: %v", err)
	}
}

// TestIPCCommandSinSocketEsError: si mpv no está, el panel tiene que saberlo.
func TestIPCCommandSinSocketEsError(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "no-existe.sock")
	if _, err := ipcCommand(socket, []any{"get_property", "time-pos"}); err == nil {
		t.Fatal("sin socket tiene que ser error")
	}
}

// TestIPCNumberAceptaLosTiposDeMPV: el valor puede venir como número o como
// texto.
func TestIPCNumberAceptaLosTiposDeMPV(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "time-pos":
			return 12.5, false
		case "duration":
			return "358.07", false
		case "eof-reached":
			return true, false
		}
		return nil, false
	})
	if got, err := ipcNumber(socket, "time-pos"); err != nil || got != 12.5 {
		t.Errorf("ipcNumber(time-pos) = %v (%v)", got, err)
	}
	if got, err := ipcNumber(socket, "duration"); err != nil || got != 358.07 {
		t.Errorf("ipcNumber(duration) = %v (%v)", got, err)
	}
}

// TestIPCNumberConValorNoNumericoEsError.
func TestIPCNumberConValorNoNumericoEsError(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		return "no es un número", false
	})
	_, err := ipcNumber(socket, "time-pos")
	if err == nil {
		t.Fatal("un valor no numérico tiene que ser error")
	}
	if !strings.Contains(err.Error(), "número") {
		t.Errorf("el error no lo dice: %v", err)
	}
}

// TestIPCNumberSinPropiedadEsError: mpv todavía no tiene time-pos.
func TestIPCNumberSinPropiedadEsError(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		return nil, false
	})
	_, err := ipcNumber(socket, "time-pos")
	if err == nil {
		t.Fatal("una propiedad ausente tiene que ser error")
	}
	if !strings.Contains(err.Error(), "todavía") {
		t.Errorf("el error no dice que todavía no está: %v", err)
	}
}

// TestIPCBoolLeeElBooleano.
func TestIPCBoolLeeElBooleano(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		if command[1] == "eof-reached" {
			return true, false
		}
		return false, false
	})
	if got, err := ipcBool(socket, "eof-reached"); err != nil || !got {
		t.Errorf("ipcBool(eof-reached) = %v (%v)", got, err)
	}
	if got, err := ipcBool(socket, "pause"); err != nil || got {
		t.Errorf("ipcBool(pause) = %v (%v)", got, err)
	}
}

// TestIPCBoolConValorNoBooleanoEsError.
func TestIPCBoolConValorNoBooleanoEsError(t *testing.T) {
	socket := fakeMPV(t, func(command []any) (any, bool) {
		return "sí", false
	})
	if _, err := ipcBool(socket, "pause"); err == nil {
		t.Fatal("un valor no booleano tiene que ser error")
	}
}

// TestStatusInformaElFinDeLaPista: EOF es la señal de "pasa a la siguiente" y
// llega ANTES de que el proceso muera. Es lo que rompía el autoplay.
func TestStatusInformaElFinDeLaPista(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return true, false
		case "time-pos":
			return 358.0, false
		case "duration":
			return 358.0, false
		}
		return nil, false
	})
	// Un PID vivo: el del proceso de test.
	pid := os.Getpid()
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     pid,
		VideoID: "abc",
		Title:   "Canción",
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !state.Ended {
		t.Error("ended = false, el player no vio el EOF")
	}
	if state.Playing {
		t.Error("playing = true, con EOF ya no está sonando")
	}
	if state.Position != 358 || state.Duration != 358 {
		t.Errorf("posición/duración = %d/%d, quería 358/358", state.Position, state.Duration)
	}
}

// TestStatusInformaQueEstaArrancando: mientras yt-dlp resuelve el stream,
// mpv todavía no tiene time-pos. Antes eso devolvía "0:00 / 0:00" para
// siempre.
func TestStatusInformaQueEstaArrancando(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return false, false
		case "time-pos":
			return nil, false
		}
		return nil, false
	})
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     os.Getpid(),
		VideoID: "abc",
		Title:   "Canción",
		Seconds: 225,
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !state.Starting {
		t.Error("starting = false, el player todavía no puede informar posición")
	}
	if state.Duration != 225 {
		t.Errorf("duración = %d, quería 225 (la del catálogo como referencia)", state.Duration)
	}
	if state.Playing != true {
		t.Error("la pista sigue sonando aunque mpv no reporte posición")
	}
}

// TestStatusInformaLaPausa.
func TestStatusInformaLaPausa(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return false, false
		case "time-pos":
			return 42.0, false
		case "duration":
			return 225.0, false
		case "pause":
			return true, false
		}
		return nil, false
	})
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     os.Getpid(),
		VideoID: "abc",
		Title:   "Canción",
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !state.Paused {
		t.Error("paused = false, mpv dijo que está en pausa")
	}
	if state.Position != 42 {
		t.Errorf("posición = %d, quería 42", state.Position)
	}
}

// TestPauseAlternaElEstado: un solo comando para un solo botón.
func TestPauseAlternaElEstado(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return false, false
		case "time-pos":
			return 10.0, false
		case "duration":
			return 225.0, false
		case "pause":
			return false, false
		}
		return nil, false
	})
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     os.Getpid(),
		VideoID: "abc",
		Title:   "Canción",
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Pause(dir)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if state.Playing != true {
		t.Error("playing = false, pausar no es detener")
	}
}

// TestSeekAceptaPosicionesNegativas: un salto negativo se clava en 0.
func TestSeekAceptaPosicionesNegativas(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return false, false
		case "time-pos":
			return 0.0, false
		case "duration":
			return 225.0, false
		}
		return nil, false
	})
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     os.Getpid(),
		VideoID: "abc",
		Title:   "Canción",
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Seek(dir, -30); err != nil {
		t.Fatalf("Seek: %v", err)
	}
}

// TestResumeFuerzaReproducir: no alterna, siempre reanuda.
func TestResumeFuerzaReproducir(t *testing.T) {
	dir := t.TempDir()
	socket := fakeMPV(t, func(command []any) (any, bool) {
		switch command[1] {
		case "eof-reached":
			return false, false
		case "time-pos":
			return 10.0, false
		case "duration":
			return 225.0, false
		case "pause":
			return false, false
		}
		return nil, false
	})
	if err := writeJSON(playerPath(dir), playerFile{
		PID:     os.Getpid(),
		VideoID: "abc",
		Title:   "Canción",
		Socket:  socket,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := Resume(dir)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if state.Playing != true {
		t.Error("playing = false, reanudar tiene que sonar")
	}
}
