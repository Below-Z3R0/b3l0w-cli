// b3l0w-ytmusic es la extensión de YouTube Music de b3l0w: innertube (home,
// charts, search, radio) y reproducción real con mpv. El plugin de Noctalia la
// invoca como `b3l0w ytmusic ...`.
package main

import (
	"os"

	ytmusiccli "github.com/Below-Z3R0/b3l0w-cli/internal/plugins/ytmusic/cli"
)

func main() { os.Exit(ytmusiccli.Run(os.Args[1:])) }
