-- SunDog Apple IIe 顯示模式與頁面寫入觀測器。
--
-- 只重播既有的 Play → Disk 2 → 右鍵狀態視窗正常玩家輸入。它觀察
-- :maincpu 對標準 Apple II 文字／HGR 頁面的寫入，以及 $c050..$c057
-- soft-switch 的 CPU read；原始 DSK 維持唯讀，輸出僅寫到 /hosttmp。
--
-- 這是原版 oracle：CPU RAM／頁面位址不會被 remake 模擬、保存、作為
-- 遊戲規則或玩家安裝需求。寫入某頁不單獨證明 SunDog 的資產格式、
-- frame、palette、anchor、collision 或 renderer 語意。

local fileIO = _G.io
local io = manager.machine.ioport
local images = manager.machine.images
local cpu = manager.machine.devices[":maincpu"]
local program = cpu.spaces["program"]
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = os.getenv("SUNDOG_DISK2") or "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local outPrefix = os.getenv("SUNDOG_DISPLAY_PAGE_TRACE_OUT") or "/hosttmp/sundog-display-page-trace-"

local pages = {
    { key = "text-page-1", start = 0x0400, finish = 0x07ff, hardwareRole = "standard text/lo-res page 1" },
    { key = "text-page-2", start = 0x0800, finish = 0x0bff, hardwareRole = "standard text/lo-res page 2" },
    { key = "hgr-page-1", start = 0x2000, finish = 0x3fff, hardwareRole = "standard hi-res page 1" },
    { key = "hgr-page-2", start = 0x4000, finish = 0x5fff, hardwareRole = "standard hi-res page 2" },
}

local switches = {
    { key = "graphics", address = 0xc050 },
    { key = "text", address = 0xc051 },
    { key = "mixed-off", address = 0xc052 },
    { key = "mixed-on", address = 0xc053 },
    { key = "page-1", address = 0xc054 },
    { key = "page-2", address = 0xc055 },
    { key = "lo-res", address = 0xc056 },
    { key = "hi-res", address = 0xc057 },
}

local frame = 0
local traceActive = false
local currentWindow = "not-started"
local pageStats = {}
local switchStats = {}
local tapHandles = {}

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function programCounter()
    local ok, value = pcall(function() return cpu.state["PC"].value end)
    return ok and string.format("$%04x", value) or "unknown"
end

local function resetWindow(name)
    currentWindow = name
    for _, page in ipairs(pages) do
        pageStats[page.key] = { count = 0, printed = 0, first = nil, last = nil }
    end
    for _, switch in ipairs(switches) do
        switchStats[switch.key] = { readCount = 0, readPrinted = 0, writeCount = 0, writePrinted = 0 }
    end
    traceActive = true
    print("SUNDOG_DISPLAY_WINDOW_START name=" .. currentWindow .. " frame=" .. frame)
end

