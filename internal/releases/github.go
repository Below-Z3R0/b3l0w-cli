// Package releases consulta la firma de los releases de GitHub.
//
// Lo usan los dos: el CLI de usuario final (para saber si un programa
// tiene firma antes de instalarlo) y el mantenedor (para verificar lo
// que acaba de publicar).
//
// Solo va lo que no depende del CLI final: `Check` y `Render` usan el
// manifiesto de programas y la salida, que son del CLI de usuario.
package releases

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Inyectables para los tests.
var (
	apiBase      = "https://api.github.com"
	downloadBase = "https://github.com"
	client       = &http.Client{Timeout: 8 * time.Second}
	// Un 404 del CDN justo después de publicar es transitorio: sin
	// reintento, el doctor marca SIN FIRMA un release que sí está
	// firmado.
	retryDelay = 300 * time.Millisecond
)

// LatestTag devuelve el tag del último release de un repo.
//
// No usa /releases/latest para el CLI propio: ese endpoint puede
// apuntar a un release de paquetes de escritorio que no trae el
// binario. Para el CLI se usa la convención v*.
func LatestTag(repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "b3l0w")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("sin releases")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

// HasSignature dice si un release publica SHA256SUMS.asc.
//
// Reintenta ante 404 y errores de red: el CDN de GitHub tarda unos
// segundos en ver un asset recién subido, y un falso "sin firma" es
// peor que esperar un segundo.
func HasSignature(repo, tag string) (bool, error) {
	url := fmt.Sprintf("%s/%s/releases/download/%s/SHA256SUMS.asc", downloadBase, repo, tag)
	var ultimo error
	for intento := 0; intento < 3; intento++ {
		if intento > 0 {
			time.Sleep(time.Duration(intento) * retryDelay)
		}
		presente, err := headSignature(url)
		if err != nil {
			ultimo = err
			continue
		}
		if presente {
			return true, nil
		}
		// 404: puede ser transitorio (el CDN tarda en ver el asset).
		// Se agotan los intentos antes de decir "no está".
	}
	return false, ultimo
}

// SetBases cambia las bases de la API y de las descargas, y devuelve
// la función que las restaura.
//
// Es la costura para los tests: las bases son variables privadas y los
// tests del CLI final necesitan apuntarlas a un servidor de prueba.
// Devolver el restaurador hace que no haya que acordarse de hacerlo.
func SetBases(api, download string) (restore func()) {
	viejoAPI, viejoDown := apiBase, downloadBase
	apiBase, downloadBase = api, download
	return func() { apiBase, downloadBase = viejoAPI, viejoDown }
}

// headSignature hace un intento. Un 404 no es error: es "no está".
func headSignature(url string) (bool, error) {
	resp, err := client.Head(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusFound:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
}
