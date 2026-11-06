// Package verify comprueba que un asset venga firmado por la clave de
// release de b3l0w.
//
// La confianza no viene de que el archivo esté en GitHub: viene de la
// firma. Un release comprometido puede cambiar el binario y el
// SHA256SUMS juntos; no puede producir una firma válida sin la clave
// privada, que nunca sale del equipo del mantenedor.
package verify

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clave pública embebida: el binario no depende de configuración
// externa ni del keyring del usuario.
//
//go:embed b3l0w.asc
var publicKey []byte

// PublicKey devuelve la clave pública embebida. La usa el setup para
// importarla al keyring de pacman.
func PublicKey() []byte { return publicKey }

// Fingerprint es la huella de la clave de release. Es pública.
//
// Es var y no const para que los tests puedan sustituirla por una
// clave efímera.
var Fingerprint = "9AE2C00AD3A9A64706A4335CC6E1D74A07833042"

// SetKey sustituye la clave de confianza y devuelve la función que la
// restaura. Es para tests: en producción la clave viene embebida y no
// se toca.
func SetKey(pub []byte, fpr string) (restore func()) {
	viejoPub, viejoFpr := publicKey, Fingerprint
	publicKey, Fingerprint = pub, fpr
	return func() { publicKey, Fingerprint = viejoPub, viejoFpr }
}

// ErrSinFirma explica que el release no trae lo necesario.
var ErrSinFirma = fmt.Errorf("el release no publica SHA256SUMS y su firma")

// SHA256 devuelve el hash de un archivo en hex minúscula.
func SHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// CheckSum verifica que assetPath coincida con la línea de assetName
// en un SHA256SUMS.
//
// Acepta el formato de sha256sum: "hash  nombre" o "hash *nombre".
func CheckSum(sumsPath, assetName, assetPath string) error {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	querido := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		nombre := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(nombre) == assetName {
			querido = strings.ToLower(fields[0])
			break
		}
	}
	if querido == "" {
		return fmt.Errorf("%s no figura en SHA256SUMS", assetName)
	}
	obtenido, err := SHA256(assetPath)
	if err != nil {
		return err
	}
	if obtenido != querido {
		return fmt.Errorf("sha256 de %s no coincide: SHA256SUMS dice %s, el archivo da %s",
			assetName, corto(querido), corto(obtenido))
	}
	return nil
}

// Signature verifica la firma detached de SHA256SUMS contra la clave
// embebida, en un keyring temporal.
//
// El keyring temporal importa: si se usara el del usuario, un keyring
// contaminado podría hacer pasar una firma ajena. Acá solo existe
// nuestra clave.
func Signature(sumsPath, sigPath string) error {
	home, err := os.MkdirTemp("", "b3l0w-gpg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)

	keyPath := filepath.Join(home, "key.asc")
	if err := os.WriteFile(keyPath, publicKey, 0o600); err != nil {
		return err
	}
	if out, err := gpg(home, "--import", keyPath); err != nil {
		return fmt.Errorf("no se pudo importar la clave de b3l0w: %s", primeraLinea(out))
	}

	out, err := gpg(home, "--status-fd", "1", "--verify", sigPath, sumsPath)
	if err != nil {
		return fmt.Errorf("la firma de SHA256SUMS no es válida: %s", primeraLinea(out))
	}
	if !strings.Contains(out, "VALIDSIG "+Fingerprint) {
		return fmt.Errorf("SHA256SUMS está firmado por otra clave, no por %s", Fingerprint)
	}
	return nil
}

// Sums hace la verificación completa: firma de SHA256SUMS y hash del
// asset. Es el único camino que debería usar el instalador.
func Sums(sumsPath, sigPath, assetName, assetPath string) error {
	if err := Signature(sumsPath, sigPath); err != nil {
		return err
	}
	return CheckSum(sumsPath, assetName, assetPath)
}

func gpg(home string, args ...string) (string, error) {
	base := []string{"--homedir", home, "--batch", "--no-tty", "--quiet"}
	cmd := exec.Command("gpg", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func primeraLinea(out string) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "[GNUPG:]"))
		if line != "" && !strings.HasPrefix(line, "gpg:") {
			return line
		}
	}
	if line := strings.TrimSpace(out); line != "" {
		return firstNonEmpty(line)
	}
	return "sin detalle"
}

func firstNonEmpty(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "sin detalle"
}

func corto(hash string) string {
	if len(hash) > 16 {
		return hash[:16] + "…"
	}
	return hash
}
