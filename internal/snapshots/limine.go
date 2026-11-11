// Package snapshots cuenta y poda las entradas de Limine.
//
// Limine no guarda snapshots de snapper: guarda entradas de arranque
// que apuntan a ellas. `limine-snapper-info` dice cuántas hay y
// `limine-snapper-remove` borra las que sobran.
package snapshots

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-kit/system"
)

// Entries lee "Snapshot entries : N" de limine-snapper-info.
//
// Con timeout: en esta maquina el comando tardo mas de 10 segundos y
// hacia que `b3l0w check` se quedara colgado casi 40.
func Entries() (int, error) {
	out, err := system.RunTimeout(5*time.Second, "limine-snapper-info")
	if err != nil {
		return 0, err
	}
	return parseEntries(out)
}

func parseEntries(out string) (int, error) {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Snapshot entries") {
			continue
		}
		_, after, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		fields := strings.Fields(after)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("no pude leer 'Snapshot entries'")
}

// IDs devuelve los IDs de limine-snapper-list, de viejo a nuevo.
func IDs() ([]int, error) {
	out, err := system.Run("limine-snapper-list")
	if err != nil {
		return nil, err
	}
	return parseIDs(out), nil
}

func parseIDs(out string) []int {
	var ids []int
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			continue // encabezado o separador
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// Prune borra las entradas más viejas y conserva las `keep` últimas.
func Prune(keep int) (int, error) {
	ids, err := IDs()
	if err != nil {
		return 0, err
	}
	if len(ids) <= keep {
		return 0, nil
	}
	old := ids[:len(ids)-keep]
	args := []string{"limine-snapper-remove"}
	for _, id := range old {
		args = append(args, strconv.Itoa(id))
	}
	if out, err := system.RunCombined("sudo", args...); err != nil {
		return 0, fmt.Errorf("%v: %s", err, out)
	}
	if out, err := system.RunCombined("sudo", "limine-snapper-sync"); err != nil {
		return 0, fmt.Errorf("sync: %v: %s", err, out)
	}
	return len(old), nil
}
