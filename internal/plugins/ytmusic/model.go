// Package ytmusic habla con el innertube de YouTube Music (cliente WEB_REMIX) y
// controla la reproducción con mpv. Vive fuera del service de Noctalia a
// propósito: parsear un JSON de 1 MB en el callback del service revienta el
// presupuesto de 25 ms y el host deshabilita el plugin.
package ytmusic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Song es una canción lista para el panel. `Thumb` es una ruta LOCAL: ui.image
// no acepta URLs, así que el CLI baja la portada antes de publicar el feed.
type Song struct {
	VideoID   string `json:"videoId"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Seconds   int    `json:"seconds,omitempty"`
	ThumbURL  string `json:"thumbUrl,omitempty"`
	Thumb     string `json:"thumb,omitempty"`
	ChartRank int    `json:"rank,omitempty"`
}

// Feed es lo que devuelve cada pantalla. `Continuation` es el token de la
// página siguiente: sin él no hay paginación infinita y la lista muere en la
// primera página, que es exactamente el bug del plugin original.
type Feed struct {
	Source       string `json:"source"`
	Title        string `json:"title,omitempty"`
	Items        []Song `json:"items"`
	Continuation string `json:"continuation,omitempty"`
	FetchedAt    int64  `json:"fetchedAt"`
}

// Session es la cola viva de una radio. Se persiste porque `next` corre en un
// proceso distinto (y a veces minutos después) del `radio` que la creó.
type Session struct {
	ID           string `json:"id"`
	Seed         string `json:"seed"`
	PlaylistID   string `json:"playlistId"`
	Continuation string `json:"continuation,omitempty"`
	Queue        []Song `json:"queue"`
	Cursor       int    `json:"cursor"`
	CreatedAt    int64  `json:"createdAt"`
}

// PlayerState refleja el proceso de mpv que está sonando ahora.
type PlayerState struct {
	PID      int    `json:"pid"`
	VideoID  string `json:"videoId"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Session  string `json:"session,omitempty"`
	Socket   string `json:"socket"`
	Started  int64  `json:"startedAt"`
	Paused   bool   `json:"paused"`
	Playing  bool   `json:"playing"`
	Position int    `json:"position"`
	Duration int    `json:"duration"`
	Ended    bool   `json:"ended,omitempty"`
	Starting bool   `json:"starting,omitempty"`
	Error    string `json:"error,omitempty"`
}

// DataDir resuelve el directorio de datos del plugin. Mismo criterio que el
// resto de b3l0w: flag explícito, si no el state dir de Noctalia.
func DataDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	stateHome := os.Getenv("NOCTALIA_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "noctalia", "plugins", "data", "b3l0w", "ytmusic")
}

// EnsureDir crea el directorio con permisos de dueño. MPV y el socket IPC viven
// acá, así que no puede quedar legible por otros.
//
// MkdirAll no cambia los permisos de un directorio que YA existía: un dataDir
// de una corrida anterior puede quedar en 0755 y dejar el socket de mpv
// legible por otros (Y8). Por eso, después de crear, se ajustan los permisos.
func EnsureDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("directorio de datos vacío")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("ajustando permisos de %s: %w", dir, err)
	}
	return nil
}

func nowUnix() int64 { return time.Now().Unix() }

// songDedup quita repetidos preservando el orden: las radios devuelven el seed
// al principio y otra vez más abajo.
func songDedup(in []Song) []Song {
	seen := make(map[string]bool, len(in))
	out := make([]Song, 0, len(in))
	for _, song := range in {
		if song.VideoID == "" || seen[song.VideoID] {
			continue
		}
		seen[song.VideoID] = true
		out = append(out, song)
	}
	return out
}

// maxDurationSeconds es el techo de una duración: un año. Un `lengthText`
// enorme (viene de la red) desbordaba el int y devolvía segundos negativos o
// absurdos (Y12); con el techo no hay forma de desbordar.
const maxDurationSeconds = 366 * 24 * 60 * 60

// durationToSeconds convierte "3:45" o "1:02:03" a segundos.
//
// Una duración que no es un número devuelve 0, como las demás entradas raras.
// Una duración absurda (más larga que un año) también: el panel nunca muestra
// un número negativo ni un overflow (Y12).
func durationToSeconds(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	parts := strings.Split(text, ":")
	total := int64(0)
	for _, part := range parts {
		n := int64(0)
		found := false
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int64(r-'0')
			if n > maxDurationSeconds {
				return 0
			}
			found = true
		}
		if !found {
			return 0
		}
		total = total*60 + n
		if total > maxDurationSeconds {
			return 0
		}
	}
	return int(total)
}

// writeJSON escribe un archivo de estado de forma atómica: un `next` puede leer
// la sesión mientras otra llamada la reescribe.
//
// El temporal se crea con os.CreateTemp para que cada escritor tenga el suyo.
// Con un `.tmp` de nombre fijo dos escritores concurrentes se pisaban: uno
// renombraba primero y al otro el rename le daba ENOENT, perdiendo su
// actualización (Y5).
func writeJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("%s está vacío", filepath.Base(path))
	}
	return json.Unmarshal(raw, out)
}
