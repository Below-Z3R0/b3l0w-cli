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
	// podar del estado las entradas que quedaron huérfanas.
	seen := make(map[string]bool)

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
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

		remote, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		remoteHash := hash(remote)
		dest := filepath.Join(homeDir, ".config", rel)

		local, lerr := os.ReadFile(dest)
		localExists := lerr == nil
		if lerr != nil && !errors.Is(lerr, fs.ErrNotExist) {
			// El destino existe pero no se puede leer (permisos, o es
			// un directorio). No se puede calcular el 3-way: se
			// conserva lo que hay y se reporta como conflicto, en vez
			// de pisarlo creyendo que no existe.
			res.Conflicts = append(res.Conflicts, rel)
			if dryRun {
				return nil
			}
			if err := writeFile(dest+".new", remote, srcMode); err != nil {
				return err
			}
			st[rel] = remoteHash
			return persistir()
		}
		localHash := hash(local)
		baseHash := st[rel]

		// escribir aplica el remoto sobre el destino: conserva el modo
		// del archivo local, no rompe symlinks y deja la base al día.
		escribir := func() error {
			if err := writeFile(resolveDest(dest), remote, srcMode); err != nil {
				return err
			}
			st[rel] = remoteHash
			if err := persistir(); err != nil {
				return err
			}
			res.Changed = true
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
			if dryRun {
				res.Missing = append(res.Missing, rel)
				return nil
			}
			if err := escribir(); err != nil {
				return err
			}
			res.Missing = append(res.Missing, rel)
			return nil

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
				res.Updated = append(res.Updated, rel)
				if dryRun {
					return nil
				}
				// Esta config nunca estuvo gestionada: el usuario no
				// sabe que existe. Antes de pisarla se deja un backup,
				// porque aca el .new no alcanza (el conflicto no se
				// reportó: --force lo resolvio en silencio).
				if err := backupLocal(dest, local); err != nil {
					return err
				}
				return escribir()
			}
			res.Conflicts = append(res.Conflicts, rel)
			if dryRun {
				return nil
			}
			if err := writeFile(dest+".new", remote, srcMode); err != nil {
				return err
			}
			// El conflicto ya se reportó: se avanza la base para que no
			// se repita en cada pull.
			st[rel] = remoteHash
			return persistir()

		case localHash == baseHash:
			// El usuario no lo tocó: el repo manda. Esto pasa con y sin
			// --force, así que no cuenta como overwrite de --force.
			if remoteHash == localHash {
				return nil
			}
			res.Updated = append(res.Updated, rel)
			if dryRun {
				return nil
			}
			return escribir()

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
				res.Updated = append(res.Updated, rel)
				if dryRun {
					return nil
				}
				// Esta config nunca estuvo gestionada: el usuario no
				// sabe que existe. Antes de pisarla se deja un backup,
				// porque aca el .new no alcanza (el conflicto no se
				// reportó: --force lo resolvio en silencio).
				if err := backupLocal(dest, local); err != nil {
					return err
				}
				return escribir()
			}
			res.Conflicts = append(res.Conflicts, rel)
			if dryRun {
				return nil
			}
			if err := writeFile(dest+".new", remote, srcMode); err != nil {
				return err
			}
			// El conflicto ya se reportó: se avanza la base para que no
			// se repita en cada pull.
			st[rel] = remoteHash
			return persistir()
		}
	})
	if err != nil {
		return res, err
	}

	// Podar el estado: lo que ya no está en el repo queda huérfano. Se
	// reporta, pero el archivo local no se borra automáticamente.
	//
	// Si el walk no vio ningún archivo (srcDir vacío, clone incompleto,
	// --dotfiles-dir apuntando a otro lado) no se poda: borrar el estado
	// entero convertiría el próximo pull en una batería de conflictos y,
	// con --force, en pérdida de trabajo local. Mejor no tocar nada.
	if len(seen) == 0 {
		return res, nil
	}
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
	// El interior de un repo git no es un dotfile: sin esto el .git/
	// entero se despliega en ~/.config/.git/.
	if base == ".git" {
		return true
	}
	if strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
		return true
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	// WriteFile aplica umask: se fuerza el modo exacto.
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	// Un rename fallido (destino que es un directorio, otro filesystem,
	// permisos) no puede dejar el .tmp tirado en ~/.config.
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
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

func loadState(path string) (state, error) {
	st := state{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, err
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
