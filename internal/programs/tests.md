# TESTS — internal/programs

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 12 |
| Verdes | 12 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **27.6 %** |
| Comando | `go test ./internal/programs/ -count=1 -cover` |

Cobertura por función (medida con `go tool cover -func`):

| Función | Cobertura | Nota |
|---|---|---|
| `PackageName` | 100.0 % | |
| `ownedByB3l0w` | 100.0 % | |
| `ParseVersion` | 100.0 % | |
| `lastLine` | 100.0 % | |
| `VerifyRelease` | 92.9 % | |
| `AssetURL` | 87.5 % | |
| `Packaged` | 75.0 % | |
| `InstallPath` | 66.7 % | |
| **`Current`** | **0.0 %** | |
| **`firstLine`** | **0.0 %** | |
| **`Statuses`** | **0.0 %** | |
| **`Update`** | **0.0 %** | |
| **`Install`** | **0.0 %** | |
| **`InstallVerified`** | **0.0 %** | |
| **`InstallPackage`** | **0.0 %** | |
| **`RemovePackage`** | **0.0 %** | |
| **`Uninstall`** | **0.0 %** | |

**Siete funciones con 0 % de cobertura**, y entre ellas están las que
**escriben en disco** (`InstallVerified`, `InstallPackage`,
`RemovePackage`, `Uninstall`) y las que **ejecutan procesos**
(`Current`, `Update`). El módulo que instala programas es el que menos se
prueba. Los 12 tests que hay son casi todos de funciones puras.

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `programs_test.go` | Tests de versión, URL de asset y ruta de instalación |
| `pacman_test.go` | Tests del nombre de paquete y de la detección de paquetes propios |
| `verify_test.go` | Tests del rechazo cuando falta la firma |
| `verify_full_test.go` | Test end-to-end de la verificación con GPG real (se salta si no hay `gpg`) |

## Detalle de cada test

### `programs_test.go`

| Test | Qué fija |
|---|---|
| `TestParseVersion` | Tabla de 6 casos: `"kitty-shapes v1.0.0"` → `v1.0.0`, `"boot-preview v0.2.2"` → `v0.2.2`, `"v1.2.3"` → `v1.2.3`, `"prog 10.20.30 extra"` → `v10.20.30` (ruido alrededor), `"sin version"` → `""`, `""` → `""` |
| `TestLastLine` | `lastLine("uno\ndos\n\n")` → `"dos"` (la última no vacía, no la última línea) y `lastLine("   ")` → `"sin salida"` |
| `TestAssetURL` | El camino feliz: `github:Below-Z3R0/kitty-shapes` + asset `kitty-shapes-linux-x64` → `https://github.com/Below-Z3R0/kitty-shapes/releases/latest/download/kitty-shapes-linux-x64` |
| `TestAssetURLSinAsset` | Sin `Asset` en el manifiesto → error |
| `TestAssetURLFuenteRara` | Fuente `gitlab:a/b` → error (solo se soporta `github:`) |
| `TestInstallPath` | La ruta termina en `/.local/bin/kitty-shapes` |

### `pacman_test.go`

| Test | Qué fija |
|---|---|
| `TestPackageName` | Tabla de 4 casos: `b3l0w` y `b3l0w-cli` → `b3l0w-cli`; `boot-preview` y `kitty-shapes` → sin cambio |
| `TestPackagedNoConfundeNombres` | Un nombre que no existe (`programa-que-no-existe-xyz`) no se toma por paquete. **Es el único test de `Packaged`** y solo cubre el caso negativo trivial |
| `TestOwnedByB3l0w` | Tabla de 6 casos: `b3l0w-cli`, `b3l0w-keyring`, `boot-preview`, `kitty-shapes` → `true`; `firefox`, `pacman` → `false` |

### `verify_test.go`

| Test | Qué fija |
|---|---|
| `TestVerifyReleaseSinFirma` | Un release que no publica `SHA256SUMS` (el servidor devuelve 404 para todo): `VerifyRelease` falla con `verify.ErrSinFirma`. Es el fail-closed del módulo |
| `TestVerifyReleaseURLExtrana` | Una URL que no termina con el nombre del asset → error ("no pude derivar la URL del release") |

### `verify_full_test.go`

| Test | Qué fija |
|---|---|
| `TestVerifyReleaseCompleto` | El camino real de punta a punta: genera una clave GPG en un `t.TempDir()`, firma un `SHA256SUMS` con `gpg --detach-sign`, levanta un `httptest` que sirve el asset, los sums y la firma, y verifica que `VerifyRelease` acepta. Después **altera el asset** y verifica que ahora falla. Se saltea si no hay `gpg` instalado (`verify_full_test.go:20-22`) |

