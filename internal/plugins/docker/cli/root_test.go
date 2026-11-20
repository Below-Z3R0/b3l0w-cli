package dockercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtraerValorSacaElFlagYSuValor(t *testing.T) {
	resto, valor, ok := extraerValor([]string{"web", "--tail", "120", "--json"}, "--tail")
	if !ok {
		t.Fatal("debería encontrar --tail")
	}
	if valor != "120" {
		t.Errorf("valor = %q, quería 120", valor)
	}
	if strings.Contains(strings.Join(resto, " "), "--tail") {
		t.Errorf("el resto conservó el flag: %v", resto)
	}
	if strings.Contains(strings.Join(resto, " "), "120") {
		t.Errorf("el resto conservó el valor: %v", resto)
	}
}

func TestExtraerValorAceptaElFormatoConIgual(t *testing.T) {
	resto, valor, ok := extraerValor([]string{"web", "--tail=120"}, "--tail")
	if !ok {
		t.Fatal("debería encontrar --tail")
	}
	if valor != "120" {
		t.Errorf("valor = %q, quería 120", valor)
	}
	if len(resto) != 1 || resto[0] != "web" {
		t.Errorf("resto = %v, quería [web]", resto)
	}
}

func TestExtraerValorSinElFlag(t *testing.T) {
	resto, valor, ok := extraerValor([]string{"web"}, "--tail")
	if ok {
		t.Error("no debería encontrar el flag")
	}
	if valor != "" {
		t.Errorf("valor = %q, quería vacío", valor)
	}
	if len(resto) != 1 || resto[0] != "web" {
		t.Errorf("resto = %v", resto)
	}
}

func TestSacarFlagQuitaTodasLasOcurrencias(t *testing.T) {
	resto, ok := sacarFlag([]string{"--json", "web", "--json"}, "--json")
	if !ok {
		t.Error("debería encontrar el flag")
	}
	if len(resto) != 1 || resto[0] != "web" {
		t.Errorf("resto = %v, quería [web]", resto)
	}
}

func TestSacarFlagSinElFlag(t *testing.T) {
	resto, ok := sacarFlag([]string{"web"}, "--json")
	if ok {
		t.Error("no debería encontrar el flag")
	}
	if len(resto) != 1 || resto[0] != "web" {
		t.Errorf("resto = %v", resto)
	}
}

func TestPartirRutaContenedor(t *testing.T) {
	casos := []struct {
		ruta    string
		prefijo string
		resto   string
	}{
		{"61555f00de09:/etc/hostname", "61555f00de09", "/etc/hostname"},
		{"id:/ruta", "id", "/ruta"},
		{"/ruta/normal", "", "/ruta/normal"},
		{"archivo.txt", "", "archivo.txt"},
	}
	for _, c := range casos {
		prefijo, resto := partirRutaContenedor(c.ruta)
		if prefijo != c.prefijo || resto != c.resto {
			t.Errorf("partirRutaContenedor(%q) = %q, %q; quería %q, %q",
				c.ruta, prefijo, resto, c.prefijo, c.resto)
		}
	}
}

func TestRutaRespaldoSeparaCarpetaYArchivo(t *testing.T) {
	casos := []struct {
		ruta    string
		carpeta string
		archivo string
	}{
		{"/tmp/backup.tar.gz", "/tmp", "backup.tar.gz"},
		{"/a/b/c/d.tar", "/a/b/c", "d.tar"},
	}
	for _, c := range casos {
		carpeta, archivo, err := rutaRespaldo(c.ruta)
		if err != nil {
			t.Errorf("rutaRespaldo(%q): %v", c.ruta, err)
			continue
		}
		if carpeta != c.carpeta || archivo != c.archivo {
			t.Errorf("rutaRespaldo(%q) = %q, %q; quería %q, %q",
				c.ruta, carpeta, archivo, c.carpeta, c.archivo)
		}
	}
}

