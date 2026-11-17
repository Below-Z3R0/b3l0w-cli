package services

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeCPUInfo escribe un /proc/cpuinfo falso y devuelve su ruta.
func fakeCPUInfo(t *testing.T, vendor string, flags string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cpuinfo")
	content := "processor\t: 0\nvendor_id\t: " + vendor + "\nflags\t\t: " + flags + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeGPU crea un cardN/device en el arbol de DRM falso.
//
// El layout es el de /sys/class/drm: cardN/device/{vendor,uevent} y
// mem_info_vram_total cuando el driver lo expone.
func fakeGPU(t *testing.T, root, card, vendorID, driver, pciID string, vram int64) {
	t.Helper()
	dev := filepath.Join(root, card, "device")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "vendor"), []byte(vendorID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uevent := "DRIVER=" + driver + "\nPCI_ID=" + pciID + "\n"
	if err := os.WriteFile(filepath.Join(dev, "uevent"), []byte(uevent), 0o644); err != nil {
		t.Fatal(err)
	}
	if vram > 0 {
		vramFile := filepath.Join(dev, "mem_info_vram_total")
		if err := os.WriteFile(vramFile, []byte(strconv.FormatInt(vram, 10)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDetectHardwareAtCPUVendor lee el vendor del cpuinfo que le pasan.
func TestDetectHardwareAtCPUVendor(t *testing.T) {
	cases := map[string]string{
		"GenuineIntel": "GenuineIntel",
		"AuthenticAMD": "AuthenticAMD",
	}
	for want, cpuinfo := range cases {
		hw := detectHardwareAt(fakeCPUInfo(t, cpuinfo, ""), t.TempDir())
		if hw.CPUVendor != want {
			t.Errorf("CPUVendor = %q, quería %q", hw.CPUVendor, want)
		}
	}
}

// TestDetectHardwareAtCPUInfoIlegible: sin /proc/cpuinfo no hay vendor,
// pero el CLI no se cae.
func TestDetectHardwareAtCPUInfoIlegible(t *testing.T) {
	hw := detectHardwareAt(filepath.Join(t.TempDir(), "no-existe"), t.TempDir())
	if hw.CPUVendor != "" {
		t.Errorf("CPUVendor = %q, quería vacío", hw.CPUVendor)
	}
	if len(hw.CPUFlags) != 0 {
		t.Errorf("CPUFlags = %v, quería vacío", hw.CPUFlags)
	}
}

// TestDetectHardwareAtCPUFlags solo mira avx2 y avx512f.
func TestDetectHardwareAtCPUFlags(t *testing.T) {
	cases := []struct {
		flags string
		want  []string
	}{
		{"fpu vme de pse tsc msr avx avx2", []string{"avx2"}},
		{"fpu vme de pse tsc msr avx avx2 avx512f", []string{"avx2", "avx512f"}},
		{"fpu vme de pse tsc msr", nil},
	}
	for _, c := range cases {
		hw := detectHardwareAt(fakeCPUInfo(t, "GenuineIntel", c.flags), t.TempDir())
		if len(hw.CPUFlags) != len(c.want) {
			t.Errorf("flags %q -> %v, quería %v", c.flags, hw.CPUFlags, c.want)
			continue
		}
		for i, f := range c.want {
			if hw.CPUFlags[i] != f {
				t.Errorf("flags %q -> %v, quería %v", c.flags, hw.CPUFlags, c.want)
				break
			}
		}
	}
}

// TestDetectGPUsAtDetectaCadaVendor arma un arbol de DRM falso y
// comprueba que cada vendor se detecta con su driver.
func TestDetectGPUsAtDetectaCadaVendor(t *testing.T) {
	root := t.TempDir()
	fakeGPU(t, root, "card0", "0x10de", "nvidia", "10DE:2504", 8<<30)
	fakeGPU(t, root, "card1", "0x1002", "amdgpu", "1002:1636", 4<<30)
	fakeGPU(t, root, "card2", "0x8086", "i915", "8086:9A49", 0)

	gpus := detectGPUsAt(root)
	if len(gpus) != 3 {
		t.Fatalf("detectó %d GPUs, quería 3: %+v", len(gpus), gpus)
	}
	// El orden es por ranking: la dedicada NVIDIA primero.
	if gpus[0].Vendor != "nvidia" {
		t.Errorf("la primera GPU es %q, quería nvidia", gpus[0].Vendor)
	}
	if gpus[0].Driver != "nvidia" {
		t.Errorf("driver = %q, quería nvidia", gpus[0].Driver)
	}
	if gpus[0].Name != "2504" {
		t.Errorf("Name = %q, quería 2504", gpus[0].Name)
	}
	if gpus[0].VRAM != 8<<30 {
		t.Errorf("VRAM = %d, quería %d", gpus[0].VRAM, int64(8<<30))
	}
	if !gpus[0].Dedicated {
		t.Error("la NVIDIA no salió dedicada")
	}
	if gpus[1].Vendor != "amd" || gpus[1].Driver != "amdgpu" {
		t.Errorf("segunda GPU = %+v, quería amd/amdgpu", gpus[1])
	}
	if gpus[2].Vendor != "intel" || gpus[2].Driver != "i915" {
		t.Errorf("tercera GPU = %+v, quería intel/i915", gpus[2])
	}
	if gpus[2].Dedicated {
		t.Error("la Intel i915 no es dedicada")
	}
}

// TestDetectGPUsAtIgnoraLoQueNoEsGPU: los conectores (cardN-DP), los
// vendors desconocidos y los card sin device no son GPUs.
func TestDetectGPUsAtIgnoraLoQueNoEsGPU(t *testing.T) {
	root := t.TempDir()
	fakeGPU(t, root, "card0", "0x10de", "nvidia", "10DE:2504", 8<<30)
	// Conector: mismo device, no cuenta.
	fakeGPU(t, root, "card0-DP", "0x10de", "nvidia", "10DE:2504", 8<<30)
	// Vendor desconocido.
	fakeGPU(t, root, "card1", "0x1234", "otro", "1234:5678", 0)
	// Card sin device.
	if err := os.MkdirAll(filepath.Join(root, "card2"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Y algo que no es un card.
	fakeGPU(t, root, "renderD128", "0x10de", "nvidia", "10DE:2504", 8<<30)

	gpus := detectGPUsAt(root)
	if len(gpus) != 1 {
		t.Fatalf("detectó %d GPUs, quería 1: %+v", len(gpus), gpus)
	}
	if gpus[0].Vendor != "nvidia" {
		t.Errorf("GPU = %+v, quería nvidia", gpus[0])
	}
}

// TestDetectGPUsAtSinDirectorio: sin /sys/class/drm no hay GPUs, pero
// el CLI no se cae.
func TestDetectGPUsAtSinDirectorio(t *testing.T) {
	gpus := detectGPUsAt(filepath.Join(t.TempDir(), "no-existe"))
	if len(gpus) != 0 {
		t.Errorf("detectó %v sin directorio", gpus)
	}
}

// TestDetectHardwareAtNoSeComeElHost: DetectHardware() lee el /proc y
// el /sys reales. Solo se comprueba que no se caiga: el contenido
// depende de la maquina.
func TestDetectHardwareAtNoSeComeElHost(t *testing.T) {
	hw := DetectHardware()
	if hw.CPUVendor == "" {
		t.Log("sin vendor de CPU en esta maquina")
	}
	for _, g := range hw.GPUs {
		if g.Vendor == "" {
			t.Errorf("GPU sin vendor: %+v", g)
		}
	}
}

// TestVariantsForDaLaVarianteAceleradaYLaCPU: la CPU siempre esta
// soportada; la acelerada depende del hardware.
func TestVariantsForDaLaVarianteAceleradaYLaCPU(t *testing.T) {
	cases := []struct {
		name       string
		hw         Hardware
		wantFirst  string
		wantProv   string
		wantSecond string
	}{
		{
			name:       "nvidia dedicada",
			hw:         Hardware{GPUs: []GPU{{Vendor: "nvidia", Dedicated: true}}},
			wantFirst:  "cuda",
			wantProv:   "CUDA",
			wantSecond: "cpu",
		},
		{
			name:       "amd dedicada",
			hw:         Hardware{GPUs: []GPU{{Vendor: "amd", Dedicated: true}}},
			wantFirst:  "amd",
			wantProv:   "MIGraphX",
			wantSecond: "cpu",
		},
		{
			name:       "intel arc",
			hw:         Hardware{GPUs: []GPU{{Vendor: "intel", Dedicated: true}}},
			wantFirst:  "intel",
			wantProv:   "OpenVINO",
			wantSecond: "cpu",
		},
		{
			name:       "intel igpu",
			hw:         Hardware{GPUs: []GPU{{Vendor: "intel"}}},
			wantFirst:  "intel",
			wantProv:   "OpenVINO",
			wantSecond: "cpu",
		},
		{
			name:       "amd apu",
			hw:         Hardware{GPUs: []GPU{{Vendor: "amd"}}},
			wantFirst:  "amd",
			wantProv:   "MIGraphX",
			wantSecond: "cpu",
		},
	}
	for _, c := range cases {
		got := VariantsFor(c.hw)
		if len(got) != 2 {
			t.Errorf("%s: %d variantes, quería 2: %+v", c.name, len(got), got)
			continue
		}
		if got[0].Name != c.wantFirst {
			t.Errorf("%s: primera variante %q, quería %q", c.name, got[0].Name, c.wantFirst)
		}
		if got[0].Provider != c.wantProv {
			t.Errorf("%s: provider %q, quería %q", c.name, got[0].Provider, c.wantProv)
		}
		if got[0].Reason == "" {
			t.Errorf("%s: sin motivo", c.name)
		}
		if !got[0].Supported {
			t.Errorf("%s: la variante acelerada no está soportada", c.name)
		}
		if got[1].Name != c.wantSecond {
			t.Errorf("%s: segunda variante %q, quería %q", c.name, got[1].Name, c.wantSecond)
		}
		if got[1].Provider != "CPU" {
			t.Errorf("%s: provider de la CPU %q", c.name, got[1].Provider)
		}
	}
}

// TestVariantsForSinGPUSoloCPU: sin GPU no hay nada que acelerar.
func TestVariantsForSinGPUSoloCPU(t *testing.T) {
	got := VariantsFor(Hardware{})
	if len(got) != 1 {
		t.Fatalf("%d variantes, quería 1: %+v", len(got), got)
	}
	if got[0].Name != "cpu" {
		t.Errorf("variante %q, quería cpu", got[0].Name)
	}
	if !got[0].Supported {
		t.Error("la variante cpu no está soportada")
	}
	if !strings.Contains(got[0].Reason, "sin GPU") {
		t.Errorf("motivo %q, quería que mencione que no hay GPU", got[0].Reason)
	}
}

// TestVariantsForTomaLaMejorGPU: con varias GPUs manda la mejor
// segun el ranking, no la primera de la lista.
//
// La NVIDIA se marca como dedicada: isDedicated() la da siempre
// dedicada, asi que es lo unico que puede producir la deteccion real.
func TestVariantsForTomaLaMejorGPU(t *testing.T) {
	hw := Hardware{GPUs: []GPU{
		{Vendor: "intel"},
		{Vendor: "amd", Dedicated: true},
		{Vendor: "nvidia", Dedicated: true},
	}}
	got := VariantsFor(hw)
	if len(got) == 0 || got[0].Name != "cuda" {
		t.Errorf("variantes = %+v, quería cuda primero", got)
	}
}

// TestGPURankOrdenaPorDedicated: el ranking ordena por vendor y por
// dedicada/integrada.
//
// Ojo con la NVIDIA: el ranking solo la premia si viene marcada como
// dedicada. isDedicated() la marca siempre asi que la deteccion real
// nunca produce una NVIDIA con Dedicated=false, pero el ranking
// depende de ahi.
func TestGPURankOrdenaPorDedicated(t *testing.T) {
	cases := []struct {
		gpu  GPU
		want int
	}{
		{GPU{Vendor: "nvidia", Dedicated: true}, 100},
		{GPU{Vendor: "amd", Dedicated: true}, 90},
		{GPU{Vendor: "intel", Dedicated: true}, 80},
		{GPU{Vendor: "intel"}, 70},
		{GPU{Vendor: "amd"}, 60},
		{GPU{Vendor: "nvidia"}, 0},
		{GPU{Vendor: ""}, 0},
	}
	for _, c := range cases {
		if got := gpuRank(c.gpu); got != c.want {
			t.Errorf("gpuRank(%+v) = %d, quería %d", c.gpu, got, c.want)
		}
	}
}

// TestRecommendSinVariantesPublicadasFalla: sin catálogo no hay nada
// que recomendar.
func TestRecommendSinVariantesPublicadasFalla(t *testing.T) {
	_, _, err := Recommend(Hardware{}, nil)
	if err == nil {
		t.Fatal("recomendó sin variantes publicadas")
	}
	if !strings.Contains(err.Error(), "no hay variantes publicadas") {
		t.Errorf("mensaje inesperado: %v", err)
	}
}

// TestRecommendAvisaSiElBuildNoEstaPublicado: si la variante que el
// hardware pide no esta publicada, el motivo lo dice.
func TestRecommendAvisaSiElBuildNoEstaPublicado(t *testing.T) {
	hw := Hardware{GPUs: []GPU{{Vendor: "nvidia", Dedicated: true}}}
	got, reason, err := Recommend(hw, []string{"intel"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cuda" {
		t.Errorf("recomendó %q, quería cuda", got)
	}
	if !strings.Contains(reason, "falta el build") {
		t.Errorf("motivo %q, quería que avise que falta el build", reason)
	}
}

// TestRecommendUsaLaCPUCuandoNoHayAceleracion: con GPU AMD integrada y
// solo la CPU publicada, la CPU es la que va.
func TestRecommendUsaLaCPUCuandoNoHayAceleracion(t *testing.T) {
	hw := Hardware{GPUs: []GPU{{Vendor: "amd"}}}
	got, _, err := Recommend(hw, []string{"cpu"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cpu" {
		t.Errorf("recomendó %q, quería cpu", got)
	}
}
