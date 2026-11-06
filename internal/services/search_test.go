package services

import "testing"

func manifold() *Manifest {
	return &Manifest{
		Version: 1,
		Services: []Service{
			{ID: "argos", Name: "Argos", Description: "Búsqueda semántica", Variants: []string{"cpu", "cuda", "intel", "amd"}},
			{ID: "sonus", Name: "Sonus", Description: "Voz y transcripción", Variants: []string{"cpu", "intel"}},
			{ID: "seanime", Name: "Seanime", Description: "Servidor de anime", Variants: []string{"default"}},
		},
	}
}

func TestMatchPorID(t *testing.T) {
	got := Match(manifold(), "ARGOS")
	if len(got) != 1 || got[0].ID != "argos" {
		t.Fatalf("Match = %v, quería [argos]", ids(got))
	}
}

func TestMatchPorDescripcion(t *testing.T) {
	got := Match(manifold(), "voz")
	if len(got) != 1 || got[0].ID != "sonus" {
		t.Fatalf("Match = %v, quería [sonus]", ids(got))
	}
}

func TestMatchPorVariante(t *testing.T) {
	got := Match(manifold(), "cuda")
	if len(got) != 1 || got[0].ID != "argos" {
		t.Fatalf("Match = %v, quería [argos]", ids(got))
	}
}

func TestMatchVacioDevuelveTodo(t *testing.T) {
	if got := Match(manifold(), "  "); len(got) != 3 {
		t.Errorf("Match vacío = %d servicios, quería 3", len(got))
	}
}

func TestMatchSinResultados(t *testing.T) {
	if got := Match(manifold(), "noexiste"); len(got) != 0 {
		t.Errorf("Match = %v, quería vacío", ids(got))
	}
}

func TestMatchNoDistingueMayusculas(t *testing.T) {
	if got := Match(manifold(), "SeAnImE"); len(got) != 1 {
		t.Errorf("Match = %v, quería [seanime]", ids(got))
	}
}

func ids(ss []Service) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}
