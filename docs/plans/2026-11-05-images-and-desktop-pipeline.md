# Plan: imágenes de servicios + paquetes de escritorio

Estado: plan operativo para la sesión del CLI. Fecha: 2026-11-05.

## Decisiones

- **Argos y Seanime** se distribuyen como imágenes Docker en GHCR. El usuario final no compila.
- **Noctalia, Umbriel y xdg-desktop-portal-umbriel** se distribuyen como paquetes Arch en
  **GitHub Releases**. Por ahora **no hay repo pacman**: sin `repo-add`, sin DB y sin `gh-pages`.
- Los paquetes propios llevan el sufijo **`-b3l0w`** para distinguirse de AUR.
- **Denshi no se hostea**: el usuario descarga el AppImage oficial de Seanime. El CLI puede
  verificar el checksum o abrir el link, pero no redistribuye el binario.
- Noctalia/Umbriel/portal **no van en contenedor**: corren en la sesión Wayland del host.

## Artefactos

### Argos

```text
ghcr.io/below-z3r0/argos:cpu-<sha>       amd64 (y arm64 más adelante)
ghcr.io/below-z3r0/argos:gpu-<sha>       NVIDIA, amd64
ghcr.io/below-z3r0/argos:intel-<sha>     Intel/OpenVINO, amd64
ghcr.io/below-z3r0/argos:amd-<sha>       AMD/ROCm-MIGraphX, amd64
```

Workflows: `.github/workflows/images-amd64.yml` y `images-arm64.yml` del repo Argos.
La base de ORT se cachea en GHCR con `type=registry`; no se usa el cache de Actions.

### Seanime

```text
ghcr.io/below-z3r0/seanime:<version>
ghcr.io/below-z3r0/seanime:sha-<commit>
ghcr.io/below-z3r0/seanime:latest
```

Workflow: `.github/workflows/image.yml` del repo `b3l0w-seanime`. Construye desde el tag exacto
de `5rahim/seanime`, corre smoke test y publica. `compose.yaml` usa solo `image:`, sin `build:`.

### Paquetes de escritorio

Release tag: `desktop-YYYY.MM.DD-<run>`.

Assets:

```text
noctalia-b3l0w-<pkgver>-<pkgrel>-x86_64.pkg.tar.zst
umbriel-b3l0w-<pkgver>-<pkgrel>-x86_64.pkg.tar.zst
xdg-desktop-portal-umbriel-b3l0w-<pkgver>-<pkgrel>-x86_64.pkg.tar.zst
manifest.json
```

`manifest.json`:

```json
{
  "release": "desktop-2026.11.05-123",
  "packages": {
    "noctalia-b3l0w": {
      "version": "5.2.1.r5789.gb409c9016-1",
      "commit": "b409c9016",
      "url": "https://github.com/Below-Z3R0/b3l0w-cli/releases/download/...",
      "sha256": "..."
    }
  }
}
```

Workflow: `.github/workflows/desktop-packages.yml` del repo del CLI. PKGBUILDs vendorizados en
`packaging/<paquete>/PKGBUILD`.

## Contrato del CLI

### Argos

```text
b3l0w argos install [--variant auto|cpu|nvidia|intel|amd]
b3l0w argos update
b3l0w argos rollback
b3l0w argos status
```

- Detectar CPU/GPU por sysfs; recomendar variante; `--variant` siempre gana.
- `install`: `docker pull` de la imagen; escribir `ARGOS_IMAGE` en el `.env` del compose.
- `update`: comparar sha/digest; pull; recrear; guardar tag anterior.
- `rollback`: usar el tag anterior guardado.
- No construir salvo que no exista imagen publicada.

### Seanime

```text
b3l0w seanime install [--version X]
b3l0w seanime update
b3l0w seanime rollback
b3l0w seanime denshi      # link oficial + checksum, sin hostear
```

- `install`: pull de la imagen, crear carpetas `data/ anime/ downloads/`, `compose up -d`.
- `update`: `SEANIME_VERSION` nuevo, pull, recrear.
- `denshi`: abrir el release oficial de Denshi; no descargar de un espejo propio.

### Desktop

```text
b3l0w desktop install noctalia|umbriel|portal
b3l0w desktop update
b3l0w desktop rollback <paquete>
b3l0w desktop status
```

Orden de instalación: **portal → umbriel → noctalia**.

- Leer `manifest.json` del último release.
- Verificar SHA-256 antes de instalar.
- Detectar paquetes de AUR en conflicto (`noctalia-git`, `umbriel-git`,
  `xdg-desktop-portal-umbriel-git`) y pedir confirmación para removerlos.
- Instalar con `pacman -U --noconfirm`; resolver dependencias con los repos oficiales.
- Guardar versión/commit y el `.pkg.tar.zst` anterior para rollback.
- No ejecutar `pacman -Syu` completo sin confirmación.

## Pruebas que quedan para la sesión del CLI (VM)

El CI valida empaquetado y arranque, pero **no una sesión Wayland real**. Eso se prueba en VM:

1. Crear VM Arch/CachyOS con GPU virtual (virtio-gpu/llvmpipe) y sesión greetd.
2. Instalar los tres paquetes `-b3l0w` desde el release.
3. Verificar:
   - Umbriel arranca como compositor y abre una terminal;
   - Noctalia arranca sobre Umbriel (barra, launcher, lock screen);
   - el portal responde a Screenshot, ScreenCast e InputCapture;
   - logout/login y reinicio de sesión;
   - `b3l0w desktop update` y `rollback` dejan la sesión funcional.
4. Probar también:
   - Argos `install/update/rollback` en Docker;
   - Seanime `install/update` con volúmenes persistentes;
   - conflicto con paquetes AUR y su remoción.

Criterio de aceptación: sesión gráfica usable tras instalar, actualizar y volver atrás; sin
`quickshell` instalado; paquetes `-b3l0w` visibles en `pacman -Q`.

## Multi-arquitectura

- Seanime: amd64 primero; arm64 después con runner `ubuntu-24.04-arm` y manifest.
- Argos: `cpu` puede ser amd64 + arm64; `gpu`, `intel` y `amd` quedan amd64.
- Desktop: amd64 primero; aarch64 es un build separado con Arch ARM y no se mezcla con x86_64.

## Riesgos / pendientes

- No hay firma GPG de paquetes todavía: se verifica SHA-256; agregar `.sig` antes de considerar
  el canal estable.
- No hay repo pacman: `pacman -Syu` no actualiza estos paquetes; el CLI es el dueño del ciclo.
- Los paquetes son `-git`: upstream puede romper; publicar solo después del smoke test y conservar
  versiones anteriores.
- El primer build de ORT Intel/AMD puede ser largo; la base se cachea/publica una vez.
- Denshi es un AppImage oficial; no se modifica ni se redistribuye.
