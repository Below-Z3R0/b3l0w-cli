package snapshots

import (
	"strings"
	"testing"
)

// Salidas REALES capturadas de /usr/sbin/limine-snapper-info en esta
// máquina (limine-snapper-sync 1.3.0, Limine 1.32.1).
//
// Son las dos variantes que importan: con TERM=dumb la tool no colorea,
// con TERM=xterm-256color colorea cada label con ANSI.
//
// Los casos marcados con [BUG] documentan bugs REALES del módulo: fallan
// hoy a propósito y describen el comportamiento esperado. Cada uno está
// detallado en tests.md.
const (
	realInfoPlain = "\nVersion           : 1.32.1\n" +
		"Manifest version  : 1.3.0\n" +
		"FS UUID           : 767190c6-cd83-49dc-8c5b-59a2fd068684\n" +
		"Last snapshot     : ID: 558, date: 2026-11-07 02:55:53\n" +
		"Excluded types    : [pre, post]\n" +
		"Snapshot entries  : 0 (max 0)\n" +
		"Boot usage        : 35.95% of 1.03 GiB (max 85.0%)\n" +
		"Verifying files ...\r\x1b[KSnapshot files    : 0 (0.0 MiB)\n" +
		"Unused files      : 0\n" +
		"Missing files     : 0\n" +
		"Corrupted files   : 0\n\n"

	realInfoColor = "\n\x1b[1;32mVersion           : \x1b[0m1.32.1\n" +
		"\x1b[1;32mManifest version  : \x1b[0m1.3.0\n" +
		"\x1b[1;32mFS UUID           : \x1b[0m767190c6-cd83-49dc-8c5b-59a2fd068684\n" +
		"\x1b[1;32mLast snapshot     : \x1b[0mID: 558, date: 2026-11-07 02:55:53\n" +
		"\x1b[1;32mExcluded types    : \x1b[0m[pre, post]\n" +
		"\x1b[1;33mSnapshot entries  : \x1b[0m0 (max 0)\n" +
		"\x1b[1;32mBoot usage        : \x1b[0m35.95% of 1.03 GiB (max 85.0%)\n" +
		"\x1b[0;33mVerifying files ...\x1b[0m\r\x1b[K\x1b[1;32mSnapshot files    : \x1b[0m0 (0.0 MiB)\n" +
		"\x1b[1;32mUnused files      : \x1b[0m0\n" +
		"\x1b[1;32mMissing files     : \x1b[0m0\n" +
		"\x1b[1;32mCorrupted files   : \x1b[0m0\n\n"

	// Salida real de /usr/sbin/limine-snapper-list (tabla vacía).
	realListEmpty = "\n ID │ Date │ Description \n────┼──────┼─────────────\n"

	// Filas reales de la tabla (mismo formato, con entradas).
	realListRows = "\n ID │ Date │ Description \n" +
		"────┼──────┼─────────────\n" +
		" 350 │ 2026-10-19 19:53:23 │ seanime-bin\n" +
		" 453 │ 2026-11-03 01:39:10 │ \n" +
		" 454 │ 2026-11-03 01:39:27 │ manual\n"
)

// ---------------------------------------------------------------------------
// parseEntries
// ---------------------------------------------------------------------------

func TestParseEntries_Tabla(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		// --- salida real ---
		{"real sin color", realInfoPlain, 0, false},
		{"real con color", realInfoColor, 0, false}, // [BUG] falla: ver tests.md
		{"solo la linea de conteo", "Snapshot entries  : 39 (max: auto)", 39, false},
		{"conteo cero", "Snapshot entries  : 0 (max 0)", 0, false},

		// --- separadores y espacios ---
		{"tabuladores", "Snapshot entries\t:\t39", 39, false},
		{"sin espacio antes de dos puntos", "Snapshot entries: 39", 39, false},
		{"sin espacio despues de dos puntos", "Snapshot entries  :39", 39, false},
		{"espacios alrededor", "   Snapshot entries   :   39   ", 39, false},
		{"crlf", "Snapshot entries  : 39\r", 39, false},
		{"crlf en medio", "Snapshot entries  : 39\r\nBoot usage : 1%", 39, false},
		{"lineas en blanco alrededor", "\n\nSnapshot entries  : 39\n\n", 39, false},

		// --- sufijos que aparecen en la salida real ---
		{"sufijo max", "Snapshot entries  : 39 (max: auto)", 39, false},
		{"sufijo max sin dos puntos", "Snapshot entries  : 39 (max 0)", 39, false},
		{"sufijo max auto", "Snapshot entries  : 39 (max auto)", 39, false},

		// --- errores: la tool no deberia producir esto ---
		{"vacio", "", 0, true},
		{"solo espacios", "   \n\t\n  ", 0, true},
		{"sin la linea", "Version : 1.32.1\nBoot usage : 30%", 0, true},
		{"solo encabezado sin dos puntos", "Snapshot entries", 0, true},
		{"encabezado con dos puntos sin valor", "Snapshot entries  : ", 0, true},
		{"valor no numerico", "Snapshot entries  : abc", 0, true},
		{"valor decimal", "Snapshot entries  : 39.5", 0, true},
		{"valor gigante", "Snapshot entries  : 99999999999999999999", 0, true},
		{"valor mayor que int64", "Snapshot entries  : 9223372036854775808", 0, true},
		{"digitos unicode", "Snapshot entries  : ٣٩", 0, true},
		{"otra clave", "Snapshot files    : 0 (0.0 MiB)", 0, true},

		// Prefijo laxo: "Snapshot entries" matchea como prefijo cualquier
		// clave que empiece así. Hoy la tool no emite otra clave con ese
		// prefijo, así que no es un bug activo: es una fragilidad
		// documentada (ver tests.md, "fragilidad").
		{"clave con el mismo prefijo", "Snapshot entriesTotal : 39", 39, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseEntries(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("esperaba error, obtuve n=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("esperaba n=%d, obtuve error: %v", c.want, err)
			}
			if got != c.want {
				t.Fatalf("esperaba n=%d, obtuve %d", c.want, got)
			}
		})
	}
}

