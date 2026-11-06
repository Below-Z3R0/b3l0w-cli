package programs

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/verify"
)

// TestVerifyReleaseCompleto ejercita el camino real: servidor HTTP,
// asset, SHA256SUMS firmado y la clave pública de confianza.
func TestVerifyReleaseCompleto(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg no está instalado")
	}

	home := t.TempDir()
	generar := exec.Command("gpg", "--homedir", home, "--batch",
		"--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "test <test@test.local>", "rsa2048", "sign", "0")
	if out, err := generar.CombinedOutput(); err != nil {
		t.Fatalf("generar clave: %v\n%s", err, out)
	}
	fpr := leerFingerprint(t, home)
	pub := exportarClave(t, home)
	restore := verify.SetKey(pub, fpr)
	defer restore()

	asset := []byte("binario de prueba")
	sum := sha256.Sum256(asset)
	sums := hex.EncodeToString(sum[:]) + "  b3l0w-linux-amd64\n"

	dir := t.TempDir()
	sumsPath := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(sumsPath, []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	firmar := exec.Command("gpg", "--homedir", home, "--batch",
		"--pinentry-mode", "loopback", "--passphrase", "",
		"--armor", "--detach-sign", "-o", sumsPath+".asc", sumsPath)
	if out, err := firmar.CombinedOutput(); err != nil {
		t.Fatalf("firmar: %v\n%s", err, out)
	}
	sig, err := os.ReadFile(sumsPath + ".asc")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "SHA256SUMS":
			w.Write([]byte(sums))
		case "SHA256SUMS.asc":
			w.Write(sig)
		case "b3l0w-linux-amd64":
			w.Write(asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	assetPath := filepath.Join(dir, "b3l0w-linux-amd64")
	if err := os.WriteFile(assetPath, asset, 0o755); err != nil {
		t.Fatal(err)
	}

	url := srv.URL + "/b3l0w-linux-amd64"
	if err := VerifyRelease(url, "b3l0w-linux-amd64", assetPath); err != nil {
		t.Fatalf("VerifyRelease: %v", err)
	}

	// Un asset distinto con las mismas sums ya no pasa.
	if err := os.WriteFile(assetPath, []byte("otro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRelease(url, "b3l0w-linux-amd64", assetPath); err == nil {
		t.Fatal("esperaba error con el asset cambiado")
	}
}

func leerFingerprint(t *testing.T, home string) string {
	t.Helper()
	out, err := exec.Command("gpg", "--homedir", home, "--list-keys", "--with-colons").CombinedOutput()
	if err != nil {
		t.Fatalf("list-keys: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "fpr:") {
			return strings.Split(line, ":")[9]
		}
	}
	t.Fatal("sin fingerprint")
	return ""
}

func exportarClave(t *testing.T, home string) []byte {
	t.Helper()
	out, err := exec.Command("gpg", "--homedir", home, "--armor", "--export").Output()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return out
}
