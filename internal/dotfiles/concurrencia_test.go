package dotfiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Tests de concurrencia del módulo internal/dotfiles.
//
// El módulo no tiene lock (BUGS.md M6) y escribe con un nombre de temporal
// fijo (`<archivo>.tmp`). Estas pruebas fijan las tres consecuencias que
// se pueden observar desde afuera:
//
//  1. Dos Pull simultáneos se pisan el estado: el último save gana y las
//     bases que avanzó el otro desaparecen.
//  2. Dos Pull simultáneos al mismo destino fallan con un error críptico
//     ("rename: no such file or directory").
//  3. Dos Pull simultáneos con contenidos distintos MEZCLAN el archivo del
//     home: queda con el principio de uno y el final del otro.
//
// REGLA DURA: todo en t.TempDir(). Nunca se toca ~/.config real.

// ---------------------------------------------------------------------------
// Helper: un FIFO como temporal, para forzar el solapamiento
// ---------------------------------------------------------------------------

// lectorFifo lee de un FIFO sin bloquear. El open es O_NONBLOCK: si
// nadie abre el FIFO para escribir, el lector no queda trabado. Eso
// importa para el caso del código corregido (temporal único por
// escritura), donde ese FIFO ya no se usa y nadie abre para escribir.
type lectorFifo struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	stop chan struct{}
	fin  chan struct{}
}

// nuevoLectorFifo empieza a leer el FIFO en segundo plano.
func nuevoLectorFifo(fifo string) *lectorFifo {
	l := &lectorFifo{
		stop: make(chan struct{}),
		fin:  make(chan struct{}),
	}
	go func() {
		defer close(l.fin)
		f, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return
		}
		defer f.Close()
		chunk := make([]byte, 32*1024)
		for {
			select {
			case <-l.stop:
				return
			default:
			}
			n, err := f.Read(chunk)
			if n > 0 {
				l.mu.Lock()
				l.buf.Write(chunk[:n])
				l.mu.Unlock()
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				if errors.Is(err, syscall.EAGAIN) {
					time.Sleep(time.Millisecond)
					continue
				}
				return
			}
		}
	}()
	return l
}

// contenido devuelve lo leído hasta ahora.
func (l *lectorFifo) contenido() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.buf.Bytes()...)
}

// cerrar para el lector y espera a que termine.
func (l *lectorFifo) cerrar() {
	close(l.stop)
	<-l.fin
}

// ---------------------------------------------------------------------------
// 1. El estado se pisa (M6, confirmado con evidencia)
// ---------------------------------------------------------------------------

