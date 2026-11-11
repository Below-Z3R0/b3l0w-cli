package snapshots

import (
	"github.com/Below-Z3R0/b3l0w-cli/kit/confirm"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// ---------------------------------------------------------------------------
// Banco de pruebas de Report
//
// Report usa tres cosas del entorno: la tool (PATH, ver fakeTools), la
// terminal (cli.IsTTY mira os.Stdin) y la entrada estándar (el paquete
// cli la lee con un bufio.Reader sobre el os.Stdin original).
//
// Para fingir una terminal:
//   1. se mete un pipe con las respuestas en el fd 0 (el bufio del
//      paquete cli sigue apuntando al fd 0 original, así que lee del
//      pipe);
//   2. se reemplaza os.Stdin por /dev/null, que es un char device, para
//      que cli.IsTTY() dé true.
//
// Ojo: el bufio del paquete cli es estado compartido entre tests, así que
// los escenarios interactivos viven en un único test y se alimentan en
// orden, uno por línea.
// ---------------------------------------------------------------------------

// ttyStdin pone respuestas en la entrada estándar y finge una terminal.
// Devuelve una función para restaurar todo.
func ttyStdin(t *testing.T, answers string) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := io.WriteString(w, answers); err != nil {
		t.Fatalf("escritura: %v", err)
	}
	w.Close()

	oldFd, err := syscall.Dup(0)
	if err != nil {
		t.Fatalf("dup: %v", err)
	}
	if err := syscall.Dup2(int(r.Fd()), 0); err != nil {
		t.Fatalf("dup2: %v", err)
	}
	oldStdin := os.Stdin
	devNull, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	os.Stdin = devNull

	return func() {
		os.Stdin = oldStdin
		devNull.Close()
		syscall.Dup2(oldFd, 0)
		syscall.Close(oldFd)
		r.Close()
	}
}

// captureStdout corre fn y devuelve lo que se imprimió por stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = old
	out := <-done
	r.Close()
	return out
}

// captureStderr corre fn y devuelve lo que se imprimió por stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stderr = old
	out := <-done
	r.Close()
	return out
}

