-- SunDog Apple IIe HGR page writer-PC oracle。
--
-- 只重播既有 Play → Disk 2 → 右鍵角色狀態視窗的正常玩家輸入。此工具只計數
-- :maincpu 寫入標準 HGR page 1／2 時 MAME 回報的 program counter，並保存同時序
-- screenshot 到 /hosttmp。它不 dump runtime RAM，也不把 RAM 位址、PC 或 screenshot
-- 當成 remake 資產、遊戲規則、存檔格式或玩家安裝需求。
--
-- callback 的 PC 是 MAME 在 memory-write tap 執行時公開的 CPU state；它可收窄後續
-- 原版 renderer／loader 研究範圍，但不是函式名稱、完整呼叫圖或 raw DSK asset route 的證明。

local ioPorts = manager.machine.ioport
local images = manager.machine.images
local cpu = manager.machine.devices[":maincpu"]
local program = cpu.spaces["program"]
local drive = images[":sl6:diskiing:0:525"]
local x = ioPorts.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = ioPorts.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = ioPorts.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = ioPorts.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = os.getenv("SUNDOG_DISK2") or "/disk2"
local outPrefix = os.getenv("SUNDOG_HGR_WRITER_PC_OUT") or "/hosttmp/sundog-hgr-writer-pc-"

local pages = {
    { key = "hgr-page-1", start = 0x2000, finish = 0x3fff, hardwareRole = "standard hi-res page 1" },
    { key = "hgr-page-2", start = 0x4000, finish = 0x5fff, hardwareRole = "standard hi-res page 2" },
}

local frame = 0
local traceActive = false
local currentWindow = "not-started"
local pageWriters = {}
local tapHandles = {}

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function programCounter()
    local ok, value = pcall(function() return cpu.state["PC"].value end)
    if ok and type(value) == "number" then
        return string.format("$%04x", value)
    end
    return "unknown"
end

local function resetWindow(name)
    currentWindow = name
    pageWriters = {}
    for _, page in ipairs(pages) do
        pageWriters[page.key] = {}
    end
    traceActive = true
    print("SUNDOG_HGR_WRITER_WINDOW_START name=" .. currentWindow .. " frame=" .. frame)
end

local function snapshot(name)
    if not screen then
        print("SUNDOG_HGR_WRITER_SNAPSHOT name=" .. name .. " screen=nil")
        return
    end
    local path = outPrefix .. name .. ".png"
    local ok, result = pcall(function() return screen:snapshot(path) end)
    print("SUNDOG_HGR_WRITER_SNAPSHOT name=" .. name .. " path=" .. path .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
end

local function recordWriter(page, offset, data, mask)
    if not traceActive then
        return
    end
    local pc = programCounter()
    local writers = pageWriters[page.key]
    local entry = writers[pc]
    if not entry then
        entry = {
            writes = 0,
            firstAddress = nil,
            lastAddress = nil,
            sampleData = nil,
            sampleMask = nil,
        }
        writers[pc] = entry
        print(
            "SUNDOG_HGR_WRITER_SAMPLE" ..
            " window=" .. currentWindow ..
            " page=" .. page.key ..
            " pc=" .. pc ..
            " address=" .. string.format("$%04x", offset) ..
            " data=" .. string.format("%02x", data) ..
            " mask=" .. string.format("%04x", mask)
        )
    end
    entry.writes = entry.writes + 1
    if not entry.firstAddress then
        entry.firstAddress = string.format("$%04x", offset)
        entry.sampleData = string.format("%02x", data)
        entry.sampleMask = string.format("%04x", mask)
    end
    entry.lastAddress = string.format("$%04x", offset)
end

local function finishWindow(name)
    local wasActive = traceActive
    traceActive = false
    print("SUNDOG_HGR_WRITER_WINDOW_END name=" .. name .. " frame=" .. frame .. " trackedWindow=" .. currentWindow)
    for _, page in ipairs(pages) do
        local writers = pageWriters[page.key] or {}
        local rows = {}
        local totalWrites = 0
        for pc, entry in pairs(writers) do
            totalWrites = totalWrites + entry.writes
            rows[#rows + 1] = { pc = pc, entry = entry }
        end
        table.sort(rows, function(left, right)
            if left.entry.writes ~= right.entry.writes then
                return left.entry.writes > right.entry.writes
            end
            return left.pc < right.pc
        end)
        print(
            "SUNDOG_HGR_WRITER_SUMMARY" ..
            " window=" .. currentWindow ..
            " page=" .. page.key ..
            " range=" .. string.format("$%04x..$%04x", page.start, page.finish) ..
            " hardwareRole=" .. page.hardwareRole ..
            " writerCount=" .. #rows ..
            " writes=" .. totalWrites
        )
        for _, row in ipairs(rows) do
            local entry = row.entry
            print(
                "SUNDOG_HGR_WRITER" ..
                " window=" .. currentWindow ..
                " page=" .. page.key ..
                " pc=" .. row.pc ..
                " writes=" .. entry.writes ..
                " firstAddress=" .. entry.firstAddress ..
                " lastAddress=" .. entry.lastAddress ..
                " sampleData=" .. entry.sampleData ..
                " sampleMask=" .. entry.sampleMask
            )
        end
    end
    traceActive = wasActive
end

local function installTap(page)
    local name = "hgr-writer-pc-" .. page.key
    local ok, tap = pcall(function()
        return program:install_write_tap(page.start, page.finish, name, function(offset, data, mask)
            recordWriter(page, offset, data, mask)
        end)
    end)
    print("SUNDOG_HGR_WRITER_TAP page=" .. page.key .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
    if ok then
        tapHandles[#tapHandles + 1] = tap
    end
end

local function removeTaps()
    for index, tap in ipairs(tapHandles) do
        local ok, result = pcall(function() return tap:remove() end)
        print("SUNDOG_HGR_WRITER_TAP_REMOVE index=" .. index .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
    tapHandles = {}
end

for _, page in ipairs(pages) do
    installTap(page)
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3500 then
        snapshot("title")
    elseif frame == 3600 then
        resetWindow("title-to-flip-prompt")
    elseif frame == 3900 then
        action:set_value(1)
        print("SUNDOG_HGR_WRITER_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
        print("SUNDOG_HGR_WRITER_PLAY_UP frame=" .. frame)
    elseif frame == 4200 then
        finishWindow("flip-prompt")
        snapshot("flip-prompt")
        resetWindow("flip-prompt-to-shipboard")
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_HGR_WRITER_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
        print("SUNDOG_HGR_WRITER_CONFIRM_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 4380 then
        action:clear_value()
        print("SUNDOG_HGR_WRITER_CONFIRM_UP frame=" .. frame)
    elseif frame == 5000 then
        finishWindow("shipboard")
        snapshot("shipboard")
        resetWindow("shipboard-to-status")
    elseif frame == 5200 then
        back:set_value(1)
        print("SUNDOG_HGR_WRITER_STATUS_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 5230 then
        back:clear_value()
        print("SUNDOG_HGR_WRITER_STATUS_UP frame=" .. frame)
    elseif frame == 5350 then
        finishWindow("status-open")
        snapshot("status-open")
        resetWindow("status-stable")
    elseif frame == 5900 then
        finishWindow("status-stable")
        snapshot("status-stable")
        removeTaps()
        print("SUNDOG_HGR_WRITER_TRACE_COMPLETE frame=" .. frame)
    end
end, "sundog-hgr-writer-pc-trace")
