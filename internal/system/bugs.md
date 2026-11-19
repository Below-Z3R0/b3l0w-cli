# BUGS — internal/system

Revisión de integridad sobre `archupdate.go` (82 líneas) y su test. El módulo
envuelve el actualizador del sistema (`cachy-update`, que es un symlink a
`arch-update`): no lo reescribe, lo llama.

Estado de la suite al cerrar: `go test ./internal/system/` → ok, cobertura
**46.4 %**.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| M1 | MEDIO | `Pending` cuenta como paquete cualquier línea posterior a un encabezado que contenga `package` o `aur`, incluidos comentarios y líneas en blanco ya filtradas | `archupdate.go:62-64` |
| B1 | BAJO | `Binary` no verifica que el resultado sea ejecutable: devuelve lo que `CommandPath` encuentre | `archupdate.go:23` |
| B2 | BAJO | `List` no distingue un timeout de un error del actualizador: los dos salen como el mismo `err` | `archupdate.go:38` |

## M1 — el conteo no distingue una línea de un paquete

`Pending` recorre las líneas y suma a `repo` o `aur` según el último encabezado
visto. Cualquier línea no vacía cuenta, incluidas las de texto libre que el
actualizador imprima dentro de una sección. El formato de `-l` hoy es una línea
por paquete, así que el número es correcto; si el upstream agrega una nota
dentro de la sección, el conteo sube sin que haya paquetes.

No se arregló: parsear el formato real exige conocer las variantes de salida de
`arch-update`, y adivinarlas sería peor que contar líneas.

## B1 — `CommandPath` no garantiza ejecutable

`Binary()` devuelve la ruta tal cual. `kit/system.CommandPath` usa `exec.LookPath`,
que sí filtra por permiso de ejecución, así que el caso está cubierto por la
capa de abajo. Queda anotado por si esa capa cambia.

## B2 — timeout y error comparten el mismo camino

`RunTimeout(5*time.Minute, bin, "-l")` devuelve `(out, err)`; el módulo
propaga `err` sin distinguir si fue el proceso o el deadline. El llamador no
puede decir "tardó demasiado" de "falló". Aceptable para un comando que el
usuario corre a mano.

## Sin hallazgos en

- El fallback `cachy-update` → `arch-update`: el orden es el correcto (primero el
  nombre de la distro, después el upstream).
- `UpdateInteractive` no automatiza nada: no hay bandera `--yes` y el código no
  finge que la haya.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| M1 | `Pending` parsea la salida real de `arch-update -l`: quita los códigos ANSI de las cabeceras (sin eso el conteo queda en cero), ignora `Flatpak Packages:` y solo cuenta líneas con forma de paquete | `TestPendingParseaLaSalidaRealDeArchUpdate`, `TestPendingNoCuentaLosPaquetesDeFlatpak`, `TestPendingNoCuentaLasLineasQueNoSonPaquetes` |
