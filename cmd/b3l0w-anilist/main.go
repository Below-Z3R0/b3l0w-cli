// b3l0w-anilist es la extensión de AniList de b3l0w: biblioteca, carátulas y
// progreso. El plugin de Noctalia la invoca como `b3l0w anilist ...`.
package main

import (
	"os"

	anilistcli "github.com/Below-Z3R0/b3l0w/internal/plugins/anilist/cli"
)

func main() { os.Exit(anilistcli.Run(os.Args[1:])) }
