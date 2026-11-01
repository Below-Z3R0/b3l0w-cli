--!strict
-- Persistencia JSON en el directorio propio del plugin, con escritura diferida:
-- los services guardan en cada acción y sin esto cada cambio toca el disco.

local M = {}

-- pluginDataDir() puede fallar y devolver nil,err: sin el guard el service
-- muere en la primera escritura.
local function path_for(file: string): string?
	local dir = noctalia.pluginDataDir()
	if type(dir) ~= "string" or dir == "" then
		return nil
	end
	return dir .. "/" .. file
end

function M.load(file: string): any
	local path = path_for(file)
	if path == nil then
		return nil
	end
	local contenido = noctalia.readFile(path)
	if type(contenido) ~= "string" or contenido == "" then
		return nil
	end
	local decoded = noctalia.json.decode(contenido)
	return if type(decoded) == "table" then decoded else nil
end

function M.save(file: string, value: any): boolean
	local path = path_for(file)
	if path == nil then
		return false
	end
	local ok = noctalia.writeFile(path, noctalia.json.encode(value))
	return ok ~= false
end

-- Copia profunda de tablas simples (JSON). Evita que el panel mute el estado
-- del service por referencia.
function M.copy(value: any): any
	if type(value) ~= "table" then
		return value
	end
	local out = {}
	for key, item in pairs(value) do
		out[key] = M.copy(item)
	end
	return out
end

return M
