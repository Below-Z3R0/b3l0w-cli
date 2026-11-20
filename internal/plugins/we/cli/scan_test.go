package wecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanExtensionesImprimeLaLista(t *testing.T) {
	codigo, salida, errSalida := correr(t, "scan", "--extensions")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var exts []string
	if err := json.Unmarshal([]byte(salida), &exts); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(exts) == 0 {
		t.Fatal("no listó ninguna extensión")
	}
	for _, want := range []string{".jpg", ".png", ".webp"} {
		if !contains(exts, want) {
			t.Errorf("extensiones = %v, faltaba %q", exts, want)
		}
	}
	// Ordenadas, para que el plugin las compare establemente.
	for i := 1; i < len(exts); i++ {
		if exts[i-1] > exts[i] {
			t.Fatalf("extensiones = %v, no están ordenadas", exts)
		}
	}
}

func TestScanExtensionesDeVideo(t *testing.T) {
	codigo, salida, _ := correr(t, "scan", "--extensions", "--kind", "video")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	var exts []string
	if err := json.Unmarshal([]byte(salida), &exts); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if !contains(exts, ".mp4") {
		t.Errorf("extensiones = %v, faltaba .mp4", exts)
	}
	if contains(exts, ".jpg") {
		t.Errorf("extensiones = %v, no quería las de imagen", exts)
	}
}

func TestScanListaLasImagenes(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, errSalida := correr(t, "scan", "--dir", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	lineas := strings.Fields(salida)
	if len(lineas) != 4 {
		t.Fatalf("listó %d archivos (%v), quería 4", len(lineas), lineas)
	}
	for _, want := range []string{"a.jpg", "b.png", "sub/c.jpg", "sub/deep/d.jpg"} {
		found := false
		for _, l := range lineas {
			if strings.HasSuffix(l, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no listó %q (listó %v)", want, lineas)
		}
	}
	// Los ocultos y los de más profundidad quedan fuera.
	for _, unwanted := range []string{"notes.txt", ".hidden/f.jpg", "deeper/e.jpg"} {
		if strings.Contains(salida, unwanted) {
			t.Errorf("listó %q, no debía (listó %v)", unwanted, lineas)
		}
	}
}

func TestScanRespetaLaProfundidad(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--depth", "1")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	lineas := strings.Fields(salida)
	if len(lineas) != 3 {
		t.Errorf("listó %d archivos (%v), quería 3 con --depth 1", len(lineas), lineas)
	}
}

func TestScanEnJSONTraeLosCamposDelPlugin(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, errSalida := correr(t, "scan", "--dir", dir, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, quería 4", len(items))
	}
	for _, it := range items {
		if it.ID == "" || it.File == "" || it.Type != "image" {
			t.Errorf("item incompleto: %+v", it)
		}
		if it.Title == "" {
			t.Errorf("item sin título: %+v", it)
		}
	}
	// El título es el nombre del archivo sin extensión y con los separadores
	// en espacios: sin esto el panel muestra la ruta completa.
	for _, it := range items {
		if strings.HasSuffix(it.File, "a.jpg") {
			if it.Title != "a" || it.Name != "a" {
				t.Errorf("title = %q, name = %q, quería \"a\"", it.Title, it.Name)
			}
		}
	}
}

func TestScanConvierteElNombreEnTitulo(t *testing.T) {
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "mi_fondo_bonito.jpg"), "x")
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, quería 1", len(items))
	}
	if items[0].Title != "mi fondo bonito" {
		t.Errorf("title = %q, quería \"mi fondo bonito\"", items[0].Title)
	}
}

func TestScanFiltraPorVideos(t *testing.T) {
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "foto.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "clip.mp4"), "x")
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--kind", "video", "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 || !strings.HasSuffix(items[0].File, "clip.mp4") {
		t.Fatalf("items = %+v, quería sólo el mp4", items)
	}
	if items[0].Type != "video" {
		t.Errorf("type = %q, quería video", items[0].Type)
	}
}

func TestScanRespetaElMaximo(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--max", "2")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if lineas := strings.Fields(salida); len(lineas) != 2 {
		t.Errorf("listó %d archivos (%v), quería 2", len(lineas), lineas)
	}
}

func TestScanTomaLaCarpetaSuelta(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, errSalida := correr(t, "scan", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	if !strings.Contains(salida, "a.jpg") {
		t.Errorf("stdout = %q, quería que liste el contenido", salida)
	}
}

func TestScanSinCarpetaFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--dir") {
		t.Errorf("stderr = %q, quería que pida la carpeta", errSalida)
	}
}

func TestScanDirSinValorFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--dir") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanMaxInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--max", "cero")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--max") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanMaxGrandeFalla(t *testing.T) {
	// El tope existe para que un --max absurdo no reserves esa memoria de
	// una y mate el proceso.
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--max", "100001")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "100000") {
		t.Errorf("stderr = %q, quería que diga el tope", errSalida)
	}
}

func TestScanSortInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--sort", "color")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--sort") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanDepthInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--depth", "-1")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--depth") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanThumbSizeInvalidoFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--thumb-size", "10")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--thumb-size") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanThumbsSinValorFalla(t *testing.T) {
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--thumbs")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--thumbs") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
}

func TestScanOpcionDesconocidaFalla(t *testing.T) {
	// Una bandera mal escrita no puede pasar como si fuera una carpeta.
	codigo, _, errSalida := correr(t, "scan", "--dir", t.TempDir(), "--no-existe")
	if codigo != 1 {
		t.Errorf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "opción desconocida") {
		t.Errorf("stderr = %q, quería que nombre la opción", errSalida)
	}
}

func TestScanKindInvalidoFalla(t *testing.T) {
	// Un typo en --kind tiene que ser un error, no un "te muestro imágenes
	// porque no entendí lo que pediste".
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "a.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "clip.mp4"), "x")

	codigo, _, errSalida := correr(t, "scan", "--dir", dir, "--kind", "videoz")
	if codigo != 1 {
		t.Fatalf("código = %d, quería 1", codigo)
	}
	if !strings.Contains(errSalida, "--kind") {
		t.Errorf("stderr = %q, quería que nombre el flag", errSalida)
	}
	if !strings.Contains(errSalida, "image|video") {
		t.Errorf("stderr = %q, quería que liste los valores válidos", errSalida)
	}
}

func TestScanKindEnMayusculasSeAcepta(t *testing.T) {
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "a.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "clip.mp4"), "x")

	codigo, salida, errSalida := correr(t, "scan", "--dir", dir, "--kind", "VIDEO", "--json")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0 (stderr: %s)", codigo, errSalida)
	}
	var items []scanItem
	if err := json.Unmarshal([]byte(salida), &items); err != nil {
		t.Fatalf("la salida no es JSON: %v (%q)", err, salida)
	}
	if len(items) != 1 || items[0].Type != "video" {
		t.Errorf("items = %+v, quería sólo el mp4", items)
	}
}

func TestScanOrdenaPorNombre(t *testing.T) {
	dir := t.TempDir()
	for _, nombre := range []string{"c.jpg", "a.jpg", "b.jpg"} {
		escribirArchivo(t, filepath.Join(dir, nombre), "x")
	}
	codigo, salida, _ := correr(t, "scan", "--dir", dir)
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	lineas := strings.Fields(salida)
	if len(lineas) != 3 {
		t.Fatalf("listó %d archivos, quería 3", len(lineas))
	}
	if !strings.HasSuffix(lineas[0], "a.jpg") || !strings.HasSuffix(lineas[2], "c.jpg") {
		t.Errorf("orden = %v, quería a, b, c", lineas)
	}
}

func TestScanOrdenaPorFecha(t *testing.T) {
	dir := t.TempDir()
	viejo := filepath.Join(dir, "viejo.jpg")
	escribirArchivo(t, filepath.Join(dir, "viejo.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "nuevo.jpg"), "x")
	base := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(viejo, base, base); err != nil {
		t.Fatal(err)
	}
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--sort", "date")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	lineas := strings.Fields(salida)
	if len(lineas) != 2 {
		t.Fatalf("listó %d archivos, quería 2", len(lineas))
	}
	if !strings.HasSuffix(lineas[0], "nuevo.jpg") {
		t.Errorf("orden = %v, quería el más nuevo primero", lineas)
	}
}

func TestScanOrdenAleatorioNoPierdeArchivos(t *testing.T) {
	dir := arbolDePrueba(t)
	codigo, salida, _ := correr(t, "scan", "--dir", dir, "--sort", "random")
	if codigo != 0 {
		t.Fatalf("código = %d, quería 0", codigo)
	}
	if lineas := strings.Fields(salida); len(lineas) != 4 {
		t.Errorf("listó %d archivos (%v), quería 4", len(lineas), lineas)
	}
}

// arbolDePrueba arma una carpeta con imágenes, un archivo que no es imagen,
// una carpeta oculta y dos niveles de subcarpetas.
func arbolDePrueba(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	escribirArchivo(t, filepath.Join(dir, "a.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "b.png"), "x")
	escribirArchivo(t, filepath.Join(dir, "notes.txt"), "x")
	escribirArchivo(t, filepath.Join(dir, "sub", "c.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "sub", "deep", "d.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, "sub", "deep", "deeper", "e.jpg"), "x")
	escribirArchivo(t, filepath.Join(dir, ".hidden", "f.jpg"), "x")
	return dir
}

func contains(lista []string, want string) bool {
	for _, s := range lista {
		if s == want {
			return true
		}
	}
	return false
}