// TestBug_DosPullConcurrentesSePisanElEstado: dos Pull sobre el mismo
// estado, corriendo a la vez.
//
// Consecuencia para el usuario: el pull siguiente ya no sabe qué base
// tenía cada archivo. Cada config cae en la rama 4 (sin base) y aparece
// como conflicto — o, con --force, se pisa.
//
// El fix es un lock (flock) sobre el estado: los Pull se serializan y el
// segundo ve el estado que el primero dejó, en vez de pisarlo.
//
// Nota: el caso de dos repos DISTINTOS sharing el mismo estado no se
// prueba acá a propósito. Podar las bases del otro repo es el
// comportamiento correcto (cambiar de --dotfiles-dir tiene que podar lo
// viejo); lo que no puede pasar es que dos pulls del MISMO repo se pisen.
func TestBug_DosPullConcurrentesSePisanElEstado(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")

	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.toml", "b.toml", "c.toml"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Tres Pull del mismo repo, al mismo tiempo.
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Pull(src, home, statePath, false, false); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Pull: %v", err)
	}

	// El estado final tiene que tener las tres bases: ninguna se perdió
	// por el solapamiento y el JSON quedó entero.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]string
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("el estado quedó corrupto: %v", err)
	}
	for _, name := range []string{"a.toml", "b.toml", "c.toml"} {
		if _, ok := st[name]; !ok {
			t.Fatalf("la base de %s se perdió: dos Pull concurrentes se pisaron el estado (state=%v)", name, st)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Error críptico al escribir el mismo destino a la vez
// ---------------------------------------------------------------------------

// TestBug_DosPullConcurrentesFallanAlEscribirElMismoDestino: dos Pull
// simultáneos sobre el mismo repo y el mismo home escriben el mismo
// archivo. El temporal tiene nombre fijo ("x.toml.tmp"), así que los dos
// writers se pisan: el que renombra primero se lleva el temporal del otro
// y el segundo falla con "rename ...: no such file or directory".
//
// El solapamiento es determinista: el temporal es un FIFO, así que los
// dos writers quedan bloqueados en el open hasta que aparece el lector.
// Cuando aparece, los dos escriben; el primero renombra el FIFO a su
// lugar y el segundo ya no encuentra el temporal.
//
// Ningún Pull debería fallar por escribir el mismo archivo que otro.
func TestBug_DosPullConcurrentesFallanAlEscribirElMismoDestino(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin FIFOs en windows")
	}
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "x.toml"), []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El temporal es un FIFO: los dos writers se bloquean al abrirlo.
	fifo := filepath.Join(home, ".config", "x.toml.tmp")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no se puede crear fifo: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for j := 0; j < 2; j++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			_, errs[k] = Pull(src, home, statePath, false, false)
		}(j)
	}
	// Darle tiempo a los dos a bloquearse en el open del FIFO.
	time.Sleep(300 * time.Millisecond)

	// El lector destraba a los dos.
	lector := nuevoLectorFifo(fifo)
	defer lector.cerrar()

	wg.Wait()

	for k, err := range errs {
		if err != nil {
			t.Fatalf("el Pull %d falló por escribir el mismo archivo que el otro: %v", k, err)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. El contenido del home queda mezclado
// ---------------------------------------------------------------------------

// TestBug_DosPullConcurrentesMezclanElContenido: dos Pull simultáneos
// con contenidos DISTINTOS escriben el mismo archivo del home. Como el
// temporal es de nombre fijo, los dos writers escriben en el mismo
// archivo: el resultado es un archivo con el principio de un repo y el
// final del otro.
//
// El archivo que queda en ~/.config no es ninguna de las dos versiones:
// es un Frankenstein. El estado, además, guarda el hash de una versión
// que no es la que está en disco.
//
// El solapamiento se fuerza de forma determinista: el temporal
// ("x.toml.tmp") es un FIFO, así que los dos writers quedan bloqueados
// esperando al mismo lector. Cuando el lector aparece, los dos escriben
// en el mismo flujo y el contenido final es la mezcla de los dos.
func TestBug_DosPullConcurrentesMezclanElContenido(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin FIFOs en windows")
	}
	root := t.TempDir()
	srcA := filepath.Join(root, "repoA")
	srcB := filepath.Join(root, "repoB")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(srcA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcB, 0o755); err != nil {
		t.Fatal(err)
	}
	largoA := strings.Repeat("A", 400000)
	largoB := strings.Repeat("B", 100000)
	if err := os.WriteFile(filepath.Join(srcA, "x.toml"), []byte(largoA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcB, "x.toml"), []byte(largoB), 0o644); err != nil {
		t.Fatal(err)
	}

	// El temporal es un FIFO: los dos writers se bloquean al abrirlo.
	fifo := filepath.Join(home, ".config", "x.toml.tmp")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("no se puede crear fifo: %v", err)
	}

	var wg sync.WaitGroup
	for j := 0; j < 2; j++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			src := srcA
			if k == 1 {
				src = srcB
			}
			Pull(src, home, statePath, false, false)
		}(j)
	}
	// Darle tiempo a los dos a bloquearse en el open del FIFO.
	time.Sleep(300 * time.Millisecond)

	// El lector: destraba a los dos y se queda con lo que escribieron.
	lector := nuevoLectorFifo(fifo)
	defer lector.cerrar()

	wg.Wait()

	dest := filepath.Join(home, ".config", "x.toml")
	var contenido []byte
	fi, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("no se creó el destino: %v", err)
	}
	if fi.Mode().IsRegular() {
		// Caso del código corregido: el destino es un archivo normal.
		contenido, err = os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// Caso del bug: el FIFO terminó en el destino (el rename lo
		// movió) y lo escrito quedó en el flujo.
		contenido = lector.contenido()
	}

	if string(contenido) != largoA && string(contenido) != largoB {
		preview := contenido
		if len(preview) > 60 {
			preview = preview[:30]
			preview = append(preview, []byte(" ... ")...)
			preview = append(preview, contenido[len(contenido)-30:]...)
		}
		t.Fatalf("el archivo del home no es ninguna de las dos versiones del repo "+
			"(%d bytes, esperaba %d o %d): %q", len(contenido), len(largoA), len(largoB), preview)
	}
}

// ---------------------------------------------------------------------------
// 4. Lo que SÍ se tiene que upholdar
// ---------------------------------------------------------------------------

// TestEdge_ElEstadoQuedaJSONValidoConPullConcurrentes: aun con Pull
// concurrentes, el estado en disco nunca queda truncado ni corrupto. Es
// la invariante de la que depende el guardado atómico.
func TestEdge_ElEstadoQuedaJSONValidoConPullConcurrentes(t *testing.T) {
	for i := 0; i < 10; i++ {
		root := t.TempDir()
		src := filepath.Join(root, "repo")
		home := filepath.Join(root, "home")
		statePath := filepath.Join(root, "estado.json")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, nombre := range []string{"a.toml", "b.toml", "c.toml"} {
			if err := os.WriteFile(filepath.Join(src, nombre), []byte("repo-"+nombre), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		var wg sync.WaitGroup
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				Pull(src, home, statePath, false, false)
			}()
		}
		wg.Wait()

		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("iteración %d: no se pudo leer el estado: %v", i, err)
		}
		var st map[string]string
		if err := json.Unmarshal(data, &st); err != nil {
			t.Fatalf("iteración %d: el estado quedó corrupto: %v (%s)", i, err, data)
		}
	}
}

// TestEdge_ElArchivoDelHomeNoQuedaConContenidoParcial: un Pull aislado
// (sin concurrencia) siempre deja el archivo completo: nunca a medias.
func TestEdge_ElArchivoDelHomeNoQuedaConContenidoParcial(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	statePath := filepath.Join(root, "estado.json")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	contenido := strings.Repeat("contenido-largo-", 50000)
	if err := os.WriteFile(filepath.Join(src, "x.toml"), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(src, home, statePath, false, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, ".config", "x.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contenido {
		t.Fatalf("el archivo quedó parcial: %d bytes de %d", len(got), len(contenido))
	}
}
