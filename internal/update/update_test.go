package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/config"
)

// Estos tests cubren las funciones puras del orquestador y los caminos
// que se pueden ejercitar sin tocar el sistema.
//
// La regla del módulo es tajante: ningún test puede mutar la máquina.
// Por eso los binarios que el código busca en el PATH (vercmp, pacman,
// snapper, noctalia, cachy-update) se reemplazan por scripts de mentira
// en un directorio temporal, y el PATH se acota a ese directorio. Si el
// código intenta ejecutar algo real, el marker queda en disco y el test
// falla.

// ansiRegex saca las secuencias de color: el paquete output las agrega
// si stdout parece una terminal, y eso no debe depender de cómo se
// corran los tests.
var ansiRegex = regexp.MustCompile("\x1b\\[[0-9;]*m")

// capturarSalida corre fn con stdout y stderr atrapados y devuelve lo
// que se imprimió, sin los códigos de color.
func capturarSalida(t *testing.T, fn func()) (string, string) {
	t.Helper()

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	outDone := make(chan struct{})
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&outBuf, rOut)
		close(outDone)
	}()
	go func() {
		_, _ = io.Copy(&errBuf, rErr)
		close(errDone)
	}()

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	fn()
	os.Stdout, os.Stderr = origOut, origErr

	_ = wOut.Close()
	_ = wErr.Close()
	<-outDone
	<-errDone
	_ = rOut.Close()
	_ = rErr.Close()

	return ansiRegex.ReplaceAllString(outBuf.String(), ""),
		ansiRegex.ReplaceAllString(errBuf.String(), "")
}

// sinTTY deja stdin como un archivo común: no es una terminal, así que
// IsTTY() da false y IsTTY no deja la terminal del runner cojea.
func sinTTY(t *testing.T) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("stdin temporal: %v", err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = orig
		_ = f.Close()
	})
}

// fakeBin escribe un script ejecutable en dir y lo devuelve.
//
// El script tiene que ser autónomo: se corre con un PATH que solo tiene
// el directorio de los fakes, así que no puede llamar a nada más.
func fakeBin(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("escribir fake %s: %v", name, err)
	}
	return path
}

// fakeSalida escribe un fake que imprime exactamente salida, byte a byte.
//
// Se usa printf con el formato '%s' y no el valor directo: printf
// interpreta un "-1" al principio como una opción y falla.
func fakeSalida(t *testing.T, dir, nombre, salida string) string {
	t.Helper()
	// Escapar la comilla simple: la forma POSIX es cerrar, escapar y
	// volver a abrir.
	escapada := strings.ReplaceAll(salida, "'", `'\''`)
	return fakeBin(t, dir, nombre, "#!/bin/sh\nprintf '%s' '"+escapada+"'\n")
}

// marcador devuelve un PATH con un único binario que, si llega a
// ejecutarse, deja un archivo en disco. Sirve para demostrar que una
// función NO llama a un comando del sistema.
func marcador(t *testing.T, dir, nombre, cuerpo string) (string, string) {
	t.Helper()
	marca := filepath.Join(t.TempDir(), "ejecutado")
	script := "#!/bin/sh\n" +
		"printf 'x' > \"" + marca + "\"\n" +
		cuerpo
	fakeBin(t, dir, nombre, script)
	return dir, marca
}

// TestConfirmPromptSinSeleccion cubre la rama que no había: sin nada
// que actualizar el prompt no lista capas.
func TestConfirmPromptSinSeleccion(t *testing.T) {
	got := confirmPrompt(false, false, false, false, false, false)
	if got != "Update? [Y/n] " {
		t.Errorf("confirmPrompt() = %q, quería %q", got, "Update? [Y/n] ")
	}
}

// TestConfirmPromptOrdenDeLasCapas fija el orden de las capas en el
// prompt: el usuario tiene que leer la lista en el mismo orden en que se
// ejecutan, no en el orden en que se pasaron los flags.
func TestConfirmPromptOrdenDeLasCapas(t *testing.T) {
	cases := []struct {
		self, system, programs, plugins, services, dotfiles bool
		want                                                string
	}{
		{false, false, false, true, true, false, "Update plugins, services? [Y/n] "},
		{false, true, true, false, false, false, "Update system, programs? [Y/n] "},
		{true, false, false, false, false, true, "Update b3l0w, dotfiles? [Y/n] "},
		{false, false, false, false, false, true, "Update dotfiles? [Y/n] "},
	}
	for _, c := range cases {
		got := confirmPrompt(c.self, c.system, c.programs, c.plugins, c.services, c.dotfiles)
		if got != c.want {
			t.Errorf("confirmPrompt(%v,%v,%v,%v,%v,%v) = %q, quería %q",
				c.self, c.system, c.programs, c.plugins, c.services, c.dotfiles, got, c.want)
		}
	}
}

