package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StatsCacheTTL es cuánto vale stats.json antes de volver a AniList. Las
// estadísticas cambian cuando el usuario marca un capítulo, no por minuto; 30
// minutos evita que el panel golpee la API en cada apertura.
const StatsCacheTTL = 30 * time.Minute

// statsGenres es cuántos géneros se publican. Más de cinco no entra en el panel
// y cada uno viaja al runtime de Luau.
const statsGenres = 5

// statsChunkSize es el tamaño de página para el recálculo desde la lista.
// MediaListCollection acepta hasta 500: una lista de 334 entra en una sola
// consulta y el fallback no se come un 429.
const statsChunkSize = 500

// GenreStat es un género con la cantidad de entradas que lo tienen.
type GenreStat struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

// ScoreStat es cuántas entradas tienen una nota dada.
type ScoreStat struct {
	Score int `json:"score"`
	Count int `json:"count"`
}

// AnimeStats son los totales de anime del usuario.
//
// DaysWatched se calcula acá y no en el plugin: el panel no tiene por qué
// dividir, y así el número es el mismo en JSON y en la salida humana.
type AnimeStats struct {
	Count           int         `json:"count"`
	EpisodesWatched int         `json:"episodesWatched"`
	MinutesWatched  int         `json:"minutesWatched"`
	DaysWatched     float64     `json:"daysWatched"`
	MeanScore       float64     `json:"meanScore"`
	Genres          []GenreStat `json:"genres"`
	Scores          []ScoreStat `json:"scores"`
}

// MangaStats son los totales de manga del usuario.
type MangaStats struct {
	Count        int     `json:"count"`
	ChaptersRead int     `json:"chaptersRead"`
	VolumesRead  int     `json:"volumesRead"`
	MeanScore    float64 `json:"meanScore"`
}

// Stats es el contrato que consume el plugin.
type Stats struct {
	User      string     `json:"user"`
	Anime     AnimeStats `json:"anime"`
	Manga     MangaStats `json:"manga"`
	FetchedAt int64      `json:"fetchedAt"`
}

// statsQuery pide el usuario por nombre: el token identifica la sesión, no al
// usuario cuyas estadísticas se quieren (puede consultarse otro perfil).
const statsQuery = `query ($name: String) {
  User(name: $name) {
    name
    statistics {
      anime {
        count episodesWatched minutesWatched meanScore
        genres(limit: 5, sort: COUNT_DESC) { genre count }
        scores { score count }
      }
      manga { count chaptersRead volumesRead meanScore }
    }
  }
}`

// statsListQuery recalcula géneros y notas desde la lista cuando la API
// devuelve los agregados vacíos. `score` va sin `format` a propósito: AniList
// lo devuelve en la escala del usuario (POINT_10, POINT_100, ...), que es la
// misma que usaría statistics.scores.
const statsListQuery = `query ($name: String, $chunk: Int, $perChunk: Int) {
  MediaListCollection(userName: $name, type: ANIME, forceSingleCompletedList: true, chunk: $chunk, perChunk: $perChunk) {
    hasNextChunk
    lists {
      entries {
        mediaId status score
        media { genres }
      }
    }
  }
}`

// StatsPath es el archivo de caché dentro del data dir.
func StatsPath(dataDir string) string {
	return filepath.Join(dataDir, "stats.json")
}

