// Package anilistcli arma los comandos de `b3l0w anilist`. Acá vive sólo el
// parseo de argumentos y la salida; la lógica está en el paquete anilist.
package anilistcli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/anilist"
)

const usage = `b3l0w anilist — biblioteca y carátulas de AniList

uso: b3l0w anilist <comando> [opciones]

  sync     trae la biblioteca y sincroniza las carátulas (un solo paso)
  discover catálogo: trending | popular | top | new | seasonal
  library  sólo la biblioteca
  covers   sólo las carátulas de una biblioteca ya bajada
  mutate   guarda progreso/estado de una entrada
  stats    totales y géneros del usuario (anime/manga)
  status   estado del token y del caché
  media    ficha de una serie (sinopsis, géneros, próximo capítulo)
  schedule agenda de emisión: los capítulos que vienen

Opciones comunes:
  --json             salida JSON (la que usa el plugin)
  --data-dir DIR     directorio de datos del plugin (default: el de b3l0w/anilist)
  --token T          token explícito (default: token.json del plugin)

sync/covers:
  --covers-dir DIR   dónde guardar los JPG (default: <data-dir>/covers)
  --reuse DIR        adoptar el caché de otro directorio (hardlink)
  --gc               borrar carátulas de entradas que ya no existen
  --workers N        descargas en paralelo (default 6)

mutate:
  --media-id N --progress N [--status CURRENT|COMPLETED|PLANNING|...]

media:
  --media-id N       id de la serie (obligatorio)

stats:
  --user NOMBRE      usuario a consultar (default: el dueño del token)
  --refresh          ignorar el caché de 30 minutos y consultar AniList
`

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
	case "sync":
		return runSync(rest)
	case "library":
		return runLibrary(rest)
	case "covers":
		return runCovers(rest)
	case "mutate":
		return runMutate(rest)
	case "discover":
		return runDiscover(rest)
	case "stats":
		return runStats(rest)
	case "status":
		return runStatus(rest)
	case "media":
		return runMedia(rest)
	case "schedule":
		return runSchedule(rest)
	default:
		fmt.Fprintf(os.Stderr, "comando desconocido: %s\n\n", command)
		fmt.Print(usage)
		return 2
	}
}

// newFlags devuelve el set de flags compartido por sync/library/covers.
func newFlags(name string) (*flag.FlagSet, *bool, *string, *string, *int) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDir := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	user := fs.Int("user", 0, "user id (0 = el del token)")
	return fs, asJSON, dataDir, token, user
}

func client(token string, dataDirFlag string) (*anilist.Client, string, error) {
	dir := dataDirFlag
	if dir == "" {
		dir = anilist.DataDir("", "b3l0w/anilist")
	}
	value, err := anilist.Token(token, "ANILIST_TOKEN", dir)
	if err != nil {
		return nil, dir, err
	}
	return anilist.NewClient(value), dir, nil
}

// payload es lo que come el plugin: biblioteca + mapa de carátulas.
type payload struct {
	anilist.Library
	Covers map[string]string   `json:"covers"`
	Stats  *anilist.CoverStats `json:"stats,omitempty"`
	Dir    string              `json:"coversDir"`
}

