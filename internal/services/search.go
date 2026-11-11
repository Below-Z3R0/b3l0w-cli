package services

import (
	"fmt"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
)

// Match filtra los servicios por id, nombre, descripción o variante.
//
// No es un catálogo remoto: busca sobre el manifiesto que ya está en
// disco. Una consulta vacía devuelve todos, para que `-Ss` sin
// argumentos se comporte como `list`.
func Match(m *Manifest, query string) []Service {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return m.Services
	}
	var out []Service
	for _, s := range m.Services {
		if coincide(s, q) {
			out = append(out, s)
		}
	}
	return out
}

func coincide(s Service, q string) bool {
	campos := []string{s.ID, s.Name, s.Description}
	campos = append(campos, s.Variants...)
	campos = append(campos, s.Components...)
	for _, campo := range campos {
		if strings.Contains(strings.ToLower(campo), q) {
			return true
		}
	}
	return false
}

// recortar acota un texto al ancho de su columna para que no empuje
// las siguientes.
func recortar(s string, ancho int) string {
	if len([]rune(s)) <= ancho {
		return s
	}
	r := []rune(s)
	if ancho <= 1 {
		return string(r[:ancho])
	}
	return string(r[:ancho-1]) + "…"
}

// RenderSearch muestra los servicios que matchean la consulta.
func RenderSearch(m *Manifest, query string) int {
	resultados := Match(m, query)
	if len(resultados) == 0 {
		output.Warn(fmt.Sprintf("ningún servicio coincide con %q", query))
		return 1
	}
	output.Step(fmt.Sprintf("Servicios: %d de %d", len(resultados), len(m.Services)))
	fmt.Printf("  %-10s %-26s %-8s %s\n", "ID", "VARIANTES", "DISCO", "DESCRIPCIÓN")
	for _, s := range resultados {
		variantes := strings.Join(s.Variants, ", ")
		if variantes == "" {
			variantes = "-"
		}
		desc := s.Description
		if desc == "" {
			desc = s.Name
		}
		disco := "-"
		if s.SizeGB > 0 {
			disco = fmt.Sprintf("%.1f GB", s.SizeGB)
		}
		fmt.Printf("  %-10s %-26s %-8s %s\n", s.ID, recortar(variantes, 26), disco, desc)
	}
	return 0
}
