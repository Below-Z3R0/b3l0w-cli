// b3l0w-docker es la extensión de contenedores de b3l0w.
package main

import (
	"os"

	dockercli "github.com/Below-Z3R0/b3l0w-cli/internal/plugins/docker/cli"
)

func main() { os.Exit(dockercli.Run(os.Args[1:])) }
