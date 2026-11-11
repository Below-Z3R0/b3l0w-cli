package systeminstall

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// planCompleto arma un plan con las siete categorías.
func planCompleto(t *testing.T, src string) *Plan {
	t.Helper()
	return &Plan{
		Packages: []string{"vim"},
		AUR:      []string{"noctalia-git"},
		Flatpak:  []string{"org.upscayl.Upscayl"},
		Own:      []string{"b3l0w"},
		Services: []string{"docker.socket"},
		Groups:   []string{"docker", "video"},
		Files:    []FileStatus{{Src: src, Dest: filepath.Join(t.TempDir(), "destino"), SrcExists: true}},
	}
}

// TestApplyOrdenDeEjecucion es el contrato del módulo: paquetes -> AUR
// -> flatpak -> propios -> servicios -> grupos -> dotfiles -> plugins.
func TestApplyOrdenDeEjecucion(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "config.fish")
	if err := os.WriteFile(src, []byte("set -g fish_greeting ''\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := planCompleto(t, src)

	fake := newFake()
	fake.paths["paru"] = "/usr/bin/paru"

	var conPlugins bool
	opts := Options{
		Runner: fake,
		SyncPlugins: func() {
			conPlugins = true
			fake.calls = append(fake.calls, "plugins")
		},
	}

	if err := Apply(plan, opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"sudo pacman -S --needed vim",
		"paru -S --needed noctalia-git",
		"flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo",
		"flatpak install -y flathub org.upscayl.Upscayl",
		self + " install b3l0w",
		"sudo systemctl enable docker.socket",
		"sudo usermod -aG docker,video " + u.Username,
		"cp -a " + src + " " + plan.Files[0].Dest,
		"plugins",
	}
	if len(fake.calls) != len(want) {
		t.Fatalf("orden = %v\nquería %v", fake.calls, want)
	}
	for i := range want {
		if fake.calls[i] != want[i] {
			t.Errorf("paso %d = %q, quería %q", i, fake.calls[i], want[i])
		}
	}
	if !conPlugins {
		t.Error("el paso de plugins tiene que ser el último")
	}
}

// TestApplyUnPasoQueFallaCortaElFlujo: si pacman falla no se toca nada
// más; el error dice qué paso fue.
func TestApplyUnPasoQueFallaCortaElFlujo(t *testing.T) {
	plan := planCompleto(t, "/no/existe")
	fake := newFake()
	fake.fail["sudo pacman -S --needed vim"] = errors.New("dependencia rota")

	err := Apply(plan, Options{
		Runner:      fake,
		SyncPlugins: func() { t.Error("plugins no tenía que ejecutarse") },
	})
	if err == nil || !strings.Contains(err.Error(), "pacman falló") {
		t.Fatalf("error = %v, quería \"pacman falló: ...\"", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "sudo pacman -S --needed vim" {
		t.Fatalf("después del fallo no se ejecuta nada más, se ejecutó %v", fake.calls)
	}
}

// TestApplyFallaEnServiciosNoTocaGruposNiDotfiles.
func TestApplyFallaEnServiciosNoTocaGruposNiDotfiles(t *testing.T) {
	plan := planCompleto(t, "/no/existe")
	fake := newFake()
	fake.paths["paru"] = "/usr/bin/paru"
	fake.fail["sudo systemctl enable docker.socket"] = errors.New("unit not found")

	err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}})
	if err == nil || !strings.Contains(err.Error(), "servicios") {
		t.Fatalf("error = %v, quería el de servicios", err)
	}
	if fake.contains("sudo usermod -aG docker,video " + currentUser(t).Username) {
		t.Error("no se pueden agregar grupos si falló un servicio antes")
	}
	if fake.contains("cp -a " + plan.Files[0].Src + " " + plan.Files[0].Dest) {
		t.Error("no se copian dotfiles si falló un servicio antes")
	}
}

