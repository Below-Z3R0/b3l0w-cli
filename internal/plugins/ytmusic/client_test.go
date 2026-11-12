package ytmusic

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestClientContextTraeElClienteWEBREMIX: si YouTube rechaza la versión, la
// respuesta viene vacía y el panel muestra "0 canciones".
func TestClientContextTraeElClienteWEBREMIX(t *testing.T) {
	client := NewClient()
	client.HL = "es"
	client.GL = "PA"
	ctx := client.context()
	inner := asMap(ctx["client"])
	if inner == nil {
		t.Fatal("el contexto no trae el cliente")
	}
	if asString(inner["clientName"]) != "WEB_REMIX" {
		t.Errorf("clientName = %q, quería WEB_REMIX", asString(inner["clientName"]))
	}
	if asString(inner["clientVersion"]) != ClientVersion {
		t.Errorf("clientVersion = %q, quería %q", asString(inner["clientVersion"]), ClientVersion)
	}
	if asString(inner["hl"]) != "es" || asString(inner["gl"]) != "PA" {
		t.Errorf("hl/gl = %q/%q, quería es/PA", asString(inner["hl"]), asString(inner["gl"]))
	}
}

// TestClientBodyPoneElContextoYLoExtra: el sobre de innertube lleva el
// contexto y encima lo del endpoint.
func TestClientBodyPoneElContextoYLoExtra(t *testing.T) {
	client := NewClient()
	body := client.Body(map[string]any{"browseId": "FEmusic_home", "params": "P"})
	if asMap(body["context"]) == nil {
		t.Error("el body no trae el contexto")
	}
	if asString(body["browseId"]) != "FEmusic_home" {
		t.Errorf("browseId = %q", asString(body["browseId"]))
	}
	if asString(body["params"]) != "P" {
		t.Errorf("params = %q", asString(body["params"]))
	}
}

// TestShortAcortaElEndpoint: el error al panel tiene que decir qué endpoint
// falló, no "error desconocido".
func TestShortAcortaElEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://music.youtube.com/youtubei/v1/browse?prettyPrint=false": "browse",
		"https://music.youtube.com/youtubei/v1/next?prettyPrint=false":   "next",
		"https://music.youtube.com/youtubei/v1/search?prettyPrint=false": "search",
		"https://music.youtube.com/youtubei/v1/player?prettyPrint=false": "player",
		"https://otro.host/x": "https://otro.host/x",
	}
	for endpoint, want := range cases {
		if got := short(endpoint); got != want {
			t.Errorf("short(%q) = %q, quería %q", endpoint, got, want)
		}
	}
}

// TestSnippetRecortaElCuerpo: el error no puede volcar un JSON de 1 MB en el
// panel.
func TestSnippetRecortaElCuerpo(t *testing.T) {
	if got := snippet([]byte("  hola  ")); got != "hola" {
		t.Errorf("snippet = %q, quería hola", got)
	}
	if got := snippet(nil); got != "sin cuerpo" {
		t.Errorf("snippet(nil) = %q, quería sin cuerpo", got)
	}
	long := strings.Repeat("x", 400)
	got := snippet([]byte(long))
	if len([]rune(got)) != 161 {
		t.Errorf("snippet recortó a %d caracteres, quería 161", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("el recorte no marca el corte: %q", got)
	}
}

// TestPlayabilityErrorDetectaElRechazo: sin esto el llamador ve "0
// canciones" y parece que la pantalla está vacía.
func TestPlayabilityErrorDetectaElRechazo(t *testing.T) {
	if err := playabilityError(map[string]any{}); err != nil {
		t.Errorf("sin playabilityStatus no hay error: %v", err)
	}
	ok := map[string]any{"playabilityStatus": map[string]any{"status": "OK"}}
	if err := playabilityError(ok); err != nil {
		t.Errorf("status OK no es error: %v", err)
	}
	rejected := map[string]any{
		"playabilityStatus": map[string]any{
			"status": "LOGIN_REQUIRED",
			"reason": "Inicia sesión",
		},
	}
	err := playabilityError(rejected)
	if err == nil {
		t.Fatal("un cliente rechazado tiene que ser error")
	}
	if !strings.Contains(err.Error(), "LOGIN_REQUIRED") {
		t.Errorf("el error no trae el estado: %v", err)
	}
	if !strings.Contains(err.Error(), "Inicia sesión") {
		t.Errorf("el error no trae el motivo: %v", err)
	}
}

// TestPlayabilityErrorSinMotivo: el motivo vacío no deja el error mudo.
func TestPlayabilityErrorSinMotivo(t *testing.T) {
	rejected := map[string]any{
		"playabilityStatus": map[string]any{"status": "UNPLAYABLE"},
	}
	err := playabilityError(rejected)
	if err == nil {
		t.Fatal("tenía que ser error")
	}
	if !strings.Contains(err.Error(), "sin motivo") {
		t.Errorf("el error no explica la falta de motivo: %v", err)
	}
}

// TestPostTraeLosHeadersDelClienteWEB: el servidor innertube rechaza las
// peticiones sin los headers del cliente.
func TestPostTraeLosHeadersDelClienteWEB(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		if got := r.Header.Get("X-YouTube-Client-Name"); got != "67" {
			t.Errorf("X-YouTube-Client-Name = %q, quería 67", got)
		}
		if got := r.Header.Get("X-YouTube-Client-Version"); got != ClientVersion {
			t.Errorf("X-YouTube-Client-Version = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Origin"); got != "https://music.youtube.com" {
			t.Errorf("Origin = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Error("falta el User-Agent")
		}
		jsonResponse(w, map[string]any{"ok": true})
	})
	payload, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/browse", fake.client.Body(nil))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if payload["ok"] != true {
		t.Errorf("payload = %+v", payload)
	}
}

