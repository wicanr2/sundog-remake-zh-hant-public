-- SunDog Apple IIe HGR writer code-provenance oracle。
--
-- 此工具只重播既有 Play → Disk 2 → 右鍵狀態視窗的正常玩家輸入。當
-- :maincpu 寫入標準 HGR page 1／2 時，它只在 callback 記錄 PC，並在同一個觀測窗口
-- 結束後才讀取 64 bytes 的 RAM code window，供 /tmp 內的唯讀 DSK 比對工具使用。程式碼
-- window、MAME log 與
-- 任何比對輸出都不是 remake runtime 輸入、存檔格式或可散布素材。
--
-- `$xxxx` 為 MAME :maincpu program-space；callback PC 及其相鄰 bytes 只表示當下
-- 動態觀測，尚未命名為函式，也不自動證明 raw DSK 的 loader／file-system 路由。

local ioPorts = manager.machine.ioport
local images = manager.machine.images
local cpu = manager.machine.devices[":maincpu"]
local program = cpu.spaces["program"]
local drive = images[":sl6:diskiing:0:525"]
local x = ioPorts.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = ioPorts.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = ioPorts.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = ioPorts.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local disk2 = os.getenv("SUNDOG_DISK2") or "/disk2"

local pages = {
    { key = "hgr-page-1", start = 0x2000, finish = 0x3fff },
    { key = "hgr-page-2", start = 0x4000, finish = 0x5fff },
}
local windows = {
    "title-to-flip-prompt",
    "flip-prompt-to-shipboard",
    "shipboard-to-status",
    "status-open",
    "status-stable",
}
local sampleBeforePC = 8
local sampleLength = 64
local writableRAMStart = 0x0800
local writableRAMFinish = 0xbfff

local frame = 0
local traceActive = false
local currentWindow = "not-started"
local writeCounts = {}
local writerPCs = {}
local codeSamples = {}
local tapHandles = {}

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function programCounterValue()
    local ok, value = pcall(function() return cpu.state["PC"].value end)
    if ok and type(value) == "number" then
        return value
    end
    return nil
end

