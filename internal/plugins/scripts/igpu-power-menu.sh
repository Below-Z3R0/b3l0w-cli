#!/bin/bash
# iGPU Power Profile menu v5
# Toggle platform_profile + modprobe.d + notify
# Cmdlines optimizados para Intel 12th gen iGPU gaming
# Ver: /home/b3low-z3r0/intel_igpu_gaming_tweaks_2026.md

set -e

MODE="${1:-menu}"

notify() {
    notify-send -a "iGPU Power" -u low "$1" "$2" -t 5000 2>/dev/null || true
}

current_platform_profile() {
    cat /sys/firmware/acpi/platform_profile 2>/dev/null || echo "unknown"
}

# Lee los flags de modprobe.d (no requiere sudo)
configured_flags() {
    local file="/etc/modprobe.d/i915-performance.conf"
    if [ -f "$file" ]; then
        grep '^options i915' "$file" | head -1
    else
        echo "(sin archivo /etc/modprobe.d/i915-performance.conf)"
    fi
}

set_profile() {
    local target="$1"
    if echo "$target" | sudo -n /usr/bin/tee /sys/firmware/acpi/platform_profile >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

configure_modprobe() {
    local mode="$1"
    local file="/etc/modprobe.d/i915-performance.conf"
    case "$mode" in
        performance|max)
            printf 'options i915 enable_psr=0 enable_fbc=1 enable_guc=2 enable_dc=-1 enable_hangcheck=1 fastboot=1 error_capture=0\n' \
                | sudo -n /usr/bin/tee "$file" >/dev/null 2>&1
            ;;
        balanced)
            printf 'options i915 enable_psr=1 enable_fbc=-1 enable_guc=-1 enable_dc=-1 enable_hangcheck=1\n' \
                | sudo -n /usr/bin/tee "$file" >/dev/null 2>&1
            ;;
        low-power|saver|min)
            printf 'options i915 enable_psr=1 enable_fbc=-1 enable_guc=-1 enable_dc=1 enable_hangcheck=1\n' \
                | sudo -n /usr/bin/tee "$file" >/dev/null 2>&1
            ;;
    esac
}

case "$MODE" in
    performance|max)
        if set_profile performance; then configure_modprobe performance; fi
        notify "Performance Mode" "iGPU al maximo. Reboot requerido para enable_psr=0, fbc=1, guc=2."
        ;;
    balanced)
        if set_profile balanced; then configure_modprobe balanced; fi
        notify "Balanced Mode" "Ahorro automatico. Reboot requerido para enable_psr=1."
        ;;
    low-power|saver|min)
        if set_profile low-power; then configure_modprobe low-power; fi
        notify "Power Saver" "Maximo ahorro. Reboot requerido para enable_dc=1."
        ;;
    status)
        PLATFORM=$(current_platform_profile)
        CONFIGURED=$(configured_flags)
        notify "iGPU Status" "Platform: $PLATFORM | Configured: $CONFIGURED"
        ;;
    menu|*)
        PLATFORM=$(current_platform_profile)
        notify "iGPU Power" "Actual: $PLATFORM. Opciones: performance | balanced | low-power | status"
        ;;
esac