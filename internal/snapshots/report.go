package snapshots

import (
	"fmt"

	"github.com/Below-Z3R0/b3l0w-cli/kit/confirm"
	"github.com/Below-Z3R0/b3l0w-cli/kit/output"
)

// Report cuenta las entradas, pregunta si reducir y con qué límite.
func Report(keep int, yes bool) int {
	// Límite por defecto cuando el llamador no pasa uno.
	if keep <= 0 {
		keep = 20
	}
	total, err := Entries()
	if err != nil {
		// No frena el update: la poda de snapshots es opcional.
		output.Warn("snapshots de Limine: " + err.Error())
		return 0
	}
	output.Ok(fmt.Sprintf("snapshots de Limine: %d", total))
	if total <= keep {
		return 0
	}

	if !yes {
		if !confirm.Confirm(fmt.Sprintf("¿Reducir de %d a %d? [Y/n] (c cancela) ", total, keep), false, true) {
			output.Warn("cancelled")
			return 0
		}
		keep = confirm.AskInt(fmt.Sprintf("¿Cuántas conservar? [%d] (c cancela) ", keep), keep)
		if keep <= 0 {
			// AskInt devuelve -1 si el usuario responde "c".
			output.Warn("cancelled")
			return 0
		}
	}

	removed, err := Prune(keep)
	if err != nil {
		output.Fail("limpiar snapshots: " + err.Error())
		return 1
	}
	// El conteo y la lista vienen de comandos distintos: si el conteo
	// decía que sobraban entradas y no se borró ninguna, la lista no
	// tenía todo. Se avisa en vez de dejar que el usuario crea que sí.
	if removed == 0 && total > keep {
		output.Warn(fmt.Sprintf("no se borró nada: el conteo dice %d pero la lista de entradas no las tiene todas", total))
	}
	// Las que quedan son las que había menos las borradas, no `keep`:
	// con un keep chico y una lista corta, `keep` mentía.
	output.Ok(fmt.Sprintf("%d entradas borradas; quedan %d", removed, total-removed))
	return 0
}
