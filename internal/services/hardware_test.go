package services

import "testing"

func TestVendorFromID(t *testing.T) {
	cases := map[string]string{
		"0x10de": "nvidia",
		"0x1002": "amd",
		"0x8086": "intel",
		"0x1234": "",
	}
	for id, want := range cases {
		if got := vendorFromID(id); got != want {
			t.Errorf("vendorFromID(%q) = %q, quería %q", id, got, want)
		}
	}
}

func TestIsDedicated(t *testing.T) {
	cases := []struct {
		gpu  GPU
		want bool
	}{
		{GPU{Vendor: "nvidia"}, true},
		{GPU{Vendor: "amd", VRAM: 2 << 30}, true},
		{GPU{Vendor: "amd", VRAM: 512 << 20}, false},
		{GPU{Vendor: "intel", Driver: "xe", VRAM: 2 << 30}, true},
		{GPU{Vendor: "intel", Driver: "xe"}, false},
		{GPU{Vendor: "intel", Driver: "i915"}, false},
	}
	for _, c := range cases {
		if got := isDedicated(c.gpu); got != c.want {
			t.Errorf("isDedicated(%+v) = %v, quería %v", c.gpu, got, c.want)
		}
	}
}

func TestVariantsFor(t *testing.T) {
	cases := []struct {
		name string
		hw   Hardware
		want string
	}{
		{
			name: "nvidia dedicada",
			hw:   Hardware{GPUs: []GPU{{Vendor: "nvidia", Dedicated: true}}},
			want: "cuda",
		},
		{
			name: "amd dedicada",
			hw:   Hardware{GPUs: []GPU{{Vendor: "amd", Dedicated: true}}},
			want: "amd",
		},
		{
			name: "intel arc",
			hw:   Hardware{GPUs: []GPU{{Vendor: "intel", Dedicated: true}}},
			want: "intel",
		},
		{
			name: "intel igpu",
			hw:   Hardware{GPUs: []GPU{{Vendor: "intel"}}},
			want: "intel",
		},
		{
			name: "amd apu",
			hw:   Hardware{GPUs: []GPU{{Vendor: "amd"}}},
			want: "amd",
		},
		{
			name: "sin gpu",
			hw:   Hardware{},
			want: "cpu",
		},
		{
			name: "cpu amd sin gpu",
			hw:   Hardware{CPUVendor: "AuthenticAMD"},
			want: "cpu",
		},
	}
	for _, c := range cases {
		got, _, err := Recommend(c.hw, []string{"cpu", "cuda", "intel", "amd"})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: recomienda %q, quería %q", c.name, got, c.want)
		}
	}
}

func TestRecommendFallsBackToPublished(t *testing.T) {
	hw := Hardware{GPUs: []GPU{{Vendor: "intel"}}}
	// La variante intel no está publicada: cae a cpu.
	got, reason, err := Recommend(hw, []string{"cpu", "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cpu" {
		t.Fatalf("recomendó %q, quería cpu", got)
	}
	if reason == "" {
		t.Fatal("sin motivo")
	}
}

func TestRecommendPrefersPublishedAcceleration(t *testing.T) {
	hw := Hardware{GPUs: []GPU{{Vendor: "nvidia", Dedicated: true}}}
	got, _, err := Recommend(hw, []string{"cpu", "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "cuda" {
		t.Fatalf("recomendó %q, quería cuda", got)
	}
}

func TestAvailableVariants(t *testing.T) {
	tags := []string{"cpu-latest", "cpu-abc1234", "cuda-latest", "intel-latest", "buildcache-cpu"}
	got := AvailableVariants(tags, []string{"cpu", "cuda", "intel", "amd"})
	want := map[string]bool{"cpu": true, "cuda": true, "intel": true}
	if len(got) != 3 {
		t.Fatalf("esperaba 3, obtuve %d: %v", len(got), got)
	}
	for _, v := range got {
		if !want[v] {
			t.Errorf("variante inesperada: %s", v)
		}
	}
}

func TestAvailableVariantsDefault(t *testing.T) {
	got := AvailableVariants([]string{"3.10.4", "latest", "buildcache-x"}, []string{"default"})
	if len(got) != 1 || got[0] != "default" {
		t.Fatalf("default: %v", got)
	}
	got = AvailableVariants([]string{"buildcache-x"}, []string{"default"})
	if len(got) != 0 {
		t.Fatalf("solo caché no cuenta como default: %v", got)
	}
}

func TestLatestTag(t *testing.T) {
	tags := []string{"buildcache-cpu", "cpu-abc1234", "cpu-latest"}
	if got := LatestTag(tags, "cpu"); got != "cpu-latest" {
		t.Fatalf("LatestTag = %q, quería cpu-latest", got)
	}
	if got := LatestTag([]string{"buildcache-cpu", "cpu-abc"}, "cpu"); got != "cpu-abc" {
		t.Fatalf("LatestTag = %q, quería cpu-abc", got)
	}
	if got := LatestTag([]string{"intel-latest"}, "amd"); got != "" {
		t.Fatalf("LatestTag = %q, quería vacío", got)
	}
}

func TestDescribeGPU(t *testing.T) {
	g := GPU{Vendor: "intel", Name: "8086:46A3", Driver: "i915", VRAM: 0}
	out := DescribeGPU(g)
	if out == "" {
		t.Fatal("vacío")
	}
}
