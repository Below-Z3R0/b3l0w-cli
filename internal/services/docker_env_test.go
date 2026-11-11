package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpPasaElEnvImage verifica que el env con IMAGE llega al comando
// de docker.
//
// Si el env se pierde, compose levanta la imagen de su default
// (${IMAGE:-intel-latest}) en vez de la variante pedida: el servicio
// queda con la imagen vieja y el estado dice otra cosa.
func TestUpPasaElEnvImage(t *testing.T) {
	// Un stub de docker que escribe el env que recibio.
	dir := t.TempDir()
	stub := filepath.Join(dir, "docker")
	salida := filepath.Join(dir, "out")
	script := "#!/bin/sh\necho \"IMAGE=$IMAGE\" >> " + salida + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })

	d := &DockerAdapter{}
	err := d.Up("", []string{"compose.yaml"}, map[string]string{"IMAGE": "cuda-latest"})
	if err != nil {
		t.Fatalf("Up: %v", err)
	}

	out, err := os.ReadFile(salida)
	if err != nil {
		t.Fatalf("el stub no escribio: %v", err)
	}
	if !strings.Contains(string(out), "IMAGE=cuda-latest") {
		t.Errorf("el env no llego: %q", out)
	}
}
