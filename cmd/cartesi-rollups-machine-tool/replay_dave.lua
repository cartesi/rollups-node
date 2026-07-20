-- (c) Cartesi and individual authors (see AUTHORS)
-- SPDX-License-Identifier: Apache-2.0 (see LICENSE)

local cartesi = require("cartesi")

local function usage(message)
    if message then io.stderr:write(message, "\n") end
    io.stderr:write(
        "usage: replay_dave.lua --template <path> --store <path> --inputs-manifest <path>\n"
    )
    os.exit(2)
end

local function parse_args(args)
    local opts = {}
    local i = 1
    while i <= #args do
        local key = args[i]
        local value = args[i + 1]
        if key == "--template" then
            opts.template = value
        elseif key == "--store" then
            opts.store = value
        elseif key == "--inputs-manifest" then
            opts.inputs_manifest = value
        else
            usage("unknown argument: " .. tostring(key))
        end
        if not value then usage("missing value for " .. tostring(key)) end
        i = i + 2
    end
    if not opts.template then usage("missing --template") end
    if not opts.store then usage("missing --store") end
    if not opts.inputs_manifest then usage("missing --inputs-manifest") end
    return opts
end

local function read_all(path)
    local file <close> = assert(io.open(path, "rb"))
    return assert(file:read("*a"))
end

local function read_input_paths(path)
    local inputs = {}
    for line in io.lines(path) do
        if line ~= "" then inputs[#inputs + 1] = line end
    end
    return inputs
end

local function run_until_manual_yield(machine)
    while true do
        local break_reason = machine:run(math.maxinteger)
        if break_reason == cartesi.BREAK_REASON_YIELDED_MANUALLY then
            return
        elseif break_reason == cartesi.BREAK_REASON_YIELDED_AUTOMATICALLY then
            machine:receive_cmio_request()
        elseif break_reason == cartesi.BREAK_REASON_HALTED then
            error("machine halted before a manual yield")
        elseif break_reason == cartesi.BREAK_REASON_FAILED then
            error("machine failed before a manual yield")
        elseif break_reason ~= cartesi.BREAK_REASON_YIELDED_SOFTLY then
            error("unexpected machine break reason: " .. tostring(break_reason))
        end
    end
end

local function ensure_manual_yield(machine)
    if machine:read_reg("iflags_Y") == 0 then
        run_until_manual_yield(machine)
    end
end

local function advance_one(machine, input_path, input_number)
    local checkpoint = machine:get_root_hash()
    machine:send_cmio_response(cartesi.HTIF_YIELD_REASON_ADVANCE_STATE, read_all(input_path), checkpoint)
    run_until_manual_yield(machine)

    local _, reason, data = machine:receive_cmio_request()
    if reason == cartesi.HTIF_YIELD_MANUAL_REASON_RX_REJECTED then
        error(string.format("Dave replay input %d was rejected", input_number))
    elseif reason == cartesi.HTIF_YIELD_MANUAL_REASON_TX_EXCEPTION then
        error(string.format("Dave replay input %d raised an exception", input_number))
    elseif reason ~= cartesi.HTIF_YIELD_MANUAL_REASON_RX_ACCEPTED then
        error(string.format("Dave replay input %d ended with unexpected yield reason %s", input_number, tostring(reason)))
    end
    if #data ~= 32 then
        error(string.format("Dave replay input %d returned an invalid outputs hash length: %d", input_number, #data))
    end
end

local opts = parse_args(arg)
local machine <close> = cartesi.machine(opts.template)
ensure_manual_yield(machine)

for index, input_path in ipairs(read_input_paths(opts.inputs_manifest)) do
    advance_one(machine, input_path, index - 1)
end

machine:store(opts.store)
