package releases

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

func usarServidor(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	viejoAPI, viejoDown := apiBase, downloadBase
	viejoTag, viejoRepo := cliTag, cliRepo
	apiBase, downloadBase = srv.URL, srv.URL
	cliTag = func() (string, error) { return "v1.0.0", nil }
	cliRepo = func() string { return "owner/cli" }
	t.Cleanup(func() {
		apiBase, downloadBase = viejoAPI, viejoDown
		cliTag, cliRepo = viejoTag, viejoRepo
	})
}

func TestLatestTag(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") != "b3l0w" {
			t.Error("falta User-Agent")
		}
		w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	})

	tag, err := LatestTag("owner/repo")
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if tag != "v1.2.3" {
		t.Errorf("tag = %q", tag)
	}
}

func TestLatestTagSinReleases(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := LatestTag("owner/repo"); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestHasSignatureSi(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("método = %s, quería HEAD", r.Method)
		}
		if r.URL.Path != "/owner/repo/releases/download/v1.0.0/SHA256SUMS.asc" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})

	ok, err := HasSignature("owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("HasSignature: %v", err)
	}
	if !ok {
		t.Error("esperaba firmado")
	}
}

func TestHasSignatureNo(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ok, err := HasSignature("owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("HasSignature: %v", err)
	}
	if ok {
		t.Error("esperaba sin firma")
	}
}

func TestCheckSoloProgramasDeGitHub(t *testing.T) {
	usarServidor(t, func(w http.ResponseWriter, r *http.Request) {
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
	})

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
