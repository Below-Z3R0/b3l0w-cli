// Package version guarda la versión del binario.
//
// Es una variable para que el build la inyecte con -ldflags:
//
//	go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v0.1.0"
//
// Si fuera una función, el linker no la tocaría y el binario diría "dev"
// para siempre.
package version

// Version la pisa el build. "dev" en una compilación local.
var Version = "dev"
