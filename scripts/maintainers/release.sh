#!/usr/bin/env bash
# Publica un release ya firmado.
#
# Por qué firmar antes de subir: si la firma fuera un paso posterior, un
# corte de red o un cierre de sesión dejaría el release publicado sin
# SHA256SUMS.asc, y `b3l0w install` se niega a instalarlo (fail-closed).
# Con este script el release nunca existe sin firma: si la firma falla,
# no se sube nada.
#
# La firma se hace acá, en el equipo del mantenedor, y no en CI: la clave
# privada nunca sale de ~/.gnupg-b3l0w.
#
# Uso:
#   bash scripts/maintainers/release.sh <repo> <tag> <archivo>...
#
# Ejemplo:
#   bash scripts/maintainers/release.sh Below-Z3R0/kitty-shapes v1.0.2 dist/kitty-shapes-linux-x64
#
# Si el release ya existe, solo sube la firma y los hashes (--firma).
set -euo pipefail

REPO="${1:?falta el repo: owner/nombre}"
TAG="${2:?falta el tag: v1.0.0}"
shift 2

export GNUPGHOME="${B3L0W_GNUPGHOME:-$HOME/.gnupg-b3l0w}"

paso() { printf '\n▸ %s\n' "$1"; }
ok()   { printf '  ✓ %s\n' "$1"; }
mal()  { printf '  ✗ %s\n' "$1" >&2; exit 1; }

command -v gh >/dev/null || mal "falta gh"
command -v gpg >/dev/null || mal "falta gpg"
[ -d "$GNUPGHOME" ] || mal "no existe $GNUPGHOME"

TRABAJO="$(mktemp -d)"
trap 'rm -rf "$TRABAJO"' EXIT
cd "$TRABAJO"

if [ "$#" -gt 0 ]; then
  ASSETS=()
  for asset in "$@"; do
    [ -f "$asset" ] || mal "no existe el asset: $asset"
    cp "$asset" .
    ASSETS+=("$(basename "$asset")")
  done
else
  mal "uso: release.sh <repo> <tag> <archivo>..."
fi

paso "Calculando SHA256SUMS"
sha256sum "${ASSETS[@]}" >SHA256SUMS
sed 's/^/  /' SHA256SUMS

paso "Firmando (te va a pedir la passphrase)"
gpg --yes --detach-sign --armor SHA256SUMS
gpg --verify SHA256SUMS.asc SHA256SUMS 2>&1 | sed 's/^/  /'

paso "Publicando $REPO@$TAG"
if gh release view "$TAG" --repo "$REPO" >/dev/null 2>&1; then
  mal "el release $TAG ya existe: usá sign-release.sh para firmarlo"
fi
gh release create "$TAG" \
  "${ASSETS[@]}" SHA256SUMS SHA256SUMS.asc \
  --repo "$REPO" \
  --title "$TAG" \
  --generate-notes
ok "publicado y firmado: https://github.com/$REPO/releases/tag/$TAG"

cat <<EOF

Verificá que quedó bien:
     b3l0w doctor
EOF