// TestApplyDryRunNoEjecutaNada: en simulación el runner no ve un solo
// comando, ni siquiera una consulta.
func TestApplyDryRunNoEjecutaNada(t *testing.T) {
	plan := planCompleto(t, "/no/existe")
	fake := newFake()
	llamado := false

	err := Apply(plan, Options{
		DryRun:      true,
		Runner:      fake,
		SyncPlugins: func() { llamado = true },
	})
	if err != nil {
		t.Fatalf("dry-run no puede fallar: %v", err)
	}
	if len(fake.calls) != 0 || len(fake.queries) != 0 || len(fake.lookups) != 0 {
		t.Fatalf("dry-run tocó la máquina: calls=%v queries=%v lookups=%v",
			fake.calls, fake.queries, fake.lookups)
	}
	if llamado {
		t.Error("dry-run no puede tocar los plugins")
	}
}

// TestApplyYesPasaNoconfirm cubre el instalador desatendido.
func TestApplyYesPasaNoconfirm(t *testing.T) {
	plan := &Plan{Packages: []string{"vim"}, AUR: []string{"noctalia-git"}}
	fake := newFake()
	fake.paths["paru"] = "/usr/bin/paru"

	if err := Apply(plan, Options{Yes: true, Runner: fake, SyncPlugins: func() {}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !fake.contains("sudo pacman -S --needed --noconfirm vim") {
		t.Errorf("pacman sin --noconfirm: %v", fake.calls)
	}
	if !fake.contains("paru -S --needed --noconfirm noctalia-git") {
		t.Errorf("paru sin --noconfirm: %v", fake.calls)
	}
}

// TestApplySinAyudanteAUREsError: sin paru ni yay no se adivina, se
// aborta con un mensaje que dice qué hacer.
func TestApplySinAyudanteAUREsError(t *testing.T) {
	plan := &Plan{AUR: []string{"noctalia-git"}}
	fake := newFake()

	err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}})
	if err == nil || !strings.Contains(err.Error(), "paru ni yay") {
		t.Fatalf("error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("sin ayudante no se ejecuta nada: %v", fake.calls)
	}
}

// TestApplyDotfileSinOrigenEsError: un dotfile sin origen corta antes
// de copiar, no a mitad de camino.
func TestApplyDotfileSinOrigenEsError(t *testing.T) {
	plan := &Plan{Files: []FileStatus{{Src: "/no/existe", Dest: "/tmp/x", SrcExists: false}}}
	fake := newFake()

	err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}})
	if err == nil || !strings.Contains(err.Error(), "falta el origen") {
		t.Fatalf("error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("no se copia nada: %v", fake.calls)
	}
}

// TestApplyDotfileHaceBackup: si el destino existe, primero lo guarda
// con timestamp y recién después copia.
func TestApplyDotfileHaceBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.conf")
	dest := filepath.Join(dir, "dest.conf")
	if err := os.WriteFile(src, []byte("nuevo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("viejo"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := newFake()
	plan := &Plan{Files: []FileStatus{{Src: src, Dest: dest, SrcExists: true, DestExists: true}}}

	if err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %v, quería backup + copia", fake.calls)
	}
	if !strings.HasPrefix(fake.calls[0], "cp -a "+dest+" "+dest+".bak-") {
		t.Errorf("primero va el backup: %q", fake.calls[0])
	}
	if fake.calls[1] != "cp -a "+src+" "+dest {
		t.Errorf("después va la copia: %q", fake.calls[1])
	}
	// El fake no copia: solo se comprueba que el destino se liberó para
	// que el cp real lo cree.
	if pathExists(dest) {
		t.Error("el destino viejo tenía que quedar borrado antes del cp")
	}
}