// TestDefaultDotfilesDirPrefiereElRepoDeGithub el repo clonado gana:
// es la fuente que el usuario editó.
func TestDefaultDotfilesDirPrefiereElRepoDeGithub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := filepath.Join(home, "Documents", "Github", "b3l0w", "dotfiles")
	share := filepath.Join(home, ".local", "share", "b3l0w", "dotfiles")
	for _, dir := range []string{repo, share} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got := defaultDotfilesDir(); got != repo {
		t.Errorf("defaultDotfilesDir() = %q, quería %q", got, repo)
	}
}

// TestDefaultDotfilesDirCaeAlShare si el repo no está, se usa la copia
// compartida.
func TestDefaultDotfilesDirCaeAlShare(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	share := filepath.Join(home, ".local", "share", "b3l0w", "dotfiles")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := defaultDotfilesDir(); got != share {
		t.Errorf("defaultDotfilesDir() = %q, quería %q", got, share)
	}
}

// TestDefaultDotfilesDirSinDotfiles no hay repo ni copia: devuelve vacío
// y el llamador avisa que falta --dotfiles-dir.
func TestDefaultDotfilesDirSinDotfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if got := defaultDotfilesDir(); got != "" {
		t.Errorf("defaultDotfilesDir() = %q, quería vacío", got)
	}
}

// TestDefaultDotfilesDirSinHome sin HOME no se puede resolver el home y
// el error se tradujo en cadena vacía.
func TestDefaultDotfilesDirSinHome(t *testing.T) {
	t.Setenv("HOME", "")

	if got := defaultDotfilesDir(); got != "" {
		t.Errorf("defaultDotfilesDir() = %q, quería vacío", got)
	}
}

// TestFileExists cubre el existence check que usa el orquestador para
// decidir si hay un TOML que validar.
func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	archivo := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(archivo, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !fileExists(archivo) {
		t.Errorf("fileExists(%q) = false, quería true", archivo)
	}
	if !fileExists(dir) {
		t.Errorf("fileExists(%q) = false, quería true (os.Stat también acepta directorios)", dir)
	}
	if fileExists(filepath.Join(dir, "no-existe.toml")) {
		t.Error("fileExists de un path inexistente dio true")
	}
	if fileExists("") {
		t.Error("fileExists(\"\") dio true")
	}
}

// TestCompareVersionsLeeLaSalidaDeVercmp verifica que el número que
// devuelve vercmp se traduce tal cual al entero que espera el resto del
// código.
func TestCompareVersionsLeeLaSalidaDeVercmp(t *testing.T) {
	cases := []struct {
		salida string
		want   int
	}{
		{"-1\n", -1},
		{"0\n", 0},
		{"1\n", 1},
		{" -1 \n", -1},
		{"1", 1},
	}
	for _, c := range cases {
		dir := t.TempDir()
		fakeSalida(t, dir, "vercmp", c.salida)
		t.Setenv("PATH", dir)

		if got := compareVersions("1.0", "2.0"); got != c.want {
			t.Errorf("compareVersions con vercmp %q = %d, quería %d", c.salida, got, c.want)
		}
	}
}

// TestCompareVersionsSinVercmpDevuelveCero si vercmp no está (o falla),
// la comparación asume que son iguales. Es la regla que evita inventar
// updates: sin vercmp no se dice que hay nada nuevo.
func TestCompareVersionsSinVercmpDevuelveCero(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if got := compareVersions("1.0", "2.0"); got != 0 {
		t.Errorf("compareVersions sin vercmp = %d, quería 0", got)
	}
}

// TestCompareVersionsConSalidaInvalida una salida que no es un número
// se trata como "sin vercmp": cero, no un update inventado.
func TestCompareVersionsConSalidaInvalida(t *testing.T) {
	for _, salida := range []string{"abc\n", "\n", "1.5\n"} {
		dir := t.TempDir()
		fakeSalida(t, dir, "vercmp", salida)
		t.Setenv("PATH", dir)

		if got := compareVersions("1.0", "2.0"); got != 0 {
			t.Errorf("compareVersions con vercmp %q = %d, quería 0", salida, got)
		}
	}
}

