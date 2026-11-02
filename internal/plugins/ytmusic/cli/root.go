// Package ytmusiccli arma los comandos de `b3l0w ytmusic`. El parseo de
// argumentos y la salida viven acá; la red, el parseo de innertube y mpv están
// en el paquete ytmusic.
package ytmusiccli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w/internal/plugins/ytmusic"
)

const usage = `b3l0w ytmusic — YouTube Music real (innertube + mpv)

uso: b3l0w ytmusic <comando> [opciones]

  home [--continuation TOK]        recomendaciones de la página de inicio
  charts [--country XX]            ranking (Top / Trending)
  search <texto>                   canciones (filtro songs)
  radio <videoId>                  abre una radio de automix (crea sesión)
  next --session ID                avanza la cola; pide más si se acabó
  prev --session ID                vuelve una canción
  queue --session ID               la cola completa
  shuffle --session ID             mezcla la cola traída (deja la actual)
  play <videoId>                   reproduce (mpv detachado)
  pause | resume | stop            control de reproducción
  seek <segundos>                  salto en la pista actual
  status                           qué suena, posición y duración

Opciones comunes:
  --json             salida JSON (la que usa el plugin)
  --data-dir DIR     datos del plugin (sesiones, socket de mpv, portadas)
  --thumbs DIR       baja las portadas a ese directorio (ui.image no lee URLs)
  --limit N          cuántas portadas bajar (default 24)

Cualquier fallo de red o de forma del JSON sale como {"error": "..."} con exit 1:
el panel muestra ese texto en vez de una lista vacía.
`

// flags parsea argumentos aceptando banderas en cualquier posición: el paquete
// `flag` de Go corta en el primer posicional y `search "daft punk" --json`
// perdía el --json en silencio.
type flags struct {
	values map[string]string
	bools  map[string]bool
	pos    []string
}

func parseArgs(args []string, boolNames ...string) flags {
	isBool := make(map[string]bool, len(boolNames))
	for _, name := range boolNames {
		isBool[name] = true
	}
	out := flags{values: map[string]string{}, bools: map[string]bool{}}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "--") {
			out.pos = append(out.pos, arg)
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if equal := strings.Index(name, "="); equal >= 0 {
			out.values[name[:equal]] = name[equal+1:]
			continue
		}
		if isBool[name] {
			out.bools[name] = true
			continue
		}
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
			out.values[name] = args[index+1]
			index++
			continue
		}
		out.bools[name] = true
	}
	return out
}

func (f flags) value(name, fallback string) string {
	if value, ok := f.values[name]; ok && value != "" {
		return value
	}
	return fallback
}

func (f flags) number(name string, fallback int) int {
	raw, ok := f.values[name]
	if !ok {
		return fallback
	}
	value := 0
	fmt.Sscanf(raw, "%d", &value)
	if value == 0 {
		return fallback
	}
	return value
}

// Run despacha un subcomando y devuelve el código de salida.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
		return 2
	}
	command := args[0]
	rest := args[1:]
	switch command {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	case "home":
		return runFeed(rest, "home")
	case "charts":
		return runFeed(rest, "charts")
	case "search":
		return runFeed(rest, "search")
	case "radio":
		return runRadio(rest)
	case "next":
		return runStep(rest, "next")
	case "prev":
		return runStep(rest, "prev")
	case "queue":
		return runQueue(rest)
	case "shuffle":
		return runShuffle(rest)
	case "play":
		return runPlay(rest)
	case "pause":
		return runControl(rest, "pause")
	case "resume":
		return runControl(rest, "resume")
	case "stop":
		return runControl(rest, "stop")
	case "seek":
		return runControl(rest, "seek")
	case "status":
		return runStatus(rest)
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", command)
		fmt.Print(usage)
		return 2
	}
}

func runFeed(args []string, kind string) int {
	f := parseArgs(args, "json")
	limit := f.number("limit", 24)
	thumbsDir := f.value("thumbs", "")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := ytmusic.NewClient()

	var feed ytmusic.Feed
	var err error
	switch kind {
	case "home":
		feed, err = ytmusic.Home(ctx, client, f.value("continuation", ""))
	case "charts":
		feed, err = ytmusic.Charts(ctx, client, f.value("country", ""))
	case "search":
		query := f.value("query", "")
		if query == "" && len(f.pos) > 0 {
			query = strings.Join(f.pos, " ")
		}
		feed, err = ytmusic.Search(ctx, client, query, f.value("continuation", ""))
	}
	if err != nil {
		return fail(err, f.bools["json"])
	}
	feed.Items = ytmusic.EnrichDurations(ctx, client, feed.Items, limit)
	if thumbsDir != "" {
		feed.Items = ytmusic.Thumbs(ctx, thumbsDir, feed.Items, limit)
	}
	return emit(feed, f.bools["json"], func() {
		fmt.Printf("%s · %s · %d canciones\n", feed.Source, feed.Title, len(feed.Items))
		for index, song := range feed.Items {
			if index >= 10 {
				break
			}
			fmt.Printf("  %-52.52s %-24.24s %s\n", song.Title, song.Artist, song.Duration)
		}
	})
}

func runRadio(args []string) int {
	f := parseArgs(args, "json")
	if len(f.pos) == 0 {
		return fail(fmt.Errorf("radio: falta el videoId"), f.bools["json"])
	}
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	thumbsDir := f.value("thumbs", "")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	session, err := ytmusic.Radio(ctx, ytmusic.NewClient(), dataDir, f.pos[0], f.value("session", ""))
	if err != nil {
		return fail(err, f.bools["json"])
	}
	if thumbsDir != "" {
		session.Queue = ytmusic.Thumbs(ctx, thumbsDir, session.Queue, f.number("limit", 18))
	}
	return emit(session, f.bools["json"], func() {
		fmt.Printf("radio %s · sesión %s · %d canciones\n", session.Seed, session.ID, len(session.Queue))
	})
}

