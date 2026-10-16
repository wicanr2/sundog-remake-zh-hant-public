"""正常載回、下船、滑鼠走到指定目標與放開保存；見 walker-timing-spec.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for arg in ("binary", "disk", "normal-save", "output"):
        parser.add_argument("--"+arg, type=Path, required=True)
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    output = args.output.resolve()
    if output.exists() or output.parent.stat().st_uid != os.getuid():
        parser.error("輸出須為目前使用者擁有的全新路徑")
    output.mkdir(mode=0o700)
    save, settings = output / "save.json", output / "settings.json"
    shutil.copyfile(args.normal_save, save)
    settings.write_text(json.dumps({"schema": 1, "theme": "atari-st-original",
                                   "tileset": "atari-st-tiles", "layout": "atari-st"}), encoding="utf-8")
    env = dict(os.environ, DISPLAY=":94")
    app = None
    def command(*argv):
        return subprocess.check_output(argv, env=env, text=True, timeout=15).strip()
    with (output / "xvfb.log").open("w") as xlog, (output / "app.log").open("w") as log:
        xvfb = subprocess.Popen(["Xvfb", ":94", "-screen", "0", "1280x800x24", "-nolisten", "tcp"],
                                env=env, stdout=xlog, stderr=subprocess.STDOUT)
        try:
            for _ in range(50):
                if xvfb.poll() is not None:
                    raise RuntimeError("Xvfb 提前退出，見 xvfb.log")
                if subprocess.run(["xdpyinfo"], env=env, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL).returncode == 0:
                    break
                time.sleep(0.1)
            else:
                raise RuntimeError("Xvfb 未就緒")
            app = subprocess.Popen([str(args.binary), "-scale", "3", "-load", "-atari-disk",
                                    str(args.disk), "-settings", str(settings), "-save", str(save),
                                    "-library", str(output / "library.json")],
                                   cwd=Path(__file__).resolve().parents[1],
                                   env=env, stdout=log, stderr=subprocess.STDOUT)
            window = ""
            for _ in range(120):
                found = subprocess.run(["xdotool", "search", "--onlyvisible", "--class",
                                        "ebitengine-application"], env=env, capture_output=True, text=True)
                if found.returncode == 0 and found.stdout.strip():
                    window = found.stdout.splitlines()[0]
                    break
                if app.poll() is not None:
                    raise RuntimeError("遊戲提前退出，見 app.log")
                time.sleep(0.1)
            if not window:
                raise RuntimeError("找不到正式遊戲視窗")
            command("xdotool", "windowfocus", "--sync", window)
            time.sleep(0.7)
            def press(key):
                command("xdotool", "keydown", "--clearmodifiers", "--window", window, key)
                time.sleep(0.15)
                command("xdotool", "keyup", "--clearmodifiers", "--window", window, key)
                time.sleep(0.4)
            press("g")
            press("Escape")
            command("xdotool", "mousemove", "--sync", "--window", window, "600", "315")
            command("xdotool", "mousedown", "--window", window, "1")
            time.sleep(0.7)
            command("xdotool", "mouseup", "--window", window, "1")
            time.sleep(0.3)
            command("import", "-window", window, str(output / "released.png"))
            time.sleep(0.5)
            command("import", "-window", window, str(output / "stopped.png"))
            command("xdotool", "keydown", "--clearmodifiers", "--window", window, "q")
            app.wait(timeout=10)
            if app.returncode:
                raise RuntimeError(f"正式程序退出碼 {app.returncode}")
        finally:
            if app is not None and app.poll() is None:
                app.terminate()
                try:
                    app.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    app.kill()
                    app.wait(timeout=5)
            xvfb.terminate()
            try:
                xvfb.wait(timeout=5)
            except subprocess.TimeoutExpired:
                xvfb.kill()
                xvfb.wait(timeout=5)
    state = json.loads(save.read_text())["state"]
    anchor = state["groundView"]["anchor"]
    stationary = sha(output / "released.png") == sha(output / "stopped.png")
    passed = state["screen"] == "ground" and anchor == {"x": 200, "y": 105} and stationary
    receipt = {"scope": "remake normal mouse input; not original wall-clock parity",
               "binary_sha256": sha(args.binary), "initial_save_sha256": sha(args.normal_save),
               "disk_sha256": sha(args.disk), "target": {"x": 200, "y": 105},
               "saved_anchor": anchor, "stationary_after_release": stationary,
               "status": "PASS" if passed else "FAIL",
               "artifacts": {p.name: sha(p) for p in sorted(output.iterdir()) if p.is_file()}}
    (output / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    if not passed:
        raise RuntimeError(f"正常滑鼠目標／放開不符：{receipt}")
    print("正常滑鼠走到 (200,105)、放開停止及退出保存通過")


if __name__ == "__main__":
    main()
