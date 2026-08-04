-- RPM rate of change in rpm/s, updated every log frame.
-- Copy to ~/txlogger/scripts/T7/ to enable; the output becomes a log column
-- and a live gauge/plotter value named RPM.RoC.

inputs  = { "ActualIn.n_Engine" }
outputs = { "RPM.RoC" }

local prev_rpm, prev_t
function update(v, t) -- v[name] = current value, t = unix milliseconds
	local rpm = v["ActualIn.n_Engine"]
	if prev_t and t > prev_t then
		out["RPM.RoC"] = (rpm - prev_rpm) * 1000 / (t - prev_t)
	end
	prev_rpm, prev_t = rpm, t
end
