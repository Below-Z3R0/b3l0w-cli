package ytmusic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// El player es un proceso DETACHADO (`setsid`), no un runAsync del service: si
// el service se recarga, la música no se corta. El PID vive en player.json y se
// mata por GRUPO (-pid), porque el grupo es `yt-dlp | mpv` y matar sólo a mpv
// deja a yt-dlp escribiendo a un pipe sin lector.
//
// Lo que es de Unix (setsid, grupos de procesos, señales) vive en
// player_unix.go con su build tag: este archivo compila en cualquier
// plataforma (Y20).

type playerFile struct {
	PID     int    `json:"pid"`
	VideoID string `json:"videoId"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Seconds int    `json:"seconds,omitempty"`
	Session string `json:"session,omitempty"`
	Socket  string `json:"socket"`
	Started int64  `json:"startedAt"`
	// StartTime es el campo 22 de /proc/<pid>/stat. Es lo único que distingue
	// nuestro proceso de un PID reciclado: el número solo no alcanza (Y1).
	StartTime uint64 `json:"startTime,omitempty"`
}

func playerPath(dataDir string) string { return filepath.Join(dataDir, "player.json") }
func socketPath(dataDir string) string { return filepath.Join(dataDir, "mpv.sock") }

// playTimeout es el techo del trabajo de `play`: resolver la metadata con
// yt-dlp y esperar el socket de mpv. Sin este techo, un yt-dlp colgado por la
// red dejaba el comando esperando para siempre (Y6).
const playTimeout = 45 * time.Second

// resolveMetadata consulta la metadata real de una pista con yt-dlp.
// Se usa cuando la canción llega sin título (por ejemplo `play <videoId>`
// directo): sin esto, el panel muestra el identificador como si fuera el
// nombre de la canción.
//
// El contexto manda: si se vence, el yt-dlp se corta y el comando no se
// queda esperando (Y6).
func resolveMetadata(ctx context.Context, videoID string) (Song, error) {
	url := "https://music.youtube.com/watch?v=" + videoID
	args := []string{
		"--no-warnings",
		"--skip-download",
		"--print", "%(title)s\t%(artist)s\t%(duration)s\t%(duration_string)s",
		url,
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	// WaitDelay: si el contexto se vence pero el proceso deja hijos con los
	// pipes abiertos, Output() se queda esperando el EOF de esos pipes. El
	// delay corta la espera: el comando no puede colgarse (Y6).
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
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

// Play arranca la reproducción de un videoId. Reemplaza lo que estuviera
// sonando: matar y relanzar van en la misma operación para que no queden dos
// mpv peleando por el dispositivo de audio.
//
// El contexto acota el trabajo: la metadata con yt-dlp y la espera del socket
// no pueden dejar el comando colgado para siempre (Y6).
func Play(ctx context.Context, dataDir string, song Song, session string) (PlayerState, error) {
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
		metaCtx, cancel := context.WithTimeout(ctx, playTimeout)
		meta, err := resolveMetadata(metaCtx, song.VideoID)
		// El error del contexto se mira ANTES de cancelar: si el deadline
		// se agotó, el comando se quedaba esperando para siempre (Y6).
		ctxErr := metaCtx.Err()
		cancel()
		if err != nil {
			if ctxErr != nil {
				return PlayerState{}, fmt.Errorf("play: se agotó el tiempo esperando la metadata: %w", ctxErr)
			}
		} else {
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
	setSessionAttr(cmd)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return PlayerState{}, fmt.Errorf("play: no se pudo lanzar yt-dlp|mpv: %w", err)
	}
	pid := cmd.Process.Pid
	// Sin Wait el hijo queda a cargo de init (setsid ya lo separó del árbol).
	go func() { _ = cmd.Wait() }()
	// Se guarda la identidad del proceso junto al PID: `stop` la compara antes
	// de matar, para no matar a un ajeno si el número se recicló (Y1).
	started, _ := processStartTime(pid)

	state := playerFile{
		PID:       pid,
		VideoID:   song.VideoID,
		Title:     song.Title,
		Artist:    song.Artist,
		Seconds:   song.Seconds,
		Session:   session,
		Socket:    socket,
		Started:   nowUnix(),
		StartTime: started,
	}
	if err := writeJSON(playerPath(dataDir), state); err != nil {
		// El reproductor ya arrancó pero su estado no se pudo guardar: sin
		// player.json no hay forma de controlarlo y la música sigue sonando
		// sin control. Se lo mata antes de devolver el error (Y7).
		killStarted(pid)
		return PlayerState{}, fmt.Errorf("play: %w", err)
	}

	// Se espera el socket un instante corto: el service quiere saber si arrancó,
	// pero no puede quedarse 10 s esperando a yt-dlp.
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return PlayerState{}, fmt.Errorf("play: se agotó el tiempo esperando a mpv: %w", ctx.Err())
		case <-time.After(120 * time.Millisecond):
		}
	}
	return Status(dataDir)
}

// killStarted mata el proceso que Play acaba de arrancar, cuando no se pudo
// guardar su estado (Y7). El PID es del proceso que este código lanzó, así
// que no hace falta validarlo como en `Stop`: no hay riesgo de Recycled PID.
//
// Se manda SIGTERM al grupo y, al final, SIGKILL al grupo sin condición: el
// `sh` puede haber forked a `yt-dlp` y `mpv` justo después del SIGTERM, y
// esos hijos tampoco pueden quedar vivos.
func killStarted(pid int) {
	if pid <= 0 {
		return
	}
	_ = killGroup(pid, signalTerm)
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = killGroup(pid, signalKill)
	_ = killProcess(pid, signalKill)
}

// Pause alterna pausa/reanudación. Un solo comando para un solo botón.
func Pause(dataDir string) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		return PlayerState{}, err
	}
	paused, err := ipcBool(socketPath(dataDir), "pause")
	if err != nil {
		return PlayerState{}, fmt.Errorf("pause: mpv no responde (%v): %w", state.VideoID, err)
	}
	if _, err := ipcCommand(socketPath(dataDir), []any{"set_property", "pause", !paused}); err != nil {
		return PlayerState{}, fmt.Errorf("pause: %w", err)
	}
	return Status(dataDir)
}

// Resume fuerza reproducir (sin alternar): lo usa el panel al cambiar de pista.
func Resume(dataDir string) (PlayerState, error) {
	if _, err := loadPlayer(dataDir); err != nil {
		return PlayerState{}, err
	}
	if _, err := ipcCommand(socketPath(dataDir), []any{"set_property", "pause", false}); err != nil {
		return PlayerState{}, fmt.Errorf("resume: %w", err)
	}
	return Status(dataDir)
}

// Seek salta a un segundo absoluto. La barra de progreso del panel lo usa
// cuando el usuario clickea una posición.
func Seek(dataDir string, seconds int) (PlayerState, error) {
	if _, err := loadPlayer(dataDir); err != nil {
		return PlayerState{}, err
	}
	if seconds < 0 {
		seconds = 0
	}
	if _, err := ipcCommand(socketPath(dataDir), []any{"seek", float64(seconds), "absolute"}); err != nil {
		return PlayerState{}, fmt.Errorf("seek: %w", err)
	}
	return Status(dataDir)
}

// Stop mata el grupo entero y limpia el estado. Antes de mandar señal valida
// que el PID siga siendo el nuestro: si el número se recicló, matar el grupo
// sería matar procesos ajenos (Y1). Un `player.json` viejo, sin `startTime`,
// no se puede validar y no se mata: se descarta el estado (fail-closed).
func Stop(dataDir string) error {
	state, err := loadPlayer(dataDir)
	if err != nil {
		if !os.IsNotExist(err) {
			// El estado está roto: no hay nada sonando y el archivo roto se
			// limpia, así el siguiente status no lo vuelve a leer (Y13).
			_ = os.Remove(playerPath(dataDir))
		}
		return nil // no había nada sonando
	}
	if ours(state) {
		// -pid: el grupo que creó setsid (sh + yt-dlp + mpv).
		if err := killGroup(state.PID, signalTerm); err != nil {
			_ = killProcess(state.PID, signalTerm)
		}
		deadline := time.Now().Add(1200 * time.Millisecond)
		for time.Now().Before(deadline) {
			if !alive(state.PID) {
				break
			}
			time.Sleep(60 * time.Millisecond)
		}
		if alive(state.PID) {
			if err := killGroup(state.PID, signalKill); err != nil {
				_ = killProcess(state.PID, signalKill)
			}
		}
	}
	// El socket sale del dataDir, nunca del campo `socket` del JSON: un
	// player.json editado a mano no puede hacer borrar un archivo arbitrario (Y2).
	_ = os.Remove(socketPath(dataDir))
	_ = os.Remove(playerPath(dataDir))
	return nil
}

// Status devuelve el estado vivo consultando el socket de mpv: la posición sale
// de mpv, no de un reloj nuestro, así una pausa o un buffer lento no mienten.
//
// Un player.json CORRUPTO es un error, no "parado": antes cualquier fallo de
// lectura se traducía en "parado" con exit 0 y el archivo roto nunca se
// limpiaba, así que cada status posterior volvía a leer el mismo JSON inválido
// (Y13). Un player.json ausente sigue siendo "parado": no hay nada que
// limpiar.
func Status(dataDir string) (PlayerState, error) {
	state, err := loadPlayer(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return PlayerState{Playing: false}, nil
		}
		// El estado está roto: se limpia (si no, el panel no puede
		// distinguir "no suena nada" de "el estado está roto") y se informa
		// el error.
		_ = os.Remove(playerPath(dataDir))
		_ = os.Remove(socketPath(dataDir))
		return PlayerState{Playing: false}, fmt.Errorf("el estado del player está roto: %w", err)
	}
	out := PlayerState{
		PID:      state.PID,
		VideoID:  state.VideoID,
		Title:    state.Title,
		Artist:   state.Artist,
		Duration: state.Seconds,
		Session:  state.Session,
		Socket:   socketPath(dataDir),
		Started:  state.Started,
		Playing:  ours(state),
	}
	if !out.Playing {
		_ = os.Remove(playerPath(dataDir))
		_ = os.Remove(socketPath(dataDir))
		return out, nil
	}
	// EOF: mpv queda en idle (--idle=yes). Es la señal de "pasa a la siguiente"
	// y llega ANTES de que el proceso muera, que es lo que rompía el autoplay.
	if eof, err := ipcBool(socketPath(dataDir), "eof-reached"); err == nil && eof {
		out.Ended = true
		out.Playing = false
		if position, err := ipcNumber(socketPath(dataDir), "time-pos"); err == nil {
			out.Position = int(position)
		}
		if duration, err := ipcNumber(socketPath(dataDir), "duration"); err == nil {
			out.Duration = int(duration)
		}
		return out, nil
	}
	// mpv puede tardar unos segundos en tener time-pos (yt-dlp todavía
	// resuelve el stream). Mientras tanto el proceso está vivo y la música
	// va a arrancar: se informa `starting` pero la pista sigue siendo la
	// que suena. Antes, este caso devolvía position=0 y duration=0 y el
	// panel mostraba "0:00 / 0:00" para siempre.
	position, err := ipcNumber(socketPath(dataDir), "time-pos")
	if err != nil {
		out.Starting = true
		// La duración conocida por el catálogo sirve de referencia mientras
		// mpv no la reporta.
		out.Duration = state.Seconds
		return out, nil
	}
	duration, _ := ipcNumber(socketPath(dataDir), "duration")
	if duration <= 0 {
		duration = float64(state.Seconds)
	}
	paused, _ := ipcBool(socketPath(dataDir), "pause")
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

// alive y las señales viven en player_unix.go / player_other.go: son de
// Unix (Y20).

// processStartTime lee el campo 22 de /proc/<pid>/stat (starttime, en ticks
// desde el boot). Junto al PID es la identidad del proceso: el número solo se
// recicla y `stop` terminaría matando a otro (Y1).
//
// El campo `comm` (2) puede tener espacios y paréntesis, así que se corta
// después del último ')': los campos siguientes van separados por espacios.
func processStartTime(pid int) (uint64, bool) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	text := string(raw)
	cut := strings.LastIndex(text, ")")
	if cut < 0 {
		return 0, false
	}
	fields := strings.Fields(text[cut+1:])
	// fields[0] es el estado (campo 3); starttime es el 22 → índice 19.
	if len(fields) < 20 {
		return 0, false
	}
	value, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// ours dice si el PID guardado sigue siendo el proceso que arrancamos. Sin
// `startTime` (player.json de una versión anterior) no se puede probar y se
// responde que no: es preferible dejar un mpv huérfano a matar un ajeno.
func ours(state playerFile) bool {
	if state.PID <= 0 || state.StartTime == 0 {
		return false
	}
	if !alive(state.PID) {
		return false
	}
	current, ok := processStartTime(state.PID)
	return ok && current == state.StartTime
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
