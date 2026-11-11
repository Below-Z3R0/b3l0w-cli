package main

import (
	"reflect"
	"testing"
)

// TestParsearFlagsLasDosFormas verifica que un flag con valor se
// acepta con "=" y con espacio.
//
// Es lo natural y que una de las dos falle es una trampa: el flag se
// consume como si fuera el valor siguiente.
func TestParsearFlagsLasDosFormas(t *testing.T) {
	f := parsearFlags([]string{"--manifest", "/tmp/x.json", "argos"})
	if got := f.valor("--manifest"); got != "/tmp/x.json" {
		t.Errorf("valor = %q, quiero /tmp/x.json", got)
	}
	if got := f.libre(); got != "argos" {
		t.Errorf("libre = %q, quiero argos", got)
	}

	f = parsearFlags([]string{"--manifest=/tmp/x.json", "argos"})
	if got := f.valor("--manifest"); got != "/tmp/x.json" {
		t.Errorf("valor = %q, quiero /tmp/x.json", got)
	}
	if got := f.libre(); got != "argos" {
		t.Errorf("libre = %q, quiero argos", got)
	}
}

// TestParsearFlagsNoConsumeElSiguiente verifica que un flag
// booleano no se come el argumento siguiente.
//
// "--dry-run argos": argos es el id del servicio, no el valor del
// flag.
func TestParsearFlagsNoConsumeElSiguiente(t *testing.T) {
	f := parsearFlags([]string{"--dry-run", "argos"})
	if !f.presente("--dry-run") {
		t.Error("no vio --dry-run")
	}
	if got := f.libre(); got != "argos" {
		t.Errorf("libre = %q, quiero argos", got)
	}
}

// TestParsearFlagsReportaDesconocidos verifica que un flag que no
// existe queda anotado.
//
// Antes se ignoraba en silencio: el usuario creia que --dry-run hacia
// algo y no hacia nada.
func TestParsearFlagsReportaDesconocidos(t *testing.T) {
	f := parsearFlags([]string{"--bogus", "argos"})
	if len(f.desconocidos) != 1 || f.desconocidos[0] != "--bogus" {
		t.Errorf("desconocidos = %v, quiero [--bogus]", f.desconocidos)
	}
}

// TestParsearFlagsVariosLibres verifica el orden de los posicionales.
func TestParsearFlagsVariosLibres(t *testing.T) {
	f := parsearFlags([]string{"--variant=cuda", "argos", "extra"})
	want := []string{"argos", "extra"}
	if !reflect.DeepEqual(f.libres, want) {
		t.Errorf("libres = %v, quiero %v", f.libres, want)
	}
}

// TestParsearFlagsValorVacio verifica que un flag con valor y "="
// queda con el valor vacio, no con el siguiente argumento.
func TestParsearFlagsValorVacio(t *testing.T) {
	f := parsearFlags([]string{"--manifest=", "argos"})
	if got := f.valor("--manifest"); got != "" {
		t.Errorf("valor = %q, quiero vacio", got)
	}
	if got := f.libre(); got != "argos" {
		t.Errorf("libre = %q, quiero argos", got)
	}
}
