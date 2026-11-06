package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// GPU describe una GPU detectada en el host.
type GPU struct {
	Vendor    string // nvidia | amd | intel
	Driver    string // i915 | xe | nvidia | amdgpu
	Name      string
	VendorID  string
	Dedicated bool
	VRAM      int64 // bytes; 0 si no se pudo leer
}

// Hardware es el resumen del hardware del host.
type Hardware struct {
	CPUVendor string
	CPUFlags  []string
	GPUs      []GPU
}

// Variant es una variante de imagen y si el host la soporta.
type Variant struct {
	Name      string
	Supported bool
	Reason    string
	Provider  string
}

// Rank de vendor para elegir la mejor GPU.
func gpuRank(g GPU) int {
	switch {
	case g.Vendor == "nvidia" && g.Dedicated:
		return 100
	case g.Vendor == "amd" && g.Dedicated:
		return 90
	case g.Vendor == "intel" && g.Dedicated:
		return 80
	case g.Vendor == "intel":
		return 70
	case g.Vendor == "amd":
		return 60
	}
	return 0
}

// DetectHardware lee el hardware del host sin binarios externos.
func DetectHardware() Hardware {
	hw := Hardware{CPUVendor: cpuVendor(), CPUFlags: cpuFlags()}
	hw.GPUs = detectGPUs()
	return hw
}

func cpuVendor() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "vendor_id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func cpuFlags() []string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "flags") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		flags := strings.Fields(parts[1])
		var out []string
		for _, want := range []string{"avx2", "avx512f"} {
			for _, f := range flags {
				if f == want {
					out = append(out, want)
				}
			}
		}
		return out
	}
	return nil
}

// detectGPUs recorre /sys/class/drm/card*/device.
func detectGPUs() []GPU {
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return nil
	}
	var out []GPU
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "card") {
			continue
		}
		// cardN y cardN-DP son el mismo device: solo el primero cuenta.
		if strings.Contains(name, "-") {
			continue
		}
		// cardN es un symlink: IsDir() da false para symlinks.
		dev := filepath.Join("/sys/class/drm", name, "device")
		if info, err := os.Stat(dev); err != nil || !info.IsDir() {
			continue
		}
		gpu := GPU{
			VendorID: readFile(filepath.Join(dev, "vendor")),
			Driver:   ueventDriver(dev),
		}
		gpu.Vendor = vendorFromID(gpu.VendorID)
		gpu.Name = gpuName(dev)
		gpu.VRAM = readVRAM(dev)
		gpu.Dedicated = isDedicated(gpu)
		if gpu.Vendor == "" {
			continue
		}
		out = append(out, gpu)
	}
	sort.Slice(out, func(i, j int) bool { return gpuRank(out[i]) > gpuRank(out[j]) })
	return out
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func ueventDriver(dev string) string {
	data, err := os.ReadFile(filepath.Join(dev, "uevent"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "DRIVER=") {
			return strings.TrimPrefix(line, "DRIVER=")
		}
	}
	return ""
}

func vendorFromID(id string) string {
	switch id {
	case "0x10de":
		return "nvidia"
	case "0x1002":
		return "amd"
	case "0x8086":
		return "intel"
	}
	return ""
}

// gpuName busca el nombre legible en el PCI id.
func gpuName(dev string) string {
	uevent := readFile(filepath.Join(dev, "uevent"))
	for _, line := range strings.Split(uevent, "\n") {
		if strings.HasPrefix(line, "PCI_ID=") {
			parts := strings.Split(strings.TrimPrefix(line, "PCI_ID="), ":")
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// readVRAM lee la VRAM si el driver la expone.
func readVRAM(dev string) int64 {
	raw := readFile(filepath.Join(dev, "mem_info_vram_total"))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// isDedicated decide integrada vs dedicada. NVIDIA siempre dedicada.
// AMD e Intel: VRAM > 1 GiB o driver de Arc.
func isDedicated(g GPU) bool {
	switch g.Vendor {
	case "nvidia":
		return true
	case "amd":
		return g.VRAM > 1<<30
	case "intel":
		if g.Driver == "xe" && g.VRAM > 1<<30 {
			return true
		}
		return false
	}
	return false
}

// VariantsFor calcula, para el hardware detectado, qué variantes de
// imagen soporta el host y por qué. No mira el registry: eso lo hace
// el llamador combinando con las variantes publicadas.
func VariantsFor(hw Hardware) []Variant {
	var out []Variant
	best := bestGPU(hw.GPUs)
	switch {
	case best != nil && best.Vendor == "nvidia":
		out = append(out,
			Variant{Name: "cuda", Supported: true, Provider: "CUDA", Reason: "GPU dedicada NVIDIA"},
			Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "ONNX Runtime siempre trae CPU"},
		)
	case best != nil && best.Vendor == "amd" && best.Dedicated:
		out = append(out,
			Variant{Name: "amd", Supported: true, Provider: "MIGraphX", Reason: "GPU dedicada AMD (gfx9+)"},
			Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "ONNX Runtime siempre trae CPU"},
		)
	case best != nil && best.Vendor == "intel" && best.Dedicated:
		out = append(out,
			Variant{Name: "intel", Supported: true, Provider: "OpenVINO", Reason: "Intel Arc dedicada"},
			Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "ONNX Runtime siempre trae CPU"},
		)
	case best != nil && best.Vendor == "intel":
		out = append(out,
			Variant{Name: "intel", Supported: true, Provider: "OpenVINO", Reason: "iGPU Intel: OpenVINO CPU (oneDNN) y GPU por device_type"},
			Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "ONNX Runtime siempre trae CPU"},
		)
	case best != nil && best.Vendor == "amd":
		out = append(out,
			Variant{Name: "amd", Supported: true, Provider: "MIGraphX", Reason: "APU AMD: MIGraphX si es gfx9+, si no cae a CPU"},
			Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "ONNX Runtime siempre trae CPU"},
		)
	default:
		out = append(out, Variant{Name: "cpu", Supported: true, Provider: "CPU", Reason: "sin GPU utilizable"})
	}
	return out
}

func bestGPU(gpus []GPU) *GPU {
	if len(gpus) == 0 {
		return nil
	}
	best := &gpus[0]
	for i := range gpus {
		if gpuRank(gpus[i]) > gpuRank(*best) {
			best = &gpus[i]
		}
	}
	return best
}

// Recommend elige la variante recomendada: la acelerada si el host la
// soporta, si no la de CPU. Si la variante no está publicada, cae a
// la mejor disponible que sí lo esté.
func Recommend(hw Hardware, available []string) (variant, reason string, err error) {
	supported := VariantsFor(hw)
	byName := make(map[string]bool, len(available))
	for _, v := range available {
		byName[v] = true
	}
	for _, v := range supported {
		if v.Supported && byName[v.Name] {
			return v.Name, v.Reason, nil
		}
	}
	for _, v := range supported {
		if v.Supported {
			return v.Name, v.Reason + " (no publicada: falta el build)", nil
		}
	}
	if len(available) == 0 {
		return "", "", fmt.Errorf("no hay variantes publicadas")
	}
	return "cpu", "sin GPU utilizable; uso CPU", nil
}

// DescribeGPU arma una línea legible de la GPU.
func DescribeGPU(g GPU) string {
	kind := "integrada"
	if g.Dedicated {
		kind = "dedicada"
	}
	out := fmt.Sprintf("%s %s (%s, %s", g.Vendor, g.Name, g.Driver, kind)
	if g.VRAM > 0 {
		out += fmt.Sprintf(", %.1f GiB VRAM", float64(g.VRAM)/(1<<30))
	}
	return out + ")"
}
