-- SunDog Apple IIe 船內長按左鍵探測器。
-- 研究用：驗證游標／左鍵是否形成可重播的船內移動畫面，不替候選位置命名。
-- 原始 DSK 唯讀；所有截圖只寫入 Docker 的 /hosttmp。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local out = "/hosttmp/sundog-apple2-ship-walk-probe-v1-20260811-"
local frame = 0

-- 這組值對應船艦左側的候選物件區；軸值仍不是 host 螢幕座標。
local targetX = 70
local targetY = 154

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function snapshot(name)
    if screen then
        local ok, result = pcall(function() return screen:snapshot(out .. name .. ".png") end)
        print("SUNDOG_SHIP_WALK_SNAPSHOT name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
end

emu.register_frame_done(function()
    frame = frame + 1
    if frame == 3900 then
        action:set_value(1)
        print("SUNDOG_SHIP_WALK_PLAY_DOWN frame=" .. frame)
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_SHIP_WALK_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5100 then
        snapshot("before-walk")
        x:set_value(targetX)
        y:set_value(targetY)
        action:set_value(1)
        print("SUNDOG_SHIP_WALK_ACTION_DOWN frame=" .. frame .. " x=" .. targetX .. " y=" .. targetY)
    elseif frame == 5400 then
        snapshot("walk-300")
    elseif frame == 5700 then
        snapshot("walk-600")
    elseif frame == 6000 then
        snapshot("walk-900")
    elseif frame == 6300 then
        action:clear_value()
        snapshot("after-walk-release")
        print("SUNDOG_SHIP_WALK_ACTION_UP frame=" .. frame)
    elseif frame == 6900 then
        back:set_value(1)
        print("SUNDOG_SHIP_WALK_BACK_DOWN frame=" .. frame)
    elseif frame == 6930 then
        back:clear_value()
    elseif frame == 7500 then
        snapshot("after-back")
    end
end, "frame")
