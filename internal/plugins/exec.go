package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecScript ejecuta un backend embebido con bash, heredando la
// terminal. Así el plugin llama al CLI y no hace falta copiar el
// script a /usr/local/bin.
func ExecScript(name string, args []string) error {
	if !strings.HasSuffix(name, ".sh") {
		name += ".sh"
	}
	data, err := scriptsFS.ReadFile("scripts/" + name)
	if err != nil {
		return fmt.Errorf("script %s no existe", name)
	}
	cmdArgs := append([]string{"-c", string(data), name}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
