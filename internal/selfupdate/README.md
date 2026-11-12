# selfupdate

Actualiza el propio binario de b3l0w.

## Qué hace y por qué existe

Es el caso especial del contrato de programas: el core no puede
actualizarse con `b3l0w update` sin bajarse a sí mismo. `b3l0w update
--self` lo hace.

El binario en ejecución se reemplaza en el sitio, con las mismas reglas
de seguridad que la instalación de cualquier otro programa: firma
obligatoria y validación antes de pisar.

## API

| Función | Qué hace |
|---|---|
| `Repository() string` | Repo GitHub del CLI: `Below-Z3R0/b3l0w-cli` |
| `Check() (string, error)` | Tag del último release del CLI |
| `Run() (Result, error)` | Baja el release y reemplaza el binario en ejecución |

```go
type Result struct {
    Before  string
    After   string
    Changed bool
    // Note explica por qué no se cambió nada cuando no fue por estar
    // al día (por ejemplo, un release más viejo que el binario).
    Note string
}
```

`Repository()` está exportado para que otros paquetes (por ejemplo el
chequeo de firmas de `internal/releases`) no repitan el string.

## Uso

```go
result, err := selfupdate.Run()
if err != nil {
    // Si el binario es de un paquete, pacman manda: no es fatal.
    return err
}
switch {
case result.Changed:
    fmt.Printf("b3l0w %s -> %s\n", result.Before, result.After)
case result.Note != "":
    fmt.Println(result.Note)
default:
    fmt.Printf("b3l0w ya está al día (%s)\n", result.After)
}
```

## Decisiones

**El tag se busca con la convención `v*`, no con `/releases/latest`.**
El repo también publica los paquetes del escritorio, y ese release
queda como "latest" aunque no tenga el binario. `Check` pide los 30
releases más recientes, descarta drafts y prereleases, y se queda con
el primero cuyo tag empieza con `v`.

**Si el binario es de un paquete, no se toca.** `pacman -Qo` sobre el
ejecutable: si tiene dueño, el error dice "se actualiza con pacman".
El caller (`internal/update`) lo trata como warning, no como fallo.

**La comparación va antes de la descarga.** Si el tag publicado es más
viejo o igual al binario local, `Run` no baja los 11 MB para
descubrirlo después. Solo compara cuando el tag es estrictamente
nuevo.

**No degrada un binario de desarrollo.** `compareVersions` compara
numéricamente e ignora los sufijos (`-dev`, `+build`): `v0.5.0-dev` y
`v0.5.0` son la misma versión. Un release más viejo que el binario no
degrada: se deja como está y se explica en `Note`.

**La firma se verifica antes de reemplazar.** El asset se valida con
`programs.VerifyRelease` (SHA256SUMS + `SHA256SUMS.asc`) antes de
tocar el binario en ejecución. Sin firma válida, no hay reemplazo.

**El binario bajado se prueba antes de reemplazar.** Se ejecuta
`version` sobre el asset: si no responde, no se pisa el binario que
funciona.

**El reemplazo es atómico.** Lo hace `system.ReplaceFile`: copia a un
temporal en el mismo directorio y hace `rename`. Así no se pisa un
binario que está corriendo ("text file busy") y no queda un archivo a
medias si algo falla.

**Todo es inyectable.** `apiURL`, `fetchAsset`, `replaceFile`,
`packageOwner`, `currentVersion`, `verifyRelease`, `checkRelease` y
`resolveExe` son variables: los tests sustituyen red, descarga,
reemplazo y verificación, y no dependen ni de GitHub ni del binario en
ejecución.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `selfupdate.go` | Todo el módulo: `Repository`, `Check`, `Run`, `Result` y los helpers de comparación de versiones |
| `selfupdate_test.go` | Tests con todas las dependencias sustituidas |

## Qué NO hace

- **No actualiza el sistema base.** Eso es de `b3l0w update --system`.
- **No actualiza los programas del manifiesto.** Cada programa se
  actualiza con su propio `update`.
- **No firma releases.** La clave privada nunca sale del equipo del
  mantenedor.
- **No toca un binario que pertenece a un paquete.**
- **No usa `/releases/latest`.** Por diseño, para no confundirlo con
  los releases de escritorio.

## Tests

13 tests: el prefijo `v` de la versión, la lectura del tag con la
convención `v*` (ignorando un release de escritorio), el error con
HTTP 500 y con JSON inválido, el rechazo del binario que pertenece a un
paquete, el error cuando el asset bajado no responde, que no cambie
nada si la versión es igual, la comparación de versiones (incluido
`v0.5.0-dev` contra `v0.5.0`), que no descargue si el tag es viejo,
que no degrade, que reemplace cuando la versión es distinta, que no
reemplace si la firma falla, y que propague el error de reemplazo.
