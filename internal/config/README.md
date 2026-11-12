# config

Lee y escribe el manifiesto de programas.

## Qué hace y por qué existe

El manifiesto es el archivo que dice qué programas son propios de la
distro y de dónde se bajan. Vive en `~/.config/b3l0w/programs.json`
(la ruta sale de `paths.ManifestPath()`).

Es un solo archivo, en JSON, con un propósito: que `b3l0w` sepa qué
programas registrar sin preguntar. Sin esto, instalar un programa
implicaría un paso extra de configuración que nadie va a hacer.

## API

| Función | Qué hace |
|---|---|
| `Default() *Manifest` | El manifiesto de la primera vez |
| `Load(path string) (*Manifest, error)` | Lee el manifiesto; si no existe, devuelve el default |
| `(*Manifest) Save(path string) error` | Escribe el manifiesto, creando el directorio |
| `(*Manifest) Get(name string) *Program` | Devuelve el programa con ese nombre, o `nil` |
| `(*Manifest) Add(name, source string) bool` | Registra un programa con su fuente |
| `(*Manifest) AddProgram(p Program) bool` | Registra un programa completo |
| `(*Manifest) Remove(name string) bool` | Borra un programa |
| `Schema` (const) | Versión del formato: `1` |

### Tipos

```go
type Program struct {
    Name   string `json:"name"`
    Source string `json:"source,omitempty"`
    Asset  string `json:"asset,omitempty"`
}

type Manifest struct {
    Version  int       `json:"version"`
    Programs []Program `json:"programs"`
}
```

`Source` es `github:owner/repo`. `Asset` es el nombre del archivo dentro
del release: cada repo lo publica con el suyo (`boot-preview-linux-amd64`,
`kitty-shapes-linux-x64`), así que no se puede adivinar desde el
nombre del programa. Por eso es un campo explícito y no una convención.

## Uso

```go
m, err := config.Load(paths.ManifestPath())
if err != nil {
    return err
}

if m.Get("kitty-shapes") == nil {
    m.AddProgram(config.Program{
        Name:   "kitty-shapes",
        Source: "github:Below-Z3R0/kitty-shapes",
        Asset:  "kitty-shapes-linux-x64",
    })
    if err := m.Save(paths.ManifestPath()); err != nil {
        return err
    }
}
```

## Decisiones

**JSON y no TOML.** El core no tiene dependencias: un parser de TOML
sería una dependencia por un archivo. Además el archivo se escribe solo
(la instalación lo agrega) aunque queda editable a mano.

**Si el archivo no existe, el default.** `Load` no falla: devuelve el
manifiesto con `boot-preview` y `kitty-shapes`. El CLI tiene que
funcionar recién instalado, sin un paso de configuración previo.

**`Add` es idempotente.** `Add` y `AddProgram` devuelven `false` si el
programa ya estaba. El patrón es "agregá y avisá si algo cambió", no
"agregá y que se duplique". `Remove` sigue la misma convención:
`false` si no estaba.

**`Asset` es explícito.** Podría derivarse del nombre del programa con
una convención (`<nombre>-linux-amd64`), pero cada repo publica el
asset como quiere. Adivinarlo produce descargas de 404 en el release
equivocado.

**`Version` con default 1.** Si el archivo no tiene `version`, se le
pone `Schema`. Los archivos viejos siguen sirviendo; la versión está
para cuando el formato cambie de verdad.

**`Save` crea el directorio.** `MkdirAll` sobre el directorio del
archivo, con `0755`. El archivo se escribe con `0644` y con JSON
indentado a dos espacios: es un archivo que alguien va a leer y
editar.

**`Get` devuelve un puntero al elemento.** El `Program` que se
obtenga es el del slice, no una copia: modificarlo modifica el
manifiesto.

## Archivos

| Archivo | Qué contiene |
|---|---|
| `manifest.go` | Todo el módulo: tipos `Program`/`Manifest` y las funciones de lectura, escritura y mutación |
| `manifest_test.go` | Tests del default, roundtrip, idempotencia de `Add` y `Remove` |

## Qué NO hace

- **No descarga nada.** Solo guarda y lee metadatos.
- **No sabe de releases ni de assets.** Armar la URL del release es
  de `internal/programs`.
- **No valida la fuente.** `github:owner/repo` se acepta tal cual; el
  error sale cuando se usa.
- **No ejecuta ni instala.** Solo es la lista.

## Tests

4 tests: el default cuando el archivo no existe (con los dos programas
esperados), roundtrip de save/load con un directorio que tampoco
existe, la idempotencia de `Add` y el comportamiento de `Remove`.
