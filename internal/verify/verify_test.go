package verify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// claveEfimera genera una clave de prueba en un GNUPGHOME temporal.
// Devuelve el homedir, el fingerprint y la clave pública armada.
func claveEfimera(t *testing.T) (string, string, []byte) {
	t.Helper()
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg no está instalado")
	}
	home := t.TempDir()
	cmd := exec.Command("gpg", "--homedir", home, "--batch",
		"--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "test <test@test.local>", "rsa2048", "sign", "0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generar clave: %v\n%s", err, out)
	}
	fpr := fingerprint(t, home)
	return home, fpr, exportar(t, home)
}

func fingerprint(t *testing.T, home string) string {
	t.Helper()
	out, err := exec.Command("gpg", "--homedir", home, "--list-keys",
		"--with-colons").CombinedOutput()
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

func exportar(t *testing.T, home string) []byte {
	t.Helper()
	out, err := exec.Command("gpg", "--homedir", home, "--armor",
		"--export").Output()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return out
}

func firmar(t *testing.T, home, archivo string) string {
	t.Helper()
	sig := archivo + ".asc"
	cmd := exec.Command("gpg", "--homedir", home, "--batch",
		"--pinentry-mode", "loopback", "--passphrase", "",
		"--armor", "--detach-sign", "-o", sig, archivo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("firmar: %v\n%s", err, out)
	}
	return sig
}

// usarClave sustituye la clave embebida por una efímera durante el test.
func usarClave(t *testing.T, home, fpr string) {
	t.Helper()
	viejoPub, viejoFpr := publicKey, Fingerprint
	publicKey, Fingerprint = exportar(t, home), fpr
	t.Cleanup(func() { publicKey, Fingerprint = viejoPub, viejoFpr })
}

func TestSignatureValida(t *testing.T) {
	home, fpr, _ := claveEfimera(t)
	usarClave(t, home, fpr)

	dir := t.TempDir()
	sums := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(sums, []byte("abc123  b3l0w-linux-amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Signature(sums, firmar(t, home, sums)); err != nil {
		t.Fatalf("Signature: %v", err)
	}
}

func TestSignatureDetectaManipulacion(t *testing.T) {
	home, fpr, _ := claveEfimera(t)
	usarClave(t, home, fpr)

	dir := t.TempDir()
	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte("abc123  b3l0w-linux-amd64\n"), 0o644)
	sig := firmar(t, home, sums)

	// Cambiar el contenido después de firmar es exactamente lo que
	// haría un atacante que controla el release.
	os.WriteFile(sums, []byte("deadbeef  b3l0w-linux-amd64\n"), 0o644)

	if err := Signature(sums, sig); err == nil {
		t.Fatal("esperaba error con el contenido modificado")
	}
}

func TestSignatureRechazaOtraClave(t *testing.T) {
	home, _, _ := claveEfimera(t)
	otroHome, _, _ := claveEfimera(t)
	usarClave(t, home, fingerprint(t, home))

	dir := t.TempDir()
	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte("abc  b3l0w\n"), 0o644)
	sig := firmar(t, otroHome, sums) // firmada por la clave equivocada

	if err := Signature(sums, sig); err == nil {
		t.Fatal("esperaba error con una firma de otra clave")
	}
}

func TestCheckSumCorrecto(t *testing.T) {
	dir := t.TempDir()
	asset := filepath.Join(dir, "b3l0w-linux-amd64")
	os.WriteFile(asset, []byte("contenido real"), 0o755)

	sum, err := SHA256(asset)
	if err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte(sum+"  b3l0w-linux-amd64\n"), 0o644)

	if err := CheckSum(sums, "b3l0w-linux-amd64", asset); err != nil {
		t.Fatalf("CheckSum: %v", err)
	}
}

func TestCheckSumDetectaHashIncorrecto(t *testing.T) {
	dir := t.TempDir()
	asset := filepath.Join(dir, "bin")
	os.WriteFile(asset, []byte("uno"), 0o755)

	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte(strings.Repeat("a", 64)+"  bin\n"), 0o644)

	if err := CheckSum(sums, "bin", asset); err == nil {
		t.Fatal("esperaba error con el hash incorrecto")
	}
}

func TestCheckSumAssetAusente(t *testing.T) {
	dir := t.TempDir()
	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte("abc  otro\n"), 0o644)

	if err := CheckSum(sums, "bin", filepath.Join(dir, "bin")); err == nil {
		t.Fatal("esperaba error con el asset ausente")
	}
}

func TestCheckSumAceptaFormatoBinario(t *testing.T) {
	dir := t.TempDir()
	asset := filepath.Join(dir, "bin")
	os.WriteFile(asset, []byte("x"), 0o755)
	sum, _ := SHA256(asset)

	// sha256sum usa "hash *nombre" en modo binario.
	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte(sum+" *bin\n"), 0o644)

	if err := CheckSum(sums, "bin", asset); err != nil {
		t.Fatalf("CheckSum: %v", err)
	}
}

func TestSumsCompleto(t *testing.T) {
	home, fpr, _ := claveEfimera(t)
	usarClave(t, home, fpr)

	dir := t.TempDir()
	asset := filepath.Join(dir, "b3l0w-linux-amd64")
	os.WriteFile(asset, []byte("binario"), 0o755)
	sum, _ := SHA256(asset)

	sums := filepath.Join(dir, "SHA256SUMS")
	os.WriteFile(sums, []byte(sum+"  b3l0w-linux-amd64\n"), 0o644)
	sig := firmar(t, home, sums)

	if err := Sums(sums, sig, "b3l0w-linux-amd64", asset); err != nil {
		t.Fatalf("Sums: %v", err)
	}

	// Si el asset cambia, el hash ya no coincide aunque la firma sea
	// válida: los dos chequeos son necesarios.
	os.WriteFile(asset, []byte("otro binario"), 0o755)
	if err := Sums(sums, sig, "b3l0w-linux-amd64", asset); err == nil {
		t.Fatal("esperaba error con el asset modificado")
	}
}
