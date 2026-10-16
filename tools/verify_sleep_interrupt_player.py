"""Docker內從正常schema43保存續行，讓正式戰術操作自然耗盡Rest。

入口：banville-restoration-spec.md「整點自動睡眠中斷READY」。
只送正式視窗輸入，不改保存／條件／時間／seed，不使用直接進場。
戰術操作的分鐘成本是現行remake路徑，這份收據不宣稱原版GUI時序。
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ["binary", "disk", "settings-input", "library-input", "output"]:
        p.add_argument("--"+name, type=Path, required=True)
    p.add_argument("--resume-forced", action="store_true", help="承接本工具正常耗盡Rest後的同局保存，不重走耗損路徑")
    args = p.parse_args()
    if not Path("/.dockerenv").is_file():
        p.error("只在Docker執行")
    root = Path(__file__).resolve().parents[1]
    for name in ["binary", "disk", "settings_input", "library_input"]:
        setattr(args, name, getattr(args, name).resolve(strict=True))
    initial = json.loads(args.library_input.read_text())["slots"]["slot-1"]
    assert initial["schema"] == 43 and initial["screen"] == "character-status" and initial["vigor"] > 0
    if args.resume_forced:
        assert initial["sleep"]["active"] and initial["sleep"]["forced"] and initial["statusReturn"] == "tactical"
    else:
        assert not initial["sleep"]["active"] and initial["statusReturn"] == "shipboard" and initial["rest"] > 0
    assert json.loads(args.settings_input.read_text())["layout"] == "atari-st"
    assert sha(args.disk) == "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6"
    out = args.output.resolve()
    assert not out.exists() and out.parent.stat().st_uid == os.getuid()
    out.mkdir(mode=0o700)
    (out/"library.json").write_bytes(args.library_input.read_bytes())
    (out/"settings.json").write_bytes(args.settings_input.read_bytes())
    env = dict(os.environ, DISPLAY=":95", EBITENGINE_SCREENSHOT_KEY="F12")
    checks, inputs, reentries = [], [], []
    process = window = None
    display = subprocess.Popen(["Xvfb", ":95", "-screen", "0", "1280x800x24", "-nolisten", "tcp"],
        stdout=(out/"xvfb.log").open("w"), stderr=subprocess.STDOUT)

    def run(command):
        subprocess.run(command, env=env, check=True, timeout=30, stdout=subprocess.DEVNULL)

    def key(value):
        inputs.append(dict(key=value))
        run(["xdotool", "key", "--clearmodifiers", value])
        time.sleep(.25)

    def start(load=False):
        nonlocal process, window
        cmd = [str(args.binary), "-scale", "3", "-start", "title", "-atari-disk", str(args.disk),
            "-settings", str(out/"settings.json"), "-save", str(out/"save.json"),
            "-library", str(out/"library.json"), "-locale", "zh-Hant"]
        if load:
            cmd.append("-load")
        process = subprocess.Popen(cmd, cwd=root, env=env, stdout=(out/f"game-{len(checks)}.log").open("w"),
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
        library = out/"library.json"
        stamp = library.stat().st_mtime_ns
        for value in ["F5", "1", "s", "Escape"]:
            key(value)
        assert library.stat().st_mtime_ns != stamp
        raw = library.read_bytes()
        (out/f"library-{name}.json").write_bytes(raw)
        state = json.loads(raw)["slots"]["slot-1"]
        checks.append(dict(name=name, state=state, sha256=hashlib.sha256(raw).hexdigest()))
        print(name, state["screen"], "time", state["timeMinutes"], "rest", state["rest"], "vigor", state["vigor"], flush=True)
        return state

    def shot(name):
        time.sleep(2.3)
        before = set(root.glob("screenshot*.png"))
        key("F12")
        for _ in range(50):
            created = set(root.glob("screenshot*.png"))-before
            if created:
                break
            time.sleep(.1)
        assert len(created) == 1
        created.pop().rename(out/f"{name}.png")

    def check_resumed(before, after, path):
        assert after["sleep"]["active"] and after["sleep"]["forced"]
        assert after["statusReturn"] == "tactical" and after["sleep"]["shipActive"]
        cycles = lambda s: s["sleep"]["elapsedMinutes"]*60+s["sleep"]["minuteCycles"]
        assert 0 <= cycles(after)-cycles(before) < 300
        for name in ["stores", "parts", "holds", "warehouses", "handItem", "cash", "bank"]:
            assert before[name] == after[name], name
        reentries.append(dict(path=path, advancedCycles=cycles(after)-cycles(before),
            before=before["sleep"], after=after["sleep"], statusReturn=after["statusReturn"]))

    try:
        for _ in range(60):
            if subprocess.run(["xwininfo", "-root"], env=env, stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(.1)
        start()
        for value in (["space", "c", "1", "l"] if args.resume_forced else ["space", "c", "1", "l", "Escape", "x"]):
            key(value)
        state = capture("01-forced-normal-resumed" if args.resume_forced else "01-normal-tactical")
        if not args.resume_forced:
            assert state["screen"] == "tactical" and not state["combatActive"]
            run(["xdotool", "mousemove", "--window", window, "480", "300"])
            # Inputs are ordinary repeated M presses; no clock/state injection.
            deadline = time.monotonic()+660
            for batch in range(1, 41):
                inputs.append(dict(key="m", count=200, delayMilliseconds=50))
                run(["xdotool", "key", "--delay", "50"]+["m"]*200)
                state = capture(f"02-clock-{batch}")
                assert state["vigor"] > 0 and state["screen"] != "outcome"
                if state["sleep"]["active"]:
                    break
                assert state["screen"] == "tactical"
                if time.monotonic() >= deadline:
                    raise RuntimeError("有界操作未自然進入強制睡眠")
            else:
                raise RuntimeError("普通戰術操作未觸發Rest0")
        else:
            assert state["sleep"]["active"]
        assert state["sleep"]["forced"] and state["statusReturn"] == "tactical"
        assert not state.get("tacticalSlewing", False) and state["rest"] < 50
        shot("03-forced-sleep")
        key("F10")
        process.wait(timeout=15)
        before = json.loads((out/"save.json").read_text())["state"]
        start(load=True)
        state = capture("04-forced-single-reopened")
        check_resumed(before, state, "single-new-process")
        before = state
        for value in ["F5", "1", "l"]:
            key(value)
        state = capture("05-forced-library-reopened")
        check_resumed(before, state, "library-same-process")
        for value in ["F1", "Escape", "F2", "Escape"]:
            key(value)
        deadline = time.monotonic()+240
        for attempt in range(1, 41):
            key("space")
            time.sleep(5)
            state = capture(f"06-wake-{attempt}")
            assert state["vigor"] > 0 and state["screen"] == "character-status"
            if not state["sleep"]["active"]:
                assert state["rest"] > 50
                break
            if time.monotonic() >= deadline:
                raise RuntimeError("有界等候未完成強制睡眠")
        else:
            raise RuntimeError("強制睡眠未允許醒來")
        shot("07-awake")
        key("Escape")
        state = capture("08-return-tactical")
        assert state["screen"] == "tactical" and not state["sleep"]["active"]
        assert state["stores"] == initial["stores"] and state["parts"] == initial["parts"]
        receipt = dict(status="PASS", schema=43, checks=checks, inputs=inputs, reentries=reentries,
            scope="正常既有新局保存、戰術操作自然耗盡Rest、自動強制面板、兩保存路徑續行、說明／設定返回、嘗試醒來及返回戰術畫面。無敵艦，不稱交戰GUI。",
            originalGUI="unverified", clockCost="remake existing tactical action; original foreground timing unverified",
            wallClock="presentation-approximation", toolSHA256=sha(Path(__file__)),
            resumedForced=args.resume_forced,
            inputSHA256={name:sha(getattr(args,name)) for name in ["binary", "disk", "settings_input", "library_input"]})
        (out/"receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+"\n")
        print(f"PASS {len(checks)}檢查點，兩保存續行，正常強制睡眠GUI", flush=True)
    except Exception as error:
        (out/"failure.json").write_text(json.dumps(dict(status="FAIL", error=str(error), checks=checks, inputs=inputs,
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
