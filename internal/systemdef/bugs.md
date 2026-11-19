# BUGS — internal/systemdef

Revisión de integridad sobre `systemdef.go` (229 líneas entre los dos archivos
de producción) y los 3 archivos de test. El módulo lee `system.json`: qué
perfiles existen y qué instala cada uno.

Estado de la suite al cerrar: `go test ./internal/systemdef/` → ok, cobertura
**74.1 %**.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| M1 | MEDIO | `Names` dice "ordenados" en el docstring y **no ordena**: devuelve el orden del mapa, que cambia entre corridas | `systemdef.go:75-83` |
| M2 | MEDIO | `LoadRemote` cae al cache sin decir que está viejo ni por qué falló el remoto | `remote.go:27-52` |
| B1 | BAJO | `writeCache` ignora los dos errores (`MkdirAll` y `WriteFile`) | `remote.go:100-107` |
| B2 | BAJO | `Merge` no valida que el nombre del perfil exista antes de buscar | `systemdef.go:84+` |

## M1 — el docstring miente sobre el orden

```go
// Names devuelve los perfiles disponibles, ordenados.
func (d *Definition) Names() []string {
	names := make([]string, 0, len(d.Profiles))
	for name := range d.Profiles {
		names = append(names, name)
	}
	return names
}
```

El recorrido de un mapa en Go no es determinista. Un llamador que muestre
`Names()` en una tabla o en un JSON ve el orden cambiar entre corridas: dos
ejecuciones del mismo comando dan listas distintas. El arreglo es un
`sort.Strings(names)` antes del `return`, o corregir el docstring si el orden
no importa.

No se aplicó: el módulo no está en el alcance de esta documentación y el
cambio, aunque de una línea, altera la salida de quien lo consuma.

## M2 — el fallback silencioso

`LoadRemote` intenta, en orden: cache fresco → red → cache viejo → embebido.
Cuando el remoto falla o devuelve un JSON inválido, cae al cache sin distinguir
"no hay red" de "el remoto devolvió basura", y el llamador recibe una definición
que puede tener días sin saberlo. Para un archivo que define qué instala el
sistema, la diferencia entre definición fresca y vieja importa.

## B1 — el cache que no se escribe

`writeCache` termina en `_ = os.WriteFile(...)`. Si el disco está lleno o el
directorio no se puede crear, el fallo es invisible: la próxima corrida vuelve
a bajar el archivo y nadie se enteró de que el cache nunca funcionó.

## B2 — `Merge` con un nombre que no existe

`Merge` recorre `names` y busca en `d.Profiles`. Si un nombre no está, el
comportamiento depende de cómo esté escrito el resto de la función (no se
verificó la rama). Un perfil inexistente debería ser un error explícito, no un
perfil vacío.

## Sin hallazgos en

- `parse` valida versión y que haya perfiles: las dos condiciones que importan.
- `Builtin` va embebido con `go:embed`: el CLI sabe qué es "su" sistema sin un
  archivo al lado.
