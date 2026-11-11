# verify

Verificación de firmas GPG y checksums SHA-256 de los releases de
b3l0w.

## Qué garantiza

Que un asset vine firmado por la clave de release de b3l0w.

La confianza no viene de que el archivo esté en GitHub: viene de la
firma. Un release comprometido puede cambiar el binario y el
SHA256SUMS juntos; no puede producir una firma válida sin la clave
privada, que nunca sale del equipo del mantenedor.

## Uso

```go
// Verificación completa: firma de SHA256SUMS + hash del asset.
// Es el único camino que debería usar el instalador.
err := verify.Sums(sumsPath, sigPath, "b3l0w-0.5.1-linux-amd64", assetPath)
```

## API

| Función | Qué hace |
|---|---|
| `Sums(sums, sig, nombre, asset)` | Verificación completa: firma + hash |
| `Signature(sums, sig)` | Verifica la firma de SHA256SUMS |
| `CheckSum(sums, nombre, asset)` | Verifica el hash contra la línea del SHA256SUMS |
| `SHA256(path)` | Hash de un archivo en hex minúscula |
| `PublicKey()` | La clave pública embebida |
| `SetKey(pub, fpr)` | Cambia la clave y devuelve cómo restaurarla. Solo tests |

## El keyring temporal

`Signature` importa la clave en un keyring temporal, no en el del
usuario. Si usara el del usuario, un keyring contaminado podría hacer
pasar una firma ajena. En el temporal solo existe nuestra clave.

## La clave

```
Fingerprint: 9AE2C00AD3A9A64706A4335CC6E1D74A07833042
```

La privada vive en `~/.gnupg-b3l0w` y nunca sale del equipo del
mantenedor. La pública va embebida en el binario (`b3l0w.asc`).

## Tests

8 tests: firma válida, manipulación detectada, otra clave rechazada,
checksum correcto, hash incorrecto, asset ausente, formato binario y
la verificación completa.