// FetchStats consulta las estadísticas de un usuario concreto. `userName` no
// puede ir vacío: la API resuelve User(name: null) de forma ambigua y acá la
// regla es explícita (el nombre lo resuelve LoadStats con el Viewer).
func FetchStats(ctx context.Context, c *Client, userName string) (Stats, error) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		return Stats{}, fmt.Errorf("falta el nombre de usuario de AniList")
	}
	var data struct {
		User *struct {
			Name       string `json:"name"`
			Statistics struct {
				Anime struct {
					Count           int         `json:"count"`
					EpisodesWatched int         `json:"episodesWatched"`
					MinutesWatched  int         `json:"minutesWatched"`
					MeanScore       float64     `json:"meanScore"`
					Genres          []GenreStat `json:"genres"`
					Scores          []ScoreStat `json:"scores"`
				} `json:"anime"`
				Manga struct {
					Count        int     `json:"count"`
					ChaptersRead int     `json:"chaptersRead"`
					VolumesRead  int     `json:"volumesRead"`
					MeanScore    float64 `json:"meanScore"`
				} `json:"manga"`
			} `json:"statistics"`
		} `json:"User"`
	}
	variables := map[string]any{"name": userName}
	if err := c.Query(ctx, statsQuery, variables, &data); err != nil {
		return Stats{}, err
	}
	// User null es "no existe" o "perfil privado": devolver ceros haría creer
	// que el usuario tiene la lista vacía.
	if data.User == nil {
		return Stats{}, fmt.Errorf("usuario de AniList no encontrado: %s", userName)
	}
	out := Stats{
		User: firstNonEmpty(data.User.Name, userName),
		Anime: AnimeStats{
			Count:           data.User.Statistics.Anime.Count,
			EpisodesWatched: data.User.Statistics.Anime.EpisodesWatched,
			MinutesWatched:  data.User.Statistics.Anime.MinutesWatched,
			MeanScore:       round1(data.User.Statistics.Anime.MeanScore),
			Genres:          data.User.Statistics.Anime.Genres,
			Scores:          data.User.Statistics.Anime.Scores,
		},
		Manga: MangaStats{
			Count:        data.User.Statistics.Manga.Count,
			ChaptersRead: data.User.Statistics.Manga.ChaptersRead,
			VolumesRead:  data.User.Statistics.Manga.VolumesRead,
			MeanScore:    round1(data.User.Statistics.Manga.MeanScore),
		},
		FetchedAt: time.Now().Unix(),
	}
	// AniList cachea los agregados por cuenta y en muchas cuentas genres/scores
	// vuelven vacíos mientras count sí es correcto. Si pasa, se recalculan desde
	// la lista: un panel con el top de géneros en cero no sirve de nada.
	if out.Anime.Count > 0 && len(out.Anime.Genres) == 0 && len(out.Anime.Scores) == 0 {
		genres, scores, err := statsBreakdown(ctx, c, out.User)
		if err == nil {
			out.Anime.Genres = genres
			out.Anime.Scores = scores
		}
	}
	out.normalize()
	return out, nil
}

// statsEntry es una entrada cruda de la lista para recalcular los agregados.
type statsEntry struct {
	MediaID int
	Status  string
	Score   float64
	Genres  []string
}

// statsBreakdown arma géneros y notas desde la lista del usuario. Excluye
// PLANNING igual que statistics.anime.count (que cuenta sólo lo que el usuario
// empezó) y deduplica por mediaId: la misma obra puede estar en dos listas.
func statsBreakdown(ctx context.Context, c *Client, userName string) ([]GenreStat, []ScoreStat, error) {
	byID := map[int]statsEntry{}
	for chunk := 0; ; chunk++ {
		variables := map[string]any{"name": userName, "chunk": chunk, "perChunk": statsChunkSize}
		var page struct {
			MediaListCollection struct {
				HasNextChunk bool `json:"hasNextChunk"`
				Lists        []struct {
					Entries []struct {
						MediaID int     `json:"mediaId"`
						Status  string  `json:"status"`
						Score   float64 `json:"score"`
						Media   struct {
							Genres []string `json:"genres"`
						} `json:"media"`
					} `json:"entries"`
				} `json:"lists"`
			} `json:"MediaListCollection"`
		}
		if err := c.Query(ctx, statsListQuery, variables, &page); err != nil {
			return nil, nil, err
		}
		for _, list := range page.MediaListCollection.Lists {
			for _, raw := range list.Entries {
				entry := statsEntry{MediaID: raw.MediaID, Status: raw.Status, Score: raw.Score, Genres: raw.Media.Genres}
				if prev, ok := byID[entry.MediaID]; ok && !betterStatsEntry(entry, prev) {
					continue
				}
				byID[entry.MediaID] = entry
			}
		}
		if !page.MediaListCollection.HasNextChunk {
			break
		}
	}
	genreCount := map[string]int{}
	scoreCount := map[int]int{}
	for _, entry := range byID {
		if entry.Status == "PLANNING" {
			continue
		}
		for _, genre := range entry.Genres {
			genreCount[genre]++
		}
		// La nota 0 es "sin puntuar": contarla llenaría la distribución de ceros.
		if score := int(math.Round(entry.Score)); score > 0 {
			scoreCount[score]++
		}
	}
	genres := make([]GenreStat, 0, len(genreCount))
	for genre, count := range genreCount {
		genres = append(genres, GenreStat{Genre: genre, Count: count})
	}
	scores := make([]ScoreStat, 0, len(scoreCount))
	for score, count := range scoreCount {
		scores = append(scores, ScoreStat{Score: score, Count: count})
	}
	return genres, scores, nil
}

// betterStatsEntry dice si `next` describe mejor la misma obra que `prev`:
// primero lo que no es PLANNING, después lo que tiene nota.
func betterStatsEntry(next, prev statsEntry) bool {
	if (next.Status != "PLANNING") != (prev.Status != "PLANNING") {
		return next.Status != "PLANNING"
	}
	return next.Score > 0 || prev.Score == 0
}

