#!/usr/bin/env python3
"""僅在 Docker 內生成現行地面夾具，再以唯讀 Talos overlay 執行原版碰撞指令。

原版衍生像素只寫呼叫者指定的本機研究輸出，不進 Git／公開封包。
契約及 Issue 入口見 docs/atari-st/native-collision-entry-20260908.md。
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run_logged(command, log, *, cwd, env, allow_failure=False, timeout=150):
    with log.open("w", encoding="utf-8") as stream:
        result = subprocess.run(command, cwd=cwd, env=env, stdout=stream,
                                stderr=subprocess.STDOUT, timeout=timeout)
    if result.returncode and not allow_failure:
        raise RuntimeError(f"命令回傳 {result.returncode}，見 {log}")
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", required=True, type=Path)
    parser.add_argument("--talos", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--include-drawn-marks", action="store_true",
                        help="供應原版字元疊完後的像素，額外核對飛船與停放物件")
    parser.add_argument("--pod-patterns", action="store_true",
                        help="核對載具0–3圖樣及色彩7、8受阻旗標；每場景輸入去重")
    parser.add_argument("--legacy-background-only", action="store_true",
                        help="診斷用 Go overlay 重播修正前只碰撞底圖的策略，不進正式路徑")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("此工具只能在 Docker 內執行")
    repo = Path(__file__).resolve().parents[1]
    disk, talos, output = args.disk.resolve(), args.talos.resolve(), args.output.resolve()
    interp = talos / "workplace/sundog/SYSTEM.INTERP"
    expected = "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6"
    if not disk.is_file() or digest(disk) != expected:
        parser.error("原版磁碟來源或雜湊不符")
    if not interp.is_file() or digest(interp) != "a344edfb07d27cafa3dfda68f1854a76f63a0e89cf2e8229dacf5aa64d603c38":
        parser.error("Talos 直譯器來源或雜湊不符")
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        parser.error("輸出必須是新路徑或空目錄，不能覆寫舊收據")
    if output.parent.stat().st_uid != os.getuid():
        parser.error("輸出父目錄擁有者不符，請先確認 UID／掛載")
    output.mkdir(mode=0o700, exist_ok=True)
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    env["SUNDOG_PARITY_DRAWN_MARKS"] = "1" if args.include_drawn_marks else "0"
    env["SUNDOG_PARITY_POD_PATTERNS"] = "1" if args.pod_patterns else "0"
    go_version = subprocess.check_output(["go", "version"], text=True).strip()
    if go_version.split()[2] != "go1.25.12":
        parser.error("請使用 sundog-remake-build:20260812 的固定 Go 1.25.12 工具鏈")
    env["DISPLAY"] = ":92"
    xvfb_log = (output / "xvfb.log").open("w", encoding="utf-8")
    xvfb = subprocess.Popen(["Xvfb", ":92", "-screen", "0", "1280x800x24", "-nolisten", "tcp"],
                            stdout=xvfb_log, stderr=subprocess.STDOUT, env=env)
    try:
        for _ in range(50):
            if xvfb.poll() is not None:
                raise RuntimeError("Xvfb 提前退出，請查看 xvfb.log")
            if subprocess.run(["xdpyinfo"], env=env, stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(0.1)
        else:
            raise RuntimeError("Xvfb 未就緒")
        remake_command = ["go", "test"]
        if args.legacy_background_only:
            source_path = repo / "cmd/sundog-ebiten/stground.go"
            text = source_path.read_text(encoding="utf-8")
            call = "\ta.paintSTGroundMarkIndices(indices, marks)"
            if text.count(call) != 1:
                raise RuntimeError("無法唯一定位已核定修正，拒絕重建舊策略")
            legacy = output / "legacy-stground.go.txt"
            legacy.write_text(text.replace(call, "\t// 診斷 overlay：舊策略未將 marks 寫入碰撞。"), encoding="utf-8")
            legacy_overlay = output / "remake-overlay.json"
            legacy_overlay.write_text(json.dumps({"Replace": {str(source_path): str(legacy)}}), encoding="utf-8")
            remake_command.extend(["-overlay", str(legacy_overlay)])
        run_logged(remake_command + ["./cmd/sundog-ebiten", "-run",
                                    "^TestExportTalosGroundCollisionFixtures$", "-count=1", "-v"],
                   output / "remake.log", cwd=repo, env=env)
        source = repo / "tools/talos_ground_collision_test.go.txt"
        with tempfile.TemporaryDirectory(prefix="sundog-talos-overlay-") as temporary:
            overlay = Path(temporary) / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {
                str(talos / "internal/m68k/sundog_ground_collision_test.go"): str(source)
            }}), encoding="utf-8")
            talos_status = run_logged(["go", "test", "-overlay", str(overlay), "./internal/m68k", "-run",
                                      "^TestSunDogGroundCollisionParity$", "-count=1", "-v"],
                                     output / "talos.log", cwd=talos, env=env, allow_failure=True)
        native_receipt = output / "native-comparison.json"
        if not native_receipt.is_file():
            raise RuntimeError(f"Talos 未產生判定收據（回傳 {talos_status}），見 {output / 'talos.log'}；不可視為產品差異")
        receipt = json.loads(native_receipt.read_text(encoding="utf-8"))
        fixtures = json.loads((output / "collision-fixtures.json").read_text(encoding="utf-8"))
        expected_samples = sum(len(scene["Samples"]) for scene in fixtures["Scenes"])
        if receipt["Samples"] != expected_samples or expected_samples < 76 * 416:
            raise RuntimeError("原版對拍結果未達本次樣本契約")
        sources = sorted(path for directory in (repo / "cmd", repo / "internal")
                         for path in directory.rglob("*")
                         if path.is_file() and path.suffix in (".go", ".json"))
        sources.extend([source, Path(__file__).resolve(), repo / "go.mod", repo / "go.sum"])
        metadata = {
            "issue": "https://github.com/wicanr2/sundog-remake-zh-hant/issues/8" if args.pod_patterns else "https://github.com/wicanr2/sundog-remake-zh-hant/issues/2",
            "scope": "same-input-native-pixel-collision",
            "limitations": ["not a normal player path", "no full-scene appearance RNG parity",
                            "does not validate movement scheduling"]
                + ([] if args.include_drawn_marks else ["does not validate drawn marks"]),
            "go_version": go_version,
            "disk_input": {"filename": disk.name, "sha256": expected},
            "source_sha256": {str(path.relative_to(repo)): digest(path) for path in sources},
            "talos_source_sha256": {str(path.relative_to(talos)): digest(path)
                for path in sorted((talos / "internal/m68k").glob("*.go"))},
            "artifacts": {path.name: digest(path) for path in sorted(output.iterdir()) if path.is_file()},
            "samples": receipt["Samples"], "differences": receipt["Differences"],
            "includes_drawn_marks": args.include_drawn_marks,
            "pod_patterns": args.pod_patterns,
            "patterns": [0, 1, 2, 3] if args.pod_patterns else [4, 6, 7, 8, 9],
            "legacy_policy_reproduction": args.legacy_background_only,
            "status": "PASS" if talos_status == 0 and receipt["Differences"] == 0 else "FAIL",
        }
        (output / "receipt.json").write_text(json.dumps(metadata, indent=2, ensure_ascii=False) + "\n",
                                            encoding="utf-8")
        if metadata["status"] != "PASS":
            raise RuntimeError(f"原版對拍不符：{receipt['Differences']} 案，見 {output / 'receipt.json'}")
        print(f"Talos 原始碰撞：{receipt['Samples']} 案，差異 0；收據 {output / 'receipt.json'}")
    finally:
        xvfb.terminate()
        try:
            xvfb.wait(timeout=5)
        except subprocess.TimeoutExpired:
            xvfb.kill()
            xvfb.wait(timeout=5)
        xvfb_log.close()


if __name__ == "__main__":
    main()
