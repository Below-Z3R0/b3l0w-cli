// Package dotfiles aplica los TOML del repo de distribución sin pisar
// los cambios locales.
//
// El estado guarda el sha256 de la última versión aplicada por archivo.
// Con eso se distingue "no lo tocó" de "lo editó":
//
//	local == base  -> el repo manda, se actualiza
//	remote == base -> solo cambió el local, se conserva
//	ambos cambian  -> conflicto, no se toca y se escribe <archivo>.new
//
// --force no cambia la lógica 3-way: solo resuelve los conflictos reales
// (cambió el local y cambió el repo) a favor del repo. Un archivo que
// solo editó el usuario se conserva aunque venga --force.
//
// Un archivo que no se puede aplicar (permisos, disco lleno, el destino
// es un directorio) no aborta el pull: el error se anota y el resto del
// repo se sigue aplicando. Los errores se devuelven juntos al final.
package dotfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Result cuenta qué pasó con cada archivo.
type Result struct {
	Updated   []string
	Kept      []string
	Conflicts []string
	Missing   []string
	// Orphaned: estaban en el estado pero ya no están en el repo. Se
	// reportan; el archivo local nunca se borra solo.
	Orphaned []string
	// ForceOverwrites: archivos cuyo contenido local reemplaza un pull
	// con --force. Sirve para anunciar el número real antes de pisar.
	ForceOverwrites []string
	Changed         bool
}

type state map[string]string // ruta relativa -> sha256 aplicado

