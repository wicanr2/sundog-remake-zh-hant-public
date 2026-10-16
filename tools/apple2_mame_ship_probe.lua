-- SunDog Apple IIe 船內游標位置探測器。
--
-- 這是研究用 oracle harness，不是 remake 輸入層：
--   * 原始 DSK 由 Docker 唯讀掛載；
--   * 只記錄明確 joystick 軸值、按鍵事件與截圖；
--   * 不把任何候選位置直接升格為設施、碰撞或規則語意。
--
-- 使用方式與 apple2_mame_player_oracle.lua 相同，輸出寫入 /hosttmp。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local out = "/hosttmp/sundog-apple2-ship-probe-v1-20260811-"

-- 軸值是 Apple II gameio 值，不是 host 螢幕像素。
-- 先保留寬分布候選；命中位置仍需以畫面與後續輸入重播確認。
local targets = {
    {name = "left-upper",    x = 80,  y = 100},
    {name = "left-middle",   x = 80,  y = 144},
    {name = "center-upper",  x = 140, y = 100},
    {name = "center-middle", x = 140, y = 144},
    {name = "center-lower",  x = 140, y = 180},
    {name = "baseline",      x = 182, y = 144},
    {name = "right-upper",   x = 224, y = 100},
    {name = "right-middle",  x = 224, y = 144},
    {name = "right-lower",   x = 224, y = 180}
}

local frame = 0
local targetIndex = 0
local state = "boot"

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function snapshot(name)
    if screen then
        local ok, result = pcall(function()
            return screen:snapshot(out .. name .. ".png")
        end)
        print("SUNDOG_SHIP_PROBE_SNAPSHOT name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    else
        print("SUNDOG_SHIP_PROBE_SNAPSHOT name=" .. name .. " screen=nil")
    end
end

local function set_target(target)
    x:set_value(target.x)
    y:set_value(target.y)
    print("SUNDOG_SHIP_PROBE_TARGET index=" .. tostring(targetIndex) .. " name=" .. target.name .. " x=" .. tostring(target.x) .. " y=" .. tostring(target.y))
end

emu.register_frame_done(function()
    frame = frame + 1

    if frame == 3900 then
        action:set_value(1)
        print("SUNDOG_SHIP_PROBE_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_SHIP_PROBE_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5100 then
        snapshot("ship-baseline")
        targetIndex = 1
        state = "move"
        set_target(targets[targetIndex])
    elseif state == "move" and frame >= 5100 + (targetIndex - 1) * 500 + 120 then
        action:set_value(1)
        print("SUNDOG_SHIP_PROBE_ACTION_DOWN frame=" .. frame .. " index=" .. tostring(targetIndex))
        state = "click"
    elseif state == "click" and frame >= 5100 + (targetIndex - 1) * 500 + 150 then
        action:clear_value()
        snapshot("after-" .. targets[targetIndex].name)
        print("SUNDOG_SHIP_PROBE_ACTION_UP frame=" .. frame .. " index=" .. tostring(targetIndex))
        if targetIndex < #targets then
            targetIndex = targetIndex + 1
            state = "move"
            set_target(targets[targetIndex])
        else
            snapshot("ship-final")
            state = "done"
        end
    end
end, "frame")