local function bytesAsHex(startAddress, length)
    local parts = {}
    for offset = 0, length - 1 do
        parts[#parts + 1] = string.format("%02x", program:read_u8(startAddress + offset))
    end
    return table.concat(parts)
end

local function resetWindow(name)
    currentWindow = name
    writeCounts[name] = {}
    writerPCs[name] = {}
    traceActive = true
    print("SUNDOG_HGR_CODE_WINDOW_START name=" .. name .. " frame=" .. frame)
end

local function recordCodeSample(page, pcValue)
    if not pcValue then
        return
    end
    local startAddress = pcValue - sampleBeforePC
    local finishAddress = startAddress + sampleLength - 1
    if startAddress < writableRAMStart or finishAddress > writableRAMFinish then
        return
    end
    local byWindow = codeSamples[currentWindow]
    if not byWindow then
        byWindow = {}
        codeSamples[currentWindow] = byWindow
    end
    local byPage = byWindow[page.key]
    if not byPage then
        byPage = {}
        byWindow[page.key] = byPage
    end
    local pc = string.format("$%04x", pcValue)
    if byPage[pc] then
        return
    end
    byPage[pc] = {
        startAddress = startAddress,
        bytes = bytesAsHex(startAddress, sampleLength),
    }
end

local function recordWriter(page)
    if not traceActive then
        return
    end
    local byPage = writeCounts[currentWindow]
    if not byPage then
        byPage = {}
        writeCounts[currentWindow] = byPage
    end
    byPage[page.key] = (byPage[page.key] or 0) + 1
    local pcValue = programCounterValue()
    if not pcValue then
        return
    end
    local byWindow = writerPCs[currentWindow]
    if not byWindow then
        byWindow = {}
        writerPCs[currentWindow] = byWindow
    end
    local byPagePC = byWindow[page.key]
    if not byPagePC then
        byPagePC = {}
        byWindow[page.key] = byPagePC
    end
    byPagePC[pcValue] = true
end

local function captureWindowSamples()
    local byWindow = writerPCs[currentWindow] or {}
    for _, page in ipairs(pages) do
        local byPagePC = byWindow[page.key] or {}
        local pcs = {}
        for pcValue, _ in pairs(byPagePC) do
            pcs[#pcs + 1] = pcValue
        end
        table.sort(pcs)
        for _, pcValue in ipairs(pcs) do
            recordCodeSample(page, pcValue)
        end
    end
end

local function finishWindow(name)
    local wasActive = traceActive
    traceActive = false
    print("SUNDOG_HGR_CODE_WINDOW_END name=" .. name .. " frame=" .. frame .. " trackedWindow=" .. currentWindow)
    captureWindowSamples()
    for _, page in ipairs(pages) do
        local count = 0
        if writeCounts[currentWindow] then
            count = writeCounts[currentWindow][page.key] or 0
        end
        print("SUNDOG_HGR_CODE_COUNT window=" .. currentWindow .. " page=" .. page.key .. " writes=" .. count)
    end
    traceActive = wasActive
end

local function emitCodeSamples()
    for _, window in ipairs(windows) do
        local byWindow = codeSamples[window] or {}
        for _, page in ipairs(pages) do
            local byPage = byWindow[page.key] or {}
            local rows = {}
            for pc, sample in pairs(byPage) do
                rows[#rows + 1] = { pc = pc, sample = sample }
            end
            table.sort(rows, function(left, right) return left.pc < right.pc end)
            for _, row in ipairs(rows) do
                print(
                    "SUNDOG_HGR_CODE_SAMPLE" ..
                    " window=" .. window ..
                    " page=" .. page.key ..
                    " pc=" .. row.pc ..
                    " start=" .. string.format("$%04x", row.sample.startAddress) ..
                    " length=" .. sampleLength ..
                    " bytes=" .. row.sample.bytes
                )
            end
        end
    end
end

local function installTap(page)
    local name = "hgr-code-provenance-" .. page.key
    local ok, tap = pcall(function()
        return program:install_write_tap(page.start, page.finish, name, function()
            recordWriter(page)
        end)
    end)
    print("SUNDOG_HGR_CODE_TAP page=" .. page.key .. " ok=" .. tostring(ok) .. " result=" .. tostring(tap))
    if ok then
        tapHandles[#tapHandles + 1] = tap
    end
end

local function removeTaps()
    for index, tap in ipairs(tapHandles) do
        local ok, result = pcall(function() return tap:remove() end)
        print("SUNDOG_HGR_CODE_TAP_REMOVE index=" .. index .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
    tapHandles = {}
end

for _, page in ipairs(pages) do
    installTap(page)
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3600 then
        resetWindow("title-to-flip-prompt")
    elseif frame == 3900 then
        action:set_value(1)
        print("SUNDOG_HGR_CODE_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
        print("SUNDOG_HGR_CODE_PLAY_UP frame=" .. frame)
    elseif frame == 4200 then
        finishWindow("flip-prompt")
        resetWindow("flip-prompt-to-shipboard")
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_HGR_CODE_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
        print("SUNDOG_HGR_CODE_CONFIRM_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 4380 then
        action:clear_value()
        print("SUNDOG_HGR_CODE_CONFIRM_UP frame=" .. frame)
    elseif frame == 5000 then
        finishWindow("shipboard")
        resetWindow("shipboard-to-status")
    elseif frame == 5200 then
        back:set_value(1)
        print("SUNDOG_HGR_CODE_STATUS_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 5230 then
        back:clear_value()
        print("SUNDOG_HGR_CODE_STATUS_UP frame=" .. frame)
    elseif frame == 5350 then
        finishWindow("status-open")
        resetWindow("status-stable")
    elseif frame == 5900 then
        finishWindow("status-stable")
        emitCodeSamples()
        removeTaps()
        print("SUNDOG_HGR_CODE_TRACE_COMPLETE frame=" .. frame)
    end
end, "sundog-hgr-code-provenance")
