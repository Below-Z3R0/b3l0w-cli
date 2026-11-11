package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Run ejecuta un comando y devuelve su stdout.
func Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimRight(string(out), "\n"), err
}

// RunCombined ejecuta un comando con stderr mezclado.
func RunCombined(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// RunTimeout ejecuta un comando y lo mata si tarda más de lo indicado.
//
// Los clientes que hablan con un daemon pueden quedarse esperando una
// respuesta que no llega: sin deadline, el comando hereda esa espera.
func RunTimeout(d time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// Interactive ejecuta un comando heredando la terminal: es lo que
// necesita un actualizador que pregunta.
func Interactive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HaveCommand dice si un binario está en el PATH.
func HaveCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// CommandPath devuelve la ruta de un binario, o "" si no está.
func CommandPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// IsTTY dice si stdin es una terminal.
//
// Un actualizador interactivo lanzado sin terminal falla o se cuelga,
// y el fallo es silencioso: hay que preguntarlo antes.
//
// No alcanza con ModeCharDevice: /dev/null también es un char device,
// y con stdin en /dev/null la lectura devuelve EOF al instante. Una
// terminal de verdad acepta TCGETS; nada más lo hace.
func IsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	var termios syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdin.Fd(),
		syscall.TCGETS, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return errno == 0
}

// SafeName valida que un nombre sirva para componer un path.
//
// Los nombres vienen de manifiestos y del usuario: si se usan tal cual
// en un filepath.Join, un "../" escapa del directorio y el CLI borra o
// escribe archivos de cualquier parte del sistema.
//
// Se acepta lo que un nombre de paquete o servicio tiene: letras,
// numeros, guion, guion bajo y punto. Nada de separadores.
func SafeName(nombre string) error {
	if nombre == "" {
		return fmt.Errorf("el nombre está vacío")
	}
	if nombre == "." || nombre == ".." {
		return fmt.Errorf("el nombre %q no es válido", nombre)
	}
	for _, r := range nombre {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("el nombre %q tiene caracteres no permitidos", nombre)
		}
	}
	return nil
}

// PackageOwner devuelve el paquete que provee un archivo, o "".
//
// Sirve para la etapa en que los programas propios sean paquetes: un
// binario con dueño no se auto-actualiza, se actualiza con pacman.
func PackageOwner(path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	out, err := RunCombined("pacman", "-Qo", path)
	if err != nil {
		return ""
	}
	// "ruta is owned by paquete version"
	fields := strings.Fields(out)
	for i, field := range fields {
		if field == "by" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// ExpandHome cambia un ~ inicial por el home del usuario.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// ReplaceFile reemplaza dest con src de forma atómica.
//
// Copia a un temporal en el mismo directorio y hace rename: así no se
// pisa un binario que está corriendo ("text file busy") y no queda un
// archivo a medias si algo falla.
func ReplaceFile(src, dest string) error {
	// Si el destino es un symlink (por ejemplo ~/.local/bin/prog ->
	// ~/.config/prog/prog), se reemplaza el archivo real: si no, quedaría
	// el symlink convertido en una copia y el binario duplicado.
	if resolved, err := filepath.EvalSymlinks(dest); err == nil {
		dest = resolved
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Se conserva el modo del destino: un config 0644 no tiene que
	// terminar 0755 por reemplazarlo.
	modo := os.FileMode(0o644)
	if info, err := os.Stat(dest); err == nil {
		modo = info.Mode().Perm()
	}

	tmp := dest + ".new"
	if err := os.WriteFile(tmp, data, modo); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
