-- t7immo.lua: TWICE immobilizer emulator for SAAB Trionic 7.
-- Answers the T7's immo challenge on 0x750 with a granting reply on 0x740 and
-- broadcasts a TWICE status on 0x320 so the T7 keeps its 2s watchdog clear.
-- Port of github.com/roffe/t7immo (IMMO.C crypto). Pass the ECU's
-- ECUID.ECUHardwNr (15 chars) as the script argument — both T7 and TWICE
-- derive the ImmoCode from it.
--   go run ./cmd/canlang -adapter "CANUSB VCP" -port /dev/ttyUSB2 -rate 500 t7immo.lua 217OZG102833812

local hardwareNr = arg[1]
if not hardwareNr or #hardwareNr ~= 15 then
	error("usage: t7immo.lua <hardwareNr> (15-char ECUID.ECUHardwNr, e.g. 217OZG102833812)", 0)
end

local ID_REQ    = 0x750 -- IREQ_ECS: T7 challenge -> us
local ID_REP    = 0x740 -- IREP_TWI: our reply -> T7
local ID_STATUS = 0x320 -- GSTA_TWI: periodic TWICE status -> T7 (all-zero: CarType 0, no faults)

local band, bor, bxor = bit.band, bit.bor, bit.bxor
local lshift, rshift = bit.lshift, bit.rshift

-- table1: T7 encodes the challenge with this (we decode with its inverse).
-- rtable2: T7 decodes our reply with this (we encode with its inverse).
local table1  = { 2, 0, 6, 0xd, 8, 0xf, 0xb, 3, 9, 0xe, 1, 0xa, 0xc, 7, 5, 4 }
local rtable2 = { 0xb, 5, 1, 6, 2, 0xc, 0xd, 9, 4, 0xf, 3, 0xa, 7, 0, 8, 0xe }

local function invert(t) -- 1-based 16-entry nibble permutation -> its inverse
	local inv = {}
	for i = 1, 16 do inv[t[i] + 1] = i - 1 end
	return inv
end

local rtable1 = invert(table1)  -- decode challenge
local table2  = invert(rtable2) -- encode reply

-- subst maps both nibbles of b through nibble table t (1-based).
local function subst(t, b)
	return bor(lshift(t[rshift(b, 4) + 1], 4), t[band(b, 0xf) + 1])
end

-- calcImmoCode reproduces IMMO.C calcImmoCode: an LFSR clocked once per bit of
-- the hardware number, then 16 idle bytes. Returns the 6-byte code (F1..F6).
local function calcImmoCode(hw)
	local code = { 0xa5, 0xe3, 0x0b, 0x78, 0x39, 0xbb }
	local pol  = { 0x89, 0x40, 0x89, 0x40, 0x89, 0x40 }

	local function step(feedback)
		local oldCarry = 0
		for i = 1, 6 do
			local newCarry = band(code[i], 1)
			code[i] = rshift(code[i], 1)
			if oldCarry ~= 0 then code[i] = bor(code[i], 0x80) end
			oldCarry = newCarry
		end
		if feedback then
			for i = 1, 6 do code[i] = bxor(code[i], pol[i]) end
		end
	end

	for i = 1, #hw do
		local cur = hw:byte(i)
		for _ = 1, 8 do
			step(band(bxor(cur, code[6]), 1) ~= 0)
			cur = rshift(cur, 1)
		end
	end
	for _ = 1, 16 do
		for _ = 1, 8 do
			step(band(code[6], 1) ~= 0)
		end
	end
	return code
end

-- answer builds the 6-byte reply to a challenge frame. Always grants (0x55);
-- a real TWICE would gate on the programmed key.
local function answer(immo, req)
	local r1 = subst(rtable1, req:u8(0))
	local r2 = subst(rtable1, req:u8(1))
	local f4, f5, f6 = immo[4], immo[5], immo[6]
	return {
		subst(table2, bxor(f4, r1)),
		subst(table2, r1),
		subst(table2, bxor(bxor(f6, r1), r2)),
		subst(table2, r2),
		subst(table2, bxor(f5, r2)),
		0x55,
	}
end

local immo = calcImmoCode(hardwareNr)
print(string.format("ImmoCode for %q: %02X %02X %02X %02X %02X %02X",
	hardwareNr, immo[1], immo[2], immo[3], immo[4], immo[5], immo[6]))
print(string.format("TWICE emulator listening on 0x%X, replying on 0x%X", ID_REQ, ID_REP))

local status = { 0, 0, 0, 0, 0, 0, 0, 0 }
local lastStatus = 0
local replied, announced = false, false

while true do
	-- Keep the T7's 2s watchdog fed: one status frame per wall-clock second.
	local now = os.time()
	if now ~= lastStatus then
		bus:send(ID_STATUS, status)
		lastStatus = now
	end

	local f = bus:recv(500, ID_REQ)
	if f then
		if f:len() >= 6 then
			local rep = answer(immo, f)
			bus:send(ID_REP, rep)
			print("req " .. f:hex() .. " -> rep " .. string.format(
				"%02X %02X %02X %02X %02X %02X", rep[1], rep[2], rep[3], rep[4], rep[5], rep[6]))
			replied, announced = true, false
		else
			print("short request: " .. f:hex())
		end
	elseif replied and not announced then
		-- ponytail: 500ms of silence after a reply == accepted. Time-based
		-- inference (no immo status on the bus); false positive if ignition
		-- is switched off mid-handshake. Original used 600ms.
		announced = true
		print("code ACCEPTED (T7 stopped requesting)")
	end
end
