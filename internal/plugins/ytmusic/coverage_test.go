package ytmusic

// Test de Y19: la cobertura que promete el README tiene que ser la real.
//
// El README decía "83,7 %", que no era ni la del paquete (85,0 %) ni la de
// `cli/` (32,5 %). Este test mide la cobertura de verdad (en un subproceso,
// para no depender del número que imprime el `go test` padre) y la compara
// con la que afirma el README.

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// coberturaChild marca el subproceso: ahí el test se saltea a sí mismo para no
// recursar.
const coberturaChild = "YTMUSIC_COVERAGE_CHILD"

// TestElREADMEDeclaraLaCoberturaReal fija Y19: el README no puede prometer
// una cobertura que no es la real, ni una sola cifra ambigua para dos
// paquetes.
func TestElREADMEDeclaraLaCoberturaReal(t *testing.T) {
	if os.Getenv(coberturaChild) != "" {
		t.Skip("es el subproceso de medición")
	}
	real := medirCobertura(t)
	readme := leerReadme(t)
	declarada := coberturaDeclarada(readme)
	for paquete, valor := range real {
		want, ok := declarada[paquete]
		if !ok {
			t.Errorf("el README no declara la cobertura de %s: el número real es %.1f %%", paquete, valor)
			continue
		}
		if diff := valor - want; diff > 0.6 || diff < -0.6 {
			t.Errorf("el README dice %.1f %% de cobertura para %s, la real es %.1f %% (Y19)", want, paquete, valor)
		}
	}
}

// TestElREADMEDistingueLosDosPaquetes: el 83,7 % no correspondía ni al
// paquete ni a `cli/`. El README tiene que decir de qué paquete habla cada
// número.
func TestElREADMEDistingueLosDosPaquetes(t *testing.T) {
	readme := leerReadme(t)
	declarada := coberturaDeclarada(readme)
	if len(declarada) < 2 {
		t.Errorf("el README declara %d coberturas, quería una por paquete (Y19)", len(declarada))
	}
	if _, ok := declarada["ytmusic"]; !ok {
		t.Error("el README no dice la cobertura del paquete ytmusic (Y19)")
	}
	if _, ok := declarada["ytmusic/cli"]; !ok {
		t.Error("el README no dice la cobertura del paquete ytmusic/cli (Y19)")
	}
	if strings.Contains(readme, "83,7") {
		t.Error("el README todavía promete la cobertura vieja de 83,7 % (Y19)")
	}
}

// leerReadme lee el README del módulo.
func leerReadme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("leyendo README.md: %v", err)
	}
	return string(raw)
}

// patrónCobertura agarra "- Cobertura del paquete `ytmusic`: **87,6 %**.".
var patrónCobertura = regexp.MustCompile("(?i)cobertura del paquete `([^`]+)`:\\s*\\*\\*([0-9]+(?:[.,][0-9]+)?)\\s*%\\*\\*")

// coberturaDeclarada saca del README los números de cobertura por paquete.
func coberturaDeclarada(readme string) map[string]float64 {
	out := map[string]float64{}
	for _, coincidencia := range patrónCobertura.FindAllStringSubmatch(readme, -1) {
		valor, err := strconv.ParseFloat(strings.Replace(coincidencia[2], ",", ".", 1), 64)
		if err != nil {
			continue
		}
		out[coincidencia[1]] = valor
	}
	return out
}

// medirCobertura corre `go test -cover` sobre el módulo y saca el porcentaje
// real de cada paquete.
func medirCobertura(t *testing.T) map[string]float64 {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("sin toolchain de Go no se puede medir la cobertura")
	}
	cmd := exec.Command(goBin, "test", "-cover", "-count=1", "./...")
	cmd.Env = append(os.Environ(), coberturaChild+"=1")
	salida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("midiendo la cobertura: %v\n%s", err, salida)
	}
	out := map[string]float64{}
	marca := regexp.MustCompile(`coverage: ([0-9]+(?:[.,][0-9]+)?)%`)
	const modulo = "internal/plugins/ytmusic"
	for _, linea := range strings.Split(string(salida), "\n") {
		if !strings.Contains(linea, "coverage:") {
			continue
		}
		encontrado := marca.FindStringSubmatch(linea)
		if encontrado == nil {
			continue
		}
		valor, err := strconv.ParseFloat(strings.Replace(encontrado[1], ",", ".", 1), 64)
		if err != nil {
			continue
		}
		// La línea es: "ok  	<paquete>	<tiempo>	coverage: N% of statements".
		paquete := ""
		for _, campo := range strings.Split(linea, "	") {
			if indice := strings.Index(campo, modulo); indice >= 0 {
				paquete = campo[indice+len(modulo):]
				break
			}
		}
		if paquete == "" {
			paquete = "ytmusic"
		}
		if strings.HasPrefix(paquete, "/") {
			paquete = "ytmusic" + paquete
		}
		out[paquete] = valor
	}
	if len(out) == 0 {
		t.Fatalf("no se pudo leer la cobertura de la salida:\n%s", salida)
	}
	return out
}