func TestParseEntries_SalidaRealConColor(t *testing.T) {
	// [BUG] La tool colorea la salida cuando el proceso hereda un TERM con
	// color (o sea, en cualquier terminal interactivo). El label queda
	// "\x1b[1;33mSnapshot entries  : \x1b[0m0 (max 0)", así que el
	// HasPrefix("Snapshot entries") falla y el conteo nunca se lee.
	//
	// Impacto: en una terminal normal Entries() siempre falla, Report()
	// avisa "no pude leer 'Snapshot entries'" y NUNCA poda. El módulo
	// queda dead en su caso de uso principal.
	//
	// Fix esperado:.strip ANSI (o buscar el label sin depender del
	// prefijo exacto) antes de parsear.
	n, err := parseEntries(realInfoColor)
	if err != nil {
		t.Fatalf("[BUG] la salida real con color debería parsear: %v", err)
	}
	if n != 0 {
		t.Fatalf("[BUG] esperaba 0 entradas, obtuve %d", n)
	}
}

func TestParseEntries_ConteoNegativo(t *testing.T) {
	// [BUG] Un conteo negativo no puede ser real: parseEntries lo acepta
	// y lo devuelve sin error. Con -5, Report() compara -5 <= keep y
	// dice "snapshots de Limine: -5" creyendo que no hay entradas.
	//
	// Fix esperado: un conteo negativo es un error de parseo.
	if _, err := parseEntries("Snapshot entries  : -5"); err == nil {
		t.Fatal("[BUG] un conteo negativo debería ser error, no un número válido")
	}
}

func TestParseEntries_PrimeraLineaGana(t *testing.T) {
	// Comportamiento actual (documentado): gana la primera línea que
	// matchea el prefijo y tiene un entero parseable.
	n, err := parseEntries("Snapshot entries  : 1\nSnapshot entries  : 2")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("esperaba 1 (primera línea), obtuve %d", n)
	}
}

func TestParseEntries_NoMatcheaConsecuencia(t *testing.T) {
	// Fragilidad, no bug activo: el prefijo "Snapshot entries" matchea
	// cualquier clave que empiece con ese texto. Hoy la tool solo emite
	// "Snapshot entries" y "Snapshot files", así que no hay conflicto.
	// Si someday aparece "Snapshot entries max : 40", el parser va a leer
	// 40 como si fuera el total de entradas.
	n, err := parseEntries("Snapshot entries max : 40")
	if err != nil {
		t.Fatalf("esperaba n=40, obtuve error: %v", err)
	}
	if n != 40 {
		t.Fatalf("esperaba n=40, obtuve %d", n)
	}
}

// ---------------------------------------------------------------------------
// parseIDs
// ---------------------------------------------------------------------------

