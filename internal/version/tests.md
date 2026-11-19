# TESTS — internal/version

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 1 |
| Verdes | 1 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | `[no statements]` (el paquete no tiene sentencias ejecutables) |
| Comando | `go test ./internal/version/ -count=1 -cover` |

El paquete tiene una sola declaración (`var Version = "dev"`), que no es una
sentencia ejecutable: por eso `go test` reporta `[no statements]` en vez de
un porcentaje. No es cobertura cero del código — es que no hay código que
cubrir.

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `version_test.go` | Que `Version` es una **variable** de tipo string (no una función) y que no está vacía: si el linker no la pudiera pisar, el build fallaría; que en una compilación local sin `-ldflags` valga `"dev"` |

## Detalle de cada test

### `version_test.go`

| Test | Qué fija |
|---|---|
| `TestVersionEsUnaVariableYNoEstaVacia` | `Version != ""`. Es el invariante mínimo: una variable no inicializada o sobreescrita a vacío rompe `b3l0w version`. Además, si `Version != "dev"` solo registra un `t.Logf` (no falla): el valor inyectado por el build es correcto y no debe volver rojo el test |

## Qué quedó sin cubrir (y por qué)

| Caso | Por qué no está |
|---|---|
| **La inyección por `-ldflags` funciona de verdad** | Requiere compilar el binario completo con `-X` y ejecutarlo: es un test de integración, no unitario. El README lo reconoce y da el comando manual (`README.md:60-67`). **Verificado a mano durante esta revisión**: `go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v9.9.9" -o /tmp/b3l0w-test ./cmd/b3l0w` y luego `/tmp/b3l0w-test version` imprime `b3l0w v9.9.9`. La inyección funciona; lo que falta es un test que lo 자동ice |
| **El `-ldflags` apunta al símbolo correcto** | Mismo motivo. El fallo es silencioso (`README.md:45-47`): si `internal/version/` se moviera, el build seguiría compilando y el binario diría `"dev"`. Un test que compile y verifique la salida lo detectaría |
| **El `PKGBUILD` usa la misma ruta que el paquete** | `packaging/b3l0w-cli/PKGBUILD:35` y `README.md:29-47` repiten el path a mano. Nada los ata al path real del paquete: un rename rompe los dos en silencio. Test posible: leer el path del `PKGBUILD` y compararlo con `reflect`/`go list` sobre el paquete real |
| **El paquete no se importa desde un lugar que lo mueva** | Es un invariante de proyecto, no del módulo. Lo cubre el `go build ./...` del CI |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/version/ -count=1 -cover

# Verificar la inyección a mano (lo que un test debería hacer):
go build -ldflags "-X github.com/Below-Z3R0/b3l0w-cli/internal/version.Version=v9.9.9" \
  -o /tmp/b3l0w-test ./cmd/b3l0w
/tmp/b3l0w-test version   # debe decir v9.9.9
```
