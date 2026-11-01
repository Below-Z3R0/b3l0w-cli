#!/usr/bin/env bash
# Copia la capa Luau compartida a cada proyecto de widget. `require` resuelve
# relativo al plugin, así que no se puede importar desde otro directorio.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
GITHUB="$(cd "$HERE/../../../.." && pwd)"
for proyecto in b3l0w-containers b3l0w-kanban b3l0w-audit b3l0w-audio b3l0w-anilist b3l0w-ytmusic; do
	[ -d "$GITHUB/$proyecto" ] || continue
	mkdir -p "$GITHUB/$proyecto/lib"
	cp "$HERE"/*.luau "$GITHUB/$proyecto/lib/"
	echo "  $proyecto/lib ← luau-shared"
done
