"""Docker內核對原版倉庫Store／Fetch／Swap與city record讀寫；入口見Banville規格。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

from verify_talos_ground_collision import run_logged


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def verify_warehouse_backlinks(repo, disk):
    index_path = repo / "docs/atari-st/banville-state-evidence-20260908.json"
    index = json.loads(index_path.read_text())
    rows = index.get("warehouseTransferResolutions", [])
    if len(rows) != 1 or rows[0]["id"] != "warehouse.true-store-fetch":
        raise RuntimeError("倉庫consumer勘誤回鏈缺項")
    expected = {("0x31f", "0x331"), ("0x484", "0x496"), ("0x58e", "0x5e6"), ("0x741", "0x814"), ("0x15ca", "0x15cc")}
    row = rows[0]
    if {(o["start"], o["endExclusive"]) for o in row["origins"]} != expected:
        raise RuntimeError("倉庫consumer原始定位缺項")
    spec = (repo / row["evidenceSpec"]["path"]).read_text()
    spec_addresses = {int(v, 16) for v in re.findall(r"0x[0-9a-fA-F]+", spec)}
    for origin in row["origins"]:
        if origin["segment"] != "XDOTRADI" or origin["diskBase"] != "0x44200" or int(origin["start"], 16) not in spec_addresses:
            raise RuntimeError("倉庫spec原始位址不符")
        matches = [s for s in index["slices"] if all(s[k] == origin[k] for k in origin)]
        if len(matches) != 1:
            raise RuntimeError("倉庫consumer缺唯一切片")
        s = matches[0]
        base, first, end = (int(s[k], 16) for k in ("diskBase", "start", "endExclusive"))
        if s.get("evidenceLevel") != "proven" or hashlib.sha256(disk[base+first:base+end]).hexdigest() != s["sha256"]:
            raise RuntimeError("倉庫consumer位元組或證據等級不符")
    for ref in [row["evidenceSpec"], *row["olderSpecs"]]:
        if ref["marker"] not in (repo / ref["path"]).read_text():
            raise RuntimeError(f"倉庫勘誤未回填：{ref['path']}")
    swaps = index.get("warehouseSwapResolutions", [])
    expected_swap = {("0x729", "0x741"), ("0x667", "0x6ab"), ("0x83e", "0x849")}
    if len(swaps) != 1 or swaps[0]["id"] != "warehouse.swap-picker" or {(o["start"], o["endExclusive"]) for o in swaps[0]["origins"]} != expected_swap:
        raise RuntimeError("Swap picker／父選單勘誤回鏈缺項")
    for origin in swaps[0]["origins"]:
        matches = [s for s in index["slices"] if all(s.get(k) == v for k, v in origin.items())]
        if len(matches) != 1 or origin["segment"] != "XDOTRADI" or origin["diskBase"] != "0x44200":
            raise RuntimeError("Swap原始定位缺唯一切片")
        s = matches[0]
        first, end = int(s["start"], 16), int(s["endExclusive"], 16)
        if s.get("evidenceLevel") != "proven" or hashlib.sha256(disk[0x44200+first:0x44200+end]).hexdigest() != s["sha256"]:
            raise RuntimeError("Swap位元組或證據等級不符")
    for ref in [swaps[0]["evidenceSpec"], *swaps[0]["olderSpecs"]]:
        if ref["marker"] not in (repo / ref["path"]).read_text():
            raise RuntimeError(f"Swap勘誤未回填：{ref['path']}")


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk",type=Path,required=True)
    parser.add_argument("--talos",type=Path,required=True)
    parser.add_argument("--output",type=Path,required=True)
    args=parser.parse_args()
    if not Path("/.dockerenv").is_file():parser.error("只能在Docker執行")
    repo=Path(__file__).resolve().parents[1]
    disk,talos=(p.resolve(strict=True) for p in (args.disk,args.talos))
    out=args.output.resolve();interp=talos/"workplace/sundog/SYSTEM.INTERP"
    if sha(disk)!="f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":parser.error("磁碟身分不符")
    if sha(interp)!="a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":parser.error("直譯器身分不符")
    verify_warehouse_backlinks(repo, disk.read_bytes())
    if out.exists() or out.parent.stat().st_uid!=os.getuid():parser.error("輸出須全新且父目錄擁有者正確")
    version=subprocess.check_output(["go","version"],text=True).strip()
    if version.split()[2]!="go1.25.12":parser.error("須沿用Go1.25.12")
    names=("combat_aim","combat_cadence","combat_course","combat_init","combat_parent","combat_display","combat_levels",
           "combat_movement","combat_fuel","fuel_depot","fuel_entry","walker_slices","banville","banville_price","banville_site","trade_classes","warehouse_transfer")
    sources=[repo/f"tools/talos_{n}_test.go.txt" for n in names]
    assert all(p.is_file() and p.stat().st_uid==os.getuid() for p in sources)
    out.mkdir(mode=0o700);overlay=out/"overlay.json"
    overlay.write_text(json.dumps({"Replace":{str(talos/f"internal/m68k/sundog_{n}_test.go"):str(p) for n,p in zip(names,sources)}}))
    env=dict(os.environ,SUNDOG_PARITY_DISK=str(disk),SUNDOG_PARITY_OUTPUT=str(out),TALOS_UCSD_INTERP=str(interp.parent),GOPROXY="off",GOTOOLCHAIN="local")
    command=["go","test","-overlay",str(overlay),"-buildvcs=false","./internal/m68k","-run","^TestSunDogWarehouse(Transfer|Persistence|Picker)$","-count=1","-v"]
    run_logged(command,out/"native.log",cwd=talos,env=env)
    native=json.loads((out/"native-warehouse-transfer.json").read_text())
    persistence=json.loads((out/"native-warehouse-persistence.json").read_text())
    picker=json.loads((out/"native-warehouse-picker.json").read_text())
    if len(native["Rows"])!=1152 or {r["Mode"] for r in native["Rows"]}!={1,2,3}:raise RuntimeError("原生操作覆蓋不符")
    if len(persistence["Rows"])!=64 or {r["City"] for r in persistence["Rows"]}!=set(range(1,65)):raise RuntimeError("倉庫索引覆蓋不符")
    formal_command=["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosWarehouseTransfer$","-count=1","-v"]
    run_logged(formal_command,out/"formal.log",cwd=repo,env=env)
    formal=json.loads((out/"remake-warehouse-transfer.json").read_text())
    if len(picker["Rows"])!=92 or sum(r["InputCalls"]==2 for r in picker["Rows"])!=36:raise RuntimeError("原版picker資格／重試覆蓋不符")
    if (formal["StoreCases"],formal["FetchCases"],formal["SwapCases"],formal["Differences"],formal["SwapUnsupported"],formal["StoreDifferences"],formal["FetchDifferences"],formal["SwapDifferences"])!=(384,384,384,0,0,0,0,0):raise RuntimeError(f"正式Store／Fetch／Swap比較不符：{formal}")
    report=dict(status="PASS-store-fetch-swap-rules-and-picker",specState="READY-GUI-save-pending",originalGUI="unverified",diskPersistence="unverified",
                native=native,persistence=persistence,picker=picker,formal=formal,diskSHA256=sha(disk),interpreterSHA256=sha(interp),
                go=version,talosHead=subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),command=command,formalCommand=formal_command,
                sources={str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve(),repo/"internal/game/warehouse_native_parity_test.go",repo/"internal/game/exchange.go",repo/"internal/game/hold.go",repo/"internal/game/cargo.go",repo/"internal/game/state.go",repo/"internal/game/reducer.go",repo/"internal/game/trade_session.go",repo/"docs/atari-st/banville-state-evidence-20260908.json"]})
    (out/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print("PASS:原生1152搬運／64城市讀寫／92picker；正式Store384＋Fetch384＋Swap384零差異；完整原版GUI未驗")


if __name__=="__main__":main()
