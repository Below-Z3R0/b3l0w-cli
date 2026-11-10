// Package releases consulta el estado de firma de los releases de los
// programas.
//
// Existe para que "acordarse de firmar" no dependa de la memoria de
// nadie: `doctor` y `update --check` lo verifican y fallan si falta.
package releases

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/selfupdate"

	"github.com/Below-Z3R0/b3l0w-kit/output"
	"github.com/Below-Z3R0/b3l0w-kit/releases"
)

// Inyectables para los tests.
var (
	client = &http.Client{Timeout: 8 * time.Second}
	// Un 404 del CDN justo después de publicar es transitorio: sin
	// reintento, doctor marca SIN FIRMA un release que sí está firmado.
	// El CLI no está en el manifiesto y su release se busca con la
	// convención v*, no con /releases/latest.
	cliTag  = selfupdate.Check
	cliRepo = selfupdate.Repository
)

// Status es el estado de firma del último release de un programa.
type Status struct {
	Name   string
	Repo   string
	Tag    string
	Signed bool
	Err    string
}

// Check revisa los programas del manifiesto y el propio CLI.
//
// El CLI se chequea aparte porque no está en el manifiesto: si se
// publica una versión sin firma, doctor tiene que verlo antes de que
// update --self falle.
func Check(m *config.Manifest) []Status {
	out := make([]Status, 0, len(m.Programs)+1)
	for _, p := range m.Programs {
		repo, ok := githubRepo(p.Source)
		if !ok {
			continue
		}
		st := Status{Name: p.Name, Repo: repo}
		tag, err := releases.LatestTag(repo)
		if err != nil {
			st.Err = err.Error()
			out = append(out, st)
			continue
		}
		st.Tag = tag
		signed, err := releases.HasSignature(repo, tag)
		if err != nil {
			st.Err = err.Error()
		} else {
			st.Signed = signed
		}
		out = append(out, st)
	}
	return append(out, checkCLI())
}

// checkCLI arma el estado del release del propio b3l0w.
func checkCLI() Status {
	repo := cliRepo()
	st := Status{Name: "b3l0w", Repo: repo}
	tag, err := cliTag()
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Tag = tag
	signed, err := releases.HasSignature(repo, tag)
	if err != nil {
		st.Err = err.Error()
		return st
	}
	st.Signed = signed
	return st
}

// Render imprime el estado y devuelve cuántos releases están sin firma.
func Render(statuses []Status) int {
	if len(statuses) == 0 {
		return 0
	}
	output.Step("Firmas de releases")
	errores := 0
	for _, st := range statuses {
		switch {
		case st.Err != "":
			output.Warn(fmt.Sprintf("%s: no se pudo consultar (%s)", st.Name, st.Err))
		case st.Signed:
			output.Ok(fmt.Sprintf("%s: firmado (%s)", st.Name, st.Tag))
		default:
			output.Fail(fmt.Sprintf("%s: SIN FIRMA (%s)", st.Name, st.Tag))
			fmt.Printf("      bash scripts/maintainers/sign-release.sh %s %s\n", st.Repo, st.Tag)
			errores++
		}
	}
	return errores
}

// githubRepo saca "owner/repo" de "github:owner/repo".
func githubRepo(source string) (string, bool) {
	if !strings.HasPrefix(source, "github:") {
		return "", false
	}
	repo := strings.TrimPrefix(source, "github:")
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", false
	}
	return repo, true
}
