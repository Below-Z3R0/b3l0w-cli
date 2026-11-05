package snapshots

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w-cli/internal/cli"
)

// Report cuenta las entradas, pregunta si reducir y con qué límite.
func Report(keep int, yes bool) int {
	if keep <= 0 {
		keep = 20
	}
	total, err := Entries()
	if err != nil {
		cli.Warn("snapshots de Limine: " + err.Error())
		return 0
	}
	cli.Ok(fmt.Sprintf("snapshots de Limine: %d", total))
	if total <= keep {
		return 0
	}

	if !yes {
		if !cli.Confirm(fmt.Sprintf("¿Reducir de %d a %d? [Y/n] (c cancela) ", total, keep), false, true) {
			cli.Warn("cancelled")
			return 0
		}
		keep = cli.AskInt(fmt.Sprintf("¿Cuántas conservar? [%d] (c cancela) ", keep), keep)
		if keep <= 0 {
			cli.Warn("cancelled")
			return 0
		}
	}

	removed, err := Prune(keep)
	if err != nil {
		cli.Fail("limpiar snapshots: " + err.Error())
		return 1
	}
	cli.Ok(fmt.Sprintf("%d entradas borradas; quedan %d", removed, keep))
	return 0
}
