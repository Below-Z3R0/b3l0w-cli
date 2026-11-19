package selfupdate

import "testing"

// TestNumerosCuatroComponentes fija que el cuarto componente cuenta.
func TestNumerosCuatroComponentes(t *testing.T) {
	if compareVersions("1.2.3.4", "1.2.3.5") >= 0 {
		t.Error("1.2.3.5 tiene que ser más nueva que 1.2.3.4")
	}
}

// TestWithVNoTocaLoQueNoEsVersion fija que "dev" no se vuelve "vdev".
func TestWithVNoTocaLoQueNoEsVersion(t *testing.T) {
	if got := withV("dev"); got != "dev" {
		t.Errorf("withV(dev) = %q, quería dev", got)
	}
	if got := withV(""); got != "" {
		t.Errorf("withV(\"\") = %q", got)
	}
}
