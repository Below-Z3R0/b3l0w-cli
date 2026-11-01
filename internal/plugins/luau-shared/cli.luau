--!strict
-- Puente al CLI en Go. Cada widget tiene su propia extensión de b3l0w
-- (b3l0w docker|kanban|audit|audio|anilist|yt ...), así que el nombre de la
-- extensión es un parámetro, no una constante.
--
-- El service es el único que llama al CLI: el panel nunca lanza procesos.

local M = {}

-- Cita un argumento para el shell. Rutas y títulos traen espacios y comillas:
-- sin esto el comando se parte en pedazos y el binario recibe basura.
local function q(value: string): string
	return "'" .. string.gsub(value, "'", "'\\''") .. "'"
end

M.quote = q

local function build(ext: string, args: { string }): string
	local argv: { string } = { "b3l0w", ext }
	for _, value in ipairs(args) do
		table.insert(argv, q(value))
	end
	return table.concat(argv, " ")
end

-- Corre un subcomando que devuelve JSON y entrega la tabla decodificada.
-- cb(value, err): uno de los dos siempre es nil.
function M.json(ext: string, args: { string }, cb: (any, string?) -> ()): ()
	noctalia.runAsync(build(ext, args), function(result: any)
		local code = result and result.exitCode or -1
		if code ~= 0 then
			local stderr = result and result.stderr or ""
			cb(nil, "exit " .. tostring(code) .. ": " .. string.sub(stderr, 1, 160))
			return
		end
		local decoded = noctalia.json.decode(result and result.stdout or "")
		if type(decoded) ~= "table" then
			cb(nil, "respuesta invalida")
			return
		end
		cb(decoded, nil)
	end)
end

-- Igual pero devuelve el texto crudo (logs, progreso de updates).
function M.text(ext: string, args: { string }, cb: (string?, string?) -> ()): ()
	noctalia.runAsync(build(ext, args), function(result: any)
		local code = result and result.exitCode or -1
		if code ~= 0 then
			local stderr = result and result.stderr or ""
			cb(nil, "exit " .. tostring(code) .. ": " .. string.sub(stderr, 1, 120))
			return
		end
		cb(result and result.stdout or "", nil)
	end)
end

-- ¿Está la extensión instalada? Evita que el widget muestre un error críptico
-- cuando el binario todavía no se compiló.
function M.available(ext: string, cb: (boolean) -> ()): ()
	noctalia.runAsync("b3l0w plugins 2>/dev/null | grep -qx " .. q(ext), function(result: any)
		cb((result and result.exitCode or 1) == 0)
	end)
end

return M