// TestCompareVersionsConVercmpQueFalla el error de vercmp no se propaga:
// se asume igualdad.
func TestCompareVersionsConVercmpQueFalla(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "vercmp", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", dir)

	if got := compareVersions("1.0", "2.0"); got != 0 {
		t.Errorf("compareVersions con vercmp fallando = %d, quería 0", got)
	}
}

// TestCompareVersionsPasaLaVersionLocalPrimero el orden de los
// argumentos importa: vercmp compara el primero contra el segundo.
func TestCompareVersionsPasaLaVersionLocalPrimero(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "vercmp", "#!/bin/sh\nif [ \"$1\" = \"local\" ]; then printf '1'; else printf '0'; fi\n")
	t.Setenv("PATH", dir)

	if got := compareVersions("local", "remoto"); got != 1 {
		t.Errorf("compareVersions = %d, quería 1 (el local va primero)", got)
	}
	if got := compareVersions("remoto", "local"); got != 0 {
		t.Errorf("compareVersions = %d, quería 0", got)
	}
}

// TestCompareVersionsConVercmpReal usa el vercmp de pacman de verdad
// para fijar la semántica de las versiones: epoch, pkgrel, vacío.
//
// vercmp solo compara dos strings y escribe -1/0/1: no lee la base de
// datos ni toca nada. Si no está instalado, el test se saltea.
func TestCompareVersionsConVercmpReal(t *testing.T) {
	if _, err := exec.LookPath("vercmp"); err != nil {
		t.Skip("vercmp no está instalado")
	}

	cases := []struct {
		local, remote string
		want          int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1:1.0", "2.0", 1},
		{"1.0-1", "1.0-2", -1},
		{"1.0", "1.0.0", -1},
		{"", "", 0},
		{"", "1.0", -1},
		{"1.0", "", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.local, c.remote); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, quería %d", c.local, c.remote, got, c.want)
		}
	}
}

// TestLocalVersionLeeLaVersionEl formato de pacman -Q es "nombre
// version": el segundo campo es la versión.
func TestLocalVersionLeeLaVersion(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "pacman", "#!/bin/sh\nprintf 'noctalia 1.2.3-1\\n'\n")
	t.Setenv("PATH", dir)

	if got := localVersion("noctalia"); got != "1.2.3-1" {
		t.Errorf("localVersion = %q, quería %q", got, "1.2.3-1")
	}
}

// TestLocalVersionConSalidaIncompleta si pacman no devuelve nombre y
// versión, no hay versión que informar.
func TestLocalVersionConSalidaIncompleta(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "pacman", "#!/bin/sh\nprintf 'noctalia\\n'\n")
	t.Setenv("PATH", dir)

	if got := localVersion("noctalia"); got != "" {
		t.Errorf("localVersion = %q, quería vacío", got)
	}
}