local function snapshot(name)
    if not screen then
        print("SUNDOG_DISPLAY_SNAPSHOT name=" .. name .. " screen=nil")
        return
    end
    local path = outPrefix .. name .. ".png"
    local ok, result = pcall(function() return screen:snapshot(path) end)
    print("SUNDOG_DISPLAY_SNAPSHOT name=" .. name .. " path=" .. path .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
end

local function dumpHGRPages(name)
    -- 原始 runtime RAM dump 只留在呼叫者指定的 /hosttmp，供同狀態 HGR 邏輯
    -- 對照使用；它不是 repository 資產、更不是 remake 的資料來源。
    local wasActive = traceActive
    traceActive = false
    for _, page in ipairs(pages) do
        if page.key == "hgr-page-1" or page.key == "hgr-page-2" then
            local path = outPrefix .. name .. "-" .. page.key .. ".bin"
            local file, openError = fileIO and fileIO.open and fileIO.open(path, "wb")
            if not file then
                print("SUNDOG_DISPLAY_HGR_DUMP name=" .. name .. " key=" .. page.key .. " path=" .. path .. " ok=false error=" .. tostring(openError))
            else
                local bytes = {}
                for address = page.start, page.finish do
                    bytes[#bytes + 1] = string.char(program:read_u8(address))
                    if #bytes == 256 then
                        file:write(table.concat(bytes))
                        bytes = {}
                    end
                end
                if #bytes > 0 then
                    file:write(table.concat(bytes))
                end
                file:close()
                print("SUNDOG_DISPLAY_HGR_DUMP name=" .. name .. " key=" .. page.key .. " path=" .. path .. " ok=true bytes=" .. (page.finish - page.start + 1))
            end
        end
    end
    traceActive = wasActive
end

local function pageShape(page)
    -- 這些是 Lua 診斷讀取，只讀 RAM，而且 traceActive 在此期間會關閉。
    local nonzero = 0
    local first = nil
    local last = nil
    local sum = 0
    local xor = 0
    for address = page.start, page.finish do
        local value = program:read_u8(address)
        sum = (sum + value) % 4294967296
        xor = (xor + (value * ((address - page.start + 1) % 251))) % 4294967296
        if value ~= 0 then
            nonzero = nonzero + 1
            if not first then
                first = address
            end
            last = address
        end
    end
    return {
        nonzero = nonzero,
        first = first and string.format("$%04x", first) or "none",
        last = last and string.format("$%04x", last) or "none",
        sum = string.format("%08x", sum),
        weightedSum = string.format("%08x", xor),
    }
end

local function finishWindow(name)
    local wasActive = traceActive
    traceActive = false
    print("SUNDOG_DISPLAY_WINDOW_END name=" .. name .. " frame=" .. frame .. " trackedWindow=" .. currentWindow)
    for _, page in ipairs(pages) do
        local stats = pageStats[page.key] or { count = 0, first = nil, last = nil }
        local shape = pageShape(page)
        print(
            "SUNDOG_DISPLAY_PAGE window=" .. currentWindow ..
            " key=" .. page.key ..
            " range=" .. string.format("$%04x..$%04x", page.start, page.finish) ..
            " hardwareRole=" .. page.hardwareRole ..
            " writes=" .. stats.count ..
            " firstWrite=" .. (stats.first or "none") ..
            " lastWrite=" .. (stats.last or "none") ..
            " nonzero=" .. shape.nonzero ..
            " firstNonzero=" .. shape.first ..
            " lastNonzero=" .. shape.last ..
            " sum=" .. shape.sum ..
            " weightedSum=" .. shape.weightedSum
        )
    end
    for _, switch in ipairs(switches) do
        local stats = switchStats[switch.key] or { readCount = 0, writeCount = 0 }
        print("SUNDOG_DISPLAY_SWITCH_SUMMARY window=" .. currentWindow .. " key=" .. switch.key .. " address=" .. string.format("$%04x", switch.address) .. " reads=" .. stats.readCount .. " writes=" .. stats.writeCount)
    end
    traceActive = wasActive
end

local function installPageTap(page)
    local name = "display-page-" .. page.key
    local ok, tap = pcall(function()
        return program:install_write_tap(page.start, page.finish, name, function(offset, data, mask)
            if not traceActive then
                return
            end
            local stats = pageStats[page.key]
            stats.count = stats.count + 1
            if not stats.first then
                stats.first = string.format("$%04x", offset)
            end
            stats.last = string.format("$%04x", offset)
            if stats.printed < 24 then
                stats.printed = stats.printed + 1
                print(
                    "SUNDOG_DISPLAY_PAGE_WRITE window=" .. currentWindow ..
                    " key=" .. page.key ..
                    " address=" .. string.format("$%04x", offset) ..
                    " data=" .. string.format("%02x", data) ..
                    " mask=" .. string.format("%04x", mask) ..
                    " pc=" .. programCounter()
                )
            end
        end)
    end)
    print("SUNDOG_DISPLAY_PAGE_TAP key=" .. page.key .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
    if ok then
        tapHandles[#tapHandles + 1] = tap
    end
end

local function installSwitchTap(switch)
    local function install(kind)
        local name = "display-switch-" .. kind:lower() .. "-" .. switch.key
        local ok, tap = pcall(function()
            local callback = function(offset, data, mask)
                if not traceActive then
                    return
                end
                local stats = switchStats[switch.key]
                local countKey = kind == "READ" and "readCount" or "writeCount"
                local printedKey = kind == "READ" and "readPrinted" or "writePrinted"
                stats[countKey] = stats[countKey] + 1
                if stats[printedKey] < 16 then
                    stats[printedKey] = stats[printedKey] + 1
                    print(
                        "SUNDOG_DISPLAY_SWITCH_" .. kind ..
                        " window=" .. currentWindow ..
                        " key=" .. switch.key ..
                        " address=" .. string.format("$%04x", offset) ..
                        " data=" .. string.format("%02x", data) ..
                        " mask=" .. string.format("%04x", mask) ..
                        " pc=" .. programCounter()
                    )
                end
            end
            if kind == "READ" then
                return program:install_read_tap(switch.address, switch.address, name, callback)
            end
            return program:install_write_tap(switch.address, switch.address, name, callback)
        end)
        print("SUNDOG_DISPLAY_SWITCH_TAP kind=" .. kind .. " key=" .. switch.key .. " address=" .. string.format("$%04x", switch.address) .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
        if ok then
            tapHandles[#tapHandles + 1] = tap
        end
    end
    install("READ")
    install("WRITE")
end

local function removeTaps()
    -- MAME 0.251 可能在 Lua 生命週期結束時釋放 memory tap；完成觀測後主動解除，
    -- 並把 API 不可用視為 harness 資訊，而不是原版行為結論。
    for index, tap in ipairs(tapHandles) do
        local ok, result = pcall(function() return tap:remove() end)
        print("SUNDOG_DISPLAY_TAP_REMOVE index=" .. index .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
    tapHandles = {}
end

for _, page in ipairs(pages) do
    installPageTap(page)
end
for _, switch in ipairs(switches) do
    installSwitchTap(switch)
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3500 then
        snapshot("title")
        finishWindow("title-baseline")
    elseif frame == 3600 then
        resetWindow("title-to-flip-prompt")
    elseif frame == 3900 then
        action:set_value(1)
        print("SUNDOG_DISPLAY_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
        print("SUNDOG_DISPLAY_PLAY_UP frame=" .. frame)
    elseif frame == 4200 then
        traceActive = false
        snapshot("flip-prompt")
        finishWindow("flip-prompt")
        resetWindow("flip-prompt-to-shipboard")
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_DISPLAY_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
        print("SUNDOG_DISPLAY_CONFIRM_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 4380 then
        action:clear_value()
        print("SUNDOG_DISPLAY_CONFIRM_UP frame=" .. frame)
    elseif frame == 5000 then
        traceActive = false
        snapshot("shipboard")
        dumpHGRPages("shipboard")
        finishWindow("shipboard")
        resetWindow("shipboard-to-status")
    elseif frame == 5200 then
        back:set_value(1)
        print("SUNDOG_DISPLAY_STATUS_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 5230 then
        back:clear_value()
        print("SUNDOG_DISPLAY_STATUS_UP frame=" .. frame)
    elseif frame == 5350 then
        traceActive = false
        snapshot("status-open")
        dumpHGRPages("status-open")
        finishWindow("status-open")
        resetWindow("status-stable")
    elseif frame == 5900 then
        traceActive = false
        snapshot("status-stable")
        dumpHGRPages("status-stable")
        finishWindow("status-stable")
        removeTaps()
        print("SUNDOG_DISPLAY_TRACE_COMPLETE frame=" .. frame)
    end
end, "sundog-display-page-trace")
