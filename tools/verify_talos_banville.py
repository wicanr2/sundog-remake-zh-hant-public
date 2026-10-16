"""Docker 內重生 Banville 原生研究收據；入口見 banville-restoration-spec.md。"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_class_backlinks(repo, disk_bytes):
    index = json.loads((repo / "docs/atari-st/banville-state-evidence-20260908.json").read_text())
    rows = index.get("tradeClassResolutions", [])
    if {r["id"] for r in rows} != {"trade.class-shift", "trade.class-demand-goods"}:
        raise RuntimeError("分類勘誤回鏈缺項")
    for row in rows:
        for origin in row["origins"]:
            matches = [s for s in index["slices"] if all(s[k] == origin[k] for k in ("segment", "diskBase", "start", "endExclusive"))]
            if len(matches) != 1:
                raise RuntimeError("分類回鏈缺唯一原始定位")
            s = matches[0]
            base, first, end = (int(s[k], 16) for k in ("diskBase", "start", "endExclusive"))
            if hashlib.sha256(disk_bytes[base+first:base+end]).hexdigest() != s["sha256"] or s.get("evidenceLevel") != "proven":
                raise RuntimeError("分類回鏈位元組或證據等級不符")
        for ref in [row["evidenceSpec"], *row["olderSpecs"]]:
            if ref["marker"] not in (repo / ref["path"]).read_text():
                raise RuntimeError(f"分類勘誤回鏈未回填：{ref['path']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--price-only", action="store_true", help="只研究完整原版估值與Banville累積caller")
    parser.add_argument("--persistence-only", action="store_true", help="研究新局成交累積清零、兩byte讀寫與保存後階段付款")
    parser.add_argument("--delivery-only", action="store_true", help="重生確認、初訪、累積與交付，並比較正式typed交易核心")
    parser.add_argument("--require-delivery-core", action="store_true", help="配合--delivery-only，要求72交付／64確認／62資格零差異")
    parser.add_argument("--confirm-only", action="store_true", help="研究Banville交付確認、撤回與無需求Exit-only")
    parser.add_argument("--fixed-price-only", action="store_true", help="研究Banville原始分類選表及與活躍市場不同的正式估值")
    parser.add_argument("--clear-only", action="store_true", help="研究原始售出後的六槽清空與正式pod入口")
    parser.add_argument("--require-clear-core", action="store_true", help="配合--clear-only，要求正式pod售出清空零差異")
    parser.add_argument("--require-fixed-price", action="store_true", help="配合--fixed-price-only，要求正式固定分類估值零差異")
    parser.add_argument("--records-only", action="store_true", help="只研究原版貨物記錄、來源定位與同一session資金")
    parser.add_argument("--site-only", action="store_true", help="研究完整原版首次選址與重畫不重抽")
    parser.add_argument("--require-site-core", action="store_true", help="配合--site-only，要求正式960資格／候選及四真實地表出城零差異")
    parser.add_argument("--growth-only", action="store_true", help="研究原版城市尺寸接到地形／建築寫圖遮罩")
    parser.add_argument("--require-growth-core", action="store_true", help="配合--growth-only，要求2640正式遮罩輸入零差異")
    parser.add_argument("--classes-only", action="store_true", help="研究原版分類位移初始化及兩項行星分類覆寫")
    parser.add_argument("--require-class-core", action="store_true", help="配合--classes-only，要求正式324分類／1536初始化零差異")
    parser.add_argument("--compare-price", action="store_true", help="匯出並比較正式已支援價格核心")
    parser.add_argument("--require-price-core", action="store_true", help="要求675案價格零差異；Banville任意供需只驗共用算術，固定分類另驗")
    parser.add_argument("--require-trade-core", action="store_true", help="配合--records-only，要求正式貨物／定位及資金比較通過")
    parser.add_argument("--tube-only", action="store_true", help="原生Tube選城與車資record讀取前綴，正式六案清單／32讀取比較")
    parser.add_argument("--require-tube-core", action="store_true", help="配合--tube-only，要求正式清單與車資讀取零差異")
    parser.add_argument("--marquee-only", action="store_true", help="原生庫存點號填充、22字環與完整／增量繪字參數；不含圖形 primitive")
    parser.add_argument("--buy-slots-only", action="store_true", help="原生購買容量、目的地分派及批准後扣款；輸入與圖形為明示邊界，不含完整購買GUI")
    parser.add_argument("--require-buy-core", action="store_true", help="配合--buy-slots-only，要求正式容量／目的地／付款零差異")
    parser.add_argument("--normal-delivery-input", type=Path, help="目前schema正常需求pod確認保存，作原生受控初態；40／41只供歷史研究回驗，不是玩家讀檔或遷移")
    parser.add_argument("--normal-delivery-after", type=Path, help="同一正常GUI交付後保存，逐項比較原生估值／需求／累積／資金／score")
    parser.add_argument("--normal-trade-input", type=Path, help="目前schema普通城市正常售出確認保存")
    parser.add_argument("--normal-trade-after", type=Path, help="同一正常售出批准後保存")
    parser.add_argument("--normal-trade-settled", type=Path, help="同一交易離開後正常地表保存")
    args = parser.parse_args()
    trade_paths = (args.normal_trade_input, args.normal_trade_after, args.normal_trade_settled)
    if any(trade_paths) and not all(trade_paths):
        parser.error("普通交易原生比較需要確認、批准及結算三份保存")
    if args.normal_trade_input and any(v for k,v in vars(args).items() if k not in ("normal_trade_input","normal_trade_after","normal_trade_settled","disk","talos","output")):
        parser.error("普通交易原生比較請使用獨立的新輸出目錄")
    if bool(args.normal_delivery_input) != bool(args.normal_delivery_after):
        parser.error("正常交付需要確認及交付後兩份保存")
    if args.normal_delivery_input and any(v for k,v in vars(args).items() if k not in ("normal_delivery_input","normal_delivery_after","disk","talos","output")):
        parser.error("正常交付原生比較請使用獨立的新輸出目錄")
    if args.require_buy_core and not args.buy_slots_only:
        parser.error("--require-buy-core須配合--buy-slots-only")
    if args.buy_slots_only and any(v for k,v in vars(args).items() if k not in ("buy_slots_only","require_buy_core","disk","talos","output")):
        parser.error("購買目的槽研究請使用獨立的新輸出目錄")
    if args.marquee_only and any(v for k,v in vars(args).items() if k not in ("marquee_only","disk","talos","output")):
        parser.error("走馬燈研究請使用獨立的新輸出目錄")
    if args.require_tube_core and not args.tube_only:
        parser.error("--require-tube-core須配合--tube-only")
    if args.tube_only and any(v for k,v in vars(args).items() if k not in ("tube_only","require_tube_core","disk","talos","output")):
        parser.error("Tube研究請使用獨立的新輸出目錄")
    if args.require_site_core and not args.site_only:
        parser.error("--require-site-core須配合--site-only")
    if args.require_growth_core and not args.growth_only:
        parser.error("--require-growth-core須配合--growth-only")
    if args.growth_only and any(v for k,v in vars(args).items() if k not in ("growth_only","require_growth_core","disk","talos","output")):
        parser.error("成長遮罩研究請使用獨立的新輸出目錄")
    if args.require_delivery_core and not args.delivery_only:
        parser.error("--require-delivery-core須配合--delivery-only")
    if args.delivery_only and any((args.confirm_only,args.persistence_only,args.price_only,args.fixed_price_only,args.clear_only,args.records_only,args.site_only,args.classes_only,args.compare_price,args.require_clear_core,args.require_fixed_price,args.require_class_core,args.require_price_core,args.require_trade_core)):
        parser.error("正式交付驗證請使用獨立的新輸出目錄")
    if args.confirm_only and any((args.persistence_only, args.price_only, args.fixed_price_only, args.clear_only, args.records_only, args.site_only, args.classes_only, args.compare_price, args.require_clear_core, args.require_fixed_price, args.require_class_core, args.require_price_core, args.require_trade_core)):
        parser.error("交付確認研究請使用獨立的新輸出目錄")
    if args.persistence_only and any((args.price_only, args.fixed_price_only, args.clear_only, args.records_only, args.site_only, args.classes_only, args.compare_price, args.require_clear_core, args.require_fixed_price, args.require_class_core, args.require_price_core, args.require_trade_core)):
        parser.error("累積保存研究請使用獨立的新輸出目錄")
    if args.require_clear_core and not args.clear_only:
        parser.error("--require-clear-core須配合--clear-only")
    if args.clear_only and (args.price_only or args.records_only or args.site_only or args.classes_only or args.fixed_price_only or args.require_price_core or args.require_trade_core or args.require_class_core or args.require_fixed_price):
        parser.error("售出清空研究請使用獨立的新輸出目錄")
    if args.require_fixed_price and not args.fixed_price_only:
        parser.error("--require-fixed-price須配合--fixed-price-only")
    if args.fixed_price_only and (args.price_only or args.records_only or args.site_only or args.classes_only or args.require_price_core or args.require_trade_core or args.require_class_core):
        parser.error("固定分類估值請使用獨立的新輸出目錄")
    if args.require_class_core and not args.classes_only:
        parser.error("--require-class-core須配合--classes-only")
    if args.require_trade_core and not args.records_only:
        parser.error("--require-trade-core須配合--records-only")
    if args.records_only and (args.price_only or args.compare_price or args.require_price_core):
        parser.error("記錄研究與價格研究請使用分別的新輸出目錄")
    if args.site_only and (args.records_only or args.price_only or args.compare_price or args.require_price_core or args.require_trade_core):
        parser.error("選址研究請使用獨立的新輸出目錄")
    if args.classes_only and (args.site_only or args.records_only or args.price_only or args.compare_price or args.require_price_core or args.require_trade_core):
        parser.error("分類研究請使用獨立的新輸出目錄")
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    repo = Path(__file__).resolve().parents[1]
    disk, talos, output = args.disk.resolve(strict=True), args.talos.resolve(strict=True), args.output.resolve()
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    if sha(disk) != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6":
        parser.error("磁碟身分不同")
    if sha(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("直譯器身分不同")
    verify_class_backlinks(repo, disk.read_bytes())
    if output.exists():
        parser.error("輸出須全新路徑，不覆寫歷史研究")
    if output.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄擁有者不符")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用 Go 1.25.12")
    sources = [repo / ("tools/talos_" + name + "_test.go.txt") for name in (
        "combat_aim", "combat_cadence", "combat_course", "combat_init", "combat_parent",
        "combat_display", "combat_levels", "combat_movement", "combat_fuel",
        "fuel_depot", "fuel_entry", "walker_slices", "banville", "banville_price", "banville_site", "trade_classes")]
    assert all(path.is_file() for path in sources)
    output.mkdir(mode=0o700)
    overlay = output / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {
        str(talos / "internal/m68k" / ("sundog_" + path.name.removeprefix("talos_").removesuffix(".txt"))): str(path)
        for path in sources}}))
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    if args.normal_delivery_input:
        args.normal_delivery_input = args.normal_delivery_input.resolve(strict=True)
        args.normal_delivery_after = args.normal_delivery_after.resolve(strict=True)
        schema = int(re.search(r"const CurrentSchema = (\d+)", (repo / "internal/game/state.go").read_text()).group(1))
        env.update(SUNDOG_NORMAL_DELIVERY_INPUT=str(args.normal_delivery_input),
                   SUNDOG_NORMAL_DELIVERY_AFTER=str(args.normal_delivery_after),
                   SUNDOG_NORMAL_DELIVERY_SCHEMA=str(schema))
    if args.normal_trade_input:
        args.normal_trade_input = args.normal_trade_input.resolve(strict=True)
        args.normal_trade_after = args.normal_trade_after.resolve(strict=True)
        args.normal_trade_settled = args.normal_trade_settled.resolve(strict=True)
        schema = int(re.search(r"const CurrentSchema = (\d+)", (repo / "internal/game/state.go").read_text()).group(1))
        trade_world = repo / "internal/source/atarist/assets/world-data.json"
        env.update(SUNDOG_NORMAL_TRADE_INPUT=str(args.normal_trade_input),
                   SUNDOG_NORMAL_TRADE_AFTER=str(args.normal_trade_after),
                   SUNDOG_NORMAL_TRADE_SETTLED=str(args.normal_trade_settled),
                   SUNDOG_NORMAL_TRADE_SCHEMA=str(schema), SUNDOG_NORMAL_TRADE_WORLD=str(trade_world))
    command = ["go", "test", "-overlay", str(overlay), "-buildvcs=false", "./internal/m68k",
               "-run", "^TestSunDogTrade(SlotClear|PickerRead|SaleSource)$" if args.clear_only else ("^TestSunDog(BanvilleFixedPrice|TradeMarketGoodsRange)$" if args.fixed_price_only else ("^TestSunDogTradeClass(es|Shift)$" if args.classes_only else ("^TestSunDogBanvilleSite(Lifecycle|PlayerEntry)?$" if args.site_only else ("^TestSunDogTrade(Records|Funds)$" if args.records_only else ("^TestSunDogBanvillePrice$" if args.price_only else "^TestSunDogBanville(Advance|Settlement|Price)$"))))), "-count=1", "-v"]
    if args.persistence_only:
        command[command.index("-run") + 1] = "^TestSunDogBanville(Persistence|Donation)$"
    if args.growth_only:
        command[command.index("-run") + 1] = "^TestSunDogBanvilleGrowthMask$"
    if args.confirm_only:
        command[command.index("-run") + 1] = "^TestSunDog(BanvilleConfirm|TradeSaleEligibility|BanvilleMarketReset)$"
    if args.delivery_only:
        command[command.index("-run")+1] = "^TestSunDog(Banville(Confirm|MarketReset|Persistence|Donation)|TradeSaleEligibility)$"
    if args.tube_only:
        command[command.index("-run")+1] = "^TestSunDogTubeDestinationRange$"
    if args.marquee_only:
        command[command.index("-run")+1] = "^TestSunDogTradeMarquee$"
    if args.buy_slots_only:
        command[command.index("-run")+1] = "^TestSunDogTradePurchase(Slots|Picker|Commit)$"
    if args.normal_delivery_input:
        command[command.index("-run")+1] = "^TestSunDogBanvilleNormalDelivery$"
    if args.normal_trade_input:
        command[command.index("-run")+1] = "^TestSunDogNormalMarketSale$"
    run_logged(command, output / "native.log", cwd=talos, env=env)
    if args.normal_trade_input:
        native = json.loads((output / "native-normal-market-sale.json").read_text())
        if native["Differences"] != 0:
            raise RuntimeError("正常普通交易原生比較有差異")
        report = {"status":"PASS-native-versus-normal-market-sale", "cases":1, "comparisonDimensions":6,
                  "originalGUI":"unverified", "quoteRefreshRND":"unexecuted",
                  "incomeConversion":"既有價格word×100契約；完整批准頁轉換未重跑", "native":native,
                  "normalInput":{"path":str(args.normal_trade_input),"sha256":sha(args.normal_trade_input)},
                  "normalAfter":{"path":str(args.normal_trade_after),"sha256":sha(args.normal_trade_after)},
                  "normalSettled":{"path":str(args.normal_trade_settled),"sha256":sha(args.normal_trade_settled)},
                  "currentSchema":schema, "worldDataSHA256":sha(trade_world),
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve()]}, "command":command}
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:實際普通交易估值／可用資金／現金／銀行／pod／warehouse六項原生比較；原版GUI及刷新RND未驗")
        return
    if args.normal_delivery_input:
        native = json.loads((output / "native-banville-normal-delivery.json").read_text())
        if native["Differences"] != 0:
            raise RuntimeError("正常交付原生比較有差異")
        report = {"status":"PASS-native-versus-normal-delivery", "cases":1, "comparisonDimensions":6,
                  "fullBanvilleSpec":"DRAFT", "originalGUI":"unverified", "native":native,
                  "normalInput":{"path":str(args.normal_delivery_input),"sha256":sha(args.normal_delivery_input)},
                  "normalAfter":{"path":str(args.normal_delivery_after),"sha256":sha(args.normal_delivery_after)},
                  "currentSchema":schema,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve()]}, "command":command}
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:實際正常貨物／分類／需求／累積／資金／score六項原生比較，完整原版GUI仍未驗")
        return
    if args.buy_slots_only:
        native = json.loads((output / "native-trade-purchase-slots.json").read_text())
        picker = json.loads((output / "native-trade-purchase-picker.json").read_text())
        commit = json.loads((output / "native-trade-purchase-commit.json").read_text())
        if len(commit["Rows"]) != 36:
            raise RuntimeError("購買批准／Pass／取消覆蓋不符")
        if len(picker["Rows"]) != 256:
            raise RuntimeError("購買目的地選項覆蓋不符")
        if len(native["Gates"]) != 128 or len(native["Placements"]) != 18 or len({(r["PodMode"],r["WarehouseMask"],r["PodMask"]) for r in native["Gates"]}) != 128:
            raise RuntimeError("購買資格／目的槽覆蓋不符")
        report = {"status":"PASS-native-purchase-slots-research", "purchaseSpec":"DRAFT", "originalGUI":"unverified", "native":native, "picker":picker, "commit":commit,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve()]}, "command":command}
        if args.require_buy_core:
            formal_command=["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosTradePurchase$","-count=1","-v"]
            run_logged(formal_command,output/"formal-buy.log",cwd=repo,env=env)
            formal=json.loads((output/"remake-trade-purchase.json").read_text())
            if tuple(formal[k] for k in ("CapacityCases","CapacityDifferences","PickerCases","PickerDifferences","CommitCases","CommitDifferences")) != (128,0,256,0,36,0):
                raise RuntimeError("正式購買核心比較不符")
            report.update(status="PASS-native-and-formal-purchase-core",purchaseSpec="READY",formal=formal,formalCommand=formal_command)
            for name in ("internal/game/exchange.go","internal/game/reducer.go","internal/game/state.go","internal/game/cargo.go","internal/game/trade_session.go","internal/game/trade_native_parity_test.go"):
                report["sources"][name]=sha(repo/name)
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:128購買容量／18目的槽MOV／256選槽／36批准時序；完整原版GUI仍未驗")
        return
    if args.marquee_only:
        native = json.loads((output / "native-trade-marquee.json").read_text())
        if len(native["Padding"]) != 176 or len(native["Draws"]) != 336 or len({(r["Cell"],r["Padding"]) for r in native["Padding"]}) != 176 or len({(r["Offset"],r["Full"]) for r in native["Draws"]}) != 336:
            raise RuntimeError("走馬燈填充／更新覆蓋不符")
        report = {"status":"PASS-native-marquee-parameters", "fullOriginalGUI":"unverified", "native":native,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve()]}, "command":command}
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:176點號writer／336走馬燈更新；不含原版圖形、計時或完整GUI")
        return
    if args.tube_only:
        native = json.loads((output / "native-tube-destination-range.json").read_text())
        if len(native["Rows"]) != 6 or {r["City"] for r in native["Rows"]} != {1,2,3,4,5,60} or len(native["Distances"]) != 32:
            raise RuntimeError("Tube六案清單／32車資讀取覆蓋不符")
        if any((r["First"],r["Count"],r["Kind"],len(r["Labels"])) != (1,5,1,4) or 60 in r["Labels"] for r in native["Rows"]):
            raise RuntimeError("Tube原始選城參數不符")
        report = {"status":"PASS-native-tube-destination-research", "fullBanvilleSpec":"DRAFT", "tube":native,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(p.relative_to(repo)):sha(p) for p in sources+[Path(__file__).resolve()]},
                  "command":command}
        if args.require_tube_core:
            formal_command=["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosTubeDestinations$","-count=1","-v"]
            run_logged(formal_command, output / "formal-tube.log", cwd=repo, env=env)
            formal=json.loads((output / "remake-tube-destination-range.json").read_text())
            if (formal["Cases"],formal["Differences"],formal["DistanceCases"],formal["DistanceDifferences"]) != (6,0,32,0):
                raise RuntimeError("正式Tube清單／車資讀取不符")
            report.update(status="PASS-native-and-formal-tube",formal=formal,formalCommand=formal_command)
            for name in ("internal/world/world.go","internal/game/tube.go","internal/game/tube_test.go","internal/game/colony.go"):
                report["sources"][name]=sha(repo/name)
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:Tube六案清單；32原始車資讀取；不含原版GUI／isqrt執行／transport")
        return
    if args.growth_only:
        growth = json.loads((output / "native-banville-growth-mask.json").read_text())
        rows = growth["Native"]
        keys = {(r["Colony"],r["Phase"],r["X"],r["Y"]) for r in rows}
        if len(rows) != 2640 or len(keys) != 2640:
            raise RuntimeError("成長寫圖覆蓋不符")
        report = {"status":"PASS-native-growth-mask-research", "fullBanvilleSpec":"DRAFT", "growth":growth,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(path.relative_to(repo)):sha(path) for path in sources+[Path(__file__).resolve()]},
                  "command":command}
        if args.require_growth_core:
            formal_command = ["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosColonyGrowth$","-count=1","-v"]
            run_logged(formal_command,output / "formal-growth.log",cwd=repo,env=env)
            formal = json.loads((output / "remake-banville-growth-mask.json").read_text())
            if (formal["Cases"],formal["Differences"]) != (2640,0):
                raise RuntimeError("正式成長遮罩覆蓋／比較不符")
            report.update(status="PASS-native-and-formal-growth-mask",formal=formal,formalCommand=formal_command)
            for name in ("internal/game/ground_route.go","internal/game/colony.go","internal/game/colony_test.go"):
                report["sources"][name]=sha(repo/name)
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:2640 unique growth-mask inputs;original terrain/building writers")
        return
    if args.delivery_only:
        native = json.loads((output / "native-banville-confirm.json").read_text())
        eligibility = json.loads((output / "native-trade-sale-eligibility.json").read_text())
        market = json.loads((output / "native-banville-market-reset.json").read_text())
        persistence = json.loads((output / "native-banville-persistence.json").read_text())
        donation = json.loads((output / "native-banville-donation.json").read_text())
        counts = (len(native["Native"]),len(eligibility["Native"]),len(market["Native"]),len(persistence["Init"]),len(persistence["Persistence"]),len(persistence["Phases"]),len(donation["Native"]))
        if counts != (32,62,8,4,16,160,72):
            raise RuntimeError("正式交付原始覆蓋不符")
        if len({(r["Needed"],r["Selection"],r["Abort"],r["Banville"]) for r in native["Native"]})!=32 or len({(r["Goods"],r["Banville"]) for r in eligibility["Native"]})!=62 or len({(r["Pattern"],r["Fresh"]) for r in market["Native"]})!=8:
            raise RuntimeError("交付初態不唯一")
        formal_command=["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosDeliveryIntegration$","-count=1","-v"]
        run_logged(formal_command, output/"formal.log",cwd=repo,env=env)
        formal=json.loads((output/"remake-banville-delivery.json").read_text())
        if (formal["Cases"],formal["ConfirmCases"],formal["EligibilityCases"])!=(72,64,62):
            raise RuntimeError("正式交付覆蓋不符")
        differences=(formal["Differences"],formal["ConfirmDifferences"],formal["EligibilityDifferences"])
        report={"status":"PASS-native-and-typed-delivery" if differences==(0,0,0) else "FAIL-typed-delivery",
                "fullBanvilleSpec":"DRAFT","nativeConfirm":native,"nativeEligibility":eligibility,
                "nativeMarketReset":market,"nativePersistence":persistence,"nativeDonation":donation,"formal":formal,
                "diskSHA256":sha(disk),"interpreterSHA256":sha(interp),"go":version,
                "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                "sources":{str(path.relative_to(repo)):sha(path) for path in sources+[Path(__file__).resolve()]},
                "command":command,"formalCommand":formal_command}
        for name in ("internal/game/state.go","internal/game/colony.go","internal/game/exchange.go","internal/game/trade_session.go","internal/game/reducer.go","internal/game/trade_native_parity_test.go","internal/storage/save.go","internal/storage/current_state.go"):
            report["sources"][name]=sha(repo/name)
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        if args.require_delivery_core and differences!=(0,0,0):
            raise RuntimeError("正式交付仍有差異")
        print("PASS:32 native confirmation,62 eligibility,8 market init,4/16/160 persistence,72 donation;typed72/64/62 zero differences")
        return
    if args.confirm_only:
        native = json.loads((output / "native-banville-confirm.json").read_text())
        if len(native["Native"]) != 32 or len({(r["Needed"], r["Selection"], r["Abort"], r["Banville"]) for r in native["Native"]}) != 32:
            raise RuntimeError("原生確認分支覆蓋不符")
        eligibility = json.loads((output / "native-trade-sale-eligibility.json").read_text())
        if len(eligibility["Native"]) != 62 or len({(r["Goods"], r["Banville"]) for r in eligibility["Native"]}) != 62:
            raise RuntimeError("原生出售資格覆蓋不符")
        report = {"status": "PASS-native-confirm", "fullBanvilleSpec": "DRAFT", "native": native,
                  "eligibility": eligibility, "marketReset": json.loads((output / "native-banville-market-reset.json").read_text()),
                  "diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
                  "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                  "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]}, "command": command}
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("PASS:32 original approval/cancel,62 eligibility cases;zero-demand Exit-only;semantic input boundary")
        return
    if args.persistence_only:
        native = json.loads((output / "native-banville-persistence.json").read_text())
        if (len(native["Init"]), len(native["Persistence"]), len(native["Phases"])) != (4, 16, 160):
            raise RuntimeError("累積保存原始覆蓋不符")
        if len({(r["StoredBefore"], r["Banville"]) for r in native["Persistence"]}) != 16:
            raise RuntimeError("兩byte保存初態不唯一")
        if len({(r["Next"], r["AccumulatorBefore"], r["Nonempty"]) for r in native["Phases"]}) != 160:
            raise RuntimeError("保存後階段付款初態不唯一")
        donation = json.loads((output / "native-banville-donation.json").read_text())
        if len(donation["Native"]) != 72 or len({(r["Goods"], r["Quality"], r["Next"], r["DemandBefore"], r["AccumulatorBefore"]) for r in donation["Native"]}) != 72:
            raise RuntimeError("交付caller初態覆蓋不符")
        report = {"status": "PASS-native-accumulator-persistence", "fullBanvilleSpec": "DRAFT", "native": native,
                  "donation": donation,
                  "diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
                  "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                  "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]},
                  "command": command}
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("PASS:4 new-game clear,16 two-byte persistence,160 load/payout/write,72 donation callers;formal state and GUI remain pending")
        return
    if args.clear_only:
        native=json.loads((output/"native-trade-slot-clear.json").read_text())
        if len(native["Native"]) != 18 or {r["Slot"] for r in native["Native"]} != set(range(1,7)):
            raise RuntimeError("原始六槽清空覆蓋不符")
        report={"status":"PASS-native-sale-clear", "fullBanvilleSpec":"DRAFT", "nativeClear":native,
                "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                "sources":{str(path.relative_to(repo)):sha(path) for path in sources+[Path(__file__).resolve()]}, "command":command}
        picker=json.loads((output/"native-trade-picker-read.json").read_text())
        source=json.loads((output/"native-trade-sale-source.json").read_text())
        if len(picker["Native"])!=18 or len(source["Native"])!=6:
            raise RuntimeError("來源picker覆蓋不符")
        report.update(nativePicker=picker,nativeSource=source)
        if args.compare_price or args.require_clear_core:
            formal_command=["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalosTradeSlotClear$","-count=1","-v"]
            run_logged(formal_command,output/"formal-clear.log",cwd=repo,env=env)
            formal=json.loads((output/"remake-trade-slot-clear.json").read_text())
            if (formal["Cases"],formal["WarehouseUnsupported"]) != (18,0):
                raise RuntimeError("正式售出原槽覆蓋不符")
            if formal["SourceCases"]!=6:
                raise RuntimeError("正式來源頁覆蓋不符")
            report.update(status="PASS-sale-clear" if not (formal["Differences"] or formal["SourceDifferences"]) else "FAIL-sale-clear",formalClear=formal,formalCommand=formal_command)
            for name in ("internal/game/exchange.go","internal/game/hold.go","internal/game/trade_native_parity_test.go"):
                report["sources"][name]=sha(repo/name)
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        if args.require_clear_core and (formal["Differences"] or formal["SourceDifferences"]):
            raise RuntimeError("正式售出原槽仍有差異")
        print("PASS:18 six-slot sale-clear,18 picker reads,6 source cases;independent transfers remain scoped")
        return
    if args.fixed_price_only:
        fixed = json.loads((output / "native-banville-fixed-price.json").read_text())
        keys = {json.dumps(row["Input"], sort_keys=True) for row in fixed["Native"]}
        if len(keys) != 2232 or len(fixed["Native"]) != 2232:
            raise RuntimeError("固定分類估值原始覆蓋不符")
        report = {"status":"PASS-native-fixed-price", "fullBanvilleSpec":"DRAFT", "fixedPrice":fixed,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(path.relative_to(repo)):sha(path) for path in sources+[Path(__file__).resolve()]}, "command":command}
        if args.compare_price or args.require_fixed_price:
            formal_command = ["go","test","-buildvcs=false","./internal/game","-run","^TestExportTalos(BanvilleFixedPrice|TradeMarketRange)$","-count=1","-v"]
            run_logged(formal_command, output/"formal-fixed-price.log", cwd=repo, env=env)
            formal = json.loads((output/"remake-banville-fixed-price.json").read_text())
            fkeys = {json.dumps(row["Input"],sort_keys=True):row for row in formal["Native"]}
            if set(fkeys) != keys or len(formal["Native"]) != 2232 or (formal["Cases"],formal["Unsupported"]) != (2232,0):
                raise RuntimeError("固定分類估值正式覆蓋不符")
            differences = sum(row["Price"] != fkeys[json.dumps(row["Input"],sort_keys=True)]["Price"] for row in fixed["Native"] if fkeys[json.dumps(row["Input"],sort_keys=True)]["Supported"])
            if differences != formal["Differences"]:
                raise RuntimeError("固定分類差異統計不符")
            report.update(status="PASS-fixed-price" if not differences else "FAIL-fixed-price",formalFixedPrice=formal,formalCommand=formal_command)
            market_range = json.loads((output/"native-trade-market-range.json").read_text())
            formal_range = json.loads((output/"remake-trade-market-range.json").read_text())
            if len(market_range["Native"]) != 24 or (formal_range["Cases"],formal_range["Differences"]) != (24,0):
                raise RuntimeError("市場第31格範圍驗證不符")
            report.update(marketRange=market_range,formalMarketRange=formal_range)
            for name in ("internal/game/exchange.go","internal/game/banville_price_parity_test.go","internal/world/trade_classes.go","internal/source/atarist/assets/trade-classes.json"):
                report["sources"][name]=sha(repo/name)
        (output/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        if args.require_fixed_price and formal["Differences"]:
            raise RuntimeError("正式固定分類估值仍有差異")
        print("PASS:2232 native fixed-class prices;all goods1..31 supported")
        return
    if args.site_only:
        site = json.loads((output / "native-banville-site.json").read_text())
        if len(site["Native"]) != 1218 or len({(r["Candidate"], r["Terrain"]) for r in site["Native"]}) != 1218 or any(row["FlagAfter"] or row["RedrawDraws"] for row in site["Native"]):
            raise RuntimeError("首次選址／重畫研究覆蓋不符")
        lifecycle = json.loads((output / "native-banville-site-lifecycle.json").read_text())
        for name, count, keys in (("Startup", 4, ("FlagsBefore",)),
                                  ("Modes", 32, ("Mode", "PendingBefore")),
                                  ("Planets", 36, ("Planet", "PendingBefore")),
                                  ("Looks", 514, ("Base", "Blank", "Random"))):
            rows = lifecycle[name]
            if len(rows) != count or len({tuple(row[key] for key in keys) for row in rows}) != count:
                raise RuntimeError("首次選址生命週期覆蓋不符：" + name)
        if any(row["RedrawDraws"] for row in lifecycle["Planets"]):
            raise RuntimeError("行星guard重畫又抽亂數")
        entry = json.loads((output / "native-banville-site-player-entry.json").read_text())
        for name, count, keys in (("Parents", 512, ("Current", "Next", "Reload")),
                                  ("Exits", 4, ("City", "PendingBefore")),
                                  ("SourceExits", 4, ("City", "PendingBefore")),
                                  ("Cells", 960, ("Index",)),
                                  ("Candidates", 960, ("Candidate",))):
            rows = entry[name]
            if len(rows) != count or len({tuple(row[key] for key in keys) for row in rows}) != count:
                raise RuntimeError("首次選址玩家入口覆蓋不符：" + name)
        if any(row["RedrawDraws"] for row in entry["Candidates"]):
            raise RuntimeError("真實地表重畫再抽亂數")
        report = {"status":"PASS-native-site-research", "fullBanvilleSpec":"DRAFT", "site":site,
                  "siteLifecycle":lifecycle,
                  "sitePlayerEntry":entry,
                  "diskSHA256":sha(disk), "interpreterSHA256":sha(interp), "go":version,
                  "talosHead":subprocess.check_output(["git","rev-parse","HEAD"],cwd=talos,text=True).strip(),
                  "sources":{str(path.relative_to(repo)):sha(path) for path in sources+[Path(__file__).resolve()]},
                  "command":command}
        if args.require_site_core:
            formal_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run", "^TestExportTalosColonySite(Eligibility|Selection)$", "-count=1", "-v"]
            run_logged(formal_command, output / "formal-site.log", cwd=repo, env=env)
            eligibility = json.loads((output / "remake-banville-site-eligibility.json").read_text())
            selection = json.loads((output / "remake-banville-site-selection.json").read_text())
            if (eligibility["Cases"], eligibility["Eligible"], eligibility["Occupied"], eligibility["Differences"], selection["Candidates"], selection["Exits"], selection["Differences"]) != (960,318,5,0,960,4,0):
                raise RuntimeError("正式首次選址覆蓋或比較不符")
            report.update(status="PASS-native-and-formal-site", formalEligibility=eligibility, formalSelection=selection, formalCommand=formal_command)
            for name in ("internal/game/colony.go", "internal/game/colony_test.go", "internal/game/ground_route.go", "internal/game/state.go"):
                report["sources"][name] = sha(repo / name)
        (output / "receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
        print("PASS:1218 site loops/redraws;4/32/36/514 lifecycle;512 parent transitions;4 exit chains;960 source cells/candidates;controlled RND")
        return
    if args.classes_only:
        classes = json.loads((output / "native-trade-classes.json").read_text())
        shift = json.loads((output / "native-trade-class-shift.json").read_text())
        if len(classes["Native"]) != 324 or len({(r["Planet"], r["Shift"]) for r in classes["Native"]}) != 324:
            raise RuntimeError("分類載入覆蓋不符")
        if len(shift["Native"]) != 1536 or len({(r["Counter"], r["Random"]) for r in shift["Native"]}) != 1536:
            raise RuntimeError("分類初始化覆蓋不符")
        report = {"status": "PASS-native-class-research", "fullBanvilleSpec": "DRAFT", "classes": classes, "shift": shift,
                  "diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
                  "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                  "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]}, "command": command}
        if args.require_class_core:
            formal_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run", "^TestExportTalosTradeClasses$", "-count=1", "-v"]
            run_logged(formal_command, output / "formal-classes.log", cwd=repo, env=env)
            formal = json.loads((output / "remake-trade-classes.json").read_text())
            if (formal["Cases"], formal["StartupCases"], formal["Differences"]) != (324,1536,0):
                raise RuntimeError("正式分類／初始化比較不符")
            report.update(status="PASS-native-and-formal-class-core", formalClasses=formal, formalCommand=formal_command)
            for name in ("internal/game/trade_native_parity_test.go", "internal/game/trade_session.go", "internal/game/state.go", "internal/game/exchange.go", "internal/world/trade_classes.go", "internal/source/atarist/trade_classes.go", "internal/source/atarist/assets/trade-classes.json"):
                report["sources"][name]=sha(repo/name)
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print("PASS:324 original class profiles;1536 startup counter/controlled-RND shifts")
        return
    if args.records_only:
        records = json.loads((output / "native-trade-records.json").read_text())
        funds = json.loads((output / "native-trade-funds.json").read_text())
        if len(records["Records"]) != 165 or len(records["Locations"]) != 256 or len(funds["Native"]) != 117:
            raise RuntimeError("貨物記錄／定位研究覆蓋不符")
        report = {"status": "PASS-native-trade-records-research", "fullBanvilleSpec": "DRAFT",
                  "diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
                  "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                  "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]},
                  "command": command, "records": records, "funds": funds}
        if args.require_trade_core:
            formal_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run",
                              "^TestExportTalosTrade(Records|Funds)$", "-count=1", "-v"]
            run_logged(formal_command, output / "formal-trade.log", cwd=repo, env=env)
            formal_records = json.loads((output / "remake-trade-records.json").read_text())
            formal_funds = json.loads((output / "remake-trade-funds.json").read_text())
            if (formal_records["Cases"], formal_records["Unsupported"], formal_records["Locations"]) != (163, 2, 256):
                raise RuntimeError("正式貨物／來源覆蓋不符")
            if formal_funds["cases"] != 117 or formal_funds["differences"] != 0:
                raise RuntimeError("正式資金覆蓋或比較不符")
            report.update(status="PASS-native-and-formal-trade-core", formalRecords=formal_records,
                          formalFunds=formal_funds, formalCommand=formal_command)
            for name in ("internal/game/trade_native_parity_test.go", "internal/game/trade_session.go",
                         "internal/game/exchange.go", "internal/game/hold.go", "internal/game/reducer.go",
                         "internal/game/state.go", "internal/world/world.go"):
                report["sources"][name] = sha(repo / name)
        (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(f"PASS:165 original lot records,256 locations,{len(funds['Native'])} financial sequences")
        return
    price = json.loads((output / "native-banville-price.json").read_text())
    pkeys = {json.dumps(row["Input"], sort_keys=True) for row in price["Native"]}
    if len(pkeys) != 675 or len(price["Native"]) != 675:
        raise RuntimeError("原生估值覆蓋不符")
    metadata = {"diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
                "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
                "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]},
                "command": command, "price": price}
    if args.compare_price or args.require_price_core:
        formal_command = ["go", "test", "-buildvcs=false", "./internal/game", "-run",
                          "^TestExportTalosBanvillePrice$", "-count=1", "-v"]
        run_logged(formal_command, output / "formal-price.log", cwd=repo, env=env)
        formal = json.loads((output / "remake-banville-price.json").read_text())
        fkeys = {json.dumps(row["Input"], sort_keys=True): row for row in formal["Native"]}
        if len(fkeys) != 675 or len(formal["Native"]) != 675 or set(fkeys) != pkeys:
            raise RuntimeError("正式估值輸入集合不符")
        differences, unsupported = [], []
        for row in price["Native"]:
            other = fkeys[json.dumps(row["Input"], sort_keys=True)]
            if not other["Supported"]:
                if not other.get("Reason") or other["Price"] != -1:
                    raise RuntimeError("未支援項目缺明示原因")
                unsupported.append(row["Input"])
            elif row["Price"] != other["Price"]:
                differences.append({"input": row["Input"], "native": row["Price"], "formal": other["Price"]})
        if len(unsupported) != 0 or formal["Cases"] != 675 or formal["Unsupported"] != 0:
            raise RuntimeError("正式價格核心覆蓋改變，需重新審查範圍")
        if formal["Differences"] != len(differences):
            raise RuntimeError("正式差異統計不符")
        metadata["formalPrice"] = {"cases": 675, "differences": differences,
                                  "unsupported": unsupported, "command": formal_command,
                                  "scope": "同供需研究輸入、來源城市與購入價、正式支援商品；一般城市為正式入口，Banville任意供需只驗共用算術helper，固定分類選擇由--fixed-price-only另驗；不是完整Banville保存驗收"}
        metadata["priceCoreStatus"] = "PASS" if not differences else "FAIL"
        for path in (repo / "internal/game/exchange.go", repo / "internal/game/banville_price_parity_test.go"):
            metadata["sources"][str(path.relative_to(repo))] = sha(path)
        if args.require_price_core and differences:
            (output / "receipt.json").write_text(json.dumps({"status": "FAIL-price-core", **metadata}, ensure_ascii=False, indent=2) + "\n")
            raise RuntimeError(f"正式價格核心仍有{len(differences)}案差異")
        print(f"formal price:675 cases, {len(differences)} differences;all goods1..31 supported")
    if args.price_only:
        (output / "receipt.json").write_text(json.dumps({"status": "PASS-native-price-research", "fullBanvilleSpec": "DRAFT", **metadata}, ensure_ascii=False, indent=2) + "\n")
        print("PASS: 675 native price observations; original Banville accumulation caller")
        return
    native = json.loads((output / "native-banville-advance.json").read_text())
    rows = native["Native"]
    keys = {(row["NextBefore"], row["AccumulatorBefore"], row["CashBefore"], row["NonzeroByte"]) for row in rows}
    if len(rows) != 334 or len(keys) != 334:
        raise RuntimeError("原生研究覆蓋不符")
    if {row["Phase"] for row in native["Extents"]} != set(range(256)) or len(native["Extents"]) != 256:
        raise RuntimeError("原生尺寸覆蓋不符")
    settlement = json.loads((output / "native-banville-settlement.json").read_text())
    skeys = {(row["BankBefore"], row["CashBefore"], row["Accumulator"], row["Cost"]) for row in settlement["Native"]}
    if len(skeys) != 75 or len(settlement["Native"]) != 75:
        raise RuntimeError("原生結算覆蓋不符")
    report = {"status": "PASS-native-banville-research", "productionMutationByVerifier": False,
              "fullBanvilleSpec": "DRAFT", "scope": native["Scope"], "scanBoundary": native["ScanBoundary"],
              "diskSHA256": sha(disk), "interpreterSHA256": sha(interp), "go": version,
              "talosHead": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=talos, text=True).strip(),
              "sources": {str(path.relative_to(repo)): sha(path) for path in sources + [Path(__file__).resolve()]},
              "command": command, "native": native, "settlement": settlement, **metadata}
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print("PASS: 334 native advance/payment, 256 extent and 75 settlement cases; SCAN/UI/end remain explicit boundaries")


if __name__ == "__main__":
    main()