// TestPostFallaConHTTP4xx: el error trae el status y un fragmento del cuerpo.
func TestPostFallaConHTTP4xx(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"prohibido"}`))
	})
	_, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/browse", fake.client.Body(nil))
	if err == nil {
		t.Fatal("un 403 tiene que ser error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("el error no trae el status: %v", err)
	}
	if !strings.Contains(err.Error(), "prohibido") {
		t.Errorf("el error no trae el cuerpo: %v", err)
	}
}

// TestPostFallaConCuerpoQueNoEsJSON: innertube devuelve HTML cuando rechaza
// al cliente.
func TestPostFallaConCuerpoQueNoEsJSON(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>no soy json</html>`))
	})
	_, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/browse", fake.client.Body(nil))
	if err == nil {
		t.Fatal("un cuerpo que no es JSON tiene que ser error")
	}
	if !strings.Contains(err.Error(), "no es JSON") {
		t.Errorf("el error no lo dice: %v", err)
	}
}

// TestPostFallaConRespuestaVacia: un `{}` es una respuesta vacía, no un feed
// sin canciones.
func TestPostFallaConRespuestaVacia(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{})
	})
	_, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/browse", fake.client.Body(nil))
	if err == nil {
		t.Fatal("una respuesta vacía tiene que ser error")
	}
	if !strings.Contains(err.Error(), "vacía") {
		t.Errorf("el error no dice que está vacía: %v", err)
	}
}

// TestPostDetectaElRechazoDeYouTube: el status de playability se traduce a un
// error legible.
func TestPostDetectaElRechazoDeYouTube(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"playabilityStatus": map[string]any{
				"status": "UNPLAYABLE",
				"reason": "El video no está disponible",
			},
		})
	})
	_, err := fake.client.Post(context.Background(), "https://music.youtube.com/youtubei/v1/next", fake.client.Body(nil))
	if err == nil {
		t.Fatal("un playabilityStatus malo tiene que ser error")
	}
	if !strings.Contains(err.Error(), "UNPLAYABLE") {
		t.Errorf("el error no trae el estado: %v", err)
	}
}

// TestPostRawIgnoraElPlayability: el endpoint player devuelve los metadatos
// aunque el video no sea reproducible sin sesión.
func TestPostRawIgnoraElPlayability(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{
			"playabilityStatus": map[string]any{"status": "LOGIN_REQUIRED"},
			"videoDetails":      map[string]any{"lengthSeconds": "100"},
		})
	})
	payload, err := fake.client.PostRaw(context.Background(), "https://music.youtube.com/youtubei/v1/player", fake.client.Body(nil))
	if err != nil {
		t.Fatalf("PostRaw no debe mirar el playability: %v", err)
	}
	if asMap(payload["videoDetails"]) == nil {
		t.Errorf("payload = %+v", payload)
	}
}

// TestBrowseMandaElBrowseIDYLosParams.
func TestBrowseMandaElBrowseIDYLosParams(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.Browse(context.Background(), "FEmusic_home", "PARAM"); err != nil {
		t.Fatalf("Browse: %v", err)
	}
	body := fake.body(0)
	if asString(body["browseId"]) != "FEmusic_home" {
		t.Errorf("browseId = %q", asString(body["browseId"]))
	}
	if asString(body["params"]) != "PARAM" {
		t.Errorf("params = %q", asString(body["params"]))
	}
	if fake.calls()[0] != "/youtubei/v1/browse" {
		t.Errorf("consultó %q", fake.calls()[0])
	}
}

