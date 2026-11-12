package snapshots

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Banco de pruebas de los comandos externos
//
// limine.go llama a la tool por PATH (cli.Run / cli.RunCombined), así que
// alcanza con poner binarios falsos adelante en el PATH para observar qué
// hace el módulo sin tocar una sola línea de producción y sin necesitar
// root.
//
//   - limine-snapper-info  → imprime $FAKE_INFO
//   - limine-snapper-list  → imprime $FAKE_LIST
//   - sudo                 → loguea sus argumentos en $FAKE_LOG y puede
//                            fallar (FAKE_SUDO_EXIT / FAKE_SUDO_FAIL_ON)
//
// ---------------------------------------------------------------------------

const fakeInfo = "Snapshot entries  : 39 (max 0)\n"
const fakeInfoEmpty = "Snapshot entries  : 0 (max 0)\n"

const fakeList3 = "\n ID │ Date │ Description \n" +
	"────┼──────┼─────────────\n" +
	" 350 │ 2026-10-19 19:53:23 │ seanime-bin\n" +
	" 453 │ 2026-11-03 01:39:10 │ \n" +
	" 454 │ 2026-11-03 01:39:27 │ manual\n"

const fakeListNone = "\n ID │ Date │ Description \n────┼──────┼─────────────\n"

// fakeTools instala los binarios falsos y devuelve el path del log de sudo.
func fakeTools(t *testing.T, info, list string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "sudo.log")

	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatalf("no pude escribir %s: %v", name, err)
		}
	}
	write("limine-snapper-info", "#!/bin/sh\nprintf '%s' \"$FAKE_INFO\"\n")
	write("limine-snapper-list", "#!/bin/sh\nprintf '%s' \"$FAKE_LIST\"\n")
	write("sudo", "#!/bin/sh\n"+
		"printf '%s\\n' \"$*\" >> \"$FAKE_LOG\"\n"+
		"if [ -n \"$FAKE_SUDO_FAIL_ON\" ] && [ \"$1\" = \"$FAKE_SUDO_FAIL_ON\" ]; then\n"+
		"  echo 'boom' >&2\n"+
		"  exit \"${FAKE_SUDO_EXIT:-1}\"\n"+
		"fi\n"+
		"exit \"${FAKE_SUDO_EXIT:-0}\"\n")

	t.Setenv("FAKE_INFO", info)
	t.Setenv("FAKE_LIST", list)
	t.Setenv("FAKE_LOG", log)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// sudoCalls devuelve las invocaciones que el módulo le hizo a sudo.
func sudoCalls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("no pude leer el log de sudo: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Entries / IDs sobre la tool real (fake)
// ---------------------------------------------------------------------------

func TestEntries_LeeElConteo(t *testing.T) {
	fakeTools(t, fakeInfo, fakeList3)
	n, err := Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if n != 39 {
		t.Fatalf("esperaba 39, obtuve %d", n)
	}
}

func TestEntries_SinLaTool(t *testing.T) {
	// PATH sin limine-snapper-info: el error tiene que propagarse.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := Entries(); err == nil {
		t.Fatal("esperaba error si la tool no está en el PATH")
	}
}

func TestIDs_LeeLaTabla(t *testing.T) {
	fakeTools(t, fakeInfo, fakeList3)
	ids, err := IDs()
	if err != nil {
		t.Fatalf("IDs: %v", err)
	}
	want := []int{350, 453, 454}
	if len(ids) != len(want) {
		t.Fatalf("esperaba %v, obtuve %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("esperaba %v, obtuve %v", want, ids)
		}
	}
}

// ---------------------------------------------------------------------------
// Prune
// ---------------------------------------------------------------------------

func TestPrune_BorraLasViejas(t *testing.T) {
	log := fakeTools(t, fakeInfo, fakeList3)
	removed, err := Prune(2)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("esperaba 1 entrada borrada, obtuve %d", removed)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("esperaba remove + sync, obtuve %v", calls)
	}
	if calls[0] != "limine-snapper-remove 350" {
		t.Fatalf("esperaba que se borrara la más vieja (350), obtuve %q", calls[0])
	}
	if calls[1] != "limine-snapper-sync" {
		t.Fatalf("esperaba el sync al final, obtuve %q", calls[1])
	}
}

func TestPrune_UnSoloRemoveConTodosLosIDs(t *testing.T) {
	// El README promete un único remove con todos los IDs viejos, para
	// reescribir limine.conf una sola vez.
	log := fakeTools(t, fakeInfo, fakeList3)
	if _, err := Prune(1); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("esperaba 2 llamadas, obtuve %v", calls)
	}
	if calls[0] != "limine-snapper-remove 350 453" {
		t.Fatalf("esperaba un remove con los 2 IDs viejos, obtuve %q", calls[0])
	}
}

func TestPrune_KeepMayorQueLaCantidad(t *testing.T) {
	log := fakeTools(t, fakeInfo, fakeList3)
	removed, err := Prune(10)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haberCalled a sudo, obtuve %v", calls)
	}
}