func TestParseIDs_Tabla(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int
	}{
		// --- salida real ---
		{"real vacia", realListEmpty, nil},
		{"real con filas", realListRows, []int{350, 453, 454}},

		// --- encabezado y separador ---
		{"solo encabezado", " ID │ Date │ Description ", nil},
		{"encabezado y separador", " ID │ Date │ Description \n────┼──────┼─────────────", nil},
		{"separador con espacios", "─────┼─────────────────────┼────────────────────────", nil},

		// --- formato de filas ---
		{"tabla del repo", "ID  │ Date                │ Description\n" +
			"─────┼─────────────────────┼────────────────────────\n" +
			" 350 │ 2026-10-19 19:53:23 │ seanime-bin\n" +
			" 453 │ 2026-11-03 01:39:10 │\n" +
			" 454 │ 2026-11-03 01:39:27 │ manual\n", []int{350, 453, 454}},
		{"tabuladores", "350\t2026-10-19\tx", []int{350}},
		{"crlf", " 350 │ 2026-10-19 19:53:23 │ x\r", []int{350}},
		{"descripcion vacia", " 350 │ 2026-10-19 19:53:23 │", []int{350}},
		{"descripcion numerica", " 350 │ 2026-10-19 19:53:23 │ 99", []int{350}},
		{"ids desordenados", " 454 │ a │ b\n 350 │ c │ d", []int{350, 454}},
		{"lineas en blanco", "\n\n 350 │ a │ b\n\n", []int{350}},

		// --- filas que NO son entradas ---
		{"vacio", "", nil},
		{"solo espacios", "   \n\t\n", nil},
		{"un solo campo", " 350", nil},
		{"sin espacio tras la barra", " 350│2026-10-19│x", nil},
		{"id no numerico", " abc │ 2026-10-19 │ x", nil},
		{"id decimal", " 35.0 │ 2026-10-19 │ x", nil},
		{"id gigante", " 99999999999999999999 │ x │ y", nil},
		{"digitos unicode", " ٣٥٠ │ 2026-10-19 │ x", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseIDs(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("esperaba %v, obtuve %v", c.want, got)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("esperaba %v, obtuve %v", c.want, got)
				}
			}
		})
	}
}

func TestParseIDs_IDNegativo(t *testing.T) {
	// [BUG] Un ID negativo no es un ID de entrada de Limine. parseIDs lo
	// acepta y lo mete en la lista.
	//
	// Impacto: Prune arma `sudo limine-snapper-remove ... -5 ...` y la
	// tool aborta el lote entero (README: "Si un ID no existe, el
	// comando puede fallar entero y no se borra nada"). Un solo ID
	// basura tumba la poda completa.
	//
	// Fix esperado: descartar IDs <= 0 como se descartan las filas no
	// numéricas.
	got := parseIDs(" -5 │ 2026-10-19 19:53:23 │ x")
	if len(got) != 0 {
		t.Fatalf("[BUG] un ID negativo debería descartarse, obtuve %v", got)
	}
}

func TestParseIDs_Duplicados(t *testing.T) {
	// [BUG] IDs repetidos se conservan duplicados.
	//
	// Impacto: Prune pasa el mismo ID dos veces a limine-snapper-remove
	// y cuenta las entradas borradas dos veces. El lote puede fallar
	// entero y el conteo que muestra Report() miente.
	//
	// Fix esperado: deduplicar.
	got := parseIDs(" 350 │ a │ b\n 350 │ c │ d")
	if len(got) != 1 || got[0] != 350 {
		t.Fatalf("[BUG] esperaba [350], obtuve %v", got)
	}
}

func TestParseIDs_Ordenado(t *testing.T) {
	// parseIDs ordena de menor a mayor: el módulo usa "ID más bajo =
	// entrada más vieja" para decidir qué borrar.
	got := parseIDs(" 454 │ a │ b\n 350 │ c │ d\n 453 │ e │ f")
	want := []int{350, 453, 454}
	if len(got) != len(want) {
		t.Fatalf("esperaba %v, obtuve %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("esperaba %v, obtuve %v", want, got)
		}
	}
}

func TestParseIDs_NoTocaLasFilasVecinas(t *testing.T) {
	// La fila de la cabecera no debe contaminar el parseo aunque tenga
	// dos o más campos.
	got := parseIDs(" ID │ Date │ Description \n 350 │ a │ b")
	if len(got) != 1 || got[0] != 350 {
		t.Fatalf("esperaba [350], obtuve %v", got)
	}
}

// ---------------------------------------------------------------------------
// Relación entre los dos parsers
// ---------------------------------------------------------------------------

func TestParseEntriesYParseIDs_MismaSalidaReal(t *testing.T) {
	// Los dos parsers se alimentan de comandos distintos pero del mismo
	// paquete. Con la salida real sin color los dos tienen que andar.
	n, err := parseEntries(realInfoPlain)
	if err != nil {
		t.Fatalf("parseEntries con la salida real: %v", err)
	}
	if n != 0 {
		t.Fatalf("esperaba 0 entradas, obtuve %d", n)
	}
	if got := parseIDs(realListRows); len(got) != 3 {
		t.Fatalf("esperaba 3 IDs, obtuve %v", got)
	}
}

func TestParseIDs_NoDevuelveLaFilaDeBoot(t *testing.T) {
	// Regresión: si algún día parseIDs se pasa la salida de
	// limine-snapper-info en vez de -list, las líneas "Last snapshot :
	// ID: 558, ..." podrían colarse como IDs.
	out := realInfoPlain
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Last snapshot") {
			if got := parseIDs(line); len(got) != 0 {
				t.Fatalf("la línea %q no debería dar IDs, obtuve %v", line, got)
			}
		}
	}
}
