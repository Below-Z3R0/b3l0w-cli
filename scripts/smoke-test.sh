#!/usr/bin/env bash
# Smoke test del CLI: no toca el sistema, no pide root.
#
#   bash scripts/smoke-test.sh [ruta-al-binario]
#
# Verifica los códigos de salida y que la ayuda/versión sean coherentes.
# Lo que sí modifica el sistema (update) no se corre acá.

set -uo pipefail

BIN="${1:-./b3l0w}"
FAILS=0
CHECKS=0

# fails "descripción" "comando" "código esperado"
check() {
  local desc="$1" cmd="$2" want="$3"
  CHECKS=$((CHECKS + 1))
  local out
  out=$(eval "$cmd" 2>&1)
  local got=$?
  if [ "$got" != "$want" ]; then
    echo "  ✗ $desc (exit $got, esperaba $want)"
    echo "    $out" | head -2 | sed 's/^/    /'
    FAILS=$((FAILS + 1))
  else
    echo "  ✓ $desc"
  fi
}

echo "▸ smoke test: $BIN"

check "version sale 0"            "$BIN version" 0
check "version tiene formato"     "$BIN version | grep -qE '^b3l0w (v[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9]+)?|dev)$'" 0
check "help sale 0"               "$BIN --help" 0
check "help menciona update"      "$BIN --help | grep -q 'update --programs'" 0
check "list sale 0"               "$BIN list" 0
check "plugins sale 0"            "$BIN plugins" 0
check "plugins list sale 0"       "$BIN plugins list" 0
check "plugins remove sin id falla" "$BIN plugins remove" 1
check "check sale 0"              "$BIN check --keep=100" 0
check "channel sale 0"            "$BIN channel" 0
check "help menciona plugins"     "$BIN --help | grep -q 'update --plugins'" 0
check "doctor corre"              "$BIN doctor >/dev/null 2>&1; [ \$? -le 1 ]" 0
check "comando desconocido falla" "$BIN no-existe" 1
check "--system y --programs son excluyentes" "$BIN update --system --programs" 1
check "opción desconocida falla"  "$BIN update --nada" 1
check "program sin args falla"    "$BIN program" 1
check "program remove inexistente falla" "$BIN program remove no-existe-$$" 1

# Sin TTY (como en este script), el update del sistema tiene que negarse.
if [ ! -t 0 ]; then
  check "update --system sin TTY cancela" "$BIN update --system" 0
fi

echo ""
if [ "$FAILS" -gt 0 ]; then
  echo "  $FAILS de $CHECKS fallaron"
  exit 1
fi
echo "  $CHECKS/$CHECKS ok"
