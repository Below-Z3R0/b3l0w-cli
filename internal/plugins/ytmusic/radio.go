package ytmusic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// sessionsDir guarda una cola por sesión. `next` puede correr en un proceso
// distinto al que abrió la radio, así que la cola no puede vivir en memoria.
func sessionsDir(dataDir string) string {
	return filepath.Join(dataDir, "sessions")
}

func sessionPath(dataDir, id string) string {
	return filepath.Join(sessionsDir(dataDir), id+".json")
}

func newSessionID() string {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "s" + fmt.Sprint(nowUnix())
	}
	return "s" + hex.EncodeToString(buffer)
}

// Radio crea una sesión de automix para un videoId y devuelve su primera tanda.
func Radio(ctx context.Context, c *Client, dataDir, videoID, sessionID string) (Session, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return Session{}, fmt.Errorf("radio: falta el videoId semilla")
	}
	payload, err := c.Radio(ctx, videoID)
	if err != nil {
		return Session{}, fmt.Errorf("radio %s: %w", videoID, err)
	}
	songs := songDedup(songsFromRenderers(payload))
	if len(songs) == 0 {
		return Session{}, fmt.Errorf("radio %s: el watch-next no trajo canciones", videoID)
	}
	if sessionID == "" {
		sessionID = newSessionID()
	}
	session := Session{
		ID:           sessionID,
		Seed:         videoID,
		PlaylistID:   radioPlaylistPrefix + videoID,
		Continuation: continuationOf(payload),
		Queue:        songs,
		Cursor:       0,
		CreatedAt:    nowUnix(),
	}
	if err := SaveSession(dataDir, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// SaveSession persiste la cola y deja el puntero a la sesión activa.
func SaveSession(dataDir string, session Session) error {
	if err := EnsureDir(sessionsDir(dataDir)); err != nil {
		return fmt.Errorf("sesiones: %w", err)
	}
	if err := writeJSON(sessionPath(dataDir, session.ID), session); err != nil {
		return fmt.Errorf("sesiones: %w", err)
	}
	return writeJSON(filepath.Join(dataDir, "current.json"), map[string]string{"session": session.ID})
}

// LoadSession lee una cola. Un id vacío usa la sesión activa.
func LoadSession(dataDir, id string) (Session, error) {
	if id == "" {
		var pointer struct {
			Session string `json:"session"`
		}
		if err := readJSON(filepath.Join(dataDir, "current.json"), &pointer); err != nil || pointer.Session == "" {
			return Session{}, fmt.Errorf("no hay una sesión de reproducción activa")
		}
		id = pointer.Session
	}
	var session Session
	if err := readJSON(sessionPath(dataDir, id), &session); err != nil {
		if os.IsNotExist(err) {
			return Session{}, fmt.Errorf("sesión desconocida: %s", id)
		}
		return Session{}, fmt.Errorf("leyendo la sesión %s: %w", id, err)
	}
	// El cursor viene de un JSON en disco: si quedó fuera de la cola
	// (sesión editada a mano o interrumpida a mitad de guardado),
	// indexar revienta con un panic. Se valida una sola vez acá.
	if session.Cursor < 0 || session.Cursor >= len(session.Queue) {
		return Session{}, fmt.Errorf(
			"la sesión %s tiene el cursor %d fuera de una cola de %d canciones",
			session.ID, session.Cursor, len(session.Queue))
	}
	return session, nil
}

// NextResult es lo que devuelve `next`: la canción siguiente y el estado de la
// cola después de avanzar.
type NextResult struct {
	Session      string `json:"session"`
	Song         Song   `json:"song"`
	Cursor       int    `json:"cursor"`
	QueueLength  int    `json:"queueLength"`
	FetchedMore  bool   `json:"fetchedMore"`
	Continuation string `json:"continuation,omitempty"`
}

// Next avanza la cola de una sesión. Cuando se acaba lo ya traído pide la
// siguiente tanda de la MISMA radio (automix); si esa radio se agotó, siembra
// una nueva desde la última canción — así "siguiente" nunca se queda sin nada.
func Next(ctx context.Context, c *Client, dataDir, id string) (NextResult, error) {
	session, err := LoadSession(dataDir, id)
	if err != nil {
		return NextResult{}, err
	}
	if len(session.Queue) == 0 {
		return NextResult{}, fmt.Errorf("la sesión %s no tiene canciones", session.ID)
	}
	fetched := false

	if session.Cursor+1 >= len(session.Queue) {
		more, token := fetchMore(ctx, c, session)
		if len(more) > 0 {
			session.Queue = append(session.Queue, more...)
			session.Continuation = token
			fetched = true
		}
	}
	if session.Cursor+1 >= len(session.Queue) {
		// La radio no dio más: se siembra otra desde la última canción y se
		// sigue. Rendirse acá deja el botón "siguiente" muerto a los 50 temas.
		last := session.Queue[len(session.Queue)-1]
		rebased, rebaseErr := Radio(ctx, c, dataDir, last.VideoID, session.ID)
		if rebaseErr != nil {
			return NextResult{}, fmt.Errorf("la radio se agotó y no se pudo sembrar otra desde %s: %w", last.VideoID, rebaseErr)
		}
		known := make(map[string]bool, len(session.Queue))
		for _, song := range session.Queue {
			known[song.VideoID] = true
		}
		for _, song := range rebased.Queue {
			if !known[song.VideoID] {
				session.Queue = append(session.Queue, song)
				known[song.VideoID] = true
			}
		}
		session.Continuation = rebased.Continuation
		session.PlaylistID = rebased.PlaylistID
		session.Seed = rebased.Seed
		fetched = true
	}
	if session.Cursor+1 >= len(session.Queue) {
		return NextResult{}, fmt.Errorf("la radio de %s no trajo canciones nuevas", session.Seed)
	}
	session.Cursor++
	if err := SaveSession(dataDir, session); err != nil {
		return NextResult{}, err
	}
	return NextResult{
		Session:      session.ID,
		Song:         session.Queue[session.Cursor],
		Cursor:       session.Cursor,
		QueueLength:  len(session.Queue),
		FetchedMore:  fetched,
		Continuation: session.Continuation,
	}, nil
}

// Previous vuelve una canción si la cola ya avanzó.
func Previous(dataDir, id string) (NextResult, error) {
	session, err := LoadSession(dataDir, id)
	if err != nil {
		return NextResult{}, err
	}
	if session.Cursor == 0 {
		return NextResult{}, fmt.Errorf("la sesión %s ya está en la primera canción", session.ID)
	}
	session.Cursor--
	if err := SaveSession(dataDir, session); err != nil {
		return NextResult{}, err
	}
	return NextResult{
		Session:     session.ID,
		Song:        session.Queue[session.Cursor],
		Cursor:      session.Cursor,
		QueueLength: len(session.Queue),
	}, nil
}

// fetchMore pide la siguiente tanda del automix.
func fetchMore(ctx context.Context, c *Client, session Session) ([]Song, string) {
	if session.Continuation == "" {
		return nil, ""
	}
	payload, err := c.RadioContinuation(ctx, session.Continuation, session.Queue[len(session.Queue)-1].VideoID, session.PlaylistID)
	if err != nil {
		return nil, ""
	}
	fresh := songDedup(songsFromRenderers(payload))
	known := make(map[string]bool, len(session.Queue))
	for _, song := range session.Queue {
		known[song.VideoID] = true
	}
	out := make([]Song, 0, len(fresh))
	for _, song := range fresh {
		if !known[song.VideoID] {
			out = append(out, song)
			known[song.VideoID] = true
		}
	}
	return out, continuationOf(payload)
}

// Shuffle mezcla la cola YA traída (Fisher-Yates con crypto/rand) y deja la
// canción actual primero: un shuffle que cambia lo que está sonando es una
// patada, no un shuffle.
func Shuffle(dataDir, id string) (Session, error) {
	session, err := LoadSession(dataDir, id)
	if err != nil {
		return Session{}, err
	}
	if len(session.Queue) < 2 {
		return session, nil
	}
	current := session.Queue[session.Cursor]
	rest := make([]Song, 0, len(session.Queue)-1)
	rest = append(rest, session.Queue[:session.Cursor]...)
	rest = append(rest, session.Queue[session.Cursor+1:]...)
	for index := len(rest) - 1; index > 0; index-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return Session{}, fmt.Errorf("shuffle: %w", err)
		}
		other := int(n.Int64())
		rest[index], rest[other] = rest[other], rest[index]
	}
	queue := make([]Song, 0, len(session.Queue))
	queue = append(queue, current)
	queue = append(queue, rest...)
	session.Queue = queue
	session.Cursor = 0
	if err := SaveSession(dataDir, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// Enqueue agrega una canción a la cola de la sesión (play de un resultado de
// búsqueda sin perder la radio).
func Enqueue(dataDir, id string, song Song) (Session, error) {
	session, err := LoadSession(dataDir, id)
	if err != nil {
		return Session{}, err
	}
	for _, known := range session.Queue {
		if known.VideoID == song.VideoID {
			return session, nil
		}
	}
	session.Queue = append(session.Queue, song)
	if err := SaveSession(dataDir, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// QueueView es la cola tal como la muestra el panel.
type QueueView struct {
	Session string `json:"session"`
	Seed    string `json:"seed"`
	Items   []Song `json:"items"`
	Cursor  int    `json:"cursor"`
	Updated int64  `json:"updated"`
}

func Queue(dataDir, id string) (QueueView, error) {
	session, err := LoadSession(dataDir, id)
	if err != nil {
		return QueueView{}, err
	}
	return QueueView{
		Session: session.ID,
		Seed:    session.Seed,
		Items:   session.Queue,
		Cursor:  session.Cursor,
		Updated: nowUnix(),
	}, nil
}
