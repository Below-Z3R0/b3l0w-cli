package plugins

import (
	"sort"
	"strconv"
	"strings"
)

// CatalogEntry es una fila [[plugin]] del catalog.toml.
type CatalogEntry struct {
	ID        string
	Version   string
	PluginAPI int
	UpdatedAt int64
	Releases  []CatalogRelease
}

// CatalogRelease es una fila [[plugin.release]]: una revisión anterior
// del plugin, con el commit exacto para检出la.
type CatalogRelease struct {
	Version   string
	PluginAPI int
	Rev       string
	UpdatedAt int64
}

// Catalog es el catálogo de un source: la fila tip de cada plugin y
// sus revisiones anteriores.
type Catalog struct {
	Entries map[string]CatalogEntry
}

// ParseCatalog lee un catalog.toml de Noctalia.
//
// El formato tiene una fila [[plugin]] por plugin (la más nueva) y una o
// más filas [[plugin.release]] con las revisiones anteriores que siguen
// siendo instalables en shells con menos API. La release no lleva id:
// hereda el de la fila [[plugin]] anterior.
func ParseCatalog(text string) *Catalog {
	c := &Catalog{
		Entries: make(map[string]CatalogEntry),
	}
	var current CatalogEntry
	inRelease := false
	var release CatalogRelease
	haveRelease := false

	// flushRelease guarda la release acumulada cuando empieza otra fila.
	flushRelease := func() {
		if haveRelease && current.ID != "" {
			current.Releases = append(current.Releases, release)
		}
		release = CatalogRelease{}
		haveRelease = false
	}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "[[plugin]]":
			flushRelease()
			if current.ID != "" {
				c.Entries[current.ID] = current
			}
			current = CatalogEntry{}
			inRelease = false
			continue
		case line == "[[plugin.release]]":
			flushRelease()
			inRelease = true
			haveRelease = true
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitTOML(line)
		if !ok {
			continue
		}
		if inRelease {
			// El orden de las claves no está garantizado: plugin_api
			// puede venir antes que version. Se acumulan y se guardan
			// cuando aparece la siguiente fila.
			switch key {
			case "version":
				release.Version = value
			case "plugin_api":
				if n, err := strconv.Atoi(value); err == nil {
					release.PluginAPI = n
				}
			case "rev":
				release.Rev = value
			case "updated_at":
				if n, err := strconv.ParseInt(value, 10, 64); err == nil {
					release.UpdatedAt = n
				}
			}
			continue
		}
		switch key {
		case "id":
			current.ID = value
		case "version":
			current.Version = value
		case "plugin_api":
			if n, err := strconv.Atoi(value); err == nil {
				current.PluginAPI = n
			}
		case "updated_at":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				current.UpdatedAt = n
			}
		}
	}
	flushRelease()
	if current.ID != "" {
		c.Entries[current.ID] = current
	}
	return c
}

// splitTOML parte una línea "key = value" y quita las comillas.
func splitTOML(line string) (key, value string, ok bool) {
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return key, value, key != ""
}

// Resolution es lo que Noctalia instalaría para un plugin con el API
// disponible del shell.
type Resolution struct {
	Version   string
	PluginAPI int
	// HeldBack es true cuando se installa una revisión anterior en vez
	// de la más nueva, porque la más nueva pide más API del que el
	// shell soporta.
	HeldBack bool
	// TipVersion y TipAPI son los de la fila más nueva.
	TipVersion string
	TipAPI     int
	// Installable es false cuando ninguna revisión del catálogo es
	// compatible con el shell.
	Installable bool
}

// Resolve dice qué versión instalaría Noctalia con el API dado.
//
// La regla de Noctalia es "instala la revisión más nueva que su rango de
// API permite": la tip si su API entra en el rango, si no la release
// compatible más reciente.
func (c *Catalog) Resolve(id string, shellAPI int) Resolution {
	entry, ok := c.Entries[id]
	if !ok {
		return Resolution{Installable: false}
	}
	res := Resolution{
		Version:     entry.Version,
		PluginAPI:   entry.PluginAPI,
		TipVersion:  entry.Version,
		TipAPI:      entry.PluginAPI,
		Installable: true,
	}
	if entry.PluginAPI <= shellAPI {
		return res
	}
	// La tip no entra: buscar la release compatible más reciente.
	releases := append([]CatalogRelease(nil), entry.Releases...)
	sort.Slice(releases, func(i, j int) bool { return releases[i].PluginAPI > releases[j].PluginAPI })
	for _, rel := range releases {
		if rel.PluginAPI <= shellAPI {
			res.Version = rel.Version
			res.PluginAPI = rel.PluginAPI
			res.HeldBack = true
			return res
		}
	}
	// Ninguna revisión compatible: no se puede instalar.
	res.Installable = false
	res.Version = ""
	return res
}
