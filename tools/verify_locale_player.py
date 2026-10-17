#!/usr/bin/env python3
"""Docker 內沿正常保存入口驗證五語選項與重開，並錄製語系展示。"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("binary", "disk", "settings", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--save", type=Path)
    parser.add_argument("--save-schema", type=int, choices=(44,45), default=45)
    args = parser.parse_args()
    assert Path("/.dockerenv").is_file()
    out = args.output.resolve()
    assert not out.exists()
    for path in (args.binary, args.disk, args.settings):
        assert path.is_file(), path
    sha = lambda path: hashlib.sha256(Path(path).read_bytes()).hexdigest()
    source_hash = None
    if args.save is not None:
        source = json.loads(args.save.read_text())["state"]
        assert source["schema"] == args.save_schema and source["screen"] == "shipboard"
        source_hash = sha(args.save)
    out.mkdir(parents=True)
    assert out.stat().st_uid == os.getuid()
    if args.save is not None:
        shutil.copy2(args.save, out / "save.json")
    shutil.copy2(args.settings, out / "settings.json")
    env = dict(os.environ, DISPLAY=":95", HOME="/tmp", LC_ALL="C.UTF-8")
    events, images = [], []
    app = recorder = None
    display = subprocess.Popen(["Xvfb", ":95", "-screen", "0", "1600x1000x24", "-nolisten", "tcp"], stdout=(out / "xvfb.log").open("w"), stderr=subprocess.STDOUT)

    def run(*argv):
        return subprocess.check_output(list(map(str, argv)), env=env, text=True, timeout=15).strip()

    def press(key):
        events.append({"key": key, "holdSeconds": .35})
        run("xdotool", "keydown", "--clearmodifiers", key)
        time.sleep(.35)
        run("xdotool", "keyup", key)
        time.sleep(.6)

    def capture(name):
        path = out / (name + ".png")
        run("import", "-window", window, path)
        images.append({"path": path.name, "sha256": sha(path)})

    def preferences():
        return json.loads((out / "settings.json").read_text())

    def change(expected, key="Right"):
        events.append({"key": key, "expectedLocale": expected})
        run("xdotool", "keydown", "--clearmodifiers", key)
        try:
            end = time.monotonic() + 15
            while time.monotonic() < end:
                assert app.poll() is None
                if preferences().get("locale") == expected:
                    break
                time.sleep(.08)
            else:
                raise RuntimeError(f"locale did not persist: {expected}")
        finally:
            run("xdotool", "keyup", key)
        time.sleep(.5)

    def start(first):
        command = [str(args.binary), "-save", str(out / "save.json"), "-settings", str(out / "settings.json"), "-library", str(out / "library.json"), "-atari-disk", str(args.disk), "-scale", "4"]
        if not first or args.save is not None:
            command += ["-load"]
        if first:
            command += ["-locale", "zh-Hant"]
        process = subprocess.Popen(command, env=env, stdout=(out / ("game-first.log" if first else "game-reopened.log")).open("w"), stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            assert process.poll() is None
            result = subprocess.run(["xdotool", "search", "--onlyvisible", "--pid", str(process.pid)], env=env, text=True, capture_output=True)
            if result.returncode == 0:
                target = result.stdout.splitlines()[-1]
                run("xdotool", "windowfocus", "--sync", target)
                time.sleep(1)
                return process, target
            time.sleep(.1)
        raise RuntimeError("no normal game window")

    try:
        time.sleep(.6)
        app, window = start(True)
        if args.save is None:
            # Same normal default-pilot entry used by package smoke tests.
            press("space")
            press("F5")
            press("p")
            capture("normal-default-pilot")
        geometry = dict(line.split("=", 1) for line in run("xdotool", "getwindowgeometry", "--shell", window).splitlines() if "=" in line)
        assert (geometry["WIDTH"], geometry["HEIGHT"]) == ("1280", "800")
        recorder = subprocess.Popen(["ffmpeg", "-v", "error", "-threads", "2", "-f", "x11grab", "-video_size", "1280x800", "-framerate", "30", "-i", f':95.0+{geometry["X"]},{geometry["Y"]}', "-t", "100", "-an", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-threads", "2", str(out / "five-languages.mp4")], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=(out / "recording.log").open("w"))
        press("F2")
        capture("options-zh-Hant")
        for locale in ("zh-Hans", "ja", "ko", "en", "zh-Hant"):
            change(locale)
            capture("options-" + locale)
            if locale in ("ja", "ko", "en"):
                press("Escape")
                press("e")
                capture("engineering-" + locale)
                press("Escape")
                press("F2")
        for locale in ("zh-Hans", "ja", "ko"):
            change(locale)
        press("Escape")
        capture("shipboard-ko")
        recorder.communicate(b"q\n", timeout=10)
        assert recorder.returncode == 0
        recorder = None
        press("q")
        assert app.wait(timeout=10) == 0
        saved = json.loads((out / "save.json").read_text())["state"]
        assert saved["schema"] == args.save_schema and saved["screen"] == "shipboard"
        assert preferences()["locale"] == "ko"
        app, window = start(False)
        assert preferences()["locale"] == "ko"
        capture("reopened-shipboard-ko")
        press("F2")
        capture("reopened-options-ko")
        press("Escape")
        press("q")
        assert app.wait(timeout=10) == 0
        if args.save is not None:
            assert sha(args.save) == source_hash
        receipt = {"status": "pass", "binarySHA256": sha(args.binary), "sourceSaveSHA256": source_hash, "sourcePreserved": True, "normalEntry": "saved game" if args.save is not None else "title / F5 / play default pilot", "diskSHA256": sha(args.disk), "locales": ["zh-Hant", "zh-Hans", "ja", "ko", "en"], "events": events, "images": images, "finalPreferences": preferences(), "scope": "normal entry, F2 language changes, engineering, preference persistence, quit and reopen; game-state isolation is separately tested without clock advancement"}
        (out / "receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n")
        print("PASS: five normal language options, three engineering views and Korean reopen")
    finally:
        if recorder is not None and recorder.poll() is None:
            recorder.communicate(b"q\n", timeout=10)
        if app is not None and app.poll() is None:
            app.terminate()
            app.wait(timeout=5)
        display.terminate()
        display.wait(timeout=5)


if __name__ == "__main__":
    main()
