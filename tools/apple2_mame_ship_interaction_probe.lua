-- SunDog Apple IIe 船內左鍵時間序列探測器。
--
-- 目的：把游標目標的「按下／持續／放開／返回」畫面分開保存，避免把單張
-- snapshot 的局部重繪或動畫誤命名成 ZoomAction、設施或碰撞語意。
-- 這是研究用 oracle harness；原始 DSK 唯讀，截圖只寫入 /hosttmp。

local io = manager.machine.ioport
local images = manager.machine.images
local drive = images[":sl6:diskiing:0:525"]
local x = io.ports[":gameio:joy:joystick_1_x"].fields["P1 Joystick X"]
local y = io.ports[":gameio:joy:joystick_1_y"].fields["P1 Joystick Y"]
local action = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 1"]
local back = io.ports[":gameio:joy:joystick_buttons"].fields["P1 Button 2"]
local screen = manager.machine.screens[":screen"]
local disk2 = "/repo/Sundog Frozen Legacy v2.0 (1984)(FTL) (2)/Sundog Frozen Legacy v2.0 (1984)(FTL)(Disk 2 of 2)[cr Nutcracker].dsk"
local targetName = os.getenv("SUNDOG_TARGET_NAME") or ""
local targetPrefix = ""
if targetName ~= "" then
    targetPrefix = targetName .. "-"
end
local out = "/hosttmp/sundog-apple2-ship-interaction-v1-20260811-" .. targetPrefix

-- 先用一個既有游標映射目標；它不是 host 螢幕座標，也不預設任何語意。
local targetX = tonumber(os.getenv("SUNDOG_TARGET_X") or "140")
local targetY = tonumber(os.getenv("SUNDOG_TARGET_Y") or "100")
local frame = 0
local phase = "boot"
local phaseFrame = 0

x:set_value(182)
y:set_value(144)
action:clear_value()
back:clear_value()

local function snapshot(name)
    if screen then
        local ok, result = pcall(function()
            return screen:snapshot(out .. name .. ".png")
        end)
        print("SUNDOG_SHIP_INTERACTION_SNAPSHOT name=" .. name .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    end
end

local function start_target()
    x:set_value(targetX)
    y:set_value(targetY)
    action:set_value(1)
    phase = "hold"
    phaseFrame = 0
    print("SUNDOG_SHIP_INTERACTION_ACTION_DOWN frame=" .. frame .. " target=" .. targetName .. " x=" .. targetX .. " y=" .. targetY)
end

emu.register_frame_done(function()
    frame = frame + 1

    if frame == 3900 then
        action:set_value(1)
        print("SUNDOG_SHIP_INTERACTION_PLAY_DOWN frame=" .. frame .. " x=182 y=144")
    elseif frame == 3930 then
        action:clear_value()
    elseif frame == 4300 then
        local ok, result = pcall(function() return drive:load(disk2) end)
        print("SUNDOG_SHIP_INTERACTION_DISK_SWAP frame=" .. frame .. " ok=" .. tostring(ok) .. " result=" .. tostring(result))
    elseif frame == 4350 then
        action:set_value(1)
    elseif frame == 4380 then
        action:clear_value()
    elseif frame == 5100 then
        snapshot("baseline")
        start_target()
    elseif phase == "hold" then
        phaseFrame = phaseFrame + 1
        if phaseFrame == 30 then
            snapshot("hold-030")
        elseif phaseFrame == 120 then
            snapshot("hold-120")
        elseif phaseFrame == 300 then
            snapshot("hold-300")
        elseif phaseFrame == 600 then
            action:clear_value()
            snapshot("release-600")
            phase = "released"
            phaseFrame = 0
            print("SUNDOG_SHIP_INTERACTION_ACTION_UP frame=" .. frame)
        end
    elseif phase == "released" then
        phaseFrame = phaseFrame + 1
        if phaseFrame == 300 then
            snapshot("released-300")
            back:set_value(1)
            phase = "back"
            phaseFrame = 0
            print("SUNDOG_SHIP_INTERACTION_BACK_DOWN frame=" .. frame)
        end
    elseif phase == "back" then
        phaseFrame = phaseFrame + 1
        if phaseFrame == 30 then
            back:clear_value()
            print("SUNDOG_SHIP_INTERACTION_BACK_UP frame=" .. frame)
        elseif phaseFrame == 300 then
            snapshot("after-back")
            phase = "done"
        end
    end
end, "frame")
