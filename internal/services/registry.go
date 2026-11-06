package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Registry es el registry de imágenes (GHCR). Solo consulta: el CLI
// consume lo que el CI publica, nunca publica.
type Registry struct {
	BaseURL string
	Client  *http.Client
}

// NewRegistry devuelve un registry para Docker Hub o GHCR.
func NewRegistry(host string) *Registry {
	if host == "" {
		host = "ghcr.io"
	}
	return &Registry{
		BaseURL: "https://" + host,
		Client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// imageRef parte "ghcr.io/below-z3r0/argos" en host y repositorio.
func imageRef(image string) (host, repo string) {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		return "ghcr.io", parts[0]
	}
	if !strings.Contains(parts[0], ".") {
		return "docker.io", image
	}
	return parts[0], parts[1]
}

// token pide un token anónimo de pull al registry.
func (r *Registry) token(host, repo string) (string, error) {
	scope := "repository:" + repo + ":pull"
	url := fmt.Sprintf("https://%s/token?scope=%s&service=%s", host, scope, host)
	resp, err := r.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: token HTTP %d", host, resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Token == "" {
		return "", fmt.Errorf("%s: el registry no devolvió token", host)
	}
	return body.Token, nil
}

// Tags lista los tags publicados de una imagen.
func (r *Registry) Tags(image string) ([]string, error) {
	host, repo := imageRef(image)
	tok, err := r.token(host, repo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v2/%s/tags/list", r.BaseURLFor(host), repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: tags HTTP %d", image, resp.StatusCode)
	}
	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	sort.Strings(body.Tags)
	return body.Tags, nil
}

// BaseURLFor devuelve la URL del registry de un host.
func (r *Registry) BaseURLFor(host string) string {
	if host == r.BaseURL[len("https://"):] {
		return r.BaseURL
	}
	return "https://" + host
}

// AvailableVariants deduce qué variantes están publicadas a partir de
// los tags. Los tags de caché ("buildcache-*") no cuentan. La variante
// "default" existe si la imagen tiene algún tag: no lleva sufijo.
func AvailableVariants(tags []string, variants []string) []string {
	var out []string
	for _, v := range variants {
		if v == "default" {
			for _, tag := range tags {
				if !strings.HasPrefix(tag, "buildcache-") {
					out = append(out, v)
					break
				}
			}
			continue
		}
		for _, tag := range tags {
			if strings.HasPrefix(tag, v+"-") || tag == v {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// Digest consulta el digest de un tag.
func (r *Registry) Digest(image, tag string) (string, error) {
	host, repo := imageRef(image)
	tok, err := r.token(host, repo)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", r.BaseURLFor(host), repo, tag)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json")
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s:%s: HTTP %d", image, tag, resp.StatusCode)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("%s:%s: el registry no devolvió digest", image, tag)
	}
	return digest, nil
}

// LatestTag devuelve el tag más nuevo de una variante. Los tags
// "<variante>-<sha>" ordenan por fecha de commit embebida en el sha
// solo si el registry los expone; sin eso se toma el "-latest".
func LatestTag(tags []string, variant string) string {
	for _, tag := range tags {
		if tag == variant+"-latest" {
			return tag
		}
	}
	for _, tag := range tags {
		if strings.HasPrefix(tag, variant+"-") && !strings.HasPrefix(tag, "buildcache-") {
			return tag
		}
	}
	return ""
}