func runSync(args []string) int {
	fs, asJSON, dataDirFlag, token, user := newFlags("sync")
	coversDir := fs.String("covers-dir", "", "directorio de carátulas")
	reuse := fs.String("reuse", "", "caché anterior a adoptar")
	gc := fs.Bool("gc", false, "borrar huérfanas")
	workers := fs.Int("workers", 6, "descargas en paralelo")
	fs.Parse(args)

	c, dir, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	target := *coversDir
	if target == "" {
		target = filepath.Join(dir, "covers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	lib, err := anilist.Sync(ctx, c, *user)
	if err != nil {
		return fail(err, *asJSON)
	}
	all := append(append([]anilist.Entry{}, lib.Anime...), lib.Manga...)
	stats, err := anilist.SyncCovers(ctx, all, target, anilist.CoverOptions{
		Workers:  *workers,
		ReuseDir: *reuse,
		GC:       *gc,
	})
	if err != nil {
		return fail(err, *asJSON)
	}
	out := payload{Library: lib, Covers: stats.Covers, Stats: &stats, Dir: stats.Dir}
	return emit(out, *asJSON, func() { printLibrary(lib, stats) })
}

func runLibrary(args []string) int {
	fs, asJSON, dataDirFlag, token, user := newFlags("library")
	fs.Parse(args)
	c, _, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	lib, err := anilist.Sync(ctx, c, *user)
	if err != nil {
		return fail(err, *asJSON)
	}
	return emit(lib, *asJSON, func() { printLibrary(lib, anilist.CoverStats{}) })
}

func runCovers(args []string) int {
	fs, asJSON, dataDirFlag, token, user := newFlags("covers")
	library := fs.String("library", "", "archivo JSON de biblioteca (default: la trae)")
	coversDir := fs.String("covers-dir", "", "directorio de carátulas")
	reuse := fs.String("reuse", "", "caché anterior a adoptar")
	gc := fs.Bool("gc", false, "borrar huérfanas")
	workers := fs.Int("workers", 6, "descargas en paralelo")
	fs.Parse(args)

	_, dir, _ := client(*token, *dataDirFlag)
	target := *coversDir
	if target == "" {
		target = filepath.Join(dir, "covers")
	}
	var lib anilist.Library
	if *library != "" {
		raw, err := os.ReadFile(*library)
		if err != nil {
			return fail(err, *asJSON)
		}
		// Tolerante a `{}` (lista vacía escrita por Luau).
		if lib, err = anilist.ReadLibraryFile(raw); err != nil {
			return fail(err, *asJSON)
		}
	} else {
		c, _, err := client(*token, *dataDirFlag)
		if err != nil {
			return fail(err, *asJSON)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		lib, err = anilist.Sync(ctx, c, *user)
		if err != nil {
			return fail(err, *asJSON)
		}
	}
	all := append(append([]anilist.Entry{}, lib.Anime...), lib.Manga...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stats, err := anilist.SyncCovers(ctx, all, target, anilist.CoverOptions{
		Workers: *workers, ReuseDir: *reuse, GC: *gc,
	})
	if err != nil {
		return fail(err, *asJSON)
	}
	return emit(stats, *asJSON, func() {
		fmt.Printf("carátulas: %d válidas · %d nuevas · %d reusadas · %d fallidas · %d borradas\n",
			len(stats.Covers), stats.Downloaded, stats.Reused, stats.Failed, stats.Removed)
	})
}

func runMutate(args []string) int {
	fs := flag.NewFlagSet("mutate", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	mediaID := fs.Int("media-id", 0, "id de la entrada")
	progress := fs.Int("progress", -1, "progreso nuevo")
	status := fs.String("status", "", "estado nuevo")
	fs.Parse(args)
	if *mediaID == 0 || *progress < 0 {
		return fail(fmt.Errorf("faltan --media-id y --progress"), *asJSON)
	}
	c, _, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := anilist.SaveProgress(ctx, c, *mediaID, *progress, *status)
	if err != nil {
		return fail(err, *asJSON)
	}
	return emit(result, *asJSON, func() {
		fmt.Printf("guardado: media %d → %d (%s)\n", result.MediaID, result.Progress, result.Status)
	})
}

func runDiscover(args []string) int {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	kind := fs.String("kind", "trending", "trending | popular | top | new | seasonal")
	page := fs.Int("page", 1, "página")
	perPage := fs.Int("per-page", 24, "items por página (máx 50)")
	coversDir := fs.String("covers-dir", "", "si se indica, baja las carátulas de esta página")
	fs.Parse(args)

	c, dir, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := anilist.Discover(ctx, c, *kind, *page, *perPage)
	if err != nil {
		return fail(err, *asJSON)
	}
	payload := struct {
		anilist.DiscoverPage
		Covers map[string]string `json:"covers,omitempty"`
	}{DiscoverPage: result}
	if *coversDir != "" {
		entries := make([]anilist.Entry, 0, len(result.Items))
		for _, item := range result.Items {
			entries = append(entries, anilist.Entry{MediaID: item.ID, CoverURL: item.CoverURL})
		}
		stats, err := anilist.SyncCovers(ctx, entries, *coversDir, anilist.CoverOptions{Workers: 6})
		if err != nil {
			return fail(err, *asJSON)
		}
		payload.Covers = stats.Covers
	}
	_ = dir
	return emit(payload, *asJSON, func() {
		fmt.Printf("%s · página %d · %d items (total %d)\n", result.Kind, result.Page, len(result.Items), result.Total)
		for i, item := range result.Items {
			if i >= 12 {
				break
			}
			fmt.Printf("  %4d  %-48.48s %3d  %d ep\n", item.ID, item.Title, item.Score, item.Episodes)
		}
	})
}

func runStats(args []string) int {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	user := fs.String("user", "", "usuario de AniList (default: el del token)")
	refresh := fs.Bool("refresh", false, "ignorar el caché de 30 minutos")
	fs.Parse(args)

	// Igual que el resto: el token manda. Pero acá los errores van sólo a
	// stderr —el plugin lee stdout como JSON y un objeto de error lo rompe—.
	c, dir, err := client(*token, *dataDirFlag)
	if err != nil {
		return statsFail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := anilist.LoadStats(ctx, c, dir, *user, *refresh)
	if err != nil {
		return statsFail(err)
	}
	return emit(result, *asJSON, func() { printStats(result) })
}

// statsFail deja stdout limpio: exit 1 y el motivo en stderr.
func statsFail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

// runMedia trae la ficha de una serie: sinopsis, géneros, capítulos y el
// próximo airing. El panel la pide al abrir el detalle.
func runMedia(args []string) int {
	fs := flag.NewFlagSet("media", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	mediaID := fs.Int("media-id", 0, "id de la media")
	fs.Parse(args)
	if *mediaID <= 0 {
		return fail(fmt.Errorf("falta --media-id"), *asJSON)
	}
	c, _, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	detail, err := anilist.FetchMedia(ctx, c, *mediaID)
	if err != nil {
		return fail(err, *asJSON)
	}
	return emit(detail, *asJSON, func() {
		fmt.Printf("%s (%d)\n", detail.Title, detail.ID)
		if detail.Format != "" {
			fmt.Printf("formato: %s · estado: %s\n", detail.Format, detail.Status)
		}
		if len(detail.Genres) > 0 {
			fmt.Printf("géneros: %s\n", strings.Join(detail.Genres, ", "))
		}
		if detail.NextAiring != nil {
			fmt.Printf("próximo: ep %d el %s\n", detail.NextAiring.Episode,
				time.Unix(detail.NextAiring.AiringAt, 0).Format("2006-01-02 15:04"))
		}
		if detail.Description != "" {
			fmt.Printf("sinopsis: %s\n", detail.Description)
		}
	})
}

// runSchedule trae la agenda de emisión: los capítulos que todavía no se
// emitieron de las series que el usuario está viendo.
func runSchedule(args []string) int {
	fs := flag.NewFlagSet("schedule", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	token := fs.String("token", "", "token de AniList")
	user := fs.Int("user", 0, "user id (0 = el del token)")
	fs.Parse(args)
	c, _, err := client(*token, *dataDirFlag)
	if err != nil {
		return fail(err, *asJSON)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schedule, err := anilist.FetchSchedule(ctx, c, *user)
	if err != nil {
		return fail(err, *asJSON)
	}
	return emit(schedule, *asJSON, func() {
		fmt.Printf("agenda: %d capítulos\n", len(schedule.Items))
		for _, item := range schedule.Items {
			fmt.Printf("  %s ep %d — %s\n", item.Title, item.Episode,
				time.Unix(item.AiringAt, 0).Format("2006-01-02 15:04"))
		}
	})
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "salida JSON")
	dataDirFlag := fs.String("data-dir", "", "directorio de datos del plugin")
	fs.Parse(args)
	dir := *dataDirFlag
	if dir == "" {
		dir = anilist.DataDir("", "b3l0w/anilist")
	}
	hasToken := true
	if _, err := anilist.Token("", "ANILIST_TOKEN", dir); err != nil {
		hasToken = false
	}
	coversDir := filepath.Join(dir, "covers")
	files := 0
	var bytes int64
	if items, err := os.ReadDir(coversDir); err == nil {
		for _, item := range items {
			if strings.HasSuffix(item.Name(), ".jpg") {
				files++
				if info, err := item.Info(); err == nil {
					bytes += info.Size()
				}
			}
		}
	}
	status := map[string]any{
		"dataDir":     dir,
		"hasToken":    hasToken,
		"coversDir":   coversDir,
		"covers":      files,
		"coversBytes": bytes,
	}
	return emit(status, *asJSON, func() {
		fmt.Printf("datos:   %s\ntoken:   %v\ncaché:   %d carátulas (%.1f MB) en %s\n",
			dir, hasToken, files, float64(bytes)/(1<<20), coversDir)
	})
}

func emit(value any, asJSON bool, human func()) int {
	if !asJSON {
		human()
		return 0
	}
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "error de salida:", err)
		return 1
	}
	return 0
}

func fail(err error, asJSON bool) int {
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		// El plugin distingue error por exit code, no por el cuerpo.
		return 1
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

func printLibrary(lib anilist.Library, stats anilist.CoverStats) {
	fmt.Printf("usuario: %s (#%d)\n", lib.Viewer.Name, lib.Viewer.ID)
	fmt.Printf("anime:   %d\nmanga:   %d\n", len(lib.Anime), len(lib.Manga))
	if stats.Dir != "" {
		fmt.Printf("carátulas: %d válidas (%d nuevas, %d fallidas)\n", len(stats.Covers), stats.Downloaded, stats.Failed)
	}
	if lib.Partial {
		fmt.Println("aviso: una de las listas no se pudo traer")
	}
}

func printStats(s anilist.Stats) {
	fmt.Printf("usuario: %s\n", s.User)
	fmt.Printf("anime:   %d · %d ep · %d min (%.1f días) · nota media %.1f\n",
		s.Anime.Count, s.Anime.EpisodesWatched, s.Anime.MinutesWatched, s.Anime.DaysWatched, s.Anime.MeanScore)
	if len(s.Anime.Genres) > 0 {
		parts := make([]string, 0, len(s.Anime.Genres))
		for _, genre := range s.Anime.Genres {
			parts = append(parts, fmt.Sprintf("%s %d", genre.Genre, genre.Count))
		}
		fmt.Printf("géneros: %s\n", strings.Join(parts, " · "))
	}
	fmt.Printf("manga:   %d · %d cap · %d tomos · nota media %.1f\n",
		s.Manga.Count, s.Manga.ChaptersRead, s.Manga.VolumesRead, s.Manga.MeanScore)
	fmt.Printf("datos:   %s\n", time.Unix(s.FetchedAt, 0).Format("2006-01-02 15:04"))
}
