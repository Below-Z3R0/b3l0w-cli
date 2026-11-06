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
		return "search"
	}
	return ""
}

// aliasHelp lista las banderas cortas de una categoría.
func aliasHelp(category string) string {
	return "  -S install   -R remove   -Q list   -U update   -Ss search"
}
