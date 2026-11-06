#!/usr/bin/env bash
# Genera la clave de firma de b3l0w en un GNUPGHOME dedicado.
#
# Por qué un GNUPGHOME aparte: la clave de firma de paquetes no es la
# clave personal. Mezclarlas hace que un backup, una rotación o un
# compromiso arrastre las dos. Acá vive sola, con permisos 700.
#
# La clave privada NO sale de este equipo. El script no la imprime, no
# la sube y no la escribe en claro: el backup va cifrado con passphrase.
#
# Uso:
#   bash scripts/gen-signing-key.sh
#
# Después de correrlo:
#   1. Mové el backup cifrado a un lugar offline.
#   2. Mové el certificado de revocación a un lugar offline distinto.
#   3. Publicá la clave pública y el fingerprint en el repo.
set -euo pipefail

NOMBRE="${B3L0W_KEY_NAME:-b3l0w packages}"
EMAIL="${B3L0W_KEY_EMAIL:-paquetes@b3l0w.local}"
DIAS="${B3L0W_KEY_DAYS:-1825}"   # 5 años
SALIDA="${B3L0W_KEY_OUT:-$PWD/keys}"

export GNUPGHOME="${B3L0W_GNUPGHOME:-$HOME/.gnupg-b3l0w}"

paso() { printf '\n▸ %s\n' "$1"; }
ok()   { printf '  ✓ %s\n' "$1"; }
mal()  { printf '  ✗ %s\n' "$1" >&2; exit 1; }

command -v gpg >/dev/null || mal "falta gpg"

if [ -d "$GNUPGHOME" ] && [ -n "$(ls -A "$GNUPGHOME" 2>/dev/null)" ]; then
	mal "$GNUPGHOME ya tiene claves. Si querés rotar, movelo antes:
       mv $GNUPGHOME $GNUPGHOME.viejo"
fi

mkdir -p "$GNUPGHOME" "$SALIDA"
chmod 700 "$GNUPGHOME"

paso "Generando la clave (te va a pedir una passphrase)"
# Solo capacidad de firma: no cifra ni autentica, así tiene menos uso y
# menos superficie. RSA 4096 porque pacman lo acepta en cualquier
# versión; ed25519 es más moderna pero no todos los pacman viejos.
#
# Nada de --batch ni --passphrase '': una clave de firma sin passphrase
# es un archivo que cualquiera copia y usa. El pinentry la pide.
gpg --quick-generate-key "$NOMBRE <$EMAIL>" rsa4096 sign "$DIAS"

FPR="$(gpg --list-keys --with-colons "$EMAIL" | awk -F: '/^fpr/{print $10; exit}')"
[ -n "$FPR" ] || mal "no pude leer el fingerprint"

ok "clave creada"
printf '\n  Fingerprint: %s\n' "$FPR"
printf '  Vence: %s\n' "$(gpg --list-keys --with-colons "$EMAIL" | awk -F: '/^pub/{print strftime("%Y-%m-%d", $7)}')"

paso "Exportando la clave pública"
gpg --armor --export "$FPR" >"$SALIDA/b3l0w.asc"
ok "$SALIDA/b3l0w.asc"

paso "Guardando el certificado de revocación"
# Si la clave se pierde o se compromete, esto es lo único que permite
# avisar a los usuarios. Va a un lugar offline, separado del backup.
gpg --gen-revoke "$FPR" >"$SALIDA/b3l0w-revoke.asc"
[ -s "$SALIDA/b3l0w-revoke.asc" ] && ok "$SALIDA/b3l0w-revoke.asc"

paso "Backup cifrado de la clave privada"
printf '  Te va a pedir la passphrase de la clave y después una para el backup.\n'
gpg --export-secret-keys --armor "$FPR" | gpg -c >"$SALIDA/b3l0w-secret.asc.gpg"
chmod 600 "$SALIDA/b3l0w-secret.asc.gpg"
ok "$SALIDA/b3l0w-secret.asc.gpg"

cat <<EOF

────────────────────────────────────────────────────────────
Listo. Ahora, en este orden:

1. Mové estos dos archivos OFFLINE (USB + papel, dos lugares):
     $SALIDA/b3l0w-secret.asc.gpg      ← backup de la clave
     $SALIDA/b3l0w-revoke.asc          ← revocación

2. Publicá la clave pública en el repo:
     cp $SALIDA/b3l0w.asc  <repo>/keys/b3l0w.asc
     Y el fingerprint en el README:
     $FPR

3. Borrá el backup del disco cuando esté offline.

Para firmar un release:
     export GNUPGHOME=$GNUPGHOME
     gpg --detach-sign --armor SHA256SUMS
     gpg --verify SHA256SUMS.asc SHA256SUMS
────────────────────────────────────────────────────────────
EOF
