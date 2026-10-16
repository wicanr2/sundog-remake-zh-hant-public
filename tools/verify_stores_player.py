"""Docker/Xvfb正常新局Stores、固定攜帶槽、食用及拖動中保存重開。

入口：banville-restoration-spec.md的正式物品操作與保存READY。
不注入狀態、食物、座標或seed。原版GUI與睡眠另驗。
"""
import argparse
from collections import deque
import copy
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import time


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["binary", "disk", "settings-input", "output"]:
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--sleep", action="store_true", help="另驗正常睡眠、兩保存路徑重開、整點及醒來")
    parser.add_argument("--awake-clock", action="store_true", help="另驗清醒時鐘、拖動停鐘、保存相位與正常地面移動")
    parser.add_argument("--library-input", type=Path, help="只複製目前schema保存庫，以正常UI續行回船並攜帶剩餘四漢堡")
    parser.add_argument("--flight-setup", action="store_true", help="由四漢堡正常船艙接點購油、導航出發並驗ETA與兩保存續行")
    parser.add_argument("--flight-arrival", action="store_true", help="由未修改航行保存驗自然抵達及兩保存續行")
    parser.add_argument("--cryo-gate", action="store_true", help="從正常Goods31取貨保存回船、開戰術、驗無遭遇及兩保存重開")
    parser.add_argument("--prepare-cryo", action="store_true", help="從Drahew四漢堡船艙正常放回一個漢堡，騰出掃描器攜帶槽並驗兩保存")
    parser.add_argument("--prepare-cryo-resume", action="store_true", help="配合--prepare-cryo，從同局已正常放回漢堡的Stores保存接續重開")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只在Docker執行")
    root = Path(__file__).resolve().parents[1]
    schema = int(re.search(r"const CurrentSchema = (\d+)", (root / "internal/game/state.go").read_text()).group(1))
    for name in ["binary", "disk", "settings_input"]:
        setattr(args, name, getattr(args, name).resolve(strict=True))
    if args.library_input:
        args.library_input = args.library_input.resolve(strict=True)
        if not args.awake_clock or args.sleep:
            parser.error("保存庫續行需要--awake-clock，且不使用--sleep")
        source_state = json.loads(args.library_input.read_text())["slots"]["slot-1"]
        if source_state["schema"] != schema:
            parser.error("只支援目前schema的新存檔，不遷移")
    elif args.flight_setup or args.flight_arrival:
        parser.error("航行驗證需要正常保存庫接點")
    if args.flight_setup and args.flight_arrival:
        parser.error("航行出發與抵達分別驗證")
    if args.cryo_gate and (not args.library_input or args.flight_setup or args.flight_arrival):
        parser.error("冷凍人門檻須獨立正常取貨保存，不併入航行驗證")
    if args.prepare_cryo and (not args.library_input or args.cryo_gate or args.flight_setup or args.flight_arrival):
        parser.error("冷凍人準備須獨立Drahew正常保存，不併入其他模式")
    if args.prepare_cryo_resume and (not args.prepare_cryo or not args.library_input):
        parser.error("準備續行須配合--prepare-cryo及同局正常保存")
    if args.prepare_cryo and (source_state["screen"] != ("stores" if args.prepare_cryo_resume else "shipboard") or source_state["cityId"] != "drahew" or source_state["parts"] != ([0,8,8,8] if args.prepare_cryo_resume else [8]*4) or source_state.get("handItem")):
        parser.error("須同局Drahew四漢堡船艙保存")
    if args.cryo_gate and (source_state["screen"] not in ("trade","shipboard") or source_state["screen"] == "trade" and source_state["groundMode"] != "pod" or source_state["cityId"] != source_state["shipCityId"] or source_state.get("tradePurchaseLot") or not any(h.get("goods") == 31 for h in source_state["holds"]) or any(h.get("kind") and h.get("goods") != 31 for h in source_state["holds"])):
        parser.error("須同城正常pod取貨保存，貨艙只持有Goods31")
    if sha(args.disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("原始磁碟身分不符")
    if json.loads(args.settings_input.read_text()).get("layout") != "atari-st":
        parser.error("需要Atari ST版面設定")
    out = args.output.resolve()
    if out.exists() or out.parent.stat().st_uid != os.getuid():
        parser.error("輸出須為目前使用者所屬的全新子目錄")
    out.mkdir()
    # Run in the writable output; source assets remain on the read-only mount.
    for name in ("assets", "docs"):
        (out / name).symlink_to(root / name, target_is_directory=True)
    (out / "settings.json").write_bytes(args.settings_input.read_bytes())
    if args.library_input:
        (out / "library.json").write_bytes(args.library_input.read_bytes())
    env = dict(os.environ, DISPLAY=":96", EBITENGINE_SCREENSHOT_KEY="F12")
    checks, inputs, reentries, clock_reentries = [], [], [], []
    process = window = None
    display = subprocess.Popen(["Xvfb", ":96", "-screen", "0", "1280x800x24", "-nolisten", "tcp", "-noreset"],
                               stdout=(out / "xvfb.log").open("w"), stderr=subprocess.STDOUT)

    def run(command):
        subprocess.run(command, env=env, check=True, stdout=subprocess.DEVNULL)

    def key(value):
        inputs.append({"key": value})
        run(["xdotool", "key", "--clearmodifiers", value])
        time.sleep(.20)

    def click(x, y, button=1):
        inputs.append({"click": [x, y, button]})
        run(["xdotool", "mousemove", "--window", window, str(x*3), str(y*3)])
        time.sleep(.10)
        run(["xdotool", "click", str(button)])
        time.sleep(.25)

    def start(load=False):
        nonlocal process, window
        cmd = [str(args.binary), "-scale", "3", "-start", "title", "-atari-disk", str(args.disk),
               "-settings", str(out / "settings.json"), "-save", str(out / "save.json"),
               "-library", str(out / "library.json"), "-locale", "zh-Hant"]
        if load:
            cmd.append("-load")
        process = subprocess.Popen(cmd, cwd=out, env=env, stdout=(out / f"game-{len(checks)}.log").open("w"),
                                   stderr=subprocess.STDOUT)
        for _ in range(100):
            if process.poll() is not None:
                raise RuntimeError("遊戲提早退出")
            result = subprocess.run(["xdotool", "search", "--onlyvisible", "--name", "SunDog"],
                                    env=env, text=True, stdout=subprocess.PIPE)
            if result.returncode == 0 and result.stdout.strip():
                window = result.stdout.strip().splitlines()[-1]
                break
            time.sleep(.1)
        else:
            raise RuntimeError("找不到遊戲視窗")
        run(["xdotool", "windowfocus", window])
        time.sleep(.7)

    def capture(name):
        library = out / "library.json"
        stamp = library.stat().st_mtime_ns if library.exists() else 0
        key("F5")
        if not library.exists():
            key("m")
        for value in ["1", "s", "Escape"]:
            key(value)
        assert library.stat().st_mtime_ns != stamp, "保存庫未更新"
        raw = library.read_bytes()
        (out / ("library-" + name + ".json")).write_bytes(raw)
        s = json.loads(raw)["slots"]["slot-1"]
        checks.append(dict(name=name, state=s, sha256=hashlib.sha256(raw).hexdigest()))
        print(name, s["screen"], s["handItem"], s["handOrigin"], flush=True)
        return s

    def shot(name):
        # Library Back has a two-second message overlay. Capture the actual
        # persistent readouts after it expires, not that transient message.
        time.sleep(2.3)
        before = set(out.glob("screenshot*.png"))
        key("F12")
        for _ in range(50):
            created = set(out.glob("screenshot*.png")) - before
            if created:
                break
            time.sleep(.1)
        assert len(created) == 1, "同步截圖未完成"
        created.pop().rename(out / (name + ".png"))

    fields = ["schema", "screen", "stores", "parts", "handItem", "handOrigin", "vigor", "rest",
              "health", "nourishment", "timeMinutes", "holds", "warehouses", "colony", "colonySite",
              "cash", "bank", "bankBalances", "fuel", "shipSystems", "position", "shipPosition",
              "shipCityId", "cargoAttached", "selectedPartIndex", "attributes", "attributeMax", "sleep", "sleepTheftProtected"]
    if args.awake_clock:
        fields = [v for v in fields if v not in ("timeMinutes", "vigor", "rest", "health", "nourishment")]

    def advance_clock(before, ticks):
        # Independent semantic clock model. No wall-clock seconds conversion.
        assert 0 <= ticks < 600, "續行重置了clock epoch或跨越有界驗證範圍"
        c = copy.deepcopy(before)
        minutes = ships = 0
        for _ in range(ticks):
            c["source"]["ticks"] = (c["source"]["ticks"]+1) % 2**32
            c["source"]["eventCountdown"] -= 1
            if c["source"]["eventCountdown"]:
                continue
            c["source"]["eventCountdown"] = 4
            b = c["background"]
            b["waitEvents"] -= 1
            if b["waitEvents"]:
                continue
            for slot in b["slots"]:
                if slot["callback"] and slot["remaining"] == 0:
                    slot["remaining"] = slot["period"]
                    minutes += slot["callback"] == 1
                    ships += slot["callback"] == 2
            delay = min([8]+[s["remaining"] for s in b["slots"] if s["callback"]])
            for slot in b["slots"]:
                if slot["callback"]:
                    slot["remaining"] -= delay
            b["waitEvents"] = delay if delay else 65536
        return c, minutes, ships

    def check_clock_resume(before, after, name, atomic_minutes=0):
        delta = (after["awakeClock"]["source"]["ticks"]-before["awakeClock"]["source"]["ticks"]) % 2**32
        expected, minutes, ships = advance_clock(before["awakeClock"], delta)
        assert after["awakeClock"] == expected, "續行改變event相位、worker等待或槽倒數"
        assert after["shipPhase"] == (before["shipPhase"]+ships) % 3, "船艦callback次數不同"
        assert after["timeMinutes"] == before["timeMinutes"]+minutes+atomic_minutes, "清醒分鐘consumer次數不同，或預扣了mode2 ETA"
        clock_reentries.append(dict(name=name, advancedVBLs=delta, minuteCallbacks=minutes, shipCallbacks=ships,
                                    retainedAtomicMinutes=atomic_minutes,
                                    before=before["awakeClock"], after=after["awakeClock"]))

    def reopen(name, library=False):
        key("q")
        process.wait(timeout=15)
        before = json.loads((out / "save.json").read_text())["state"]
        reference = json.loads((out / "library.json").read_text())["slots"]["slot-1"] if library else before
        start(load=True)
        if library:
            for value in ["F5", "1", "l"]:
                key(value)
        after = capture(name)
        assert all(before.get(k) == after.get(k) for k in fields), "新程序重開欄位不同"
        if args.awake_clock:
            check_clock_resume(reference, after, name)
        reentries.append(dict(name=name, fields=fields, before={k: before.get(k) for k in fields},
                              after={k: after.get(k) for k in fields}))
        return after

    try:
        for _ in range(60):
            if display.poll() is not None:
                raise RuntimeError("Xvfb已退出，不能啟動遊戲")
            if subprocess.run(["xwininfo", "-root"], env=env, stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(.1)
        else:
            raise RuntimeError("Xvfb未就緒，不能啟動遊戲")
        start()
        if args.library_input:
            for value in ["space", "c", "1", "l"]:
                key(value)
            loaded = capture("01-current-save-loaded")
            if args.prepare_cryo:
                original_stores = list(loaded["stores"])
                expected_stores = list(original_stores)
                if args.prepare_cryo_resume:
                    assert loaded["screen"] == "stores" and loaded["parts"] == [0,8,8,8]
                    stored = loaded
                else:
                    assert loaded["screen"] == "shipboard" and loaded["parts"] == [8]*4
                    slot = next(i for i, item in enumerate(original_stores) if item == 0)
                    key("b")
                    stores = capture("02-preparation-stores")
                    assert stores["screen"] == "stores"
                    check_clock_resume(loaded,stores,"stores-entry")
                    click(108,58)
                    held = capture("03-preparation-burger-held")
                    assert held["handItem"] == 8 and held["handOrigin"] == dict(kind="pocket",index=0)
                    assert held["parts"] == [0,8,8,8] and held["awakeClock"]["minutePause"] == 60
                    click(80+1+51+22*(slot%5),1+24+17*(slot//5))
                    stored = capture("04-preparation-burger-stored")
                    expected_stores[slot] = 8
                assert stored["parts"] == [0,8,8,8] and stored["stores"] == expected_stores
                assert not stored["handItem"] and stored["awakeClock"]["minutePause"] == 0
                for name in ("holds","warehouses","colony","colonySite","cash","bank","shipSystems"):
                    assert stored[name] == loaded[name], name
                # Existing ship callbacks may charge guns and consume fuel.
                # Keep the live clock; fuel is not a frozen UI/save field.
                assert 0 < stored["fuel"] <= loaded["fuel"]
                shot("04-preparation-burger-stored")
                key("Escape")
                ready = capture("05-preparation-shipboard")
                assert ready["screen"] == "shipboard"
                fields.remove("fuel")
                reopen("06-preparation-single-reopened")
                final = reopen("07-preparation-library-reopened",library=True)
                assert final["parts"] == [0,8,8,8] and final["stores"] == expected_stores
                assert sum(item != 0 and item & 31 == 8 for item in final["parts"]+final["stores"]) == 4
                receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,reentries=reentries,
                    clockReentries=clock_reentries,
                    scope="同局Drahew正常供貨主線，以Stores滑鼠將攜帶槽0漢堡放回第一空庫存槽，保留四漢堡並騰出掃描器空槽，兩保存新程序重開；不食用、不注入或改寫保存。",
                    originalGUI="unverified",toolSHA256=sha(Path(__file__)),
                    resumedStores=args.prepare_cryo_resume,
                    inputSHA256={name:sha(getattr(args,name)) for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／{len(reentries)}次{len(fields)}欄位新程序重開",flush=True)
                return
            if args.cryo_gate:
                original_holds, original_warehouses = loaded["holds"], loaded["warehouses"]
                if loaded["screen"] == "trade":
                    state = loaded
                    for attempt in range(4):
                        if state["screen"] == "ground":
                            break
                        key("Escape")
                        state = capture(f"02-trade-exit-{attempt}")
                    assert state["screen"] == "ground" and state["groundMode"] == "pod" and not state["groundWorld"]
                    for attempt in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "remake-modern-tiles":
                            break
                        key("F8")
                        state = capture(f"03-grid-presentation-{attempt}")
                    assert state["layout"] == "remake" and state["tileset"] == "remake-modern-tiles"
                    source = json.loads((root/"internal/source/atarist/assets/world-data.json").read_text())
                    row = next(x for x in source["tables"]["cities"]["records"] if re.sub(r"[^a-z0-9]+", "-", x["name"].lower()).strip("-") == state["cityId"])
                    city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][row["index"]]
                    current = (state["position"]["x"], state["position"]["y"])
                    ship = (state["shipPosition"]["x"], state["shipPosition"]["y"])
                    # Reach an east/west neighbour first. A horizontal final move
                    # is the existing normal pod boarding action, not a teleport.
                    goals = {(ship[0]-1,ship[1]), (ship[0]+1,ship[1])}
                    bounds = (min(-1,current[0],ship[0]-1),max(21,current[0],ship[0]+1),min(-1,current[1],ship[1]),max(12,current[1],ship[1]))
                    buildings = {(b["row"],b["column"]) for b in city_map["buildings"]}
                    queue, previous, goal = deque([current]), {current: None}, None
                    while queue:
                        point = queue.popleft()
                        if point in goals:
                            goal = point
                            break
                        for value,dx,dy in [("Up",0,-1),("Right",1,0),("Down",0,1),("Left",-1,0)]:
                            nxt = (point[0]+dx,point[1]+dy)
                            nx,ny = nxt
                            if nxt in previous or nxt == ship or not (bounds[0]<=nx<=bounds[1] and bounds[2]<=ny<=bounds[3]):
                                continue
                            if 0<=nx<20 and 0<=ny<12 and nxt not in buildings and city_map["grid"][nx][ny] in [6,7,12,13,14]:
                                continue
                            previous[nxt] = (point,value)
                            queue.append(nxt)
                    assert goal is not None, "正常來源地圖找不到船門路線"
                    route, point = [], goal
                    while point != current:
                        parent,value = previous[point]
                        route.append((value,point))
                        point = parent
                    for index,(value,point) in enumerate(reversed(route)):
                        key(value)
                        state = capture(f"04-return-to-ship-{index}")
                        if state["screen"] == "dialog" and state["groundReturn"] == "ground":
                            key("Escape")
                            state = capture(f"04-dismiss-dialog-{index}")
                        assert state["screen"] == "ground" and not state["groundWorld"] and (state["position"]["x"],state["position"]["y"]) == point
                    key("Right" if goal[0]<ship[0] else "Left")
                    boarded = capture("05-normal-pod-boarded")
                    assert boarded["screen"] == "shipboard" and boarded["cargoAttached"]
                    assert boarded["holds"] == original_holds and boarded["warehouses"] == original_warehouses
                else:
                    boarded = loaded
                if boarded["layout"] != "atari-st":
                    key("F8")
                ready = capture("06-atari-st-shipboard")
                assert ready["layout"] == "atari-st"
                check_clock_resume(boarded,ready,"presentation-after-boarding")
                key("x")
                tactical = capture("07-cryo-tactical-gate")
                assert tactical["screen"] == "tactical" and not tactical.get("combatActive") and tactical.get("enemyMaxHull",0) == 0
                assert tactical["combatRolls"] == ready["combatRolls"], "Goods31錯誤觸發遭遇或RND"
                assert tactical["holds"] == original_holds and tactical["warehouses"] == original_warehouses
                check_clock_resume(ready,tactical,"tactical-entry")
                shot("07-cryo-tactical-gate")
                fields.remove("fuel")
                fields += ["cityId","systemId","planetId","cargoCityId","cargoPosition","combatActive","combatRolls","enemyMaxHull","enemyHull","shipControls"]
                reopen("08-tactical-single-reopened")
                final = reopen("09-tactical-library-reopened",library=True)
                assert final["holds"] == original_holds and final["warehouses"] == original_warehouses
                assert not final.get("combatActive") and final["combatRolls"] == ready["combatRolls"]
                assert sum(l["goods"]==31 for slots in final["warehouses"].values() for l in slots) == 16
                shot("09-cryo-tactical-reopened")
                receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,reentries=reentries,clockReentries=clock_reentries,
                    scope="同一正常新局Goods31取貨保存，正常交易退出、Modern鍵盤移動及水平pod回船、Atari ST戰術無敵艦／無RND、兩保存新程序重開；16倉庫及一批貨艙資料保持。",
                    resumedShipboard=loaded["screen"]=="shipboard",
                    remaining="取得與設備／購油／航行由同局上游收據保存；完整有貨遭遇／交戰、原版GUI及九期另待。",
                    originalGUI="unverified",randomness="同局自然新存檔，不注入或重抽；原版1024貨艙門檻不使用RND，這條GUI驗無新增遭遇抽值。",
                    toolSHA256=sha(Path(__file__)),inputSHA256={name:sha(getattr(args,name)) for name in ["binary","disk","settings_input","library_input"]})
                if loaded["screen"] == "shipboard":
                    receipt["scope"] = "承接同局正常Goods31已回船保存，正式X鍵開Atari ST戰術無敵艦／無RND、兩保存新程序重開；取貨與回船由同局上游收據保存。16倉庫及一批貨艙資料保持。"
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／{len(reentries)}次{len(fields)}欄位新程序重開",flush=True)
                return
            if args.flight_arrival:
                assert loaded["screen"] == "flight" and loaded["parts"] == [8]*4
                assert loaded["flightTargetId"] == "lightning" and loaded["flightTick"] < loaded["flightTicks"]
                final = loaded
                for attempt in range(24):
                    time.sleep(1.2)
                    previous = final
                    final = capture(f"02-natural-flight-{attempt:02}")
                    check_clock_resume(previous, final, f"natural-flight-{attempt:02}")
                    if final["flightTick"] >= final["flightTicks"]:
                        break
                    assert final["screen"] == "flight" and final["fuel"] > 0, "自然航行意外中止"
                else:
                    raise RuntimeError("有界自然航行尚未抵達")
                shot("03-natural-arrival")
                receipt = dict(status="FAIL-arrival-pending", saveSchema=schema, checks=checks, inputs=inputs,
                    reentries=reentries, clockReentries=clock_reentries, originalGUI="unverified",
                    scope="未修改schema44正常航行保存，自然背景抵達，無FlightTick／Engage快捷操作或注入。",
                    toolSHA256=sha(Path(__file__)), inputSHA256={name:sha(getattr(args,name)) for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                assert final["screen"] == "shipboard" and final["cityId"] == "lightning", "航段100%仍未完成所在地／畫面轉移"
                assert final["flightTargetId"] == "" and final["shipCityId"] == "lightning"
                assert final["cargoAttached"] and final["cargoCityId"] == "lightning" and final["parts"] == [8]*4
                fields.remove("fuel")
                fields += ["cityId", "planetId", "systemId", "cargoCityId", "flightTargetId", "flightTick", "flightTicks"]
                reopen("04-arrival-single-reopened")
                final = reopen("05-arrival-library-reopened",library=True)
                receipt.update(status="PASS", checks=checks, inputs=inputs, reentries=reentries,
                    clockReentries=clock_reentries, remaining="原子world／warp時間近似、交戰／航行睡眠及原版GUI另驗，非整款same-state。")
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／兩次{len(fields)}欄位抵達重開",flush=True)
                return
            if args.flight_setup:
                assert loaded["screen"] == "shipboard" and loaded["parts"] == [8]*4 and loaded["cityId"] == "drahew"
                assert loaded["fuel"] == 0 and loaded["handItem"] == 0

                def walk(x, y, seconds):
                    inputs.append(dict(walk=[x,y], heldSeconds=seconds))
                    run(["xdotool", "mousemove", "--window", window, str(x*3), str(y*3)])
                    run(["xdotool", "mousedown", "1"])
                    time.sleep(seconds)
                    run(["xdotool", "mouseup", "1"])
                    time.sleep(.2)

                for value in ["c", "e"]:
                    key(value)
                ground = capture("02-normal-pod-exit")
                assert ground["screen"] == "ground" and ground["groundMode"] == "pod"
                for attempt in range(3):
                    walk(225,95,1.6)
                    approach = capture(f"03-depot-approach-{attempt}")
                    anchor = approach["groundView"]["anchor"]
                    assert approach["screen"] == "ground"
                    if abs(anchor["x"]-225)<=1 and abs(anchor["y"]-95)<=1:
                        break
                else:
                    raise RuntimeError("正常depot接近未完成")
                walk(259,101,1.6)
                depot = capture("04-normal-depot")
                assert depot["screen"] == "fuel-purchase" and depot["fuelPurchase"]["kind"] == "main"
                key("1")
                purchased = capture("05-full-main-quantity")
                assert purchased["fuelPurchase"]["quantity"] == 16
                key("3")
                settled = capture("06-main-fuel-settled")
                assert settled["screen"] == "ground" and settled["fuel"] == 255 and settled["parts"] == [8]*4
                assert settled["bank"] < depot["bank"], "購油未實際付款"
                for x,y in [(190,101),(190,100),(184,100)]:
                    walk(x,y,.4)
                ready = capture("07-fueled-normal-shipboard")
                assert ready["screen"] == "shipboard" and ready["fuel"] > 0 and ready["parts"] == [8]*4
                key("n")
                key("Down")
                navigation = capture("08-normal-navigation")
                assert navigation["screen"] == "navigation" and navigation["navigationTargetId"] == "lightning"
                key("Return")
                # The normal ascent overlay consumes F5. Space is its existing
                # player shortcut, not a direct-entry or state-injection flag.
                key("space")
                flight = capture("09-mode2-departure")
                assert flight["screen"] == "flight" and flight["flightTargetId"] == "lightning"
                assert flight["landing"]["phase"] == ""
                assert flight["flightTick"] < flight["flightTicks"] and flight["parts"] == [8]*4
                check_clock_resume(navigation,flight,"mode2-departure",atomic_minutes=20)
                time.sleep(1.2)
                advanced = capture("10-flight-background")
                assert advanced["flightTick"] > flight["flightTick"] and advanced["screen"] == "flight"
                check_clock_resume(flight,advanced,"mode2-background")
                shot("10-flight-background")
                fields.remove("fuel")
                fields += ["flightTargetId","flightTicks","flightFromAngle","flightFromRadius","flightDeltaAngle","flightDeltaRadius"]
                reopen("11-flight-single-reopened")
                final = reopen("12-flight-library-reopened",library=True)
                assert final["parts"] == [8]*4 and final["screen"] == "flight"
                receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,reentries=reentries,
                    clockReentries=clock_reentries,scope="正常schema44四漢堡接點，貨艙出口、depot購滿主油並付款、水平回船、Lightning導航出發及正常空白鍵跳過起飛視覺效果；移除mode2 ETA預扣後驗自然背景與兩保存29欄位重開。",
                    remaining="20分鐘原子world近似保留；完整抵達、交戰及原版GUI未驗，不外推整段same-state。",
                    originalGUI="unverified",randomness="正常新局自然狀態；原生ETA初始化不使用RND。GUI引擎狀態16，不挑seed。",
                    toolSHA256=sha(Path(__file__)),inputSHA256={name:sha(getattr(args,name)) for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／{len(reentries)}次{len(fields)}欄位新程序重開",flush=True)
                return
            assert loaded["screen"] == "ground" and loaded["groundMode"] == "foot"
            assert loaded["parts"] == [0, 0, 0, 0] and loaded["handItem"] == 0
            burgers = [(i, v) for i, v in enumerate(loaded["stores"]) if v and v & 31 == 8]
            assert len(burgers) == 4, "正常接點須保留四個漢堡"
            view = loaded["groundView"]
            x = (loaded["shipPosition"]["x"]-view["origin"]["x"])*40+17
            y = (loaded["shipPosition"]["y"]-view["origin"]["y"])*40+20
            inputs.append(dict(walkToShipDoor=[x, y], heldSeconds=.25))
            run(["xdotool", "mousemove", "--window", window, str(x*3), str(y*3)])
            run(["xdotool", "mousedown", "1"])
            time.sleep(.25)
            run(["xdotool", "mouseup", "1"])
            time.sleep(.3)
            boarded = capture("02-normal-ship-door")
            assert boarded["screen"] == "shipboard", "正常滑鼠回船未成功"
            assert boarded["awakeClock"]["shipActive"] and boarded["stores"] == loaded["stores"]
            key("b")
            expected_stores = list(loaded["stores"])
            expected_parts = [0, 0, 0, 0]
            for pocket, ((slot, item), (px, py)) in enumerate(zip(burgers, [(27,57), (9,60), (9,77), (27,77)])):
                click(80+1+51+22*(slot%5), 1+24+17*(slot//5))
                held = capture(f"03-drag-burger-{pocket}")
                assert held["handItem"] == item and held["handOrigin"] == dict(kind="stores", index=slot)
                assert held["awakeClock"]["minutePause"] == 60
                click(80+1+px, 1+py)
                expected_stores[slot] = 0
                expected_parts[pocket] = item
                carried = capture(f"04-carry-burger-{pocket}")
                assert carried["stores"] == expected_stores and carried["parts"] == expected_parts
                assert carried["handItem"] == 0 and carried["awakeClock"]["minutePause"] == 0
            shot("04-four-carried")
            key("Escape")
            ready = capture("05-four-carried-shipboard")
            assert ready["screen"] == "shipboard" and ready["parts"] == expected_parts
            reopen("06-four-carried-single-reopened")
            final = reopen("07-four-carried-library-reopened", library=True)
            assert final["parts"] == expected_parts and final["stores"] == expected_stores
            assert final["nourishment"] == loaded["nourishment"], "只搬動物品不食用"
            shot("07-four-carried-shipboard")
            receipt = dict(status="PASS", saveSchema=schema, checks=checks, inputs=inputs,
                           reentries=reentries, clockReentries=clock_reentries,
                           scope="目前schema正常新局接點，保存庫UI載入、滑鼠原版門口回船、四漢堡逐一放入四攜帶槽、兩保存路徑重開及時鐘續行；不注入、遷移或食用。",
                           originalGUI="unverified", randomness="沿用正常新局自然狀態，不挑seed",
                           toolSHA256=sha(Path(__file__)), inputSHA256={name: sha(getattr(args, name)) for name in ["binary", "disk", "settings_input", "library_input"]})
            (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+"\n")
            print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／{len(reentries)}次{len(fields)}欄位新程序重開", flush=True)
            return
        for value in ["space", "c", "n", "z", "e", "d", "Return", "Return", "Return"]:
            key(value)
        initial = capture("01-newgame")
        assert initial["schema"] == schema and initial["screen"] == "shipboard"
        assert len(initial["parts"]) == 4 and not any(initial["parts"])
        assert len(initial["stores"]) == 20 and sum(v != 0 and v & 31 == 8 for v in initial["stores"]) == 5
        if args.awake_clock:
            time.sleep(1.2)
            idle = capture("01a-idle-clock")
            delta = idle["awakeClock"]["source"]["ticks"]-initial["awakeClock"]["source"]["ticks"]
            expected, minutes, ships = advance_clock(initial["awakeClock"], delta)
            assert idle["awakeClock"] == expected and minutes >= 1 and idle["timeMinutes"] == initial["timeMinutes"]+minutes
        key("b")
        stores = capture("02-stores-keyboard")
        assert stores["screen"] == "stores"
        slot = next(i for i, item in enumerate(stores["stores"]) if item != 0 and item & 31 == 8)
        for _ in range(slot):
            key("Right")
        key("Return")
        held = capture("03-drag-keyboard")
        assert held["handOrigin"] == dict(kind="stores", index=slot) and held["stores"][slot] == 0
        if args.awake_clock:
            assert held["awakeClock"]["minutePause"] == 60
            time.sleep(1.2)
            paused = capture("03a-drag-clock-paused")
            assert paused["timeMinutes"] == held["timeMinutes"]
            check_clock_resume(held, paused, "drag-held")
        shot("03-drag")
        reopen("04-drag-single-reopened")
        click(260, 160, 3)
        cancelled = capture("05-cancel-right")
        assert cancelled["stores"] == stores["stores"] and cancelled["handItem"] == 0
        sx, sy = 80+1+51+22*(slot%5), 1+24+17*(slot//5)
        click(sx, sy)
        other = next(i for i, v in enumerate(stores["stores"]) if i != slot and v != 0)
        click(80+1+51+22*(other%5), 1+24+17*(other//5))
        refused = capture("06-occupied-refusal")
        assert refused["stores"] == stores["stores"] and refused["handItem"] == 0
        click(sx, sy)
        click(108, 78)  # make_man pocket3 at (27,77), within native window (80,0).
        carried = capture("07-carry3-mouse")
        assert carried["parts"] == [0, 0, 0, stores["stores"][slot]] and carried["handItem"] == 0
        assert carried["stores"][slot] == 0 and carried["nourishment"] == initial["nourishment"]
        shot("07-stores")
        for value in ["Escape", "h", "Left", "h"]:
            key(value)
        body = capture("08-body-drag")
        assert body["screen"] == "character-status" and body["parts"] == [0, 0, 0, 0]
        assert body["handOrigin"] == dict(kind="pocket", index=3)
        reopen("09-body-library-reopened", library=True)
        key("e")
        eaten = capture("10-eaten")
        assert eaten["handItem"] == 0 and eaten["handOrigin"] == dict(kind="", index=0)
        assert eaten["nourishment"] == min(255, body["nourishment"]+70)
        for k in ["vigor", "rest", "health", "timeMinutes", "stores", "parts", "holds", "warehouses"]:
            if args.awake_clock and k == "timeMinutes":
                assert body[k] <= eaten[k] < body[k]+10
            elif args.awake_clock and k in ("vigor", "rest", "health") and eaten["timeMinutes"]//60 != body["timeMinutes"]//60:
                continue
            else:
                assert eaten[k] == body[k], k
        shot("10-eaten")
        final = reopen("11-eaten-library-reopened", library=True)
        sleep_reentries = []
        sleep_base = final["timeMinutes"]
        if args.sleep:
            def check_sleep(s):
                b = s["sleep"]
                assert b["active"] and b["shipActive"] and not b["forced"]
                assert b["shipCycles"] == (b["elapsedMinutes"]*60+b["minuteCycles"]) % 32
                assert s["timeMinutes"] == sleep_base+b["elapsedMinutes"]
                assert s["stores"] == final["stores"] and s["parts"] == final["parts"]
                assert s["handItem"] == final["handItem"]

            def check_resumed(before, after, path):
                check_sleep(after)
                b, a = before["sleep"], after["sleep"]
                delta = (a["elapsedMinutes"]-b["elapsedMinutes"])*60+a["minuteCycles"]-b["minuteCycles"]
                assert 0 <= delta < 180, "重開重置或跳過睡眠"
                sleep_reentries.append(dict(path=path, before=b, after=a, advancedCycles=delta,
                    scope="正常UI讀檔後立即續行，所以比較計數連續性；逐欄位完全一致由保存fixture另驗"))

            key("r")
            time.sleep(2)
            active = capture("12-sleep-active")
            sleep_base = active["timeMinutes"]-active["sleep"]["elapsedMinutes"]
            assert final["timeMinutes"] <= sleep_base < final["timeMinutes"]+10
            check_sleep(active)
            if args.awake_clock:
                assert active["awakeClock"]["minutePause"] == 60 and active["awakeClock"]["shipPause"] == 32
            shot("12-sleep-active")
            # Closing a reading/settings card must consume its Escape input.
            for value in ["F1", "Escape", "F2", "Escape"]:
                key(value)
            key("F10")
            process.wait(timeout=15)
            before = json.loads((out / "save.json").read_text())["state"]
            assert before["sleep"]["active"]
            start(load=True)
            resumed = capture("13-sleep-single-reopened")
            check_resumed(before, resumed, "single")
            before = resumed
            for value in ["F5", "1", "l"]:
                key(value)
            resumed = capture("14-sleep-library-reopened")
            check_resumed(before, resumed, "library")
            # A real sleeping new character crosses the hourly condition gate.
            # No state/coordinate/condition injection or -start shortcut.
            deadline = time.monotonic()+180
            attempt = 0
            while True:
                time.sleep(5)
                attempt += 1
                hour = capture(f"15-sleep-clock-{attempt}")
                check_sleep(hour)
                if hour["timeMinutes"] >= (active["timeMinutes"]//60+1)*60:
                    break
                if time.monotonic() >= deadline:
                    raise RuntimeError("有界等候內未跨遊戲時鐘整點")
            assert hour["rest"] == 255 and hour["nourishment"] == active["nourishment"]-3 and hour["vigor"] == active["vigor"]-2
            key("space")
            time.sleep(.7)
            awake = capture("16-awake")
            assert not awake["sleep"]["active"] and awake["screen"] == "character-status"
            assert awake["sleep"] == dict(active=False, forced=False, wakeRequested=False,
                minuteCycles=0, shipCycles=0, elapsedMinutes=0, shipActive=False)
            assert hour["timeMinutes"] <= awake["timeMinutes"] < hour["timeMinutes"]+3
            shot("16-awake")
        if args.awake_clock:
            for value in ["Escape", "g", "space"]:
                key(value)
            ground = capture("17-ground-entry")
            assert ground["screen"] == "ground" and not ground["awakeClock"]["shipActive"]
            anchor = ground["groundView"]["anchor"]
            run(["xdotool", "mousemove", "--window", window, str(anchor["x"]*3), str(anchor["y"]*3)])
            run(["xdotool", "mousedown", "1"])
            time.sleep(1.2)
            run(["xdotool", "mouseup", "1"])
            walked = capture("18-ground-clock")
            assert walked["screen"] == "ground"
            delta = walked["awakeClock"]["source"]["ticks"]-ground["awakeClock"]["source"]["ticks"]
            _, minutes, _ = advance_clock(ground["awakeClock"], delta)
            assert walked["timeMinutes"] > ground["timeMinutes"]+minutes, "正常移動指令沒有累計地面分鐘"
            shot("18-ground-clock")
        receipt = dict(status="PASS", saveSchema=schema, checks=checks, inputs=inputs, reentries=reentries,
                       scope="正常新局、Stores鍵盤取物、拖動中單檔重開／右鍵取消、滑鼠占用拒絕／攜帶槽3放物、角色選槽取物、保存庫重開及食用；初值營養255所以GUI食用夾至255。低讀數由原生700矩陣另驗。",
                       originalGUI="unverified", sleep="正常新局角色面板可中止睡眠、單檔及保存庫續行、整點及醒來" if args.sleep else "not-tested",
                       sleepReentries=sleep_reentries, sleepForcedGUI="not-tested", sleepWallClock="presentation-approximation",
                       awakeClock="正常新局、idle分鐘、拖動停鐘、等待中兩保存相位續行及ST地面移動" if args.awake_clock else "not-tested",
                       clockReentries=clock_reentries,
                       randomness="正常新局自然初始化；物品操作與船上睡眠無失竊RND，不挑seed",
                       toolSHA256=sha(Path(__file__)), inputSHA256={name: sha(getattr(args, name)) for name in ["binary", "disk", "settings_input"]})
        (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+"\n")
        print(f"PASS {len(checks)}檢查點／{len(inputs)}輸入／{len(reentries)}次{len(fields)}欄位新程序重開", flush=True)
    except Exception as error:
        (out / "failure.json").write_text(json.dumps(dict(status="FAIL", error=str(error), checks=checks, inputs=inputs,
            toolSHA256=sha(Path(__file__))), ensure_ascii=False, indent=2)+"\n")
        raise
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
        display.terminate()
        display.wait(timeout=5)


if __name__ == "__main__":
    main()
