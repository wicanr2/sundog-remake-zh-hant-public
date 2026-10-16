-- SunDog Apple IIe 角色狀態視窗文字資料流探測器。
--
-- 只重播已證實的 Play → Disk 2 → 右鍵狀態視窗玩家輸入，並觀察五個已知
-- ASCII 標籤的 MAME :maincpu program-space reads。原始 DSK 以唯讀方式掛載，
-- 輸出只寫入 /hosttmp。CPU RAM 位址不是 raw disk offset，也不會被 remake 模擬、
-- 保存或納入規則。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local cpu = manager.machine.devices[":maincpu"]
local program = cpu.spaces["program"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = os.getenv("SUNDOG_DISK2") or "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local outPrefix = os.getenv("SUNDOG_STATUS_LABEL_TRACE_OUT") or "/hosttmp/sundog-status-label-trace-"

local labels = {
    { key = "condition", text = "Condition" },
    { key = "vigor", text = "Vigor" },
    { key = "rest", text = "Rest" },
    { key = "health", text = "Health" },
    { key = "sleep", text = "Sleep" },
}

local frame = 0
local traceActive = false
local addresses = {}
local tapped = {}
local tapHandles = {}
local counts = {}

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function programCounter()
    local ok, value = pcall(function() return cpu.state["PC"].value end)
    return ok and string.format("$%04x", value) or "unknown"
end

local function bytesAsHex(address, length)
    local values = {}
    for offset = 0, length - 1 do
        values[#values + 1] = string.format("%02x", program:read_u8(address + offset))
    end
    return table.concat(values, " ")
end

local function bytesAsASCII(address, length)
    local values = {}
    for offset = 0, length - 1 do
        local value = program:read_u8(address + offset)
        values[#values + 1] = value >= 32 and value < 127 and string.char(value) or "."
    end
    return table.concat(values, "")
end

local function scan(definition, stage)
    local raw = { string.byte(definition.text, 1, #definition.text) }
    local matches = {}
    local last = 0xbfff - #raw + 1
    -- 僅掃可寫 RAM；不讀 Apple II I/O 或 ROM page，避免探測引入 side effect。
    for address = 0x0000, last do
        if program:read_u8(address) == raw[1] then
            local same = true
            for index = 2, #raw do
                if program:read_u8(address + index - 1) ~= raw[index] then
                    same = false
                    break
                end
            end
            if same then
                matches[#matches + 1] = address
            end
        end
    end
    print("SUNDOG_STATUS_LABEL_PATTERN stage=" .. stage .. " key=" .. definition.key .. " text=" .. definition.text .. " matches=" .. #matches)
    for _, address in ipairs(matches) do
        print("SUNDOG_STATUS_LABEL_MATCH stage=" .. stage .. " key=" .. definition.key .. " address=" .. string.format("$%04x", address) .. " bytes=" .. bytesAsHex(address, #definition.text))
    end
    return matches
end

local function installTap(kind, definition, address)
    local name = kind:lower() .. "-" .. definition.key .. "-" .. string.format("%04x", address)
    if tapped[name] then
        return
    end
    tapped[name] = true
    counts[name] = 0
    local callback = function(offset, data, mask)
        if not traceActive or counts[name] >= 24 then
            return
        end
        counts[name] = counts[name] + 1
        print("SUNDOG_STATUS_LABEL_" .. kind .. " name=" .. name .. " frame=" .. frame .. " key=" .. definition.key .. " address=" .. string.format("$%04x", offset) .. " data=" .. string.format("%02x", data) .. " mask=" .. string.format("%04x", mask) .. " pc=" .. programCounter())
    end
    local ok, tap = pcall(function()
        if kind == "READ" then
            return program:install_read_tap(address, address + #definition.text - 1, name, callback)
        end
        return program:install_write_tap(address, address + #definition.text - 1, name, callback)
    end)
    print("SUNDOG_STATUS_LABEL_TAP kind=" .. kind .. " name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
    if ok then
        tapHandles[#tapHandles + 1] = tap
    end
end

local function scanAndTap(stage)
    local wasActive = traceActive
    traceActive = false
    for _, definition in ipairs(labels) do
        local matches = scan(definition, stage)
        addresses[definition.key] = matches
        for _, address in ipairs(matches) do
            installTap("READ", definition, address)
            installTap("WRITE", definition, address)
        end
    end
    traceActive = wasActive
end

local function snapshot(name)
    if not screen then
        return
    end
    local path = outPrefix .. name .. ".png"
    local ok, result = pcall(function() return screen:snapshot(path) end)
    print("SUNDOG_STATUS_LABEL_SNAPSHOT name=" .. name .. " path=" .. path .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
end

local function printContexts()
    -- 以下 program:read_u8 是 Lua 自身的診斷讀取，不能污染 CPU consumer log。
    local wasActive = traceActive
    traceActive = false
    for _, definition in ipairs(labels) do
        for _, address in ipairs(addresses[definition.key] or {}) do
            local start = math.max(0, address - 16)
            print("SUNDOG_STATUS_LABEL_CONTEXT key=" .. definition.key .. " start=" .. string.format("$%04x", start) .. " bytes=" .. bytesAsHex(start, #definition.text + 32) .. " ascii=" .. bytesAsASCII(start, #definition.text + 32))
        end
    end
    local names = {}
    for name, _ in pairs(counts) do
        names[#names + 1] = name
    end
    table.sort(names)
    for _, name in ipairs(names) do
        print("SUNDOG_STATUS_LABEL_TAP_COUNT name=" .. name .. " count=" .. counts[name])
    end
    traceActive = wasActive
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3900 then
        action:set_value(1)
        print("SUNDOG_STATUS_LABEL_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_STATUS_LABEL_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5000 then
        scanAndTap("shipboard-before-status")
        traceActive = true
    elseif frame == 5200 then
        back:set_value(1)
        print("SUNDOG_STATUS_LABEL_STATUS_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 5230 then
        back:clear_value()
    elseif frame == 5350 then
        scanAndTap("status-after-open")
    elseif frame == 5900 then
        snapshot("status")
        scanAndTap("status-stable")
        printContexts()
        traceActive = false
    end
end, "sundog-status-label-trace")
