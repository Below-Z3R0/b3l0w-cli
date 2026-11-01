package anilist

import "context"

const saveQuery = `mutation ($mediaId: Int, $progress: Int, $status: MediaListStatus) {
  SaveMediaListEntry(mediaId: $mediaId, progress: $progress, status: $status) {
    id mediaId progress status
  }
}`

// Mutation es el resultado de guardar progreso.
type Mutation struct {
	ID       int    `json:"id"`
	MediaID  int    `json:"mediaId"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
}

// SaveProgress actualiza el progreso de una entrada. Status vacío deja el
// estado como está (la API interpreta nil como "no tocar").
func SaveProgress(ctx context.Context, c *Client, mediaID, progress int, status string) (Mutation, error) {
	var data struct {
		Save MediaListEntry `json:"SaveMediaListEntry"`
	}
	variables := map[string]any{"mediaId": mediaID, "progress": progress}
	if status != "" {
		variables["status"] = status
	}
	if err := c.Query(ctx, saveQuery, variables, &data); err != nil {
		return Mutation{}, err
	}
	return Mutation{ID: data.Save.ID, MediaID: data.Save.MediaID, Progress: data.Save.Progress, Status: data.Save.Status}, nil
}

// MediaListEntry es la fila devuelta por la mutación.
type MediaListEntry struct {
	ID       int    `json:"id"`
	MediaID  int    `json:"mediaId"`
	Progress int    `json:"progress"`
	Status   string `json:"status"`
}
