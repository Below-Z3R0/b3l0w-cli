package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/internal/channel"
	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
	"github.com/Below-Z3R0/b3l0w-cli/internal/snapshots"
	"github.com/Below-Z3R0/b3l0w-cli/internal/update"
	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// Los comandos de actualizacion: el update general, el chequeo y el
// canal del escritorio.
func updateCommand(args []string) int {
	opts := update.Options{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "--dotfiles-dir=") {
			opts.DotfilesDir = strings.TrimPrefix(arg, "--dotfiles-dir=")
			continue
		}
		if strings.HasPrefix(arg, "--keep=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--keep=")); err == nil && n > 0 {
				opts.Keep = n
			}
			continue
		}
		switch arg {
		case "--system":
			opts.System = true
		case "--programs":
			opts.Programs = true
		case "--check":
			opts.Check = true
		case "--yes", "-y":
			opts.Yes = true
		case "--dotfiles":
			opts.Dotfiles = true
		case "--desktop":
			opts.Desktop = true
		case "--plugins":
			opts.Plugins = true
		case "--services":
			opts.Services = true
		case "--dry-run":
			opts.DryRun = true
		case "--force":
			opts.Force = true
		case "--adopt":
			opts.Adopt = true
		case "--self":
			opts.Self = true
		case "-h", "--help":
			fmt.Println("uso: b3l0w update [--self] [--system|--programs|--plugins|--services|--dotfiles] [--check] [--yes] [--dry-run] [--force]")
			return 0
		default:
			output.Fail(fmt.Sprintf("opción desconocida: %s", arg))
			return 1
		}
	}
	if opts.System && opts.Programs {
		output.Fail("--system y --programs son excluyentes; sin banderas se hacen las dos")
		return 1
	}
	return withManifest(func(m *config.Manifest) int { return update.Run(m, opts) })
}

func checkCommand(args []string) int {
	keep := 20
	yes := false
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--keep="):
			if n, err := strconv.Atoi(strings.TrimPrefix(arg, "--keep=")); err == nil && n > 0 {
				keep = n
			}
		case arg == "--yes" || arg == "-y":
			yes = true
		}
	}
	code := withManifest(func(m *config.Manifest) int {
		return update.Run(m, update.Options{Check: true})
	})
	if code != 0 {
		return code
	}
	return snapshots.Report(keep, yes)
}

func channelCommand(args []string) int {
	if len(args) == 0 {
		fmt.Printf("canal: %s\n", channel.Current())
		return 0
	}
	if err := channel.Set(args[0]); err != nil {
		output.Fail(err.Error())
		return 1
	}
	output.Ok("canal: " + args[0])
	return 0
}