// TestLocalVersionConErrorEl error de pacman se traduce en versión
// desconocida, no en un panic.
func TestLocalVersionConError(t *testing.T) {
	dir := t.TempDir()
	fakeBin(t, dir, "pacman", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", dir)

	if got := localVersion("noctalia"); got != "" {
		t.Errorf("localVersion = %q, quería vacío", got)
	}
}

// TestUpdateProgramsSinManifiesto sin programas registrados no hay nada
// que actualizar y no se ejecuta nada.
func TestUpdateProgramsSinManifiesto(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	salida, _ := capturarSalida(t, func() {
		if code := updatePrograms(&config.Manifest{}); code != 0 {
			t.Errorf("updatePrograms() = %d, quería 0", code)
		}
	})
	if !strings.Contains(salida, "no hay programas registrados") {
		t.Errorf("no avisó que no hay programas: %q", salida)
	}
}

// TestUpdateProgramsReportaElProgramaAusente un programa que no está
// instalado se cuenta como fallo. El binario no se busca en el PATH real:
// con el PATH acotado, CommandPath no lo encuentra y no se ejecuta nada.
func TestUpdateProgramsReportaElProgramaAusente(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	m := &config.Manifest{
		Version:  config.Schema,
		Programs: []config.Program{{Name: "programa-inexistente"}},
	}

	_, errOut := capturarSalida(t, func() {
		if code := updatePrograms(m); code != 1 {
			t.Errorf("updatePrograms() = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "no está instalado") {
		t.Errorf("no reportó el programa ausente: %q", errOut)
	}
	if !strings.Contains(errOut, "1 de 1 programas fallaron") {
		t.Errorf("no reportó el conteo de fallos: %q", errOut)
	}
}

// TestUpdateServicesSinEstado los servicios son opcionales: sin estado
// guardado no hay nada que hacer y no se toca Docker.
func TestUpdateServicesSinEstado(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	salida, _ := capturarSalida(t, func() {
		if code := updateServices(true); code != 0 {
			t.Errorf("updateServices() = %d, quería 0", code)
		}
	})
	if strings.Contains(salida, "Actualizando servicios") {
		t.Errorf("anunció la actualización de servicios sin estado: %q", salida)
	}
}

// TestUpdateSystemSinActualizador si no hay cachy-update ni arch-update
// el sistema se omite. Pedir solo el sistema es un error; pedirlo junto
// a otra capa es solo un aviso.
func TestUpdateSystemSinActualizador(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	salida, _ := capturarSalida(t, func() {
		if code := updateSystem(Options{System: true}); code != 1 {
			t.Errorf("updateSystem(--system) = %d, quería 1", code)
		}
	})
	if !strings.Contains(salida, "no hay cachy-update ni arch-update") {
		t.Errorf("no avisó que falta el actualizador: %q", salida)
	}

	salida, _ = capturarSalida(t, func() {
		if code := updateSystem(Options{System: true, Programs: true}); code != 0 {
			t.Errorf("updateSystem(--system --programs) = %d, quería 0", code)
		}
	})
	if !strings.Contains(salida, "no hay cachy-update ni arch-update") {
		t.Errorf("no avisó que falta el actualizador: %q", salida)
	}
}

// TestUpdateSystemSinTerminalEl actualizador del sistema es
// interactivo: sin terminal no se lanza. El fake deja marker si llega a
// ejecutarse, así que el test falla si se invoca.
func TestUpdateSystemSinTerminal(t *testing.T) {
	dir := t.TempDir()
	_, marca := marcador(t, dir, "cachy-update", "exit 0\n")
	t.Setenv("PATH", dir)
	sinTTY(t)

	_, errOut := capturarSalida(t, func() {
		if code := updateSystem(Options{System: true}); code != 1 {
			t.Errorf("updateSystem(--system) sin terminal = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "sin terminal") {
		t.Errorf("no avisó que falta la terminal: %q", errOut)
	}
	if _, err := os.Stat(marca); err == nil {
		t.Error("se ejecutó el actualizador del sistema sin terminal")
	}
}

// TestUpdateDotfilesSinDirectorio sin repo de dotfiles el comando avisa
// y sale con error, sin tocar el home.
func TestUpdateDotfilesSinDirectorio(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	_, errOut := capturarSalida(t, func() {
		if code := updateDotfiles(Options{}); code != 1 {
			t.Errorf("updateDotfiles() sin directorio = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "dotfiles/ not found") {
		t.Errorf("no avisó que falta el repo de dotfiles: %q", errOut)
	}
}

// TestUpdateDotfilesAdoptEl modo --adopt registra el hash de lo que hay
// en el home como base y no toca ni un archivo: es la línea de base
// para que el merge 3-way no marque todo como conflicto.
//
// El PATH está acotado a un fake de snapper y noctalia que dejan marker
// si se ejecutan: --adopt no debe tocar ninguno de los dos.
func TestUpdateDotfilesAdopt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app", "config.toml"), []byte("del repo = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(home, ".config", "app", "config.toml")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("local = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	_, marcaSnapper := marcador(t, dir, "snapper", "exit 0\n")
	_, marcaNoctalia := marcador(t, dir, "noctalia", "exit 0\n")
	t.Setenv("PATH", dir)

	salida, _ := capturarSalida(t, func() {
		if code := updateDotfiles(Options{Adopt: true, DotfilesDir: repo}); code != 0 {
			t.Errorf("updateDotfiles(--adopt) = %d, quería 0", code)
		}
	})
	if !strings.Contains(salida, "1 files adopted (untouched)") {
		t.Errorf("no reportó la adopción: %q", salida)
	}

	// El archivo local no se tocó.
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "local = true\n" {
		t.Errorf("el archivo local cambió: %q", string(data))
	}

	// El estado guarda el hash de lo que había, no lo del repo.
	estado := filepath.Join(home, ".local", "state", "b3l0w", "dotfiles.json")
	raw, err := os.ReadFile(estado)
	if err != nil {
		t.Fatalf("no se escribió el estado: %v", err)
	}
	suma := sha256.Sum256([]byte("local = true\n"))
	want := hex.EncodeToString(suma[:])
	if !strings.Contains(string(raw), want) {
		t.Errorf("el estado no tiene el hash del archivo local: %s", string(raw))
	}
	if !strings.Contains(string(raw), "app/config.toml") {
		t.Errorf("el estado no tiene la ruta relativa: %s", string(raw))
	}

	if _, err := os.Stat(marcaSnapper); err == nil {
		t.Error("--adopt creó una snapshot")
	}
	if _, err := os.Stat(marcaNoctalia); err == nil {
		t.Error("--adopt ejecutó noctalia")
	}
}

// TestDesktopUpdateMuestraElComandoPorCanalEl escritorio no se actualiza
// con pacman: el CLI muestra el comando y el canal.
//
// El fake de pacman deja marker: si el CLI lo ejecutara, el test falla.
func TestDesktopUpdateMuestraElComandoPorCanal(t *testing.T) {
	cases := []struct {
		canal string
		want  string
		no    string
	}{
		{"stable", "pacman -S umbriel noctalia", "umbriel-git"},
		{"edge", "pacman -S umbriel-git noctalia-git", "pacman -S umbriel noctalia"},
	}
	for _, c := range cases {
		home := t.TempDir()
		t.Setenv("HOME", home)
		canalDir := filepath.Join(home, ".config", "b3l0w")
		if err := os.MkdirAll(canalDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(canalDir, "channel"), []byte(c.canal+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		dir := t.TempDir()
		_, marca := marcador(t, dir, "pacman", "exit 0\n")
		t.Setenv("PATH", dir)

		salida, _ := capturarSalida(t, func() {
			if code := desktopUpdate(); code != 0 {
				t.Errorf("desktopUpdate() = %d, quería 0", code)
			}
		})
		if !strings.Contains(salida, "canal: "+c.canal) {
			t.Errorf("canal %s: no lo muestra: %q", c.canal, salida)
		}
		if !strings.Contains(salida, c.want) {
			t.Errorf("canal %s: no mostró %q: %q", c.canal, c.want, salida)
		}
		if strings.Contains(salida, c.no) {
			t.Errorf("canal %s: mostró %q que no corresponde: %q", c.canal, c.no, salida)
		}
		if _, err := os.Stat(marca); err == nil {
			t.Errorf("canal %s: el CLI ejecutó pacman", c.canal)
		}
	}
}

// TestRunSinYesCancelaSinTerminal sin --yes y sin terminal el update se
// cancela: no se actualiza a ciegas.
func TestRunSinYesCancelaSinTerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	sinTTY(t)

	salida, _ := capturarSalida(t, func() {
		if code := Run(&config.Manifest{}, Options{}); code != 0 {
			t.Errorf("Run() sin --yes = %d, quería 0", code)
		}
	})
	if !strings.Contains(salida, "cancelled") {
		t.Errorf("no canceló: %q", salida)
	}
}

// TestRunConYesReportaElProgramaAusente el flujo completo con --yes
// llega a los programas y reporta los que no están instalados, sin
// ejecutar nada del sistema.
func TestRunConYesReportaElProgramaAusente(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	sinTTY(t)

	m := &config.Manifest{
		Version:  config.Schema,
		Programs: []config.Program{{Name: "programa-inexistente"}},
	}

	_, errOut := capturarSalida(t, func() {
		if code := Run(m, Options{Yes: true, Programs: true}); code != 1 {
			t.Errorf("Run(--yes --programs) = %d, quería 1", code)
		}
	})
	if !strings.Contains(errOut, "no está instalado") {
		t.Errorf("no reportó el programa ausente: %q", errOut)
	}
}

// TestRunDryRunMuestraElPlanEl dry-run informa qué haría y no ejecuta
// nada: en este caso no hay self-update que consultar, así que el plan
// se arma sin tocar la red.
func TestRunDryRunMuestraElPlan(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	sinTTY(t)

	m := &config.Manifest{
		Version:  config.Schema,
		Programs: []config.Program{{Name: "boot-preview"}},
	}

	salida, _ := capturarSalida(t, func() {
		if code := Run(m, Options{DryRun: true, Programs: true}); code != 0 {
			t.Errorf("Run(--dry-run --programs) = %d, quería 0", code)
		}
	})
	if !strings.Contains(salida, "Dry-run: no se toca nada") {
		t.Errorf("no announces el dry-run: %q", salida)
	}
	if !strings.Contains(salida, "programas: 1 se revisarian") {
		t.Errorf("no contó los programas: %q", salida)
	}
}