func runStep(args []string, direction string) int {
	f := parseArgs(args, "json", "play")
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	thumbsDir := f.value("thumbs", "")
	sessionID := f.value("session", "")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	var result ytmusic.NextResult
	var err error
	if direction == "next" {
		result, err = ytmusic.Next(ctx, ytmusic.NewClient(), dataDir, sessionID)
	} else {
		result, err = ytmusic.Previous(dataDir, sessionID)
	}
	if err != nil {
		return fail(err, f.bools["json"])
	}
	if thumbsDir != "" {
		resulted := ytmusic.Thumbs(ctx, thumbsDir, []ytmusic.Song{result.Song}, 1)
		result.Song = resulted[0]
	}
	payload := map[string]any{
		"session":      result.Session,
		"song":         result.Song,
		"cursor":       result.Cursor,
		"queueLength":  result.QueueLength,
		"fetchedMore":  result.FetchedMore,
		"continuation": result.Continuation,
	}
	if f.bools["play"] {
		state, playErr := ytmusic.Play(dataDir, result.Song, result.Session)
		if playErr != nil {
			return fail(playErr, f.bools["json"])
		}
		payload["player"] = state
	}
	return emit(payload, f.bools["json"], func() {
		fmt.Printf("%d/%d  %s — %s\n", result.Cursor+1, result.QueueLength, result.Song.Title, result.Song.Artist)
	})
}

func runQueue(args []string) int {
	f := parseArgs(args, "json")
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	queue, err := ytmusic.Queue(dataDir, f.value("session", ""))
	if err != nil {
		return fail(err, f.bools["json"])
	}
	if thumbsDir := f.value("thumbs", ""); thumbsDir != "" {
		queue.Items = ytmusic.Thumbs(context.Background(), thumbsDir, queue.Items, f.number("limit", 20))
	}
	return emit(queue, f.bools["json"], func() {
		fmt.Printf("cola %s · %d canciones · actual %d\n", queue.Session, len(queue.Items), queue.Cursor+1)
	})
}

func runShuffle(args []string) int {
	f := parseArgs(args, "json")
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	session, err := ytmusic.Shuffle(dataDir, f.value("session", ""))
	if err != nil {
		return fail(err, f.bools["json"])
	}
	view := ytmusic.QueueView{Session: session.ID, Seed: session.Seed, Items: session.Queue, Cursor: session.Cursor, Updated: time.Now().Unix()}
	return emit(view, f.bools["json"], func() {
		fmt.Printf("shuffle · %d canciones · empieza en %s\n", len(session.Queue), session.Queue[0].Title)
	})
}

func runPlay(args []string) int {
	f := parseArgs(args, "json")
	if len(f.pos) == 0 && f.value("video", "") == "" {
		return fail(fmt.Errorf("play: falta el videoId"), f.bools["json"])
	}
	videoID := f.value("video", "")
	if videoID == "" {
		videoID = f.pos[0]
	}
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	song := ytmusic.Song{
		VideoID:  videoID,
		Title:    f.value("title", ""),
		Artist:   f.value("artist", ""),
		Duration: f.value("duration", ""),
	}
	// El título NO se rellena con el videoId: si queda vacío, `Play` lo
	// resuelve con yt-dlp. Poner el id como título hacía que la playerbar
	// mostrara "Z85lxckrtzg" en lugar de "Thriller".
	if song.Title == "" {
		song.Title = ""
	}
	state, err := ytmusic.Play(dataDir, song, f.value("session", ""))
	if err != nil {
		return fail(err, f.bools["json"])
	}
	return emit(state, f.bools["json"], func() {
		fmt.Printf("reproduciendo %s — %s (pid %d)\n", state.Title, state.Artist, state.PID)
	})
}

func runControl(args []string, action string) int {
	f := parseArgs(args, "json")
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	var state ytmusic.PlayerState
	var err error
	switch action {
	case "pause":
		state, err = ytmusic.Pause(dataDir)
	case "resume":
		state, err = ytmusic.Resume(dataDir)
	case "stop":
		err = ytmusic.Stop(dataDir)
		state = ytmusic.PlayerState{Playing: false}
	case "seek":
		seconds := 0
		if len(f.pos) > 0 {
			fmt.Sscanf(f.pos[0], "%d", &seconds)
		} else {
			seconds = f.number("seconds", 0)
		}
		state, err = ytmusic.Seek(dataDir, seconds)
	}
	if err != nil {
		return fail(err, f.bools["json"])
	}
	return emit(state, f.bools["json"], func() {
		fmt.Printf("%s: playing=%v paused=%v pos=%d\n", action, state.Playing, state.Paused, state.Position)
	})
}

func runStatus(args []string) int {
	f := parseArgs(args, "json")
	dataDir := ytmusic.DataDir(f.value("data-dir", ""))
	state, err := ytmusic.Status(dataDir)
	if err != nil {
		return fail(err, f.bools["json"])
	}
	return emit(state, f.bools["json"], func() {
		if !state.Playing {
			fmt.Println("parado")
			return
		}
		fmt.Printf("%s — %s  %s/%s  pausa=%v\n", state.Title, state.Artist,
			clock(state.Position), clock(state.Duration), state.Paused)
	})
}

func clock(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func emit(value any, asJSON bool, human func()) int {
	if !asJSON {
		human()
		return 0
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "error de salida:", err)
		return 1
	}
	return 0
}

func fail(err error, asJSON bool) int {
	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(map[string]string{"error": err.Error()})
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}
