package releases

import (
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// TestGitHubRepoRechazaRutaExtra es el bug M1: `github:owner/repo/extra`
// pasaba la validación y la ruta extra se armaba en la URL de la API,
// que no existe. El error tiene que ser de manifiesto, no un 404.
func TestGitHubRepoRechazaRutaExtra(t *testing.T) {
	casos := []string{
		"github:owner/repo/extra",
		"github:owner/repo/sub/extra",
		"github:owner/repo/",
	}
	for _, source := range casos {
		got, ok := githubRepo(source)
		if ok {
			t.Errorf("githubRepo(%q) = (%q, true): la ruta extra tiene que rechazarse", source, got)
		}
		if got != "" {
			t.Errorf("githubRepo(%q) devolvió %q además del error", source, got)
		}
	}

	// El camino feliz sigue igual.
	got, ok := githubRepo("github:owner/repo")
	if !ok || got != "owner/repo" {
		t.Fatalf("githubRepo(github:owner/repo) = (%q, %v)", got, ok)
	}
}

// TestCheckUsaLasConsultasInyectadas es el bug M2: el paquete declaraba
// un cliente HTTP que nadie usaba, con un comentario que decía
// "Inyectables para los tests". El seam real es el que se usa: las
// consultas se inyectan y Check tiene que usar las inyectadas.
func TestCheckUsaLasConsultasInyectadas(t *testing.T) {
	viejoTag, viejoFirma := latestTag, hasSignature
	t.Cleanup(func() { latestTag, hasSignature = viejoTag, viejoFirma })

	latestTag = func(repo string) (string, error) { return "v9.9.9", nil }
	hasSignature = func(repo, tag string) (bool, error) { return true, nil }

	viejoCLITag, viejoCLIRepo := cliTag, cliRepo
	t.Cleanup(func() { cliTag, cliRepo = viejoCLITag, viejoCLIRepo })
	cliTag = func() (string, error) { return "v1.0.0", nil }
	cliRepo = func() string { return "owner/cli" }

	m := &config.Manifest{Programs: []config.Program{
		{Name: "prog", Source: "github:owner/repo"},
	}}

	got := Check(m)
	if len(got) != 2 {
		t.Fatalf("Check devolvió %d estados, quiero 2 (el programa y el CLI)", len(got))
	}
	if got[0].Tag != "v9.9.9" || !got[0].Signed {
		t.Errorf("Check no usó las consultas inyectadas: %+v", got[0])
	}
	if got[0].Err != "" {
		t.Errorf("Err = %q, quería vacío", got[0].Err)
	}
}
