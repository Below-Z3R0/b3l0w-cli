// Package dotfiles aplica los TOML del repo de distribución sin pisar
// los cambios locales.
//
// El estado guarda el sha256 de la última versión aplicada por archivo.
// Con eso se distingue "no lo tocó" de "lo editó":
//
//	local == base  -> el repo manda, se actualiza
//	remote == base -> solo cambió el local, se conserva
//	ambos cambian  -> conflicto, no se toca y se escribe <archivo>.new
package dotfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Result cuenta qué pasó con cada archivo.
type Result struct {
	Updated   []string
	Kept      []string
	Conflicts []string
	Missing   []string
	Changed   bool
}

type state map[string]string // ruta relativa -> sha256 aplicado

// Pull aplica srcDir sobre ~/.config.
//
// force pisa todo (sin 3-way). dryRun no escribe nada.
func Pull(srcDir, homeDir, statePath string, force, dryRun bool) (Result, error) {
	var res Result
	stateDirty := false
	st, err := loadState(statePath)
	if err != nil {
		return res, err
	}

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
		if skip(rel) {
			return nil
		}

		remote, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		remoteHash := hash(remote)
		dest := filepath.Join(homeDir, ".config", rel)

		local, err := os.ReadFile(dest)
		localExists := err == nil
		localHash := hash(local)
		baseHash := st[rel]

		switch {
		case !localExists:
			res.Missing = append(res.Missing, rel)
			if !dryRun {
				if err := writeFile(dest, remote); err != nil {
					return err
				}
				st[rel] = remoteHash
				res.Changed = true
			}
			return nil

		case force:
			res.Updated = append(res.Updated, rel)
			if !dryRun {
				if err := writeFile(dest, remote); err != nil {
					return err
				}
				st[rel] = remoteHash
				res.Changed = true
			}
			return nil

		case baseHash == "":
			// Primera vez sin estado: si el local es idéntico, se adopta;
			// si difiere, no se pisa.
			if localHash == remoteHash {
				st[rel] = remoteHash
				stateDirty = true
				res.Kept = append(res.Kept, rel)
				return nil
			}
			res.Conflicts = append(res.Conflicts, rel)
			if !dryRun {
				if err := writeFile(dest+".new", remote); err != nil {
					return err
				}
			}
			return nil

		case localHash == baseHash:
			// No lo tocó: el repo manda, salvo que ya sean iguales.
			if remoteHash == localHash {
				return nil
			}
			res.Updated = append(res.Updated, rel)
			if !dryRun {
				if err := writeFile(dest, remote); err != nil {
					return err
				}
				st[rel] = remoteHash
				res.Changed = true
			}
			return nil

		case remoteHash == baseHash:
			// Solo cambió el local: se conserva.
			res.Kept = append(res.Kept, rel)
			return nil

		default:
			// Ambos cambiaron: no se pisa.
			res.Conflicts = append(res.Conflicts, rel)
			if !dryRun {
				if err := writeFile(dest+".new", remote); err != nil {
					return err
				}
			}
			return nil
		}
	})
	if err != nil {
		return res, err
	}

	if !dryRun && (res.Changed || stateDirty) {
		if err := saveState(statePath, st); err != nil {
			return res, err
		}
	}
	return res, nil
}

// Adopt registra el hash actual de cada archivo local como base, sin
// tocar nada. Sirve para la primera corrida: evita que todo sea conflicto.
func Adopt(srcDir, homeDir, statePath string) (int, error) {
	st, err := loadState(statePath)
	if err != nil {
		return 0, err
	}
	count := 0
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
		if skip(rel) {
			return nil
		}
		dest := filepath.Join(homeDir, ".config", rel)
		data, err := os.ReadFile(dest)
		if err != nil {
			return nil // no existe: se instalará en el próximo pull
		}
		st[rel] = hash(data)
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	if err := saveState(statePath, st); err != nil {
		return 0, err
	}
	return count, nil
}

// skip deja fuera lo que no es configuración de usuario.
func skip(rel string) bool {
	base := filepath.Base(rel)
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

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
		return nil, fmt.Errorf("estado de dotfiles inválido: %w", err)
	}
	return st, nil
}

func saveState(path string, st state) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// copyFile existe para los tests y para un futuro --adopt.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
