# CI/CD playbook — diagnóstico y reglas

Fecha: 2026-11-06. Este documento registra por qué falló el primer CI de Seanime, por qué el mismo
`docker run` funcionaba en local, y las reglas que aplican a los próximos workflows (Argos, Seanime,
paquetes de escritorio).

## 1. Qué pasó, run por run

Workflow: `.github/workflows/image.yml` de `Below-Z3R0/seanime-docker`.

| Run | Síntoma | Causa | Fix |
|---|---|---|---|
| `42131962863` | `chown: Operation not permitted` | El runner no es root (UID 1001); el `chown` a UID 1000 falla | `chown ... \|\| sudo chown ...` |
| `42132456361` | `curl: (56) Connection reset by peer` | El smoke test no pasaba `SEANIME_SERVER_HOST=0.0.0.0`; Seanime quedaba en `127.0.0.1` dentro del contenedor. Docker publica el puerto hacia la IP del contenedor, no hacia su loopback | Agregar `SEANIME_SERVER_HOST` y `SEANIME_SERVER_PORT` |
| `42133739633` | exit 1 a los 2 s | Las aserciones del host (`test -f "$tmp/data/.seeded"`, `curl /`) eran frágiles por permisos/HTTP | Chequear el seed con `docker exec`; comparar código HTTP explícito |
| `42134455999` | `rm: cannot remove ...: Operation not permitted` | El directorio temporal quedó owned por UID 1000; el runner es UID 1001 | `sudo rm -rf "$tmp"` |
| `42135065350` | **success** | — | Imagen publicada |

Resultado verificado:

```text
ghcr.io/below-z3r0/seanime:3.10.4
ghcr.io/below-z3r0/seanime:sha-d558762
ghcr.io/below-z3r0/seanime:latest
digest: sha256:e3690d0bfc10a807e5f9fc68fda27c0f885c480d837fe6605fbdec6bcb3c6263
```

## 2. Por qué en local funcionaba y en CI no

No era “el código anda distinto”; era el **contexto del runner**:

1. **UID**: en local el usuario puede `sudo chown` y borrar lo que creó. En GitHub Actions el job corre
   como UID 1001, y el contenedor como UID 1000. Un bind mount conserva el dueño del host.
2. **Loopback**: Seanime escucha en `127.0.0.1` si no se le pasa host. Dentro del contenedor eso es
   válido, pero Docker DNATea el puerto publicado a la interfaz del contenedor, no a su loopback.
   Localmente el primer test también dio `Connection reset` hasta que se pasó `0.0.0.0`; en CI faltaba
   esa variable.
3. **Bind mounts**: el directorio temporal creado por el runner pertenece al runner; si el proceso
   interno es UID 1000, hay que alinear permisos o usar volúmenes nombrados. `docker exec` evita
   depender de los permisos del host.
4. **Limpieza**: `rm -rf` del host falla si el directorio quedó con otro dueño; hay que limpiar con
   `sudo` o con un `trap` que use el mismo usuario del contenedor.

Regla práctica: **el smoke test debe correr con el mismo UID que el runner o usar un volumen nombrado**;
si el contenedor exige UID 1000, el host debe preparar y limpiar con `sudo`.

## 3. Reglas para los próximos workflows

### Build

- Construir una vez, probar, después pushear. No construir dos veces para “probar y publicar”.
- Para imágenes grandes o multi-arch, no usar `load: true`; usar `push: true` y probar tirando la
  imagen desde el registry, o separar build y smoke.
- Cache: `type=gha` tiene **10 GB y 7 días**; para capas grandes usar `type=registry` en GHCR.
- Ordenar el Dockerfile de lo estable a lo volátil y usar `--mount=type=cache` para apt/npm/go.
- Pinnear imágenes base por digest cuando importe la reproducibilidad.

### Smoke test

- `set -euo pipefail` + `trap` de limpieza.
- `docker run -d --name ...`, esperar healthcheck con timeout, no un `sleep` fijo.
- Verificar HTTP con código explícito (`-w '%{http_code}'`), no solo `curl -f` sobre `/`.
- Verificar el seed **dentro** del contenedor (`docker exec`), no el bind mount del host.
- En fallo, imprimir `docker ps -a` y `docker logs` antes de limpiar.
- Usar `$RUNNER_TEMP` para temporales, no `/tmp` compartido.
- Si el contenedor corre como UID 1000, preparar volúmenes con `sudo` y limpiar con `sudo`, o usar
  volúmenes nombrados.

### Seguridad

- `permissions:` mínimo; `contents: read` global y `packages: write` solo en el job que publica.
- Pinnear actions de terceros a **SHA completo**, no a `@v4`; Dependabot/Renovate mantiene los SHAs.
- No usar `pull_request_target` con checkout de código no confiable ni secretos.
- Preferir OIDC para clouds; no guardar credenciales de larga vida.
- Para releases: `environment` con aprobación manual y tags inmutables.

### Operación

- `concurrency` para cancelar runs viejos del mismo ref.
- `timeout-minutes` por job.
- `fail-fast: false` en matrices para ver todos los fallos.
- `workflow_dispatch` con inputs para versiones; no publicar `latest` desde cualquier commit.
- Runner estándar: 14 GB de disco. `jlumbroso/free-disk-space` libera hasta ~31 GB; verificar antes
  de builds de ORT/ROCm.
- Multi-arch: runner nativo (`ubuntu-24.04-arm`), tags por arquitectura y `docker buildx imagetools
  create` para el manifest final. QEMU solo para pruebas pequeñas.

### Calidad de imagen

- `docker/metadata-action` para tags/labels.
- SBOM + provenance (`sbom: true`, `provenance: true`) y, si se publica, firmar con cosign.
- Tags: versión upstream, `sha-<commit>`, `latest` solo para la última versión probada.

## 4. Referencias

- GitHub Actions limits (6 h/job, runners): https://docs.github.com/en/actions/reference/limits
- GitHub-hosted runners (14 GB SSD): https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- Secure use / pin SHA: https://docs.github.com/en/actions/reference/security/secure-use
- Docker cache en GitHub Actions: https://docs.docker.com/build/ci/github-actions/cache
- build-push-action: https://github.com/docker/build-push-action
- Multi-platform y manifests: https://docs.docker.com/build/ci/github-actions/multi-platform
- Runner UID/bind mounts: https://github.com/actions/runner/issues/2411
- free-disk-space: https://github.com/jlumbroso/free-disk-space

## 5. Estado actual

- Seanime amd64: **verde**, imagen en GHCR.
- Seanime arm64: workflow listo, sin ejecutar.
- Argos amd64/arm64: workflows listos, sin ejecutar.
- Desktop: workflow y PKGBUILDs listos; falta pushear `b3l0w-cli` y ejecutar.
- Pendiente de hardening: pinnear actions por SHA, agregar `concurrency`, SBOM/provenance y firma de
  paquetes.
