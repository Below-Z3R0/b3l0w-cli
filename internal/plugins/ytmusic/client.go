package ytmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClientVersion es la del cliente WEB_REMIX. Si YouTube la rechaza, la respuesta
// viene vacía (no con un 4xx): el error lo detecta quien parsea y lo reporta.
const ClientVersion = "1.20250101.00.00"

const (
	browseURL = "https://music.youtube.com/youtubei/v1/browse?prettyPrint=false"
	nextURL   = "https://music.youtube.com/youtubei/v1/next?prettyPrint=false"
	searchURL = "https://music.youtube.com/youtubei/v1/search?prettyPrint=false"

	// Songs filter del buscador de YTM. Sin este params vienen "todos los
	// resultados" y la mitad son videos de gente hablando.
	searchSongsParams = "EgWKAQIIAWoKEAkQBRAKEAMQBA%3D%3D"

	// Prefijo de playlist "radio" del automix de un video.
	radioPlaylistPrefix = "RDAMVM"
)

// Client es un cliente innertube mínimo con el contexto correcto.
type Client struct {
	HTTP *http.Client
	HL   string
	GL   string
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 25 * time.Second},
		HL:   "en",
		GL:   "US",
	}
}

func (c *Client) context() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB_REMIX",
			"clientVersion": ClientVersion,
			"hl":            c.HL,
			"gl":            c.GL,
		},
		"user": map[string]any{"lockedSafetyMode": false},
	}
}

// Body arma el sobre de innertube. Un endpoint con forma propia (formData,
// continuation, query) se agrega encima del contexto.
func (c *Client) Body(extra map[string]any) map[string]any {
	body := map[string]any{"context": c.context()}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

// Post hace la llamada y devuelve el JSON decodificado. Un fallo de red, un
// 4xx/5xx o un cuerpo que no es JSON devuelven error con el endpoint adentro:
// el service muestra ese texto, así que no puede ser un "error desconocido".
func (c *Client) Post(ctx context.Context, endpoint string, body map[string]any) (map[string]any, error) {
	return c.post(ctx, endpoint, body, true)
}

// PostRaw no mira playabilityStatus. Lo necesita `player`, que devuelve los
// metadatos (duración) aunque el video no sea reproducible sin sesión.
func (c *Client) PostRaw(ctx context.Context, endpoint string, body map[string]any) (map[string]any, error) {
	return c.post(ctx, endpoint, body, false)
}

func (c *Client) post(ctx context.Context, endpoint string, body map[string]any, checkPlayability bool) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("innertube %s: no se pudo armar el pedido: %w", short(endpoint), err)
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("innertube %s: %w", short(endpoint), err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://music.youtube.com")
		req.Header.Set("Referer", "https://music.youtube.com/")
		req.Header.Set("X-YouTube-Client-Name", "67")
		req.Header.Set("X-YouTube-Client-Version", ClientVersion)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("innertube %s: %w", short(endpoint), err)
			time.Sleep(400 * time.Millisecond)
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("innertube %s: leyendo la respuesta: %w", short(endpoint), readErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("innertube %s: HTTP %d: %s", short(endpoint), resp.StatusCode, snippet(payload))
			if resp.StatusCode >= 500 {
				time.Sleep(400 * time.Millisecond)
				continue
			}
			return nil, lastErr
		}
		var decoded map[string]any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("innertube %s: la respuesta no es JSON (%d bytes): %w", short(endpoint), len(payload), err)
		}
		if len(decoded) == 0 {
			return nil, fmt.Errorf("innertube %s: respuesta vacía", short(endpoint))
		}
		// YouTube devuelve un `playabilityStatus` cuando rechaza el cliente; sin
		// esto el llamador ve "0 canciones" y parece que la pantalla está vacía.
		if checkPlayability {
			if err := playabilityError(decoded); err != nil {
				return nil, fmt.Errorf("innertube %s: %w", short(endpoint), err)
			}
		}
		return decoded, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("innertube %s: falló sin detalle", short(endpoint))
	}
	return nil, lastErr
}

func playabilityError(payload map[string]any) error {
	status, ok := payload["playabilityStatus"].(map[string]any)
	if !ok {
		return nil
	}
	state := asString(status["status"])
	if state == "" || state == "OK" {
		return nil
	}
	reason := asString(status["reason"])
	if reason == "" {
		reason = "sin motivo"
	}
	return fmt.Errorf("YouTube rechazó el pedido (%s): %s", state, reason)
}

func short(endpoint string) string {
	cut := strings.TrimPrefix(endpoint, "https://music.youtube.com/youtubei/v1/")
	if index := strings.Index(cut, "?"); index >= 0 {
		cut = cut[:index]
	}
	return cut
}

func snippet(payload []byte) string {
	text := strings.TrimSpace(string(payload))
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	if text == "" {
		return "sin cuerpo"
	}
	return text
}

// Browse pide una página por browseId (con params opcionales).
func (c *Client) Browse(ctx context.Context, browseID, params string) (map[string]any, error) {
	extra := map[string]any{"browseId": browseID}
	if params != "" {
		extra["params"] = params
	}
	return c.Post(ctx, browseURL, c.Body(extra))
}

// BrowseContinuation sigue una página de browse (home).
func (c *Client) BrowseContinuation(ctx context.Context, token string) (map[string]any, error) {
	return c.Post(ctx, browseURL, c.Body(map[string]any{"continuation": token}))
}

// Search busca con el filtro de canciones.
func (c *Client) Search(ctx context.Context, query, continuation string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if continuation == "" {
		body["params"] = searchSongsParams
	} else {
		body["continuation"] = continuation
	}
	return c.Post(ctx, searchURL, c.Body(body))
}

// Radio trae el "watch next" de un video: la cola de automix.
func (c *Client) Radio(ctx context.Context, videoID string) (map[string]any, error) {
	if videoID == "" {
		return nil, fmt.Errorf("radio: falta el videoId")
	}
	body := map[string]any{
		"videoId":                       videoID,
		"playlistId":                    radioPlaylistPrefix + videoID,
		"isAudioOnly":                   true,
		"enablePersistentPlaylistPanel": true,
		"tunerSettingValue":             "AUTOMIX_SETTING_NORMAL",
	}
	return c.Post(ctx, nextURL, c.Body(body))
}

// RadioContinuation pide la siguiente tanda de la misma radio.
func (c *Client) RadioContinuation(ctx context.Context, token, videoID, playlistID string) (map[string]any, error) {
	if token == "" {
		return nil, fmt.Errorf("radio: falta el continuation")
	}
	if playlistID == "" && videoID != "" {
		playlistID = radioPlaylistPrefix + videoID
	}
	body := map[string]any{
		"continuation": token,
		"videoId":      videoID,
		"playlistId":   playlistID,
		"isAudioOnly":  true,
	}
	return c.Post(ctx, nextURL, c.Body(body))
}
