// Package snapshots cuenta y poda las entradas de Limine.
//
// Limine no guarda snapshots de snapper: guarda entradas de arranque
// que apuntan a ellas. `limine-snapper-info` dice cuántas hay y
// `limine-snapper-remove` borra las que sobran.
package snapshots

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Below-Z3R0/b3l0w-cli/kit/system"
)

// ansiRe matchea las secuencias de escape ANSI (CSI).
//
// limine-snapper-info colorea la salida cuando hereda un TERM con
// color, o sea en cualquier terminal interactivo: el label queda
// "\x1b[1;33mSnapshot entries  : \x1b[0m0 (max 0)" y el parser no lo
// encuentra. Se limpian antes de parsear.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// Entries lee "Snapshot entries : N" de limine-snapper-info.
func Entries() (int, error) {
	out, err := system.Run("limine-snapper-info")
	if err != nil {
		return 0, err
	}
	return parseEntries(out)
}

// parseEntries busca la línea "Snapshot entries : N" entre las muchas
// líneas "clave : valor" que imprime limine-snapper-info. Después del
// dos puntos viene "39 (max: auto)": el número es el primer campo.
//
// La salida puede venir coloreada con ANSI: se limpia antes de parsear.
func parseEntries(out string) (int, error) {
	for _, line := range strings.Split(ansiRe.ReplaceAllString(out, ""), "\n") {
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
		if err != nil || n < 0 {
			continue // valor no numérico o conteo negativo: no es un conteo
		}
		return n, nil
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

// parseIDs saca la primera columna de la tabla de limine-snapper-list.
// Las filas que no son numéricas (encabezado, separador) se saltean.
// Los IDs inválidos (0 o negativos) y los repetidos se descartan: un ID
// basura tumba el lote de limine-snapper-remove y uno repetido hace
// contar dos veces la misma entrada.
func parseIDs(out string) []int {
	var ids []int
	seen := make(map[int]bool)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil || id <= 0 {
			continue // encabezado, separador o ID inválido
		}
		if seen[id] {
			continue // ID repetido
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// Prune borra las entradas más viejas y conserva las `keep` últimas.
//
// keep es parte de la API pública: un keep negativo haría
// ids[:len(ids)-keep], que mete un ID 0 inventado en el lote o entra
// en panic. Se rechaza antes de tocar nada.
func Prune(keep int) (int, error) {
	if keep < 0 {
		return 0, fmt.Errorf("keep inválido: %d", keep)
	}
	ids, err := IDs()
	if err != nil {
		return 0, err
	}
	if len(ids) <= keep {
		return 0, nil
	}
	old := ids[:len(ids)-keep]
	// Un solo remove con todos los IDs: la tool acepta varios y así
	// reescribe limine.conf una vez sola.
	args := []string{"limine-snapper-remove"}
	for _, id := range old {
		args = append(args, strconv.Itoa(id))
	}
	if out, err := system.RunCombined("sudo", args...); err != nil {
		return 0, fmt.Errorf("%v: %s", err, out)
	}
	// remove deja limine.conf desactualizado: sync regenera lo que queda.
	if out, err := system.RunCombined("sudo", "limine-snapper-sync"); err != nil {
		return 0, fmt.Errorf("sync: %v: %s", err, out)
	}
	return len(old), nil
}