// Pull aplica srcDir sobre ~/.config.
//
// force resuelve a favor del repo solo los conflictos reales. dryRun no
// escribe nada: ni archivos ni estado.
func Pull(srcDir, homeDir, statePath string, force, dryRun bool) (Result, error) {
	var res Result

	// Lock sobre el estado: el estado es un read-modify-write (se lee
	// al principio y se guarda por cada archivo). Sin lock, dos Pull
	// simultaneos se pisan y las bases que avanzó uno desaparecen:
	// el pull siguiente ve todo como "sin base" y reporta conflictos
	// falsos (o, con --force, pisa en masa).
	unlock, err := lockState(statePath)
	if err != nil {
		return res, err
	}
	defer unlock()

	st, err := loadState(statePath)
	if err != nil {
		return res, err
	}

	// persistir guarda el estado después de cada archivo aplicado: si
	// una escritura falla a mitad, los archivos ya escritos no quedan
	// con la base vieja (que dispararía conflictos falsos al siguiente pull).
	persistir := func() error {
		if dryRun {
			return nil
		}
		return saveState(statePath, st)
	}

	// seen: rutas relativas presentes en el repo. Al final se usa para
	// podar del estado las entradas que quedaron huérfanas. Incluye los
	// archivos que skip() descarta: un .md del repo no se despliega,
	// pero si estaba en el estado no es un huérfano.
	seen := make(map[string]bool)
	// applied: los archivos que realmente se aplican. El guard de repo
	// vacío mira ESTE, no seen: un repo con un solo README.md (o solo
	// .git/) no tiene nada que aplicar y no debe podar el estado.
	applied := make(map[string]bool)

	// fallos: los errores por archivo. Un archivo que no se puede
	// aplicar (permisos, disco lleno, el destino es un directorio) no
	// aborta el pull entero: se anota el fallo, el walk sigue con el
	// resto y los errores se devuelven juntos al final.
	var fallos []error
	notar := func(quien string, err error) {
		fallos = append(fallos, fmt.Errorf("%s: %w", quien, err))
	}
	// vistaIncompleta: el walk no pudo ver todo el repo (un directorio
	// ilegible, por ejemplo). Si no se vio todo, la lista de "vistos"
	// no es confiable y no se puede podar el estado: hacerlo borraría
	// las bases de archivos que el repo sigue teniendo.
	vistaIncompleta := false

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// El repo entero ilegible es fatal: no hay nada que
			// aplicar. Un directorio suelto ilegible no: se anota y
			// el walk sigue con el resto.
			if path == srcDir {
				return err
			}
			vistaIncompleta = true
			notar(path, err)
			return nil
		}
		if info.IsDir() {
			// Los directorios de control de versiones se saltan
			// enteros: no se lee objeto por objeto para descartarlo
			// después. El root nunca: que el repo esté dentro de un
			// ".git" no vacía el repo.
			if path != srcDir && vcsDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		seen[rel] = true
		if skip(rel) {
			return nil
		}

		// Solo se aplican archivos regulares. Un FIFO, un socket o un
		// device bloquearían ReadFile para siempre; un symlink a un
		// directorio o roto lo haría fallar. Se ignoran sin abortar el
		// pull entero. Para un symlink a un archivo regular se usa el
		// modo del target, no el 0777 del symlink.
		src, serr := os.Stat(path)
		if serr != nil || !src.Mode().IsRegular() {
			return nil
		}
		srcMode := src.Mode()
		// Cuenta como aplicado recién ahora: un repo donde nada se
		// puede aplicar (solo FIFOs, symlinks rotos) tampoco puede
		// podar el estado.
		applied[rel] = true

		remote, err := os.ReadFile(path)
		if err != nil {
			notar(rel, err)
			return nil
		}
		remoteHash := hash(remote)
		dest := filepath.Join(homeDir, ".config", rel)

		// conflicto deja el remoto al lado como <archivo>.new para que
		// el usuario lo mergee, y avanza la base para que no se repita
		// en cada pull. El local queda intacto.
		conflicto := func() error {
			res.Conflicts = append(res.Conflicts, rel)
			if dryRun {
				return nil
			}
			if err := writeFile(dest+".new", remote, srcMode); err != nil {
				notar(rel, err)
				return nil
			}
			st[rel] = remoteHash
			return persistir()
		}

		local, lerr := os.ReadFile(dest)
		localExists := lerr == nil
		if lerr != nil && !errors.Is(lerr, fs.ErrNotExist) {
			// El destino existe pero no se puede leer (permisos, o es
			// un directorio). No se puede calcular el 3-way: se
			// conserva lo que hay y se reporta como conflicto, en vez
			// de pisarlo creyendo que no existe.
			return conflicto()
		}
		localHash := hash(local)
		baseHash := st[rel]

		// aplicar escribe el remoto sobre el destino, deja la base al
		// día y lo anota en la lista. Con backup, deja antes un .bak
		// del contenido local.
		//
		// En dry-run solo anota: no escribe nada. Un error al escribir
		// el archivo no aborta el pull: se anota y el walk sigue con el
		// resto. Un error al guardar el estado sí aborta: sin base al
		// día, seguir dejaría las bases desfasadas.
		aplicar := func(lista *[]string, backup bool) error {
			if dryRun {
				*lista = append(*lista, rel)
				return nil
			}
			if backup {
				// El backup va antes de pisar: el .new no alcanza,
				// porque el conflicto no se reporta cuando --force
				// lo resuelve en silencio. Sin esto, el trabajo local
				// se pierde para siempre.
				if err := backupLocal(dest, local); err != nil {
					notar(rel, err)
					return nil
				}
			}
			if err := writeFile(resolveDest(dest), remote, srcMode); err != nil {
				notar(rel, err)
				return nil
			}
			st[rel] = remoteHash
			if err := persistir(); err != nil {
				return err
			}
			res.Changed = true
			*lista = append(*lista, rel)
			return nil
		}

		switch {
		case !localExists && baseHash != "":
			// Había base y el local no está: el usuario borró el archivo.
			// Borrado intencional, no se resucita.
			res.Kept = append(res.Kept, rel)
			return nil

		case !localExists:
			// Nunca existió: se instala.
			return aplicar(&res.Missing, false)

		case baseHash == "":
			// Sin historial no hay forma de saber quién cambió: si es
			// idéntico se adopta, si no es conflicto.
			if localHash == remoteHash {
				st[rel] = remoteHash
				if err := persistir(); err != nil {
					return err
				}
				res.Kept = append(res.Kept, rel)
				return nil
			}
			res.ForceOverwrites = append(res.ForceOverwrites, rel)
			if force {
				// Esta config nunca estuvo gestionada: el usuario no
				// sabe que existe. El backup va antes de pisarla.
				return aplicar(&res.Updated, true)
			}
			return conflicto()

		case localHash == baseHash:
			// El usuario no lo tocó: el repo manda. Esto pasa con y sin
			// --force, así que no cuenta como overwrite de --force.
			if remoteHash == localHash {
				return nil
			}
			return aplicar(&res.Updated, false)

		case remoteHash == baseHash:
			// Solo cambió el local: se conserva, incluso con --force.
			res.Kept = append(res.Kept, rel)
			return nil

		case localHash == remoteHash:
			// Local y repo llegaron al mismo contenido: no hay
			// conflicto real, solo hay que avanzar la base. Es el caso
			// "ambos editaron lo mismo" de un merge 3-way.
			st[rel] = remoteHash
			if err := persistir(); err != nil {
				return err
			}
			res.Kept = append(res.Kept, rel)
			return nil

		default:
			// Ambos cambiaron: conflicto real.
			res.ForceOverwrites = append(res.ForceOverwrites, rel)
			if force {
				// Con --force el conflicto se resuelve en silencio: no
				// queda .new. El backup deja la resolución reversible.
				return aplicar(&res.Updated, true)
			}
			return conflicto()
		}
	})
	if err != nil {
		return res, err
	}

	// Podar el estado: lo que ya no está en el repo queda huérfano. Se
	// reporta, pero el archivo local no se borra automáticamente.
	//
	// Si el walk no aplicó ningún archivo (srcDir vacío, clone
	// incompleto, --dotfiles-dir apuntando a otro lado) no se poda:
	// borrar el estado entero convertiría el próximo pull en una
	// batería de conflictos y, con --force, en pérdida de trabajo local.
	// Mejor no tocar nada.
	//
	// Lo mismo si la vista del repo quedó incompleta: la lista de
	// "vistos" no es confiable y podar borraría bases que el repo
	// sigue teniendo.
	if len(applied) > 0 && !vistaIncompleta {
		var orphans []string
		for rel := range st {
			if !seen[rel] {
				orphans = append(orphans, rel)
			}
		}
		sort.Strings(orphans)
		res.Orphaned = orphans
		if len(orphans) > 0 {
			for _, rel := range orphans {
				delete(st, rel)
			}
			if err := persistir(); err != nil {
				return res, err
			}
		}
	}

	// Los fallos por archivo se devuelven juntos: el pull aplicó todo
	// lo que pudo y el error dice qué quedó sin aplicar.
	if len(fallos) > 0 {
		return res, fmt.Errorf("no se pudieron aplicar %d archivo(s): %w",
			len(fallos), errors.Join(fallos...))
	}

	return res, nil
}

