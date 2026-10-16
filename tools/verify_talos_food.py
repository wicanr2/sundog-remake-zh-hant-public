"""Docker 內重生 #16 漢堡食用原生收據；入口見 banville-restoration-spec.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_platform_clock(native):
    assert len(native["Rows"])==36
    events=0
    for row in native["Rows"]:
        after=(row["Before"]+row["VBLs"])%2**32
        expected_events=row["VBLs"]//4 if row["Enabled"] else 0
        countdown=(4-row["VBLs"]%4) if row["Enabled"] else 65535
        assert row["After"]==after and row["Events"]==expected_events and row["Countdown"]==countdown,row
        assert row["TimeBefore"]==list(divmod(row["Before"],65536)),row
        assert row["TimeAfter"]==list(divmod(after,65536)),row
        events+=expected_events
    return dict(cases=36,eventCallbacks=events,modelDifferences=0,
                eventVBLRatio=4,timeBasis="native-VBL-counter; no-Hz-or-wall-clock")

def verify_scene_clock_pause(native):
    assert len(native["Rows"]) == 72
    missing = relocated = 0
    for row in native["Rows"]:
        slots = [list(v) for v in row["Before"]]
        target = row["PausedCallback"]
        found = next((n for n,v in enumerate(slots) if v[0]==target),None)
        saved = -1 if found is None else slots[found][2]*4
        assert row["SavedPeriod"]==saved,row
        if found is not None: slots[found]=[0,0,0]
        else: missing+=1
        assert row["Paused"]==slots,row
        if saved>0:
            destination=next(n for n,v in enumerate(slots) if v[0]==0)
            slots[destination]=[target,0,max(1,(saved+2)//4)]
            relocated+=destination!=found
        assert row["After"]==slots,row
    return dict(cases=72,missingCallbackCases=missing,relocatedCallbackCases=relocated,
                modelDifferences=0,timeBasis="no-time-or-callback-in-pause-slices")

def verify_ground_clock(native):
    assert native["Endpoint"] == [65535,65535]
    assert (len(native["Wide"]),len(native["Deadlines"]),len(native["Ground"])) == (625,24,288)
    for row in native["Wide"]:
        left,right = (v[0]*65536+v[1] for v in (row["Left"],row["Right"]))
        high_delta=(row["Right"][0]-row["Left"][0]+32768)%65536-32768
        assert row["Less"] == int(left < right),row
        assert row["Gate"] == int(left < right and high_delta < 3),row
    for row in native["Deadlines"]:
        total=(row["Before"][0]*65536+row["Before"][1]+row["Delta"])%2**32
        assert row["After"] == list(divmod(total,65536)),row
        assert row["Trace"]["Reads"] == 1,row
    forced=deaths=applied=0
    for row in native["Ground"]:
        eligible=row["Record5"]==1 and row["NowDelta"]>0
        cost=4 if row["Foot"] else 1
        counter=row["CounterBefore"]+int(eligible)
        minutes=cost if eligible and counter>=10 else 0
        if minutes: counter=0
        assert row["Cost"]==cost and row["CounterAfter"]==counter and row["Applied"]==minutes,row
        assert row["DeadlineAfter"]==[0,101+row["NowDelta"] if eligible else 100],row
        assert row["Trace"]["Reads"]==(2 if eligible else 1),row
        assert (row["HourAfter"],row["MinuteAfter"])==divmod(59+minutes,60),row
        vigor,rest,health,nourishment=row["Before"]
        scene,interrupt=row["SceneBefore"],0
        if minutes:
            rest=max(0,rest-[4,2,3,4][rest//64])
            if rest==0:
                scene=scene & 0xfff0 | (scene>>4 & 15)
                scene=scene & 0xff0f | (scene>>4 & 0xf0)
                scene=scene & 0xf0ff | 10<<8
                interrupt=1
            boost=max(0,int((vigor-128)/32))
            health=min(255,max(0,health-int(health<64)+(boost*4 if health>128 else 0)))
            nourishment=max(0,nourishment-[1,1,2,3][nourishment//64])
            penalty=sum(v//16-4 for v in (rest,health,nourishment) if v<64)
            if penalty<0: vigor+=penalty
            else:
                target=(rest+health+nourishment)//3
                vigor+=2*(target>vigor)-2*(target<vigor)
            vigor=min(255,max(0,vigor))
            if vigor==0:
                scene=scene & 0xf0ff | 12<<8
                interrupt=1
        assert row["After"]==[vigor,rest,health,nourishment],row
        assert row["SceneAfter"]==scene and row["Interrupt"]==interrupt,row
        applied+=bool(minutes);forced+=bool(interrupt and vigor);deaths+=bool(interrupt and not vigor)
    return dict(wideCases=625,deadlineCases=24,groundCases=288,totalCases=937,
                appliedMinuteCases=applied,forcedInterruptCases=forced,deathInterruptCases=deaths,
                modelDifferences=0,timeBasis="controlled-kernel-two-word-time")


def verify_awake_background(native):
    """Independent event-pulse/countdown model; no wall-clock conversion."""
    assert len(native["Rows"]) == 241
    forced = deaths = 0
    for row in native["Rows"]:
        slots = [list(v) for v in row["SlotsBefore"]]
        events, pulses = [], 0
        for iteration in range(1, row["Passes"] + 1):
            delay = min([8] + [v[1] for v in slots if v[0] > 0])
            for v in slots:
                if v[0] > 0:
                    v[1] -= delay
            if row["Phase"] == -1 and iteration == 1:
                slots[0] = [1, 0, 15]
                if row["Location"] == 2:
                    slots[1] = [2, 0, 8]
            pulses += delay if delay else 65536
            for v in slots:
                if v[0] > 0 and v[1] == 0:
                    v[1] = v[2]
                    events.append([iteration, pulses, v[0]])
        trace = row["Trace"]
        assert (trace["Callbacks"] or []) == events, row
        assert trace["Pulses"] == pulses and trace["Passes"] == row["Passes"], row
        assert trace["Copies"] == 3*row["Passes"] and trace["LockWaits"] == row["Passes"], row
        assert trace["Signals"] == row["Passes"] + 1, row
        assert row["SlotsAfter"] == slots and row["SystemAfter"] == row["SystemBefore"], row
        minutes = sum(v[2] == 1 for v in events)
        assert row["Minutes"] == minutes and row["ShipUpdates"] == sum(v[2] == 2 for v in events), row
        assert (row["HourAfter"], row["MinuteAfter"]) == divmod(59 + minutes, 60), row
        vigor, rest, health, nourishment = row["Before"]
        scene, interrupt = row["SceneBefore"], 0
        if minutes:
            assert minutes < 60
            rest = max(0, rest - [4, 2, 3, 4][rest // 64])
            if rest == 0:
                scene = scene & 0xfff0 | (scene >> 4 & 15)
                scene = scene & 0xff0f | (scene >> 4 & 0xf0)
                scene = scene & 0xf0ff | 10 << 8
                interrupt = 1
            boost = max(0, int((vigor - 128) / 32))
            health = min(255, max(0, health - int(health < 64) + (boost*4 if health > 128 else 0)))
            nourishment = max(0, nourishment - [1, 1, 2, 3][nourishment // 64])
            penalty = sum(v // 16 - 4 for v in (rest, health, nourishment) if v < 64)
            if penalty < 0:
                vigor += penalty
            else:
                target = (rest + health + nourishment) // 3
                vigor += 2 * (target > vigor) - 2 * (target < vigor)
            vigor = min(255, max(0, vigor))
            if vigor == 0:
                scene = scene & 0xf0ff | 12 << 8
                interrupt = 1
        assert row["After"] == [vigor, rest, health, nourishment], row
        assert row["SceneAfter"] == scene and row["Interrupt"] == interrupt, row
        forced += bool(interrupt and vigor)
        deaths += bool(interrupt and not vigor)
    return dict(cases=241, controlledInterleavedStartupCases=60, positiveCountdownCases=180,
                artificialZeroDelayCases=1, modelDifferences=0, forcedInterruptCases=forced,
                deathInterruptCases=deaths, timeBasis="controlled-event-semaphore-pulses")


def verify_sleep_interrupt(native):
    """獨立時鐘／場景模型；不替換原版父流程 writer。"""
    assert len(native["Rows"]) == 48
    asleep = deaths = 0
    for row in native["Rows"]:
        vigor, rest, health, nourishment = row["Before"]
        scene = row["SceneBefore"]
        interrupt = 0
        total = 59 + row["Delta"]
        for _ in range(total // 60):
            rest = max(0, rest - [4, 2, 3, 4][rest // 64])
            if rest == 0:
                if scene >> 8 & 15 < 7:
                    scene = scene & 0xfff0 | (scene >> 4 & 15)
                    scene = scene & 0xff0f | (scene >> 4 & 0xf0)
                scene = scene & 0xf0ff | 10 << 8
                interrupt = 1
            boost = max(0, int((vigor - 128) / 32))
            health = min(255, max(0, health - int(health < 64) + (boost*4 if health > 128 else 0)))
            nourishment = max(0, nourishment - [1, 1, 2, 3][nourishment // 64])
            penalty = sum(v // 16 - 4 for v in (rest, health, nourishment) if v < 64)
            if penalty < 0:
                vigor += penalty
            else:
                target = (rest + health + nourishment) // 3
                vigor += 2 * (target > vigor) - 2 * (target < vigor)
            vigor = min(255, max(0, vigor))
            if vigor == 0:
                scene = scene & 0xf0ff | 12 << 8
                interrupt = 1
        assert row["AfterClock"] == [vigor, rest, health, nourishment], row
        assert row["SceneAfterClock"] == scene and row["Interrupt"] == interrupt, row
        assert (row["HourAfterClock"], row["MinuteAfterClock"]) == divmod(total, 60), row
        minutes = 0
        if scene >> 8 & 15 == 10:
            assert vigor > 0 and rest == 0
            minutes = 51
            rest = 51
            restored = scene >> 4 & 15
            scene = scene & 0xf0ff | restored << 8
            scene = scene & 0xff0f | (scene & 15 if restored >= 5 else 7) << 4
            scene = scene & 0xfff0 | 11
            asleep += 1
            assert row["Trace"]["Cycles"] == 3060 and row["Trace"]["Minutes"] == 51, row
            assert row["Trace"].get("ShipUpdates", 0) == (95 if row["Location"] == 2 else 0), row
            assert row["GroundLoopExited"] == int(row["Location"] != 2), row
        elif vigor == 0:
            deaths += 1
        assert row["AfterSleep"] == [vigor, rest, health, nourishment], row
        assert row["SceneAfterSleep"] == scene, row
        assert (row["HourAfterSleep"], row["MinuteAfterSleep"]) == divmod(total + minutes, 60), row
        assert row["ShipSlotBefore"] == row["ShipSlotAfter"] == (2 if row["Location"] == 2 else 0), row
    return dict(cases=48, forcedSleepCases=asleep, deathCases=deaths, modelDifferences=0)


def export_chain_sources(repo, disk, output, selected=None):
    """結構匯出只供RE導覽；footer保留原bytes，不將其解成指令。"""
    from psys_disasm import decode_one, format_one, segment_info
    raw = disk.read_bytes()
    segments = []
    selected = selected or [(0x31200, "WINDOWLI"), (0x34800, "XDOUSERM"), (0x3d800, "XDOINTER"),
                       (0x2e800, "MAINLIB"), (0x36600, "XDOREPAI"), (0x37200, "XSHOWMOV"),
                       (0x37e00, "XMOVEONS")]
    for base, name in selected:
        info = segment_info(raw, base)
        assert info["name"] == name
        data = raw[base:]
        size = int.from_bytes(data[:2], "big") * 2
        count = int.from_bytes(data[size:size+2], "big")
        assert 1 <= count <= 128
        entries = {n: int.from_bytes(data[size-2*n:size-2*n+2], "big") * 2 for n in range(1, count+1)}
        assert all(0x18 <= entry < size - 2*count for entry in entries.values())
        if name == "WINDOWLI":
            assert entries[8] == 0x2ca
        if name == "XDOUSERM":
            assert entries[16] == 0x5ae
        if name == "MAINLIB":
            assert entries[0x35] == 0xbf6 and entries[0x52] == 0xb60
        limits = sorted(set(entries.values())) + [size - 2*count]
        procedures = []
        for number, entry in entries.items():
            end = next(v for v in limits if v > entry)
            locals_word = int.from_bytes(data[entry:entry+2], "big")
            if locals_word == 0xffff:
                # 0xffff標頭與MC68000 bytes：原生程序強推論，禁止當65535個locals解p-code。
                heading = f"{name}:{number:02x} rawBase{base:x} header{entry:04x} localsffff native-body-unknown nextHeader{end:04x}"
                (output / f"{name}-{number:02x}.txt").write_text(heading + "\n")
                procedures.append(dict(number=number, header=hex(entry), nextHeader=hex(end),
                    nativeMarker="0xffff", SHA256=hashlib.sha256(data[entry:end]).hexdigest(),
                    instructions=[], semantics="strong inference 原生程序標記；body語意unknown，不作p-code解碼"))
                continue
            pos, lines, instructions = entry+2, [], []
            while pos < end:
                instruction = decode_one(data, pos)
                assert pos + instruction["length"] <= end, (name, number, hex(pos))
                lines.append(format_one(instruction, data, pos))
                instructions.append(instruction)
                pos += instruction["length"]
                if instruction["name"] == "rpu":
                    break
            assert instructions and instructions[-1]["name"] == "rpu", (name, number)
            heading = f"{name}:{number:02x} rawBase{base:x} header{entry:04x} locals{locals_word} codeEndExclusive{pos:04x}"
            (output / f"{name}-{number:02x}.txt").write_text(heading + "\n" + "\n".join(lines) + "\n")
            procedures.append(dict(number=number, header=hex(entry), codeEndExclusive=hex(pos),
                nextHeader=hex(end), footerHex=data[pos:end].hex(),
                SHA256=hashlib.sha256(data[entry:end]).hexdigest(), instructions=instructions,
                semantics="unknown；原始數字只作導覽，不由名稱猜補正式規則"))
        segments.append(dict(name=name, rawBase=hex(base), size=hex(size), info=info, procedures=procedures))
    result = dict(status="DRAFT-source-structure-only", input=str(disk), diskSHA256=sha(disk),
        decoderSHA256=sha(repo / "tools/psys_disasm.py"), toolSHA256=sha(Path(__file__).resolve()),
        addressSpace="sundog.st segment-relative byte offset", segments=segments,
        boundary="每程序只解至首個RPU；後續footer原樣保留。未做CFG或原生執行，不視為語意證實。")
    (output / "source-index.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(f"DRAFT：{len(segments)}個相關段落的原始程序結構已匯出，語意待原生追查。")


def verify_sleep_research(wake, theft):
    """獨立研究模型；只審查原生答案，不進正式規則。"""
    assert len(wake["Rows"]) == 63 and len(theft["Rows"]) == 360
    for row in wake["Rows"]:
        vigor, rest, health, nourishment = row["Before"]
        local_rest, mode = rest, row["Mode"]
        total = row["Day"] * 1440 + row["Hour"] * 60 + row["Minute"]
        attrs = row["AttributesBefore"][:]
        minutes, g3, g18, redraws, dirty = 0, 0, 7, 0, False
        while True:
            g3 = int(mode == 12)  # 真正helper04於時鐘前檢查mode12。
            if dirty:
                redraws += 1
                dirty = False
            local_rest += 1
            total += 1
            minutes += 1
            if total % 60 == 0:
                boost = max(0, int((vigor - 128) / 32))
                health -= int(health < 64)
                if health > 128:
                    health += boost * 4
                nourishment -= [1, 1, 2, 3][nourishment // 64]
                rest, health, nourishment = [min(255, max(0, v)) for v in (rest, health, nourishment)]
                if rest == 0:
                    mode, g3 = 10, 1
                penalty = sum(v // 16 - 4 for v in (rest, health, nourishment) if v < 64)
                if penalty < 0:
                    vigor += penalty
                else:
                    target = (rest + health + nourishment) // 3
                    vigor += 2 * (target > vigor) - 2 * (target < vigor)
                vigor = min(255, max(0, vigor))
                if vigor == 0:
                    mode, g3, g18 = 12, 1, 0
                for n in range(5):
                    difference = attrs[n] - attrs[5 + n]
                    step = -(difference > 0) + (difference < 0)
                    change = step * attrs[10 + n]
                    if abs(change) > abs(difference):
                        change = -difference
                    attrs[n] += change
                dirty = True
            local_rest = min(255, local_rest)
            rest = local_rest  # 時鐘讀取的是前一分鐘Rest；原始writer在35返回後。
            if mode == 10 and rest > 50:
                mode = 7
            if mode == 12 or minutes >= row["Trace"]["WakePoll"] and mode != 10:
                break
            assert minutes < 4096, "研究模型超過有界睡眠"
        if dirty:
            redraws += 1
        if mode == 12:
            g3 = 1
        observed = (row["After"], row["MinuteAfter"], row["HourAfter"], row["DayAfter"],
                    row["ModeAfter"], row["G3"], row["G18"], row["Dead"], row["AttributesAfter"],
                    row["Trace"]["Minutes"], row["Trace"]["StatusDraws"])
        expected = ([vigor, rest, health, nourishment], total % 60, total // 60 % 24, total // 1440,
                    mode, g3, g18, int(mode == 12), attrs, minutes, redraws)
        assert observed == expected, (row, expected)

    for row in theft["Rows"]:
        seed = row["SeedBefore"][:]
        rng = row["Trace"]["Random"] or []
        for actual in rng:
            seed[1] = ((seed[1] + seed[0]) * 11 - 1) & 65535
            seed[0] = (0x7ab9 * seed[0] + 11 * seed[1]) & 65535
            signed = seed[0] if seed[0] < 32768 else seed[0] - 65536
            assert actual == abs(int(signed / 4)), (row, actual, seed)
        assert seed == row["SeedAfter"], row
        offset, cash, carry, elapsed = 0, row["CashBefore"], list(row["CarryBefore"]), row["Minutes"]

        def next_random():
            nonlocal offset
            value = rng[offset]
            offset += 1
            return value

        if row["Bit13"] and not row["Flag57"] and row["LocationMode"] in (3, 5):
            while row["Risk"] < elapsed:
                if next_random() % 9 * row["Risk"] < elapsed:
                    # 原始LAND是bitwise AND；cash != 0為1，僅RND的低位保留。
                    if next_random() & 1 and cash != 0:
                        cash = 0
                    else:
                        first = next_random() % 4
                        for delta in range(4):
                            slot = (first + delta) % 4
                            if carry[slot]:
                                carry[slot] = 0
                                break
                elapsed -= 10
        assert (offset, cash, carry) == (len(rng), row["CashAfter"], row["CarryAfter"]), row
    return {"wakeCases": 63, "theftCases": 360, "modelDifferences": 0,
            "randomCalls": sum(len(r["Trace"]["Random"] or []) for r in theft["Rows"]),
            "cashLossCases": sum(r["CashAfter"] != r["CashBefore"] for r in theft["Rows"]),
            "itemLossCases": sum(r["CarryAfter"] != r["CarryBefore"] for r in theft["Rows"])}


def verify_sleep_parent(ship, menu):
    """獨立研究模型；只審查原始父鏈，不進runtime。"""
    assert len(ship["Rows"]) == 90 and len(menu["Rows"]) == 24
    for row in ship["Rows"]:
        cycle = minutes = updates = 0
        mode, rest = row["Mode"], row["Rest"]
        while True:
            cycle += 1
            if cycle % 60 == 0:
                minutes += 1
                rest = min(255, rest + 1)
            if cycle % 32 == 0:
                updates += 1
            if mode == 10 and rest > 50:
                mode = 7
            if cycle % 60 == 0 or cycle % 32 == 0:
                if cycle >= row["WakeCycle"] and mode != 10:
                    break
            assert cycle < 8192
        assert (row["Trace"]["Cycles"], row["Trace"]["Minutes"], row["Trace"]["ShipUpdates"],
                row["RestAfter"], row["ModeAfter"]) == (cycle, minutes, updates, rest, mode), row
        fuel, weapon, shield, setting = row["EnergyBefore"]
        guns = True
        for tick in range(1, updates + 1):
            phase, old_fuel = tick % 3, fuel
            if phase & 1 == 0:
                if fuel > 0 and guns:
                    delta = min(32, 255 - weapon)
                    if delta > 0:
                        fuel = max(0, fuel - 1)
                        weapon += delta
                elif weapon > 0:
                    weapon = max(0, weapon - 8)
            if phase == 0:
                if shield != setting:
                    fuel = max(0, fuel)
                    shield = max(0, min(255, shield + 64, setting))
                    if shield < setting:
                        fuel -= (setting + 1) // 64
                elif shield > 0:
                    fuel -= 1
            fuel = max(0, fuel)
            if fuel == 0 and old_fuel > 0:
                guns, setting = False, 0
        assert row["EnergyAfter"] == [fuel, weapon, shield, setting], row
    for row in menu["Rows"]:
        minutes = 51 if row["Mode"] == 10 else 1
        assert row["Trace"]["Minutes"] == minutes, row
        assert row["RestAfter"] == min(255, row["Rest"] + minutes), row
        assert row["NourishmentAfter"] == row["Nourishment"], row
        assert row["SceneAfter"] == 0x5000 | row["Location"] << 8 | 7 << 4 | 11, row
    return dict(shipCases=90, menuCases=24, modelDifferences=0)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--disk", type=Path, required=True)
    p.add_argument("--talos", type=Path, required=True)
    p.add_argument("--output", type=Path, required=True)
    p.add_argument("--compare-remake", action="store_true", help="比較原版與正式規則；正常GUI及睡眠另驗")
    p.add_argument("--require-condition-core", action="store_true", help="要求四項讀數與未跨整點正式差異為零；進食仍另列")
    p.add_argument("--source-chain-only", action="store_true", help="只匯出物品與睡眠相關原始程序結構，未驗語意")
    p.add_argument("--sleep-parent-source-only", action="store_true", help="只匯出睡眠caller／背景槽必要原始模組結構")
    p.add_argument("--stores-chain-only", action="store_true", help="只執行原版Stores取物／頭部食用／取消還原鏈")
    p.add_argument("--sleep-chain-only", action="store_true", help="研究完整睡眠、醒來、父時鐘及固定種子失竊；不比較正式規則")
    p.add_argument("--sleep-parent-only", action="store_true", help="研究正常睡眠父入口及idle船艦callback，並回歸既有423案")
    p.add_argument("--attribute-clock-only", action="store_true", help="驗原版恢復速率初始化與屬性整點鏈")
    p.add_argument("--sleep-interrupt-only", action="store_true", help="驗原始整點中斷到船上／地面強制睡眠父流程")
    p.add_argument("--awake-background-only", action="store_true", help="研究SUNDOG:02完整清醒背景分派與受控event脈衝")
    p.add_argument("--ground-clock-only", action="store_true", help="研究原始地面計時閘門及十次更新附加分鐘")
    p.add_argument("--scene-clock-pause-only", action="store_true", help="研究原始拖曳物品與航行場景的背景槽停止／恢復")
    p.add_argument("--platform-clock-only", action="store_true", help="驗原始PRG VBL與BIOS TIME的數值比例")
    p.add_argument("--awake-clock-core-only", action="store_true", help="重生四批原始時鐘收據並比較typed核心；GUI與保存接線另驗")
    p.add_argument("--awake-clock-integration-only", action="store_true", help="重生四批時鐘及睡眠回歸，另比較正式分鐘／地面consumer；GUI另驗")
    args = p.parse_args()
    if args.awake_clock_core_only or args.awake_clock_integration_only:
        if args.awake_clock_core_only and args.awake_clock_integration_only:
            p.error("核心與整合旗標不得混用")
        if any((args.source_chain_only,args.sleep_parent_source_only,args.stores_chain_only,
                args.sleep_chain_only,args.sleep_parent_only,args.attribute_clock_only,
                args.require_condition_core,args.sleep_interrupt_only,args.awake_background_only,
                args.ground_clock_only,args.scene_clock_pause_only,args.platform_clock_only,args.compare_remake)):
            p.error("typed時鐘核心必須獨立執行")
        if not Path("/.dockerenv").is_file():
            p.error("只在Docker執行")
        repo=Path(__file__).resolve().parents[1]
        output=args.output.resolve()
        if output.exists() or output.parent.stat().st_uid != os.getuid():
            p.error("輸出須為目前使用者所屬的全新子目錄")
        output.mkdir()
        inputs=[("platform","--platform-clock-only"),("background","--awake-background-only"),
                ("pause","--scene-clock-pause-only"),("ground","--ground-clock-only")]
        reports={}
        for name,flag in inputs:
            run_logged(["python3",str(Path(__file__).resolve()),"--disk",str(args.disk),
                        "--talos",str(args.talos),"--output",str(output/name),flag],
                       output/(name+"-run.log"),cwd=repo,env=os.environ.copy())
            reports[name]=json.loads((output/name/"receipt.json").read_text())
        env=dict(os.environ,SUNDOG_AWAKE_CLOCK_NATIVE=str(output),GOPROXY="off",GOTOOLCHAIN="local")
        command=["go","test","./internal/game","-run","^TestExportTalosAwakeClockCore$","-count=1","-v"]
        run_logged(command,output/"remake.log",cwd=repo,env=env)
        formal=json.loads((output/"remake-awake-clock-core.json").read_text())
        assert [formal[k] for k in ["PlatformCases","BackgroundCases","PauseCases","GroundCases"]]==[36,241,72,937]
        assert formal["Differences"]==0
        source_names=["tools/verify_talos_food.py","tools/talos_food_test.go.txt",
                      "internal/game/awake_clock.go","internal/game/awake_clock_test.go",
                      "internal/game/awake_clock_native_parity_test.go"]
        receipt=dict(status="PASS-awake-clock-core-same-state",specState="READY-awake-clock-core",
                     scope="typed core only; State/save/new-game/GUI integration pending",formal=formal,
                     nativeReports=reports,command=command,
                     sources={name:sha(repo/name) for name in source_names})
        if args.awake_clock_integration_only:
            from psys_disasm import decode_one, format_one
            raw = args.disk.read_bytes()
            lifecycle = []
            for name,base,start,end in [("SUNDOG:01 lock/event/worker START",0x2e200,0x2d5,0x316),
                    ("SUNDOG:03 ship register",0x2e200,0x1b4,0x1b8),
                    ("SUNDOG:03 ship stop",0x2e200,0x1ee,0x1f1),
                    ("XMOVEONG:28 ground reset",0x40000,0x209f,0x20b3),
                    ("SHIPLIB:12 idle phase/movement/energy",0x31e00,0x128b,0x129a)]:
                data, pos, lines = raw[base:], start, []
                while pos < end:
                    instruction=decode_one(data,pos)
                    lines.append(format_one(instruction,data,pos))
                    pos += instruction["length"]
                assert pos == end, (name,hex(pos))
                lifecycle.append(dict(name=name,rawBase=hex(base),start=hex(start),endExclusive=hex(end),
                    addressSpace="disk raw base plus segment-relative byte offset",grade="proven-source-bytes; caller scope in READY",
                    SHA256=hashlib.sha256(data[start:end]).hexdigest(),hex=data[start:end].hex(),instructions=lines))
            integration_command=["go","test","./internal/game","-run","^TestExportTalosAwakeClockIntegration$","-count=1","-v"]
            run_logged(integration_command,output/"integration.log",cwd=repo,env=env)
            integration=json.loads((output/"remake-awake-clock-integration.json").read_text())
            assert integration["BackgroundCases"]==240 and integration["GroundCases"]==216 and integration["Differences"]==0
            run_logged(["python3",str(Path(__file__).resolve()),"--disk",str(args.disk),"--talos",str(args.talos),
                        "--output",str(output/"sleep"),"--sleep-parent-only","--compare-remake"],
                       output/"sleep-run.log",cwd=repo,env=env)
            receipt.update(status="PASS-awake-clock-formal-consumers",specState="READY-awake-clock-integration",
                           scope="typed core1286 and formal worker/ground consumers456; original GUI and complete flight/combat pending",
                           integration=integration,integrationCommand=integration_command,
                           lifecycleEvidence=lifecycle,decoderSHA256=sha(repo/"tools/psys_disasm.py"),
                           startupContract=dict(grade="platform-contract-derived; not full Atari boot parity",
                               source="https://www.bitsavers.org/pdf/advancedComputerDesign/AOS_Programmers_Manual_Ver_1.0_Jun82.pdf",
                               section="3.0.0.3 printed page18",mainPriority=128,workerPriority=160,eventInitial=0,lockInitial=1),
                           sleepRegression=json.loads((output/"sleep/receipt.json").read_text()))
            receipt["sources"].update({str(v.relative_to(repo)):sha(v) for v in sorted((repo/"internal/game").glob("*.go"))})
            receipt["resolutionBacklinks"] = []
            for name in ["docs/atari-st/ground-rules.md","docs/atari-st/ship-rules.md","docs/atari-st/banville-restoration-spec.md"]:
                text = (repo/name).read_text()
                for marker in ["awake-clock-integration-20261004","raw0x2e200","raw0x40000","209f..20b3"]:
                    assert marker in text, (name,marker)
                receipt["resolutionBacklinks"].append(dict(document=name,marker="awake-clock-integration-20261004",checked=True))
        (output/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
        print("typed時鐘核心1286案及正式consumer456案零差異；正常GUI另驗。" if args.awake_clock_integration_only else
              "typed時鐘核心1286案零差異；正常GUI及兩保存接線仍待。")
        return
    if args.platform_clock_only and any((args.source_chain_only,args.sleep_parent_source_only,
            args.stores_chain_only,args.sleep_chain_only,args.sleep_parent_only,args.attribute_clock_only,
            args.require_condition_core,args.sleep_interrupt_only,args.awake_background_only,
            args.ground_clock_only,args.scene_clock_pause_only,args.compare_remake)):
        p.error("平台時鐘研究須獨立執行；正式時序尚未READY")
    if args.scene_clock_pause_only and any((args.source_chain_only,args.sleep_parent_source_only,
            args.stores_chain_only,args.sleep_chain_only,args.sleep_parent_only,args.attribute_clock_only,
            args.require_condition_core,args.sleep_interrupt_only,args.awake_background_only,
            args.ground_clock_only,args.compare_remake)):
        p.error("場景停鐘研究須獨立執行；正式時序尚未READY")
    source_only = args.source_chain_only or args.sleep_parent_source_only
    if args.attribute_clock_only and any((source_only, args.stores_chain_only, args.sleep_chain_only,
                                         args.sleep_parent_only, args.require_condition_core)):
        p.error("屬性整點鏈須獨立執行")
    if source_only and (args.compare_remake or args.require_condition_core):
        p.error("結構匯出不能與正式對拍混用")
    if args.stores_chain_only and (source_only or args.require_condition_core):
        p.error("Stores原生鏈須獨立執行")
    if args.require_condition_core and not args.compare_remake:
        p.error("--require-condition-core須配合--compare-remake")
    if (args.sleep_chain_only or args.sleep_parent_only) and any((source_only, args.stores_chain_only,
                                     args.require_condition_core)):
        p.error("睡眠對拍須獨立執行")
    if args.sleep_chain_only and args.compare_remake:
        p.error("正式睡眠比較需使用--sleep-parent-only")
    if args.source_chain_only and args.sleep_parent_source_only:
        p.error("每次結構匯出只能選一個來源範圍")
    if args.sleep_chain_only and args.sleep_parent_only:
        p.error("每次睡眠研究只能選一個範圍")
    if args.sleep_interrupt_only and any((source_only, args.stores_chain_only, args.sleep_chain_only,
                                         args.sleep_parent_only, args.attribute_clock_only, args.require_condition_core)):
        p.error("自動睡眠中斷須獨立執行")
    if args.awake_background_only and any((source_only, args.stores_chain_only, args.sleep_chain_only,
            args.sleep_parent_only, args.attribute_clock_only, args.require_condition_core,
            args.sleep_interrupt_only, args.compare_remake)):
        p.error("清醒背景研究須獨立執行；正式時序尚未READY")
    if args.ground_clock_only and any((source_only,args.stores_chain_only,args.sleep_chain_only,
            args.sleep_parent_only,args.attribute_clock_only,args.require_condition_core,
            args.sleep_interrupt_only,args.awake_background_only,args.compare_remake)):
        p.error("地面計時研究須獨立執行；正式時序尚未READY")
    if not Path("/.dockerenv").is_file():
        p.error("此工具只在 Docker 執行")
    repo = Path(__file__).resolve().parents[1]
    game_schema = int((repo / "internal/game/state.go").read_text().split("const CurrentSchema = ", 1)[1].splitlines()[0])
    disk, talos = (v.resolve(strict=True) for v in (args.disk, args.talos))
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6" or sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        p.error("原始磁碟／直譯器身分不符")
    output = args.output.resolve()
    if output.exists() or output.parent.stat().st_uid != os.getuid():
        p.error("輸出須全新且父目錄擁有者正確")
    if source_only:
        output.mkdir(mode=0o700)
        selected = [(0x2e200, "SUNDOG"), (0x34800, "XDOUSERM"), (0x31e00, "SHIPLIB"),
                    (0x39c00, "XSTARTUP"), (0x2e800, "MAINLIB")] if args.sleep_parent_source_only else None
        export_chain_sources(repo, disk, output, selected)
        return
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        p.error("須沿用 Go1.25.12")
    names = ("combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent",
             "combat_display", "combat_levels", "combat_movement", "combat_fuel", "fuel_depot",
             "fuel_entry", "walker_slices", "banville", "banville_price", "banville_site",
             "trade_classes", "food")
    sources = [repo / f"tools/talos_{n}_test.go.txt" for n in names]
    assert all(v.is_file() and v.stat().st_uid == os.getuid() for v in sources)
    output.mkdir(mode=0o700)
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(talos / f"internal/m68k/sundog_{n}_test.go"): str(v)
        for n, v in zip(names, sources)}}))
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    if args.platform_clock_only:
        command=["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k",
                 "-run","^TestSunDogFoodPlatformClock$","-count=1","-v"]
        run_logged(command,output/"native.log",cwd=talos,env=env)
        native=json.loads((output/"native-food-platform-clock.json").read_text())
        model=verify_platform_clock(native)
        raw=disk.read_bytes();prg=raw[0x2400:0x2400+3976]
        report=dict(status="PASS-native-platform-clock",specState="DRAFT-awake-clock",
            formalRuntime=f"not-compared-schema{game_schema}",originalGUI="unverified",model=model,native=native,
            diskSHA256=sha(disk),interpreterSHA256=sha(interp),prgSHA256=hashlib.sha256(prg).hexdigest(),
            prgTextSHA256=hashlib.sha256(prg[28:28+0xf3c]).hexdigest(),go=version,command=command,
            talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
            addressSpace="SYSTEM.INTERP binary base0；SUNDOG.PRG text base0，file offset=text+0x1c、disk raw=0x2400+file offset",
            sources={str(v.relative_to(repo)):sha(v) for v in sources+[Path(__file__).resolve(),repo/"tools/psys_disasm.py"]})
        ranges=[("BIOS SYSSTAT",0x112,0x11a),("QUIET/ENABLE",0x11a,0x142),
                ("VBL prefix",0x154,0x17c),("bootstrap BIOS pointer",0x332,0x33a)]
        report["prgSlices"]=[dict(name=name,start=hex(start),endExclusive=hex(end),
            SHA256=hashlib.sha256(prg[28+start:28+end]).hexdigest()) for name,start,end in ranges]
        report["interpreterSlices"]=[dict(name=name,start=hex(start),endExclusive=hex(end),
            SHA256=hashlib.sha256(interp.read_bytes()[start:end]).hexdigest()) for name,start,end in
            [("KERNEL TIME",0x1832,0x1866),("BIOS clock service",0x2160,0x2188)]]
        vet=["go","vet","-overlay",str(overlay),"./internal/m68k"]
        run_logged(vet,output/"vet.log",cwd=talos,env=env);report["vetCommand"]=vet
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("原始平台時鐘36案，TIME每VBL加1、背景event每4 VBL；獨立模型零差異。")
        return
    if args.scene_clock_pause_only:
        command=["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k",
                 "-run","^TestSunDogFoodSceneClockPause$","-count=1","-v"]
        run_logged(command,output/"native.log",cwd=talos,env=env)
        native=json.loads((output/"native-food-scene-clock-pause.json").read_text())
        model=verify_scene_clock_pause(native)
        report=dict(status="PASS-native-scene-clock-pause",specState="DRAFT-awake-clock",
            formalRuntime=f"not-compared-schema{game_schema}",originalGUI="unverified",model=model,native=native,
            diskSHA256=sha(disk),interpreterSHA256=sha(interp),go=version,command=command,
            talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
            addressSpace="sundog.st raw offset；start/endExclusive為segment-relative byte offset",
            sources={str(v.relative_to(repo)):sha(v) for v in sources+[Path(__file__).resolve(),repo/"tools/psys_disasm.py"]})
        ranges=[("WINDOWLI:08 pause",0x31200,0x2cc,0x2d4),("WINDOWLI:08 resume",0x31200,0x36a,0x36e),
                ("XSHOWMOV:03 pause",0x37200,0x52,0x5b),("XSHOWMOV:06 resume",0x37200,0x1c4,0x1c8),
                ("XMOVEONS:06 pause",0x37e00,0x1ca,0x1d2),("XMOVEONS:06 resume",0x37e00,0x26d,0x271),
                ("MAINLIB:2f",0x2e800,0x9a6,0x9d5),("MAINLIB:30",0x2e800,0x9da,0xa3d),
                ("MAINLIB:31",0x2e800,0xa42,0xa74)]
        raw=disk.read_bytes()
        report["originalSlices"]=[dict(name=name,rawBase=hex(base),start=hex(start),endExclusive=hex(end),
             SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest()) for name,base,start,end in ranges]
        vet=["go","vet","-overlay",str(overlay),"./internal/m68k"]
        run_logged(vet,output/"vet.log",cwd=talos,env=env);report["vetCommand"]=vet
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("原始場景停止／恢復72案，獨立槽模型零差異；正式清醒時序仍待READY。")
        return
    if args.ground_clock_only:
        command=["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k",
                 "-run","^TestSunDogFoodGroundClockGate$","-count=1","-v"]
        run_logged(command,output/"native.log",cwd=talos,env=env)
        native=json.loads((output/"native-food-ground-clock.json").read_text())
        model=verify_ground_clock(native)
        report=dict(status="PASS-native-ground-clock",specState="DRAFT-ground-clock",
            formalRuntime=f"not-compared-schema{game_schema}",originalGUI="unverified",model=model,native=native,
            diskSHA256=sha(disk),interpreterSHA256=sha(interp),go=version,command=command,
            talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
            addressSpace="sundog.st raw offset；start/endExclusive為segment-relative byte offset",
            sources={str(v.relative_to(repo)):sha(v) for v in sources+[Path(__file__).resolve(),repo/"tools/psys_disasm.py"]})
        raw=disk.read_bytes()
        ranges=[("XSTARTUP:07 endpoint",0x39c00,0x3c3,0x3cf),
                ("MAINLIB:25 unsigned compare",0x2e800,0x830,0x898),
                ("MAINLIB:26 timestamp addition",0x2e800,0x89c,0x8cd),
                ("MAINLIB:27 time reset",0x2e800,0x8d2,0x8df),
                ("MAINLIB:28 time gate",0x2e800,0x8e4,0x8f9),
                ("XMOVEONG:23 cost writer",0x40000,0x1ab2,0x1ae2),
                ("XMOVEONG:14 update prefix",0x40000,0xa9e,0xaca),
                ("MAINLIB:35",0x2e800,0xbf8,0xd22)]
        report["originalSlices"]=[dict(name=name,rawBase=hex(base),start=hex(start),endExclusive=hex(end),
             SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest()) for name,base,start,end in ranges]
        vet=["go","vet","-overlay",str(overlay),"./internal/m68k"]
        run_logged(vet,output/"vet.log",cwd=talos,env=env);report["vetCommand"]=vet
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("原始地面計時937案，獨立雙字時間模型零差異；正式清醒時序仍待READY。")
        return
    if args.awake_background_only:
        command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                   "-run", "^TestSunDogFoodAwakeBackground$", "-count=1", "-v"]
        run_logged(command, output / "native.log", cwd=talos, env=env)
        native = json.loads((output / "native-food-awake-background.json").read_text())
        model = verify_awake_background(native)
        report = dict(status="PASS-native-awake-background", specState="DRAFT-awake-background",
            formalRuntime=f"not-compared-schema{game_schema}", originalGUI="unverified", model=model,
            native=native, diskSHA256=sha(disk), interpreterSHA256=sha(interp), go=version, command=command,
            talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
            addressSpace="sundog.st raw offset；start/endExclusive為segment-relative byte offset",
            sources={str(v.relative_to(repo)): sha(v) for v in sources + [Path(__file__).resolve(), repo / "tools/psys_disasm.py"]})
        raw = disk.read_bytes()
        ranges = [("SUNDOG:02 procedure bytes; bounded loop executed",0x2e200,0x1a,0x101),
                  ("SUNDOG:02 original XJP",0x2e200,0x344,0x358),
                  ("SUNDOG:01 minute init",0x2e200,0x311,0x316),
                  ("SUNDOG:03 ship init",0x2e200,0x1b4,0x1b8),
                  ("MAINLIB:30 countdown writer",0x2e800,0x9da,0xa3d),
                  ("MAINLIB:35",0x2e800,0xbf8,0xd22), ("SHIPLIB:12",0x31e00,0x1278,0x12c3)]
        report["originalSlices"] = [dict(name=name, rawBase=hex(base), start=hex(start),
            endExclusive=hex(end), SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest())
            for name, base, start, end in ranges]
        vet = ["go", "vet", "-overlay", str(overlay), "./internal/m68k"]
        run_logged(vet, output / "vet.log", cwd=talos, env=env)
        report["vetCommand"] = vet
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("原始清醒背景241案，獨立event脈衝模型零差異；正式時序與原版GUI仍待READY。")
        return
    if args.sleep_interrupt_only:
        command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                   "-run", "^TestSunDogFoodSleepInterruptParent$", "-count=1", "-v"]
        run_logged(command, output / "native.log", cwd=talos, env=env)
        native = json.loads((output / "native-food-sleep-interrupt-parent.json").read_text())
        model = verify_sleep_interrupt(native)
        report = dict(status="PASS-native-sleep-interrupt", specState="DRAFT-auto-sleep-interrupt",
            formalRuntime=f"not-compared-schema{game_schema}", originalGUI="unverified", model=model,
            native=native, diskSHA256=sha(disk), interpreterSHA256=sha(interp), go=version, command=command,
            talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
            addressSpace="sundog.st raw offset；start/endExclusive為segment-relative byte offset",
            sources={str(v.relative_to(repo)): sha(v) for v in sources + [Path(__file__).resolve(), repo / "tools/psys_disasm.py"]})
        raw = disk.read_bytes()
        report["originalSlices"] = [dict(name=name, rawBase=hex(base), start=hex(start),
            endExclusive=hex(end), SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest())
            for name, base, start, end in [("MAINLIB:35",0x2e800,0xbf8,0xd22),
                ("MAINLIB:52",0x2e800,0xb62,0xbf3),("XMOVEONS:01 forced caller",0x37e00,0x5d6,0x5fe),
                ("XMOVEONS:0b scene writer",0x37e00,0x50e,0x540),
                ("XMOVEONG:01 interrupt exit",0x40000,0x27ed,0x28d7),
                ("SUNDOG:04 dispatch",0x2e200,0x253,0x2a3),("SUNDOG:04 XJP table",0x2e200,0x380,0x394),
                ("SHIPLIB:02 user wrapper",0x31e00,0x1a,0x20),
                ("XDOUSERM:01",0x34800,0x7be,0x860),("XDOUSERM:11",0x34800,0x76e,0x7b9)]]
        if args.compare_remake:
            env["SUNDOG_SLEEP_INTERRUPT_NATIVE"] = str(output)
            run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosSleepInterrupt$", "-count=1", "-v"],
                       output / "remake.log", cwd=repo, env=env)
            formal = json.loads((output / "remake-food-sleep-interrupt.json").read_text())
            assert formal["Cases"] == 48 and formal["Differences"] == 0
            report.update(status="PASS-sleep-interrupt-same-state", specState="READY-auto-sleep-interrupt",
                          formal=formal, formalRuntime=f"schema{game_schema}")
            report["sources"].update({str(v.relative_to(repo)): sha(v) for v in sorted((repo / "internal/game").glob("*.go"))})
            report["resolutionBacklinks"] = []
            for document in ["docs/atari-st/ground-rules.md", "docs/atari-st/ship-rules.md",
                             "docs/atari-st/banville-restoration-spec.md"]:
                contents = (repo / document).read_text()
                for required in ["sleep-interrupt-20261004", "raw0x2e800", "0b98..0bc9",
                                 "raw0x40000", "27ed..28d7", "raw0x37e00", "05d6..05fe"]:
                    assert required in contents, (document, required)
                report["resolutionBacklinks"].append(dict(document=document, checked=True,
                    correctionMarker="sleep-interrupt-20261004", evidenceLevel="proven-controlled-native",
                    originalAddress="raw0x2e800:0b98..0bc9;raw0x40000:27ed..28d7;raw0x37e00:05d6..05fe"))
        vet = ["go", "vet", "-overlay", str(overlay), "./internal/m68k"]
        run_logged(vet, output / "vet.log", cwd=talos, env=env)
        report["vetCommand"] = vet
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("自動睡眠原始中斷父流程48案，獨立模型零差異；" +
              ("正式同狀態零差異。" if args.compare_remake else "正式仍待READY及實作。"))
        return
    if args.attribute_clock_only:
        command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                   "-run", "^TestSunDogFoodAttributeClock$", "-count=1", "-v"]
        run_logged(command, output / "native.log", cwd=talos, env=env)
        native = json.loads((output / "native-food-attribute-clock.json").read_text())
        assert native["InitAfter"][:10] == native["InitBefore"][:10]
        assert native["InitAfter"][10:] == [5, 5, 5, 5, 1]
        assert len(native["Rows"]) == 128
        for row in native["Rows"]:
            expected = []
            for index, current in enumerate(row["Current"]):
                difference = row["Target"][index] - current
                delta = min(abs(difference), [5, 5, 5, 5, 1][index])
                value = current + delta * ((difference > 0) - (difference < 0)) if row["MinuteBefore"] == 59 else current
                expected.append(value)
            assert row["After"] == expected, row
        report = dict(status="PASS-native-attribute-clock", cases=128, modelDifferences=0,
            formalRuntime=f"not-compared-schema{game_schema}", native=native, diskSHA256=sha(disk),
            interpreterSHA256=sha(interp), go=version, command=command,
            talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
            addressSpace="sundog.st raw offset；start/endExclusive為segment-relative byte offset",
            sources={str(v.relative_to(repo)): sha(v) for v in sources + [Path(__file__).resolve()]})
        raw = disk.read_bytes()
        report["originalSlices"] = [dict(name=name, rawBase=hex(base), start=hex(start),
            endExclusive=hex(end), SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest())
            for name, base, start, end in [("XSTARTUP:30 rates", 0x39c00, 0x1bd1, 0x1bf1),
                                         ("MAINLIB:35", 0x2e800, 0xbf8, 0xd22)]]
        report["resolutionBacklinks"] = []
        for document in ["docs/atari-st/ground-rules.md", "docs/atari-st/banville-restoration-spec.md"]:
            contents = (repo / document).read_text()
            for required in ["attribute-clock-20261004", "raw0x39c00", "1bd1..1bf1", "raw0x2e800", "0bf8..0d20"]:
                assert required in contents, (document, required)
            report["resolutionBacklinks"].append(dict(platformModule="atari-st:XSTARTUP:30+MAINLIB:35",
                originalAddress="raw0x39c00:1bd1..1bf1;raw0x2e800:0bf8..0d20", document=document,
                correctionMarker="attribute-clock-20261004", evidenceLevel="proven-controlled-native", checked=True))
        if args.compare_remake:
            env["SUNDOG_ATTRIBUTE_NATIVE"] = str(output / "native-food-attribute-clock.json")
            run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosAttributeClock$", "-count=1", "-v"],
                       output / "remake.log", cwd=repo, env=env)
            formal = json.loads((output / "remake-food-attribute-clock.json").read_text())
            assert formal["Cases"] == 128 and formal["Differences"] == 0
            report.update(status="PASS-attribute-clock-same-state", formal=formal, formalRuntime=f"schema{game_schema}")
            report["sources"].update({str(v.relative_to(repo)): sha(v) for v in
                sorted((repo / "internal/game").glob("*.go"))})
        vet = ["go", "vet", "-overlay", str(overlay), "./internal/m68k"]
        run_logged(vet, output / "vet.log", cwd=talos, env=env)
        report["vetCommand"] = vet
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("原版恢復速率初始化、屬性整點128案及獨立模型通過；" +
              ("正式零差異。" if args.compare_remake else "正式尚未比較。"))
        return
    if args.sleep_chain_only or args.sleep_parent_only:
        command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                   "-run", "^TestSunDogFoodSleep(Wake|Theft|ShipParent|MenuParent)$" if args.sleep_parent_only
                   else "^TestSunDogFoodSleep(Wake|Theft)$", "-count=1", "-v"]
        # The 537-case batch completed in 129s and exceeded the generic 150s
        # limit under shared-host load. Keep a bounded deadline and exact
        # comparisons; the native harness retains its per-call step limits.
        native_timeout = 240 if args.sleep_parent_only else 150
        run_logged(command, output / "native.log", cwd=talos, env=env, timeout=native_timeout)
        wake = json.loads((output / "native-food-sleep-wake.json").read_text())
        theft = json.loads((output / "native-food-sleep-theft.json").read_text())
        model = verify_sleep_research(wake, theft)
        vet = ["go", "vet", "-overlay", str(overlay), "./internal/m68k"]
        run_logged(vet, output / "vet.log", cwd=talos, env=env)
        report = dict(status="PASS-native-sleep-wake-theft-controlled", specState="DRAFT-full-sleep",
                      formalRuntime=f"not-compared-schema{game_schema}", originalGUI="unverified", model=model,
                      diskSHA256=sha(disk), interpreterSHA256=sha(interp), go=version,
                      talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                      command=command, vetCommand=vet, nativeWake=wake, nativeTheft=theft,
                      nativeTimeoutSeconds=native_timeout,
                      sources={str(v.relative_to(repo)): sha(v) for v in sources + [Path(__file__).resolve()]},
                      boundaries=["背景槽2缺席；SHIPLIB:12未執行", "圖形與事件ABI隔離；無正常GUI",
                                  "分鐘槽1由預設60真實讀停恢復；父槽初始化／wall-clock未核定",
                                  "location mode/bit13/flag57語意consumer待核對；不直接命名安全地點",
                                  "mode12初態為研究邊界，不當正常死亡狀態或可復活證據",
                                  "原始RND雙字種子預設(0,0)/(1234,5678)，全案保留；正式RNG未比較"])
        if args.sleep_parent_only:
            ship = json.loads((output / "native-food-sleep-ship-parent.json").read_text())
            menu = json.loads((output / "native-food-sleep-menu-parent.json").read_text())
            report.update(status="PASS-native-sleep-parent-controlled", parentModel=verify_sleep_parent(ship, menu),
                          nativeShipParent=ship, nativeMenuParent=menu)
            raw = disk.read_bytes()
            ranges = [("SUNDOG:01 minute init", 0x2e200, 0x311, 0x316),
                      ("SUNDOG:03 ship init", 0x2e200, 0x1b4, 0x1b8),
                      ("XDOUSERM:01", 0x34800, 0x7be, 0x860),
                      ("XDOUSERM:08", 0x34800, 0x14e, 0x18a),
                      ("XDOUSERM:11", 0x34800, 0x76e, 0x7b9),
                      ("XDOUSERM:07 isolated writer", 0x34800, 0x134, 0x138),
                      ("XDOUSERM:10", 0x34800, 0x5b0, 0x76a),
                      ("SHIPLIB:12", 0x31e00, 0x1278, 0x12c3),
                      ("SHIPLIB:1f", 0x31e00, 0x8ea, 0xc4c),
                      ("SHIPLIB:23", 0x31e00, 0x105c, 0x1274),
                      ("SHIPLIB:22", 0x31e00, 0xf6c, 0xf83),
                      ("SHIPLIB:25", 0x31e00, 0xffe, 0x1058),
                      ("SHIPLIB:24", 0x31e00, 0xf88, 0xff9),
                      ("MAINLIB:0a", 0x2e800, 0x66c, 0x682),
                      ("MAINLIB:35", 0x2e800, 0xbf8, 0xd22)]
            report["addressSpace"] = "sundog.st raw file byte offset；每項start/endExclusive為segment-relative byte offset"
            report["originalSlices"] = [dict(name=name, rawBase=hex(base), start=hex(start),
                endExclusive=hex(end), SHA256=hashlib.sha256(raw[base+start:base+end]).hexdigest())
                for name, base, start, end in ranges]
            report["boundaries"] = ["正常槽初始化原始caller已執行；idle SHIPLIB:12及能量helpers已執行",
                "移動、Enlie、交戰、tractor/cloak、rescue/jettison及aux tank不在本新增矩陣",
                "完整XDOUSERM:01/08/11狀態writer已執行；menu/hit-test、圖形建構與事件ABI隔離",
                "既有423案仍保持其槽2缺席與受控失竊gate，不外推正常GUI或硬體timer",
                "模式12人工初態保持研究限制；正式休息/schema42尚未比較"]
        if args.compare_remake:
            env["SUNDOG_SLEEP_NATIVE"] = str(output)
            run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosSleep$", "-count=1", "-v"],
                       output / "remake.log", cwd=repo, env=env)
            formal = json.loads((output / "remake-food-sleep.json").read_text())
            assert [formal[name]["Cases"] for name in ["Wake", "Ship", "Menu", "Theft"]] == [42, 90, 24, 360]
            assert all(formal[name]["Differences"] == 0 for name in ["Wake", "Ship", "Menu", "Theft"])
            assert formal["ResearchOnly"] == 21
            report.update(status="PASS-sleep-status-same-state", specState="READY-status-sleep", formal=formal,
                          formalRuntime=f"schema{game_schema}", formalBoundaries=["人工mode12初態21案留研究",
                          "睡眠四讀數／分鐘／醒來與idle能量比較；非正式屬性速率矩陣不比較，正常屬性時鐘由128案另驗",
                          "失竊使用執行前固定的原版RND輸入；不聲稱正式RNG骰序一致",
                          "航行／交戰中自動強制睡眠父排程與原版GUI仍未驗"])
            report["resolutionBacklinks"] = []
            for document in ["docs/atari-st/ground-rules.md", "docs/atari-st/ship-rules.md",
                             "docs/atari-st/banville-restoration-spec.md"]:
                contents = (repo / document).read_text()
                for required in ["sleep-status-20261004", "raw0x34800", "05b0..0768", "raw0x37e00", "014e..0172", "0493..04d3"]:
                    assert required in contents, (document, required)
                report["resolutionBacklinks"].append(dict(platformModule="atari-st:XDOUSERM:10+XMOVEONS:04/09",
                    originalAddress="raw0x34800:05b0..0768;raw0x37e00:014e..0172;0493..04d3", document=document,
                    correctionMarker="sleep-status-20261004", evidenceLevel="proven-controlled-native", checked=True))
            report["sources"].update({str(v.relative_to(repo)): sha(v) for v in
                sorted((repo / "internal/game").glob("*.go")) + sorted((repo / "internal/storage").glob("*.go"))})
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(f"原生睡眠／醒來{model['wakeCases']}案、失竊{model['theftCases']}案" +
              ("、正常睡眠父入口24案、idle船艦90案" if args.sleep_parent_only else "") +
              ("；正式角色面板睡眠516案零差異，完整強制中斷父流程仍DRAFT。" if args.compare_remake
               else "；研究模型零差異。正式鏈仍DRAFT。"))
        return
    if args.stores_chain_only:
        command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                   "-run", "^TestSunDogFoodStores(Chain|Window)$", "-count=1", "-v"]
        run_logged(command, output / "native.log", cwd=talos, env=env)
        native = json.loads((output / "native-food-stores-chain.json").read_text())
        assert len(native["Rows"]) == 700
        report = dict(status="PASS-native-stores-chain", specState="DRAFT-food-sleep-chain",
            formalRuntime="not-compared", originalGUI="unverified", randomness="無RND分支",
            diskSHA256=sha(disk), interpreterSHA256=sha(interp), go=version, command=command,
            talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
            native=native, window=json.loads((output / "native-food-stores-window.json").read_text()),
            cases=len(native["Rows"]), sources={str(v.relative_to(repo)): sha(v)
                for v in sources + [Path(__file__).resolve(), repo / "tools/psys_disasm.py"]})
        if args.compare_remake:
            run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosStoresComparison$",
                        "-count=1", "-v"], output / "remake.log", cwd=repo, env=env)
            formal = json.loads((output / "remake-food-stores-chain.json").read_text())
            assert formal["Cases"] == 700 and formal["Differences"] == 0
            report.update(status="PASS-stores-same-state-rules", formalRuntime=f"schema{game_schema}",
                          formal=formal, specState="READY-Stores-integration-pending-GUI-save")
            report["sources"].update({str(v.relative_to(repo)): sha(v) for v in
                sorted((repo / "internal/game").glob("*.go"))})
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(f"原版Stores鏈：{len(native['Rows'])}案通過；" +
              ("正式同初態零差異，正常GUI與保存另驗。" if args.compare_remake else "正式鏈待驗。"))
        return
    command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
               "-run", "^TestSunDogFood(BurgerConsumption|ClockHourGate|ConditionProbe|ConditionMatrix|SleepMinute)$", "-count=1", "-v"]
    run_logged(command, output / "native.log", cwd=talos, env=env)
    native = json.loads((output / "native-food-burger.json").read_text())
    assert len(native["Rows"]) == 4096
    clock = json.loads((output / "native-food-clock-gate.json").read_text())
    assert len(clock["Rows"]) == 960
    condition = json.loads((output / "native-food-condition.json").read_text())
    assert len(condition["Rows"]) == condition["Inputs"] * 2
    sleep = json.loads((output / "native-food-sleep-minute.json").read_text())
    assert len(sleep["Rows"]) == 1536
    # 獨立研究模型依原始指令，包括低於64的額外Vigor懲罰。
    def condition_model(row):
        vigor, rest, health, nourishment = row["Before"]
        steps = max(0, int((vigor-128)/32))
        if not row["Asleep"]:
            rest -= [4, 2, 3, 4][rest//64]
        health -= [1, 0, 0, 0][health//64]
        if health > 128:
            health += steps*4
        nourishment -= [1, 1, 2, 3][nourishment//64]
        rest, health, nourishment = [min(255, max(0, v)) for v in (rest, health, nourishment)]
        penalty = sum(v//16-4 for v in (rest, health, nourishment) if v < 64)
        if penalty < 0:
            vigor += penalty
        else:
            target = (rest+health+nourishment)//3
            vigor += 2*(target > vigor)-2*(target < vigor)
        mode, g3, g18 = (0x7a77, 1, 7) if rest == 0 else (0x7777, 0, 7)
        if vigor <= 0:
            vigor, mode, g3, g18 = 0, (mode & ~0xf00) | 0xc00, 1, 0
        return [min(255, vigor), rest, health, nourishment], mode, g3, g18
    differences = sum(condition_model(row) != (row["After"], row["Mode"], row["G3"], row["G18"])
                      for row in condition["Rows"])
    if differences:
        raise RuntimeError(f"原始條件更新與獨立研究模型差異{differences}")
    report = dict(status="PASS-native-burger-and-hour-gate", cases=4096, clockCases=960, specState="DRAFT-food-sleep-chain",
        formalRuntime="not-compared", originalGUI="unverified", randomness="無RND分支",
        diskSHA256=sha(disk), interpreterSHA256=sha(interp), go=version, command=command,
        talosHead=subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
        native=native, clock=clock, condition=condition, sleep=sleep, sleepCases=1536,
        conditionCases=len(condition["Rows"]), modelDifferences=differences,
        sources={str(v.relative_to(repo)): sha(v) for v in sources +
            [Path(__file__).resolve(), repo / "tools/psys_disasm.py"]})
    if args.compare_remake:
        remake_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run",
                          "^TestExportTalosFoodBaseline$", "-count=1", "-v"]
        run_logged(remake_command, output / "remake.log", cwd=repo, env=env)
        baseline = json.loads((output / "remake-food-baseline.json").read_text())
        report.update(status="controlled-condition-burger-comparison",
                      remakeBaseline=baseline, remakeCommand=remake_command)
        for name in ("internal/game/food_native_parity_test.go", "internal/game/reducer.go",
                     "internal/game/condition.go", "internal/game/state.go"):
            report["sources"][name] = sha(repo/name)
        if args.require_condition_core:
            if (baseline["ConditionCases"], baseline["ConditionDiffs"], baseline["NoHourCases"], baseline["NoHourDiffs"]) != (7092, 0, 225, 0):
                raise RuntimeError("正式四項讀數／整點閘門尚不符合窄READY契約")
            report.update(status="PASS-condition-core-and-burger-head", conditionSpec="CONFORMED-condition-hour-core", formalRuntime="schema42-condition-hour-burger-head")
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(f"原版PASS：漢堡4096案、整點閘門960案、條件{len(condition['Rows'])}案；獨立模型零差異。")
    if args.compare_remake:
        print(f"正式差異：條件{baseline['ConditionDiffs']}/{baseline['ConditionCases']}、進食{baseline['EatDiffs']}/{baseline['EatCases']}、未跨整點{baseline['NoHourDiffs']}/{baseline['NoHourCases']}；完整食物與睡眠鏈尚未CONFORMED。")


if __name__ == "__main__":
    main()
