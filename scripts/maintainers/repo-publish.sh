#!/usr/bin/env bash
# Publica paquetes en b3l0w-packages y firma la base de datos del repo.
#
#   bash scripts/maintainers/repo-publish.sh [--push] <paquete.pkg.tar.zst>...
#
# La firma se hace acá, en el equipo del mantenedor: `repo-add --sign`
# necesita la clave privada y eso nunca va a CI.
#
# Detalle que rompe pacman si se olvida: `repo-add` deja `b3l0w.db` y
# `b3l0w.files` como symlinks a los `.tar.gz`. En git se guardan como
# symlinks y raw.githubusercontent sirve el TEXTO del enlace, no el
# archivo. Por eso se reemplazan por copias reales.
set -euo pipefail

REPO="${B3L0W_PACKAGES:-$HOME/Documents/Github/b3l0w-packages}"
DEST="$REPO/x86_64"
PUSH=false
declare -a PKGS=()

for arg in "$@"; do
	case "$arg" in
	--push) PUSH=true ;;
	*.pkg.tar.zst) PKGS+=("$arg") ;;
	-h | --help)
		sed -n '2,12p' "$0"
		exit 0
		;;
	*)
		printf '✗ no entiendo: %s\n' "$arg" >&2
		exit 1
		;;
	esac
done

[ "${#PKGS[@]}" -gt 0 ] || {
	printf '✗ faltan paquetes\n' >&2
	exit 1
}
[ -d "$REPO" ] || {
	printf '✗ no existe %s\n' "$REPO" >&2
	exit 1
}

export GNUPGHOME="${B3L0W_GNUPGHOME:-$HOME/.gnupg-b3l0w}"
[ -d "$GNUPGHOME" ] || {
	printf '✗ no existe %s\n' "$GNUPGHOME" >&2
	exit 1
}

mkdir -p "$DEST"
cp -f "${PKGS[@]}" "$DEST/"
printf '▸ %d paquete(s) copiados a %s\n' "${#PKGS[@]}" "$DEST"

cd "$DEST"
printf '▸ firmando la base de datos (te va a pedir la passphrase)\n'
repo-add --sign --remove b3l0w.db.tar.gz ./*.pkg.tar.zst

# Symlinks -> copias reales, para que raw.githubusercontent los sirva.
cp -f b3l0w.db.tar.gz b3l0w.db
cp -f b3l0w.files.tar.gz b3l0w.files

printf '\n▸ contenido del repo\n'
ls -lh b3l0w.db b3l0w.db.sig b3l0w.files ./*.pkg.tar.zst ./*.sig 2>/dev/null || true

if $PUSH; then
	cd "$REPO"
	git add -A
	git commit -m "repo: $(date +%Y.%m.%d) — ${#PKGS[@]} paquete(s)"
	git push origin main
	printf '✓ publicado en https://raw.githubusercontent.com/Below-Z3R0/b3l0w-packages/main/x86_64\n'
else
	printf '\nPara publicar:\n  cd %s && git add -A && git commit -m "repo" && git push\n' "$REPO"
fi
