package anilist

import (
	"context"
	"fmt"
	"strings"
)

// userByNameQuery busca el usuario de AniList por nombre.
const userByNameQuery = `query ($name: String) { User(name: $name) { id name } }`

// userByIDQuery busca el usuario de AniList por id.
const userByIDQuery = `query ($id: Int) { User(id: $id) { id name } }`

// FetchUserByName devuelve el usuario de AniList con ese nombre. Es lo que
// permite que `--user b3low` funcione igual en todos los subcomandos (B4).
func FetchUserByName(ctx context.Context, c *Client, name string) (Viewer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Viewer{}, fmt.Errorf("falta el nombre de usuario de AniList")
	}
	return fetchUser(ctx, c, userByNameQuery, map[string]any{"name": name}, name)
}

// FetchUserByID devuelve el usuario de AniList con ese id.
func FetchUserByID(ctx context.Context, c *Client, id int) (Viewer, error) {
	if id <= 0 {
		return Viewer{}, fmt.Errorf("id de usuario inválido: %d", id)
	}
	return fetchUser(ctx, c, userByIDQuery, map[string]any{"id": id}, fmt.Sprintf("#%d", id))
}

// fetchUser hace la consulta y traduce "no existe" a un error claro: un
// `User: null` sin error dejaría al comando con datos vacíos.
func fetchUser(ctx context.Context, c *Client, query string, variables map[string]any, quien string) (Viewer, error) {
	var data struct {
		User *Viewer `json:"User"`
	}
	if err := c.Query(ctx, query, variables, &data); err != nil {
		return Viewer{}, err
	}
	if data.User == nil {
		return Viewer{}, fmt.Errorf("usuario de AniList no encontrado: %s", quien)
	}
	return *data.User, nil
}
