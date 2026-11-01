// Package anilist habla con la API GraphQL de AniList y mantiene el caché de
// carátulas que consume el plugin de Noctalia.
//
// El plugin ya no descarga ni pagina: todo eso vive acá. Así el service de Luau
// sólo publica estado y el trabajo caro no compite con el presupuesto de 25 ms.
package anilist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DataDir devuelve el directorio de datos del plugin de Noctalia para un id.
// El host lo crea, pero puede no existir si el plugin nunca corrió.
func DataDir(stateHome, pluginID string) string {
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "noctalia", "plugins", "data", filepath.FromSlash(pluginID))
}

// Token lee el token de AniList. Orden: flag explícito, variable de entorno,
// token.json del plugin. Nunca se imprime: el error sólo dice qué falta.
func Token(explicit, env, dataDir string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env != "" {
		if value := os.Getenv(env); value != "" {
			return value, nil
		}
	}
	if dataDir == "" {
		return "", fmt.Errorf("no hay token: falta --token, %s o --data-dir", env)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "token.json"))
	if err != nil {
		return "", fmt.Errorf("no hay token: %w", err)
	}
	var stored struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", fmt.Errorf("token.json inválido: %w", err)
	}
	if stored.AccessToken == "" {
		return "", fmt.Errorf("token.json sin access_token")
	}
	return stored.AccessToken, nil
}