// Adopt registra el contenido LOCAL actual como base de cada archivo que
// ya existe en el home, sin tocar nada. Sirve para la primera corrida:
// evita que todo sea conflicto.
//
// Ojo: adopta el local como base, no lo congela. Si el repo difiere, el
// siguiente Pull lo trata como "no editado" y aplica el del repo (rama 5).
//
// dryRun (opcional) evita escribir el estado.
func Adopt(srcDir, homeDir, statePath string, dryRun ...bool) (int, error) {
	st, err := loadState(statePath)
	if err != nil {
		return 0, err
	}
	count := 0
	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != srcDir && vcsDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if skip(rel) {
			return nil
		}
		dest := filepath.Join(homeDir, ".config", rel)
		if _, err := os.Stat(dest); err != nil {
			return nil // no existe local: se instalará en el próximo pull
		}
		// Solo archivos regulares y legibles: un directorio, un FIFO o
		// un archivo sin permisos no se puede adoptar y no debe abortar
		// la adopción de los demás.
		destInfo, serr := os.Stat(dest)
		if serr != nil || !destInfo.Mode().IsRegular() {
			return nil
		}
		local, rerr := os.ReadFile(dest)
		if rerr != nil {
			return nil
		}
		// Adopt "solo agrega": no pisa una base existente. Si la pisara,
		// una edición local pasaría a ser "no editada" y el próximo pull
		// la perdería en silencio.
		if _, yaAdoptado := st[rel]; yaAdoptado {
			return nil
		}
		// La base es el contenido local actual: desde acá el 3-way
		// normal decide. Si el repo difiere, el próximo pull lo aplica.
		st[rel] = hash(local)
		count++
		return nil
	})
	if walkErr != nil {
		return count, walkErr
	}
	if len(dryRun) > 0 && dryRun[0] {
		return count, nil
	}
	// Se devuelve el conteo real aunque falle el guardado: el llamador
	// puede informar cuántos archivos se adoptaron antes del error.
	if err := saveState(statePath, st); err != nil {
		return count, err
	}
	return count, nil
}

