package ytmusic

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestClockOfCubreLasFormasDelPanel: el panel muestra "3:45" y "1:02:03".
func TestClockOfCubreLasFormasDelPanel(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		{0, ""},
		{-1, ""},
		{59, "0:59"},
		{60, "1:00"},
		{225, "3:45"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{3723, "1:02:03"},
	}
	for _, item := range cases {
		if got := clockOf(item.seconds); got != item.want {
			t.Errorf("clockOf(%d) = %q, quería %q", item.seconds, got, item.want)
		}
	}
}

// TestDurationToSecondsConEntradasRareas: una duración rota tiene que dar 0,
// no un número inventado.
func TestDurationToSecondsConEntradasRareas(t *testing.T) {
	cases := map[string]int{
		"":        0,
		"   ":     0,
		"3:45":    225,
		" 3:45 ":  225,
		"1:02:03": 3723,
		"5":       5,
		"0:00":    0,
		"::":      0,
		":30":     0,
		"3:45:":   0,
		"-5":      0,
		"3:4x":    0,
		"abc":     0,
		"1:2:3:4": 223384,
	}
	for input, want := range cases {
		if got := durationToSeconds(input); got != want {
			t.Errorf("durationToSeconds(%q) = %d, quería %d", input, got, want)
		}
	}
}

// TestVideoSecondsLeeElPlayer: la duración sale de videoDetails.lengthSeconds.
func TestVideoSecondsLeeElPlayer(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"videoDetails": map[string]any{"lengthSeconds": "225"},
		})
	})
	seconds, err := videoSeconds(context.Background(), fake.client, "abc123")
	if err != nil {
		t.Fatalf("videoSeconds: %v", err)
	}
	if seconds != 225 {
		t.Errorf("seconds = %d, quería 225", seconds)
	}
	if path := fake.calls()[0]; path != "/youtubei/v1/player" {
		t.Errorf("consultó %q, quería el endpoint player", path)
	}
	if id := fake.body(0)["videoId"]; id != "abc123" {
		t.Errorf("videoId enviado = %v", id)
	}
}

// TestVideoSecondsSinVideoIDNoSaleALaRed: sin id no hay a quién preguntar.
func TestVideoSecondsSinVideoIDNoSaleALaRed(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar la red sin videoId")
	})
	if _, err := videoSeconds(context.Background(), fake.client, ""); err == nil {
		t.Fatal("un videoId vacío tiene que ser error")
	}
	if len(fake.calls()) != 0 {
		t.Errorf("hizo %d peticiones, quería 0", len(fake.calls()))
	}
}

// TestVideoSecondsConRespuestaIncompleta: si el player no trae lengthSeconds,
// el error lo dice; no se inventa una duración de 0 como si fuera válida.
func TestVideoSecondsConRespuestaIncompleta(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"videoDetails": map[string]any{}})
	})
	_, err := videoSeconds(context.Background(), fake.client, "abc123")
	if err == nil {
		t.Fatal("sin lengthSeconds tiene que ser error")
	}
	if !strings.Contains(err.Error(), "lengthSeconds") {
		t.Errorf("el error no menciona lengthSeconds: %v", err)
	}
}

// TestVideoSecondsConLengthSecondsInvalido: "cinco" no son cinco segundos.
func TestVideoSecondsConLengthSecondsInvalido(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"videoDetails": map[string]any{"lengthSeconds": "cinco"},
		})
	})
	_, err := videoSeconds(context.Background(), fake.client, "abc123")
	if err == nil {
		t.Fatal("un lengthSeconds no numérico tiene que ser error")
	}
	if !strings.Contains(err.Error(), "inválido") {
		t.Errorf("el error no dice que es inválido: %v", err)
	}
}

