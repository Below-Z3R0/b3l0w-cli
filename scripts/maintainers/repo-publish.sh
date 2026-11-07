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

# `repo-add --sign` firma la BASE DE DATOS, no los paquetes. Con
# SigLevel = Required, pacman exige un .sig por paquete: hay que
# firmarlos antes.
printf '▸ firmando cada paquete (te va a pedir la passphrase)\n'
for pkg in ./*.pkg.tar.zst; do
	if [ ! -f "$pkg.sig" ]; then
		gpg --detach-sign "$pkg"
	fi
done

printf '▸ firmando la base de datos\n'
repo-add --sign --remove b3l0w.db.tar.gz ./*.pkg.tar.zst

# `repo-add` deja symlinks y `cp -f` los sigue (escribe en el destino,
# no reemplaza el enlace). Hay que borrarlos primero: raw.githubusercontent
# sirve el TEXTO del symlink y pacman recibiría la ruta, no la DB.
rm -f b3l0w.db b3l0w.db.sig b3l0w.files b3l0w.files.sig
cp b3l0w.db.tar.gz b3l0w.db
cp b3l0w.db.tar.gz.sig b3l0w.db.sig
cp b3l0w.files.tar.gz b3l0w.files
cp b3l0w.files.tar.gz.sig b3l0w.files.sig

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
