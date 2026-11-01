package docker

import (
	"sort"
	"strconv"
	"strings"
)

// Port es un puerto publicado, ya deduplicado.
//
// `docker ps` lista el mismo puerto dos veces (0.0.0.0 y [::]) y a veces repite
// el interno cuando conviven tcp y tcp6: el original mostraba chips duplicados.
type Port struct {
	HostIP    string `json:"hostIp"`
	HostPort  int    `json:"hostPort"`
	Container int    `json:"containerPort"`
	Proto     string `json:"proto"`
	URL       string `json:"url,omitempty"`
}

// ParsePorts toma el campo Ports de `docker ps` y devuelve la lista limpia,
// ordenada por puerto de host.
func ParsePorts(raw string) []Port {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []Port
	for _, trozo := range strings.Split(raw, ",") {
		trozo = strings.TrimSpace(trozo)
		if trozo == "" {
			continue
		}
		port := Port{Proto: "tcp"}
		// formato: 0.0.0.0:8080->80/tcp  |  [::]:8080->80/tcp  |  80/tcp
		if idx := strings.LastIndex(trozo, "/"); idx >= 0 {
			port.Proto = trozo[idx+1:]
			trozo = trozo[:idx]
		}
		container := 0
		if idx := strings.LastIndex(trozo, "->"); idx >= 0 {
			container = atoi(trozo[idx+2:])
			trozo = trozo[:idx]
		} else {
			container = atoi(trozo)
			trozo = ""
		}
		hostIP, hostPort := "", 0
		if trozo != "" {
			if idx := strings.LastIndex(trozo, ":"); idx >= 0 {
				hostIP = strings.Trim(trozo[:idx], "[]")
				hostPort = atoi(trozo[idx+1:])
			} else {
				hostPort = atoi(trozo)
			}
		}
		if container == 0 {
			continue
		}
		// La clave ignora la IP: 0.0.0.0 y [::] son el mismo puerto publicado.
		clave := strconv.Itoa(hostPort) + ":" + strconv.Itoa(container) + "/" + port.Proto
		if seen[clave] {
			continue
		}
		seen[clave] = true
		port.Container = container
		port.HostPort = hostPort
		port.HostIP = hostIP
		if hostPort > 0 {
			port.URL = "http://localhost:" + strconv.Itoa(hostPort)
		}
		out = append(out, port)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HostPort < out[j].HostPort })
	return out
}

func atoi(s string) int {
	value, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return value
}
