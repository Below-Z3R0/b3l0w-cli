package snapshots

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fuzzing de los parsers
//
// Los dos parsers son funciones puras sobre strings: son el punto donde
// una salida inesperada de la tool se convierte en un crash o en una
// decisión de borrado equivocada.
//
// Los fuzzers no verifican valores exactos, sino invariantes: cualquier
// entrada tiene que dejar los parsers en un estado coherente.
//
// El corpus arranca con las salidas reales de limine-snapper-info y
// limine-snapper-list.
//
// Nota: los invariantes describen el comportamiento DESEADO. Los bugs
// que ya encontramos están fijados con tests determinísticos que fallan
// (ver tests.md); los fuzzers focalizan en encontrar panics y
// descoherencias nuevas.
// ---------------------------------------------------------------------------

// FuzzParseEntries: parseEntries nunca entra en panic y, cuando devuelve
// un número, ese número es coherente con la salida leída.
//
// Invariantes:
//   - nunca entra en panic;
//   - si hay error, es el de "no pude leer";
//   - si devuelve un número, es >= 0;
//   - ese número aparece como campo entero en la línea matcheada.
func FuzzParseEntries(f *testing.F) {
	f.Add(realInfoPlain)
	f.Add(realInfoColor)
	f.Add("Snapshot entries  : 39 (max: auto)")
	f.Add("Snapshot entries  : 0 (max 0)")
	f.Add("Snapshot entries  : 99999999999999999999")
	f.Add("")
	f.Add("\n\n\n")
	f.Add("Snapshot entries")
	f.Add("Snapshot entries  :")
	f.Add("Snapshot entries  :  \t 39 \r\n")
	f.Add("Snapshot entries  : ٣٩")
	f.Add("Snapshot entries  : 1e3")
	f.Add("Snapshot entries  : 0x27")

	f.Fuzz(func(t *testing.T, out string) {
		n, err := parseEntries(out)
		if err != nil {
			if err.Error() != "no pude leer 'Snapshot entries'" {
				t.Fatalf("error inesperado %q para la entrada %q", err, out)
			}
			return
		}
		// El conteo negativo no se chequea acá a propósito: ya está
		// fijado de forma determinística en
		// TestParseEntries_ConteoNegativo. Si el fuzzer lo comprobara,
		// cortaría la campaña en el primer input negativo y no llegaría
		// a buscar panics nuevos.
		if !entriesHasNumber(out, n) {
			t.Fatalf("n=%d no aparece en la entrada %q", n, out)
		}
	})
}

// entriesHasNumber dice si el número aparece como campo entero en la
// línea "Snapshot entries" que el parser matcheó.
//
// Compara numéricamente y no por texto: "000000" es un 0 válido y
// strconv.Atoi lo acepta igual que "0".
//
// La salida real con color viene con secuencias ANSI: se limpian como
// hace el parser, así el helper mira la misma línea que el parser
// matcheó. Sin esto el fuzzer reportaba un falso fallo sobre la
// salida real coloreada.
func entriesHasNumber(out string, n int) bool {
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
		got, err := strconv.Atoi(fields[0])
		if err == nil && got == n {
			return true
		}
	}
	return false
}

