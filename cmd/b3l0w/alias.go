package main

// aliasVerb traduce una bandera corta estilo pacman al verbo canónico
// de una categoría.
//
// Los verbos son la forma canónica y la que se documenta. Las banderas
// existen para que las categorías se sientan parecidas entre sí, no
// para reimplementar pacman: `-Syu` en services verifica y pregunta,
// no actualiza el sistema. Devuelve "" si la bandera no aplica a esa
// categoría, y el llamador falla con el uso.
func aliasVerb(category, arg string) string {
	switch arg {
	case "-S", "--install":
		return "install"
	case "-R", "-Rns", "--remove":
		return "remove"
	case "-Q", "-Qe", "--list":
		return "list"
	case "-U", "-Syu", "--update":
		return "update"
	case "-Ss", "-Qs":
		// Buscar solo tiene sentido donde hay catálogo que consultar.
		if category == "plugins" {
			return "search"
		}
		return ""
	}
	return ""
}

// aliasHelp lista las banderas cortas de una categoría.
func aliasHelp(category string) string {
	base := "  -S install   -R remove   -Q list   -U update"
	if category == "plugins" {
		return base + "   -Ss search"
	}
	return base
}
