package main

import (
	"fmt"
	"sort"

	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
	"github.com/Below-Z3R0/b3l0w-kit/cli"
)

// systemList muestra la definición del sistema: los perfiles y cuántos
// paquetes, apps, programas y grupos declara cada uno.
func systemList() int {
	def, err := systemdef.Builtin()
	if err != nil {
		cli.Fail(err.Error())
		return 1
	}
	cli.Step("Definición del sistema")
	names := def.Names()
	sort.Strings(names)
	fmt.Printf("  %-10s %6s %5s %5s %5s %6s %s\n",
		"PERFIL", "PAQUETES", "AUR", "FLAT", "PROP", "GRUPOS", "ARCHIVOS")
	for _, name := range names {
		p := def.Profiles[name]
		fmt.Printf("  %-10s %6d %5d %5d %5d %6d %d\n",
			name, len(p.Repo), len(p.AUR), len(p.Flatpak), len(p.Own), len(p.Groups), len(p.Files))
	}
	fmt.Println()
	fmt.Println("  b3l0w install system --apply   aplica lo que falta")
	fmt.Println("  b3l0w system diff              compara con el remoto")
	return 0
}