// skip deja fuera lo que no es configuración de usuario.
func skip(rel string) bool {
	base := filepath.Base(rel)
	// El interior de un repo de control de versiones no es un dotfile:
	// sin esto el .git/ entero se despliega en ~/.config/.git/. Se
	// compara por segmento de la ruta, no por prefijo: un .git
	// anidado (sub/.git/) también se descarta.
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if vcsDir(seg) {
			return true
		}
	}
	if base == "README.md" || strings.HasSuffix(base, ".md") {
		return true
	}
	if strings.Contains(rel, "templates/hermes") {
		return true
	}
	if base == "outputs.toml" {
		return true
	}
	return false
}

// vcsDir dice si un nombre es el de un directorio de control de
// versiones. Esos directorios no son configuración del usuario.
func vcsDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn":
		return true
	}
	return false
}

// writeFile escribe data en path. Si el destino existe conserva su modo
// (un 0600 no puede quedar 0644 después de un update); si no, usa el modo
// del origen.
func writeFile(path string, data []byte, srcMode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := srcMode.Perm()
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	// Temporal unico por escritura. Con un nombre fijo (path + ".tmp")
	// dos escrituras simultaneas escribian en el mismo archivo: el
	// resultado era un Frankenstein (la mitad de una version y la mitad
	// de la otra), y ademas un ".tmp" que el usuario ya tenia se
	// truncaba y desaparecia con el rename.
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op si el rename salio bien

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// El modo va en el temporal: el destino todavia no existe.
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

// backupLocal deja una copia del contenido local antes de pisarlo.
// Se usa cuando --force pisa un archivo que el usuario nunca vio
// gestionado: sin este backup, ese trabajo local se pierde en silencio.
func backupLocal(dest string, content []byte) error {
	if len(content) == 0 {
		return nil
	}
	backup := dest + ".bak"
	// Un .bak previo se sobreescribe: el último es el que importa.
	return writeFile(backup, content, 0o644)
}

// resolveDest sigue los symlinks del destino: escribir encima del link
// lo reemplazaría por un archivo regular y dejaría el target huérfano.
func resolveDest(dest string) string {
	if r, err := filepath.EvalSymlinks(dest); err == nil {
		return r
	}
	// Link roto: se sigue a mano.
	if link, err := os.Readlink(dest); err == nil {
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(dest), link)
		}
		return filepath.Clean(link)
	}
	// El destino no existe: se resuelve el padre.
	dir, base := filepath.Split(dest)
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(r, base)
	}
	return dest
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// lockState toma un lock exclusivo sobre el archivo de estado y
// devuelve la funcion que lo libera.
//
// El lock vive en un archivo aparte (statePath + ".lock") para no
// interferir con el rename atomico del estado. Usa flock(2): si otro
// proceso ya lo tiene, espera.
func lockState(statePath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(statePath+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("no se pudo tomar el lock del estado: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("no se pudo tomar el lock del estado: %w", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func loadState(path string) (state, error) {
	st := state{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		// Si el estado no se puede leer (es un directorio, sin
		// permisos), el error tiene que decir como recuperarse: si no,
		// el usuario ve "is a directory" y no sabe que hacer.
		return nil, fmt.Errorf("no se pudo leer el estado de dotfiles (%s): %w; borralo para regenerarlo", path, err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("estado de dotfiles inválido (%s): %w; borralo para regenerarlo", path, err)
	}
	// JSON "null" es válido y deja el map en nil. Asignar sobre un map
	// nil panickea: se normaliza a estado vacío, igual que "no existe".
	if st == nil {
		st = state{}
	}
	return st, nil
}

func saveState(path string, st state) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if st == nil {
		st = state{}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Escritura atómica: tmp en el mismo directorio + rename. Si el
	// proceso muere a mitad, el estado anterior sigue entero; nunca queda
	// un JSON truncado que bloquee todos los pulls siguientes.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dotfiles-*.json.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op si el rename salió bien
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
