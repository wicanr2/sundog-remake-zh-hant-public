-- SunDog Apple IIe 玩家路徑最小 oracle。
-- 只在 Docker 內由 MAME 執行；原始 DSK 以唯讀方式掛載，截圖寫入 /hosttmp。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local frame = 0

-- 這是 Apple II gameio 的 8-bit 軸值，不是 host 螢幕座標。
x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function snapshot(name)
    if screen then
        local ok, result = pcall(function()
            return screen:snapshot("/hosttmp/sundog-apple2-play-disk2-v1-20260811-" .. name .. ".png")
        end)
        print("SUNDOG_SNAPSHOT name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    else
        print("SUNDOG_SNAPSHOT name=" .. name .. " screen=nil")
    end
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3900 then
        print("SUNDOG_PLAY_CLICK_DOWN frame=" .. frame .. " x=" .. tostring(x.port:read()) .. " y=" .. tostring(y.port:read()))
        action:set_value(1)
    elseif frame == 3930 then
        action:clear_value()
        print("SUNDOG_PLAY_CLICK_UP frame=" .. frame)
    elseif frame == 4200 then
        snapshot("prompt-before-swap")
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        local filename = drive and drive.filename or "<no-drive>"
        print("SUNDOG_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result) .. " filename=" .. tostring(filename))
    elseif frame == 4350 then
        print("SUNDOG_PLAY_AFTER_SWAP_DOWN frame=" .. frame)
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
        print("SUNDOG_PLAY_AFTER_SWAP_UP frame=" .. frame)
    elseif frame == 5200 then
        snapshot("after-swap")
        print("SUNDOG_STATUS_CLICK_DOWN frame=" .. frame .. " x=" .. tostring(x.port:read()) .. " y=" .. tostring(y.port:read()))
        back:set_value(1)
    elseif frame == 5230 then
        back:clear_value()
        print("SUNDOG_STATUS_CLICK_UP frame=" .. frame)
    elseif frame == 6000 then
        snapshot("after-back")
    elseif frame == 7000 then
        snapshot("late")
    end
end, "frame")
