#!/usr/bin/env bash
# Firma el SHA256SUMS de un release y lo sube.
#
# La firma se hace acá, en el equipo del mantenedor, y no en CI: la
# clave privada nunca sale de ~/.gnupg-b3l0w.
#
# Uso:
#   bash scripts/maintainers/sign-release.sh <repo> <tag> [asset...]
#
# Ejemplo:
#   bash scripts/maintainers/sign-release.sh Below-Z3R0/kitty-shapes v1.0.1
#
# Sin assets, firma todos los archivos del release.
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

paso "Bajando los assets de $REPO@$TAG"
if [ "$#" -gt 0 ]; then
	for asset in "$@"; do
		gh release download "$TAG" --repo "$REPO" --pattern "$asset" --clobber
	done
else
	gh release download "$TAG" --repo "$REPO" --clobber
fi
# No se firman las firmas ni el propio SHA256SUMS.
rm -f SHA256SUMS SHA256SUMS.asc
ASSETS=(*)
[ "${#ASSETS[@]}" -gt 0 ] || mal "el release no tiene assets"
ok "${#ASSETS[@]} asset(s)"

paso "Calculando SHA256SUMS"
sha256sum "${ASSETS[@]}" >SHA256SUMS
cat SHA256SUMS | sed 's/^/  /'

paso "Firmando (te va a pedir la passphrase)"
gpg --yes --detach-sign --armor SHA256SUMS
gpg --verify SHA256SUMS.asc SHA256SUMS 2>&1 | sed 's/^/  /'

paso "Subiendo al release"
gh release upload "$TAG" SHA256SUMS SHA256SUMS.asc --repo "$REPO" --clobber
ok "publicado en https://github.com/$REPO/releases/tag/$TAG"

cat <<EOF

Ahora el CLI puede verificar:
     b3l0w install <programa>
EOF
