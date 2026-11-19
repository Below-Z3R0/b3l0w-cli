# BUGS — internal/snapshots

Revisión de integridad sobre `limine.go`, `report.go` y los 5 archivos de test.
El módulo lista, poda y reporta las entradas de snapshots en Limine.

Estado de la suite al cerrar: `go test ./internal/snapshots/` → ok, cobertura
**100 %**.

## Resumen

| ID | Sev | Qué | Línea |
|---|---|---|---|
| A1 | ALTO | `Prune` borra con `remove` y después falla si `sync` falla: el llamador recibe error con los snapshots ya borrados | `limine.go:132-135` |
| M1 | MEDIO | `Report` descarta el error de `Entries()` y devuelve 0, así que un fallo de la herramienta se ve como "no hay nada que podar" | `report.go:19-24` |
| M2 | MEDIO | `Report` normaliza `keep <= 0` a 20; `Prune` rechaza `keep < 0`. El mismo parámetro tiene dos contratos | `report.go:14-16` vs `limine.go:105-107` |
| B1 | BAJO | El mensaje "quedan N" usa `total-removed`, que puede no coincidir con lo que hay en disco si otro proceso creó snapshots entre el conteo y la poda | `report.go:52` |

## A1 — el borrado y el sync no son atómicos

`Prune` hace un solo `limine-snapper-remove` con todos los IDs viejos y después
un `limine-snapper-sync`. Si el primero funciona y el segundo falla, devuelve
`0, err`: el llamador reporta un error y no sabe que las entradas **sí** se
borraron. `limine.conf` queda desactualizado hasta el próximo sync.

No es destructivo —los snapshots se borraron, que es lo que se pidió— pero el
mensaje miente sobre el estado. Lo correcto sería devolver `len(old), err` para
que el llamador pueda decir "se borraron N pero el sync falló".

## M1 — el error de conteo se descarta

```go
total, err := Entries()
if err != nil {
    output.Warn("snapshots de Limine: " + err.Error())
    return 0
}
```

Un fallo de `limine-snapper-list` (no instalado, permiso, salida rara) se
reporta como un aviso y el comando sale 0. Para `update`, que llama a `Report`,
eso significa "la poda no aplica" en vez de "la poda falló". Es una decisión
deliberada —la poda es opcional y no debe frenar el update— pero el exit code
no distingue los dos casos.

## M2 — dos contratos para `keep`

`Report` convierte `keep <= 0` en 20 antes de llamar a `Prune`. `Prune` rechaza
`keep < 0` con error y acepta `0`. Un llamador que use `Prune` directo con
`-1` recibe un error; uno que use `Report` con `-1` recibe la poda con 20. La
normalización vive en una capa y la validación en otra.

## B1 — el número que se anuncia

`total` viene de un comando y `removed` de otro. Entre los dos, otro proceso
puede haber creado o borrado snapshots. El mensaje "N entradas borradas;
quedan M" es correcto para el momento del conteo, no necesariamente para el
momento de leerlo. El código ya advierte cuando `removed == 0 && total > keep`
(el caso de la lista incompleta), así que el caso está reconocido.

## Sin hallazgos en

- `parseEntries` y `parseIDs`: hay tests de tabla, de salida real con color, de
  conteo negativo, de duplicados y de orden, más fuzzing.
- `Prune` rechaza `keep` negativo antes de tocar nada: evita el `ids[:len-keep]`
  con índice inventado.
- Un solo `remove` con todos los IDs: `limine.conf` se reescribe una vez.