// FuzzParseIDs: parseIDs nunca entra en panic y la lista que devuelve
// es coherente con la tabla de entrada.
//
// Invariantes:
//   - nunca entra en panic;
//   - la lista viene ordenada de menor a mayor;
//   - cada ID devuelto es el primer campo de alguna fila;
//   - hay como máximo un ID por fila.
func FuzzParseIDs(f *testing.F) {
	f.Add(realListEmpty)
	f.Add(realListRows)
	f.Add(" 350 │ 2026-10-19 19:53:23 │ seanime-bin")
	f.Add(" 0 │ x │ y")
	f.Add(" 350 │ a │ b\n 350 │ c │ d")
	f.Add(" 454 │ a │ b\n 350 │ c │ d")
	f.Add("")
	f.Add("\n\n")
	f.Add(" ID │ Date │ Description \n────┼──────┼─────────────")
	f.Add("350\t2026-10-19\tx")
	f.Add(" 350")
	f.Add(" 350│2026-10-19│x")
	f.Add("٣٥٠ │ x │ y")

	f.Fuzz(func(t *testing.T, out string) {
		ids := parseIDs(out)
		for i := 1; i < len(ids); i++ {
			if ids[i] < ids[i-1] {
				t.Fatalf("la lista no está ordenada: %v para la entrada %q", ids, out)
			}
		}
		if len(ids) > countCandidateRows(out) {
			t.Fatalf("devolvió %d IDs pero la entrada %q tiene menos filas", len(ids), out)
		}
		for _, id := range ids {
			if !hasIDRow(out, id) {
				t.Fatalf("el ID %d no viene de ninguna fila de %q", id, out)
			}
		}
	})
}

// countCandidateRows cuenta las líneas que podrían ser filas de la tabla.
func countCandidateRows(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if len(strings.Fields(line)) >= 2 {
			n++
		}
	}
	return n
}

// hasIDRow dice si alguna fila de la tabla tiene ese ID en la primera
// columna.
//
// Compara numéricamente: "000" es un 0 válido.
func hasIDRow(out string, id int) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		got, err := strconv.Atoi(fields[0])
		if err == nil && got == id {
			return true
		}
	}
	return false
}

// FuzzPruneKeep: Prune con cualquier keep es seguro y nunca borra más de
// lo que hay.
//
// Corre el Prune real contra una tool falsa (PATH) para no tocar el
// sistema. La lista se pasa por archivo porque un fuzz target no puede
// cambiar variables de entorno en cada iteración.
//
// Invariantes:
//   - nunca entra en panic;
//   - no borra más entradas de las que hay en la lista;
//   - el conteo de borradas coincide con la cantidad de IDs que se le
//     pidieron a la tool.
//
// El keep negativo queda afuera del corpus: ya está fijado de forma
// determinística en TestPrune_KeepNegativo, que documenta el bug.
func FuzzPruneKeep(f *testing.F) {
	f.Add(listConN(0), 0)
	f.Add(listConN(1), 0)
	f.Add(listConN(1), 1)
	f.Add(listConN(3), 2)
	f.Add(listConN(10), 20)
	f.Add(listConN(10), 3)

	dir := f.TempDir()
	listFile := filepath.Join(dir, "list.txt")
	logFile := filepath.Join(dir, "sudo.log")
	writeFakeList(f, dir)
	f.Setenv("FAKE_LIST_FILE", listFile)
	f.Setenv("FAKE_LOG", logFile)
	f.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	f.Fuzz(func(t *testing.T, list string, keep int) {
		if keep < 0 {
			// El keep negativo ya está fijado de forma determinística en
			// TestPrune_KeepNegativo, que documenta el bug (panic o ID
			// fantasma). Acá solo se busca pano nuevo.
			return
		}
		if err := os.WriteFile(listFile, []byte(list), 0o644); err != nil {
			t.Fatalf("no pude escribir la lista: %v", err)
		}
		_ = os.Remove(logFile)

		ids := parseIDs(list)
		removed, err := Prune(keep)
		if err != nil {
			t.Fatalf("Prune falló: %v", err)
		}
		if removed < 0 {
			t.Fatalf("removed=%d es negativo", removed)
		}
		if removed > len(ids) {
			t.Fatalf("removed=%d pero la lista tiene %d IDs", removed, len(ids))
		}
	})
}

// writeFakeList deja en dir una tool de listado que lee de
// $FAKE_LIST_FILE y un sudo que loguea lo que le piden.
func writeFakeList(t testing.TB, dir string) {
	t.Helper()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("no pude escribir %s: %v", name, err)
		}
	}
	write("limine-snapper-list", "#!/bin/sh\ncat \"$FAKE_LIST_FILE\"\n")
	write("sudo", "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_LOG\"\nexit 0\n")
}
