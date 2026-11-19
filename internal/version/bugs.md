# BUGS — internal/version

Revision de integridad sobre `internal/version` (HEAD `fb76242`).
Se leyó `version.go` (12 líneas), `version_test.go` (15 líneas) y `README.md`.
No se modificó código ni tests.

Estado de la suite al cerrar la revisión: `go test ./internal/version/ -count=1`
→ **ok** (0.002s). `go vet` limpio.

## Sin hallazgos

**No se encontraron bugs en `internal/version`.** El módulo tiene una sola
declaración: `var Version = "dev"` (`version.go:12`). Se revisó:

- **Que la variable sea una y no una función.** Es el invariante que
  sostiene el `-ldflags -X` (`version.go:1-12`, `README.md:13-27`). Es
  correcto: una función no sería pisable por el linker y el binario
  informaría `"dev"` para siempre. Verificado de punta a punta: `go build
  -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v9.9.9"`
  y el binario resultante imprime `b3l0w v9.9.9`. La inyección funciona.
- **Que la ruta del `-ldflags` coincida con el módulo real.** El README la
  documenta en `README.md:29-47` y el `PKGBUILD` la usa en
  `packaging/b3l0w-cli/PKGBUILD:35`. Las dos coinciden con el `module` de
  `go.mod` (`github.com/Below-Z3R0/b3l0w-cli`) y con el path real del
  paquete (`internal/version`). Correcto.
- **Que el valor por defecto sea `"dev"`** (`version.go:12`). Correcto: una
  compilación local no inyectada debe decir `"dev"`, no una versión falsa.
- **Que el test cubra lo que dice cubrir** (`version_test.go:7-15`). Verifica
  que la variable no esté vacía y que en local valga `"dev"`. Es exactamente
  lo que el README promete (`README.md:55-58`).
- **Que el paquete no exponga más superficie de la necesaria.** No hay
  funciones, ni estado mutable, ni dependencias: `version.go` importa nada.
  No hay forma de que alguien lo rompa desde adentro.

**Lo que sí es una limitación, no un bug:** el fallo de que el símbolo se
mueva de lugar no lo detecta nadie. El README lo dice explícitamente
(`README.md:45-47`: "El build **no falla**: compila igual y la versión queda
en `dev`"). Es una decisión consciente y documentada, no un defecto. Queda
anotado en `tests.md` como lo que no se cubre.

## Acoplamiento y testeabilidad

**Bien:**

- El paquete no importa nada: es compilable y testeable en aislamiento.
- El test no necesita red, ni disco, ni el home real.
- La variable es de tipo `string`, que es el único tipo que `-X` puede pisar.

**Mal:**

- El paquete depende de su *path*: si `internal/version/` se mueve, el
  `-ldflags` de `PKGBUILD:35` y el del README quedan apuntando a un símbolo
  inexistente. El linker no avisa y el CLI reporta `"dev"` en cada
  `b3l0w version`. Un test que compila el binario con `-X` y verifica la
  salida lo detectaría; hoy ese test no existe (ver `tests.md`).

## Lo que no encontré

- No hay bugs de lógica: no hay lógica.
- No hay panic posible: no hay accesos a mapas, punteros ni slices.
- No hay dependencia de red ni de sistema en este paquete.
- El valor `"dev"` es el único string mágico del módulo y está documentado
  como tal en `README.md:9-11`.
