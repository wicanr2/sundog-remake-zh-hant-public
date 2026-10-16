-- SunDog Apple IIe 工程艙標籤資料流探測器。
--
-- 這是 Docker MAME oracle 的窄追蹤：在 Disk 2 載入後觀察 `Sub-c Engines`
-- raw table label 寫入 shipboard RAM，並在工程艙轉場時讀出、複製到另一個
-- runtime buffer。原始 DSK 唯讀；截圖只寫入 /hosttmp。CPU 位址不是 DSK file
-- offset，tap 所見資料流不自動證明函式名稱、欄位意義或完整文字 renderer。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local cpu = manager.machine.devices[":maincpu"]
local program = cpu.spaces["program"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local screen = manager.machine.screens[":screen"]
local disk2 = os.getenv("SUNDOG_DISK2") or "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local outPrefix = os.getenv("SUNDOG_ENGINEERING_TRACE_OUT") or "/hosttmp/sundog-apple2-engineering-label-trace-20260811-"
local targetX = tonumber(os.getenv("SUNDOG_TARGET_X") or "140")
local targetY = tonumber(os.getenv("SUNDOG_TARGET_Y") or "100")
local label = "Sub-c Engines"
local frame = 0
local traceActive = false
local tapHandles = {}
local traceCounts = {}

x:set_value(182)
y:set_value(144)
action:clear_value()

local function bytesAsHex(startAddress, length)
    local values = {}
    for offset = 0, length - 1 do
        values[#values + 1] = string.format("%02x", program:read_u8(startAddress + offset))
    end
    return table.concat(values, " ")
end

local function bytesAsASCII(startAddress, length)
    local values = {}
    for offset = 0, length - 1 do
        local value = program:read_u8(startAddress + offset)
        values[#values + 1] = value >= 32 and value < 127 and string.char(value) or "."
    end
    return table.concat(values, "")
end

local function findLabel(stage)
    local bytes = { string.byte(label, 1, #label) }
    local matches = {}
    local last = 0xbfff - #bytes + 1
    -- 僅掃 Apple II 可寫 RAM；刻意不讀 I/O／ROM page，避免探測造成 side effect。
    for address = 0x0000, last do
        if program:read_u8(address) == bytes[1] then
            local same = true
            for index = 2, #bytes do
                if program:read_u8(address + index - 1) ~= bytes[index] then
                    same = false
                    break
                end
            end
            if same then
                matches[#matches + 1] = address
            end
        end
    end
    print("SUNDOG_ENGINEERING_LABEL_PATTERN stage=" .. stage .. " label=" .. label .. " matches=" .. #matches)
    for _, address in ipairs(matches) do
        print("SUNDOG_ENGINEERING_LABEL_MATCH stage=" .. stage .. " address=" .. string.format("$%04x", address) .. " bytes=" .. bytesAsHex(address, #bytes))
    end
end

local function programCounter()
    local ok, value = pcall(function() return cpu.state["PC"].value end)
    return ok and string.format("$%04x", value) or "unknown"
end

local function installTap(kind, name, startAddress, length)
    traceCounts[name] = 0
    local callback = function(offset, data, mask)
        if not traceActive or traceCounts[name] >= 16 then
            return
        end
        traceCounts[name] = traceCounts[name] + 1
        print("SUNDOG_ENGINEERING_LABEL_" .. kind .. " name=" .. name .. " frame=" .. frame .. " offset=" .. string.format("$%04x", offset) .. " data=" .. string.format("%02x", data) .. " mask=" .. string.format("%04x", mask) .. " pc=" .. programCounter())
    end
    local ok, tap = pcall(function()
        if kind == "READ" then
            return program:install_read_tap(startAddress, startAddress + length - 1, name, callback)
        end
        return program:install_write_tap(startAddress, startAddress + length - 1, name, callback)
    end)
    print("SUNDOG_ENGINEERING_LABEL_TAP name=" .. name .. " kind=" .. kind .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
    if ok then
        tapHandles[#tapHandles + 1] = tap
    end
end

local function snapshot()
    if not screen then
        return
    end
    local path = outPrefix .. "engineering.png"
    local ok, result = pcall(function() return screen:snapshot(path) end)
    print("SUNDOG_ENGINEERING_LABEL_SNAPSHOT path=" .. path .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3800 then
        findLabel("pre-play-disk1")
    elseif frame == 3900 then
        action:set_value(1)
        print("SUNDOG_ENGINEERING_LABEL_PLAY_DOWN frame=" .. frame)
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4000 then
        installTap("WRITE", "shipboard-runtime-load", 0x65df, #label)
        traceActive = true
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_ENGINEERING_LABEL_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5000 then
        findLabel("shipboard-after-disk2")
    elseif frame == 5050 then
        installTap("READ", "engineering-destination-before-copy", 0x5d03, #label)
        installTap("READ", "shipboard-source-copy", 0x65df, #label)
        installTap("WRITE", "engineering-destination-copy", 0x5d03, #label)
    elseif frame == 5100 then
        x:set_value(targetX)
        y:set_value(targetY)
        action:set_value(1)
        print("SUNDOG_ENGINEERING_LABEL_TARGET_DOWN frame=" .. frame .. " x=" .. targetX .. " y=" .. targetY)
    elseif frame == 5700 then
        action:clear_value()
        print("SUNDOG_ENGINEERING_LABEL_TARGET_UP frame=" .. frame)
    elseif frame == 6000 then
        traceActive = false
        print("SUNDOG_ENGINEERING_LABEL_TAP_COUNTS shipboard-runtime-load=" .. traceCounts["shipboard-runtime-load"] .. " engineering-destination-before-copy=" .. traceCounts["engineering-destination-before-copy"] .. " shipboard-source-copy=" .. traceCounts["shipboard-source-copy"] .. " engineering-destination-copy=" .. traceCounts["engineering-destination-copy"])
        print("SUNDOG_ENGINEERING_LABEL_CONTEXT name=shipboard-source start=$65c0 bytes=" .. bytesAsHex(0x65c0, 64) .. " ascii=" .. bytesAsASCII(0x65c0, 64))
        print("SUNDOG_ENGINEERING_LABEL_CONTEXT name=engineering-destination start=$5ce0 bytes=" .. bytesAsHex(0x5ce0, 64) .. " ascii=" .. bytesAsASCII(0x5ce0, 64))
        snapshot()
        findLabel("engineering")
    end
end, "sundog-engineering-label-trace")
