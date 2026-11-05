#!/usr/bin/env bash
# Greeting de b3l0wz3r0: fastfetch con el emblema de kitty-shapes.
#
# El emblema es el avatar de Noctalia recortado con una forma Material 3. El
# corte cuesta ~140 ms, así que se cachea: la primera terminal lo genera, las
# siguientes lo reutilizan (~1 ms) y en segundo plano se prepara la próxima.
#
# El greeting queda vivo solo unos segundos (FASTFETCH_RERENDER_GRACE): si
# redimensionás en ese lapso se redibuja adaptándose; pasado el plazo el
# watcher se apaga y no vuelve a tocar la pantalla, así que lo que hayas
# escrito queda intacto.
#
# Fuera de kitty: chafa si está, si no el logo por defecto.
set -uo pipefail

# ── Rutas y ajustes ─────────────────────────────────────────────────────────
config="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/config.jsonc"
config_min="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/config-min.jsonc"
config_micro="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/config-micro.jsonc"
shapes_bin="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/kitty-shapes"
shapes_list="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/emblem-shapes.txt"
fallback_emblem="${XDG_CONFIG_HOME:-$HOME/.config}/fastfetch/emblem/current.png"
cache_dir="${XDG_RUNTIME_DIR:-/tmp}/fastfetch-cache"
cached_emblem="$cache_dir/emblem.png"
cached_stamp="$cache_dir/stamp"
logo_gap=4                              # aire entre imagen y keys
logo_min=4                              # ancho mínimo útil para la imagen
grace=${FASTFETCH_RERENDER_GRACE:-15}   # segundos en que se permite redibujar
case "$grace" in ''|*[!0-9]*) grace=15 ;; esac   # basura → default
# compatibilidad con la variable vieja: FASTFETCH_RERENDER=0 lo desactiva
[ "${FASTFETCH_RERENDER:-1}" = "0" ] && grace=0

is_kitty=0
if [ -n "${KITTY_WINDOW_ID:-}" ] || [ "${TERM:-}" = "xterm-kitty" ]; then
  is_kitty=1
fi

# ── Medidas de la terminal ──────────────────────────────────────────────────
# `tput cols` sin TTY devuelve 80 (default de terminfo); el kernel no miente.
term_size() {
  local size r c
  size=$( { stty size < /dev/tty; } 2>/dev/null )
  r="${size%% *}"
  c="${size#* }"
  [ "$c" = "$size" ] && c=""                  # sin espacio = salida inválida
  [ -z "$c" ] && c=$(tput cols 2>/dev/null)
  case "$r" in ''|*[!0-9]*) r=40 ;; esac
  case "$c" in ''|*[!0-9]*) c=100 ;; esac
  TERM_ROWS=$r
  TERM_COLS=$c
}

# Ancho y alto (en líneas) del bloque de texto de un config.
module_size() {
  local cfg="${1:-$config}"
  fastfetch --config "$cfg" --logo-type none --pipe false 2>/dev/null |
    awk '{ gsub(/\033\[[0-9;?]*[a-zA-Z]/, ""); if (length($0) > w) w = length($0); n++ }
         END { print w + 0, n + 0 }'
}

# ── Layout ──────────────────────────────────────────────────────────────────
# Filas que ocupa una imagen de `w` celdas de ancho. El emblema es 288×384 y la
# celda es más alta que ancha: ~0,8 filas por columna (medido). Se redondea
# para arriba para no quedarse corto.
img_rows() { echo $(( ($1 * 8 + 9) / 10 + 1 )); }

