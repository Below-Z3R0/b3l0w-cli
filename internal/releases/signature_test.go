package releases

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	kitreleases "github.com/Below-Z3R0/b3l0w-cli/kit/releases"
)

func TestCheckSoloProgramasDeGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/firmado/releases/latest":
			w.Write([]byte(`{"tag_name":"v1.0.0"}`))
		case r.URL.Path == "/repos/owner/sin-firma/releases/latest":
			w.Write([]byte(`{"tag_name":"v2.0.0"}`))
		case r.URL.Path == "/owner/firmado/releases/download/v1.0.0/SHA256SUMS.asc":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(kitreleases.SetBases(srv.URL, srv.URL))

	// El CLI propio no usa /releases/latest: se busca con la
	// convención v*, que inyectan cliTag y cliRepo.
	viejoTag, viejoRepo := cliTag, cliRepo
	cliTag = func() (string, error) { return "v1.0.0", nil }
	cliRepo = func() string { return "owner/cli" }
	t.Cleanup(func() { cliTag, cliRepo = viejoTag, viejoRepo })

	m := &config.Manifest{Programs: []config.Program{
		{Name: "firmado", Source: "github:owner/firmado"},
		{Name: "sin-firma", Source: "github:owner/sin-firma"},
		{Name: "local", Source: "file:///tmp"},
	}}

	got := Check(m)
	// 2 programas de GitHub + el propio CLI.
	if len(got) != 3 {
		t.Fatalf("Check devolvió %d, quería 3", len(got))
	}
	if !got[0].Signed {
		t.Errorf("%s debería estar firmado", got[0].Name)
	}
	if got[1].Signed {
		t.Errorf("%s no debería estar firmado", got[1].Name)
	}
	if got[1].Tag != "v2.0.0" {
		t.Errorf("tag = %q", got[1].Tag)
	}
	if got[2].Name != "b3l0w" || got[2].Tag != "v1.0.0" {
		t.Errorf("CLI = %+v, quería b3l0w v1.0.0", got[2])
	}
}

func TestGitHubRepo(t *testing.T) {
	casos := []struct {
		source string
		quiere string
		ok     bool
	}{
		{"github:owner/repo", "owner/repo", true},
		{"github:owner", "", false},
		{"github:/repo", "", false},
		{"file:///tmp", "", false},
	}
	for _, c := range casos {
		got, ok := githubRepo(c.source)
		if got != c.quiere || ok != c.ok {
			t.Errorf("githubRepo(%q) = (%q, %v), quería (%q, %v)", c.source, got, ok, c.quiere, c.ok)
		}
	}
}
