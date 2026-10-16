"""Docker內重生冷凍人倉庫初始化；入口見Banville規格冷凍人來源段。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_parent_backlinks(repo, data):
    index = json.loads((repo/"docs/atari-st/banville-state-evidence-20260908.json").read_text())
    rows = index.get("cryoParentResolutions", [])
    if len(rows) != 1 or len(rows[0]["origins"]) != 9 or len(rows[0]["procedures"]) != 8:
        raise RuntimeError("冷凍人父流程回鏈缺項")
    row = rows[0]
    spec = (repo/row["evidenceSpec"]["path"]).read_text()
    addresses = {int(v,16) for v in re.findall(r"0x[0-9a-fA-F]+",spec)}
    for origin in row["origins"]:
        if (origin["segment"],origin["diskBase"]) not in {("XSTARTUP","0x39c00"),("XDOTRADI","0x44200")} or int(origin["start"],16) not in addresses:
            raise RuntimeError("父流程spec原始位址不符")
        matches = [s for s in index["slices"] if all(s[k]==v for k,v in origin.items())]
        if len(matches) != 1:
            raise RuntimeError("父流程回鏈缺唯一切片")
        s = matches[0]
        base,first,end = (int(s[k],16) for k in ("diskBase","start","endExclusive"))
        if s.get("evidenceLevel") != "proven" or hashlib.sha256(data[base+first:base+end]).hexdigest() != s["sha256"]:
            raise RuntimeError("父流程位元組或證據等級不符")
    for p in row["procedures"]:
        base,entry,local = (int(p[k],16) for k in ("diskBase","entry","locals"))
        if data[base+entry:base+entry+2].hex() != p["entryBytes"] or int(p["entryBytes"],16)*2 != local or int.from_bytes(data[base+local-2:base+local],"big") != int(p["returnInstruction"],16):
            raise RuntimeError("父流程原始程序字典不符")
    for ref in [row["evidenceSpec"],*row["olderSpecs"]]:
        if ref["marker"] not in (repo/ref["path"]).read_text():
            raise RuntimeError("冷凍人父流程勘誤未回填")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--ida-evidence", type=Path, required=True)
    parser.add_argument("--parent-chain", action="store_true", help="接原始行星初始化與17批倉庫載入／Fetch／全域表寫回；原版GUI／磁碟I/O另列未知")
    parser.add_argument("--compare-remake", action="store_true", help="配合--parent-chain，對拍正式228組初始化與17批Fetch；正常GUI另驗")
    args = parser.parse_args()
    if args.compare_remake and not args.parent_chain:
        parser.error("正式初始化對拍須配合--parent-chain")
    if not Path("/.dockerenv").is_file():
        parser.error("此工具只能在Docker執行")
    repo = Path(__file__).resolve().parents[1]
    disk, talos, ida = (p.resolve(strict=True) for p in (args.disk, args.talos, args.ida_evidence))
    output = args.output.resolve()
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("磁碟身分不符")
    if sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("直譯器身分不符")
    if args.parent_chain:
        verify_parent_backlinks(repo, disk.read_bytes())
    ida_result = json.loads(ida.read_text())
    if ida_result["version"] != "9.4" or ida_result["input_sha256"] != sha(interp) or ida_result["addressSpace"] != "SYSTEM.INTERP binary base0":
        parser.error("IDA證據身分／位址空間不符")
    raw = interp.read_bytes()
    for r in ida_result["rows"]:
        at = int(r["address"], 16)
        value = bytes.fromhex(r["bytes"])
        if raw[at:at+len(value)] != value:
            parser.error("IDA原始bytes不符")
    if not any(r["label"] == "stack-check" and r["address"] == "0x3a0" for r in ida_result["rows"]):
        parser.error("缺少SRS stack guard證據")
    if output.exists() or output.parent.stat().st_uid != os.getuid():
        parser.error("輸出必須全新且父目錄擁有者正確")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用Go1.25.12")
    names = ("combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent", "combat_display",
             "combat_levels", "combat_movement", "combat_fuel", "fuel_depot", "fuel_entry", "walker_slices",
             "banville", "banville_price", "banville_site", "trade_classes", "cryo_source")
    if args.parent_chain:
        names += ("cryo_parent",)
    sources = [repo / f"tools/talos_{name}_test.go.txt" for name in names]
    assert all(p.is_file() and p.stat().st_uid == os.getuid() for p in sources)
    output.mkdir(mode=0o700)
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(talos / f"internal/m68k/sundog_{name}_test.go"):str(path)
        for name,path in zip(names,sources)}}))
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
               "-run", "^TestSunDogCryoSourceInitialization$", "-count=1", "-v"]
    run_logged(command, output / "native.log", cwd=talos, env=env)
    native = json.loads((output / "native-cryo-source.json").read_text())
    if len(native["Rows"]) != 57 or {r["Initial"] for r in native["Rows"]} != set(range(1,58)):
        raise RuntimeError("受控起點覆蓋不符")
    # 獨立研究模型從world原始bytes推導排除範圍，再比較原生輸出。
    data = disk.read_bytes()
    system_first = int.from_bytes(data[0x5800+2*20+10:0x5800+2*20+12], "big") >> 2 & 31
    layout_at = 0x58f0 + (system_first-1)*16 + 14
    layout = int.from_bytes(data[layout_at:layout_at+2], "big")
    first, last = layout >> 9 & 63, (layout >> 9 & 63) + (layout & 7)-1
    differences = 0
    for r in native["Rows"]:
        cities, current, calls = {last}, r["Initial"], 0
        while len(cities) < 17:
            current = (current+1) % 57 + 1
            calls += 1
            if current not in cities and not first <= current <= last:
                cities.add(current)
        expected = [dict(City=c,Slot=1,Goods=31,Quality=7,Source=c,Price=0) for c in sorted(cities)]
        differences += r["Lots"] != expected or r["RandomCalls"] != calls or (r["FixedCity"],r["ExcludedFirst"],r["ExcludedLast"]) != (last,first,last) or r["CopyLengths"] != [32,1024]
    if differences:
        raise RuntimeError(f"獨立受控模型差異{differences}")
    report = dict(status="PASS-native-controlled-cryo-initialization", cases=57, records=969,
        differences=0, specState="DRAFT", formalRuntime="unchanged", originalGUI="unverified",
        normalFetch="unverified", sourceCityFirst=first, sourceCityLast=last,
        randomCondition="執行前固定range起點1..57；每組RND回0。原始SRS/INN執行；不宣稱自然骰序。",
        native=native, ida=ida_result, diskSHA256=sha(disk), interpreterSHA256=sha(interp),
        talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
        go=version, command=command, idaSource=dict(path=str(ida),sha256=sha(ida)),
        sources={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),repo/"tools/cryo_source_interp_export.py",repo/"tools/psys_disasm.py"]})
    if args.parent_chain:
        parent_command = command.copy()
        parent_command[parent_command.index("-run")+1] = "^TestSunDogCryo(ParentFields|WarehouseRoundtrip|SourceVariedInitialization)$"
        run_logged(parent_command, output / "parent.log", cwd=talos, env=env)
        parent = json.loads((output / "native-cryo-parent-fields.json").read_text())
        chain = json.loads((output / "native-cryo-warehouse-roundtrip.json").read_text())
        if len(parent["Systems"]) != 12 or len(parent["Planets"]) != 256 or {r["Initial"] for r in parent["Planets"]} != set(range(256)):
            raise RuntimeError("父流程欄位覆蓋不符")
        for row in parent["Systems"]:
            if row["After"] != row["Before"] & ~0x80 or row["TailAfter"] != row["TailBefore"] & 0xfff:
                raise RuntimeError("系統來源欄位改變")
        for row in parent["Planets"]:
            expected = [[w[0] & 0xff00 | (row["Initial"]+16*(i+1)) & 255, w[1] & 0x7fff] for i,w in enumerate(row["Before"])]
            if row["After"] != expected or row["RandomCalls"] != 37:
                raise RuntimeError("行星來源欄位／受控拒絕分支不同")
        expected_lots = native["Rows"][0]["Lots"]
        if len(chain["Rows"]) != 17 or {r["City"] for r in chain["Rows"]} != {r["City"] for r in expected_lots}:
            raise RuntimeError("冷凍人倉庫鏈覆蓋不符")
        for row in chain["Rows"]:
            if (row["Goods"],row["Quality"],row["Source"],row["Price"],row["Destination"],row["OtherBytesPreserved"]) != (31,7,row["City"],0,1,3068):
                raise RuntimeError("冷凍人倉庫鏈record不同")
        varied = {}
        for random in range(1,4):
            varied[random] = json.loads((output/f"native-cryo-source-rnd-{random}.json").read_text())
            if len(varied[random]["Rows"]) != 57 or {r["Initial"] for r in varied[random]["Rows"]} != set(range(1,58)):
                raise RuntimeError("來源RND mod4覆蓋不足")
            for row in varied[random]["Rows"]:
                cities, current, calls = {last}, row["Initial"], 0
                while len(cities) < 17:
                    current = (current+random+1) % 57 + 1
                    calls += 1
                    if calls > 2000:
                        raise RuntimeError("來源模型未收斂")
                    if current not in cities and not first <= current <= last:
                        cities.add(current)
                expected = [dict(City=c,Slot=1,Goods=31,Quality=7,Source=c,Price=0) for c in sorted(cities)]
                if row["Lots"] != expected or row["RandomCalls"] != calls:
                    raise RuntimeError("來源RND mod4獨立模型不同")
        report.update(status="PASS-native-cryo-parent-and-warehouse-chain", specState="READY-content-initialization", fullAcquisitionSpec="DRAFT", parentFields=parent, warehouseChain=chain, variedSource=varied, allocationCases=228, allocationRecords=3876, parentCommand=parent_command, fullNewGame="unverified", diskPersistence="unverified")
        report["sources"]["docs/atari-st/banville-state-evidence-20260908.json"] = sha(repo/"docs/atari-st/banville-state-evidence-20260908.json")
    if args.compare_remake:
        remake_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run", "^TestExportTalosCryoInitialization$", "-count=1", "-v"]
        run_logged(remake_command, output / "remake.log", cwd=repo, env=env)
        remake = json.loads((output / "remake-cryo-source.json").read_text())
        if (remake["Cases"], remake["Records"], remake["Differences"], remake["FetchCases"], remake["FetchDifferences"]) != (228,3876,0,17,0):
            raise RuntimeError("正式冷凍人初始化／Fetch對拍不符")
        report.update(status="PASS-controlled-original-remake-cryo-initialization", formalRuntime="implemented-schema41", remake=remake, remakeCommand=remake_command)
        for name in ["internal/game/cryo_source.go", "internal/game/cryo_source_test.go", "internal/game/cryo_source_native_parity_test.go", "internal/game/state.go", "internal/game/ground_route.go", "internal/game/ground_route_test.go", "internal/storage/save.go", "internal/storage/current_state.go", "internal/storage/save_test.go", "docs/atari-st/city-maps.json", "docs/atari-st/ship-rules.json"]:
            report["sources"][name] = sha(repo/name)
    (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    if args.compare_remake:
        print("PASS:正式228配置／3876record、17批Fetch零差異；原版12系統／256行星／17批寫回重載保持。正常GUI另驗。")
    elif args.parent_chain:
        print("PASS:12系統旗標／256行星初始化；228來源初態／3876record；17批全域倉庫Fetch／寫回重載。正式初始化／原版GUI／磁碟I/O仍待。")
    else:
        print("PASS:57受控起點、969冷凍人倉庫record；正式初始化／Fetch／GUI仍待驗")


if __name__ == "__main__":
    main()
