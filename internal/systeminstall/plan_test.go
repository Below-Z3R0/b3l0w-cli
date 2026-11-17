package systeminstall

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Below-Z3R0/b3l0w-cli/internal/systemdef"
)

// TestBuildPerfilVacio comprueba que un perfil sin nada no genera plan.
func TestBuildPerfilVacio(t *testing.T) {
	plan := Build(systemdef.Profile{}, "", Options{Runner: newFake()})
	if !plan.Empty() {
		t.Fatalf("un perfil vacío tiene que dar un plan vacío, dio %+v", plan)
	}
}

// TestBuildPerfilCompleto comprueba que con una máquina limpia el plan
// pide todo lo que declara el perfil, en el orden del perfil.
func TestBuildPerfilCompleto(t *testing.T) {
	fake := newFake()
	profile := systemdef.Profile{
		Repo:     []string{"vim", "git"},
		AUR:      []string{"noctalia-git"},
		Flatpak:  []string{"app.zen_browser.zen"},
		Own:      []string{"b3l0w"},
		Services: []string{"docker.socket"},
		Groups:   []string{"docker", "video"},
	}
	// La máquina está limpia: ningún servicio habilitado.
	fake.fail["systemctl is-enabled docker.socket"] = errors.New("disabled")

	plan := Build(profile, "", Options{Runner: fake})

	if !reflect.DeepEqual(plan.Packages, []string{"vim", "git"}) {
		t.Errorf("paquetes = %v", plan.Packages)
	}
	if !reflect.DeepEqual(plan.AUR, []string{"noctalia-git"}) {
		t.Errorf("AUR = %v", plan.AUR)
	}
	if !reflect.DeepEqual(plan.Flatpak, []string{"app.zen_browser.zen"}) {
		t.Errorf("flatpak = %v", plan.Flatpak)
	}
	if !reflect.DeepEqual(plan.Own, []string{"b3l0w"}) {
		t.Errorf("propios = %v", plan.Own)
	}
	if !reflect.DeepEqual(plan.Services, []string{"docker.socket"}) {
		t.Errorf("servicios = %v", plan.Services)
	}
	if !reflect.DeepEqual(plan.Groups, []string{"docker", "video"}) {
		t.Errorf("grupos = %v", plan.Groups)
	}
	if plan.Empty() {
		t.Fatal("un plan con todo pendiente no está vacío")
	}
}

// TestBuildOmitirLoInstalado es el caso normal en una máquina ya
// armada: lo que está no se repite.
func TestBuildOmitirLoInstalado(t *testing.T) {
	fake := newFake()
	fake.outputs["pacman -Qq"] = "vim\ngit\nbase\n"
	fake.outputs["flatpak list --app --columns=application"] = "app.zen_browser.zen\n"
	fake.outputs["id -nG"] = "wheel audio docker\n"
	fake.paths["b3l0w"] = "/usr/bin/b3l0w"

	profile := systemdef.Profile{
		Repo:     []string{"vim", "git", "neovim"},
		AUR:      []string{"git", "umbriel-git"},
		Flatpak:  []string{"app.zen_browser.zen", "org.upscayl.Upscayl"},
		Own:      []string{"b3l0w", "kitty-shapes"},
		Services: []string{},
		Groups:   []string{"docker", "wheel", "seat"},
	}

	plan := Build(profile, "", Options{Runner: fake})

	if !reflect.DeepEqual(plan.Packages, []string{"neovim"}) {
		t.Errorf("paquetes = %v, quería solo neovim", plan.Packages)
	}
	// Un paquete AUR que ya está en pacman no se reinstala.
	if !reflect.DeepEqual(plan.AUR, []string{"umbriel-git"}) {
		t.Errorf("AUR = %v, quería solo umbriel-git", plan.AUR)
	}
	if !reflect.DeepEqual(plan.Flatpak, []string{"org.upscayl.Upscayl"}) {
		t.Errorf("flatpak = %v", plan.Flatpak)
	}
	if !reflect.DeepEqual(plan.Own, []string{"kitty-shapes"}) {
		t.Errorf("propios = %v", plan.Own)
	}
	if !reflect.DeepEqual(plan.Groups, []string{"seat"}) {
		t.Errorf("grupos = %v, quería solo seat", plan.Groups)
	}
}

// TestBuildServiciosHabilitados cubre la consulta a systemctl: solo
// entra al plan el que no está habilitado.
func TestBuildServiciosHabilitados(t *testing.T) {
	fake := newFake()
	fake.fail["systemctl is-enabled NetworkManager"] = errors.New("disabled")
	// sddm queda con err == nil (habilitado).

	profile := systemdef.Profile{Services: []string{"NetworkManager", "sddm"}}
	plan := Build(profile, "", Options{Runner: fake})

	if !reflect.DeepEqual(plan.Services, []string{"NetworkManager"}) {
		t.Errorf("servicios = %v, quería solo NetworkManager", plan.Services)
	}
	if !reflect.DeepEqual(fake.queries, []string{
		"pacman -Qq",
		"flatpak list --app --columns=application",
		"systemctl is-enabled NetworkManager",
		"systemctl is-enabled sddm",
		"id -nG",
	}) {
		t.Errorf("consultas = %v", fake.queries)
	}
}