// Una ruta relativa se resuelve dentro del directorio actual: antes
// devolvía la raíz del sistema y el comando montaba "/" del host (D4).
func TestRutaRespaldoResuelveLaRelativaDentroDelDirectorioActual(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	carpeta, archivo, err := rutaRespaldo("backup.tar.gz")
	if err != nil {
		t.Fatalf("rutaRespaldo: %v", err)
	}
	if carpeta != cwd {
		t.Errorf("carpeta = %q, quería el directorio actual %q", carpeta, cwd)
	}
	if archivo != "backup.tar.gz" {
		t.Errorf("archivo = %q", archivo)
	}
	if carpeta == "/" {
		t.Error("una ruta relativa no puede convertirse en la raíz del host")
	}
}

// Lo que queda fuera del directorio actual se rechaza, y la raíz nunca.
func TestRutaRespaldoRechazaLoQueQuedaFuera(t *testing.T) {
	casos := []string{"../fuera.tar.gz", "/backup.tar.gz", "", "   "}
	for _, ruta := range casos {
		if _, _, err := rutaRespaldo(ruta); err == nil {
			t.Errorf("rutaRespaldo(%q) debería fallar", ruta)
		}
	}
}

func TestRecorta(t *testing.T) {
	if got := recorta("corto", 10); got != "corto" {
		t.Errorf("recorta = %q, quería corto", got)
	}
	got := recorta("0123456789", 5)
	if len([]rune(got)) != 5 {
		t.Errorf("recorta = %q (%d runes), quería 5", got, len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("recorta = %q, quería elipsis al final", got)
	}
}

func TestListaFlagAcumula(t *testing.T) {
	var l listaFlag
	if l.String() != "" {
		t.Errorf("String() = %q, quería vacío", l.String())
	}
	if err := l.Set("8080"); err != nil {
		t.Fatal(err)
	}
	if err := l.Set("9090"); err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 {
		t.Fatalf("len = %d, quería 2", len(l))
	}
	if l.String() != "8080,9090" {
		t.Errorf("String() = %q, quería 8080,9090", l.String())
	}
}

func TestRutaLogSaneaElNombre(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	archivo, pid := rutaLog("Web/Server 01")
	if !strings.HasSuffix(archivo, ".log") {
		t.Errorf("archivo = %q, quería .log", archivo)
	}
	if !strings.HasSuffix(pid, ".pid") {
		t.Errorf("pid = %q, quería .pid", pid)
	}
	base := filepath.Base(archivo)
	if strings.ContainsAny(base, "/ ") {
		t.Errorf("el nombre del archivo debería estar saneado: %q", base)
	}
	// El nombre es el id saneado más un hash corto del id original: sin
	// el hash dos contenedores distintos ("Web" y "web") compartían el
	// archivo de log y el pidfile (D11).
	if !strings.HasPrefix(base, "web_server_01-") {
		t.Errorf("base = %q, quería el prefijo 'web_server_01-'", base)
	}
	if !strings.HasSuffix(base, ".log") {
		t.Errorf("base = %q, quería el sufijo '.log'", base)
	}
}

func TestRutaLogAcortaElNombre(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	largo := strings.Repeat("a", 100)
	archivo, _ := rutaLog(largo)
	base := filepath.Base(archivo)
	if len(base) > 45 {
		t.Errorf("el nombre es demasiado largo: %d caracteres", len(base))
	}
}

func TestLeerPid(t *testing.T) {
	dir := t.TempDir()
	archivo := filepath.Join(dir, "x.pid")
	if err := os.WriteFile(archivo, []byte("4321"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := leerPid(archivo); got != 4321 {
		t.Errorf("leerPid = %d, quería 4321", got)
	}
	if got := leerPid(filepath.Join(dir, "no-existe.pid")); got != 0 {
		t.Errorf("leerPid de un archivo inexistente = %d, quería 0", got)
	}
	if err := os.WriteFile(archivo, []byte("basura"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := leerPid(archivo); got != 0 {
		t.Errorf("leerPid de basura = %d, quería 0", got)
	}
}

func TestProcesoVivo(t *testing.T) {
	if procesoVivo(0) {
		t.Error("el pid 0 no es un proceso")
	}
	if procesoVivo(-1) {
		t.Error("un pid negativo no es un proceso")
	}
	if !procesoVivo(os.Getpid()) {
		t.Error("el proceso actual debería estar vivo")
	}
}