func TestPrune_KeepIgualALaCantidad(t *testing.T) {
	log := fakeTools(t, fakeInfo, fakeList3)
	removed, err := Prune(3)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haber llamado a sudo, obtuve %v", calls)
	}
}

func TestPrune_KeepCero(t *testing.T) {
	// keep=0 con entradas presentes: se borra todo. No es alcanzable
	// desde Report (normaliza keep<=0 a 20) pero Prune es API pública.
	log := fakeTools(t, fakeInfo, fakeList3)
	removed, err := Prune(0)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 3 {
		t.Fatalf("esperaba 3 entradas borradas, obtuve %d", removed)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 || calls[0] != "limine-snapper-remove 350 453 454" {
		t.Fatalf("esperaba borrar las 3, obtuve %v", calls)
	}
}

func TestPrune_KeepNegativo(t *testing.T) {
	// [BUG] Prune es API pública (el README publica la firma) y con keep
	// negativo hace ids[:len(ids)-keep]:
	//   - si la capacidad alcanza, mete un ID 0 inventado en el lote y
	//     borra de más;
	//   - si no alcanza, entra en panic y tira el proceso.
	//
	// Impacto: un caller que pase un keep negativo (un --keep mal
	// parseado, un cálculo mal hecho) destruye entradas de arranque o
	// revienta el binario. El panic es lo peor: no hay error, hay crash.
	//
	// Fix esperado: keep<=0 es un no-op (o un error), nunca un índice
	// negativo.
	fakeTools(t, fakeInfo, fakeList3)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("[BUG] Prune con keep negativo Entró en panic: %v", r)
		}
	}()
	removed, err := Prune(-1)
	if err != nil {
		return // un error es un final aceptable
	}
	if removed != 0 {
		t.Fatalf("[BUG] un keep negativo no debería borrar nada, borró %d", removed)
	}
}

func TestPrune_KeepNegativoConPocaCapacidad(t *testing.T) {
	// [BUG] Variante del anterior: con pocas entradas el slice entra en
	// panic de verdad (capacity insuficiente).
	fakeTools(t, fakeInfo, " 350 │ a │ b\n")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("[BUG] Prune con keep negativo Entró en panic: %v", r)
		}
	}()
	if _, err := Prune(-1); err != nil {
		return
	}
}

func TestPrune_ListaVacia(t *testing.T) {
	log := fakeTools(t, fakeInfo, fakeListNone)
	removed, err := Prune(5)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haber llamado a sudo, obtuve %v", calls)
	}
}

func TestPrune_ListaConUnSoloID(t *testing.T) {
	log := fakeTools(t, fakeInfo, " 350 │ a │ b\n")
	removed, err := Prune(1)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if calls := sudoCalls(t, log); len(calls) != 0 {
		t.Fatalf("no debería haber llamado a sudo, obtuve %v", calls)
	}
}

func TestPrune_UnSoloIDYKeepCero(t *testing.T) {
	log := fakeTools(t, fakeInfo, " 350 │ a │ b\n")
	removed, err := Prune(0)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("esperaba 1 entrada borrada, obtuve %d", removed)
	}
	calls := sudoCalls(t, log)
	if len(calls) != 2 || calls[0] != "limine-snapper-remove 350" {
		t.Fatalf("esperaba borrar el 350, obtuve %v", calls)
	}
}

func TestPrune_ErrorEnRemove(t *testing.T) {
	fakeTools(t, fakeInfo, fakeList3)
	t.Setenv("FAKE_SUDO_FAIL_ON", "limine-snapper-remove")
	removed, err := Prune(1)
	if err == nil {
		t.Fatal("esperaba error si el remove falla")
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("el error debería traer la salida del comando: %v", err)
	}
}

func TestPrune_ErrorEnSync(t *testing.T) {
	// El remove funcionó pero el sync no: la config queda desactualizada.
	// El error tiene que distinguish el sync del remove.
	fakeTools(t, fakeInfo, fakeList3)
	t.Setenv("FAKE_SUDO_FAIL_ON", "limine-snapper-sync")
	removed, err := Prune(1)
	if err == nil {
		t.Fatal("esperaba error si el sync falla")
	}
	if removed != 0 {
		t.Fatalf("esperaba 0 entradas borradas, obtuve %d", removed)
	}
	if !strings.HasPrefix(err.Error(), "sync:") {
		t.Fatalf("el error del sync debería arrancar con 'sync:': %v", err)
	}
}

func TestPrune_NoHaceSyncSiElRemoveFalla(t *testing.T) {
	// Si el remove falla no tiene que correr el sync: la config quedó
	// como estaba y un sync sólo agregaría ruido.
	log := fakeTools(t, fakeInfo, fakeList3)
	t.Setenv("FAKE_SUDO_FAIL_ON", "limine-snapper-remove")
	if _, err := Prune(1); err == nil {
		t.Fatal("esperaba error")
	}
	for _, c := range sudoCalls(t, log) {
		if c == "limine-snapper-sync" {
			t.Fatalf("no debería haber corrido el sync: %v", sudoCalls(t, log))
		}
	}
}

func TestPrune_SinLaToolDeListado(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := Prune(5); err == nil {
		t.Fatal("esperaba error si limine-snapper-list no está")
	}
}
