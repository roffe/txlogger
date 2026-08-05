-- RPM rate of change in rpm/s, updated every log frame.
-- Copy to ~/txlogger/scripts/T5/ to enable; the output becomes a log column
-- and a live gauge/plotter value named RPM.RoC. Requires Rpm in the log.

inputs  = { "Rpm" }
outputs = { "RPM.RoC" }

local prev_rpm, prev_t
function update(v, t) -- v[name] = current value, t = unix milliseconds
	local rpm = v["Rpm"]
	if prev_t and t > prev_t then
		out["RPM.RoC"] = (rpm - prev_rpm) * 1000 / (t - prev_t)
	end
	prev_rpm, prev_t = rpm, t
end