# ¿Entra la imagen con este texto? Devuelve "side:N" o "stack:N" (N = ancho
# del logo) o nada.
fit_with() {
  local c=$1 r=$2 tw=$3 tl=$4 logo ir
  # 1) al lado del texto
  logo=$(( c - tw - logo_gap ))
  [ "$logo" -gt "$logo_full" ] && logo=$logo_full
  if [ "$logo" -ge "$logo_min" ] && [ "$r" -ge "$tl" ]; then
    ir=$(img_rows "$logo")
    [ "$r" -ge "$ir" ] && { echo "side:$logo"; return; }
  fi
  # 2) arriba del texto
  logo=$(( (r - tl - 1) * 10 / 8 ))
  [ "$logo" -gt "$logo_full" ] && logo=$logo_full
  [ "$logo" -gt $(( c - 2 )) ] && logo=$(( c - 2 ))
  if [ "$logo" -ge "$logo_min" ]; then
    ir=$(img_rows "$logo")
    [ "$r" -ge $(( tl + ir )) ] && { echo "stack:$logo"; return; }
  fi
}

# Elige config y disposición: prueba los textos de mayor a menor (completo,
# compacto, micro) y para cada uno intenta imagen al lado y arriba. Si ningún
# texto deja entrar la imagen, usa el más chico que entre sin imagen.
decide_layout() {
  local c=$1 r=$2 name tw tl got
  lmode=none; logo_w=0; text_w=0; cfg_active="$config"
  for cand in "full:$tw_full:$tl_full" "min:$tw_min:$tl_min" "micro:$tw_micro:$tl_micro"; do
    IFS=: read -r name tw tl <<< "$cand"
    [ "${tw:-0}" -gt 0 ] && [ "${tl:-0}" -gt 0 ] || continue
    case "$name" in
      min)   [ -f "$config_min" ]   || continue; cfg_arg="$config_min" ;;
      micro) [ -f "$config_micro" ] || continue; cfg_arg="$config_micro" ;;
      *)     cfg_arg="$config" ;;
    esac
    got=$(fit_with "$c" "$r" "$tw" "$tl")
    if [ -n "$got" ]; then
      lmode=${got%%:*}
      logo_w=${got##*:}
      text_w=$tw
      cfg_active=$cfg_arg
      return
    fi
  done
  # Sin imagen: el texto más grande que entre (full → min → micro).
  for cand in "full:$tw_full:$tl_full:$config" "min:$tw_min:$tl_min:$config_min" \
              "micro:$tw_micro:$tl_micro:$config_micro"; do
    IFS=: read -r name tw tl cfg_arg <<< "$cand"
    [ "${tw:-0}" -gt 0 ] && [ "${tl:-0}" -gt 0 ] || continue
    [ -f "$cfg_arg" ] || continue
    if [ "$c" -ge "$tw" ] && [ "$r" -ge "$tl" ]; then
      lmode=text; text_w=$tw; cfg_active=$cfg_arg
      return
    fi
  done
  lmode=none
}

# Traduce lmode/logo_w/text_w a flags de fastfetch.
apply_layout() {
  local c=$1 block pad bw lado
  indent=0
  skip_draw=0
  case "$lmode" in
    side)
      block=$(( logo_w + logo_gap + text_w ))
      pad=$(( (c - block) / 2 ))
      [ "$pad" -lt 0 ] && pad=0
      logo_args=(--logo-width "$logo_w"
                 --logo-padding-left "$pad"
                 --logo-padding-right "$logo_gap")
      ;;
    stack)
      bw=$logo_w
      [ "$text_w" -gt "$bw" ] && bw=$text_w
      indent=$(( (c - bw) / 2 ))
      [ "$indent" -lt 0 ] && indent=0
      lado=$(( indent + (bw - logo_w) / 2 ))
      [ "$lado" -lt 0 ] && lado=0
      logo_args=(--logo-position top --logo-padding-bottom 1
                 --logo-padding-left "$lado" --logo-width "$logo_w")
      ;;
    text)
      logo_args=(--logo-type none)
      ;;
    *)
      logo_args=(--logo-type none)
      skip_draw=1
      ;;
  esac
}

# Antepone la imagen al layout si hay imagen y el layout la admite.
with_logo() {
  if [ -n "${1:-}" ] && [ "${logo_w:-0}" -gt 0 ]; then
    logo_args=(--logo "$1" --logo-type kitty-direct "${logo_args[@]}")
  fi
  return 0
}

