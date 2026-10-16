#!/usr/bin/env python3
"""Docker/Xvfb 正常玩家回歸；從正式保存庫續行，不注入遊戲狀態。

入口與範圍見 docs/atari-st/banville-restoration-spec.md 的城市成長遮罩。
需要 Docker 工具鏈中的 Xvfb、xdotool、Go，以及玩家自備 Atari ST 磁碟。
船上保存續行以目前正式Reducer重播實際VBL；離線Go快取沿用workplace/trade-marquee-research-r1-20261003/cache。
原版比較由 verify_talos_banville.py 執行；本工具只驗 remake 玩家鏈。
"""

import argparse
from collections import deque
import math
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import time


def verify_awake_resume(root, out, before, after, name):
    """Replay observed VBLs through the current reducer, including hour gates.

    This checks the normal GUI against existing native-verified rules. It
    does not execute the original GUI or claim new original clock parity.
    """
    ticks = (after["awakeClock"]["source"]["ticks"]-before["awakeClock"]["source"]["ticks"]) % 2**32
    assert 0 <= ticks < 600, "續行時計epoch或有界範圍不同"
    source = '''package main
import (
 "encoding/json"
 "os"
 "strconv"
 "github.com/wicanr2/sundog-remake/internal/game"
)
func main() {
 raw, err := os.ReadFile(os.Args[1]); if err != nil { panic(err) }
 var s game.State
 if err = json.Unmarshal(raw, &s); err != nil { panic(err) }
 ticks, err := strconv.Atoi(os.Args[2]); if err != nil { panic(err) }
 r, err := game.NewReducer(); if err != nil { panic(err) }
 for i := 0; i < ticks; i++ { s, err = r.Step(s, game.ActionVBLTick); if err != nil { panic(err) } }
 if err = json.NewEncoder(os.Stdout).Encode(s); err != nil { panic(err) }
}
'''
    replay = out/"resume-state.go"
    replay.write_text(source)
    input_path = out/(name+"-resume-before.json")
    input_path.write_text(json.dumps(before,ensure_ascii=False,indent=2)+"\n")
    cache = root/"workplace/trade-marquee-research-r1-20261003/cache"
    assert cache.is_dir(), "查無既有離線Go快取"
    env = dict(os.environ,GOCACHE=str(cache),GOPROXY="off",GOTOOLCHAIN="local")
    result = subprocess.run(["go","run",str(replay),str(input_path),str(ticks)],
        cwd=root,env=env,text=True,capture_output=True,check=True,timeout=120)
    expected = json.loads(result.stdout)
    fields = [k for k in before if k not in ["messageKey","saved","quitRequested"]]
    for field in fields:
        assert after.get(field) == expected.get(field), "續行初態不同："+field
    report = dict(advancedVBLs=ticks,crossedHours=after["timeMinutes"]//60-before["timeMinutes"]//60,
        fields=fields,expected=expected,replaySourceSHA256=hashlib.sha256(source.encode()).hexdigest(),
        beforeSHA256=hashlib.sha256(input_path.read_bytes()).hexdigest(),
        scope="目前正式Reducer按實際VBL重播完整初態，包含整點條件／屬性及船艦writer；既有原生規則證據支持，不新增原版GUI或此狀態原生整點重跑聲明")
    (out/(name+"-resume-expected.json")).write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    return report


