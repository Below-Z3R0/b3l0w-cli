package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Below-Z3R0/b3l0w-cli/internal/registry"
	"github.com/Below-Z3R0/b3l0w-cli/internal/services"
	"github.com/Below-Z3R0/b3l0w-kit/output"
)

// Los subcomandos de `b3l0w services`: instalar, actualizar y operar
// los servicios Docker de la distro.
//
// Cada subcomando hace una sola cosa y devuelve el codigo de salida:
// el dispatcher decide que hacer con el.

// servicesInstall instala un servicio.
func servicesInstall(args []string) int {
	manifest, stateDir, rest, dryRun := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		output.Fail("falta el servicio: b3l0w services install <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		output.Fail("servicio desconocido: " + id)
		return 1
	}
	if svc.State == "planned" {
		output.Fail(svc.ID + " todavía no está listo")
		return 1
	}
	variant := ""
	for _, arg := range rest {
		switch {
		case arg == "--yes" || arg == "-y":
			// el install no pregunta nada
		case strings.HasPrefix(arg, "--variant="):
			variant = strings.TrimPrefix(arg, "--variant=")
		case strings.HasPrefix(arg, "-"):
			output.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	state, err := in.Install(*svc, variant, dryRun)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if state == nil || dryRun {
		return 0
	}
	output.Ok(fmt.Sprintf("%s instalado (%s, %s)", state.ID, state.Variant, state.Health))
	if state.Provider != "" {
		fmt.Printf("  provider: %s\n", state.Provider)
	}
	if svc.Models != "" {
		fmt.Printf("  modelos: %s\n", output.Dim("correr "+svc.Models))
	}
	return 0
}

// servicesUpdate actualiza los servicios instalados.
func servicesUpdate(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	yes := false
	rebuild := true
	var only string
	for _, arg := range rest {
		switch {
		case arg == "--yes" || arg == "-y":
			yes = true
		case arg == "--no-rebuild":
			rebuild = false
		case strings.HasPrefix(arg, "--service="):
			only = strings.TrimPrefix(arg, "--service=")
		default:
			if !strings.HasPrefix(arg, "-") {
				only = arg
				continue
			}
			output.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	states, _ := services.ListStates(stateDir)
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	if len(states) == 0 {
		output.Warn("no hay servicios instalados")
		return 0
	}
	output.Step("Actualizando servicios")
	// Un fallo tiene que verse en el exit code: un script que hace
	// "b3l0w services update && echo OK" no puede mentir.
	fallaron := 0
	for _, state := range states {
		if only != "" && state.ID != only {
			continue
		}
		svc := m.Find(state.ID)
		if svc == nil {
			continue
		}
		newState, err := in.Update(*svc, state, rebuild, yes)
		if err != nil {
			output.Fail(fmt.Sprintf("%s: %v", state.ID, err))
			fallaron++
			continue
		}
		if newState.UpdatedAt != state.UpdatedAt {
			output.Ok(fmt.Sprintf("%s actualizado", state.ID))
		}
	}
	if fallaron > 0 {
		return 1
	}
	return 0
}

// servicesStatus muestra el estado de los servicios instalados.
func servicesStatus(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	if len(states) == 0 {
		output.Warn("no hay servicios instalados")
		return 0
	}
	docker := services.NewDocker()
	output.Step("Estado de los servicios")
	for _, state := range states {
		svc := m.Find(state.ID)
		name := state.ID
		if svc != nil {
			name = svc.Name
		}
		fmt.Printf("  %-10s %-8s %-10s %s\n", state.ID, state.Variant, state.Health, output.Dim(name))
		if state.Provider != "" {
			fmt.Printf("  %-10s provider: %s\n", "", state.Provider)
		}
		if svc == nil || svc.Compose == "" {
			continue
		}
		variant := ""
		if state != nil {
			variant = state.Variant
		}
		dir := serviceDir(*svc)
		ps, err := docker.PS(dir, services.ComposeFiles(*svc, variant))
		if err != nil {
			continue
		}
		for _, p := range ps {
			fmt.Printf("  %-10s %-12s %s\n", "", p.Name, output.Dim(p.State+" "+p.Health))
		}
	}
	return 0
}

func servicesCommand(args []string) int {
	if len(args) == 0 {
		return servicesList(args)
	}
	if verb := aliasVerb("services", args[0]); verb != "" {
		args = append([]string{verb}, args[1:]...)
	}
	switch args[0] {
	case "list":
		return servicesList(args[1:])
	case "search":
		return servicesSearch(args[1:])
	case "hardware":
		return servicesHardware(args[1:])
	case "images":
		return servicesImages(args[1:])
	case "sync":
		return servicesSync(args[1:])
	case "install":
		return servicesInstall(args[1:])
	case "update":
		return servicesUpdate(args[1:])
	case "status":
		return servicesStatus(args[1:])
	case "doctor":
		return servicesDoctor(args[1:])
	case "logs":
		return servicesLogs(args[1:])
	case "restart":
		return servicesSimple(args[1:], "restart")
	case "stop":
		return servicesSimple(args[1:], "stop")
	case "remove":
		return servicesRemove(args[1:])
	case "variant":
		return servicesVariant(args[1:])
	case "rollback":
		return servicesRollback(args[1:])
	case "prune":
		return servicesPrune(args[1:])
	}
	output.Fail("uso: b3l0w services [list|search|hardware|images|sync|install|update|status|doctor|logs|restart|stop|remove|variant|rollback|prune]\n" +
		aliasHelp("services"))
	return 1
}

// servicesRollback vuelve a la imagen anterior de un servicio.
func servicesRollback(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		output.Fail("falta el servicio: b3l0w services rollback <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		output.Fail("servicio desconocido: " + id)
		return 1
	}
	state, err := services.LoadState(stateDir, svc.ID)
	if err != nil {
		output.Fail(svc.ID + " no está instalado")
		return 1
	}
	in := &services.Installer{
		Docker:   services.NewDocker(),
		Registry: services.NewRegistry(""),
		StateDir: stateDir,
	}
	newState, err := in.Rollback(*svc, state)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	output.Ok(fmt.Sprintf("%s volvió a %s (%s)", newState.ID, newState.Tag, newState.Health))
	return 0
}

// servicesPrune borra las imágenes colgantes del registry local.
func servicesPrune(args []string) int {
	out, err := services.NewDocker().PruneImages()
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if out == "" {
		output.Ok("no hay imágenes colgantes")
		return 0
	}
	fmt.Println(out)
	return 0
}

// servicesDoctor chequea que la máquina pueda correr servicios.
func servicesDoctor(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	docker := services.NewDocker()
	res := docker.Doctor(m, services.DetectHardware(), states)
	if res.Errors > 0 {
		return 1
	}
	return 0
}

// servicesLogs muestra las últimas líneas de un servicio.
func servicesLogs(args []string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		output.Fail("falta el servicio: b3l0w services logs <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		output.Fail("servicio desconocido: " + id)
		return 1
	}
	tail := 100
	service := ""
	// Se recorre TODO rest, no rest[1:]: con el flag adelante
	// (--tail=50 argos) el id queda fuera del slice y --tail se ignora.
	for _, arg := range rest {
		switch {
		case strings.HasPrefix(arg, "--tail="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--tail="))
			if err != nil || n < 1 {
				output.Fail("--tail debe ser un entero positivo")
				return 1
			}
			tail = n
		case arg == "--dry-run":
			// ya lo agarro servicePaths
		case arg == id:
			// el id del servicio, no un flag
		case strings.HasPrefix(arg, "--"):
			output.Fail("flag desconocido: " + arg)
			return 1
		default:
			service = arg
		}
	}
	dir := serviceDir(*svc)
	out, err := services.NewDocker().Logs(dir, services.ComposeFiles(*svc, ""), service, tail)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	fmt.Print(out)
	return 0
}

// servicesSimple ejecuta restart o stop.
func servicesSimple(args []string, action string) int {
	manifest, _, rest, dryRun := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if len(rest) == 0 {
		output.Fail("falta el servicio: b3l0w services " + action + " <id>")
		return 1
	}
	svc := m.Find(rest[0])
	if svc == nil {
		output.Fail("servicio desconocido: " + rest[0])
		return 1
	}
	dir := serviceDir(*svc)
	files := services.ComposeFiles(*svc, "")
	docker := services.NewDocker()
	if dryRun {
		output.Ok(fmt.Sprintf("%s: %s (dry-run, no se tocó nada)", svc.ID, action))
		return 0
	}
	var err2 error
	if action == "restart" {
		err2 = docker.Restart(dir, files)
	} else {
		err2 = docker.Stop(dir, files)
	}
	if err2 != nil {
		output.Fail(err2.Error())
		return 1
	}
	output.Ok(fmt.Sprintf("%s: %s", svc.ID, action))
	return 0
}

// servicesRemove baja el stack. --purge borra los volúmenes.
func servicesRemove(args []string) int {
	manifest, stateDir, rest, dryRun := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	id := firstPositional(rest)
	if id == "" {
		output.Fail("falta el servicio: b3l0w services remove <id>")
		return 1
	}
	svc := m.Find(id)
	if svc == nil {
		output.Fail("servicio desconocido: " + id)
		return 1
	}
	purge := false
	// Los flags se buscan en TODO rest, no en rest[1:]: con el flag
	// adelante (--purge argos) el id caía en la rama default y el
	// comando decía "flag desconocido: argos".
	for _, arg := range rest {
		switch {
		case arg == "--purge":
			purge = true
		case arg == "--dry-run":
			// ya lo agarro servicePaths
		case arg == svc.ID:
			// el id, no un flag
		case strings.HasPrefix(arg, "--"):
			output.Fail("flag desconocido: " + arg)
			return 1
		default:
			output.Fail("flag desconocido: " + arg)
			return 1
		}
	}
	if dryRun {
		output.Ok(fmt.Sprintf("%s removido (dry-run, no se tocó nada)", svc.ID))
		return 0
	}
	dir := serviceDir(*svc)
	files := services.ComposeFiles(*svc, "")
	if err := services.NewDocker().Remove(dir, files, purge); err != nil {
		output.Fail(err.Error())
		return 1
	}
	os.Remove(filepath.Join(stateDir, svc.ID+".json"))
	if purge {
		output.Ok(fmt.Sprintf("%s removido con sus volúmenes", svc.ID))
	} else {
		output.Ok(fmt.Sprintf("%s removido (los volúmenes quedan)", svc.ID))
	}
	return 0
}

// servicesVariant cambia la variante de un servicio instalado.
func servicesVariant(args []string) int {
	manifest, stateDir, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if len(rest) < 2 {
		output.Fail("uso: b3l0w services variant <id> <variante>")
		return 1
	}
	svc := m.Find(firstPositional(rest))
	if svc == nil {
		output.Fail("servicio desconocido: " + firstPositional(rest))
		return 1
	}
	want := ""
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") && arg != firstPositional(rest) {
			want = arg
			break
		}
	}
	state, err := services.LoadState(stateDir, svc.ID)
	if err != nil {
		output.Fail(svc.ID + " no está instalado")
		return 1
	}
	tags, err := services.NewRegistry("").Tags(svc.Image)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	published := services.AvailableVariants(tags, svc.Variants)
	found := false
	for _, v := range published {
		if v == want {
			found = true
			break
		}
	}
	if !found {
		output.Fail(fmt.Sprintf("%s: la variante %q no está publicada (hay: %s)", svc.ID, want, strings.Join(published, ", ")))
		return 1
	}
	tag := services.TagFor(*svc, want, tags)
	image := svc.Image + ":" + tag
	output.Step(fmt.Sprintf("%s → %s", svc.ID, want))
	fmt.Printf("  imagen: %s\n", image)
	fmt.Println("  → pull y recreate del stack")

	docker := services.NewDocker()
	if err := docker.Pull(svc.Image, tag); err != nil {
		output.Fail(err.Error())
		return 1
	}
	digest, _ := docker.ImageDigest(image)

	// Recrear el stack es lo que hace que la variante se aplique.
	//
	// Antes solo se hacia el pull y se guardaba el estado: el
	// contenedor seguía corriendo la imagen anterior y el estado
	// decía otra cosa. Ademas el env con IMAGE es lo que le dice a
	// compose que imagen levantar.
	dir := serviceDir(*svc)
	files := services.ComposeFiles(*svc, want)
	if err := docker.Up(dir, files, map[string]string{"IMAGE": tag}); err != nil {
		output.Fail(err.Error())
		return 1
	}

	state.Previous = state.Image
	state.PreviousTag = state.Tag
	state.Variant = want
	state.Image = image
	state.Tag = tag
	state.Digest = digest
	state.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := state.Save(stateDir); err != nil {
		output.Fail(err.Error())
		return 1
	}
	output.Ok(fmt.Sprintf("%s ahora usa %s (%s)", svc.ID, want, tag))
	return 0
}

func servicesSync(args []string) int {
	explicit, _, _, _ := servicePaths(args)
	// Primero el "pacman -Sy": bajar la DB remota al caché.
	if explicit == "" {
		if _, err := registry.Sync(registry.Services, services.Embedded()); err != nil {
			output.Warn(err.Error())
		}
		explicit = registry.CachePath(registry.Services)
	}
	manifest := explicit
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	changed, err := services.Sync(m, manifest, services.NewRegistry(""))
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	if changed == 0 {
		output.Ok("manifiesto al día")
		return 0
	}
	output.Ok(fmt.Sprintf("manifiesto actualizado (%d servicios)", changed))
	return 0
}

func servicesList(args []string) int {
	manifest, stateDir, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	states, _ := services.ListStates(stateDir)
	byID := make(map[string]*services.State, len(states))
	for _, s := range states {
		byID[s.ID] = s
	}
	reg := services.NewRegistry("")
	st := services.CheckRegistry(m, reg)
	services.RenderList(m, byID, st, services.DetectHardware())
	return 0
}

// servicesSearch filtra el manifiesto por texto.
func servicesSearch(args []string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	q := strings.TrimSpace(strings.Join(rest, " "))
	// Sin busqueda no es una busqueda: listaba todo y salia con 0.
	// plugins search ya fallaba; los dos hermanos hacen lo mismo.
	if q == "" {
		output.Fail("falta la búsqueda: b3l0w services search <texto>")
		return 1
	}
	return services.RenderSearch(m, q)
}

func servicesHardware(args []string) int {
	manifest, _, _, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	st := services.CheckRegistry(m, services.NewRegistry(""))
	services.RenderHardware(services.DetectHardware(), m, st)
	return 0
}

func servicesImages(args []string) int {
	manifest, _, rest, _ := servicePaths(args)
	m, err := services.LoadManifest(manifest)
	if err != nil {
		output.Fail(err.Error())
		return 1
	}
	reg := services.NewRegistry("")
	for i := range m.Services {
		svc := &m.Services[i]
		if len(rest) > 0 && svc.ID != rest[0] {
			continue
		}
		if svc.Image == "" {
			continue
		}
		output.Step(svc.ID + " · " + svc.Image)
		tags, err := reg.Tags(svc.Image)
		if err != nil {
			output.Warn(err.Error())
			continue
		}
		for _, t := range tags {
			if strings.HasPrefix(t, "buildcache-") {
				continue
			}
			fmt.Printf("  %s\n", t)
		}
	}
	return 0
}