// TestApplyDotfileSinDestinoNoHaceBackup.
func TestApplyDotfileSinDestinoNoHaceBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.conf")
	dest := filepath.Join(dir, "sub", "dest.conf")
	if err := os.WriteFile(src, []byte("nuevo"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := newFake()
	plan := &Plan{Files: []FileStatus{{Src: src, Dest: dest, SrcExists: true}}}

	if err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "cp -a "+src+" "+dest {
		t.Errorf("calls = %v", fake.calls)
	}
	if !pathExists(filepath.Dir(dest)) {
		t.Error("Apply tiene que crear el directorio del destino")
	}
}

// TestApplyPlanVacioNoHaceNada: sin trabajo solo corre el paso de
// plugins (es idempotente y mantiene la source al día).
func TestApplyPlanVacioNoHaceNada(t *testing.T) {
	fake := newFake()
	plugins := 0
	if err := Apply(&Plan{}, Options{Runner: fake, SyncPlugins: func() { plugins++ }}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("calls = %v", fake.calls)
	}
	if plugins != 1 {
		t.Errorf("plugins = %d, quería 1", plugins)
	}
}

// currentUser es user.Current con el chequeo de error del test.
func currentUser(t *testing.T) *user.User {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestApplyCadaPasoFallaConSuMensaje recorre las seis categorías que
// ejecutan un comando: el error nombra el paso y después no corre nada.
func TestApplyCadaPasoFallaConSuMensaje(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	u := currentUser(t)

	casos := []struct {
		nombre string
		plan   *Plan
		falla  string
		quiere string
	}{
		{"paquetes", &Plan{Packages: []string{"vim"}}, "sudo pacman -S --needed vim", "pacman falló"},
		{"AUR", &Plan{AUR: []string{"x"}}, "paru -S --needed x", "paru falló"},
		{"flatpak", &Plan{Flatpak: []string{"org.x"}}, "flatpak install -y flathub org.x", "flatpak falló"},
		{"propios", &Plan{Own: []string{"b3l0w"}}, self + " install b3l0w", "no se pudo instalar b3l0w"},
		{"servicios", &Plan{Services: []string{"sshd"}}, "sudo systemctl enable sshd", "servicios"},
		{"grupos", &Plan{Groups: []string{"docker"}}, "sudo usermod -aG docker " + u.Username, "grupos"},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			fake := newFake()
			fake.paths["paru"] = "/usr/bin/paru"
			fake.fail[caso.falla] = errors.New("boom")

			err := Apply(caso.plan, Options{
				Runner:      fake,
				SyncPlugins: func() { t.Error("plugins no tiene que correr si un paso falló") },
			})
			if err == nil || !strings.Contains(err.Error(), caso.quiere) {
				t.Fatalf("error = %v, quería %q", err, caso.quiere)
			}
			if got := fake.calls[len(fake.calls)-1]; got != caso.falla {
				t.Errorf("el último comando fue %q, quería el que falla %q", got, caso.falla)
			}
		})
	}
}

// TestApplyErroresDeDotfile cubre los dos cp del dotfile: si falla el
// backup no se borra el destino, y si falla la copia el error la nombra.
func TestApplyErroresDeDotfile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.conf")
	dest := filepath.Join(dir, "dest.conf")
	if err := os.WriteFile(src, []byte("nuevo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("viejo"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("backup", func(t *testing.T) {
		fake := newFake()
		fake.failPrefix = []string{"cp -a " + dest + " " + dest + ".bak-"}
		plan := &Plan{Files: []FileStatus{{Src: src, Dest: dest, SrcExists: true, DestExists: true}}}

		err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}})
		if err == nil || !strings.Contains(err.Error(), "backup de") {
			t.Fatalf("error = %v, quería el de backup", err)
		}
		// Si el backup falló, el destino no se tocó.
		if !pathExists(dest) {
			t.Error("el destino no se puede borrar si el backup falló")
		}
	})

	t.Run("copia", func(t *testing.T) {
		fake := newFake()
		fake.failPrefix = []string{"cp -a " + src + " "}
		// Sin destino no hay backup: el único cp es el de la copia.
		plan := &Plan{Files: []FileStatus{{Src: src, Dest: filepath.Join(dir, "nuevo.conf"), SrcExists: true}}}

		err := Apply(plan, Options{Runner: fake, SyncPlugins: func() {}})
		if err == nil || !strings.Contains(err.Error(), "copiar "+src) {
			t.Fatalf("error = %v, quería el de copia", err)
		}
	})
}

// TestDryRunPlanVacio no imprime secciones y no toca nada.
func TestDryRunPlanVacio(t *testing.T) {
	fake := newFake()
	if err := Apply(&Plan{}, Options{DryRun: true, Runner: fake}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("calls = %v", fake.calls)
	}
}

// TestSystemRunnerNoEsNil: el runner real existe y nil cae en él.
func TestSystemRunnerNoEsNil(t *testing.T) {
	if SystemRunner() == nil {
		t.Fatal("SystemRunner() no puede ser nil")
	}
	if orReal(nil) == nil {
		t.Fatal("orReal(nil) tiene que devolver el runner real")
	}
	if got := orReal(&fakeRunner{}); got == nil {
		t.Fatal("orReal no puede descartar un runner inyectado")
	}
}