// TestBuildComandoQueFallaNoRompe: si pacman no está o falla, el set de
// instalados queda vacío y el plan pide todo, en vez de explotar.
func TestBuildComandoQueFallaNoRompe(t *testing.T) {
	fake := newFake()
	fake.fail["pacman -Qq"] = errors.New("pacman: command not found")
	fake.fail["flatpak list --app --columns=application"] = errors.New("sin flatpak")
	fake.fail["id -nG"] = errors.New("id: falló")

	profile := systemdef.Profile{
		Repo:    []string{"vim"},
		Flatpak: []string{"org.upscayl.Upscayl"},
		Groups:  []string{"docker"},
	}
	plan := Build(profile, "", Options{Runner: fake})

	if !reflect.DeepEqual(plan.Packages, []string{"vim"}) ||
		!reflect.DeepEqual(plan.Flatpak, []string{"org.upscayl.Upscayl"}) ||
		!reflect.DeepEqual(plan.Groups, []string{"docker"}) {
		t.Fatalf("con los comandos caídos tenía que pedir todo: %+v", plan)
	}
}

// TestBuildDotfiles resuelve rutas: relativa -> bajo dotfiles, absoluta
// -> tal cual, ~ -> home, y marca qué existe.
func TestBuildDotfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dotfiles := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dotfiles, "fish"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotfiles, "fish", "config.fish"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// El destino existe: no hace falta copiarlo.
	if err := os.MkdirAll(filepath.Join(home, ".config", "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	// El origen de noctalia también, para que quede en su lugar.
	if err := os.MkdirAll(filepath.Join(dotfiles, "noctalia"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Un origen absoluto fuera del árbol de dotfiles.
	absSrc := filepath.Join(t.TempDir(), "extra.conf")
	if err := os.WriteFile(absSrc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := systemdef.Profile{Files: []systemdef.File{
		{Src: "fish", Dest: "~/.config/fish"},
		{Src: "noctalia", Dest: "~/.config/noctalia"},
		{Src: absSrc, Dest: "~/.config/extra.conf"},
		{Src: "falta", Dest: "~/.config/falta"},
	}}

	plan := Build(profile, dotfiles, Options{Runner: newFake()})

	if got, want := plan.Files[0].Src, filepath.Join(dotfiles, "fish"); got != want {
		t.Errorf("origen relativo = %q, quería %q", got, want)
	}
	if got, want := plan.Files[0].Dest, filepath.Join(home, ".config", "fish"); got != want {
		t.Errorf("destino ~ = %q, quería %q", got, want)
	}
	if !plan.Files[0].SrcExists || plan.Files[0].DestExists {
		t.Errorf("fish: srcExiste=%v destExiste=%v", plan.Files[0].SrcExists, plan.Files[0].DestExists)
	}
	if !plan.Files[1].SrcExists || !plan.Files[1].DestExists {
		t.Errorf("noctalia: tenía que existir origen y destino")
	}
	if got := plan.Files[2].Src; got != absSrc {
		t.Errorf("origen absoluto = %q, quería %q", got, absSrc)
	}
	if plan.Files[3].SrcExists {
		t.Error("un origen ausente no puede figurar como existente")
	}
}

// TestBuildSinDirectorioDeDotfiles deja la ruta relativa tal cual.
func TestBuildSinDirectorioDeDotfiles(t *testing.T) {
	profile := systemdef.Profile{Files: []systemdef.File{{Src: "fish", Dest: "/tmp/x"}}}
	plan := Build(profile, "", Options{Runner: newFake()})
	if got := plan.Files[0].Src; got != "fish" {
		t.Errorf("origen = %q, quería fish", got)
	}
}

// TestPlanEmptyConArchivos cubre la regla de Empty: un archivo cuenta
// solo si tiene origen y le falta destino. Un origen ausente es un
// error de Apply, no trabajo pendiente.
func TestPlanEmptyConArchivos(t *testing.T) {
	casos := []struct {
		nombre string
		file   FileStatus
		vacio  bool
	}{
		{"sin origen", FileStatus{SrcExists: false, DestExists: false}, true},
		{"destino en su lugar", FileStatus{SrcExists: true, DestExists: true}, true},
		{"falta destino", FileStatus{SrcExists: true, DestExists: false}, false},
	}
	for _, caso := range casos {
		plan := &Plan{Files: []FileStatus{caso.file}}
		if got := plan.Empty(); got != caso.vacio {
			t.Errorf("%s: Empty() = %v, quería %v", caso.nombre, got, caso.vacio)
		}
	}
}