// normalize deja el contrato listo para el plugin: días con un decimal, top 5
// de géneros y notas ascendentes. Los slices se crean siempre: Luau hace
// `#payload.genres` y un null lo rompe.
func (s *Stats) normalize() {
	s.Anime.DaysWatched = round1(float64(s.Anime.MinutesWatched) / 1440)
	if s.Anime.Genres == nil {
		s.Anime.Genres = []GenreStat{}
	}
	sort.SliceStable(s.Anime.Genres, func(i, j int) bool {
		if s.Anime.Genres[i].Count != s.Anime.Genres[j].Count {
			return s.Anime.Genres[i].Count > s.Anime.Genres[j].Count
		}
		// Empate: alfabético, para que dos corridas den el mismo JSON.
		return s.Anime.Genres[i].Genre < s.Anime.Genres[j].Genre
	})
	if len(s.Anime.Genres) > statsGenres {
		s.Anime.Genres = s.Anime.Genres[:statsGenres]
	}
	if s.Anime.Scores == nil {
		s.Anime.Scores = []ScoreStat{}
	}
	sort.SliceStable(s.Anime.Scores, func(i, j int) bool {
		return s.Anime.Scores[i].Score < s.Anime.Scores[j].Score
	})
}

// round1 redondea a un decimal (el contrato de daysWatched y meanScore).
func round1(value float64) float64 {
	return math.Round(value*10) / 10
}

// LoadStats sirve stats.json si tiene menos de StatsCacheTTL y, si no, consulta
// la API y deja el caché escrito. `userName` vacío usa el dueño del token.
func LoadStats(ctx context.Context, c *Client, dataDir, userName string, refresh bool) (Stats, error) {
	userName = strings.TrimSpace(userName)
	// El caché se consulta ANTES que el Viewer: si el panel abre sin red y hay
	// stats frescas, tiene que responder igual.
	if !refresh {
		if cached, ok := readStatsCache(dataDir, userName, time.Now()); ok {
			return cached, nil
		}
	}
	// Sin nombre no se puede pedir User(name:): el token identifica la sesión,
	// no al usuario cuyas estadísticas se quieren.
	if userName == "" {
		viewer, err := FetchViewer(ctx, c)
		if err != nil {
			return Stats{}, err
		}
		if viewer.Name == "" {
			return Stats{}, fmt.Errorf("el token no devolvió un usuario de AniList")
		}
		userName = viewer.Name
	}
	stats, err := FetchStats(ctx, c, userName)
	if err != nil {
		return Stats{}, err
	}
	// El caché es un atajo, no el resultado: si el disco no deja escribir, la
	// salida sigue siendo válida y el próximo llamado vuelve a consultar.
	_ = writeStatsCache(dataDir, stats)
	return stats, nil
}

// readStatsCache devuelve el caché si es fresco y del usuario pedido.
func readStatsCache(dataDir, userName string, now time.Time) (Stats, bool) {
	if dataDir == "" {
		return Stats{}, false
	}
	raw, err := os.ReadFile(StatsPath(dataDir))
	if err != nil {
		return Stats{}, false
	}
	var cached Stats
	if err := json.Unmarshal(raw, &cached); err != nil {
		return Stats{}, false
	}
	if cached.FetchedAt <= 0 {
		return Stats{}, false
	}
	// Un fetchedAt en el futuro (reloj movido) cuenta como fresco, pero uno
	// viejo por más de la ventana no.
	if now.Sub(time.Unix(cached.FetchedAt, 0)) > StatsCacheTTL {
		return Stats{}, false
	}
	name := userName
	if name == "" {
		// Sin --user el dueño esperado es el de la biblioteca ya sincronizada:
		// si el token cambió de cuenta, el caché es de otro y no sirve.
		name = libraryViewerName(dataDir)
	}
	if name != "" && cached.User != "" && !strings.EqualFold(cached.User, name) {
		return Stats{}, false
	}
	cached.normalize()
	return cached, true
}

// libraryViewerName lee el dueño de library.json, si existe.
func libraryViewerName(dataDir string) string {
	raw, err := os.ReadFile(filepath.Join(dataDir, "library.json"))
	if err != nil {
		return ""
	}
	lib, err := ReadLibraryFile(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(lib.Viewer.Name)
}

// writeStatsCache escribe stats.json con .tmp + rename: el plugin nunca lee un
// archivo a medio escribir.
func writeStatsCache(dataDir string, stats Stats) error {
	if dataDir == "" {
		return fmt.Errorf("sin data dir")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	final := StatsPath(dataDir)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
