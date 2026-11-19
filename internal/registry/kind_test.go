package registry

import (
	"strings"
	"testing"
)

// TestLoadRechazaUnaCategoriaDesconocida fija que un Kind inválido es un
// error del llamador, no un "no hay red": antes caía hasta el embebido y
// devolvía el catálogo de otra categoría con err nil.
func TestLoadRechazaUnaCategoriaDesconocida(t *testing.T) {
	datos, origen, err := Load(Kind("no-existe"), []byte("[]"))
	if err == nil {
		t.Fatalf("Load aceptó una categoría desconocida: origen=%v datos=%q", origen, datos)
	}
	if !strings.Contains(err.Error(), "categoría desconocida") {
		t.Errorf("error = %q, tenía que decir que la categoría no existe", err)
	}
	if datos != nil {
		t.Errorf("datos = %q, tenía que ser nil", datos)
	}
}

// TestLoadConCategoriaValidaSigueFuncionando es el contraste.
func TestLoadConCategoriaValidaSigueFuncionando(t *testing.T) {
	_, _, err := Load(System, []byte("[]"))
	if err != nil && strings.Contains(err.Error(), "categoría desconocida") {
		t.Fatalf("una categoría válida no puede caer en el rechazo: %v", err)
	}
}