// TestEnrichDurationsSoloPideLasQueFaltan: las estanterías de home traen
// título y artista pero no duración; el player se consulta sólo por esas.
func TestEnrichDurationsSoloPideLasQueFaltan(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		id, _ := body["videoId"].(string)
		seconds := map[string]string{"uno": "225", "dos": "3723"}[id]
		jsonResponse(w, map[string]any{
			"videoDetails": map[string]any{"lengthSeconds": seconds},
		})
	})
	songs := []Song{
		{VideoID: "uno"},
		{VideoID: "ya", Seconds: 100, Duration: "1:40"},
		{VideoID: "dos"},
	}
	got := EnrichDurations(context.Background(), fake.client, songs, 10)
	if len(got) != 3 {
		t.Fatalf("devolvió %d canciones, quería 3", len(got))
	}
	if got[0].Seconds != 225 || got[0].Duration != "3:45" {
		t.Errorf("primera canción = %+v", got[0])
	}
	if got[1].Seconds != 100 || got[1].Duration != "1:40" {
		t.Errorf("la que ya traía duración cambió: %+v", got[1])
	}
	if got[2].Seconds != 3723 || got[2].Duration != "1:02:03" {
		t.Errorf("tercera canción = %+v", got[2])
	}
	if len(fake.calls()) != 2 {
		t.Errorf("hizo %d consultas, quería 2 (sólo las que faltaban)", len(fake.calls()))
	}
}

// TestEnrichDurationsRespetaElLimite: 50 canciones en home y 50 llamadas al
// player es lo que el panel no aguanta.
func TestEnrichDurationsRespetaElLimite(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"videoDetails": map[string]any{"lengthSeconds": "100"},
		})
	})
	songs := []Song{{VideoID: "a"}, {VideoID: "b"}, {VideoID: "c"}}
	got := EnrichDurations(context.Background(), fake.client, songs, 2)
	if len(fake.calls()) != 2 {
		t.Errorf("hizo %d consultas, quería 2 (el límite manda)", len(fake.calls()))
	}
	if got[2].Seconds != 0 {
		t.Errorf("enriquizó la canción fuera del límite: %+v", got[2])
	}
}

// TestEnrichDurationsConLimiteCeroNoHaceNada: un límite inválido no dispara
// peticiones, pero tampoco borra lo que ya venía.
func TestEnrichDurationsConLimiteCeroNoHaceNada(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar el player")
	})
	songs := []Song{{VideoID: "a"}}
	got := EnrichDurations(context.Background(), fake.client, songs, 0)
	if len(got) != 1 || got[0].Seconds != 0 {
		t.Errorf("cambió la lista: %+v", got)
	}
	if len(fake.calls()) != 0 {
		t.Errorf("hizo %d peticiones, quería 0", len(fake.calls()))
	}
}

// TestEnrichDurationsConListaVacia: no hay nada que enricher.
func TestEnrichDurationsConListaVacia(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar el player")
	})
	if got := EnrichDurations(context.Background(), fake.client, nil, 5); len(got) != 0 {
		t.Errorf("devolvió %d canciones, quería 0", len(got))
	}
}

// TestEnrichDurationsIgnoraLosErroresDelPlayer: si el player falla, la canción
// se queda sin duración pero la lista entera sigue en pie.
func TestEnrichDurationsIgnoraLosErroresDelPlayer(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400}}`))
	})
	songs := []Song{{VideoID: "a"}, {VideoID: "b"}}
	got := EnrichDurations(context.Background(), fake.client, songs, 5)
	if len(got) != 2 {
		t.Fatalf("devolvió %d canciones, quería 2", len(got))
	}
	for _, song := range got {
		if song.Seconds != 0 || song.Duration != "" {
			t.Errorf("se inventó una duración: %+v", song)
		}
	}
}

// TestEnrichDurationsRespetaElContextoCancelado: si el panel cortó la
// pantalla, el player no se sigue consultando.
func TestEnrichDurationsRespetaElContextoCancelado(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar el player con el contexto cancelado")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	songs := []Song{{VideoID: "a"}}
	got := EnrichDurations(ctx, fake.client, songs, 5)
	if len(got) != 1 || got[0].Seconds != 0 {
		t.Errorf("cambió la lista: %+v", got)
	}
}

// TestVideoSecondsRespetaElContextoCancelado: la llamada aborta, no cuelga.
func TestVideoSecondsRespetaElContextoCancelado(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar el player con el contexto cancelado")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := videoSeconds(ctx, fake.client, "abc123"); err == nil {
		t.Fatal("con el contexto cancelado tiene que haber error")
	}
}
