package ytmusic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// El player es un proceso DETACHADO (`setsid`), no un runAsync del service: si
// el service se recarga, la música no se corta. El PID vive en player.json y se
// mata por GRUPO (-pid), porque el grupo es `yt-dlp | mpv` y matar sólo a mpv
// deja a yt-dlp escribiendo a un pipe sin lector.

type playerFile struct {
	PID     int    `json:"pid"`
	VideoID string `json:"videoId"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Seconds int    `json:"seconds,omitempty"`
	Session string `json:"session,omitempty"`
	Socket  string `json:"socket"`
	Started int64  `json:"startedAt"`
}

func playerPath(dataDir string) string { return filepath.Join(dataDir, "player.json") }
func socketPath(dataDir string) string { return filepath.Join(dataDir, "mpv.sock") }

// Play arranca la reproducción de un videoId. Reemplaza lo que estuviera
// sonando: matar y relanzar van en la misma operación para que no queden dos
// mpv peleando por el dispositivo de audio.
// resolveMetadata consulta la metadata real de una pista con yt-dlp.
// Se usa cuando la canción llega sin título (por ejemplo `play <videoId>`
// directo): sin esto, el panel muestra el identificador como si fuera el
// nombre de la canción.
func resolveMetadata(videoID string) (Song, error) {
	url := "https://music.youtube.com/watch?v=" + videoID
	args := []string{
		"--no-warnings",
		"--skip-download",
		"--print", "%(title)s\t%(artist)s\t%(duration)s\t%(duration_string)s",
		url,
	}
	out, err := exec.Command("yt-dlp", args...).Output()
	if err != nil {
		return Song{}, err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return Song{}, fmt.Errorf("metadata vacía")
	}
	parts := strings.SplitN(line, "\t", 4)
	song := Song{VideoID: videoID}
	if len(parts) > 0 {
		song.Title = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		song.Artist = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		song.Seconds = parseSeconds(parts[2])
	}
	if len(parts) > 3 {
		song.Duration = strings.TrimSpace(parts[3])
	}
	if song.Title == "" {
		return Song{}, fmt.Errorf("metadata sin título")
	}
	return song, nil
}

// parseSeconds convierte los segundos de yt-dlp ("358.07") a entero.
func parseSeconds(value string) int {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return int(seconds)
}

func Play(dataDir string, song Song, session string) (PlayerState, error) {
	if song.VideoID == "" {
		return PlayerState{}, fmt.Errorf("play: falta el videoId")
	}
	if err := EnsureDir(dataDir); err != nil {
		return PlayerState{}, err
	}
	Stop(dataDir)
	socket := socketPath(dataDir)
	_ = os.Remove(socket)

	// Si la canción llega sin metadata (pasa con `play <videoId>` directo),
	// se resuelve con yt-dlp: sin esto el panel muestra el videoId como
	// título y la playerbar queda sin artista ni duración.
	if song.Title == "" || song.Title == song.VideoID {
		if meta, err := resolveMetadata(song.VideoID); err == nil {
			song.Title = meta.Title
			if song.Artist == "" {
				song.Artist = meta.Artist
			}
			if song.Duration == "" {
				song.Duration = meta.Duration
			}
			if song.Seconds == 0 {
				song.Seconds = meta.Seconds
			}
		}
	}

	url := "https://music.youtube.com/watch?v=" + song.VideoID
	script := strings.Join([]string{
		"yt-dlp -f bestaudio -o - --no-warnings --no-playlist " + shellQuote(url),
		"|",
		"mpv - --no-video --no-terminal --force-window=no --really-quiet --idle=yes",
		"--audio-client-name=b3l0w-ytmusic",
		"--input-ipc-server=" + shellQuote(socket),
	}, " ")

	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return PlayerState{}, fmt.Errorf("play: no se pudo lanzar yt-dlp|mpv: %w", err)
	}
	pid := cmd.Process.Pid
	// Sin Wait el hijo queda a cargo de init (setsid ya lo separó del árbol).
	go func() { _ = cmd.Wait() }()

	state := playerFile{
		PID:     pid,
		VideoID: song.VideoID,
		Title:   song.Title,
		Artist:  song.Artist,
		Seconds: song.Seconds,
		Session: session,
		Socket:  socket,
		Started: nowUnix(),
	}
	if err := writeJSON(playerPath(dataDir), state); err != nil {
		return PlayerState{}, fmt.Errorf("play: %w", err)
	}

	// Se espera el socket un instante corto: el service quiere saber si arrancó,
	// pero no puede quedarse 10 s esperando a yt-dlp.
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	return Status(dataDir)
}

// Pause alterna pausa/reanudación. Un solo comando para un solo botón.
func Pause(dataDir string) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return PlayerState{}, err
	}
	paused, err := ipcBool(state.Socket, "pause")
	if err != nil {
		return PlayerState{}, fmt.Errorf("pause: mpv no responde (%v): %w", state.VideoID, err)
	}
	if _, err := ipcCommand(state.Socket, []any{"set_property", "pause", !paused}); err != nil {
		return PlayerState{}, fmt.Errorf("pause: %w", err)
	}
	return Status(dataDir)
}

// Resume fuerza reproducir (sin alternar): lo usa el panel al cambiar de pista.
func Resume(dataDir string) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return PlayerState{}, err
	}
	if _, err := ipcCommand(state.Socket, []any{"set_property", "pause", false}); err != nil {
		return PlayerState{}, fmt.Errorf("resume: %w", err)
	}
	return Status(dataDir)
}

// Seek salta a un segundo absoluto. La barra de progreso del panel lo usa
// cuando el usuario clickea una posición.
func Seek(dataDir string, seconds int) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return PlayerState{}, err
	}
	if seconds < 0 {
		seconds = 0
	}
	if _, err := ipcCommand(state.Socket, []any{"seek", float64(seconds), "absolute"}); err != nil {
		return PlayerState{}, fmt.Errorf("seek: %w", err)
	}
	return Status(dataDir)
}

// Stop mata el grupo entero y limpia el estado.
func Stop(dataDir string) error {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return nil // no había nada sonando
	}
	if state.PID > 0 {
		// -pid: el grupo que creó setsid (sh + yt-dlp + mpv).
		if err := syscall.Kill(-state.PID, syscall.SIGTERM); err != nil {
			_ = syscall.Kill(state.PID, syscall.SIGTERM)
		}
		deadline := time.Now().Add(1200 * time.Millisecond)
		for time.Now().Before(deadline) {
			if !alive(state.PID) {
				break
			}
			time.Sleep(60 * time.Millisecond)
		}
		if alive(state.PID) {
			if err := syscall.Kill(-state.PID, syscall.SIGKILL); err != nil {
				_ = syscall.Kill(state.PID, syscall.SIGKILL)
			}
		}
	}
	_ = os.Remove(state.Socket)
	_ = os.Remove(playerPath(dataDir))
	return nil
}

// Status devuelve el estado vivo consultando el socket de mpv: la posición sale
// de mpv, no de un reloj nuestro, así una pausa o un buffer lento no mienten.
func Status(dataDir string) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return PlayerState{Playing: false}, nil
	}
	out := PlayerState{
		PID:     state.PID,
		VideoID: state.VideoID,
		Title:   state.Title,
		Artist:  state.Artist,
		Duration: state.Seconds,
		Session: state.Session,
		Socket:  state.Socket,
		Started: state.Started,
		Playing: alive(state.PID),
	}
	if !out.Playing {
		_ = os.Remove(playerPath(dataDir))
		_ = os.Remove(state.Socket)
		return out, nil
	}
	// EOF: mpv queda en idle (--idle=yes). Es la señal de "pasa a la siguiente"
	// y llega ANTES de que el proceso muera, que es lo que rompía el autoplay.
	if eof, err := ipcBool(state.Socket, "eof-reached"); err == nil && eof {
		out.Ended = true
		out.Playing = false
		if position, err := ipcNumber(state.Socket, "time-pos"); err == nil {
			out.Position = int(position)
		}
		if duration, err := ipcNumber(state.Socket, "duration"); err == nil {
			out.Duration = int(duration)
		}
		return out, nil
	}
	// mpv puede tardar unos segundos en tener time-pos (yt-dlp todavía
	// resuelve el stream). Mientras tanto el proceso está vivo y la música
	// va a arrancar: se informa `starting` pero la pista sigue siendo la
	// que suena. Antes, este caso devolvía position=0 y duration=0 y el
	// panel mostraba "0:00 / 0:00" para siempre.
	position, err := ipcNumber(state.Socket, "time-pos")
	if err != nil {
		out.Starting = true
		// La duración conocida por el catálogo sirve de referencia mientras
		// mpv no la reporta.
		out.Duration = state.Seconds
		return out, nil
	}
	duration, _ := ipcNumber(state.Socket, "duration")
	if duration <= 0 {
		duration = float64(state.Seconds)
	}
	paused, _ := ipcBool(state.Socket, "pause")
	out.Position = int(position)
	out.Duration = int(duration)
	out.Paused = paused
	return out, nil
}

func loadPlayer(dataDir string) (playerFile, error) {
	var state playerFile
	if err := readJSON(playerPath(dataDir), &state); err != nil {
		return playerFile{}, err
	}
	return state, nil
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// ── IPC de mpv ───────────────────────────────────────────────────────────

func ipcCommand(socket string, command []any) (map[string]any, error) {
	conn, err := net.DialTimeout("unix", socket, 1200*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	payload, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var decoded map[string]any
		if json.Unmarshal(line, &decoded) != nil {
			continue
		}
		// mpv manda eventos por el mismo socket: se descartan hasta la respuesta.
		if _, isEvent := decoded["event"]; isEvent {
			continue
		}
		if status := asString(decoded["error"]); status != "" && status != "success" {
			return nil, fmt.Errorf("mpv: %s", status)
		}
		return decoded, nil
	}
}

func ipcNumber(socket, property string) (float64, error) {
	response, err := ipcCommand(socket, []any{"get_property", property})
	if err != nil {
		return 0, err
	}
	switch value := response["data"].(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("mpv: %s no es un número", property)
		}
		return parsed, nil
	}
	return 0, fmt.Errorf("mpv: %s no está disponible todavía", property)
}

func ipcBool(socket, property string) (bool, error) {
	response, err := ipcCommand(socket, []any{"get_property", property})
	if err != nil {
		return false, err
	}
	value, ok := response["data"].(bool)
	if !ok {
		return false, fmt.Errorf("mpv: %s no es booleano", property)
	}
	return value, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
