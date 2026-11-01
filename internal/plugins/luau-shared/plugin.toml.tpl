# Plantilla de plugin b3l0w-widgets. Dos paneles: el compacto (attached) y el
# flotante grande. El widget elige según `panel_mode` (lib/prefs.luau).
id = "b3l0w/__NAME__"
name = "b3l0w __NAME__"
version = "1.0.0"
plugin_api = 43
description_key = "plugin.description"
author = "b3low-z3r0"

[[setting]]
key = "show_label"
type = "bool"
default = true
label_key = "settings.show_label"
description_key = "settings.show_label.desc"

[[setting]]
key = "panel_mode"
type = "select"
default = "compact"
label_key = "settings.panel_mode"
description_key = "settings.panel_mode.desc"
# `values = [...]` NO existe: el parser sólo lee `options`. Con values el select
# queda vacío aunque el default sea válido (plugin_manifest.cpp:267-299).
options = [
  { value = "compact", label_key = "settings.panel_mode.compact" },
  { value = "float", label_key = "settings.panel_mode.float" },
]

[[setting]]
key = "animations"
type = "bool"
default = true
label_key = "settings.animations"
description_key = "settings.animations.desc"

[[setting]]
key = "density"
type = "select"
default = "cozy"
label_key = "settings.density"
description_key = "settings.density.desc"
options = [
  { value = "compact", label_key = "settings.density.compact" },
  { value = "cozy", label_key = "settings.density.cozy" },
  { value = "comfortable", label_key = "settings.density.comfortable" },
]

[[service]]
id = "start"
entry = "service.luau"

[[widget]]
id = "widget"
entry = "widget.luau"

[[panel]]
id = "panel"
entry = "panel.luau"
width = 520
height = 620
placement = "attached"
capture_keys = ["Left", "Right", "Return"]

[[panel]]
id = "float"
entry = "panel.luau"
width = 1180
height = 820
placement = "floating"
