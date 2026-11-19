# BUGS — internal/pacman

Revisión de integridad sobre `repo.go` (48 líneas) y `repo_test.go`. Módulo
nuevo, creado al cerrar el gap de arquitectura: la escritura de `pacman.conf`
salió de `cmd/` a su propio dominio.

Estado de la suite al cerrar: `go test ./internal/pacman/` → ok, cobertura
**36.4 %**. Lo no cubierto es `AgregarRepo`, que necesita sudo.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| M1 | MEDIO | `Configurado` busca `[b3l0w]` como substring: una línea comentada (`# [b3l0w]`) o el nombre dentro de otra sección cuenta como configurado | `repo.go:30` |
| B1 | BAJO | `Section` no termina en `\n`: si `pacman.conf` no termina en salto de línea, la sección se pega a la última línea | `repo.go:11-15` |
| B2 | BAJO | `AgregarRepo` acepta cualquier `section` string; no valida que sea la esperada | `repo.go:39` |

## M1 — el substring no distingue una sección de un comentario

`strings.Contains(string(data), "[b3l0w]")` devuelve verdadero con:

```ini
# [b3l0w]
# Server = https://...
```

Un usuario que comentó el repo a mano para desactivarlo hace que `setup repo`
lo reporte como configurado y no lo agregue de nuevo. No es destructivo —el
repo queda desactivado y el mensaje dice que está— pero el diagnóstico miente.

Arreglo posible: parsear línea por línea y aceptar solo una línea que empiece
con `[b3l0w]` sin `#` delante. No se aplicó: es un caso de borde de un módulo
que se usa una vez en la vida.

## B1 — el `\n` inicial

`Section` empieza con un salto de línea (correcto: separa del contenido previo)
pero no termina en uno. Si el archivo no termina en `\n`, la sección queda
pegada a la última línea y pacman no la reconoce. En la práctica `pacman.conf`
siempre termina en salto, así que no se reprodujo.

## Sin hallazgos en

- La decisión de usar `sudo tee -a` en vez de `os.OpenFile`: la redirección la
  haría el shell sin privilegios, así que `tee` como root es el camino correcto.
- El manejo del error: se devuelve con el path, no se traga.
- `Configurado` devuelve `false` si el archivo no se puede leer: es lo que el
  llamador necesita para intentar el append.

## Arreglados

| ID | Arreglo | Test |
|---|---|---|
| M1 | `Configurado` compara la línea exacta `[b3l0w]`, no un substring: `[b3l0w-extra]` y `# [b3l0w]` (comentada) ya no cuentan como configuradas | `TestConfiguradoSoloCuentaLaSeccionExacta` |
