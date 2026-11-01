--!strict
-- Animación para paneles Noctalia. La API no tiene transiciones: todo se hace
-- con onFrameTick + panel.setNeedsFrameTick(true) y valores interpolados por frame.

local M = {}

function M.clamp(v: number, low: number, high: number): number
	if v < low then return low end
	if v > high then return high end
	return v
end

function M.lerp(a: number, b: number, t: number): number
	return a + (b - a) * t
end

function M.ease_out(t: number): number
	local inv = 1 - t
	return 1 - inv * inv * inv
end

function M.ease_in_out(t: number): number
	if t < 0.5 then return 4 * t * t * t end
	local f = -2 * t + 2
	return 1 - (f * f * f) / 2
end

-- Progreso 0..1 de una animación corta. `state` es una tabla con dueño externo:
-- campos t (0..1), from, to y ms.
function M.step(state: any, delta_ms: number): number
	if state.t >= 1 then return 1 end
	local ms = state.ms or 200
	state.t = math.min(1, state.t + (delta_ms / ms))
	return state.t
end

-- Valor actual de una animación ya configurada (from/to/t/ms).
function M.value(state: any, ease: ((number) -> number)?): number
	local fn = ease or M.ease_out
	return M.lerp(state.from, state.to, fn(M.clamp(state.t, 0, 1)))
end

-- Dispara una animación nueva conservando el valor actual como origen.
function M.restart(state: any, target: number, ms: number?): ()
	state.from = M.value(state)
	state.to = target
	state.t = 0
	state.ms = ms or 200
end

function M.idle(state: any): boolean
	return (state.t or 1) >= 1
end

-- Escalonado por índice: cada item entra 100 ms después del anterior.
function M.stagger(index: number, t: number, step: number?): number
	local offset = index * (step or 0.1)
	return M.clamp((t - offset) / 0.55, 0, 1)
end

return M
