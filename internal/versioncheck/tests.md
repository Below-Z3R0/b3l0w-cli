# TESTS — internal/versioncheck

Qué prueba cada test del módulo y qué quedó sin cubrir.

## Estado de la suite

| Métrica | Valor |
|---|---|
| Tests | 2 |
| Verdes | 2 |
| **Rojos** | **0** |
| Saltados | 0 |
| Cobertura | **85.0 %** |
| Comando | `go test ./internal/versioncheck/ -count=1 -cover` |

Cobertura por función (medida con `go tool cover -func`):

| Función | Cobertura |
|---|---|
| `AUR` | 85.0 % |

El 15 % que falta son las ramas de error del decoder JSON y el caso de
`names` vacío.

## Mapa de los archivos

| Archivo | Qué prueba |
|---|---|
| `aur_test.go` | 2 tests: respuesta OK contra un `httptest` (2 paquetes, nombres y versiones) y error ante HTTP 500 |

## Detalle de cada test

### `aur_test.go`

| Test | Qué fija |
|---|---|
| `TestAUR` | El camino feliz completo: el servidor devuelve un JSON válido con 2 paquetes; se verifica que vuelven 2 y que el primero es el esperado con su versión no vacía. Cambia `aurEndpoint` por el del `httptest` y lo restaura con `defer` |
| `TestAURError` | Un HTTP 500 produce error (no una lista vacía). Verifica que `AUR` devuelve `err != nil` ante un status distinto de 200 |

## Qué quedó sin cubrir (y por qué)

| Caso | Por qué no está | Consecuencia |
|---|---|---|
| **El RPC v5 devuelve `type:error` con HTTP 200** | Nadie miró ese campo: `aurResponse` solo declara `results` | **Bug A1 (ALTO).** El módulo no puede diferenciar "error del AUR" de "sin resultados". Verificado contra el AUR real: `curl "https://aur.archlinux.org/rpc/v5/info"` devuelve `{"error":"No request type/data specified.","resultcount":0,"results":[],"type":"error","version":5}` con HTTP 200 |
| **Un paquete pedido que no existe en el AUR** | El test usa un servidor que devuelve siempre todos los paquetes pedidos | **Bug A2 (MEDIO).** El paquete ausente desaparece de la lista sin error. Verificado con un `httptest` que devuelve 1 de los 2 pedidos |
| **JSON malformado** | El README lo reconoce (`README.md:88-90`): "lo cubre el error del decoder" | El decoder sí falla y `AUR` devuelve el error, pero **no hay test que lo fije**. Si alguien cambia el decode por un `json.Unmarshal` silencioso, nada lo detecta |
| **`names` vacío** | Es un `early return` trivial (`aur.go:32-34`), y el README lo justifica (`README.md:37-38`) | El contrato `(nil, nil)` está documentado pero no protegido por un test. Un cambio que lo convierta en `(nil, error)` pasaría inadvertido |
| **El `User-Agent` enviado** | Ningún test mira los headers | **Bug A3 (MEDIO).** Se manda `Go-http-client/1.1` (verificado). `internal/selfupdate` sí manda `b3l0w` (`internal/selfupdate/selfupdate.go:64`) |
| **El timeout de 8 s** | Requiere un servidor que se cuelgue: un test de 8 s es lento para el valor que aporta | No hay red de seguridad: si el timeout se sube a 5 minutos, nada lo detecta |
| **Nombres con caracteres especiales** (`+`, `@`, `.`) | `url.QueryEscape` los maneja y los nombres del AUR son Restricted-Package-Name | Riesgo bajo. El escape está aplicado (`aur.go:37`), verificado en el query enviado |
| **Consultas concurrentes** | El endpoint es una `var` global mutable | **Bug A4 (BAJO).** Dos `AUR` concurrentes con un test que cambie el endpoint se pisan. El README (`README.md:67-70`) ya lo advierte |

## Cómo correr la suite

```bash
cd /home/b3low-z3r0/Documents/Github/Cli/b3l0w-cli

go test ./internal/versioncheck/ -count=1 -cover

# Ver el código de salida del AUR por HTTP 200 (el bug A1):
curl -s "https://aur.archlinux.org/rpc/v5/info"
# {"error":"No request type/data specified.",...,"type":"error",...}
```
