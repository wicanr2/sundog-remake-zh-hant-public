-- SunDog Apple IIe status-window sleep oracle.
-- 原始 DSK 由 Docker 以唯讀方式掛載；截圖只寫入 /hosttmp。
local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local out = "/hosttmp/sundog-apple2-status-sleep-v1-20260811-"
local frame = 0

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function snapshot(name)
    if screen then
        local ok, result = pcall(function() return screen:snapshot(out .. name .. ".png") end)
        print("SUNDOG_STATUS_SLEEP_SNAPSHOT name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3900 then
        action:set_value(1)
        print("SUNDOG_STATUS_SLEEP_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_STATUS_SLEEP_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5200 then
        back:set_value(1)
        print("SUNDOG_STATUS_SLEEP_STATUS_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 5230 then
        back:clear_value()
    elseif frame == 5900 then
        snapshot("before-sleep")
        x:set_value(140)
        y:set_value(144)
        action:set_value(1)
        print("SUNDOG_STATUS_SLEEP_ACTION_DOWN frame=" .. frame .. " x=140 y=144")
    elseif frame == 6030 then
        action:clear_value()
        print("SUNDOG_STATUS_SLEEP_ACTION_UP frame=" .. frame)
    elseif frame == 6600 then
        snapshot("after-sleep")
    elseif frame == 7200 then
        snapshot("late-sleep")
        back:set_value(1)
        print("SUNDOG_STATUS_SLEEP_WAKE_DOWN frame=" .. frame .. " x=140 y=144")
    elseif frame == 7230 then
        back:clear_value()
        print("SUNDOG_STATUS_SLEEP_WAKE_UP frame=" .. frame)
    elseif frame == 7800 then
        snapshot("after-wake")
    end
end, "frame")
