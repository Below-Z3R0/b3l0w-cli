--!strict
-- Preferencias comunes a todos los widgets b3l0w.
--
-- El host no deja cambiar el tamaño del panel en runtime, así que "panel
-- flotante grande" se resuelve declarando DOS paneles en el manifest y que el
-- widget elija cuál abrir según `panel_mode`.

local M = {}

function M.show_label(): boolean
	local value = noctalia.getConfig("show_label")
	return value ~= false
end

-- "compact" (attached, chico) | "float" (floating, grande)
function M.panel_mode(): string
	local value = noctalia.getConfig("panel_mode")
	if type(value) == "string" and value ~= "" then
		return value
	end
	return "compact"
end

function M.panel_id(plugin: string): string
	local suffix = if M.panel_mode() == "float" then ":float" else ":panel"
	return plugin .. suffix
end

function M.animations(): boolean
	local value = noctalia.getConfig("animations")
	return value ~= false
end

-- Densidad de la UI, para que cada plugin ajuste tamaño de card/fila.
function M.density(): string
	local value = noctalia.getConfig("density")
	if value == "compact" or value == "comfortable" then
		return value
	end
	return "cozy"
end

return M
