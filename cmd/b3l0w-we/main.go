// b3l0w-we es la extensión de wallpaper de b3l0w: convierte wallpapers de
// Wallpaper Engine a video y mantiene un manifiesto que el plugin de Noctalia
// usa para el modo "videos".
//
// Se invoca como `b3l0w we ...`: el core de b3l0w encuentra este binario en su
// directorio privado de extensiones y le pasa los argumentos tal cual.
package main

import (
	"os"

	"github.com/Below-Z3R0/b3l0w-cli/internal/plugins/we/cli"
)

func main() { os.Exit(wecli.Run(os.Args[1:])) }