// listConN arma una tabla de list con n IDs (350, 351, ...).
func listConN(n int) string {
	var b strings.Builder
	b.WriteString("\n ID │ Date │ Description \n────┼──────┼─────────────\n")
	for i := 0; i < n; i++ {
		b.WriteString(" " + itoa(350+i) + " │ 2026-11-03 01:39:10 │ manual\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// ---------------------------------------------------------------------------
// Report sin interacción (yes=true)
// ---------------------------------------------------------------------------

func TestReport_ConYes_Poda(t *testing.T) {
	log := fakeTools(t, fakeInfo, listConN(10))
	var code int
	out := captureStdout(t, func() { code = Report(5, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "snapshots de Limine: 39") {
		t.Fatalf("esperaba el conteo en la salida, obtuve %q", out)
	}
	if !strings.Contains(out, "5 entradas borradas") {
		t.Fatalf("esperaba el reporte de borradas, obtuve %q", out)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("esperaba remove + sync, obtuve %v", calls)
	}
	if !strings.HasPrefix(calls[0], "limine-snapper-remove 350 351 352 353 354") {
		t.Fatalf("esperaba borrar las 5 más viejas, obtuve %q", calls[0])
	}
}

func TestReport_ConYes_KeepCeroSeNormaliza(t *testing.T) {
	// keep<=0 se normaliza a 20 antes de cualquier cosa.
	log := fakeTools(t, fakeInfo, listConN(30))
	var code int
	captureStdout(t, func() { code = Report(0, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("esperaba remove + sync, obtuve %v", calls)
	}
	// 30 entradas, keep 20 → borra 10.
	if n := strings.Count(calls[0], " "); n != 10 {
		t.Fatalf("esperaba 10 IDs en el remove, obtuve %d (%q)", n, calls[0])
	}
}

func TestReport_ConYes_KeepNegativoSeNormaliza(t *testing.T) {
	// Un --keep=-5 no llega al CLI (main.go exige n>0), pero Report es
	// API: keep<=0 tiene que volverse 20, no un keep negativo.
	log := fakeTools(t, fakeInfo, listConN(30))
	var code int
	captureStdout(t, func() { code = Report(-5, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("esperaba remove + sync, obtuve %v", calls)
	}
	if n := strings.Count(calls[0], " "); n != 10 {
		t.Fatalf("esperaba 10 IDs (keep=20), obtuve %d (%q)", n, calls[0])
	}
}

func TestReport_ConYes_TotalMenorQueKeepNoPoda(t *testing.T) {
	log := fakeTools(t, fakeInfoEmpty, listConN(3))
	var code int
	captureStdout(t, func() { code = Report(20, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haber llamado a sudo, obtuve %v", calls)
	}
}

func TestReport_ConYes_FallaElConteoNoFrena(t *testing.T) {
	// Si la tool de conteo no está, Report avisa y devuelve 0: la poda
	// es opcional y no puede romper el update.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	var code int
	out := captureStdout(t, func() { code = Report(5, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "snapshots de Limine:") {
		t.Fatalf("esperaba el aviso, obtuve %q", out)
	}
}

func TestReport_ConYes_FallaLaPodaDevuelveUno(t *testing.T) {
	// Comportamiento actual, documentado en el README ("1 si la poda
	// falló"). Se fija acá para que un cambio sea deliberado.
	//
	// Nota de diseño (no bug): es incoherente con el caso contiguo. Si
	// el conteo falla —que es igual de grave— Report devuelve 0 y el
	// update sigue; si falla la poda devuelve 1 y "b3l0w update" sale
	// con error aunque el sistema ya se actualizó bien. Ver tests.md.
	fakeTools(t, fakeInfo, listConN(10))
	t.Setenv("FAKE_SUDO_FAIL_ON", "limine-snapper-remove")
	var code int
	errOut := captureStderr(t, func() { code = Report(5, true) })
	if code != 1 {
		t.Fatalf("esperaba 1, obtuve %d", code)
	}
	if !strings.Contains(errOut, "limpiar snapshots:") {
		t.Fatalf("esperaba el error por stderr, obtuve %q", errOut)
	}
}

func TestReport_ConYes_FallaElSyncDevuelveUno(t *testing.T) {
	fakeTools(t, fakeInfo, listConN(10))
	t.Setenv("FAKE_SUDO_FAIL_ON", "limine-snapper-sync")
	var code int
	errOut := captureStderr(t, func() { code = Report(5, true) })
	if code != 1 {
		t.Fatalf("esperaba 1, obtuve %d", code)
	}
	if !strings.Contains(errOut, "sync:") {
		t.Fatalf("esperaba distinguir el sync del remove, obtuve %q", errOut)
	}
}

func TestReport_ConYes_SinLaToolDeListado(t *testing.T) {
	// El conteo funciona pero la lista no: Prune falla y Report avisa.
	// Solo se instala limine-snapper-info, no limine-snapper-list.
	dir := t.TempDir()
	info := filepath.Join(dir, "limine-snapper-info")
	if err := os.WriteFile(info, []byte("#!/bin/sh\nprintf '%s' \"$FAKE_INFO\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_INFO", fakeInfo)
	t.Setenv("FAKE_LIST", "")
	t.Setenv("FAKE_LOG", filepath.Join(dir, "sudo.log"))
	t.Setenv("PATH", dir)
	var code int
	captureStdout(t, func() { code = Report(5, true) })
	if code != 1 {
		t.Fatalf("esperaba 1, obtuve %d", code)
	}
}

func TestReport_ElMensajeDeQuedanMiente(t *testing.T) {
	// [BUG] El cierre dice "quedan keep", pero el número real de entradas
	// que quedan es el total menos las borradas.
	//
	// Acá: info dice 39, la lista tiene 10, keep 5 → se borran 5 y
	// quedan 34 entradas de arranque en Limine. El mensaje dice
	// "quedan 5".
	//
	// Impacto: el usuario cree que le quedan 5 entradas de arranque
	// cuando le quedan 34, y deja de limpiar.
	//
	// Fix esperado: informar el total real (total - borradas).
	fakeTools(t, fakeInfo, listConN(10))
	var code int
	out := captureStdout(t, func() { code = Report(5, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "quedan 34") {
		t.Fatalf("[BUG] el mensaje debería decir cuántas quedan de verdad (34), obtuve %q", out)
	}
}

func TestReport_ConDiscrepanciaEntreConteoYLista(t *testing.T) {
	// [BUG] El conteo y la lista vienen de comandos distintos:
	// limine-snapper-info dice cuántas entradas hay y limine-snapper-list
	// dice cuáles son. El README ya lo admite ("no es necesariamente el
	// mismo conjunto"), pero el módulo no lo contempla.
	//
	// Acá: info dice 39, la lista tiene 10, keep 20.
	//   - Report compara 39 > 20 y pregunta "¿Reducir de 39 a 20?".
	//   - el usuario dice que sí.
	//   - Prune ve len(ids)=10 <= 20 y no borra NADA.
	//   - el mensaje dice "0 entradas borradas; quedan 20".
	//
	// Impacto: el usuario confirma una reducción y no ocurre nada, sin
	// aviso. Peor con keep chico: el conteo dice 39 y la lista dice 3,
	// se borran 0 y el mensaje dice "quedan 20".
	//
	// Fix esperado: usar una sola fuente de verdad, o avisar cuando el
	// conteo y la lista no coinciden. Acá se espera que el cierre diga
	// la verdad: 39 - 0 borradas = 39.
	fakeTools(t, fakeInfo, listConN(10))
	var code int
	out := captureStdout(t, func() { code = Report(20, true) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "quedan 39") {
		t.Fatalf("[BUG] con 39 entradas y 0 borradas deberían quedar 39, obtuve %q", out)
	}
}

// ---------------------------------------------------------------------------
// Report interactivo
// ---------------------------------------------------------------------------

func TestReport_Interactivo(t *testing.T) {
	// Todos los escenarios interactivos en un solo test porque el bufio
	// de stdin del paquete cli es estado compartido: las respuestas se
	// leen en orden.
	//
	// Escenarios, en orden:
	//   1. "y" + "5"    → poda con keep=5
	//   2. "c"         → el usuario cancela en la confirmación
	//   3. "y" + "c"   → el usuario cancela al pedir el número
	//   4. "n"         → el usuario dice que no
	restoreTTY := confirm.SetTTYForTest(func() bool { return true })
	defer restoreTTY()
	restore := ttyStdin(t, "y\n5\nc\ny\nc\nn\n")
	defer restore()

	// 1. Confirmar y pedir 5.
	log := fakeTools(t, fakeInfo, listConN(10))
	var code int
	out := captureStdout(t, func() { code = Report(5, false) })
	if code != 0 {
		t.Fatalf("escenario 1: esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "5 entradas borradas") {
		t.Fatalf("escenario 1: esperaba el reporte de borradas, obtuve %q", out)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("escenario 1: esperaba remove + sync, obtuve %v", calls)
	}
	if !strings.HasPrefix(calls[0], "limine-snapper-remove 350 351 352 353 354") {
		t.Fatalf("escenario 1: esperaba borrar 5, obtuve %q", calls[0])
	}

	// 2. "c" en la confirmación → cancelado, no poda.
	log2 := fakeTools(t, fakeInfo, listConN(10))
	out = captureStdout(t, func() { code = Report(5, false) })
	if code != 0 {
		t.Fatalf("escenario 2: esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("escenario 2: esperaba 'cancelled', obtuve %q", out)
	}
	if calls := sudoCalls(t, log2); len(calls) != 0 {
		t.Fatalf("escenario 2: no debería haber llamado a sudo, obtuve %v", calls)
	}

	// 3. Confirma y después cancela al pedir el número.
	log3 := fakeTools(t, fakeInfo, listConN(10))
	out = captureStdout(t, func() { code = Report(5, false) })
	if code != 0 {
		t.Fatalf("escenario 3: esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("escenario 3: esperaba 'cancelled', obtuve %q", out)
	}
	if calls := sudoCalls(t, log3); len(calls) != 0 {
		t.Fatalf("escenario 3: no debería haber llamado a sudo, obtuve %v", calls)
	}

	// 4. "n" → no poda.
	log4 := fakeTools(t, fakeInfo, listConN(10))
	out = captureStdout(t, func() { code = Report(5, false) })
	if code != 0 {
		t.Fatalf("escenario 4: esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("escenario 4: esperaba 'cancelled', obtuve %q", out)
	}
	if calls := sudoCalls(t, log4); len(calls) != 0 {
		t.Fatalf("escenario 4: no debería haber llamado a sudo, obtuve %v", calls)
	}
}

func TestReport_SinTerminalCancela(t *testing.T) {
	// Sin terminal (stdin redirected a un archivo) cli.Confirm devuelve
	// false: es más seguro no podar a ciegas. El update sigue igual.
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	log := fakeTools(t, fakeInfo, listConN(10))
	var code int
	out := captureStdout(t, func() { code = Report(5, false) })
	if code != 0 {
		t.Fatalf("esperaba 0, obtuve %d", code)
	}
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("esperaba 'cancelled', obtuve %q", out)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haber llamado a sudo, obtuve %v", calls)
	}
}
