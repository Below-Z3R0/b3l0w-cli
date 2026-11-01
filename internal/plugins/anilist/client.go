package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Endpoint es el único host que se consulta.
const Endpoint = "https://graphql.anilist.co"

// Client es un cliente GraphQL mínimo con reintentos.
type Client struct {
	HTTP  *http.Client
	Token string
	Retry time.Duration
}

// NewClient arma un cliente con timeouts razonables. Retry es la espera base
// del backoff (se duplica en cada intento).
func NewClient(token string) *Client {
	return &Client{
		HTTP:  &http.Client{Timeout: 25 * time.Second},
		Token: token,
		Retry: 800 * time.Millisecond,
	}
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Query ejecuta una consulta y decodifica `data` en out.
//
// Reintenta en 429 y 5xx respetando Retry-After: la API de AniList corta por
// ritmo y sin backoff una biblioteca de 600 entradas falla a mitad de camino.
func (c *Client) Query(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	wait := c.Retry
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			wait *= 2
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, Endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "b3l0w-anilist")
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			if after, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && after > 0 {
				wait = time.Duration(after) * time.Second
			}
			continue
		case resp.StatusCode == http.StatusUnauthorized:
			return fmt.Errorf("token rechazado (401): renovar sesión de AniList")
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncar(string(payload), 160))
		}
		var parsed gqlResponse
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return fmt.Errorf("respuesta inválida: %w", err)
		}
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("AniList: %s", parsed.Errors[0].Message)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(parsed.Data, out)
	}
	return fmt.Errorf("sin respuesta tras reintentos: %w", lastErr)
}

func truncar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
