package snapshot

import (
	"strings"
	"testing"
)

// TestCreateRechazaUnLabelConSaltoDeLinea fija que el label no puede
// romper el listado de snapper.
func TestCreateRechazaUnLabelConSaltoDeLinea(t *testing.T) {
	err := Create("pre\ndotfiles")
	if err == nil {
		t.Fatal("Create aceptó un label con salto de línea")
	}
	if !strings.Contains(err.Error(), "saltos de línea") {
		t.Errorf("error = %q", err)
	}
}

// TestCreateRechazaUnLabelMuyLargo fija el tope: un label de 300
// caracteres no se lee en `snapper list`.
func TestCreateRechazaUnLabelMuyLargo(t *testing.T) {
	err := Create(strings.Repeat("x", 201))
	if err == nil {
		t.Fatal("Create aceptó un label de 201 caracteres")
	}
	if !strings.Contains(err.Error(), "demasiado largo") {
		t.Errorf("error = %q", err)
	}
}