// TestBrowseSinParamsNoMandaLaClave: innertube ignora un params vacío.
func TestBrowseSinParamsNoMandaLaClave(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.Browse(context.Background(), "FEmusic_home", ""); err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if _, exists := fake.body(0)["params"]; exists {
		t.Error("no debía mandar params vacío")
	}
}

// TestBrowseContinuationMandaElToken.
func TestBrowseContinuationMandaElToken(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.BrowseContinuation(context.Background(), "TOKEN-1"); err != nil {
		t.Fatalf("BrowseContinuation: %v", err)
	}
	if asString(fake.body(0)["continuation"]) != "TOKEN-1" {
		t.Errorf("continuation = %q", asString(fake.body(0)["continuation"]))
	}
}

// TestSearchMandaElFiltroDeCanciones: sin el params del buscador, la mitad de
// los resultados son videos de gente hablando.
func TestSearchMandaElFiltroDeCanciones(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.Search(context.Background(), "daft punk", ""); err != nil {
		t.Fatalf("Search: %v", err)
	}
	body := fake.body(0)
	if asString(body["query"]) != "daft punk" {
		t.Errorf("query = %q", asString(body["query"]))
	}
	if asString(body["params"]) != searchSongsParams {
		t.Errorf("params = %q, quería el filtro de canciones", asString(body["params"]))
	}
	if _, exists := body["continuation"]; exists {
		t.Error("la primera página no lleva continuation")
	}
}

// TestSearchEnLaPaginaSiguienteMandaElToken: con continuation no se manda el
// filtro, o la página siguiente viene mezclada.
func TestSearchEnLaPaginaSiguienteMandaElToken(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.Search(context.Background(), "daft punk", "TOKEN-2"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	body := fake.body(0)
	if asString(body["continuation"]) != "TOKEN-2" {
		t.Errorf("continuation = %q", asString(body["continuation"]))
	}
	if _, exists := body["params"]; exists {
		t.Error("la página siguiente no debe llevar el params de la primera")
	}
}

// TestRadioMandaElPlaylistIDDelAutomix.
func TestRadioMandaElPlaylistIDDelAutomix(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.Radio(context.Background(), "seed01"); err != nil {
		t.Fatalf("Radio: %v", err)
	}
	body := fake.body(0)
	if asString(body["videoId"]) != "seed01" {
		t.Errorf("videoId = %q", asString(body["videoId"]))
	}
	if asString(body["playlistId"]) != "RDAMVMseed01" {
		t.Errorf("playlistId = %q, quería RDAMVMseed01", asString(body["playlistId"]))
	}
	if asString(body["tunerSettingValue"]) != "AUTOMIX_SETTING_NORMAL" {
		t.Errorf("tunerSettingValue = %q", asString(body["tunerSettingValue"]))
	}
	if fake.calls()[0] != "/youtubei/v1/next" {
		t.Errorf("consultó %q, quería next", fake.calls()[0])
	}
}

// TestRadioClienteSinVideoIDEsError: el cliente no arma la petición.
func TestRadioClienteSinVideoIDEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar la red sin videoId")
	})
	if _, err := fake.client.Radio(context.Background(), ""); err == nil {
		t.Fatal("sin videoId tiene que ser error")
	}
}

// TestRadioContinuationMandaElTokenYLaPlaylist.
func TestRadioContinuationMandaElTokenYLaPlaylist(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		jsonResponse(w, map[string]any{"ok": true})
	})
	if _, err := fake.client.RadioContinuation(context.Background(), "TOK", "seed01", ""); err != nil {
		t.Fatalf("RadioContinuation: %v", err)
	}
	body := fake.body(0)
	if asString(body["continuation"]) != "TOK" {
		t.Errorf("continuation = %q", asString(body["continuation"]))
	}
	if asString(body["playlistId"]) != "RDAMVMseed01" {
		t.Errorf("playlistId = %q, quería que se derivara del videoId", asString(body["playlistId"]))
	}
}

// TestRadioContinuationSinTokenEsError.
func TestRadioContinuationSinTokenEsError(t *testing.T) {
	fake := newFakeInnertube(t, func(w http.ResponseWriter, r *http.Request, body map[string]any) {
		t.Error("no debía consultar la red sin continuation")
	})
	if _, err := fake.client.RadioContinuation(context.Background(), "", "seed01", "PL"); err == nil {
		t.Fatal("sin continuation tiene que ser error")
	}
	if len(fake.calls()) != 0 {
		t.Errorf("hizo %d peticiones, quería 0", len(fake.calls()))
	}
}