def verify_main_refuel(menu, after):
    """Check a normal Buy Full / Close pair against the funded quantity.

    Existing fuel READY evidence allows partial purchases. This checks the
    actual GUI pair, and does not claim a new original native experiment.
    """
    p = menu["fuelPurchase"]
    assert menu["screen"] == "fuel-purchase" and p["kind"] == "main"
    assert p["unitPrice"] > 0 and 0 <= p["quantity"] <= 16
    purchased = min(16-p["quantity"], p["funds"]//p["unitPrice"])
    quantity = p["quantity"]+purchased
    expected_fuel = min(255, p["baseFuel"] % 16+16*quantity)
    funds = p["funds"]-purchased*p["unitPrice"]
    spent = max(0, p["initialBank"])+p["initialCash"]-funds
    bank_spend = min(max(0, p["initialBank"]), spent)
    assert after["screen"] == "ground" and after["fuel"] == expected_fuel, "預算限量購油讀數不同"
    assert after["bank"] == p["initialBank"]-bank_spend
    assert after["cash"] == p["initialCash"]-(spent-bank_spend)
    assert after["holds"] == menu["holds"] and after["warehouses"] == menu["warehouses"]
    assert not after["fuelPurchase"]["kind"]
    return dict(purchasedUnits=purchased, expectedFuel=expected_fuel, fundsAfter=funds,
                scope="正常買滿依實際可用資金限量；沿用既有購油READY，不新增原版原生重跑聲明")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["binary", "disk", "settings-input", "output"]:
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--library-input", type=Path)
    parser.add_argument("--ftl-service-only", action="store_true", help="承接正常Banville保存，驗FTL報價、資金不足、取消及保存重開；不注入資金或升級")
    parser.add_argument("--ftl-install", action="store_true", help="配合FTL服務，資金足額時正常確認安裝並驗航距9與保存；不注入資金")
    parser.add_argument("--stores-eat-only", action="store_true", help="正常Banville交易保存回船，食用既存Stores漢堡後回交易所，不注入食物")
    parser.add_argument("--stores-eat-resume", action="store_true", help="承接已正常食用後pod地面保存，只續行返交易所，不再食用")
    parser.add_argument("--site-only", action="store_true", help="正常新局出城選址與新程序重開，不讀舊保存")
    parser.add_argument("--site-input", choices=["keyboard", "mouse", "cell-keyboard"], default="keyboard")
    parser.add_argument("--site-consumers", action="store_true", help="正常選址保存續行，驗導航兩種畫法與Tube新址車資／到達／重開")
    parser.add_argument("--site-arrival", action="store_true", help="承接正常Obburg保存，搭車回船、購買並安裝掃描器、購油、抵達保存新城址及交易重開")
    parser.add_argument("--marquee-only", action="store_true", help="從正常交易保存續行，抽樣入口、購買子頁清除及返回重畫；不成交")
    parser.add_argument("--warehouse-only", action="store_true", help="正常目前schema交易保存存倉、兩次重開及取貨；入口見倉庫Store／Fetch窄規格")
    parser.add_argument("--condition-only", action="store_true", help="正常目前schema交易保存續行移動、讀數保持、單檔及保存庫重開；地面父時鐘另待核定")
    parser.add_argument("--purchase-only", action="store_true", help="正常新局步行存倉、pod批准選槽、付款後保存／重開及鍵鼠放貨；不讀舊保存")
    parser.add_argument("--purchase-resume", action="store_true", help="配合--purchase-only，從本輪正常目前schema步行交易保存續行")
    parser.add_argument("--phase0-delivery", action="store_true", help="承接目前schema正常pod交易保存，取得所需貨物、出城選址、設備／購油／飛航及Banville第一期部分交付")
    parser.add_argument("--phase0-supply", action="store_true", help="承接正常目前schema第一期pod交易保存，正常飛航至供應城市、購買現有需求貨物、回Banville交付及重開")
    parser.add_argument("--colony-supply", action="store_true", help="承接正常目前schema任一期pod保存，依該期實際需求供貨；已有貨物用--supply-return交付")
    parser.add_argument("--supply-city", default="lightning", help="正常導航目的城市ID")
    parser.add_argument("--supply-goods", default="8,13", help="此段實際尋找的當期貨種，逗號分隔")
    parser.add_argument("--supply-market-needed", action="store_true", help="到站後依實際八筆報價、當期需求、預算與空pod數選取額外貨物；價格由低至高，不重進交易所抽報價")
    parser.add_argument("--market-visit-only", action="store_true", help="正常飛到城市並讀取一次實際報價，保存Main頁，不購貨或返航；可空船回Banville")
    parser.add_argument("--market-refuel", action="store_true", help="配合市場造訪，到站後走正常depot補主油，再回交易所")
    parser.add_argument("--merchant-withdraw", action="store_true", help="一般交易成交後，正常走銀行領出本星系收入並回交易所；供下一段跨星系交易")
    parser.add_argument("--merchant-refuel", action="store_true", help="一般交易提款後，燃料低於180時正常depot補主油；無depot則先正常飛本行星第一城市")
    parser.add_argument("--supply-landed", action="store_true", help="承接同目的城市正常船艙、pod地面或交易保存；船艙以既有鍵序下船，續用實際庫存，不重飛")
    parser.add_argument("--supply-revisit", action="store_true", help="配合--supply-landed，從同城正常交易保存離開再進交易所一次，記錄實際報價更新")
    parser.add_argument("--supply-rest-only", action="store_true", help="配合--colony-supply，從同局交易保存正常回船睡至Rest255，驗睡眠中兩保存新程序續行，醒來後正常返回交易所；不買貨或飛航")
    parser.add_argument("--supply-eat-only", action="store_true", help="配合--colony-supply，從同局交易保存正常角色面板食用一個已攜漢堡，返回交易所並驗新程序續行")
    parser.add_argument("--supply-trade-only", action="store_true", help="配合--colony-supply，依目前報價買入--supply-goods一般貨物，正常到--supply-city售出並結算；不交殖民補給")
    parser.add_argument("--supply-funding-only", action="store_true", help="配合--colony-supply，正常到來源城市領出既有正銀行餘額，選用--supply-refuel-return補油並記錄報價；不買貨或交付")
    parser.add_argument("--supply-prep", action="store_true", help="供貨前正常回Drahew補主油箱、買一批備用燃料並以Atari ST銀行頁面取款")
    parser.add_argument("--supply-departure", action="store_true", help="從正常pod地面、地面對話、帶pod船上或貨艙保存續行出發；補油及提款另以明示旗標選用")
    parser.add_argument("--supply-return", action="store_true", help="承接同供應城市已正常買入需求貨物的pod／帶pod船艙或導航保存，正常返航交付")
    parser.add_argument("--supply-return-via", default="", help="供貨返航依序正常停靠的中繼城市ID，逗號分隔；不改寫航程或保存")
    parser.add_argument("--supply-reentry", action="store_true", help="配合--phase0-supply，從同局已正常交付的Banville保存只驗兩保存與分鐘相位")
    parser.add_argument("--locale", choices=["zh-Hant", "en", "zh-Hans", "ja"], default="zh-Hant")
    parser.add_argument("--cryo-newgame", action="store_true", help="正常目前schema新局17批、設備補油、旅行取貨及保存重開；入口見冷凍人READY規格")
    parser.add_argument("--cryo-resume", action="store_true", help="配合--cryo-newgame及同一正常pod登陸保存，續走交易所；不重開新局")
    parser.add_argument("--cryo-start-resume", action="store_true", help="配合--cryo-newgame，沿用同一目前schema的Drahew初局船艙保存，不重抽新局")
    parser.add_argument("--cryo-fetched-resume", action="store_true", help="配合冷凍人續行，從已正常Fetch保存只驗重開，不再次取貨")
    parser.add_argument("--colony-first-visit", action="store_true", help="同一目前schema已取得冷凍人的新局保存，正常返Drahew出城選址及首次Banville需求／保存重開")
    parser.add_argument("--colony-first-visit-resume", action="store_true", help="承接同一正常首訪後第一期31零需求確認頁，續驗禁止成交／撤回及保存重開")
    parser.add_argument("--supply-refuel-return", action="store_true", help="配合供貨返航，從實際購貨保存走來源depot補滿主油箱，再返航")
    parser.add_argument("--supply-refuel-before", action="store_true", help="供貨出發先正常飛Drahew，走來源depot只補主油箱，再飛目的城市")
    parser.add_argument("--supply-withdraw-before", action="store_true", help="供貨出發前正常離船提款，把當前星系餘額帶成現金")
    parser.add_argument("--supply-withdraw-return", action="store_true", help="取得貨物及補油後，返航前正常領出來源星系餘額；來源銀行須有實際存款")
    args = parser.parse_args()
    if args.ftl_install and not args.ftl_service_only:
        parser.error("FTL安裝須配合獨立FTL服務")
    if args.stores_eat_resume and not args.stores_eat_only:
        parser.error("已食用續行須配合--stores-eat-only")
    if args.stores_eat_only:
        if not args.library_input or args.colony_supply or args.ftl_service_only:
            parser.error("船上補給須獨立正常Banville保存")
        args.colony_supply, args.supply_city, args.supply_goods = True, "banville", ""
    if args.ftl_service_only:
        if not args.library_input or args.colony_supply:
            parser.error("FTL服務須獨立正常Banville保存")
        args.colony_supply, args.supply_city, args.supply_goods = True, "banville", ""
    if args.colony_supply:
        if args.phase0_supply:
            parser.error("第一期與各期供貨入口擇一")
        args.phase0_supply = True
    if args.market_visit_only and (not args.colony_supply or args.supply_goods.strip()):
        parser.error("市場造訪須各期正常保存及空貨種清單")
    if args.market_refuel and not args.market_visit_only:
        parser.error("到站補油須配合市場造訪")
    if args.merchant_withdraw and not args.supply_trade_only:
        parser.error("交易後提款須配合一般交易")
    if args.merchant_refuel and not args.merchant_withdraw:
        parser.error("交易後補油須先配合正常提款")
    if args.supply_market_needed and (not args.colony_supply or any([
            args.supply_reentry, args.supply_return, args.supply_funding_only,
            args.supply_rest_only, args.supply_eat_only, args.supply_trade_only])):
        parser.error("實際需求報價選取限正常各期採購，不併入其他驗證流程")
    if args.supply_return_via and (not args.colony_supply or args.supply_funding_only or args.supply_reentry or args.supply_rest_only):
        parser.error("返航中繼須正常各期供貨或已攜貨物返航，不併入提款或僅重開")
    if args.supply_eat_only and (not args.colony_supply or not args.library_input or any([
            args.supply_reentry, args.supply_return, args.supply_departure, args.supply_landed,
            args.supply_funding_only, args.supply_rest_only, args.supply_trade_only, args.supply_prep, args.supply_return_via,
            args.supply_refuel_before, args.supply_refuel_return, args.supply_withdraw_before,
            args.supply_withdraw_return])):
        parser.error("供貨進食須獨立--colony-supply正常交易保存，不併入其他流程")
    if args.supply_trade_only and (not args.colony_supply or not args.library_input or any([
            args.supply_reentry, args.supply_return, args.supply_departure, args.supply_landed,
            args.supply_funding_only, args.supply_rest_only, args.supply_eat_only, args.supply_prep,
            args.supply_return_via, args.supply_refuel_before, args.supply_refuel_return,
            args.supply_withdraw_before, args.supply_withdraw_return]) or args.supply_city == "banville"):
        parser.error("一般交易須獨立正常交易保存及普通城市目的地，不併入殖民交付")
    if args.supply_reentry and (not args.phase0_supply or not args.library_input or args.supply_return or args.supply_departure or args.supply_landed):
        parser.error("交付重開須同局Banville正常保存及--phase0-supply")
    if args.supply_revisit and (not args.phase0_supply or not args.supply_landed or not args.library_input):
        parser.error("交易所重訪須同城正常保存及--phase0-supply --supply-landed")
    if args.supply_funding_only and (not args.colony_supply or not args.library_input or any([
            args.supply_reentry, args.supply_return, args.supply_departure, args.supply_revisit,
            args.supply_rest_only, args.supply_prep, args.supply_refuel_before,
            args.supply_withdraw_before, args.supply_withdraw_return])):
        parser.error("來源融資須獨立--colony-supply正常保存，可配合--supply-landed及--supply-refuel-return")
    if args.supply_rest_only and (not args.colony_supply or not args.library_input or any([
            args.supply_reentry, args.supply_return, args.supply_departure, args.supply_landed,
            args.supply_prep, args.supply_refuel_before, args.supply_refuel_return,
            args.supply_withdraw_before, args.supply_withdraw_return])):
        parser.error("供貨休息須獨立--colony-supply正常交易保存，不併入買貨或補給")
    if args.condition_only and (not Path("/.dockerenv").is_file() or not args.library_input or any([
            args.warehouse_only, args.cryo_newgame, args.site_only, args.site_consumers,
            args.site_arrival, args.marquee_only, args.purchase_only, args.phase0_delivery,
            args.phase0_supply, args.colony_first_visit])):
        parser.error("讀數抽樣須Docker內獨立正常保存及輸出")
    if args.supply_withdraw_before and (not args.phase0_supply or any([args.supply_return,args.supply_landed,args.supply_prep])):
        parser.error("出發提款須配合供貨出發")
    if args.supply_withdraw_return and (not args.phase0_supply or args.supply_reentry):
        parser.error("來源提款須配合實際取貨返航，不能配合僅重開")
    if args.supply_refuel_return and not args.phase0_supply:
        parser.error("返航購油須配合第一期供貨")
    if args.supply_refuel_before and (not args.phase0_supply or any([args.supply_return,args.supply_prep,args.supply_landed])):
        parser.error("出發購油須配合獨立供貨出發")
    if args.colony_first_visit_resume and not args.colony_first_visit:
        parser.error("首訪續行須配合--colony-first-visit")
    if args.colony_first_visit and (not args.library_input or any([args.cryo_newgame,args.warehouse_only,args.site_only,args.site_consumers,args.site_arrival,args.marquee_only,args.purchase_only,args.phase0_delivery,args.phase0_supply])):
        parser.error("首次殖民地訪問須獨立同局正常保存")
    if args.cryo_fetched_resume and not args.cryo_resume:
        parser.error("已取貨續行須配合--cryo-resume")
    if args.cryo_resume and (not args.cryo_newgame or not args.library_input):
        parser.error("冷凍人續行須配合同一正常保存及--cryo-newgame")
    if args.cryo_start_resume and (not args.cryo_newgame or not args.library_input or args.cryo_resume):
        parser.error("初局續行須配合冷凍人新局及同一正常船艙保存")
    if args.cryo_newgame and (args.library_input and not (args.cryo_resume or args.cryo_start_resume) or any([args.warehouse_only,args.site_only,args.site_consumers,args.site_arrival,args.marquee_only,args.purchase_only,args.phase0_delivery,args.phase0_supply])):
        parser.error("冷凍人新局須獨立正常新局與輸出")
    if args.warehouse_only and (not args.library_input or any([args.site_only, args.site_consumers, args.site_arrival, args.marquee_only, args.purchase_only, args.phase0_delivery, args.phase0_supply])):
        parser.error("倉庫存取須獨立正常保存與輸出")
    if args.supply_landed and not args.phase0_supply:
        parser.error("--supply-landed須配合--phase0-supply")
    if args.supply_prep and (not args.phase0_supply or args.supply_landed):
        parser.error("--supply-prep須配合獨立--phase0-supply，不能配合地面續行")
    if args.supply_departure and (not args.phase0_supply or args.supply_prep or args.supply_landed):
        parser.error("--supply-departure須配合獨立--phase0-supply，不重做補給或目的地續行")
    if args.supply_return and (not args.phase0_supply or args.supply_prep or args.supply_landed or args.supply_departure):
        parser.error("--supply-return須配合獨立--phase0-supply，不重飛或購買")
    if args.phase0_supply and (not args.library_input or args.site_only or args.site_consumers or args.site_arrival or args.marquee_only or args.purchase_only or args.phase0_delivery):
        parser.error("第一期供貨須獨立正常保存及輸出")
    if args.phase0_delivery and (not args.library_input or args.site_only or args.site_consumers or args.site_arrival or args.marquee_only or args.purchase_only):
        parser.error("第一期續行須獨立輸出與目前schema正常保存")
    if args.purchase_resume and (not args.purchase_only or not args.library_input):
        parser.error("購買續行須--purchase-only及本輪正常保存")
    if args.purchase_only and (args.site_only or args.site_consumers or args.site_arrival or args.marquee_only or args.library_input and not args.purchase_resume):
        parser.error("購買驗證須獨立新局與輸出，不載入舊保存")
    if args.marquee_only and (args.site_only or args.site_consumers or args.site_arrival):
        parser.error("走馬燈抽樣需要獨立輸出與正常交易保存")
    if args.site_consumers and (not args.site_only or args.site_input != "keyboard"):
        parser.error("--site-consumers 需要 --site-only --site-input keyboard")
    if args.site_arrival and (args.site_only or args.site_consumers):
        parser.error("--site-arrival 是保存續行，不能與新局選址模式並用")
    if not Path("/.dockerenv").is_file():
        parser.error("只在Docker執行")
    root = Path(__file__).resolve().parents[1]
    schema = int(re.search(r"const CurrentSchema = (\d+)", (root / "internal/game/state.go").read_text()).group(1))
    if not args.cryo_newgame and not args.site_only and not args.purchase_only and args.library_input is None:
        parser.error("成長玩家鏈需要 --library-input")
    for name in ["binary", "disk", "settings_input"] + (["library_input"] if args.library_input else []):
        path = getattr(args, name).resolve(strict=True)
        if not path.is_file():
            parser.error(f"{name} 必須是檔案")
        setattr(args, name, path)
    initial = json.loads(args.library_input.read_text())["slots"]["slot-1"] if args.library_input else None
    if initial is not None and initial["schema"] != schema:
        parser.error("只支援目前schema的新存檔，不遷移")
    if args.cryo_start_resume and (initial["schema"] != schema or initial["screen"] != "shipboard" or initial["cityId"] != "drahew" or any(h.get("kind") for h in initial["holds"]) or initial.get("handItem")):
        parser.error("須同一目前schema的Drahew初局船艙保存，不改寫狀態")
    if args.condition_only and (initial["schema"] != schema or initial["screen"] != "trade"
            or initial.get("tradePage", 0) != 0 or initial.get("tradePurchaseLot")
            or initial["cityId"] != "banville" or initial["health"] <= 0):
        parser.error("讀數抽樣須同局目前schema Banville正常交易Main保存")
    if args.warehouse_only and (initial["schema"] != schema or initial["screen"] != "trade" or initial.get("tradePurchaseLot") or initial.get("tradeSaleSlot") or not any(h.get("kind") == "goods" for h in initial["holds"]) or not any(l["goods"] == 0 for l in initial["warehouses"][initial["cityId"]])):
        parser.error("倉庫回歸須正常目前schema交易、已有貨物及空倉庫槽")
    if args.phase0_supply:
        expected_screens = ("ground","dialog","shipboard","navigation","cargo") if args.supply_departure else (("ground","trade","dialog","arena","shipboard","navigation") if args.supply_return else (("shipboard","ground","trade") if args.supply_landed else ("trade",)))
        if args.stores_eat_resume:
            expected_screens = ("ground",)
        if args.market_visit_only:
            expected_screens = ("ground", "trade")
        if (initial["schema"] != schema or initial["screen"] not in expected_screens
                or initial["screen"] == "dialog" and initial["groundReturn"] != "ground"
                or initial["screen"] == "arena" and (initial["layout"] != "remake" or initial["groundReturn"] != "ground")
                or args.supply_landed and (initial["cityId"] != args.supply_city or initial["groundWorld"])
                or args.supply_return and initial["cityId"] not in [args.supply_city,"banville"]
                or initial["screen"] in ["shipboard","navigation","cargo"] and not initial["cargoAttached"]
                or initial["screen"] == "cargo" and (initial["shipCityId"] != initial["cityId"] or initial["groundWorld"])
                or (initial["groundMode"] != "pod" and not (initial["screen"] in ["shipboard","navigation","cargo"] and initial["cargoAttached"])) or initial.get("tradePurchaseLot")
                or not initial["colony"]["initialized"]
                or not 0 <= initial["colony"]["phase"] <= 8
                or initial["colony"]["nextPhase"] != initial["colony"]["phase"]+1
                or not args.colony_supply and initial["colony"]["phase"] != 0
                or initial["colonySite"]["pending"]):
            parser.error("供貨續行須正常目前schema有效殖民階段pod保存；第一期入口限Phase0，地面續行須同目的城市")
        if args.supply_revisit and (initial["screen"] != "trade" or initial.get("tradePage",0) not in [0,3]):
            parser.error("交易所重訪須正常交易Main或入口")
    if args.phase0_delivery and (initial["schema"] != schema or initial["screen"] != "trade" or initial["cityId"] != "drahew" or initial["groundMode"] != "pod" or initial.get("tradePurchaseLot") or initial["colony"]["initialized"] or not initial["colonySite"]["pending"]):
        parser.error("第一期續行需要真正目前schema Drahew未選址pod交易保存")
    if args.purchase_resume and (initial["schema"] != schema or initial["screen"] != "trade" or initial["groundMode"] != "foot" or initial.get("tradePurchaseLot")):
        parser.error("購買續行需要本輪目前schema正常步行交易保存")
    if args.site_arrival and (initial["schema"] != schema or initial["screen"] != "ground"
            or initial["cityId"] != "obburg" or initial["shipCityId"] != "drahew"
            or initial["colonySite"]["pending"] or initial["colony"]["initialized"]
            or initial["parts"] or initial["handItem"]):
        parser.error("需要正常選址／Tube玩家鏈的目前schema Obburg保存，不改寫狀態")
    if args.marquee_only and (initial["schema"] != schema or initial["screen"] != "trade" or initial["tradePage"] != 3 or len(initial["quotes"]) != 8):
        parser.error("走馬燈需要正常 目前schema 交易入口保存及八項報價")
    if not args.condition_only and not args.colony_first_visit and not args.cryo_newgame and not args.warehouse_only and not args.site_only and not args.site_arrival and not args.marquee_only and not args.purchase_only and not args.phase0_delivery and not args.phase0_supply and (initial["schema"] != schema or initial["screen"] != "shipboard"
            or initial["cityId"] != "drahew" or initial["colony"]["initialized"]
            or initial["parts"] or initial["handItem"]):
        parser.error("需要 目前schema 的 Drahew 新局船上 slot-1；不改寫保存狀態")
    if args.cryo_fetched_resume and (initial["schema"] != schema or initial["screen"] != "trade" or initial["tradePage"] != 4 or initial["holds"][0].get("goods") != 31 or initial["warehouses"][initial["cityId"]][0]["goods"] != 0):
        parser.error("須同一正常Fetch後目前schema交易保存")
    if args.cryo_resume and not args.cryo_fetched_resume and (initial["schema"] != schema or initial["screen"] != "ground" or initial["groundMode"] != "pod" or initial.get("groundWorld") or initial["warehouses"][initial["cityId"]][0]["goods"] != 31):
        parser.error("冷凍人續行須同一正常目前schema登陸pod保存，原倉庫批次仍在")
    if args.colony_first_visit:
        if args.colony_first_visit_resume:
            if (initial["schema"] != schema or initial["screen"] != "trade" or initial["cityId"] != "banville"
                    or initial["groundMode"] != "pod" or not initial["colony"]["initialized"]
                    or initial["colony"]["phase"] != 0 or initial["colonySite"]["pending"]
                    or initial.get("tradePage",0) not in [0,9] or initial.get("tradeSalePrice",0) != 0
                    or (initial.get("tradePage",0) == 9 and (
                        initial.get("tradeSaleSlot") not in [5,6]
                        or initial["holds"][initial["tradeSaleSlot"]-5].get("goods") != 31))
                    or (initial.get("tradePage",0) == 0 and initial.get("tradeSaleSlot",0) != 0)
                    or not any(h.get("goods") == 31 for h in initial["holds"])):
                parser.error("首訪續行須同局目前schema第一期31零需求正常確認保存")
        elif (initial["schema"] != schema or initial["screen"] not in ["trade","ground","shipboard"] or initial["groundMode"] != "pod" or initial["colony"]["initialized"] or not initial["colonySite"]["pending"] or not any(h.get("goods") == 31 for h in initial["holds"])):
            parser.error("首次殖民地訪問須同一目前schema未選址且已取得31的正常pod保存")
    settings = json.loads(args.settings_input.read_text())
    if settings.get("layout") != "atari-st":
        parser.error("需要已選定 Atari ST 版面的設定")
    out = args.output.resolve()
    if out.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄須屬於目前使用者")
    out.mkdir(exist_ok=False)
    for name in ("assets", "docs"):
        (out / name).symlink_to(root / name, target_is_directory=True)
    (out / "settings.json").write_bytes(args.settings_input.read_bytes())
    if args.library_input and not args.site_only:
        (out / "library.json").write_bytes(args.library_input.read_bytes())
    env = dict(os.environ, DISPLAY=":99", EBITENGINE_SCREENSHOT_KEY="F12")
    checks = []
    inputs = []
    process = None
    window = None
    display = subprocess.Popen(
        ["Xvfb", ":99", "-screen", "0", "1280x800x24", "-nolisten", "tcp", "-noreset"],
        stdout=(out / "xvfb.log").open("w"), stderr=subprocess.STDOUT,
    )

    def run(command):
        return subprocess.run(command, env=env, check=True, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, text=True).stdout

    def key(value):
        inputs.append({"key": value})
        run(["xdotool", "keydown", value])
        time.sleep(.4 if value == "F8" else .13)
        run(["xdotool", "keyup", value])
        time.sleep(.6 if value == "F8" else .22)

    def click(x, y):
        inputs.append({"click": [x, y], "scale": 3})
        run(["xdotool", "mousemove", "--window", window, str(x * 3), str(y * 3)])
        run(["xdotool", "click", "1"])
        time.sleep(.3)

    def start(load=False):
        nonlocal process, window
        command = [str(args.binary), "-scale", "3", "-start", "title",
                   "-atari-disk", str(args.disk), "-settings", str(out / "settings.json"),
                   "-save", str(out / "save.json"), "-library", str(out / "library.json"),
                   "-locale", args.locale]
        if load:
            command.append("-load")
        process = subprocess.Popen(command, cwd=out, env=env,
                                   stdout=(out / ("game-load.log" if load else "game.log")).open("w"),
                                   stderr=subprocess.STDOUT)
        for _ in range(100):
            if process.poll() is not None:
                raise RuntimeError("遊戲提早退出")
            result = subprocess.run(["xdotool", "search", "--onlyvisible", "--name", "SunDog"],
                                    env=env, stdout=subprocess.PIPE, text=True)
            if result.returncode == 0 and result.stdout.strip():
                window = result.stdout.strip().splitlines()[-1]
                break
            time.sleep(.1)
        else:
            raise RuntimeError("找不到遊戲視窗")
        run(["xdotool", "windowfocus", window])
        time.sleep(1)

    def capture(name):
        library = out / "library.json"
        before = library.stat().st_mtime_ns if library.exists() else 0
        key("F5")
        if not library.exists():
            key("m")
        for value in ["1", "s", "Escape"]:
            key(value)
        assert library.stat().st_mtime_ns != before, "保存庫未更新"
        raw = library.read_bytes()
        (out / ("library-" + name + ".json")).write_bytes(raw)
        state = json.loads(raw)["slots"]["slot-1"]
        checks.append({"name": name, "state": state, "sha256": hashlib.sha256(raw).hexdigest()})
        print(name, state["screen"], state["cityId"], state["position"], flush=True)
        return state

    def shot(name):
        before = set(out.glob("screenshot*.png"))
        key("F12")
        for _ in range(50):
            created = set(out.glob("screenshot*.png")) - before
            if created:
                break
            time.sleep(.1)
        assert len(created) == 1, "Ebiten 同步截圖未完成"
        created.pop().rename(out / (name + ".png"))

    def terminal_capture(name):
        # Score deliberately has no F5 library entry. Q uses the production
        # autosave; observe its real single-save envelope, without synthesizing
        # a library or writing a state into the running game.
        inputs.append(dict(waitSeconds=3,reason="terminal-score-message-expiry"))
        time.sleep(3)
        shot(name)
        key("q")
        process.wait(timeout=20)
        path=out/"save.json"
        assert path.is_file(), "終局Q未產生正式自動保存"
        raw=path.read_bytes()
        (out/("save-"+name+".json")).write_bytes(raw)
        state=json.loads(raw)["state"]
        checks.append(dict(name=name,state=state,sha256=hashlib.sha256(raw).hexdigest(),saveKind="single-autosave"))
        print(name,state["screen"],state["cityId"],state["colony"]["phase"],flush=True)
        return state

    def walk(x, y, name, expected_screen="ground"):
        inputs.append({"walkTarget": [x, y], "scale": 3, "holdSeconds": 2})
        run(["xdotool", "mousemove", "--window", window, str(x * 3), str(y * 3)])
        run(["xdotool", "mousedown", "1"])
        time.sleep(2)
        run(["xdotool", "mouseup", "1"])
        time.sleep(.3)
        state = capture(name)
        allowed = (expected_screen,) if isinstance(expected_screen, str) else expected_screen
        assert state["screen"] in allowed, "步行抵達的畫面不同"
        return state

    try:
        for _ in range(50):
            if subprocess.run(["xwininfo", "-root"], env=env,
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(.1)
        start()
        key("space")
        if args.condition_only:
            for value in ["F5", "1", "l"]:
                key(value)
            state = capture("01-condition-resumed")
            readings = ["vigor", "rest", "health", "nourishment"]
            invariants = ["schema", "holds", "warehouses", "colony", "colonySite", "foodUnits"]
            assert all(state.get(k) == initial.get(k) for k in readings + invariants + ["timeMinutes"])
            for _ in range(3):
                if state["screen"] == "ground":
                    break
                key("Escape")
                state = capture("02-condition-exit-" + str(len(checks)))
            assert state["screen"] == "ground"
            for index in range(4):
                previous = state
                key(["Left", "Right", "Up", "Down"][index % 4])
                state = capture(f"03-condition-move-{index:02d}")
                assert state["screen"] == "ground", "正常移動離開地面，停止抽樣"
                # ST地面shell目前只有GroundTick，沒有父時鐘推進。
                # 此處只抽樣未推進時的UI與保存；跨整點由原生／正式規則另驗。
                assert state["timeMinutes"] == previous["timeMinutes"]
                assert all(state[k] == previous[k] for k in readings)
                assert all(state.get(k) == initial.get(k) for k in invariants)
            assert any(c["state"]["position"] != state["position"] for c in checks[3:])
            shot("04-condition-after-movement")
            fields = readings + invariants + ["screen", "cityId", "position", "groundMode",
                "timeMinutes", "fuel", "cash", "bank", "bankBalances", "tradeRolls", "combatRolls",
                "shipCityId", "shipPosition", "cargoCityId", "cargoPosition", "cargoAttached"]
            key("q")
            process.wait(timeout=20)
            before = json.loads((out / "save.json").read_text())["state"]
            start(load=True)
            reopened = capture("05-condition-single-reopened")
            assert all(reopened.get(k) == before.get(k) for k in fields)
            for value in ["F5", "1", "l"]:
                key(value)
            final = capture("06-condition-library-reopened")
            assert all(final.get(k) == reopened.get(k) for k in fields)
            shot("06-condition-reopened")
            receipt = {"status": "PASS", "scope": "同局目前schema正常交易退出、方向鍵移動、分鐘未推進時讀數保持、單檔及保存庫新程序重開；地面父時鐘、原版完整GUI及完整食物睡眠鏈未驗",
                "checks": checks, "inputs": inputs, "groundClock": "not-advanced-pending-parent-clock", "crossedHours": 0,
                "reentryFields": fields, "before": {k: before.get(k) for k in fields},
                "after": {k: final.get(k) for k in fields},
                "toolSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                "inputsSHA256": {name: hashlib.sha256(getattr(args, name).read_bytes()).hexdigest()
                    for name in ["binary", "disk", "settings_input", "library_input"]}}
            (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n")
            print(f"PASS:正常移動讀數保持／{len(fields)}欄位重開；地面父時鐘待接", flush=True)
            return
        if args.cryo_newgame:
            if args.cryo_fetched_resume:
                for value in ["F5","1","l"]:
                    key(value)
                state = capture("01-cryo-normal-fetched-resumed")
                target = state["cityId"]
                lot = {k:state["holds"][0].get(k,0) for k in ["goods","quality","price","sourceCode"]}
                source = json.loads((root/"internal/source/atarist/assets/world-data.json").read_text())
                row = next(row for row in source["tables"]["cities"]["records"] if row["name"].lower().replace("'","").replace(".","").replace(" ","-") == target)
                assert lot == {"goods":31,"quality":7,"price":0,"sourceCode":row["index"]+1}
                cold = {city:slots[0] for city,slots in state["warehouses"].items() if slots[0]["goods"] == 31}
                assert len(cold) == 16 and target not in cold
                # These seventeen batches now occupy sixteen warehouses and
                # one pod. This is observed post-Fetch content, not a new init.
                cold[target] = lot
                expected_warehouses = state["warehouses"]
            else:
                if args.cryo_resume or args.cryo_start_resume:
                    for value in ["F5","1","l"]:
                        key(value)
                    initial_cryo = state = capture("01-cryo-normal-resumed")
                    assert state["schema"] == schema and state["screen"] == ("ground" if args.cryo_resume else "shipboard")
                else:
                    for value in ["c", "n", "z", "e", "d", "Return", "Return", "Return"]:
                        key(value)
                    initial_cryo = state = capture("01-cryo-newgame")
                    assert state["schema"] == schema and state["screen"] == "shipboard"
                cold = {city: slots[0] for city, slots in state["warehouses"].items() if slots[0]["goods"] == 31}
                assert len(cold) == 17 and all(lot["quality"] == 7 and lot["price"] == 0 for lot in cold.values())
                assert cold["tuie"]["sourceCode"] == 17 and all(not hold.get("goods") for hold in state["holds"])
                source = json.loads((root / "internal/source/atarist/assets/world-data.json").read_text())
                def city_id(name):
                    # Match the stable identity from atarist/world.go: every
                    # punctuation run becomes one dash, including N'ljones.
                    return re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")
                records = source["tables"]["cities"]["records"]
                runs = source["grouping"]["planetCityRuns"]
                candidates = []
                for city in cold:
                    if city == state["cityId"]:
                        continue
                    row = next(row for row in records if city_id(row["name"]) == city)
                    info = next(info for info in runs if info["start"] <= row["index"] < info["start"] + info["count"])
                    candidates.append((info["system"] != "Jondd", info["planet"] != "Jondd", row["index"], city, info))
                if args.cryo_resume:
                    target = state["cityId"]
                    row = next(row for row in records if city_id(row["name"]) == target)
                    info = next(info for info in runs if info["start"] <= row["index"] < info["start"]+info["count"])
                else:
                    _, _, _, target, info = min(candidates)
                first_city = city_id(records[info["start"]]["name"])
                target_system = city_id(info["system"])
                lot = dict(cold[target])
                if not args.cryo_resume:
                    # Buy and install the scanner, then refuel through normal menus.
                    for value in ["g", "space", "y"]:
                        key(value)
                    state = capture("02-cryo-parts")
                    assert state["screen"] == "parts"
                    systems = json.loads((root / "docs/atari-st/ship-systems.json").read_text())
                    sale = sorted({23} | {int(c["itemIndex"], 16) for s in systems["systems"] for c in s["columns"]}
                                  | {int(s["alternate"]["itemIndex"], 16) for s in systems["systems"] if s.get("alternate")})
                    for _ in range((sale.index(22)-state.get("selectedPartIndex",0)) % len(sale)):
                        key("Right")
                    key("b")
                    assert 22 in capture("03-cryo-scanner-bought")["parts"]
                    for value in ["Escape", "Right", "Right"]:
                        key(value)
                    state = capture("04-cryo-depot")
                    if state["screen"] == "ground":
                        key("r")
                    assert capture("05-cryo-fuel-menu")["screen"] == "fuel-purchase"
                    key("1")
                    key("3")
                    assert capture("06-cryo-refueled")["fuel"] > 0
                    key("Left")
                    key("Left")
                    assert capture("07-cryo-boarded")["screen"] == "shipboard"
                    key("e")
                    state = capture("08-cryo-engineering")
                    assert state["screen"] == "engineering"
                    for _ in range((5-state.get("selectedSystemIndex",0)) % 6):
                        key("Right")
                    click(100,122)
                    held = capture("09-cryo-scanner-in-hand")
                    assert held["handItem"] == 22
                    pilotage_parts = [int(c["itemIndex"],16) for c in systems["systems"][5]["columns"]]
                    scanner_row = next(i for i,row in enumerate(held["shipSystems"][5]["rows"]) if row == pilotage_parts)
                    # SHIPLIB:0e only enables an alternate in a working row.
                    # New-game damage can leave row0 dead; use the intact row
                    # already present, without rerolling the new game.
                    click(168,89+17*scanner_row)
                    installed = capture("10-cryo-scanner-installed")
                    assert installed["shipSystems"][5]["rows"][scanner_row][2] == 22
                    click(97,94)
                    key("Escape")
                    key("n")
                    state = capture("11-cryo-navigation")
                    for attempt in range(12):
                        if target_system in state["navigationTarget"].lower().replace(" ","-").split("／")[:1]:
                            break
                        key("bracketright")
                        state = capture(f"12-cryo-system-{attempt}")
                    else:
                        raise AssertionError("normal navigation could not find cold source system")
                    for attempt in range(3):
                        if state["navigationTargetId"] in [first_city,target]:
                            break
                        key("Down")
                        state = capture(f"13-cryo-planet-{attempt}")
                    else:
                        raise AssertionError("normal navigation could not find cold source planet")
                    for attempt in range(6):
                        if state["navigationTargetId"] == target:
                            break
                        key("Right")
                        state = capture(f"14-cryo-city-{attempt}")
                    else:
                        raise AssertionError("normal navigation could not find cold source city")
                    key("Return")
                    key("space")
                    state = capture("15-cryo-departed")
                    if state["screen"] == "flight":
                        key("e")
                        key("space")
                        state = capture("16-cryo-engaged")
                    assert state["screen"] == "shipboard" and state["shipCityId"] == target and state["cargoAttached"]
                    key("c")
                    key("e")
                    key("space")
                    state = capture("17-cryo-pod-ground")
                    assert state["groundMode"] == "pod" and state["cityId"] == target
                if state["screen"] == "dialog":
                    key("Escape")
                # The actual exchange is source building slot2. Landing is not
                # assumed to be the exchange cell.
                for attempt in range(3):
                    if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                        break
                    key("F8")
                    state = capture(f"17-cryo-grid-presentation-{attempt}")
                row = next(row for row in records if city_id(row["name"]) == target)
                city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][row["index"]]
                exchange = next(b for b in city_map["buildings"] if b["slot"] == 2)
                goal = (exchange["row"],exchange["column"])
                current = (state["position"]["x"],state["position"]["y"])
                ship = (state["shipPosition"]["x"],state["shipPosition"]["y"])
                buildings = {(b["row"],b["column"]) for b in city_map["buildings"]}
                queue, previous = deque([current]), {current:None}
                while queue and goal not in previous:
                    point = queue.popleft()
                    for value,dx,dy in [("Up",0,-1),("Right",1,0),("Down",0,1),("Left",-1,0)]:
                        nx,ny = (point[0]+dx+10)%40-10,(point[1]+dy+6)%24-6
                        next_point = (nx,ny)
                        if next_point in previous or next_point == ship:
                            continue
                        if 0 <= nx < 20 and 0 <= ny < 12 and next_point not in buildings and city_map["grid"][nx][ny] in [6,7,12,13,14]:
                            continue
                        previous[next_point] = (point,value)
                        queue.append(next_point)
                assert goal in previous, "source map has no normal exchange route"
                route, point = [], goal
                while point != current:
                    parent,value = previous[point]
                    route.append((value,point))
                    point = parent
                for i,(value,point) in enumerate(reversed(route)):
                    key(value)
                    state = capture(f"17-cryo-walk-{i}")
                    if state["screen"] == "dialog" and state["groundReturn"] == "ground":
                        key("Escape")
                        state = capture(f"17-cryo-walk-dialog-{i}")
                    assert state["screen"] == "ground" and (state["position"]["x"],state["position"]["y"]) == point
                assert (state["position"]["x"],state["position"]["y"]) == goal and not state["groundWorld"]
                key("F8")
                state = capture("17-cryo-exchange-st")
                assert state["layout"] == "atari-st"
                key("t")
                original = state = capture("18-cryo-trade")
                assert state["screen"] == "trade" and state["warehouses"][target][0] == lot
                key("w")
                key("f")
                key("1")
                fetched = capture("19-cryo-fetched")
                assert all(fetched["holds"][0].get(k,0) == v for k,v in lot.items())
                assert fetched["warehouses"][target][0] == {"goods":0,"quality":0,"price":0,"sourceCode":0}
                expected_warehouses = json.loads(json.dumps(original["warehouses"]))
                expected_warehouses[target][0] = {"goods":0,"quality":0,"price":0,"sourceCode":0}
                assert fetched["warehouses"] == expected_warehouses
                assert all(fetched.get(k) == original.get(k) for k in ["cash","bank","bankBalances","tradeSession","tradeRolls","combatRolls"])
                shot("19-cryo-fetched")
            fields = ["schema","screen","cityId","systemId","planetId","position","groundMode","groundView","holds","warehouses","tradePage","tradeSession","tradeClassShift","tradeRolls","combatRolls","cash","bank","bankBalances","colony","colonySite","fuel","shipCityId","shipPosition","cargoCityId","cargoPosition","cargoAttached","timeMinutes"]
            key("q")
            process.wait(timeout=20)
            before = json.loads((out/"save.json").read_text())["state"]
            start(load=True)
            reopened = capture("20-cryo-single-reopened")
            assert all(reopened.get(k) == before.get(k) for k in fields)
            for value in ["F5","1","l"]:
                key(value)
            final = capture("21-cryo-library-reopened")
            assert all(final.get(k) == reopened.get(k) for k in fields)
            assert final["warehouses"] == expected_warehouses
            assert sum(l["goods"] == 31 for slots in final["warehouses"].values() for l in slots) == 16
            shot("21-cryo-reopened")
            receipt = {"status":"PASS","saveSchema":schema,"scope":"正常目前schema新局17批、購買安裝掃描器、補油、正式導航航行至實際來源、pod倉庫Fetch、單檔與保存庫新程序重開；不注入或挑seed。原版受控對拍另列，完整Banville交付／九期待接。","sourceCity":target,"sourceLot":lot,"initialColdLots":cold,"checks":checks,"inputs":inputs,"reentryFields":fields,"before":{k:before.get(k) for k in fields},"after":{k:final.get(k) for k in fields},"toolSHA256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),"inputsSHA256":{name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input"]+(["library_input"] if args.library_input else [])}}
            if args.cryo_start_resume:
                receipt["startedFromNormalNewgame"] = True
                receipt["scope"] = "沿用同一目前schema的新局船艙保存，正常購買與安裝掃描器、補油、旅行到來源及Fetch；新局17批由上游同局保存取得，沒有重抽或改寫。單檔／保存庫重開通過；後續Banville交付另列。"
            if args.cryo_resume:
                receipt["scope"] = receipt["scope"].replace("正常目前schema新局17批、購買安裝掃描器、補油、正式導航航行至實際來源、", "承接同一正常目前schema新局已完成設備／補油／航行的登陸保存，正常步行至原始交易所格、")
                receipt["resumed"] = True
            if args.cryo_fetched_resume:
                receipt["scope"] = "承接同一正常目前schema新局已取貨保存，只驗單檔／保存庫與新程序重開，保留16倉庫批次及pod第17批；新局、設備、航行、步行及Fetch由同局上游收據保存。原版GUI／正常Banville交付未驗。"
                receipt["fetchedResume"] = True
                receipt["initialColdLotsMeaning"] = "續行初態觀測：16倉庫批次及同一pod已取出批次，非重新初始化"
            (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
            print(f"PASS:正常17批新局、旅行至{target}取貨、{len(fields)}欄位單檔／保存庫重開",flush=True)
            return
        if args.warehouse_only:
            for value in ["F5", "1", "l"]:
                key(value)
            original = state = capture("01-normal-warehouse-resumed")
            assert state["screen"] == "trade"
            fields = ["schema", "screen", "cityId", "position", "groundMode", "tradePage", "tradeSession", "cash", "bank", "bankBalances", "holds", "warehouses", "tradeClassShift", "tradeRolls", "combatRolls", "colony", "colonySite", "fuel", "shipCityId", "shipPosition", "cargoCityId", "cargoPosition", "timeMinutes"]
            pod_slot = next(i for i,h in enumerate(state["holds"]) if h.get("kind") == "goods")
            wh_slot = next(i for i,l in enumerate(state["warehouses"][state["cityId"]]) if l["goods"] == 0)
            lot = {k: state["holds"][pod_slot][k] for k in ["goods", "quality", "price", "sourceCode"]}
            assert lot["quality"] > 0 and lot["price"] > 0 and lot["sourceCode"] > 0
            for _ in range(3):
                if state.get("tradePage", 0) == 3:
                    break
                key("Escape")
                state = capture("02-return-to-entrance")
            assert state.get("tradePage", 0) == 3
            key("w")
            key("s")
            key(str(pod_slot+1))
            stored = capture("03-stored")
            assert stored["holds"][pod_slot] == {"sourceCode": 0}, "Store沒有清空整筆原槽"
            assert stored["warehouses"][stored["cityId"]][wh_slot] == lot
            assert all(stored[k] == original[k] for k in ["tradeSession", "cash", "bank", "bankBalances", "tradeRolls", "combatRolls"])
            click(220, 150)
            shot("03-stored")
            key("q")
            process.wait(timeout=20)
            before = json.loads((out/"save.json").read_text())["state"]
            start(load=True)
            after = capture("04-stored-reopened")
            assert all(before.get(k) == after.get(k) for k in fields), "存倉後重開不同"
            assert after["holds"][pod_slot] == {"sourceCode": 0}
            assert after["tradePage"] == 4
            free_pod = next(i for i,h in enumerate(after["holds"]) if not h.get("kind"))
            key("f")
            key(str(wh_slot+1))
            fetched = capture("05-fetched")
            assert all(fetched["holds"][free_pod][k] == v for k,v in lot.items())
            assert fetched["warehouses"][fetched["cityId"]][wh_slot] == {"goods": 0, "quality": 0, "price": 0, "sourceCode": 0}
            assert all(fetched[k] == original[k] for k in ["tradeSession", "cash", "bank", "bankBalances", "tradeRolls", "combatRolls"])
            click(220, 150)
            shot("05-fetched")
            key("q")
            process.wait(timeout=20)
            before_final = json.loads((out/"save.json").read_text())["state"]
            start(load=True)
            final = capture("06-fetched-reopened")
            assert all(before_final.get(k) == final.get(k) for k in fields), "取貨後重開不同"
            assert all(final["holds"][free_pod][k] == v for k,v in lot.items())
            receipt = {"status": "PASS", "scope": "既有正常目前schema購貨保存的獨立回歸分支，正常鍵盤Store／Fetch、保存庫及單檔兩次新程序重開；不注入、不改原保存、不延續第二期主線。原版GUI未驗。", "checks": checks, "inputs": inputs, "lot": lot, "podSlot": pod_slot+1, "warehouseSlot": wh_slot+1, "reentryFields": fields, "storedBefore": {k: before.get(k) for k in fields}, "storedAfter": {k: after.get(k) for k in fields}, "fetchedBefore": {k: before_final.get(k) for k in fields}, "fetchedAfter": {k: final.get(k) for k in fields}, "toolSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "inputsSHA256": {name: hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary", "disk", "settings_input", "library_input"]}}
            (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
            print(f"PASS:正常Store／Fetch、兩次{len(fields)}欄位重開", flush=True)
            return
        if args.purchase_only:
            if args.purchase_resume:
                for value in ["F5", "1", "l"]:
                    key(value)
                state = capture("01-normal-purchase-resumed")
                for _ in range(3):
                    if state.get("tradePage", 0) in [0, 3]:
                        break
                    key("Escape")
                    state = capture("02-sale-exit")
                assert state["screen"] == "trade" and state["groundMode"] == "foot"
            else:
                for value in ["c", "n", "z", "e", "d", "Return", "Return", "Return"]:
                    key(value)
                state = capture("01-newgame")
                assert state["schema"] == schema and state["screen"] == "shipboard"
                key("g")
                key("space")
                state = capture("02-foot-city")
                assert state["screen"] == "ground" and state["groundMode"] == "foot"
                key("t")
                state = capture("03-foot-exchange")
                assert state["screen"] == "trade" and state["groundMode"] == "foot"
            key("e")
            click(220, 150)  # 正式delay_button提示消費，不觸發購買。
            key("b")
            for attempt in range(8):
                quoted = capture(f"04-foot-quote-{attempt}")
                assert quoted.get("tradePage", 0) == 1
                first = quoted["quotes"][quoted.get("quoteIndex", 0)]
                if first["goods"] not in [1, 31] and first["price"] * 100 <= quoted["tradeSession"]["funds"]:
                    break
                key("p")  # 正常挑可在普通城市出售的商品，不重抽seed。
            else:
                raise AssertionError("正常庫存没有可購買／售出的商品")
            existing = quoted.get("warehouses", {}).get(quoted["cityId"], [{"goods": 0}] * 4)
            warehouse_slot = next(i for i, value in enumerate(existing) if value["goods"] == 0)
            key("b")
            bought = capture("05-foot-warehouse-bought")
            assert bought.get("tradePage", 0) == 0 and not bought.get("tradePurchaseLot")
            assert bought["holds"] == quoted["holds"], "步行買貨改動pod"
            lot = bought["warehouses"][bought["cityId"]][warehouse_slot]
            assert all(lot[k] == first[k] for k in ["goods", "quality", "price"]) and lot["sourceCode"] > 0
            assert bought["tradeSession"]["funds"] == quoted["tradeSession"]["funds"] - first["price"] * 100
            click(220, 150)
            shot("05-foot-warehouse-bought")
            # 正常倉庫售出，回收資金後走回船，從貨艙頁乘pod出去。
            key("s")
            key(str(warehouse_slot + 1))
            key("1")
            sold = capture("06-foot-warehouse-sold")
            assert sold["warehouses"][sold["cityId"]][warehouse_slot]["goods"] == 0
            key("Escape")
            key("Escape")
            key("F8")
            key("Right")
            key("Left")
            state = capture("07-boarded")
            assert state["screen"] == "shipboard", "正常格子步行未回船"
            key("F8")
            key("c")
            key("e")
            key("t")
            state = capture("08-pod-exchange")
            assert state["screen"] == "trade" and state["groundMode"] == "pod"
            key("e")
            click(220, 150)
            key("b")
            quoted = capture("09-pod-quote")
            second = quoted["quotes"][quoted.get("quoteIndex", 0)]
            key("b")
            paid = capture("10-paid-destination")
            assert paid["tradePage"] == 10 and paid["tradePurchaseLot"]["goods"] == second["goods"]
            assert paid["tradeSession"]["funds"] == quoted["tradeSession"]["funds"] - second["price"] * 100
            assert paid["holds"] == quoted["holds"] and paid.get("warehouses") == quoted.get("warehouses")
            shot("10-paid-destination")
            # 官方保存庫第二槽保留同一已付款狀態，用於兩種選槽輸入分支。
            for value in ["F5", "2", "s", "Escape"]:
                key(value)
            key("q")
            process.wait(timeout=20)
            before = json.loads((out / "save.json").read_text())["state"]
            start(load=True)
            reopened = capture("11-paid-reopened")
            fields = ["schema", "screen", "cityId", "systemId", "groundMode", "position", "groundView",
                      "tradePage", "tradePurchaseLot", "tradeSession", "quotes", "quoteIndex", "tradeRolls",
                      "warehouses", "holds", "markets", "colony", "colonySite", "cash", "bank"]
            assert all(before.get(k) == reopened.get(k) for k in fields), "已付款待選槽重開不同"
            # menu14來自stZoomOrigin(152,80)及原始box(3..78,4..14)。
            click(193, 90)
            placed = capture("12-mouse-warehouse")
            assert placed.get("tradePage", 0) == 0 and not placed.get("tradePurchaseLot")
            warehouse_slot = next(i for i, value in enumerate(reopened["warehouses"][reopened["cityId"]]) if value["goods"] == 0)
            assert placed["warehouses"][placed["cityId"]][warehouse_slot] == reopened["tradePurchaseLot"]
            assert placed["tradeSession"]["funds"] == reopened["tradeSession"]["funds"]
            click(220, 150)
            shot("12-mouse-warehouse")
            for value in ["F5", "2", "l"]:
                key(value)
            restored = capture("13-paid-library-restore")
            assert all(paid.get(k) == restored.get(k) for k in fields), "官方保存庫還原改動已付款狀態"
            key("2")
            placed = capture("14-keyboard-pod")
            assert placed.get("tradePage", 0) == 0 and not placed.get("tradePurchaseLot")
            hold = placed["holds"][0]
            assert all(hold[k] == restored["tradePurchaseLot"][k] for k in ["sourceCode", "goods", "quality", "price"])
            assert placed["tradeSession"]["funds"] == restored["tradeSession"]["funds"]
            click(220, 150)
            shot("14-keyboard-pod")
            key("q")
            process.wait(timeout=20)
            before_final = json.loads((out / "save.json").read_text())["state"]
            start(load=True)
            final = capture("15-placed-reopened")
            assert all(before_final.get(k) == final.get(k) for k in fields), "放貨後重開不同"
            receipt = {"status": "PASS", "scope": "正常新局步行買貨入倉／售出、回船乘pod、批准先扣款、待選槽單檔新程序重開／官方保存庫還原、滑鼠倉庫及鍵盤pod、放貨後重開；不含原版GUI、Banville第一期或九期",
                       "checks": checks, "inputs": inputs, "saveSchema": 40, "reentryFields": fields,
                       "pendingBefore": {k: before.get(k) for k in fields}, "pendingAfter": {k: reopened.get(k) for k in fields},
                       "finalBefore": {k: before_final.get(k) for k in fields}, "finalAfter": {k: final.get(k) for k in fields},
                       "inputsSHA256": {name: hashlib.sha256(getattr(args, name).read_bytes()).hexdigest() for name in ["binary", "disk", "settings_input"]},
                       "toolSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
            if args.purchase_resume:
                receipt["normalStart"] = str(args.library_input)
                receipt["inputsSHA256"]["library_input"] = hashlib.sha256(args.library_input.read_bytes()).hexdigest()
                receipt["scope"] = receipt["scope"].replace("正常新局", "承接同一正常目前schema新局保存")
            (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n")
            print("PASS:正常新局購買／兩目的地／20欄位付款後重開", flush=True)
            return
        if args.site_only:
            # 標題選局→新角色→姓名→確認屬性。所有保存由正式UI寫入。
            for value in ["c", "n", "z", "e", "d", "Return", "Return", "Return"]:
                key(value)
            state = capture("01-newgame")
            assert state["schema"] == schema and state["screen"] == "shipboard", "未由正常選單建立新局"
            assert state["colonySite"] == {"pending": True, "position": {"x": 31, "y": 7}}
            key("g")
            key("space")
            state = capture("02-city")
            assert state["screen"] == "ground" and not state.get("groundWorld", False)
            assert state["colonySite"]["pending"], "下船先選址"
            parked = {k: state[k] for k in ["shipCityId", "shipPosition", "cargoCityId", "cargoPosition", "colony"]}
            if args.site_input == "cell-keyboard":
                # 這是另一條正式呈現路徑，不代替 ST 像素出城驗收。
                key("F8")
            shot("02-city")
            # 先走到船的東側再向北，避開白色船身與登船門位。
            key("Right")
            state = capture("02-east-of-ship")
            assert state["screen"] == "ground" and state["colonySite"]["pending"], "離船途中錯誤切換模式"
            key("Up")
            for attempt in range(12):
                if args.site_input in ["keyboard", "cell-keyboard"]:
                    for _ in range(2):
                        key("Up")
                    state = capture(f"03-walk-{attempt:02d}")
                else:
                    state = walk(224, 5, f"03-walk-{attempt:02d}")
                assert state["screen"] == "ground", "出城途中離開地面"
                if state.get("groundWorld", False):
                    break
            else:
                raise AssertionError("有界正常輸入未走出城市")
            assert not state["colonySite"]["pending"], "第一行星出城未選址"
            assert all(state[k] == v for k,v in parked.items()), "選址搬動船、貨艙或殖民進度"
            selected = state["colonySite"]
            shot("04-selected-surface")
            for _ in range(2):
                key("F8")
            repaint = capture("05-presentation-return")
            assert repaint["colonySite"] == selected and repaint["combatRolls"] == state["combatRolls"], "呈現切換重新選址"
            key("q")
            process.wait(timeout=20)
            before = json.loads((out / "save.json").read_text())["state"]
            start(load=True)
            after = capture("06-reopened")
            fields = ["schema", "screen", "cityId", "planetId", "position", "groundWorld", "groundView", "colonySite", "colony", "combatRolls", "shipCityId", "shipPosition", "cargoCityId", "cargoPosition", "cash", "bank", "timeMinutes"]
            assert all(before.get(k) == after.get(k) for k in fields), "保存重開狀態不同"
            consumers = None
            if args.site_consumers:
                # 承接同一新局的真正保存；用正式F8選擇格子步行，不注入座標。
                assert after["cityId"] == "drahew" and after["groundWorld"]
                assert after["position"] == {"x": 12, "y": 7}
                key("F8")
                key("Right")
                key("Left")
                state = capture("07-returned-city")
                assert not state["groundWorld"] and state["cityId"] == "drahew"
                city = json.loads((root / "internal/source/atarist/assets/city-maps.json").read_text())["cities"][0]
                buildings = {(b["row"], b["column"]) for b in city["buildings"]}

                def walk_cells(state, goal, name):
                    start = (state["position"]["x"], state["position"]["y"])
                    queue = deque([start])
                    previous = {start: None}
                    while queue and goal not in previous:
                        current = queue.popleft()
                        for value, dx, dy in [("Up", 0, -1), ("Right", 1, 0), ("Down", 0, 1), ("Left", -1, 0)]:
                            nx, ny = (current[0]+dx+10)%40-10, (current[1]+dy+6)%24-6
                            position = (nx,ny)
                            if position in previous:
                                continue
                            if position == (1,0) and position != goal:
                                continue  # 途中不穿過登船門。
                            if 0 <= nx < 20 and 0 <= ny < 12 and position not in buildings:
                                if city["grid"][nx][ny] in [6,7,12,13,14]:
                                    continue
                            previous[position] = (current,value)
                            queue.append(position)
                    assert goal in previous, "來源城市格找不到正常步行路線"
                    route = []
                    while goal != start:
                        parent,value = previous[goal]
                        route.append(value)
                        goal = parent
                    for value in reversed(route):
                        key(value)
                    return capture(name)

                state = walk_cells(state, (1,0), "08-boarded")
                assert state["screen"] == "shipboard"
                key("F8")  # Apple II→Atari ST
                key("n")
                state = capture("09-nav-sector")
                assert state["screen"] == "navigation" and state["navigationTargetId"] == "banville"
                key("Right")
                key("Left")
                state = capture("10-nav-st-planet")
                assert state["navigationView"] == "planet" and state["navigationTargetId"] == "banville"
                assert state["theme"] == "atari-st-original" and state["layout"] == "atari-st"
                shot("10-nav-st-planet")
                key("F8")
                state = capture("11-nav-remake-planet")
                assert state["navigationView"] == "planet" and state["colonySite"] == selected
                assert state["tileset"] == "apple-ii-tiles" and state["layout"] == "remake"
                shot("11-nav-remake-planet")
                key("Escape")
                key("g")
                key("space")
                state = capture("12-tube-walk-start")
                assert state["screen"] == "ground" and state["position"] == {"x":1,"y":0}
                tube = next(b for b in city["buildings"] if b["slot"] == 0)
                state = walk_cells(state, (tube["row"],tube["column"]), "13-tube-building")
                key("F8")  # 在開站之前切回ST，避免呈現訊息遮住選單。
                key("b")
                state = capture("14-tube-list")
                assert state["screen"] == "tube-station"
                assert state["theme"] == "atari-st-original" and state["layout"] == "atari-st"
                # 保存庫返回會重設提示。按原版delay_button語意點擊訊息，
                # 點在標題下的提示列，避開任何目的地按鈕，再擷取清單。
                click(160, 45)
                shot("14-tube-st-list")
                bank_list, cash_list = state["bank"], state["cash"]
                key("5")
                rejected = capture("14-tube-hidden-rejected")
                assert rejected.get("tubePage", 0) == 0 and rejected.get("tubeTargetId", "") == ""
                assert rejected["bank"] == bank_list and rejected["cash"] == cash_list
                key("1")
                state = capture("15-tube-confirm")
                assert state["tubeTargetId"] == "obburg" and state["tubePage"] == 1
                assert state["colonySite"] == selected
                shot("15-tube-st-confirm")
                world = json.loads((root / "internal/source/atarist/assets/world-data.json").read_text())["tables"]
                price = world["systems"]["records"][0]["priceLevel"]
                departure = world["cities"]["records"][0]["position"]
                original_site = world["cities"]["records"][-1]["position"]
                def fare(site):
                    return 2*price*math.isqrt((site["x"]-departure["column"])**2+(site["y"]-departure["row"])**2)
                arrival = world["cities"]["records"][1]["position"]
                expected = fare({"x":arrival["column"],"y":arrival["row"]})
                old_fare = fare({"x":original_site["column"],"y":original_site["row"]})
                bank_before, cash_before = state["bank"], state["cash"]
                rolls_before = state["combatRolls"]
                key("Return")
                state = capture("16-tube-arrived")
                assert state["screen"] == "ground" and state["cityId"] == "obburg" and not state["groundWorld"]
                assert state["bank"] == bank_before-expected and state["cash"] == cash_before
                assert state["shipCityId"] == parked["shipCityId"] and state["shipPosition"] == parked["shipPosition"]
                assert state["cargoCityId"] == parked["cargoCityId"] and state["cargoPosition"] == parked["cargoPosition"]
                assert state["colonySite"] == selected and state["combatRolls"] == rolls_before
                shot("16-tube-arrived")
                key("q")
                process.wait(timeout=20)
                before = json.loads((out / "save.json").read_text())["state"]
                start(load=True)
                after = capture("17-tube-reopened")
                fields += ["navigationTargetId", "navigationView", "tubeTargetId", "tubePage"]
                assert all(before.get(k) == after.get(k) for k in fields), "Tube到達後保存重開不同"
                consumers = {"tubeFare": expected, "oldSiteInboundFare": old_fare, "priceLevel": price,
                             "hiddenDestinationRejected": True, "destination": "obburg",
                             "departure": departure, "selectedSite": selected["position"],
                             "bankBefore": bank_before, "bankAfter": state["bank"],
                             "cashBefore": cash_before, "cashAfter": state["cash"],
                             "combatRollsBefore": rolls_before, "combatRollsAfter": state["combatRolls"],
                             "stNavigation": "原版ST行星圖保持隱藏Banville；不因新址強行顯示",
                             "remakeNavigation": "同一保存城址的Remake行星圖",
                             "scope": "正常remake導航、新址保存、普通城市Tube、第五框拒絕；清單範圍另由原生／正式比較，Banville後期出口正常路徑仍待"}
            receipt = {"status": "PASS", "scope": "remake正常新局、角色建立、下船、鍵盤或ST滑鼠出城、呈現輪替、單檔與保存庫、新程序重開；加選site-consumers時包含導航兩種畫法及Tube車資／到達／重開。原版對拍另由受控候選收據驗證，未宣稱自然骰序或原版GUI一致", "siteInput": args.site_input, "consumers": consumers,
                       "checks": checks, "inputs": inputs, "reentryFields": fields,
                       "before": {k:before.get(k) for k in fields}, "after": {k:after.get(k) for k in fields},
                       "inputsSHA256": {name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary", "disk", "settings_input"]}}
            (out / "receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
            print(f"PASS:正常新局出城／呈現／單檔與保存庫／{len(fields)}欄位重開", flush=True)
            return
        for value in ["F5", "1", "l"]:
            key(value)
        state = capture("01-start")
        live_start = None
        def check_ground_resume(before_state, after_state):
            # The foreground resumes before F5 captures the loaded game.
            # Replay the known semantic VBLs rather than freeze TIME or slots.
            before_clock = before_state["awakeClock"]
            assert not before_clock["shipActive"]
            expected_clock = json.loads(json.dumps(before_clock))
            ticks = (after_state["awakeClock"]["source"]["ticks"]-before_clock["source"]["ticks"]) % 2**32
            assert 0 <= ticks < 600, "續行時計epoch或有界範圍不同"
            minutes = 0
            for _ in range(ticks):
                expected_clock["source"]["ticks"] = (expected_clock["source"]["ticks"]+1) % 2**32
                expected_clock["source"]["eventCountdown"] -= 1
                if expected_clock["source"]["eventCountdown"]:
                    continue
                expected_clock["source"]["eventCountdown"] = 4
                background = expected_clock["background"]
                background["waitEvents"] -= 1
                if background["waitEvents"]:
                    continue
                for slot in background["slots"]:
                    if slot["callback"] and slot["remaining"] == 0:
                        assert slot["callback"] == 1, "地面續行不應有船艦callback"
                        slot["remaining"] = slot["period"]
                        minutes += 1
                delay = min([8]+[slot["remaining"] for slot in background["slots"] if slot["callback"]])
                for slot in background["slots"]:
                    if slot["callback"]:
                        slot["remaining"] -= delay
                background["waitEvents"] = delay if delay else 65536
            assert after_state["awakeClock"] == expected_clock, "續行相位／槽倒數不同"
            assert after_state["timeMinutes"] == before_state["timeMinutes"]+minutes, "續行分鐘consumer不同"
            # A natural load may cross an hour. Keep strict comparison of
            # every saved field, including the actual condition consumers.
            replay = verify_awake_resume(root,out,before_state,after_state,"ground-"+str(len(checks)))
            return dict(advancedVBLs=ticks,minuteCallbacks=minutes,before=before_clock,after=expected_clock,replay=replay)
        if (args.phase0_supply or args.colony_first_visit) and not initial["awakeClock"]["shipActive"]:
            live_start = check_ground_resume(initial,state)
        expected_start = initial
        if args.phase0_supply and initial["awakeClock"]["shipActive"]:
            # A loaded ship continues running before F5 captures it. Retain
            # strict whole-state comparison by replaying observed VBLs through
            # the existing verified reducer, including phase and fuel writers.
            assert initial["screen"] in ["shipboard","navigation","cargo"] and not initial["sleep"]["active"]
            live_start = verify_awake_resume(root,out,initial,state,"ship")
            expected_start = live_start["expected"]
        elif live_start:
            expected_start = live_start["replay"]["expected"]
        for field in initial:
            if field not in ["messageKey", "saved", "quitRequested"] + (["awakeClock","timeMinutes"] if live_start and not initial["awakeClock"]["shipActive"] else []):
                assert state.get(field) == expected_start.get(field), "續行初態不同：" + field

        if args.phase0_supply or args.colony_first_visit:
            if args.supply_reentry:
                assert state["cityId"] == "banville" and state["screen"] == "trade" and state.get("tradePage",0) == 0
                fields = ["schema","screen","cityId","systemId","planetId","position","groundView","groundMode","holds","warehouses","colony","colonySite","tradeSession","tradePage","tradeSaleSlot","tradeSalePrice","tradePurchaseLot","tradeRolls","combatRolls","cash","bank","bankBalances","cargoAttached","cargoCityId","cargoPosition","shipCityId","shipPosition","fuel","shipSystems","parts","stores","vigor","rest","health","nourishment"]
                reentries = []
                for name, library in [("02-delivery-single-reopened",False),("03-delivery-library-reopened",True)]:
                    key("q")
                    process.wait(timeout=20)
                    before = json.loads((out/"save.json").read_text())["state"]
                    reference = json.loads((out/"library.json").read_text())["slots"]["slot-1"] if library else before
                    start(load=True)
                    if library:
                        for value in ["F5","1","l"]:
                            key(value)
                    after = capture(name)
                    clock = check_ground_resume(reference,after)
                    expected = clock["replay"]["expected"]
                    assert all(expected.get(k)==after.get(k) for k in fields), "交付後兩保存consumer不同"
                    reentries.append(dict(name=name,fields=fields,before={k:before.get(k) for k in fields},
                        reference={k:reference.get(k) for k in fields},expected={k:expected.get(k) for k in fields},
                        after={k:after.get(k) for k in fields},clock=clock))
                time.sleep(2.3)
                shot("03-delivery-library-reopened")
                receipt=dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,reentries=reentries,liveStart=live_start,
                    scope="同局schema44正常交付後保存，兩次新程序單檔／保存庫重開；以實際VBL核對完整consumer、分鐘與worker相位，35欄位按保存時計續行，四漢堡保持。正常購貨／返航／交付由上游同局收據保存，不再次交付。",
                    originalGUI="unverified",toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                    inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS:交付後兩次{len(fields)}欄位重開及分鐘／相位",flush=True)
                return
            wanted = [int(value) for value in args.supply_goods.split(",") if value.strip()]
            if args.supply_market_needed:
                assert wanted in [[],[31]], "實際需求選貨使用空貨種清單，或先取一批31後利用剩餘pod"
            if args.colony_first_visit or args.supply_rest_only or args.supply_eat_only:
                wanted = []
            else:
                allowed_goods = range(1,32) if args.colony_supply else [8,9,11,13]
                assert len(wanted) <= 2 and all((2 <= g <= 30 if args.supply_trade_only else g in allowed_goods and state["colony"]["remaining"][g] > 0) for g in wanted)
            source = json.loads((root / "internal/source/atarist/assets/world-data.json").read_text())
            runs = source["grouping"]["planetCityRuns"]
            city_records = source["tables"]["cities"]["records"]
            # ID沿目前來源名稱；只用來選正式導航，不改寫狀態。
            def city_id(name):
                return re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")

            city_record = next(row for row in city_records if city_id(row["name"]) == args.supply_city)
            if args.ftl_service_only or args.stores_eat_only:
                # Banville is the generated colony, outside planetCityRuns.
                # This scenario stays in its current city and never navigates.
                target_system, first_city = state["systemId"], state["cityId"]
            elif args.market_visit_only and args.supply_city == "banville":
                target_system, first_city = "jondd", "drahew"
            else:
                run_info = next(row for row in runs if row["start"] <= city_record["index"] < row["start"] + row["count"])
                target_system = run_info["system"].lower().replace(" ", "-")
                first_city = city_id(city_records[run_info["start"]]["name"])

            def dismiss_ground_dialog(state, name):
                was_dialog = state["screen"] == "dialog"
                if state["screen"] == "dialog":
                    assert state["groundReturn"] == "ground", "供貨路線遇到其他對話來源"
                    key("Escape")
                    state = capture(name+"-dialog-closed")
                if state["screen"] == "arena":
                    # Normal keyboard flee in the existing remake layout.
                    # Do not treat this as original XDOCOMBA escape parity or
                    # apply it to the ST real-time arena without its own check.
                    assert state["layout"] == "remake" and state["groundReturn"] == "ground"
                    before = state
                    key("x")
                    state = capture(name+"-arena-fled")
                    assert state["screen"] == "ground" and not state.get("arena", {}).get("active", False)
                    assert all(state.get(k) == before.get(k) for k in ["holds", "warehouses", "parts", "stores", "cash", "bank", "bankBalances", "colony", "colonySite", "health", "combatRolls", "tradeRolls"])
                if was_dialog:
                    assert state["screen"] == "ground"
                return state

            def walk_grid(state, goal, name):
                city_index = next(row["index"] for row in city_records if city_id(row["name"]) == state["cityId"])
                city_map = json.loads((root / "internal/source/atarist/assets/city-maps.json").read_text())["cities"][city_index]
                buildings = {(b["row"], b["column"]) for b in city_map["buildings"]}
                start = (state["position"]["x"], state["position"]["y"])
                ship = (state["shipPosition"]["x"], state["shipPosition"]["y"])
                surface = state["groundWorld"]
                # GroundCityCellVisible / CityExtent, already CONFORMED against
                # XMOVEONG: never apply the Phase0 visibility mask to later cities.
                colony_width = state["colony"]["phase"]
                if colony_width == 8:
                    colony_width = 9
                colony_height = (colony_width + 1) // 2
                blocked_cities = set()
                if surface:
                    planet_run = next(r for r in runs if r["planet"].lower().replace(" ","-")==state["planetId"])
                    markers = {(r["position"]["column"],r["position"]["row"]):city_id(r["name"]) for r in city_records[planet_run["start"]:planet_run["start"]+planet_run["count"]]}
                    if state["planetId"] == "jondd" and not state["colonySite"]["pending"]:
                        site = state["colonySite"]["position"]
                        markers[(site["x"],site["y"])] = "banville"
                    assert goal in markers, "地表目的座標必須是來源城市或保存殖民址"
                    target_city = markers[goal]
                    blocked_cities = set(markers)-{goal}
                queue, previous = deque([start]), {start: None}
                while queue and goal not in previous:
                    current = queue.popleft()
                    for value, dx, dy in [("Up",0,-1),("Right",1,0),("Down",0,1),("Left",-1,0)]:
                        nx, ny = ((current[0]+dx)%40,(current[1]+dy)%24) if surface else (current[0]+dx,current[1]+dy)
                        if not surface and not (min(-1,start[0],goal[0]) <= nx < max(30,start[0]+1,goal[0]+1) and min(-1,start[1],goal[1]) <= ny < max(18,start[1]+1,goal[1]+1)):
                            continue
                        position = (nx,ny)
                        if position in previous or position in blocked_cities or not surface and position == ship and position != goal:
                            continue
                        visible = state["cityId"] != "banville" or (8-colony_width < nx < 11+colony_width and 4-colony_height < ny < 7+colony_height)
                        if not surface and visible and 0 <= nx < 20 and 0 <= ny < 12 and position not in buildings and city_map["grid"][nx][ny] in [6,7,12,13,14]:
                            continue
                        previous[position] = (current,value)
                        queue.append(position)
                assert goal in previous, "供貨城市格找不到正式步行路線"
                route, end = [], goal
                while end != start:
                    parent,value = previous[end]
                    route.append(value)
                    end = parent
                for step,value in enumerate(reversed(route)):
                    key(value)
                    state = capture(name+f"-step-{step}")
                    state = dismiss_ground_dialog(state,name+f"-step-{step}")
                    assert state["screen"] in ["ground","shipboard"], "正常步行遇到其他畫面"
                state = capture(name)
                if surface:
                    assert not state["groundWorld"] and state["cityId"] == target_city, "正常地表未按來源標記返城"
                    return state
                assert (state["position"]["x"],state["position"]["y"]) == goal, "正常格子步行未抵達"
                return state

            def enter_supply_trade(state, name):
                # #15已接來源交易所slot2，按實際原始格正常步行。
                if state["cityId"] != "banville":
                    current_city = next(row for row in city_records if city_id(row["name"]) == state["cityId"])
                    city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][current_city["index"]]
                    exchange = next(b for b in city_map["buildings"] if b["slot"] == 2)
                    goal = (exchange["row"],exchange["column"])
                    if state["position"] != {"x":goal[0],"y":goal[1]}:
                        for step in range(3):
                            if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                                break
                            key("F8")
                            state = capture(name+f"-grid-{step}")
                        state = walk_grid(state,goal,name+"-exchange-position")
                    if state["layout"] == "remake":
                        key("F8")
                        state = capture(name+"-st")
                key("t")
                state = capture(name+"-trade")
                assert state["screen"] == "trade"
                return state

            def leave_trade(state, name):
                # Buy/Sell pages need one more Back than the main page.
                # Observe each normal transition; never assume two Escapes
                # always leave the entire exchange.
                for step in range(4):
                    if state["screen"] != "trade":
                        return state
                    page = state.get("tradePage",0)
                    key("Escape")
                    state = capture(name+f"-back-{step}")
                    assert state["screen"] != "trade" or state.get("tradePage",0) != page, "交易頁面返回沒有前進"
                assert state["screen"] != "trade", "正常返回未離開交易所"
                return state

            def refuel_main(state, name):
                held = json.loads(json.dumps(state["holds"]))
                warehouses = json.loads(json.dumps(state["warehouses"]))
                current_city = next(row for row in city_records if city_id(row["name"])==state["cityId"])
                maps = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"]
                if not any(b["slot"]==1 for b in maps[current_city["index"]]["buildings"]):
                    current_run = next(row for row in runs if row["start"] <= current_city["index"] < row["start"]+row["count"])
                    first = city_id(city_records[current_run["start"]]["name"])
                    assert any(b["slot"]==1 for b in maps[current_run["start"]]["buildings"]), "來源首城也沒有depot"
                    state = return_to_ship(state,name+"-no-depot-depart")
                    state = fly_to(first,first,current_run["system"].lower().replace(" ","-"),name+"-depot-city")
                if state["screen"] == "trade":
                    state = leave_trade(state,name+"-ground")
                for step in range(3):
                    if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                        break
                    key("F8")
                    state = capture(name+f"-grid-{step}")
                assert state["screen"] == "ground" and state["shipCityId"] == state["cityId"]
                current_city = next(row for row in city_records if city_id(row["name"])==state["cityId"])
                city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][current_city["index"]]
                depot = next(b for b in city_map["buildings"] if b["slot"]==1)
                state = walk_grid(state,(depot["row"],depot["column"]),name+"-depot-position")
                key("r")
                menu = capture(name+"-fuel-menu")
                assert menu["screen"] == "fuel-purchase" and menu["fuelPurchase"]["kind"] == "main"
                key("1")
                key("3")
                state = capture(name+"-main-filled")
                verify_main_refuel(menu, state)
                assert state["holds"] == held and state["warehouses"] == warehouses
                return state

            def return_to_ship(state, name):
                if state["screen"] == "cargo":
                    key("e")
                    key("space")
                    state = capture(name+"-cargo-disembarked")
                    assert state["screen"] == "ground" and state["groundMode"] == "pod", "正常貨艙未以pod下船"
                if state["screen"] == "navigation":
                    key("Escape")
                    state = capture(name+"-navigation-closed")
                    assert state["screen"] == "shipboard" and state["cargoAttached"], "正常導航返回未回到帶pod船艙"
                state = dismiss_ground_dialog(state,name)
                if state["groundWorld"]:
                    for step in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                            break
                        key("F8")
                        state = capture(name+f"-surface-grid-{step}")
                    if state["shipCityId"] == "banville":
                        marker = state["colonySite"]["position"]
                        goal = (marker["x"],marker["y"])
                    else:
                        city = next(r for r in city_records if city_id(r["name"]) == state["shipCityId"])
                        goal = (city["position"]["column"],city["position"]["row"])
                    state = walk_grid(state,goal,name+"-surface-return")
                if state["screen"] == "trade":
                    state = leave_trade(state,name+"-trade-closed")
                for step in range(3):
                    if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                        break
                    key("F8")
                    state = capture(name+f"-presentation-{step}")
                state = capture(name + "-ground")
                assert state["screen"] in ["ground","shipboard"] and state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles"
                # Banville Phase0下方一格為船；普通城市初港與交易同格。
                ship = state["shipPosition"]
                if state["cityId"] != "banville" and state["position"] != ship:
                    state = walk_grid(state,(ship["x"],ship["y"]),name+"-ship-position")
                for attempt in range(40):
                    if state["screen"] == "shipboard":
                        break
                    if state["position"] == ship:
                        key("Right")
                        key("Left")
                    elif state["position"]["y"] != ship["y"]:
                        key("Down" if state["position"]["y"] < ship["y"] else "Up")
                    else:
                        key("Right" if state["position"]["x"] < ship["x"] else "Left")
                    state = capture(name + f"-walk-{attempt}")
                    state = dismiss_ground_dialog(state,name+f"-walk-{attempt}")
                    assert state["screen"] in ["ground","shipboard"], "回船途中遇到其他畫面"
                assert state["screen"] == "shipboard" and state["cargoAttached"], "正常pod未回船"
                key("F8")
                return state

            def fly_to(city, first, system, name, disembark=True):
                key("n")
                state = capture(name + "-navigation")
                for attempt in range(12):
                    if system in state["navigationTarget"].lower().replace(" ", "-").split("／")[:1]:
                        break
                    key("bracketright")
                    state = capture(name + f"-system-{attempt}")
                else:
                    raise AssertionError("正式導航找不到供貨星系")
                desired_city = next(r for r in city_records if city_id(r["name"]) == first)
                desired_run = next(r for r in runs if r["start"] <= desired_city["index"] < r["start"]+r["count"])
                desired_ids = {city_id(r["name"]) for r in city_records[desired_run["start"]:desired_run["start"]+desired_run["count"]]}
                for attempt in range(4):
                    if state["navigationTargetId"] in desired_ids:
                        break
                    key("Down")
                    state = capture(name + f"-planet-{attempt}")
                else:
                    raise AssertionError("正式導航找不到供貨行星")
                for attempt in range(6):
                    if state["navigationTargetId"] == city:
                        break
                    key("Right")
                    state = capture(name + f"-city-{attempt}")
                else:
                    raise AssertionError("正式導航找不到供貨城市")
                key("Return")
                key("space")
                state = capture(name + "-departed")
                if state["screen"] == "flight":
                    key("e")  # 正式Engage，保持實際燃料及RNG，非直接到達。
                    key("space")
                    state = capture(name + "-engaged")
                assert state["screen"] == "shipboard" and state["shipCityId"] == city, "正常航行未抵達"
                if not disembark:
                    return state
                key("c")
                key("e")
                key("space")
                state = capture(name + "-pod-ground")
                assert state["groundMode"] == "pod" and state["cityId"] == city
                if city == "banville":
                    # 沿正常ST路線避開船體，到上方交易格。
                    for x,y,label in [(196,101,"east"),(196,137,"south"),(156,137,"west"),(156,56,"north"),(175,56,"exchange")]:
                        state = walk(x,y,name+"-"+label, expected_screen=("ground", "dialog"))
                        state = dismiss_ground_dialog(state,name+"-"+label)
                    assert state["position"] == {"x":9,"y":5}
                return enter_supply_trade(state,name)

            if args.colony_first_visit:
                cold_hold = dict(next(h for h in state["holds"] if h.get("goods") == 31))
                warehouses = json.loads(json.dumps(state["warehouses"]))
                if args.colony_first_visit_resume:
                    selected = state["colonySite"]
                else:
                    state = return_to_ship(state,"02-cold-depart")
                    state = fly_to("drahew","drahew","jondd","03-drahew")
                    key("Escape")
                    for step in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                            break
                        key("F8")
                        state = capture(f"04-site-presentation-{step}")
                    state = capture("04-site-ground")
                    assert state["screen"] == "ground" and state["cityId"] == "drahew"
                    key("Right")
                    key("Up")
                    state = capture("04-site-east-north")
                    for attempt in range(12):
                        key("Up")
                        key("Up")
                        state = capture(f"05-site-walk-{attempt}")
                        state = dismiss_ground_dialog(state,f"05-site-walk-{attempt}")
                        assert state["screen"] == "ground"
                        if state["groundWorld"]:
                            break
                    else:
                        raise AssertionError("同局正常出城未完成首次選址")
                    assert not state["colonySite"]["pending"]
                    selected = state["colonySite"]
                    assert next(h for h in state["holds"] if h.get("goods")==31) == cold_hold
                    key("Right")
                    key("Left")
                    state = capture("06-site-city-return")
                    state = dismiss_ground_dialog(state,"06-site-city-return")
                    assert not state["groundWorld"] and state["cityId"] == "drahew"
                    state = return_to_ship(state,"07-selected-board")
                    state = fly_to("banville","drahew","jondd","08-first-banville")
                phases = json.loads((root/"internal/source/atarist/assets/colony-phases.json").read_text())["phases"]
                assert state["colony"]["initialized"] and state["colony"]["phase"] == 0 and state["colony"]["nextPhase"] == 1
                assert state["colony"]["remaining"] == phases[0]["requiredCounts"] and state["colony"]["accumulated"] == 0
                assert state["colonySite"] == selected and state["warehouses"] == warehouses
                assert next(h for h in state["holds"] if h.get("goods")==31) == cold_hold
                initialized = state
                # 第一期尚未需要冷凍人。正常提交該槽應拒絕並保留內容。
                slot = next(i for i,h in enumerate(state["holds"]) if h.get("goods")==31)
                if not args.colony_first_visit_resume:
                    key("e")
                if state.get("tradePage",0) != 9:
                    for value in ["s","2",str(slot+1)]:
                        key(value)
                confirmed = capture("09-phase0-cold-zero-demand")
                assert confirmed["tradePage"] == 9 and confirmed.get("tradeSalePrice",0) == 0
                assert confirmed["colony"]["remaining"][31] == 0
                key("1")
                rejected = capture("09-phase0-cold-approval-unavailable")
                assert rejected["tradePage"] == 9 and rejected["tradeSaleSlot"] == confirmed["tradeSaleSlot"]
                key("2")
                rejected = capture("09-phase0-cold-withdrawn")
                assert rejected["colony"] == initialized["colony"] and rejected["holds"] == initialized["holds"]
                assert rejected["tradeSession"] == initialized["tradeSession"] and rejected.get("tradePage",0) == 0
                shot("09-phase0-cold-withdrawn")
                fields = ["schema","screen","cityId","systemId","planetId","position","groundView","groundMode","holds","warehouses","colony","colonySite","tradeSession","tradePage","tradeSaleSlot","tradeSalePrice","tradePurchaseLot","tradeRolls","combatRolls","cash","bank","bankBalances","cargoAttached","cargoCityId","cargoPosition","shipCityId","shipPosition","fuel","timeMinutes","shipSystems"]
                key("q")
                process.wait(timeout=20)
                before = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                after = capture("10-first-visit-reopened")
                fields.remove("timeMinutes")
                clock_reentry = check_ground_resume(before,after)
                assert all(before.get(k)==after.get(k) for k in fields)
                assert next(h for h in after["holds"] if h.get("goods")==31) == cold_hold
                receipt={"status":"PASS","scope":"承接同一目前schema正常取得冷凍人的新局，pod回船／返Drahew／出城首次選址／返船／Banville初訪需求／第一期拒收31及新程序重開；不注入、不重擲。第一期換期及第二期31交付仍待。原版GUI未驗。","coldHold":cold_hold,"selectedSite":selected,"firstPhase":initialized["colony"],"rejectionMessage":rejected.get("messageKey"),"resumedFirstVisit":args.colony_first_visit_resume,"checks":checks,"inputs":inputs,"reentryFields":fields,"before":{k:before.get(k) for k in fields},"after":{k:after.get(k) for k in fields},"toolSHA256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),"inputsSHA256":{name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]}}
                receipt["liveStart"] = live_start
                receipt["clockReentry"] = clock_reentry
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS:同局首次選址／Banville需求／拒收31／{len(fields)}欄位重開",flush=True)
                return

            if args.stores_eat_only:
                assert state["cityId"] == "banville"
                held = eaten = None
                if not args.stores_eat_resume:
                    assert state["screen"] == "trade"
                    state = return_to_ship(state,"02-food-board")
                    key("b")
                    stores = capture("03-food-stores")
                    slot = next(i for i,item in enumerate(stores["stores"]) if item and item & 31 == 8)
                    for attempt in range(20):
                        if stores["selectedPartIndex"] == slot:
                            break
                        key("Right")
                        stores = capture(f"04-food-slot-{attempt}")
                    key("Return")
                    held = capture("05-food-held")
                    assert held["handItem"] & 31 == 8 and held["stores"][slot] == 0
                    key("e")
                    eaten = capture("06-food-eaten")
                    assert not eaten["handItem"] and eaten["nourishment"] == min(255,held["nourishment"]+70)
                    assert eaten["cash"] == held["cash"] and eaten["bank"] == held["bank"]
                    for value in ["Escape","c","e","space"]:
                        key(value)
                    state = capture("07-food-pod-ground")
                assert state["screen"] == "ground" and state["groundMode"] == "pod"
                if state["layout"] != "remake":
                    key("F8")
                    state = capture("07-food-grid")
                assert state["layout"] == "remake"
                state = walk_grid(state,(9,5),"08-food-exchange-cell")
                assert state["position"] == {"x":9,"y":5}
                key("F8")
                key("t")
                state = capture("09-food-trade")
                assert state["screen"] == "trade" and state["layout"] == "atari-st"
                key("q")
                process.wait(timeout=20)
                before = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                after = capture("10-food-reopened")
                fields = ["schema","screen","cityId","systemId","position","holds","stores","parts","colony","tradeSession","cash","bank","bankBalances","cargoAttached","cargoCityId","shipCityId"]
                assert all(before[k] == after[k] for k in fields)
                receipt = dict(status="PASS",scope="正常Banville交易保存回船、Stores取出既存漢堡及食用、pod返回交易所與16欄保存重開；不注入食物、資金或重設seed。",checks=checks,inputs=inputs,
                    held=held,eaten=eaten,reentryFields=fields,before={k:before[k] for k in fields},after={k:after[k] for k in fields},
                    toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print("PASS:正常Stores既存漢堡／返回交易所／16欄重開",flush=True)
                return

            if args.ftl_service_only:
                assert state["cityId"] == "banville" and state["screen"] == "trade"
                state = leave_trade(state,"02-ftl-ground")
                for step in range(2):
                    if state["layout"] == "remake":
                        break
                    key("F8")
                    state = capture(f"03-ftl-grid-{step}")
                # Resolve the decoded depot, then exercise the real visibility
                # and facility gates through the normal input path.
                city_index = next(row["index"] for row in city_records if city_id(row["name"]) == "banville")
                city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][city_index]
                building = next(b for b in city_map["buildings"] if b["slot"] == 1)
                depot = (building["row"],building["column"])
                state = walk_grid(state,depot,"04-ftl-depot-position")
                key("F8")
                state = capture("05-ftl-st")
                key("f")
                assert capture("06-ftl-depot-menu")["screen"] == "depot"
                shot("06-ftl-depot-menu")
                key("2")
                repair = capture("06-ftl-repair-menu")
                assert repair["screen"] == "fuel-purchase" and repair["fuelPurchase"]["kind"] == "hull"
                shot("06-ftl-repair-menu")
                key("q")
                process.wait(timeout=20)
                repair_saved = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                repair_resumed = capture("06-ftl-repair-reopened")
                repair_fields = ["schema","screen","groundReturn","hullCondition","fuelPurchase","fuel","cash","bank","bankBalances","holds","warehouses","colony"]
                assert all(repair_saved[k] == repair_resumed[k] for k in repair_fields), "正常修船子頁保存不同"
                key("3")
                repair_closed = capture("06-ftl-repair-closed")
                assert repair_closed["screen"] == "depot" and repair_closed["hullCondition"] == repair["hullCondition"]
                assert all(repair_closed[k] == repair[k] for k in ["fuel","cash","bank","bankBalances"])
                key("4")
                quote = capture("07-ftl-quote")
                assert quote["screen"] == "ftl-boost" and quote["layout"] == "atari-st"
                price = (290-(quote["colony"]["phase"]-5)*30)*1000
                assert quote["jumpRange"] == 8
                assert (max(0,quote["bank"])+quote["cash"] >= price) == args.ftl_install
                click(160,45)  # 正常點掉保存庫返回提示，避開下方安裝按鈕。
                run(["import", "-window", window, str(out/"07-ftl-quote.png")])
                click(160,100)  # 已核對報價圖中的安裝按鈕。
                result_label = "installed" if args.ftl_install else "insufficient"
                run(["import", "-window", window, str(out/f"08-ftl-{result_label}-live.png")])
                denied = capture("08-ftl-"+result_label)
                if args.ftl_install:
                    bank_spend = min(max(0,quote["bank"]),price)
                    assert denied["jumpRange"] == 9
                    assert denied["cash"] == quote["cash"]-price+bank_spend and denied["bank"] == quote["bank"]-bank_spend
                    key("1")
                    repeated = capture("08-ftl-repeated")
                    assert all(repeated[k] == denied[k] for k in ["jumpRange","cash","bank","bankBalances"])
                else:
                    assert denied["jumpRange"] == 8
                    assert denied["cash"] == quote["cash"] and denied["bank"] == quote["bank"]
                key("q")
                process.wait(timeout=20)
                before = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                after = capture("09-ftl-reopened")
                fields = ["schema","screen","cityId","systemId","position","jumpRange","cash","bank","bankBalances","holds","colony","shipCityId","shipPosition","fuelPurchase","groundReturn","hullCondition","fuel"]
                assert all(before[k] == after[k] for k in fields)
                click(160,45)
                run(["import", "-window", window, str(out/"09-ftl-reopened.png")])
                key("3")
                final = capture("10-ftl-cancelled")
                assert final["screen"] == "depot" and final["jumpRange"] == (9 if args.ftl_install else 8)
                key("Escape")
                final = capture("10-ftl-ground-returned")
                assert final["screen"] == "ground"
                if final["cityId"] == "banville":
                    key("F8")
                    final = capture("11-ftl-return-grid")
                    assert final["layout"] == "remake"
                    final = walk_grid(final,(9,5),"11-ftl-exchange-position")
                    key("F8")
                    final = capture("11-ftl-return-st")
                final = enter_supply_trade(final,"11-ftl-trade-return")
                receipt = dict(status="PASS", scope="正常Banville交易保存續行，Apple II格移動至depot後切回Atari ST，正常FTL報價、資金不足、14欄保存重開與取消；未成功安裝、不注入資金、原版完整GUI未驗。", price=price,
                    checks=checks,inputs=inputs,reentryFields=fields,before={k:before[k] for k in fields},after={k:after[k] for k in fields},liveStart=live_start,
                    toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                receipt.update(installed=args.ftl_install,repair=dict(opened=repair,closed=repair_closed,reopened=repair_resumed,reentryFields=repair_fields),final=final,
                    scope="正常Banville保存續行至depot父選單、修船報價／關閉、工程報價及實際確認、14欄保存重開、返回父選單／交易所；不注入資金、不改開局航距。成功安裝="+str(args.ftl_install))
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS:正常FTL報價／{result_label}／{len(fields)}欄位重開／返回",flush=True)
                return

            if args.supply_eat_only:
                state = leave_trade(state,"02-food-ground")
                key("h")
                body = capture("03-food-body")
                assert body["screen"] == "character-status" and not body.get("handItem")
                pocket = next(i for i, item in enumerate(body["parts"]) if item and item & 31 == 8)
                for attempt in range(4):
                    if body["selectedPartIndex"] == pocket:
                        break
                    key("Right")
                    body = capture(f"03-food-pocket-{attempt}")
                assert body["selectedPartIndex"] == pocket
                key("h")
                held = capture("04-food-held")
                expected_parts = list(body["parts"])
                expected_parts[pocket] = 0
                assert held["handItem"] == body["parts"][pocket] and held["handOrigin"] == dict(kind="pocket",index=pocket)
                assert held["parts"] == expected_parts and held["awakeClock"]["minutePause"] == 60
                key("e")
                eaten = capture("05-food-eaten")
                assert eaten["handItem"] == 0 and eaten["handOrigin"] == dict(kind="",index=0)
                assert eaten["nourishment"] == min(255,held["nourishment"]+70), "正常漢堡營養讀數不同"
                stable = ["parts","stores","holds","warehouses","colony","colonySite","cash","bank","bankBalances","tradeRolls","combatRolls","fuel","shipSystems"]
                assert all(eaten[k] == held[k] for k in stable), "正常進食改動貨物／資金或殖民進度"
                shot("05-food-eaten")
                key("Escape")
                state = capture("06-food-ground")
                state = enter_supply_trade(state,"07-food-return")
                key("q")
                process.wait(timeout=20)
                before = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                after = capture("08-food-reopened")
                clock_reentry = check_ground_resume(before,after)
                receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,
                    food=dict(pocket=pocket,before=held,after=eaten,stableFields=stable),
                    before=before,after=after,liveStart=live_start,clockReentry=clock_reentry,
                    scope="同局交易保存正常離開、角色面板手持並食用一個已攜漢堡、正常返回交易所與新程序重開；不買貨、不飛航、不注入。",
                    originalGUI="unverified",ruleEvidence="banville-restoration-spec.md#stores-取物與漢堡規則-ready",
                    toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                    inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS:正常漢堡{held['nourishment']}→{eaten['nourishment']}及完整保存續行",flush=True)
                return

            if args.supply_trade_only:
                colony_before = json.loads(json.dumps(state["colony"]))
                warehouses_before = json.loads(json.dumps(state["warehouses"]))
                score_before = state["banvilleCargo"]
                acquired = []
                key("e")
                click(220,150)
                key("b")
                quoted = capture("02-merchant-quotes")
                for goods in wanted:
                    assert any(lot["goods"] == goods for lot in quoted["quotes"]), "正常報價未列出所選一般貨物"
                    for attempt in range(8):
                        lot = quoted["quotes"][quoted.get("quoteIndex",0)]
                        if lot["goods"] == goods:
                            break
                        key("p")
                        quoted = capture(f"02-merchant-quote-{goods}-{attempt}")
                    assert lot["goods"] == goods and lot["price"]*100 <= quoted["tradeSession"]["funds"]
                    key("b")
                    paid = capture(f"03-merchant-paid-{goods}")
                    assert paid["tradePage"] == 10 and paid["tradePurchaseLot"]["goods"] == goods
                    previous_holds = json.loads(json.dumps(paid["holds"]))
                    key("2")
                    state = capture(f"04-merchant-acquired-{goods}")
                    added = [dict(hold) for i,hold in enumerate(state["holds"]) if hold != previous_holds[i]]
                    assert len(added) == 1 and added[0].get("goods") == goods, "正常購買沒有精確新增一個貨艙記錄"
                    acquired.append(added[0])
                    click(220,150)
                    key("b")
                    quoted = capture(f"04-merchant-next-{goods}")
                key("Escape")
                state = capture("05-merchant-source-result")
                state = return_to_ship(state,"06-merchant-depart")
                state = fly_to(args.supply_city,first_city,target_system,"07-merchant-arrive")
                key("e")
                sales = []
                for lot in acquired:
                    slot = next(i for i,h in enumerate(state["holds"]) if h == lot)
                    for value in ["s","2",str(slot+1)]:
                        key(value)
                    pending = capture(f"08-merchant-confirm-{lot['goods']}")
                    assert pending["tradePage"] == 9 and pending["tradeSalePrice"] > 0
                    key("1")
                    state = capture(f"09-merchant-sold-{lot['goods']}")
                    assert not state["holds"][slot].get("goods")
                    assert state["tradeSession"]["funds"] == pending["tradeSession"]["funds"]+pending["tradeSalePrice"]*100
                    sales.append(dict(lot=lot,price=pending["tradeSalePrice"],before=pending,after=state))
                    click(220,150)
                last_funds = state["tradeSession"]["funds"]
                state = leave_trade(state,"10-merchant-settled")
                assert state["cash"]+max(0,state["bank"]) == last_funds
                assert state["colony"] == colony_before and state["warehouses"] == warehouses_before and state["banvilleCargo"] == score_before
                funding = None
                if args.merchant_withdraw:
                    current_city = next(row for row in city_records if city_id(row["name"]) == state["cityId"])
                    city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][current_city["index"]]
                    tellers = [(x,y) for x,row in enumerate(city_map["grid"]) for y,code in enumerate(row) if code == 15]
                    assert tellers, "成交城市沒有原始碼15銀行格"
                    goal = min(tellers,key=lambda p: (abs(p[0]-state["position"]["x"])+abs(p[1]-state["position"]["y"]),p))
                    for step in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                            break
                        key("F8")
                        state = capture(f"10-merchant-bank-grid-{step}")
                    state = walk_grid(state,goal,"10-merchant-teller-position")
                    key("F8")
                    key("u")
                    teller = capture("10-merchant-teller")
                    assert teller["screen"] == "uniteller" and teller["bank"] > 0
                    balances = dict(teller["bankBalances"])
                    balances[teller["systemId"]] = 0
                    for value in ["w","a","Escape"]:
                        key(value)
                    funded = capture("10-merchant-withdrawn")
                    assert funded["cash"] == last_funds and funded["bank"] == 0 and funded["bankBalances"] == balances
                    funding = dict(before=teller,after=funded)
                    key("Escape")
                    state = capture("10-merchant-bank-ground")
                    if args.merchant_refuel and state["fuel"] < 180:
                        state = refuel_main(state,"10-merchant-refuel")
                    state = enter_supply_trade(state,"10-merchant-market-return")
                shot("10-merchant-settled")
                key("q")
                process.wait(timeout=20)
                before = json.loads((out/"save.json").read_text())["state"]
                start(load=True)
                after = capture("11-merchant-reopened")
                clock_reentry = check_ground_resume(before,after)
                receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,
                    acquired=acquired,sales=sales,funding=funding,before=before,after=after,liveStart=live_start,clockReentry=clock_reentry,
                    scope="同局目前報價正常買入一般貨物、回船、飛至普通城市、正常售出及結算、新程序續行；殖民進度／冷凍倉庫保持，不注入、不重抽報價。",
                    originalGUI="unverified",originalNormalSale="待原生同狀態比較",
                    toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                    inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print(f"PASS:一般貨物{len(sales)}批正常售出／Cash{after['cash']}／Bank{after['bank']}及完整續行",flush=True)
                return

            acquired = []
            warehouse_fetches = []
            if args.supply_return:
                state = dismiss_ground_dialog(state,"03-return-dialog-closed")
                acquired = [dict(next(h for h in state["holds"] if h.get("goods") == goods)) for goods in wanted]
                if args.supply_refuel_return:
                    state = refuel_main(state,"03-return-refuel")
                if state["cityId"] == "banville" and state["screen"] == "ground":
                    for step in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                            break
                        key("F8")
                        state = capture(f"03-return-resumed-presentation-{step}")
                    if state["groundWorld"]:
                        site = state["colonySite"]["position"]
                        state = walk_grid(state,(site["x"],site["y"]),"03-return-surface-colony")
                    state = walk_grid(state,(9,5),"03-return-resumed-exchange")
                    key("F8")
                    state = enter_supply_trade(state,"03-return-resumed")
            elif args.supply_landed:

                if state["screen"] == "shipboard":
                    for value in ["c","e","space"]:
                        key(value)
                    state = capture("03-supply-arrived-pod-ground")
                    assert state["screen"] == "ground" and state["groundMode"] == "pod" and state["cityId"] == args.supply_city, "正常已到站下船未抵達同城pod地面"
                if args.supply_revisit:
                    for value in (["Escape"] if state.get("tradePage",0)==3 else ["Escape","Escape"]):
                        key(value)
                    state = capture("03-supply-revisit-ground")
                    assert state["screen"] == "ground" and state["cityId"] == args.supply_city
                if state["screen"] == "ground":
                    state = enter_supply_trade(state,"03-supply-resumed")
                else:
                    assert state.get("tradePage",0) in [0,3], "庫存續行須正常交易入口或Main"
            else:
                state = return_to_ship(state, "02-depart-city")
                if args.supply_rest_only:
                    ready = capture("02-rest-shipboard")
                    assert ready["screen"] == "shipboard" and not ready["sleep"]["active"] and ready["rest"] > 0
                    key("h")
                    # Let the ST zoom's transient message expire before observing
                    # the actual readouts; saving would create another message.
                    inputs.append(dict(waitSeconds=3,reason="ST-character-message-expiry"))
                    time.sleep(3)
                    shot("03-rest-character-before-save")
                    panel = capture("03-rest-character")
                    assert panel["screen"] == "character-status" and panel["statusReturn"] == "shipboard"
                    key("r")
                    active = capture("04-rest-active")
                    base_time = active["timeMinutes"]-active["sleep"]["elapsedMinutes"]
                    base_rest = active["rest"]-active["sleep"]["elapsedMinutes"]
                    assert 0 < base_rest <= 255 and ready["timeMinutes"] <= base_time < ready["timeMinutes"]+10
                    stable = ["schema","screen","statusReturn","cityId","systemId","planetId","position",
                        "groundMode","groundView","holds","warehouses","colony","colonySite","parts","stores",
                        "handItem","handOrigin","cash","bank","bankBalances","tradeClassShift","tradeRolls",
                        "combatRolls","cargoAttached","cargoCityId","cargoPosition","shipCityId","shipPosition"]
                    sleep_reentries = []

                    def check_rest_sleep(s):
                        b = s["sleep"]
                        assert s["screen"] == "character-status" and s["statusReturn"] == "shipboard"
                        assert b["active"] and b["shipActive"] and not b["forced"] and not b["wakeRequested"]
                        assert b["shipCycles"] == (b["elapsedMinutes"]*60+b["minuteCycles"]) % 32
                        assert s["timeMinutes"] == base_time+b["elapsedMinutes"]
                        assert s["rest"] == min(255,base_rest+b["elapsedMinutes"])
                        assert s["awakeClock"]["minutePause"] == 60 and s["awakeClock"]["shipPause"] == 32
                        assert all(s.get(k) == active.get(k) for k in stable), "休息改動貨物、資金或供貨狀態"

                    check_rest_sleep(active)
                    shot("04-rest-active")
                    previous = active
                    for name, library in [("05-rest-single-reopened",False),("06-rest-library-reopened",True)]:
                        key("F10")
                        process.wait(timeout=20)
                        single = json.loads((out/"save.json").read_text())["state"]
                        reference = json.loads((out/"library.json").read_text())["slots"]["slot-1"] if library else single
                        check_rest_sleep(single)
                        start(load=True)
                        if library:
                            for value in ["F5","1","l"]:
                                key(value)
                        resumed = capture(name)
                        check_rest_sleep(resumed)
                        b, a = reference["sleep"], resumed["sleep"]
                        cycles = (a["elapsedMinutes"]-b["elapsedMinutes"])*60+a["minuteCycles"]-b["minuteCycles"]
                        assert 0 <= cycles < 600, "睡眠讀檔重置或跳過計數"
                        assert a["shipCycles"] == (b["shipCycles"]+cycles) % 32
                        assert resumed["shipPhase"] == (reference["shipPhase"]+(b["shipCycles"]+cycles)//32) % 3
                        sleep_reentries.append(dict(name=name,library=library,advancedCycles=cycles,before=reference,after=resumed))
                        previous = resumed
                    deadline = time.monotonic()+1200
                    attempt = 0
                    while previous["rest"] < 255:
                        if time.monotonic() >= deadline:
                            raise RuntimeError("正常休息有界等候未到Rest255，保留最新睡眠保存")
                        time.sleep(10)
                        attempt += 1
                        previous = capture(f"07-rest-progress-{attempt:03d}")
                        check_rest_sleep(previous)
                        print(f"REST {previous['rest']}/255 elapsed={previous['sleep']['elapsedMinutes']}",flush=True)
                    shot("08-rest-full")
                    key("space")
                    inputs.append(dict(waitSeconds=3,reason="ST-awake-message-expiry"))
                    time.sleep(3)
                    shot("09-rest-awake-before-save")
                    awake = capture("09-rest-awake")
                    assert awake["screen"] == "character-status" and not awake["sleep"]["active"]
                    assert awake["sleep"] == dict(active=False,forced=False,wakeRequested=False,minuteCycles=0,shipCycles=0,elapsedMinutes=0,shipActive=False)
                    assert all(awake.get(k) == active.get(k) for k in stable) and awake["rest"] == 255
                    key("Escape")
                    final = capture("10-rest-ready-shipboard")
                    assert final["screen"] == "shipboard" and final["cargoAttached"] and not final["sleep"]["active"]
                    shot("10-rest-ready-shipboard")
                    for value in ["c","e","space"]:
                        key(value)
                    ready_trade = capture("11-rest-pod-ground")
                    assert ready_trade["screen"] == "ground" and ready_trade["groundMode"] == "pod"
                    if ready_trade["cityId"] == "banville":
                        # The same verified ST route used after normal arrival.
                        for x,y,label in [(196,101,"east"),(196,137,"south"),(156,137,"west"),(156,56,"north"),(175,56,"exchange")]:
                            ready_trade = walk(x,y,"11-rest-"+label,expected_screen=("ground","dialog"))
                            ready_trade = dismiss_ground_dialog(ready_trade,"11-rest-"+label)
                        assert ready_trade["position"] == {"x":9,"y":5}
                    ready_trade = enter_supply_trade(ready_trade,"12-rest-source")
                    assert ready_trade["colony"] == final["colony"] and ready_trade["holds"] == final["holds"]
                    assert ready_trade["warehouses"] == final["warehouses"] and ready_trade["parts"] == final["parts"] and ready_trade["stores"] == final["stores"]
                    # Entering trade moves the positive current bank into Funds.
                    # Keep other accounts and verify the complete saved session.
                    expected_balances = dict(final["bankBalances"])
                    if final["bank"] > 0:
                        expected_balances[final["systemId"]] = 0
                    assert ready_trade["cash"] == final["cash"] and ready_trade["bankBalances"] == expected_balances
                    assert ready_trade["bank"] == min(0,final["bank"])
                    assert ready_trade["tradeSession"] == dict(active=True,cityId=final["cityId"],systemId=final["systemId"],
                        initialCash=final["cash"],initialBank=final["bank"],funds=final["cash"]+max(0,final["bank"]))
                    shot("12-rest-ready-trade")
                    receipt = dict(status="PASS",saveSchema=schema,checks=checks,inputs=inputs,
                        sleepReentries=sleep_reentries,sleepStableFields=stable,liveStart=live_start,
                        scope="同局供貨交易保存正常回船、角色面板睡至Rest255、睡眠中單檔與保存庫各一次新程序重開及正常醒來返回來源交易所；貨物／資金／倉庫／殖民進度保持，不食用、不飛航、不注入。",
                        originalGUI="unverified",sleepWallClock="presentation-approximation",
                        ruleEvidence="banville-restoration-spec.md#角色面板睡眠與保存-conformed",
                        baseTime=base_time,baseRest=base_rest,final=final,readyTrade=ready_trade,
                        screenshotsBeforeSave=["03-rest-character-before-save.png","09-rest-awake-before-save.png"],
                        toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                        inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                    (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                    print(f"PASS:供貨主線正常船上休息Rest{base_rest}→255／兩保存睡眠續行",flush=True)
                    return
                if args.supply_refuel_before:
                    state = fly_to("drahew","drahew","jondd","02-refuel-drahew")
                    state = refuel_main(state,"02-refuel-main")
                    state = return_to_ship(state,"02-refuel-depart")
                if args.supply_withdraw_before:
                    held = json.loads(json.dumps(state["holds"]))
                    warehouses = json.loads(json.dumps(state["warehouses"]))
                    # The ship may be parked outside the city grid. Walk to a
                    # decoded teller tile as the funding-only path does; the
                    # former Tube shortcut depends on the landing position.
                    if state["screen"] == "shipboard":
                        key("c")
                        key("e")
                        key("space")
                        state = capture("02-funding-disembarked")
                    assert state["screen"] == "ground" and not state["groundWorld"]
                    current_city = next(row for row in city_records if city_id(row["name"]) == state["cityId"])
                    city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][current_city["index"]]
                    tellers = [(x,y) for x,row in enumerate(city_map["grid"]) for y,code in enumerate(row) if code == 15]
                    assert tellers, "出發城市沒有原始碼15銀行格"
                    goal = min(tellers,key=lambda p: (abs(p[0]-state["position"]["x"])+abs(p[1]-state["position"]["y"]),p))
                    for step in range(3):
                        if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                            break
                        key("F8")
                        state = capture(f"02-funding-grid-{step}")
                    state = walk_grid(state,goal,"02-funding-teller-position")
                    key("F8")
                    state = capture("02-funding-st")
                    key("u")
                    teller = capture("02-funding-teller")
                    assert teller["screen"] == "uniteller" and teller["layout"] == "atari-st" and teller["bank"] > 0
                    for value in ["w","a","Escape"]:
                        key(value)
                    funded = capture("02-funding-withdrawn")
                    assert funded["bank"] == 0 and funded["cash"] == teller["cash"]+teller["bank"]
                    assert funded["holds"] == held and funded["warehouses"] == warehouses
                    key("Escape")
                    state = capture("02-funding-ground")
                    state = return_to_ship(state,"02-funding-board")
                if args.supply_prep:
                    state = fly_to("drahew","drahew","jondd","02-prep-drahew")
                    for value in ["Escape","F8","F8"]:
                        key(value)
                    state = capture("02-prep-grid")
                    assert state["screen"] == "ground" and state["layout"] == "remake"
                    depot = next(b for b in json.loads((root / "internal/source/atarist/assets/city-maps.json").read_text())["cities"][0]["buildings"] if b["slot"] == 1)
                    state = walk_grid(state,(depot["row"],depot["column"]),"02-prep-depot")
                    key("r")
                    main_fuel = capture("02-prep-main-menu")
                    assert main_fuel["screen"] == "fuel-purchase"
                    key("1")
                    key("3")
                    state = capture("02-prep-main-filled")
                    assert state["fuel"] == 255, "正常主油箱未補滿"
                    key("o")
                    aux_fuel = capture("02-prep-aux-menu")
                    assert aux_fuel["screen"] == "fuel-purchase" and all(not h.get("goods") and not h.get("kind") for h in state["holds"])
                    key("2")
                    key("3")
                    state = capture("02-prep-aux-bought")
                    assert sum(h.get("kind")=="fuel" for h in state["holds"]) == 1
                    state = walk_grid(state,(state["shipPosition"]["x"],state["shipPosition"]["y"]),"02-prep-boarded")
                    assert state["screen"] == "shipboard" and state["cargoAttached"]
                    key("F8")
                    for value in ["c","e","space","u"]:
                        key(value)
                    teller = capture("02-prep-teller")
                    assert teller["screen"] == "uniteller" and teller["layout"] == "atari-st"
                    for value in ["w","a","Escape"]:
                        key(value)
                    withdrawn = capture("02-prep-withdrawn")
                    assert withdrawn["bank"] == 0 and withdrawn["cash"] == teller["cash"]+teller["bank"]
                    key("Escape")
                    key("t")
                    key("e")
                    click(220,150)
                    state = capture("02-prep-ready-trade")
                    assert state["screen"] == "trade"
                    state = return_to_ship(state,"02-prep-depart-ready")
                state = fly_to(args.supply_city, first_city, target_system, "03-supply")
            funding = None
            if args.market_refuel:
                state = refuel_main(state,"03-market-refuel")
                assert state["cityId"] == args.supply_city, "市場補油須指定有depot的目的城市"
                state = enter_supply_trade(state,"03-market-refueled")
            if args.supply_funding_only:
                state = leave_trade(state,"04-funding-ground")
                current_city = next(row for row in city_records if city_id(row["name"]) == state["cityId"])
                city_map = json.loads((root/"internal/source/atarist/assets/city-maps.json").read_text())["cities"][current_city["index"]]
                tellers = [(x,y) for x,row in enumerate(city_map["grid"]) for y,code in enumerate(row) if code == 15]
                assert tellers, "來源城市沒有原始碼15銀行格"
                goal = min(tellers,key=lambda p: (abs(p[0]-state["position"]["x"])+abs(p[1]-state["position"]["y"]),p))
                for step in range(3):
                    if state["layout"] == "remake" and state["tileset"] == "apple-ii-tiles":
                        break
                    key("F8")
                    state = capture(f"04-funding-grid-{step}")
                state = walk_grid(state,goal,"04-funding-teller-position")
                key("F8")
                state = capture("04-funding-st")
                key("u")
                teller = capture("04-funding-teller")
                assert teller["screen"] == "uniteller" and teller["layout"] == "atari-st" and teller["bank"] > 0
                balances = dict(teller["bankBalances"])
                balances[teller["systemId"]] = 0
                for value in ["w","a","Escape"]:
                    key(value)
                funded = capture("04-funding-withdrawn")
                assert funded["bank"] == 0 and funded["cash"] == teller["cash"]+teller["bank"]
                assert funded["bankBalances"] == balances
                stable = ["holds","warehouses","colony","colonySite","parts","stores","tradeRolls","combatRolls"]
                assert all(funded[k] == teller[k] for k in stable), "來源提款改動貨物或進度"
                funding = dict(before=teller,after=funded,stableFields=stable)
                key("Escape")
                state = capture("04-funding-ground-returned")
                if args.supply_refuel_return:
                    state = refuel_main(state,"05-funding-refuel")
                state = enter_supply_trade(state,"06-funded-source")
            if not args.supply_return:
                if 31 in wanted and not args.supply_funding_only:
                    # Existing CONFORMED Fetch: normal keys only, preserving the
                    # actual source batch and clearing precisely its warehouse slot.
                    if state.get("tradePage", 0) == 0:
                        key("Escape")
                        state = capture("04-cryo-warehouse-entrance")
                    assert state.get("tradePage", 0) == 3
                    original = state = capture("04-cryo-warehouse-before")
                    wh_slot = next(i for i, lot in enumerate(state["warehouses"][state["cityId"]]) if lot["goods"] == 31)
                    pod_slot = next(i for i, hold in enumerate(state["holds"]) if not hold.get("kind"))
                    lot = dict(state["warehouses"][state["cityId"]][wh_slot])
                    for value in ["w", "f", str(wh_slot + 1)]:
                        key(value)
                    state = capture("04-cryo-warehouse-fetched")
                    assert all(state["holds"][pod_slot].get(k, 0) == v for k, v in lot.items())
                    expected_warehouses = json.loads(json.dumps(original["warehouses"]))
                    expected_warehouses[state["cityId"]][wh_slot] = dict(goods=0, quality=0, price=0, sourceCode=0)
                    assert state["warehouses"] == expected_warehouses
                    assert all(state["holds"][i] == hold for i, hold in enumerate(original["holds"]) if i != pod_slot)
                    assert all(state.get(k) == original.get(k) for k in ["cash", "bank", "bankBalances", "tradeSession", "tradeRolls", "combatRolls", "colony", "colonySite"])
                    acquired.append(dict(state["holds"][pod_slot]))
                    warehouse_fetches.append(dict(city=state["cityId"], warehouseSlot=wh_slot + 1, podSlot=pod_slot + 1, lot=lot))
                    click(220, 150)
                    shot("04-cryo-warehouse-fetched")
                    key("Escape")
                    state = capture("04-cryo-warehouse-closed")
                    assert state.get("tradePage", 0) == 3
                key("e")
                click(220,150)
                key("b")
                quoted = capture("04-supply-stock")
            if args.market_visit_only:
                assert quoted["cityId"] == args.supply_city and quoted["screen"] == "trade"
                key("Escape")
                final = capture("05-market-main")
                assert final["screen"] == "trade" and final.get("tradePage",0) == 0
                assert final["quotes"] == quoted["quotes"] and final["holds"] == quoted["holds"]
                key("q")
                process.wait(timeout=20)
                receipt = dict(status="PASS",scope="正常保存回船、正式導航及Engage到普通城市，步行至原始交易所，讀取一輪八筆報價後返回Main保存；不購貨、不重進抽報價。",checks=checks,inputs=inputs,
                    quotes=quoted["quotes"],city=final["cityId"],funds=final["tradeSession"]["funds"],
                    toolSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),inputsSHA256={name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]})
                (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
                print("PASS:正常普通城市市場造訪／保留實際報價",flush=True)
                return

            automatic_stock = []
            if args.supply_market_needed:
                budget = quoted["tradeSession"]["funds"]
                empty_pods = sum(not hold.get("kind") for hold in quoted["holds"])
                for lot in sorted(quoted["quotes"], key=lambda q: (q["price"],q["goods"])):
                    goods = lot["goods"]
                    if not empty_pods:
                        break
                    if (2 <= goods <= 30 and goods not in wanted
                            and quoted["colony"]["remaining"][goods] > 0
                            and lot["price"]*100 <= budget):
                        wanted.append(goods)
                        automatic_stock.append(dict(lot))
                        budget -= lot["price"]*100
                        empty_pods -= 1
                print("實際需求報價選取",[lot["goods"] for lot in automatic_stock],flush=True)
            stock_visibility = [] if args.supply_return else [dict(goods=g,
                supply=quoted["markets"][args.supply_city]["supply"][g],
                offered=any(lot["goods"]==g for lot in quoted["quotes"])) for g in wanted]
            for goods in ([] if args.supply_return or args.supply_funding_only else wanted):
                if goods == 31:
                    continue  # Cold passengers were fetched from their actual warehouse.
                if not any(lot["goods"] == goods for lot in quoted["quotes"]):
                    continue  # 八筆報價未列出，不等於市場供給為零；分開記錄，不重設seed。
                for attempt in range(8):
                    lot = quoted["quotes"][quoted.get("quoteIndex",0)]
                    if lot["goods"] == goods:
                        break
                    key("p")
                    quoted = capture(f"04-quote-{goods}-{attempt}")
                assert lot["goods"] == goods
                assert lot["price"] * 100 <= quoted["tradeSession"]["funds"], "正常可用資金不足"
                key("b")
                paid = capture(f"05-paid-{goods}")
                assert paid["tradePage"] == 10 and paid["tradePurchaseLot"]["goods"] == goods
                key("2")
                state = capture(f"06-acquired-{goods}")
                acquired.append(next(dict(lot) for lot in state["holds"] if lot.get("goods") == goods))
                click(220,150)
                key("b")
                quoted = capture(f"06-next-stock-{goods}")
            if not args.supply_return:
                key("Escape")
            state = capture("07-supply-result")
            if acquired:
                if args.supply_refuel_return and not args.supply_return:
                    state = refuel_main(state,"08-return-refuel")
                if state["cityId"] != "banville":
                    state = return_to_ship(state, "08-return-with-cargo")
                    if args.supply_withdraw_return:
                        held = json.loads(json.dumps(state["holds"]))
                        warehouses = json.loads(json.dumps(state["warehouses"]))
                        for value in ["c", "e", "space", "u"]:
                            key(value)
                        teller = capture("08-return-funding-teller")
                        assert teller["screen"] == "uniteller" and teller["layout"] == "atari-st" and teller["bank"] > 0
                        balances = dict(teller["bankBalances"])
                        balances[teller["systemId"]] = 0
                        for value in ["w", "a", "Escape"]:
                            key(value)
                        funded = capture("08-return-funding-withdrawn")
                        assert funded["bank"] == 0 and funded["cash"] == teller["cash"] + teller["bank"]
                        assert funded["bankBalances"] == balances
                        assert funded["holds"] == held and funded["warehouses"] == warehouses
                        assert all(funded[k] == teller[k] for k in ["colony", "colonySite", "tradeRolls", "combatRolls"])
                        key("Escape")
                        state = capture("08-return-funding-ground")
                        state = return_to_ship(state, "08-return-funding-board")
                    for index, waypoint in enumerate(filter(None, (part.strip() for part in args.supply_return_via.split(",")))):
                        waypoint_city = next(row for row in city_records if city_id(row["name"]) == waypoint)
                        waypoint_run = next(row for row in runs if row["start"] <= waypoint_city["index"] < row["start"]+row["count"])
                        first_waypoint = city_id(city_records[waypoint_run["start"]]["name"])
                        state = fly_to(waypoint, first_waypoint, city_id(waypoint_run["system"]), f"08-return-via-{index}-{waypoint}", disembark=False)
                    state = fly_to("banville", "drahew", "jondd", "09-return-banville")
                if state.get("tradePage",0) != 9:
                    key("e")
                deliveries = []
                terminal_saved = False
                for lot in acquired:
                    slot = next(i for i, value in enumerate(state["holds"]) if value.get("goods") == lot["goods"])
                    if state.get("tradePage",0) != 9:
                        for value in ["s","2",str(slot+1)]:
                            key(value)
                    pending = capture(f"10-confirm-{lot['goods']}")
                    assert pending["tradePage"] == 9 and pending["tradeSalePrice"] > 0
                    key("1")
                    final_batch = pending["colony"]["phase"] == 8 and sum(pending["colony"]["remaining"]) == 1
                    if final_batch:
                        state = terminal_capture(f"11-delivered-{lot['goods']}")
                        terminal_saved = True
                    else:
                        state = capture(f"11-delivered-{lot['goods']}")
                    if final_batch:
                        assert state["holds"][slot] == pending["holds"][slot], "最終EXIT前原槽應保持，不套用普通售出清槽"
                    else:
                        assert not state["holds"][slot].get("goods",0)
                    if state["colony"]["phase"] == pending["colony"]["phase"]:
                        assert state["colony"]["remaining"][lot["goods"]] == pending["colony"]["remaining"][lot["goods"]]-1
                    else:
                        next_phase = pending["colony"]["phase"]+1
                        phases = json.loads((root / "internal/source/atarist/assets/colony-phases.json").read_text())["phases"]
                        assert state["colony"]["phase"] == next_phase and state["colony"]["nextPhase"] == next_phase+1
                        if next_phase == 9:
                            assert state["colony"]["remaining"] == [0]*32
                            assert state["missionComplete"] and state["screen"] == "score" and state["outcomeKey"] == "ui.outcome.success"
                            assert state["colony"]["accumulated"] == pending["colony"]["accumulated"]+pending["tradeSalePrice"]//2
                            assert not state["tradeSession"]["active"]
                            assert state["cash"]+max(0,state["bank"]) == pending["tradeSession"]["funds"], "原版最終期不發中間期累積收入"
                        else:
                            assert state["colony"]["remaining"] == phases[next_phase]["requiredCounts"]
                            assert state["colony"]["accumulated"] == 0
                            assert state["tradeSession"]["funds"] == pending["tradeSession"]["funds"]+100*(pending["colony"]["accumulated"]+pending["tradeSalePrice"]//2)
                    deliveries.append({"lot":lot,"salePrice":pending["tradeSalePrice"],"before":pending["colony"],"after":state["colony"],"fundsBefore":pending["tradeSession"]["funds"],"fundsAfter":state["tradeSession"]["funds"]})
                    if state["screen"] != "score":
                        click(220,150)
                        shot(f"11-delivered-{lot['goods']}")
            else:
                deliveries = []
                terminal_saved = False
            if not terminal_saved:
                key("q")
                process.wait(timeout=20)
            before = json.loads((out/"save.json").read_text())["state"]
            start(load=True)
            after = terminal_capture("12-reopened-final-score") if terminal_saved else capture("12-reopened")
            fields = ["schema","screen","cityId","systemId","planetId","position","groundView","groundMode","holds","warehouses","colony","colonySite","tradeSession","tradePage","tradeSaleSlot","tradeSalePrice","tradePurchaseLot","tradeRolls","combatRolls","cash","bank","bankBalances","cargoAttached","cargoCityId","cargoPosition","shipCityId","shipPosition","fuel","timeMinutes","shipSystems"]
            fields.remove("timeMinutes")
            fields.extend(["missionComplete","outcomeKey","banvilleCargo","deaths"])
            clock_reentry = check_ground_resume(before,after)
            assert all(before.get(k)==after.get(k) for k in fields), "供貨交付重開不同"
            if after["colony"]["phase"] == 9:
                assert after["screen"] == "score" and after["missionComplete"] and after["outcomeKey"] == "ui.outcome.success"
            receipt={"status":"PASS", "scope":"正常目前schema第一期供貨續行、星系／行星／城市導航、正式Engage、pod出行、實際庫存購買／回Banville交付及保存重開；庫存缺項保持，不注入或重設seed。原版規則切片另對拍，不稱原版GUI或完整九期", "supplyCity":args.supply_city,"wantedGoods":wanted,"acquired":acquired,"deliveries":deliveries,"checks":checks,"inputs":inputs,"reentryFields":fields,"before":{k:before.get(k) for k in fields},"after":{k:after.get(k) for k in fields},"toolSHA256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),"inputsSHA256":{name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest() for name in ["binary","disk","settings_input","library_input"]}}
            receipt["liveStart"] = live_start
            receipt["clockReentry"] = clock_reentry
            receipt["stockVisibility"] = stock_visibility
            receipt["revisitedExchange"] = args.supply_revisit
            receipt["carriedDelivery"] = args.supply_return
            receipt["returnVia"] = [part.strip() for part in args.supply_return_via.split(",") if part.strip()]
            receipt["warehouseFetches"] = warehouse_fetches
            receipt["automaticStockSelection"] = automatic_stock
            receipt["selectedByCurrentDemand"] = args.supply_market_needed
            receipt["phaseBefore"] = initial["colony"]["phase"]
            receipt["phaseAfter"] = after["colony"]["phase"]
            receipt["scope"] = "正常目前schema供貨續行、正式導航／Engage、pod出行、既有倉庫Fetch／實際報價購買／返Banville交付及保存重開；市場供給與八筆報價分開記錄，正常地面對話關閉後續行，不注入或重設seed。原版規則切片另對拍，不稱原版GUI或完整九期。"
            if args.supply_funding_only:
                receipt["funding"] = funding
                receipt["scope"] = "同局供貨主線正常前往來源城市、領出既有正銀行餘額、選用正常補油及查實際報價、保存重開；不買貨或交付，不注入或重設seed。銀行為正常remake玩家鏈，購油沿用既有READY；不新增原版銀行GUI或提款原生對拍聲明。"
            (out/"receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
            acquisition = "沿用已攜帶" if args.supply_return else "實際取得"
            print(f"PASS:供貨{args.supply_city}{acquisition}{len(acquired)}批／交付{len(deliveries)}批／{len(fields)}欄位重開",flush=True)
            return
        if args.marquee_only:
            fields = ["schema", "screen", "cityId", "shipCityId", "shipPosition", "position", "groundView",
                      "colony", "markets", "tradeSession", "tradeClassShift", "cash", "bank", "combatRolls",
                      "tradeRolls", "quotes", "colonySite", "cargoCityId", "cargoPosition", "fuel", "holds", "warehouses"]
            before = {k: state.get(k) for k in fields}
            shot("01-entrance")
            time.sleep(1.4)
            shot("02-entrance-later")
            click(190, 104)  # style11 左框：Exchange。使用正式滑鼠 consumer。
            assert capture("03-main").get("tradePage", 0) == 0
            # 保存庫關閉也可能帶入返回提示。正式空白區點擊只關提示，不成交。
            click(220, 150)
            shot("03-main")
            click(190, 104)  # 同一左框現在是 Buy；不按購買確認。
            assert capture("04-buy")["tradePage"] == 1
            shot("04-buy")
            key("Escape")
            assert capture("05-main-returned").get("tradePage", 0) == 0
            time.sleep(2)
            shot("05-main-returned")
            # 正式 delay_button 點擊關閉提示，不以主機秒數猜有效更新率。
            click(220, 150)
            shot("05-main-returned-stable")
            key("Escape")
            assert capture("06-entrance-returned")["tradePage"] == 3
            shot("06-entrance-returned")
            key("q")
            process.wait(timeout=20)
            start(load=True)
            after = capture("07-reopened")
            assert after["tradePage"] == 3
            assert all(after.get(k) == before[k] for k in fields), "不成交頁面往返或重開改動狀態"
            shot("07-reopened")
            receipt = {"status": "PASS", "scope": "正常交易保存續行、入口移動字窗、Exchange／Buy子頁、Escape返回、單檔保存與新程序重開；不成交、不注入。只驗remake GUI，原版圖形／計時仍unverified。",
                       "locale": args.locale, "checks": checks, "inputs": inputs, "reentryFields": fields,
                       "before": before, "after": {k: after.get(k) for k in fields},
                       "inputsSHA256": {name: hashlib.sha256(getattr(args,name).read_bytes()).hexdigest()
                                        for name in ["binary", "disk", "settings_input", "library_input"]}}
            (out / "receipt.json").write_text(json.dumps(receipt,ensure_ascii=False,indent=2)+"\n")
            print(f"PASS:走馬燈正常{args.locale}頁面往返／{len(fields)}欄位重開",flush=True)
            return
        selected = state["colonySite"]
        acquired = []
        if args.phase0_delivery:
            # 先售出先前正常購得、此期不需要的商品12，空出原槽。
            for value in ["s", "2", "1"]:
                key(value)
            offered = capture("01-unneeded-sale-confirm")
            assert offered["tradePage"] == 9 and offered["tradeSaleSlot"] == 5 and offered["tradeSalePrice"] > 0
            key("1")
            state = capture("01-unneeded-sold")
            assert not state["holds"][0].get("goods", 0)
            key("b")
            for attempt in range(8):
                quoted = capture(f"01-required-quote-{attempt}")
                lot = quoted["quotes"][quoted.get("quoteIndex", 0)]
                if lot["goods"] in [8, 9, 11, 13] and lot["price"] * 100 <= quoted["tradeSession"]["funds"]:
                    break
                key("p")
            else:
                raise AssertionError("本輪正常市場沒有可購第一期貨物")
            key("b")
            paid = capture("01-required-paid")
            assert paid["tradePage"] == 10 and paid["tradePurchaseLot"]["goods"] == lot["goods"]
            key("2")
            state = capture("01-required-pod")
            acquired.append(dict(state["holds"][0]))
            assert all(state["holds"][0][k] == lot[k] for k in ["goods", "quality", "price"])
            for value in ["Escape", "Escape", "F8", "F8", "Right", "Left"]:
                key(value)
            state = capture("01-returned-with-pod")
            assert state["screen"] == "shipboard" and state["cargoAttached"]
            key("F8")
            key("g")
            key("space")
            key("Right")
            key("Up")
            for attempt in range(12):
                key("Up")
                key("Up")
                state = capture(f"01-site-walk-{attempt}")
                assert state["screen"] == "ground"
                if state.get("groundWorld", False):
                    break
            else:
                raise AssertionError("正常出城未完成首次選址")
            selected = state["colonySite"]
            assert not selected["pending"]
            key("F8")
            key("Right")
            key("Left")
            state = capture("01-selected-returned-city")
            assert not state.get("groundWorld", False) and state["cityId"] == "drahew"
            # 依來源城市資料尋正常格子步行路線，避免穿過船門。
            city = json.loads((root / "internal/source/atarist/assets/city-maps.json").read_text())["cities"][0]
            buildings = {(b["row"], b["column"]) for b in city["buildings"]}
            start_cell = (state["position"]["x"], state["position"]["y"])
            goal = (state["shipPosition"]["x"], state["shipPosition"]["y"])
            previous = {start_cell: None}
            queue = deque([start_cell])
            while queue and goal not in previous:
                current = queue.popleft()
                for value, dx, dy in [("Up",0,-1),("Right",1,0),("Down",0,1),("Left",-1,0)]:
                    nx, ny = (current[0]+dx+10)%40-10, (current[1]+dy+6)%24-6
                    position = (nx, ny)
                    if position in previous:
                        continue
                    if position != goal and 0 <= nx < 20 and 0 <= ny < 12 and position not in buildings and city["grid"][nx][ny] in [6,7,12,13,14]:
                        continue
                    previous[position] = (current,value)
                    queue.append(position)
            assert goal in previous
            route = []
            while goal != start_cell:
                goal, value = previous[goal]
                route.append(value)
            for value in reversed(route):
                key(value)
            state = capture("01-selected-boarded")
            assert state["screen"] == "shipboard" and state["colonySite"] == selected
            key("F8")
            key("g")
            key("space")
        elif args.site_arrival:
            key("F8")
            state = capture("01-obburg-remake")
            assert state["layout"] == "remake"
            city = json.loads((root / "internal/source/atarist/assets/city-maps.json").read_text())["cities"][1]
            buildings = {(b["row"], b["column"]) for b in city["buildings"]}
            start_cell = (state["position"]["x"], state["position"]["y"])
            tube = next(b for b in city["buildings"] if b["slot"] == 0)
            goal = (tube["row"], tube["column"])
            previous = {start_cell: None}
            queue = deque([start_cell])
            while queue and goal not in previous:
                current = queue.popleft()
                for value, dx, dy in [("Up",0,-1),("Right",1,0),("Down",0,1),("Left",-1,0)]:
                    nx,ny = (current[0]+dx+10)%40-10,(current[1]+dy+6)%24-6
                    position = (nx,ny)
                    if position in previous:
                        continue
                    if 0 <= nx < 20 and 0 <= ny < 12 and position not in buildings and city["grid"][nx][ny] in [6,7,12,13,14]:
                        continue
                    previous[position] = (current,value)
                    queue.append(position)
            assert goal in previous, "Obburg來源城市格找不到Tube路線"
            route = []
            while goal != start_cell:
                goal,value = previous[goal]
                route.append(value)
            for value in reversed(route):
                key(value)
            state = capture("01-obburg-tube-building")
            assert state["position"] == {"x":tube["row"],"y":tube["column"]}
            key("b")
            state = capture("01-obburg-tube")
            assert state["screen"] == "tube-station"
            key("1")
            state = capture("01-drahew-confirm")
            assert state["tubeTargetId"] == "drahew"
            key("Return")
            state = capture("01-drahew-returned")
            assert state["screen"] == "ground" and state["cityId"] == "drahew"
            assert state["colonySite"] == selected
            key("F8")
            state = capture("01-drahew-st")
            assert state["theme"] == "atari-st-original" and state["layout"] == "atari-st"
        else:
            key("g")
            key("space")  # 正常略過降落動畫，之後才開保存庫。
        state = capture("02-ground")
        assert state["screen"] == "ground"
        key("y")
        state = capture("03-parts")
        assert state["screen"] == "parts"
        source = json.loads((root / "docs/atari-st/ship-systems.json").read_text())
        sale = sorted({23}
                      | {int(c["itemIndex"], 16) for s in source["systems"] for c in s["columns"]}
                      | {int(s["alternate"]["itemIndex"], 16) for s in source["systems"] if s.get("alternate")})
        for _ in range((sale.index(22) - state.get("selectedPartIndex", 0)) % len(sale)):
            key("Right")
        key("b")
        assert 22 in capture("04-scanner-bought")["parts"]
        key("Escape")
        key("Right")
        key("Right")
        state = capture("05-depot")
        if state["screen"] == "ground":
            key("r")
        assert capture("05-fuel-menu")["screen"] == "fuel-purchase"
        key("1")
        key("3")
        assert capture("05-refueled")["fuel"] > 0
        key("Left")
        key("Left")
        state = capture("06-return-position" if args.site_arrival else "06-boarded")
        if args.site_arrival and state["screen"] == "ground":
            # Tube返回的格內站位不同。依已核定步行登船門(17,20)、容差4，
            # 先走到船東側的門高度，再靠門；不用格子位置假定已登船。
            origin = state["groundView"]["origin"]
            ship_x = (state["shipPosition"]["x"]-origin["x"])*40
            ship_y = (state["shipPosition"]["y"]-origin["y"])*40
            state = walk(ship_x+49, ship_y+20, "06-ship-east-door-height", ("ground", "shipboard"))
            if state["screen"] == "ground":
                state = walk(ship_x+20, ship_y+20, "06-boarded", "shipboard")
        assert state["screen"] == "shipboard"
        key("e")
        state = capture("07-engineering")
        assert state["screen"] == "engineering"
        for _ in range((5 - state.get("selectedSystemIndex", 0)) % 6):
            key("Right")
        click(100, 122)
        assert capture("08-scanner-in-hand")["handItem"] == 22
        click(168, 89)
        assert capture("09-scanner-installed")["shipSystems"][5]["rows"][0][2] == 22
        click(97, 94)
        key("Escape")
        key("n")
        assert capture("10-navigation")["navigationTargetId"] == "banville"
        key("Return")
        if args.site_arrival or args.phase0_delivery:
            # 同行星航程已完成；起飛動畫的Update優先於F5。以正式Space
            # 略過動畫後才開保存庫，不用主機秒數假定動畫已結束。
            key("space")
        else:
            time.sleep(4)
        state = capture("11-arrived")
        assert state["shipCityId"] == "banville" and state["screen"] == "shipboard"
        if args.site_arrival or args.phase0_delivery:
            assert state["colonySite"] == selected and not selected["pending"]
            assert state["planetId"] == "jondd"
            state = capture("11-arrived-st")
            assert state["theme"] == "atari-st-original" and state["layout"] == "atari-st"
        if args.phase0_delivery:
            key("c")
            key("e")
        else:
            key("g")
        key("space")
        state = capture("12-banville-ground")
        assert state["position"] == {"x": 9, "y": 6} and state["screen"] == "ground"
        shot("12-banville-phase0")
        walk(196, 101, "13-east-of-ship")
        walk(196, 137, "13-south-of-ship")
        walk(156, 137, "13-west-of-building")
        walk(156, 56, "13-north-of-building")
        state = walk(175, 56, "14-exchange-cell")
        assert state["position"] == {"x": 9, "y": 5}
        key("t")  # 保留的鍵盤交易入口；不宣稱原版滑鼠門位已驗收。
        state = capture("14-exchange")
        assert state["screen"] == "trade" and state["colony"]["initialized"] and state["colony"]["phase"] == 0
        shot("14-banville-exchange")
        deliveries = []
        if args.phase0_delivery:
            assert state["groundMode"] == "pod" and state["colonySite"] == selected
            assert state["holds"][0] == acquired[0]
            key("e")
            key("s")
            key("2")
            key("1")
            pending = capture("16-colony-confirm")
            assert pending["tradePage"] == 9 and pending["tradeSaleSlot"] == 5
            assert pending["colony"]["remaining"][acquired[0]["goods"]] > 0
            price = pending["tradeSalePrice"]
            key("2")
            cancelled = capture("17-colony-withdrawn")
            assert cancelled["holds"] == pending["holds"] and cancelled["colony"] == pending["colony"]
            assert cancelled["tradeSession"] == pending["tradeSession"]
            for value in ["s", "2", "1"]:
                key(value)
            confirmed = capture("18-colony-confirm-again")
            assert confirmed["tradeSalePrice"] == price
            shot("18-colony-confirm")
            key("1")
            state = capture("19-colony-delivered")
            goods = acquired[0]["goods"]
            assert state["colony"]["remaining"][goods] == pending["colony"]["remaining"][goods] - 1
            assert state["colony"]["accumulated"] == pending["colony"]["accumulated"] + price // 2
            assert state["colony"]["phase"] == 0 and not state["holds"][0].get("goods", 0)
            assert state["tradeSession"]["funds"] == pending["tradeSession"]["funds"], "未換期卻收到付款"
            deliveries.append({"lot": acquired[0], "salePrice": price, "before": pending["colony"], "after": state["colony"], "fundsBefore": pending["tradeSession"]["funds"], "fundsAfter": state["tradeSession"]["funds"]})
            click(220,150)
            shot("19-colony-delivered")
        key("q")
        process.wait(timeout=20)
        before = json.loads((out / "save.json").read_text())["state"]
        start(load=True)
        after = capture("15-loaded")
        fields = ["schema", "screen", "cityId", "shipCityId", "shipPosition", "position", "groundView",
                  "colony", "markets", "tradeSession", "tradePage", "tradeClassShift", "cash", "bank",
                  "combatRolls", "tradeRolls", "shipSystems"]
        if args.site_arrival:
            fields += ["colonySite", "cargoCityId", "cargoPosition", "fuel", "navigationTargetId", "navigationView"]
        for field in fields:
            assert before.get(field) == after.get(field), field
        receipt = {"status": "PASS", "scope": "正常保存庫續行、掃描器購買／安裝、購油、靜態Banville到達、步行至交易格並按T、保存重開；不含隨機位置、原版GUI或九期",
                   "checks": checks, "inputs": inputs, "reentryFields": fields,
                   "before": {k: before.get(k) for k in fields}, "after": {k: after.get(k) for k in fields},
                   "inputsSHA256": {name: hashlib.sha256(getattr(args, name).read_bytes()).hexdigest()
                                    for name in ["binary", "disk", "settings_input", "library_input"]}}
        if args.site_arrival:
            receipt["scope"] = "承接正常隨機城址／Obburg保存，Tube回Drahew、購買安裝掃描器、購油、導航抵達保存城址、ST步行交易與23欄位重開；不含原版完整GUI、後期Tube或九期"
            receipt["siteArrival"] = True
            receipt["selectedSite"] = selected
        if args.phase0_delivery:
            fields += ["colonySite", "cargoAttached", "cargoCityId", "cargoPosition", "fuel", "holds", "warehouses", "tradeSaleSlot", "tradeSalePrice", "tradePurchaseLot"]
            assert all(before.get(k) == after.get(k) for k in fields), "第一期交付重開不同"
            receipt.update({"scope": "承接目前schema正常pod交易保存、所需貨物取得、首次出城選址、設備／購油／導航、Banville部分第一期確認／撤回／交付及新程序重開；不注入、不挑seed；不含完整原版GUI、完整第一期或九期", "saveSchema":schema, "deliveries":deliveries, "selectedSite":selected, "toolSHA256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "reentryFields":fields, "before":{k:before.get(k) for k in fields}, "after":{k:after.get(k) for k in fields}})
        (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n")
        print(f"PASS: Banville初期玩家鏈與{len(fields)}欄位重開", flush=True)
    except Exception as error:
        failure = {"status":"tool-failure", "error":str(error), "checks":checks, "inputs":inputs,
                   "toolSHA256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                   "inputsSHA256":{name:hashlib.sha256(getattr(args,name).read_bytes()).hexdigest()
                                   for name in ["binary","disk","settings_input"]+(["library_input"] if args.library_input else [])}}
        (out/"failure.json").write_text(json.dumps(failure,ensure_ascii=False,indent=2)+"\n")
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
