#!/usr/bin/env bash
# Compara el último release desktop-* con lo publicado en b3l0w-packages.
#
#   bash scripts/maintainers/repo-sync.sh --check   # solo informa
#   bash scripts/maintainers/repo-sync.sh           # baja y publica
#   bash scripts/maintainers/repo-sync.sh --push    # además hace git push
#
# Es local a propósito: primero actualizás y probás la versión nueva,
# revisás qué se rompió en la config, y recién ahí publicás.
set -euo pipefail

REPO_CLI="Below-Z3R0/b3l0w-cli"
PACKAGES="${B3L0W_PACKAGES:-$HOME/Documents/Github/b3l0w-packages}"
DEST="$PACKAGES/x86_64"
CHECK=false
PUSH=false

for arg in "$@"; do
	case "$arg" in
	--check) CHECK=true ;;
	--push) PUSH=true ;;
	-h | --help)
		sed -n '2,10p' "$0"
		exit 0
		;;
	*)
		printf '✗ no entiendo: %s\n' "$arg" >&2
		exit 1
		;;
	esac
done

[ -d "$DEST" ] || {
	printf '✗ no existe %s\n' "$DEST" >&2
	exit 1
}

printf '▸ Último release desktop-*\n'
TAG="$(gh release list --repo "$REPO_CLI" --limit 30 \
	--json tagName,createdAt \
	--jq '[.[] | select(.tagName | startswith("desktop-"))][0].tagName')"
[ -n "$TAG" ] && [ "$TAG" != "null" ] || {
	printf '✗ no hay releases desktop-*\n' >&2
	exit 1
}
printf '  %s\n' "$TAG"

printf '\n▸ Paquetes del release\n'
ASSETS="$(gh release view "$TAG" --repo "$REPO_CLI" --json assets \
	--jq '.assets[].name | select(endswith(".pkg.tar.zst"))')"

faltan=0
for asset in $ASSETS; do
	if [ -f "$DEST/$asset" ]; then
		printf '  ✓ %s\n' "$asset"
	else
		printf '  ✗ falta %s\n' "$asset"
		faltan=$((faltan + 1))
	fi
done

if [ "$faltan" -eq 0 ]; then
	printf '\n✓ %s ya está publicado en [b3l0w]\n' "$TAG"
	exit 0
fi

if $CHECK; then
	printf '\n%s paquete(s) sin publicar de %s\n' "$faltan" "$TAG"
	exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

printf '\n▸ Bajando %d paquete(s)\n' "$faltan"
gh release download "$TAG" --repo "$REPO_CLI" --dir "$TMP" -p "*.pkg.tar.zst"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
printf '\n▸ Publicando (firma + repo-add)\n'
if $PUSH; then
	bash "$SCRIPT_DIR/repo-publish.sh" --push "$TMP"/*.pkg.tar.zst
else
	bash "$SCRIPT_DIR/repo-publish.sh" "$TMP"/*.pkg.tar.zst
fi
