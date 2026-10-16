"""Docker 原生排程研究；入口見 docs/atari-st/talos-combat-audit-20260907.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run_flight_back(repo,talos,output,env,version):
    names=("combat_aim","combat_cadence","combat_course","combat_init","combat_parent","combat_display",
        "combat_levels","combat_movement","combat_fuel","fuel_depot","fuel_entry","walker_slices",
        "banville","banville_price","banville_site","trade_classes","food","flight_clock")
    sources=[repo/f"tools/talos_{name}_test.go.txt" for name in names]
    overlay=output/"overlay.json"
    overlay.write_text(json.dumps({"Replace":{str(talos/f"internal/m68k/sundog_{n}_test.go"):str(p) for n,p in zip(names,sources)}}))
    native=["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k","-run","^TestSunDogFlightExitMenu$","-count=1","-v"]
    formal=["go","test","./internal/game","-run","^TestFlightBackPreservesRouteAndAllowsShipboardSleep$","-count=1","-v"]
    run_logged(native,output/"native.log",cwd=talos,env=env)
    run_logged(formal,output/"formal.log",cwd=repo,env=env)
    rows=json.loads((output/"native-flight-exit.json").read_text());assert len(rows)==16
    for row in rows:
        expected=row['Before']&0xf0ff|0x200 if row['Local2']==row['G3']==0 else row['Before']
        assert row['After']==expected and row['FieldsBefore']==row['FieldsAfter'] and row['TravelAfter'] and row['SubcAfter']
    report=dict(status='PASS-narrow-flight-back',cases=16,formalCases=6,native=rows,commands=[native,formal],
        scope='XPILOTAG raw0x35200+04dd..04ec退出mode writer及14原始欄位／Travel／Subc保持；正式Back六個Subc×phase場景及船艙睡眠可達性回歸。非兩側完整GUI同狀態。',
        sceneMapping='XMOVEONS mode2/10 root consumer及既有船艙oracle支持Shipboard；完整SUNDOG跨段dispatch本輪未跑，strong inference。writer proven-native-scoped。',
        go=version,diskSHA256=sha(Path(env['SUNDOG_PARITY_DISK'])),interpreterSHA256=sha(Path(env['TALOS_UCSD_INTERP'])/'SYSTEM.INTERP'),
        talosHead=subprocess.check_output(['git','rev-parse','HEAD'],cwd=talos,text=True).strip(),
        sources={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),repo/'internal/game/reducer.go',repo/'internal/game/flight_back_test.go']})
    (output/'receipt.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print('PASS 原生退出16案／正式Back六場景')


def run_flight_ship_parent(repo,talos,output,env,version):
    names=("combat_aim","combat_cadence","combat_course","combat_init","combat_parent","combat_display",
        "combat_levels","combat_movement","combat_fuel","fuel_depot","fuel_entry","walker_slices",
        "banville","banville_price","banville_site","trade_classes","food","flight_clock")
    sources=[repo/f"tools/talos_{name}_test.go.txt" for name in names]
    overlay=output/"overlay.json"
    overlay.write_text(json.dumps({"Replace":{str(talos/f"internal/m68k/sundog_{n}_test.go"):str(p) for n,p in zip(names,sources)}}))
    commands=[["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k","-run","^TestSunDog(FlightShipSleepParent|FlightCargoGate|FoodSleepShipParent)$","-count=1","-v"],
        ["go","test","./internal/game","-run","^TestExportTalosFlight(ShipSleep|CargoGate)$","-count=1","-v"],
        ["go","vet","-overlay",str(overlay),"./internal/m68k"]]
    run_logged(commands[0],output/"native.log",cwd=talos,env=env)
    run_logged(commands[1],output/"remake.log",cwd=repo,env=env)
    run_logged(commands[2],output/"vet.log",cwd=talos,env=env)
    sleep=json.loads((output/"native-flight-ship-sleep.json").read_text())
    formal_sleep=json.loads((output/"remake-flight-ship-sleep.json").read_text())
    gate=json.loads((output/"native-flight-cargo-gate.json").read_text())
    formal_gate=json.loads((output/"remake-flight-cargo-gate.json").read_text())
    key=lambda i:json.dumps(i,sort_keys=True)
    assert len(sleep)==len(formal_sleep)==72 and len({key(n["Input"]) for n in sleep})==72
    assert [n["Input"] for n in sleep]==[n["Input"] for n in formal_sleep]
    assert len(gate)==len(formal_gate)==1024 and {tuple(n["Codes"]) for n in gate}=={(a,b) for a in range(32) for b in range(32)}
    assert [n["Codes"] for n in gate]==[n["Codes"] for n in formal_gate]
    gate_diff=[dict(input=n["Codes"],native=n["Allowed"],formal=f["Allowed"]) for n,f in zip(gate,formal_gate) if n["Allowed"]!=f["Allowed"]]
    sleep_diff=[f for f in formal_sleep if f["Differences"]]
    report=dict(status="PASS-flight-ship-parent-projections" if not gate_diff and not sleep_diff else "FAIL-flight-ship-parent-projections",
        sleepCases=72,sleepDifferences=len(sleep_diff),cargoGateCases=1024,cargoGateDifferences=len(gate_diff),gateDifferences=gate_diff,sleepDifferenceRows=sleep_diff,
        nativeSleep=sleep,formalSleep=formal_sleep,nativeGate=gate,formalGate=formal_gate,commands=commands,
        scope="原版完整XDOUSERM睡眠父程序→完整SHIPLIB:12／1f／23，空貨艙與正常／末刻mode2，分鐘／能量／抵達／醒來；第四貨艙閘門全1024案。完整有貨遭遇／交戰及正常Goods31 GUI仍未驗。",
        boundaries="原版睡眠圖形／事件／鎖及MAINLIB提示音12的a54呼叫ABI隔離，writer真正執行；原生背景硬體event未供應。固定RND雙字(1,0)在原始MAINLIB真正執行；正式RNG算法不同，intact engine與無遭遇貨物使本輪結果可比，不宣稱跨流程亂數呼叫數相同。",
        originalGUI="unverified",saveSchema=44,go=version,diskSHA256=sha(Path(env["SUNDOG_PARITY_DISK"])),interpreterSHA256=sha(Path(env["TALOS_UCSD_INTERP"])/"SYSTEM.INTERP"),
        talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
        sourceSHA256={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),repo/"internal/game/talos_flight_setup_fixture_test.go",repo/"internal/game/combat.go",repo/"internal/game/reducer.go",repo/"internal/game/sleep.go"]})
    from psys_disasm import decode_one,format_one
    raw=Path(env["SUNDOG_PARITY_DISK"]).read_bytes()
    report["slices"]=[]
    for name,base,start,end in [("cargo gate",0x31e00,0xafc,0xb29),("arrival sound ABI",0x31e00,0xa53,0xa56),("complete ship callback",0x31e00,0x1278,0x12c3)]:
        data,pos,lines=raw[base:],start,[]
        while pos<end:
            op=decode_one(data,pos);lines.append(format_one(op,data,pos));pos+=op["length"]
        assert pos==end
        report["slices"].append(dict(name=name,rawBase=hex(base),start=hex(start),endExclusive=hex(end),hex=data[start:end].hex(),instructions=lines,addressSpace="磁碟raw基底加段內byte位移",grade="proven-native-scoped"))
    report["decoderSHA256"]=sha(repo/"tools/psys_disasm.py")
    (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(f"完整空艙航行睡眠72案差異{len(sleep_diff)}，貨艙閘門1024案差異{len(gate_diff)}。")
    if gate_diff or sleep_diff: raise SystemExit(1)


def run_flight_arrival(repo, talos, output, env, version):
    """原始抵達writer與正式背景／狀態／睡眠的目的地接線。"""
    names = ("combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent",
             "combat_display", "combat_levels", "combat_movement", "combat_fuel", "fuel_depot",
             "fuel_entry", "walker_slices", "banville", "banville_price", "banville_site", "trade_classes",
             "food", "flight_clock")
    sources = [repo/f"tools/talos_{name}_test.go.txt" for name in names]
    overlay=output/"overlay.json"
    overlay.write_text(json.dumps({"Replace":{str(talos/f"internal/m68k/sundog_{name}_test.go"):str(p) for name,p in zip(names,sources)}}))
    commands=[["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k","-run","^TestSunDogFlightArrivalBoundary$","-count=1","-v"],
              ["go","test","./internal/game","-run","^TestExportTalosFlightArrival$","-count=1","-v"],
              ["go","vet","-overlay",str(overlay),"./internal/m68k"]]
    run_logged(commands[0],output/"native.log",cwd=talos,env=env)
    run_logged(commands[1],output/"remake.log",cwd=repo,env=env)
    run_logged(commands[2],output/"vet.log",cwd=talos,env=env)
    native=json.loads((output/"native-flight-arrival.json").read_text())
    formal=json.loads((output/"remake-flight-arrival.json").read_text())
    key=lambda x:json.dumps(x,sort_keys=True)
    assert len(native)==36 and len({key(r["Input"]) for r in native})==36
    assert len(formal)==108 and {(key(r["Input"]),r["Scene"]) for r in formal}=={(key(r["Input"]),scene) for r in native for scene in ("flight","status","sleep")}
    for n in native:
        assert n["Mode"]==0 and not n["Travel"] and n["Tick"]==n["Input"]["Ticks"]
        assert n["Ship"][1:]==[0x3301,0x3302,0x3303,0x3304,0x3305]
        assert n["Location"]==n["Ship"][:4]+[0x22a4,0x33a5]
    differences=[r for r in formal if r["Differences"]]
    report=dict(status="PASS-arrival-connection" if not differences else "FAIL-arrival-connection",nativeCases=36,formalCases=108,
        formalDifferences=len(differences),differences=differences,native=native,formal=formal,
        scope="原版mode2完整移動／抵達記錄writer與父相位；正式完整分鐘／船艦callback至抵達，flight／status／sleep三路。其他原版HUD／遭遇後綴與完整交戰未驗。",
        originalGUI="unverified",randomness="原版seed17執行前固定，正式CombatRolls17；intact status16隔離移動判定，兩種RNG序列未冒稱相同算法。",
        diskSHA256=sha(Path(env["SUNDOG_PARITY_DISK"])),interpreterSHA256=sha(Path(env["TALOS_UCSD_INTERP"])/"SYSTEM.INTERP"),go=version,
        talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),commands=commands,
        sourceSHA256={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),repo/"internal/game/talos_flight_setup_fixture_test.go",repo/"internal/game/reducer.go"]})
    from psys_disasm import decode_one,format_one
    raw=Path(env["SUNDOG_PARITY_DISK"]).read_bytes()
    report["slices"]=[]
    for name,start,end in [("SHIPLIB:1f arrival",0xa14,0xa75),("SHIPLIB:12 phase",0x128b,0x1294)]:
        data,pos,lines=raw[0x31e00:],start,[]
        while pos<end:
            op=decode_one(data,pos);lines.append(format_one(op,data,pos));pos+=op["length"]
        assert pos==end
        report["slices"].append(dict(name=name,rawBase="0x31e00",start=hex(start),endExclusive=hex(end),addressSpace="磁碟raw基底加段內byte位移",hex=data[start:end].hex(),instructions=lines,grade="proven-native-scoped"))
    report["decoderSHA256"]=sha(repo/"tools/psys_disasm.py")
    (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(f"原版抵達36案、正式背景108案，差異{len(differences)}。")
    if differences: raise SystemExit(1)


def run_flight_setup(repo, talos, output, env, version):
    """原版初始化與正式Navigate的mode2 ETA投影；保留整段world/warp未知。"""
    names = ("combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent",
             "combat_display", "combat_levels", "combat_movement", "combat_fuel", "fuel_depot",
             "fuel_entry", "walker_slices", "banville", "banville_price", "banville_site", "trade_classes",
             "food", "flight_clock")
    sources = [repo / f"tools/talos_{n}_test.go.txt" for n in names]
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(talos / f"internal/m68k/sundog_{n}_test.go"): str(p)
                                             for n, p in zip(names, sources)}}))
    formal_command = ["go", "test", "./internal/game", "-run", "^TestExportTalosFlightSetupClock$", "-count=1", "-v"]
    run_logged(formal_command, output / "remake.log", cwd=repo, env=env)
    native_command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
                      "-run", "^TestSunDogFlightSetupClock$", "-count=1", "-v"]
    run_logged(native_command, output / "native.log", cwd=talos, env=env)
    formal = json.loads((output / "remake-flight-setup.json").read_text())
    native = json.loads((output / "native-flight-setup.json").read_text())
    assert len(formal) == 24 and len(native["Rows"]) == 72
    assert len({json.dumps(r["Input"], sort_keys=True) for r in formal}) == 24
    by_input = {json.dumps(r["Input"], sort_keys=True): r for r in formal}
    unique_native = {(json.dumps(r["Input"],sort_keys=True),r["Period"]) for r in native["Rows"]}
    assert unique_native == {(key,period) for key in by_input for period in (4,32,60)} and len(unique_native)==72
    differences = []
    for row in native["Rows"]:
        i = row["Input"]
        delta = i["ToAngle"]-i["Angle"]
        if delta > 128: delta -= 256
        if delta < -128: delta += 256
        dr = i["ToRadius"]-i["Radius"]
        ticks = (abs(dr)//3 + abs(delta)*(i["Radius"]+i["ToRadius"])//256)*4+1
        eta = ticks+ticks//4
        assert row["ClockBefore"] == row["ClockAfter"] and row["TimeAfter"] == i["Time"], row
        assert [row["Ticks"],row["DeltaAngle"],row["DeltaRadius"],row["ETA"]] == [ticks,delta,dr,i["Time"]+eta], row
        assert row["Slots"] == [[1,3,15],[2,0,row["Period"]//4]]+[[0,0,0]]*6, row
        assert row["RecordAfter"][0]&0xc0 == row["RecordBefore"][0]&0xc0, row
        assert row["RecordAfter"][1]&0xc000 == row["RecordBefore"][1]&0xc000, row
        if row["Period"] != 32: continue
        f = by_input[json.dumps(i,sort_keys=True)]
        assert [f["Ticks"],f["DeltaAngle"],f["DeltaRadius"],f["ArcETA"]] == [ticks,delta,dr,eta], f
        # The formal command still composes atomic world/warp legs. Report
        # those separately; this projection must not certify their timing.
        prepayment = f["TimeAfter"]-i["Time"]-f["AtomicMinutes"]
        if prepayment != 0:
            differences.append(dict(input=i, mode2MinutesPrepaid=prepayment, expected=0,
                                    retainedAtomicMinutes=f["AtomicMinutes"]))
    vet = ["go", "vet", "-overlay", str(overlay), "./internal/m68k"]
    run_logged(vet,output/"vet.log",cwd=talos,env=env)
    report = dict(status="PASS-mode2-ETA-projection" if not differences else "FAIL-mode2-ETA-prepayment",
        nativeCases=72, formalCases=24, formalDifferences=len(differences), differences=differences,
        scope="原版完整數值初始化與時間記錄；正式Navigate的mode2 ETA預扣投影，保留原子world/warp時間近似，非整段same-state。",
        originalGUI="unverified", randomness="無event與RND；固定幾何／主時鐘初態，不挑seed",
        diskSHA256=sha(Path(env["SUNDOG_PARITY_DISK"])),
        interpreterSHA256=sha(Path(env["TALOS_UCSD_INTERP"])/"SYSTEM.INTERP"), go=version,
        talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
        commands=[formal_command,native_command,vet], native=native, formal=formal,
        sourceSHA256={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),
            repo/"internal/game/talos_flight_setup_fixture_test.go",repo/"internal/game/reducer.go",repo/"internal/game/flight.go"]})
    from psys_disasm import decode_one,format_one
    raw=Path(env["SUNDOG_PARITY_DISK"]).read_bytes()
    report["slices"]=[]
    for name,base,start,end in [("XSHOWMOV:03 pause",0x37200,0x52,0x5b),
            ("XSHOWMOV:0a numeric setup",0x37200,0x4b7,0x58a),
            ("SHIPLIB:14 set_time",0x31e00,0x19a,0x1b3),
            ("SHIPLIB:13 add_time",0x31e00,0x128,0x15a),
            ("XMOVEONS:09 redraw caller",0x37e00,0x3d4,0x3fd),
            ("XMOVEONS:06 ship image decode",0x37e00,0x238,0x24e)]:
        data,pos,lines=raw[base:],start,[]
        while pos<end:
            instruction=decode_one(data,pos);lines.append(format_one(instruction,data,pos));pos+=instruction["length"]
        assert pos==end
        report["slices"].append(dict(name=name,rawBase=hex(base),start=hex(start),endExclusive=hex(end),
            addressSpace="磁碟raw基底加段內byte位移",grade="proven-source-context; native redraw lifecycle unverified" if name.startswith("XMOVEONS") else "proven-native-scoped",hex=data[start:end].hex(),
            SHA256=hashlib.sha256(data[start:end]).hexdigest(),instructions=lines))
    report["decoderSHA256"]=sha(repo/"tools/psys_disasm.py")
    (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print(f"原版初始化72案、正式ETA投影24案，差異{len(differences)}。")
    if differences: raise SystemExit(1)


def run_player_input(repo, talos, output, env, sources, version):
    """原版選單／分派與未初始化邊界；不要求正式排程已存在。"""
    sources = sources + [repo / "tools/talos_combat_player_input_test.go.txt",
                         repo / "tools/talos_walker_slices_test.go.txt"]
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {
        str(talos / "internal/m68k" / ("sundog_" + p.name.removeprefix("talos_").removesuffix(".txt"))): str(p)
        for p in sources}}))
    command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
               "-run", "^TestSunDogCombatPlayerInput$", "-count=1", "-v"]
    run_logged(command, output / "native.log", cwd=talos, env=env)
    if "--- PASS: TestSunDogCombatPlayerInput " not in (output / "native.log").read_text():
        raise RuntimeError("缺少原生玩家入口測試")
    answer = output / "native-combat-player-input.json"
    native = json.loads(answer.read_text())
    unique = {(r["StatusInput"], r["Attached"], r["CloakInstalled"], r["CloakActive"])
              for r in native["Menus"]}
    if len(unique) != 32 or len(native["Menus"]) != 32 or native["Dispatch"] != {"5": 0x1003, "7": 0x1021}:
        raise RuntimeError("原版入口覆蓋不符")
    report = {"status": "PASS-native-player-input", "formalRulesChanged": False,
              "scope": native["Scope"], "randomness": native["Randomness"],
              "menus": 32, "dispatches": 2, "ambiguousFrameChains": native["CloakCourse"],
              "diskSHA256": sha(Path(env["SUNDOG_PARITY_DISK"])),
              "interpreterSHA256": sha(Path(env["TALOS_UCSD_INTERP"]) / "SYSTEM.INTERP"),
              "go": version, "command": command,
              "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
              "sources": {str(p.relative_to(repo)): sha(p) for p in sources + [Path(__file__).resolve()]},
              "answerSHA256": sha(answer), "native": native}
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("PASS: 32 native menus, 2 dispatches; 2 explicit-frame cloak/course chains")


def compare_fuel_consumers(output):
    """Recompute differences from native answers; never trust formal counters."""
    paths = ("native-fuel.json", "native-aux-pod.json", "native-levels.json",
             "remake-fuel-consumers.json")
    missing = [name for name in paths if not (output / name).is_file()]
    if missing:
        return {"status": "FAIL", "missing": missing}
    fuel, aux, levels, formal = [json.loads((output / name).read_text()) for name in paths]
    aux_rows = []
    for row in aux.get("Native", []):
        projected = {k: v for k, v in row.items() if k != "Labels"}
        projected["LabelEmpty"] = [label[0] == 0 for label in row["Labels"]]
        aux_rows.append(projected)
    result = {"status": "PASS", "scope": formal.get("Scope"), "groups": {}}
    for name, expected, actual, count, original_count, original_diff in (
            ("Fuel", fuel.get("Native", []), formal.get("Fuel", []), 1728,
             fuel.get("Cases"), fuel.get("Differences")),
            ("Aux", aux_rows, formal.get("Aux", []), 320,
             aux.get("Cases"), aux.get("Differences")),
            ("Core", levels.get("CoreNative", []), formal.get("Core", []), 1152,
             levels.get("CoreCases"), levels.get("CoreDifferences"))):
        inputs = [json.dumps(row.get("Input"), sort_keys=True) for row in expected]
        coverage = (len(expected) == len(actual) == original_count == formal.get(name+"Cases") == count
                    and len(set(inputs)) == count)
        differences = sum(a != b for a, b in zip(expected, actual)) + abs(len(expected)-len(actual))
        passed = (coverage and original_diff == 0 and differences == 0
                  and formal.get(name+"Differences") == differences)
        result["groups"][name] = {"cases": len(actual), "required_cases": count,
                                 "complete_unique_inputs": coverage,
                                 "differences": differences, "original_differences": original_diff}
        if not passed:
            result["status"] = "FAIL"
    return result


def compare_mixed_energy(output):
    paths = ("native-fuel-mixed.json", "remake-fuel-mixed.json")
    missing = [name for name in paths if not (output / name).is_file()]
    if missing:
        return {"status": "FAIL", "missing": missing}
    native, formal = [json.loads((output / name).read_text()) for name in paths]
    expected, actual = native.get("Native", []), formal.get("Native", [])
    unique = len({json.dumps(row.get("Input"), sort_keys=True) for row in expected})
    coverage = len(expected) == len(actual) == native.get("Cases") == formal.get("Cases") == unique == 12096
    differences = sum(a != b for a, b in zip(expected, actual)) + abs(len(expected)-len(actual))
    return {"status": "PASS" if coverage and differences == formal.get("Differences") == 0 else "FAIL",
            "scope": formal.get("Scope"), "randomness": native.get("Randomness"),
            "cases": len(actual), "required_cases": 12096, "complete_unique_inputs": coverage,
            "differences": differences}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--player-input-only", action="store_true",
                        help="只重生原版通訊／隱形選單、分派及明示未初始化frame鏈，不宣稱正式排程一致")
    parser.add_argument("--flight-setup-only", action="store_true",
                        help="只驗原版航行初始化／時間記錄與正式mode2 ETA預扣，保留整段world/warp未知")
    parser.add_argument("--flight-arrival-only", action="store_true",
                        help="重生原版抵達36案及正式背景／狀態／睡眠108案")
    parser.add_argument("--flight-ship-parent-only", action="store_true",
                        help="完整空艙航行睡眠父鏈、第四貨艙閘門及正式比較")
    parser.add_argument("--flight-back-only", action="store_true",
                        help="原版航行退出16案及正式返回船艙六場景，不新增Flight H")
    parser.add_argument("--require-initial-cooldown", action="store_true",
                        help="驗收已修初始化冷卻；正式完整航向／排程投影仍只報差異")
    parser.add_argument("--require-mode5-drift", action="store_true",
                        help="驗收模式5獨立布林翻軸；完整set_course順序仍只報差異")
    parser.add_argument("--require-shield-levels", action="store_true",
                        help="驗收護盾phase0子規則；完整三相排程仍只報差異")
    parser.add_argument("--shield-gui", type=Path,
                        help="另比較正常GUI護盾扣油收據的phase0投影")
    parser.add_argument("--require-movement-fuel", action="store_true",
                        help="驗收三種初始相位的18案航程估油與實際航段；差異拒絕通過")
    parser.add_argument("--require-weapon-levels", action="store_true",
                        help="驗收啟用艦砲的一次充能／耗油子規則，完整排程另列差異")
    parser.add_argument("--weapon-gui", type=Path,
                        help="另核對正常GUI回充中的單次武器更新投影")
    parser.add_argument("--fuel-gui", type=Path,
                        help="另核對正常GUI耗盡燃料的末段護盾關閉投影")
    parser.add_argument("--require-aux-pod", action="store_true",
                        help="驗收既有63案船艙補油；全量320案另用--require-fuel-consumers")
    parser.add_argument("--require-fuel-consumers", action="store_true",
                        help="驗收兩入口全部320／1728案與活躍武器／護盾1152案，缺案及差異拒絕通過")
    parser.add_argument("--require-mixed-energy", action="store_true",
                        help="驗收活躍充能與備用油／特殊耗能12096案；不是完整父排程")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    repo = Path(__file__).resolve().parents[1]
    disk, talos, output = args.disk.resolve(), args.talos.resolve(), args.output.resolve()
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("磁碟身分不同")
    if sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("直譯器身分不同")
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        parser.error("輸出不可覆寫歷史收據")
    if output.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄擁有者不符")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用 Go 1.25.12")
    output.mkdir(mode=0o700, exist_ok=True)
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    if args.flight_back_only:
        if any(value for name,value in vars(args).items() if name not in ['disk','talos','output','flight_back_only']):
            parser.error('--flight-back-only須獨立執行')
        run_flight_back(repo,talos,output,env,version)
        return
    if args.flight_ship_parent_only:
        if args.flight_arrival_only or args.flight_setup_only or args.player_input_only or any(value for name,value in vars(args).items() if name.startswith("require_") or name.endswith("_gui")):
            parser.error("--flight-ship-parent-only須獨立執行")
        run_flight_ship_parent(repo,talos,output,env,version)
        return
    if args.flight_arrival_only:
        if args.flight_setup_only or args.player_input_only or any(value for name,value in vars(args).items() if name.startswith("require_") or name.endswith("_gui")):
            parser.error("--flight-arrival-only須獨立執行")
        run_flight_arrival(repo,talos,output,env,version)
        return
    if args.flight_setup_only:
        if args.player_input_only or any(value for name,value in vars(args).items() if name.startswith("require_") or name.endswith("_gui")):
            parser.error("--flight-setup-only須獨立執行")
        run_flight_setup(repo,talos,output,env,version)
        return
    if args.shield_gui:
        env["SUNDOG_PARITY_SHIELD_GUI"] = str(args.shield_gui.resolve(strict=True))
    if args.weapon_gui:
        env["SUNDOG_PARITY_WEAPON_GUI"] = str(args.weapon_gui.resolve(strict=True))
    if args.fuel_gui:
        env["SUNDOG_PARITY_FUEL_GUI"] = str(args.fuel_gui.resolve(strict=True))
    sources = [repo / "tools/talos_combat_aim_test.go.txt", repo / "tools/talos_combat_cadence_test.go.txt",
               repo / "tools/talos_combat_course_test.go.txt", repo / "tools/talos_combat_init_test.go.txt",
               repo / "tools/talos_combat_parent_test.go.txt", repo / "tools/talos_combat_display_test.go.txt",
               repo / "tools/talos_combat_levels_test.go.txt", repo / "tools/talos_combat_movement_test.go.txt",
               repo / "tools/talos_combat_fuel_test.go.txt"]
    sources.extend([repo / "tools/talos_fuel_depot_test.go.txt", repo / "tools/talos_fuel_entry_test.go.txt"])
    if args.player_input_only:
        if any(value for name, value in vars(args).items() if name.startswith("require_")):
            parser.error("--player-input-only 不可合用正式驗收旗標")
        run_player_input(repo, talos, output, env, sources, version)
        return
    run_logged(["go", "test", "./internal/game", "-run", "^TestExportTalosCombat(Cadence|Course|CourseBounds|Init|Mode5Drift|LevelsShield|LevelsWeapon|MovementFuel|AuxPod)$", "-count=1", "-v"],
               output / "remake.log", cwd=repo, env=env)
    with tempfile.TemporaryDirectory(prefix="sundog-cadence-overlay-") as temp:
        overlay = Path(temp) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {
            str(talos / "internal/m68k/sundog_combat_aim_test.go"): str(sources[0]),
            str(talos / "internal/m68k/sundog_combat_cadence_test.go"): str(sources[1]),
            str(talos / "internal/m68k/sundog_combat_course_test.go"): str(sources[2]),
            str(talos / "internal/m68k/sundog_combat_init_test.go"): str(sources[3]),
            str(talos / "internal/m68k/sundog_combat_parent_test.go"): str(sources[4]),
            str(talos / "internal/m68k/sundog_combat_display_test.go"): str(sources[5]),
            str(talos / "internal/m68k/sundog_combat_levels_test.go"): str(sources[6]),
            str(talos / "internal/m68k/sundog_combat_movement_test.go"): str(sources[7]),
            str(talos / "internal/m68k/sundog_combat_fuel_test.go"): str(sources[8]),
            str(talos / "internal/m68k/sundog_fuel_depot_test.go"): str(sources[9]),
            str(talos / "internal/m68k/sundog_fuel_entry_test.go"): str(sources[10])}}), encoding="utf-8")
        pattern = "TestSunDogCombat(Cadence|Course|CourseBounds|Init|Mode5Drift|Parent|CloakFlag|Flag44Writes|Display|Levels|Movement|Fuel|AuxPod)"
        if args.require_mixed_energy:
            pattern = "(" + pattern + "|TestSunDogFuelMixedEnergy)"
        command = ["go", "test", "-overlay", str(overlay), "./internal/m68k",
                   "-run", "^" + pattern + "$", "-count=1", "-v"]
        status = run_logged(command, output / "talos.log", cwd=talos, env=env, allow_failure=True,
                            timeout=480 if args.require_mixed_energy else 150)
    native_path = output / "native-cadence.json"
    native = json.loads(native_path.read_text()) if native_path.is_file() else {}
    if status == 0 and any("--- PASS: "+name+" " not in (output / "talos.log").read_text()
                           for name in ("TestSunDogCombatCadence", "TestSunDogCombatCourse", "TestSunDogCombatCourseBounds", "TestSunDogCombatInit", "TestSunDogCombatMode5Drift", "TestSunDogCombatParent", "TestSunDogCombatCloakFlag", "TestSunDogCombatFlag44Writes", "TestSunDogCombatDisplay", "TestSunDogCombatLevels", "TestSunDogCombatMovement", "TestSunDogCombatFuel", "TestSunDogCombatAuxPod")):
        raise RuntimeError("缺少原生測試，不能宣稱通過")
    consumers_status = 0
    if args.require_fuel_consumers:
        consumers_status = run_logged(
            ["go", "test", "./internal/game", "-run", "^TestExportTalosFuelConsumers$", "-count=1", "-v"],
            output / "remake-consumers.log", cwd=repo, env=env, allow_failure=True)
        if "--- PASS: TestExportTalosFuelConsumers " not in (output / "remake-consumers.log").read_text():
            consumers_status = consumers_status or 1
    mixed_status = 0
    if args.require_mixed_energy:
        mixed_status = run_logged(
            ["go", "test", "./internal/game", "-run", "^TestExportTalosFuelMixedEnergy$", "-count=1", "-v"],
            output / "remake-mixed.log", cwd=repo, env=env, allow_failure=True)
        if ("--- PASS: TestExportTalosFuelMixedEnergy " not in (output / "remake-mixed.log").read_text()
                or "--- PASS: TestSunDogFuelMixedEnergy " not in (output / "talos.log").read_text()):
            mixed_status = mixed_status or 1
    sources.extend([Path(__file__).resolve(), repo / "tools/ida_combat_interp_export.py",
                    repo / "tools/test_verify_talos_combat_cadence.py",
                    repo / "tools/atari_st_ship_rules.py", repo / "tools/replay_atari_st_combat.py",
                    repo / "tools/verify_talos_ground_collision.py",
                    repo / "internal/game/talos_combat_cadence_fixture_test.go",
                    repo / "internal/game/talos_fuel_energy_fixture_test.go",
                    repo / "internal/game/combat.go", repo / "internal/game/flight.go", repo / "internal/game/state.go",
                    repo / "internal/game/aux_fuel.go", repo / "internal/game/hold.go", repo / "internal/game/reducer.go",
                    repo / "internal/game/ship_energy.go", repo / "internal/game/fuel_state.go",
                    repo / "internal/game/fuel_purchase.go", repo / "internal/game/ground_route.go",
                    repo / "internal/source/atarist/assets/ship-rules.json",
                    repo / "go.mod", repo / "go.sum"])
    report = {"issue": "https://github.com/wicanr2/sundog-remake-zh-hant/issues/6",
              "status": "PASS" if status == 0 else "FAIL", "scope": native.get("Scope"),
              "cases": native.get("Cases"), "differences": native.get("Differences"),
              "ready_consumption_cases": native.get("PendingCases"),
              "course_gate_cases": native.get("CourseCases"),
              "course_gate_differences": native.get("CourseDifferences"),
              "formal_projection": {"scope": "現行 resolveEnemyFire 與原版準備階段的冷卻／counter 投影；ready、週期與旗標未有對應 runtime 欄位，非完整同狀態",
                                    "cases": native.get("FormalProjectionCases"),
                                    "timer_differences": native.get("FormalTimerDifferences"),
                                    "counter_differences": native.get("FormalCounterDifferences")},
              "randomness": native.get("Randomness"), "go_version": version,
              "command": command,
              "talos_head": subprocess.check_output(["git", "-C", str(talos), "rev-parse", "HEAD"], text=True).strip(),
              "disk_sha256": sha(disk), "interp_sha256": sha(interp),
              "source_sha256": {str(p.relative_to(repo)): sha(p) for p in sources},
              "talos_source_sha256": {str(p.relative_to(talos)): sha(p)
                  for p in sorted((talos / "internal/m68k").glob("*.go"))},
              "artifacts": {p.name: sha(p) for p in sorted(output.iterdir()) if p.is_file()}}
    course_path = output / "native-course.json"
    if course_path.is_file():
        course = json.loads(course_path.read_text())
        report["course"] = {"scope": course["Scope"], "randomness": course["Randomness"],
                            "cases": course["Cases"], "differences": course["Differences"],
                            "explicit_uninitialized_frame_cases": course["AmbiguousFrameCases"],
                            "formal_projection_cases": course["FormalCases"],
                            "formal_drift_differences": course["FormalDriftDifferences"],
                            "formal_closing_differences": course["FormalClosingDifferences"],
                            "formal_counter_differences": course["FormalCounterDifferences"]}
    bounds_path = output / "native-course-bounds.json"
    if bounds_path.is_file():
        bounds = json.loads(bounds_path.read_text())
        report["course_bounds"] = {"scope": bounds["Scope"], "randomness": bounds["Randomness"],
                                   "cases": bounds["Cases"], "differences": bounds["Differences"]}
    init_path = output / "native-init.json"
    if init_path.is_file():
        init = json.loads(init_path.read_text())
        report["init_fight"] = {"scope": init["Scope"], "randomness": init["Randomness"],
                               "cases": init["Cases"], "differences": init["Differences"]}
    encounter_path = output / "native-encounter-init.json"
    if encounter_path.is_file():
        encounter = json.loads(encounter_path.read_text())
        report["encounter_init"] = {k: v for k, v in encounter.items() if k != "Native"}
    position_path = output / "native-init-position.json"
    if position_path.is_file():
        position = json.loads(position_path.read_text())
        report["init_position"] = {k: v for k, v in position.items() if k != "Native"}
    drift_path = output / "native-mode5-drift.json"
    if drift_path.is_file():
        drift = json.loads(drift_path.read_text())
        report["mode5_drift"] = {k: v for k, v in drift.items() if k != "Native"}
    parent_path = output / "native-parent.json"
    if parent_path.is_file():
        parent = json.loads(parent_path.read_text())
        report["parent"] = {k: v for k, v in parent.items() if k not in ("CoreNative", "AttackNative", "LifecycleNative")}
    cloak_path = output / "native-cloak-flag.json"
    if cloak_path.is_file():
        cloak = json.loads(cloak_path.read_text())
        report["cloak_flag"] = {k: v for k, v in cloak.items() if k != "Native"}
    flag44_path = output / "native-flag44-writes.json"
    if flag44_path.is_file():
        flag44 = json.loads(flag44_path.read_text())
        report["flag44_writes"] = {k: v for k, v in flag44.items() if k != "Native"}
    display_path = output / "native-display.json"
    if display_path.is_file():
        display = json.loads(display_path.read_text())
        report["display"] = {k: v for k, v in display.items() if not k.endswith("Native")}
    levels_path = output / "native-levels.json"
    if levels_path.is_file():
        levels = json.loads(levels_path.read_text())
        report["levels"] = {k: v for k, v in levels.items() if not k.endswith("Native")}
    movement_path = output / "native-movement.json"
    if movement_path.is_file():
        movement = json.loads(movement_path.read_text())
        report["movement"] = {k: v for k, v in movement.items() if not k.endswith("Native")}
    fuel_path = output / "native-fuel.json"
    if fuel_path.is_file():
        fuel = json.loads(fuel_path.read_text())
        report["fuel"] = {k: v for k, v in fuel.items() if not k.endswith("Native")}
    if args.fuel_gui and (report.get("fuel", {}).get("GUICases", 0) != 1 or report.get("fuel", {}).get("GUIDifferences") != 0):
        report["status"] = "FAIL"
        status = status or 1
    aux_path = output / "native-aux-pod.json"
    if aux_path.is_file():
        aux = json.loads(aux_path.read_text())
        report["aux_pod"] = {k: v for k, v in aux.items() if not k.endswith("Native")}
    if args.require_fuel_consumers:
        report["formal_fuel_consumers"] = compare_fuel_consumers(output)
        if consumers_status or report["formal_fuel_consumers"]["status"] != "PASS":
            report["status"] = "FAIL"
            status = status or consumers_status or 1
    if args.require_mixed_energy:
        report["formal_mixed_energy"] = compare_mixed_energy(output)
        if mixed_status or report["formal_mixed_energy"]["status"] != "PASS":
            report["status"] = "FAIL"
            status = status or mixed_status or 1
    if report.get("aux_pod", {}).get("FormalCases") != 63:
        report["status"] = "FAIL"
        status = status or 1
    if args.require_aux_pod and any(report.get("aux_pod", {}).get(k) != 0 for k in
            ("FormalFuelDifferences", "FormalKindDifferences", "FormalConsumedDifferences")):
        report["status"] = "FAIL"
        status = status or 1
    if report.get("movement", {}).get("FormalFuelCases", 0) < 1:
        report["status"] = "FAIL"
        status = status or 1
    if args.require_movement_fuel and (report.get("movement", {}).get("FormalFuelCases") != 18
                                     or report.get("movement", {}).get("FormalFuelDifferences") != 0
                                     or report.get("movement", {}).get("FormalMovementDifferences") != 0):
        report["status"] = "FAIL"
        status = status or 1
    if args.require_shield_levels and report.get("levels", {}).get("FormalShieldDifferences") != 0:
        report["status"] = "FAIL"
        status = status or 1
    if args.require_weapon_levels and (report.get("levels", {}).get("FormalWeaponCases", 0) < 1 or
                                      report.get("levels", {}).get("FormalWeaponChargeDifferences") != 0 or
                                      report.get("levels", {}).get("FormalWeaponFuelDifferences") != 0):
        report["status"] = "FAIL"
        status = status or 1
    if args.shield_gui:
        report["shield_gui_input"] = {"path": str(args.shield_gui), "sha256": sha(args.shield_gui)}
        if report.get("levels", {}).get("GUIShieldCases", 0) < 1 or report.get("levels", {}).get("GUIShieldDifferences") != 0:
            report["status"] = "FAIL"
            status = status or 1
    if args.weapon_gui:
        report["weapon_gui_input"] = {"path": str(args.weapon_gui), "sha256": sha(args.weapon_gui)}
        if report.get("levels", {}).get("GUIWeaponCases", 0) < 1 or report.get("levels", {}).get("GUIWeaponDifferences") != 0:
            report["status"] = "FAIL"
            status = status or 1
    if args.require_mode5_drift and report.get("mode5_drift", {}).get("Differences") != 0:
        report["status"] = "FAIL"
        status = status or 1
    if args.require_initial_cooldown and (
            report.get("encounter_init", {}).get("FormalCooldownDifferences") != 0 or
            report.get("init_position", {}).get("Differences") != 0):
        report["status"] = "FAIL"
        status = status or 1
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")
    print(f"原生排程研究 {report['status']}：{output / 'receipt.json'}")
    if status:
        raise RuntimeError("原生排程研究未通過，見保存收據")


if __name__ == "__main__":
    main()