# ── Dibujo ──────────────────────────────────────────────────────────────────
# El bloque se dibuja siempre en (1,1). El filtro convierte el CUP absoluto de
# fastfetch (`ESC[2;NH`) en un avance de columna y, en modo apilado, corre el
# texto y sus columnas absolutas (`ESC[NG`). En re-render se quitan `ESC[2J` y
# `ESC[3J` porque el borrado lo hace render().
draw() {
  if [ "$is_kitty" = "1" ]; then
    fastfetch --config "$cfg_active" --pipe false "${logo_args[@]}" "$@" |
      FF_PAD="${indent:-0}" FF_NOCLEAR="${suppress_clear:-0}" perl -ne '
        BEGIN { $p = $ENV{FF_PAD} + 0; $nc = $ENV{FF_NOCLEAR} eq "1" }
        if ($nc) { s/\e\[2J//g; s/\e\[3J//g; }
        s/\e\[2;(\d+)H/"\e[" . ($1 - 1) . "C"/ge;
        if (index($_, "\e_G") >= 0) { print; next }
        if ($p > 0) {
          s/^/" " x $p/e;
          s/\e\[(\d+)G/"\e[" . ($1 + $p) . "G"/ge;
        }
        print'
  elif command -v chafa >/dev/null 2>&1; then
    fastfetch --config "$cfg_active" --logo-type chafa "$@"
  else
    fastfetch --config "$cfg_active" --logo-type none "$@"
  fi
}

render() {
  if [ "${suppress_clear:-0}" = "1" ]; then
    printf '\x1b[2J'          # borra pantalla e imágenes; el scrollback queda
  fi
  printf '\x1b[1;1H'
  [ "$skip_draw" = "1" ] || draw "$@"
}

# ── Emblema ─────────────────────────────────────────────────────────────────
# Elige la PRÓXIMA forma en rotación (nunca repite la anterior) y la deja en
# `SHAPE`. Con 8 formas, abrir 8 terminales las muestra todas una vez.
next_shape() {
  local names=() idx=0
  [ -f "$shapes_list" ] &&
    mapfile -t names < <(grep -vE '^[[:space:]]*(#|$)' "$shapes_list" 2>/dev/null)
  [ "${#names[@]}" -gt 0 ] || names=(cookie12 hexagon octagon pentagon gem sunny arch semiCircle)
  [ -f "$cache_dir/shape-idx" ] && read -r idx < "$cache_dir/shape-idx" 2>/dev/null
  case "$idx" in ''|*[!0-9]*) idx=0 ;; esac
  idx=$(( (idx + 1) % ${#names[@]} ))
  printf '%s\n' "$idx" > "$cache_dir/shape-idx.tmp.$$" 2>/dev/null &&
    mv -f "$cache_dir/shape-idx.tmp.$$" "$cache_dir/shape-idx" 2>/dev/null
  SHAPE="${names[$idx]}"
}

# Recorta el avatar en `$1` (~140 ms) con `SHAPE` (o al azar si no hay).
crop_emblem() {
  local shape_args2=()
  [ -n "${SHAPE:-}" ] && shape_args2=(--shape "$SHAPE")
  "$shapes_bin" pick "$source_img" --size 384 --palette 256 \
    "${shape_args[@]}" "${shape_args2[@]}" --path --out "$1" >/dev/null 2>&1
}

# Publica `$1` como cache: primero el PNG, después el stamp (si el proceso
# muere en el medio, queda un cache sin stamp y se regenera solo).
publish_emblem() {
  mkdir -p "$cache_dir" 2>/dev/null || true
  cp -f "$1" "$cache_dir/emblem.tmp.$$" 2>/dev/null || return 0
  mv -f "$cache_dir/emblem.tmp.$$" "$cached_emblem" 2>/dev/null || true
  if printf '%s\n' "$stamp_now" > "$cache_dir/stamp.tmp.$$" 2>/dev/null; then
    mv -f "$cache_dir/stamp.tmp.$$" "$cached_stamp" 2>/dev/null || true
  fi
}

# Borra emblemas y locks de procesos muertos (sin forks). Un lock guarda el PID
# del watcher; si ese PID ya no existe, el TTY se cerró sin pasar por el trap.
sweep_stale() {
  local f p
  for f in "${XDG_RUNTIME_DIR:-/tmp}"/fastfetch-emblem-*.png \
           "${XDG_RUNTIME_DIR:-/tmp}"/fastfetch-watch-*; do
    [ -e "$f" ] || continue
    case "$f" in
      *fastfetch-watch-*) p="$(cat "$f" 2>/dev/null)" ;;
      *)                  p="${f##*-}"; p="${p%.png}" ;;
    esac
    { [ -n "$p" ] && [ -d "/proc/$p" ]; } || rm -f "$f"
  done
}

# Primer `avatar_path = "..."` de un settings.toml (lee por stdin).
avatar_path() {
  sed -n 's/^[[:space:]]*avatar_path[[:space:]]*=[[:space:]]*"\(.*\)".*/\1/p' | head -1
}

# Deja en `source_img` el avatar: state (~1 ms) o `config export` (~24 ms).
pick_avatar() {
  local state="${XDG_STATE_HOME:-$HOME/.local/state}/noctalia/settings.toml"
  source_img=""
  [ -f "$state" ] && source_img="$(avatar_path < "$state")"
  if [ -z "$source_img" ] && command -v noctalia >/dev/null 2>&1; then
    source_img="$(noctalia config export 2>/dev/null | avatar_path)"
  fi
  source_img="${source_img/#\~/$HOME}"
  [ -f "$source_img" ] || source_img="$fallback_emblem"
}

# Deja en `logo_file` el PNG a usar (vacío si no hay ninguno).
resolve_emblem() {
  emblem="${XDG_RUNTIME_DIR:-/tmp}/fastfetch-emblem-$$.png"
  rm -f "$emblem"
  sweep_stale
  pick_avatar

  shape_args=()
  [ -f "$shapes_list" ] && shape_args=(--shapes-file "$shapes_list")

  # Huella: avatar (mtime+tamaño y PATH), lista de formas y tamaño/paleta.
  # OJO: NO se incluye el mtime de `settings.toml` de Noctalia — se reescribe
  # seguido (GUI) e invalidaba el cache en cada apertura (~140 ms cada vez).
  stamp_now="$(stat -c '%Y-%s' "$source_img" "$shapes_list" 2>/dev/null | tr '\n' '-')${source_img}-384-256"

  # Cache primero (~1 ms); si no hay, corte en vivo (~140 ms).
  used_cache=0
  stamp_cache=""
  [ -r "$cached_stamp" ] && read -r stamp_cache < "$cached_stamp" 2>/dev/null
  if [ -s "$cached_emblem" ] && [ "$stamp_cache" = "$stamp_now" ]; then
    cp -f "$cached_emblem" "$emblem" 2>/dev/null && used_cache=1
  fi
  if [ "$used_cache" = "0" ] && [ -x "$shapes_bin" ] && [ -f "$source_img" ]; then
    next_shape
    crop_emblem "$emblem"
    [ -s "$emblem" ] && publish_emblem "$emblem"
  fi

  # Copia estable para el config (escritura atómica).
  if [ -s "$emblem" ]; then
    mkdir -p "$(dirname "$fallback_emblem")" 2>/dev/null || true
    cp -f "$emblem" "${fallback_emblem}.tmp.$$" 2>/dev/null &&
      mv -f "${fallback_emblem}.tmp.$$" "$fallback_emblem" 2>/dev/null || true
    logo_file="$emblem"
  elif [ -s "$fallback_emblem" ]; then
    cp -f "$fallback_emblem" "$emblem" 2>/dev/null || true
    logo_file="$emblem"
  else
    logo_file=""
  fi
}

# Prepara la forma de la PRÓXIMA terminal (en background, con lock).
prefetch_emblem() {
  mkdir -p "$cache_dir" 2>/dev/null || return 0
  # Solo se salta si el caché se regeneró hace <0,3 s: eso cubre ráfagas de
  # terminales lanzadas juntas sin comerse la variedad al abrirlas a mano.
  [ -n "$(find "$cached_emblem" -newermt '-0.3 seconds' 2>/dev/null)" ] && return 0
  find "$cache_dir" -maxdepth 1 -name .lock -mmin +1 -exec rmdir {} + 2>/dev/null || true
  mkdir "$cache_dir/.lock" 2>/dev/null || return 0
  local tmp="$cache_dir/crop.$$"
  next_shape
  crop_emblem "$tmp"
  [ -s "$tmp" ] && publish_emblem "$tmp"
  rm -f "$tmp" 2>/dev/null
  rmdir "$cache_dir/.lock" 2>/dev/null || true
}

# ── Watcher ─────────────────────────────────────────────────────────────────
# Se relanza a sí mismo. Detecta el cambio de tamaño por polling (SIGWINCH va
# al grupo de primer plano y este proceso vive en background).
if [ "${1:-}" = "--watch" ]; then
  shift
  TERM_COLS=$1; sizes=$2; logo_full=$3; emblem=$4; lock=$5; grace=$6
  shift 6
  IFS=: read -r tw_full tl_full tw_min tl_min tw_micro tl_micro _logo <<< "$sizes"

  [ -n "${lock:-}" ] && printf '%s\n' "$$" > "$lock" 2>/dev/null

  # El emblema se renombra al PID del watcher: el del padre muere enseguida y
  # el barrido de otra terminal lo borraría.
  if [ -s "$emblem" ]; then
    nuevo_emblem="${emblem%/*}/fastfetch-emblem-$$.png"
    # `mv` primero: así no queda el archivo del padre (ya no lo usa nadie).
    mv -f "$emblem" "$nuevo_emblem" 2>/dev/null ||
      cp -f "$emblem" "$nuevo_emblem" 2>/dev/null
    [ -s "$nuevo_emblem" ] && emblem="$nuevo_emblem"
  fi

  suppress_clear=1
  # shellcheck disable=SC2329  # la invoca el trap EXIT
  limpiar_lock() {
    # La imagen ya está en memoria de kitty (t=f se carga al mostrarla), así
    # que borrar el PNG al salir no rompe nada y evita 77 KB por terminal.
    [ -n "${emblem:-}" ] && rm -f "$emblem" 2>/dev/null
    [ -n "${lock:-}" ] || return 0
    [ "$(cat "$lock" 2>/dev/null)" = "$$" ] && rm -f "$lock" 2>/dev/null
    return 0
  }
  # HUP incluido: al cerrar la terminal el kernel manda SIGHUP y, sin el trap,
  # bash muere sin correr el EXIT y deja el lock huérfano. `exit 0` dispara el
  # EXIT → limpiar_lock.
  trap limpiar_lock EXIT
  trap 'exit 0' TERM INT HUP

  ticks=$(( ${grace:-15} * 10 / 8 ))
  last_cols=$TERM_COLS
  while [ "$ticks" -gt 0 ]; do
    ticks=$(( ticks - 1 ))
    sleep 0.8
    { : >/dev/tty; } 2>/dev/null || exit 0
    term_size
    [ "$TERM_COLS" != "$last_cols" ] || continue
    # Debounce: en Dwindle el ancho cambia en ráfaga.
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      sleep 0.15
      c=$TERM_COLS
      term_size
      [ "$TERM_COLS" = "$c" ] && break
    done
    last_cols=$TERM_COLS
    decide_layout "$TERM_COLS" "$TERM_ROWS"
    # Si no entra nada, no se toca la pantalla: borrarla dejaría al usuario a
    # oscuras (y podría comerse lo que haya escrito).
    [ "$lmode" = "none" ] && continue
    apply_layout "$TERM_COLS"
    with_logo "$emblem"
    render
  done
  exit 0
fi

# ── Arranque ────────────────────────────────────────────────────────────────
term_size
cols=$TERM_COLS

# Medidas de los tres configs, cacheadas: 3 fastfetch son ~21 ms de CPU.
# La huella son mtime+tamaño de los configs: si cambia uno, se remiden.
mkdir -p "$cache_dir" 2>/dev/null || true
size_stamp="$(stat -c '%Y-%s' "$config" "$config_min" "$config_micro" 2>/dev/null | tr '\n' '-')"
sizes=""
[ "$(head -1 "$cache_dir/sizes" 2>/dev/null)" = "$size_stamp" ] &&
  sizes="$(sed -n 2p "$cache_dir/sizes" 2>/dev/null)"
case "$sizes" in *[!0-9:]*) sizes="" ;; esac   # escritura rota → remide

if [ -z "$sizes" ]; then
  m_full=$(mktemp); m_min=$(mktemp); m_micro=$(mktemp)
  module_size "$config" > "$m_full" 2>/dev/null & p_full=$!
  [ -f "$config_min" ] && { module_size "$config_min" > "$m_min" 2>/dev/null & p_min=$!; }
  [ -f "$config_micro" ] && { module_size "$config_micro" > "$m_micro" 2>/dev/null & p_micro=$!; }
  wait "$p_full" 2>/dev/null
  [ -n "${p_min:-}" ] && wait "$p_min" 2>/dev/null
  [ -n "${p_micro:-}" ] && wait "$p_micro" 2>/dev/null
  read -r a b < "$m_full"; read -r c d < "$m_min" 2>/dev/null; read -r e f < "$m_micro" 2>/dev/null
  rm -f "$m_full" "$m_min" "$m_micro"
  # El ancho del logo también depende del config: se cachea con las medidas.
  g=$(grep -oE '"width"[[:space:]]*:[[:space:]]*[0-9]+' "$config" 2>/dev/null |
    head -1 | grep -oE '[0-9]+')
  sizes="${a:-58}:${b:-25}:${c:-0}:${d:-0}:${e:-0}:${f:-0}:${g:-40}"
  # Solo se cachea si la medida completa salió bien: un fallo transitorio de
  # fastfetch dejaría tw_full=0 y el config completo no se usaría nunca.
  if [ "${a:-0}" -gt 0 ]; then
    printf '%s\n%s\n' "$size_stamp" "$sizes" > "$cache_dir/sizes.tmp.$$" 2>/dev/null &&
      mv -f "$cache_dir/sizes.tmp.$$" "$cache_dir/sizes" 2>/dev/null || true
  fi
fi

resolve_emblem

IFS=: read -r tw_full tl_full tw_min tl_min tw_micro tl_micro logo_cached <<< "$sizes"
logo_full="${logo_cached:-40}"
case "$logo_full" in ''|*[!0-9]*) logo_full=40 ;; esac

[ -f "$config" ] || exec fastfetch "$@"

decide_layout "$cols" "$TERM_ROWS"
apply_layout "$cols"
with_logo "${logo_file:-}"

render "$@"

# Un watcher por terminal (lock por TTY) que se apaga solo tras la ventana.
if [ "$is_kitty" = "1" ] && { : >/dev/tty; } 2>/dev/null; then
  tty_id=$(tty 2>/dev/null | tr '/' '_')
  watch_lock="${XDG_RUNTIME_DIR:-/tmp}/fastfetch-watch-${tty_id:-notty}"
  if [ -s "$watch_lock" ]; then
    viejo=$(cat "$watch_lock" 2>/dev/null)
    if [ -n "$viejo" ] && [ -d "/proc/$viejo" ]; then
      kill -TERM "$viejo" 2>/dev/null || true
    else
      rm -f "$watch_lock" 2>/dev/null || true
    fi
  fi
  printf '%s\n' "$$" > "$watch_lock" 2>/dev/null || true

  bash "$0" --watch "$cols" "$sizes" "$logo_full" "${logo_file:-}" \
    "$watch_lock" "$grace" >/dev/tty 2>/dev/null &
  disown 2>/dev/null || true

  if [ -x "$shapes_bin" ] && [ -f "$source_img" ] && [ "$source_img" != "$fallback_emblem" ]; then
    prefetch_emblem >/dev/null 2>&1 &
    disown 2>/dev/null || true
  fi
fi

exit 0
