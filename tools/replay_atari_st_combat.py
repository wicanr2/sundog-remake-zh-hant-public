"""正式 UI 由正常保存裝補給、交火、保存重入；以唯讀保存輔助瞄準。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "disk", "normal-save", "output"):
        parser.add_argument("--"+name, type=Path, required=True)
    parser.add_argument("--loaded-shipboard", action="store_true",
                        help="從未修改的正常有貨登船保存直接按X，不重做歷史裝貨前綴")
    parser.add_argument("--exercise-course-boundary", action="store_true",
                        help="用正式護盾／回充按鍵走至模式4/5近距離，核對新增停留／遠離結果；不注入狀態")
    parser.add_argument("--verify-initial-cooldown", action="store_true",
                        help="核對正常新遭遇冷卻為原版初值20，不改寫保存或counter")
    parser.add_argument("--verify-shield-levels", action="store_true",
                        help="正常命中後回充一次，核對護盾子規則扣油與燃料保存重入")
    parser.add_argument("--verify-weapon-levels", action="store_true",
                        help="正常回充核對兩次既有武器更新的充能／耗油，另列護盾耗油")
    parser.add_argument("--exercise-fuel-exhaustion", action="store_true",
                        help="以有界正常R操作耗盡燃料，核對護盾關閉及保存重入，不改寫初態")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    out = args.output.resolve()
    if out.exists() or out.parent.stat().st_uid != os.getuid():
        parser.error("須為目前使用者擁有的全新輸出")
    out.mkdir(mode=0o700)
    save, library, settings = out/"save.json", out/"library.json", out/"settings.json"
    shutil.copyfile(args.normal_save, save)
    settings.write_text(json.dumps({"schema": 1, "theme": "atari-st-original",
                                   "tileset": "atari-st-tiles", "layout": "atari-st"}))
    env = dict(os.environ, DISPLAY=":97")
    actions, observations = [], []
    course_boundary = None
    shield_levels = []
    weapon_levels = []
    fuel_exhaustion = None
    app = None
    def run(*argv):
        return subprocess.check_output(list(map(str,argv)),env=env,text=True,timeout=15).strip()
    def start(log):
        nonlocal app
        actions.append({"event":"start_loaded_program","save_sha256":sha(save)})
        app = subprocess.Popen([str(args.binary),"-scale","3","-load","-atari-disk",str(args.disk),
                                "-settings",str(settings),"-save",str(save),"-library",str(library)],
                               cwd=Path(__file__).resolve().parents[1],env=env,stdout=log,stderr=subprocess.STDOUT)
        for _ in range(120):
            r = subprocess.run(["xdotool","search","--onlyvisible","--class","ebitengine-application"],
                               env=env,capture_output=True,text=True)
            if r.returncode == 0 and r.stdout.strip():
                window=r.stdout.splitlines()[0]
                run("xdotool","windowfocus","--sync",window)
                time.sleep(.7)
                return window
            if app.poll() is not None:
                raise RuntimeError("正式程序提前退出")
            time.sleep(.1)
        raise RuntimeError("找不到正式遊戲視窗")
    def press(key,pause=.15):
        actions.append({"key":key,"pause":pause})
        run("xdotool","keydown","--clearmodifiers","--window",window,key)
        time.sleep(.08)
        run("xdotool","keyup","--clearmodifiers","--window",window,key)
        time.sleep(pause)
    def observe():
        press("F5",.18)
        if not library.exists():
            press("m",.2) # 正式「建立保存庫」，不由工具合成 JSON。
            run("import","-window",window,out/"library-created.png")
        prior=library.stat().st_mtime_ns if library.exists() else None
        press("s",.2)
        if library.stat().st_mtime_ns==prior:
            raise RuntimeError("正式保存庫未寫入")
        state=json.loads(library.read_text())["slots"]["slot-1"]
        if state.get("screen") != "tactical" and state.get("libraryReturn") != "tactical":
            raise RuntimeError("保存庫入口不是戰術畫面，停止送後續射擊")
        observations.append(state)
        press("Escape")
        # ActionBack 本身已清除 TacticalSlewing；再送一次 Escape 會離開戰術。
        return state
    def quit_app():
        actions.append({"key":"q","event":"normal_exit"})
        run("xdotool","keydown","--clearmodifiers","--window",window,"q")
        app.wait(timeout=10)
        if app.returncode:
            raise RuntimeError("正式程序非正常退出")
    with (out/"xvfb.log").open("w") as xlog, (out/"app.log").open("w") as log:
        xvfb=subprocess.Popen(["Xvfb",":97","-screen","0","1280x800x24","-nolisten","tcp"],
                              stdout=xlog,stderr=subprocess.STDOUT,env=env)
        try:
            for _ in range(50):
                if subprocess.run(["xdpyinfo"],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:
                    break
                time.sleep(.1)
            else: raise RuntimeError("Xvfb 未就緒")
            window=start(log)
            keys = ("x",) if args.loaded_shipboard else ("g","Escape","p","Right","Left","x")
            for key in keys:
                press(key,.5)
            run("import","-window",window,out/"tactical-start.png")
            initial=observe()
            if not initial["combatActive"]:
                raise RuntimeError("正常裝補給路徑未產生有效遭遇")
            if args.verify_initial_cooldown and initial["enemyFireCountdown"] != 20:
                raise RuntimeError("正常遭遇初始冷卻不等於已核定原版20")
            if args.exercise_course_boundary:
                mode = initial["enemyCourseMode"]
                if mode not in (4,5):
                    raise RuntimeError("既有正常路徑未產生模式4/5，不改寫模式或挑選重擲")
                upper = 80 if mode == 4 else 70
                press("u",.2) # 正式全護盾操作，同樣會推進一拍。
                state = observe()
                for _ in range(100):
                    if 40 <= state["combatRange"] <= upper:
                        break
                    if not state["combatActive"] or state["hullCondition"] <= 0:
                        raise RuntimeError("正常近距離路徑已結束交戰")
                    press("r",.1)
                    state = observe()
                else:
                    raise RuntimeError("有界操作未進入已核定距離，不宣稱覆蓋")
                samples=[]
                for _ in range(8):
                    before_step=state
                    if before_step["enemyCourseMode"]!=mode or not 40<=before_step["combatRange"]<=upper:
                        raise RuntimeError("近距離抽樣離開已核定狀態")
                    press("r",.1);state=observe()
                    delta=state["combatRange"]-before_step["combatRange"]
                    samples.append({"range_before":before_step["combatRange"],"range_after":state["combatRange"],
                                    "delta":delta,"counter_before":before_step["combatRolls"],"counter_after":state["combatRolls"]})
                    if delta not in (-2,-1,0,1):
                        raise RuntimeError("正式近距離結果超出原版抽樣範圍")
                    if delta>=0:
                        break
                if not any(x["delta"]>=0 for x in samples):
                    raise RuntimeError("有界正常抽樣未見新增停留／遠離，保留全部結果而不挑種子")
                course_boundary={"mode":mode,"samples":samples,"input":"正式U全護盾、R回充與保存庫唯讀觀察；初始保存未改寫"}
                run("import","-window",window,out/"course-boundary.png")
            table_raw=args.disk.read_bytes()[0xd422:0xd422+64]
            table=[int.from_bytes(table_raw[i:i+2],"big",signed=True) for i in range(0,64,2)]
            landed=[]
            for _ in range(45):
                state=observations[-1]
                if not state["combatActive"] or state["hullCondition"]<=0:
                    break
                if state["weaponCharge"]<100:
                    press("r");observe();continue
                distance=max(10,state["combatRange"])
                base=(15+state["attributes"][2])//10
                laser=max(0,base-distance//32)
                cannon=max(0,4*base-distance//12)
                error=abs(state["enemyBearing"]-512)+abs(state["enemyPitch"]-512)
                if error<=cannon:
                    weapon="g" if error<=laser else "f"
                    run("import","-window",window,out/f"before-shot-{len(landed)}.png")
                    press(weapon)
                    run("import","-window",window,out/f"after-shot-{len(landed)}.png")
                    after=observe()
                    if after["enemyHull"]<state["enemyHull"]:
                        landed.append({"weapon":weapon,"before":state["enemyHull"],"after":after["enemyHull"],"charge_after":after["weaponCharge"]})
                        break
                    continue
                duration=.25 if error>60 else .03
                n=max(1,round((duration+.08)*15))
                weight=3-distance//32
                db,dp=state.get("enemyDriftBearing",0),state.get("enemyDriftPitch",0)
                target_b=(state["enemyBearing"]-db*4-512)/n
                target_p=(512-state["enemyPitch"]-dp*4)/n
                xi=min(range(32),key=lambda i:abs(2*table[31-i]+weight*db-target_b))
                yi=min(range(32),key=lambda i:abs(2*table[i]+weight*dp-target_p))
                x,y=xi*10+5,max(0,yi*6-4+2)
                # --sync 在游標已處於相同位置時可能一直等不到新事件；命令完成即可送後續輸入。
                run("xdotool","mousemove","--window",window,x*3,y*3)
                actions.append({"mouse_x":x,"mouse_y":y,"slew_duration":duration})
                press("m",duration)
                observe()
            if not landed:
                (out/"actions.json").write_text(json.dumps(actions,ensure_ascii=False,indent=2)+"\n")
                (out/"observations.json").write_text(json.dumps(observations,ensure_ascii=False,indent=2)+"\n")
                raise RuntimeError("有界正常交火未取得命中；不宣稱通過")
            if args.verify_shield_levels or args.verify_weapon_levels:
                prior=observations[-1]
                if not prior["combatActive"] or prior["fuel"]<5:
                    raise RuntimeError("正常護盾樣本已離開READY的交戰／燃料範圍")
                status=next(s["status"] for s in prior["shipSystems"] if s["id"]=="force-shields")
                target=prior["shieldSetting"] if prior["shieldsActive"] else 0
                level=prior["shieldLevel"]
                updated=level if level==target else min(max(0,level+status*8),target)
                cost=1 if level==target and level>0 else ((target+1)//64 if updated<target else 0)
                gun_status=next(s["status"] for s in prior["shipSystems"] if s["id"]=="ship-guns")
                def weapon_update(charge, fuel):
                    result=min(255,charge+gun_status*4) if fuel>0 and gun_status>0 else max(0,charge-8)
                    remaining=fuel-(1 if result>charge else 0)
                    return {"Charge":charge,"Status":gun_status,"Fuel":fuel,
                            "ResultCharge":result,"ResultFuel":remaining}
                first=weapon_update(prior["weaponCharge"],prior["fuel"])
                last=weapon_update(first["ResultCharge"],first["ResultFuel"]-cost)
                press("r"); after=observe()
                if after["fuel"]!=last["ResultFuel"] or after["weaponCharge"]!=last["ResultCharge"]:
                    raise RuntimeError("正常回充的充能／總耗油與已核定子規則不同")
                weapon_levels.extend([first,last])
                shield_levels.append({"Level":level,"Setting":prior["shieldSetting"],"Status":status,
                                      "Fuel":first["ResultFuel"],"Active":prior["shieldsActive"],
                                      "ResultFuel":last["Fuel"],"Cost":cost,
                                      "TotalFuelBefore":prior["fuel"],"TotalFuelAfter":after["fuel"],
                                      "scope":"正常R總耗油／充能已核對；分開列兩次武器與一次護盾投影，非原版完整三相排程"})
            if args.exercise_fuel_exhaustion:
                samples = []
                for _ in range(150):
                    prior = observations[-1]
                    if prior["fuel"] <= 0 or not prior["combatActive"]:
                        raise RuntimeError("正常耗油入口已無燃料或交戰結束，不重擲")
                    if any(h.get("kind") == "fuel" for h in prior.get("holds", [])):
                        raise RuntimeError("本窄規格不涵蓋備用油槽")
                    press("r"); after = observe()
                    samples.append({"fuel_before":prior["fuel"],"fuel_after":after["fuel"],
                                    "shields_active":after["shieldsActive"],
                                    "shield_setting":after["shieldSetting"],"shield_level":after["shieldLevel"]})
                    (out/"fuel-exhaustion-samples.json").write_text(json.dumps(samples,ensure_ascii=False,indent=2)+"\n")
                    if after["fuel"] == 0:
                        if after["shieldsActive"] or after["shieldSetting"] != 0:
                            raise RuntimeError("燃料耗盡後護盾未關閉或設定未清零")
                        fuel_exhaustion = {"steps":len(samples),"last":samples[-1],
                                           "scope":"正式R連續耗油至零；唯讀保存觀察，非原版完整排程同狀態"}
                        run("import","-window",window,out/"fuel-exhausted.png")
                        break
                else:
                    raise RuntimeError("150次正常回充仍未耗盡燃料，不宣稱覆蓋")
            press("Escape");press("s")
            before=json.loads(save.read_text())["state"]
            (out/"reentry-before-save.json").write_bytes(save.read_bytes())
            quit_app()
            # Q autosaves a later live clock instant than the earlier S key.
            # Compare with the file the new process actually loads.
            (out/"reentry-loaded-save.json").write_bytes(save.read_bytes())
            before=json.loads(save.read_text())["state"]
            with (out/"reloaded.log").open("w") as rlog:
                window=start(rlog)
                press("x")
                run("import","-window",window,out/"reloaded-tactical.png")
                reloaded=observe()
                (out/"reentry-observed.json").write_text(json.dumps(reloaded,ensure_ascii=False,indent=2)+"\n")
                fields=["fuel","enemyType","enemyHull","enemyMaxHull","combatRange","enemyBearing","enemyPitch","weaponCharge","combatRolls",
                        "enemyFireCountdown","enemyDriftBearing","enemyDriftPitch",
                        "hullCondition","shieldLevel","shieldSetting","shieldsActive","shipSystems","selectedSystemIndex"]
                # Normal loaded gameplay keeps advancing the saved VBL clock.
                # Validate the same elapsed ticks, not frozen energy values.
                overlay = out/"clock-overlay.json"
                root = Path(__file__).resolve().parents[1]
                overlay.write_text(json.dumps({"Replace": {
                    str(root/"internal/game/gui_combat_reentry_clock_test.go"):
                    str(root/"tools/gui_combat_reentry_clock_test.go.txt")}}))
                replay_env = dict(env, SUNDOG_GUI_REENTRY=str(out), GOPROXY="off", GOTOOLCHAIN="local")
                with (out/"reentry-clock-validation.log").open("w") as validation_log:
                    subprocess.run(["go","test","-overlay",str(overlay),"./internal/game",
                                    "-run","^TestValidateGUICombatReentryClock$","-count=1","-v"],
                                   cwd=root,env=replay_env,stdout=validation_log,stderr=subprocess.STDOUT,check=True,timeout=90)
                quit_app()
            (out/"actions.json").write_text(json.dumps(actions,ensure_ascii=False,indent=2)+"\n")
            (out/"observations.json").write_text(json.dumps(observations,ensure_ascii=False,indent=2)+"\n")
        finally:
            if app is not None and app.poll() is None:
                app.terminate()
                try: app.wait(timeout=5)
                except subprocess.TimeoutExpired: app.kill();app.wait(timeout=5)
            xvfb.terminate()
            try: xvfb.wait(timeout=5)
            except subprocess.TimeoutExpired: xvfb.kill();xvfb.wait(timeout=5)
    report={"status":"PASS","scope":"remake 正常裝補給／遭遇／唯讀保存輔助瞄準／命中／全新程序保存重入；非純視覺、非原版同狀態",
            "shots":landed,"course_boundary":course_boundary,"shield_levels":shield_levels,"weapon_levels":weapon_levels,"fuel_exhaustion":fuel_exhaustion,"initial_cooldown":initial["enemyFireCountdown"],
            "initial_drift":{"mode":initial["enemyCourseMode"],"bearing":initial["enemyDriftBearing"],
                             "pitch":initial["enemyDriftPitch"],"counter":initial["combatRolls"]},
            "reentry":{k:{"before":before[k],"after":reloaded[k]} for k in fields},
            "reentry_clock_scope":"以Q實際退出保存為起點，重播真實VBL差並核對26欄位；不要求正常續行中的能量讀數凍結。詳見reentry-clock-validation.log。",
            "binary_sha256":sha(args.binary),"disk_sha256":sha(args.disk),
            "initial_save_sha256":sha(args.normal_save),"artifacts":{p.name:sha(p) for p in out.iterdir() if p.is_file()}}
    (out/"receipt.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n")
    print("正常戰鬥命中及全新程序保存重入通過")


if __name__=="__main__":
    main()