## Qué quedó sin cubierto (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **`InstallVerified` completo** | Necesita red (bajar el asset) y una firma válida | **0 % de cobertura.** Es la función que **escribe el binario en `~/.local/bin`**. Con ella sin probar nadie detectó que el archivo queda en `0644` |
| **El modo del binario instalado** | `ReplaceFile` real, no un stub | **Bug M2 (MEDIO).** Verificado: `system.ReplaceFile` con destino inexistente deja el archivo en `-rw-r--r--`, sin `+x`. El usuario instala y no puede ejecutarlo |
| **`Install` / `InstallPackage` / `RemovePackage`** | Ejecutan `sudo pacman` | **0 % de cobertura.** `InstallPackage` (`pacman.go:36-52`) borra el binario suelto que tapa al del paquete (`pacman.go:42-46`) y luego exige que el programa esté en el PATH — ninguna de las dos cosas está probada |
| **`Uninstall`** | Escribe en disco y toca el manifiesto real | **0 % de cobertura.** **Bug A2 (ALTO)**: borra el archivo antes de guardar el manifiesto (`uninstall.go:43-51`), así que un fallo del save deja el estado invertido. Y **bug A1 (ALTO)**: con `Packaged` consultando la BD sync, un nombre que colisione con un paquete de la distro dispara `sudo pacman -Rns --noconfirm` |
| **`Packaged` con un paquete que sí existe** | `TestPackagedNoConfundeNombres` solo prueba un nombre inexistente | **Bug A1 (ALTO).** Verificado en esta máquina: `Packaged("firefox") == true`, `Packaged("bash") == true` — la BD sync, no la de instalados |
| **`Current`** | Ejecuta un binario externo | **0 % de cobertura.** Es la función que traduce la salida de `prog version` en una versión. **Bug M1 (MEDIO)**: `ParseVersion("kitty-shapes 2026.11.06-r1") == "v2026.11.06"` — una fecha se lee como versión |
| **`firstLine`** | Solo se exercise indirectamente por `Current` | **0 %.** Es la función que da el mensaje útil cuando el binario no conoce `version` (`programs.go:45-49`) |
| **`Statuses`** | Necesita un manifiesto y binarios en el PATH | **0 %.** Es el estado que muestra `b3l0w update --check` |
| **`Update`** | Ejecuta `prog update` | **0 %.** Y **bug B3 (BAJO)**: devuelve solo la última línea de la salida, no "la salida" como dice el README |
| **`AssetURL` con `github:owner/repo/extra`** | La tabla de casos no tiene ese input | **Bug M3 (MEDIO).** Verificado: produce `https://github.com/owner/repo/extra/releases/latest/download/a` sin error |
| **`Uninstall` cuando el manifiesto no tiene el programa** | Nadie lo prueba | El error `"%s no está registrado"` (`uninstall.go:16-18`) no está protegido |
| **`Uninstall` con un binario de paquete ajeno** | Necesita pacman | El camino `uninstall.go:35-42` (error "se quita con pacman", sin borrar a mano) no está probado. Es el que el README promete (`README.md:109-114`) |
| **`Uninstall` cuando el save del manifiesto falla** | Requiere un destino de escritura que falle | **Bug A2 (ALTO).** Es por inspección: el orden de las sentencias en `uninstall.go:43-51` es borrar → quitar en memoria → guardar |
| **La ruta de instalación real** (`~/.local/bin`) | `TestInstallPath` solo mira el sufijo de la ruta | El test corre contra el `$HOME` real del usuario. No escribe nada, así que es inocuo, pero un `HOME` con espacios o sin permisos no está considerado |
| **`VerifyRelease` con un `SHA256SUMS` malformado** | Solo se prueba el 404 y el éxito | Un `SHA256SUMS` con una suma que no corresponde (en vez de un asset distinto) no está cubierto |
| **La firma que no verifica** (GPG con otra clave) | Requiere generar dos claves | `TestVerifyReleaseCompleto` prueba el asset alterado, no una firma de otra clave. Es el caso de ataque: el sums es correcto pero la firma no |
| **El `--insecure`** (`requireSignature=false`) | `InstallVerified` no se prueba | El camino que **salta la verificación** (`install.go:76-80`) no tiene ningún test. Es el interruptor de seguridad del módulo |
| **La concurrencia** | No hay `t.Parallel()` | Las vars de paquete que se sustituyen son pocas; no hay problema hoy |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/programs/ -count=1 -cover

# Solo la verificación end-to-end con GPG (se salta si no hay gpg):
go test ./internal/programs/ -run TestVerifyReleaseCompleto -v -count=1

# Ver la cobertura por función (siete funciones aparecen con 0.0 %):
go test ./internal/programs/ -count=1 -coverprofile=/tmp/programs.cover
go tool cover -func=/tmp/programs.cover
```
