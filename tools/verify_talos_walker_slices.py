"""Docker 內執行原版人物切片；契約見 docs/atari-st/walker-timing-spec.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

from verify_talos_ground_collision import run_logged


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--disk", type=Path, required=True)
    parser.add_argument("--talos", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
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
        parser.error("請使用現有固定 Go 1.25.12 工具鏈")
    output.mkdir(mode=0o700, exist_ok=True)
    env = dict(os.environ, SUNDOG_PARITY_DISK=str(disk), SUNDOG_PARITY_OUTPUT=str(output),
               TALOS_UCSD_INTERP=str(interp.parent), GOPROXY="off", GOTOOLCHAIN="local")
    source = repo / "tools/talos_walker_slices_test.go.txt"
    env["DISPLAY"] = ":92"
    with (output / "xvfb.log").open("w", encoding="utf-8") as log:
        xvfb = subprocess.Popen(["Xvfb", ":92", "-screen", "0", "1280x800x24", "-nolisten", "tcp"],
                                stdout=log, stderr=subprocess.STDOUT, env=env)
        try:
            for _ in range(50):
                if xvfb.poll() is not None:
                    raise RuntimeError("Xvfb 提前退出")
                if subprocess.run(["xdpyinfo"], env=env, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL).returncode == 0:
                    break
                time.sleep(0.1)
            else:
                raise RuntimeError("Xvfb 未就緒")
            run_logged(["go", "test", "./cmd/sundog-ebiten", "-run", "^TestExportTalosWalkerSequences$",
                        "-count=1", "-v"], output / "remake.log", cwd=repo, env=env)
            with tempfile.TemporaryDirectory(prefix="sundog-walker-overlay-") as temp:
                overlay = Path(temp) / "overlay.json"
                overlay.write_text(json.dumps({"Replace": {
                    str(talos / "internal/m68k/sundog_walker_slices_test.go"): str(source)}}), encoding="utf-8")
                status = run_logged(["go", "test", "-overlay", str(overlay), "./internal/m68k",
                                     "-run", "^TestSunDog(SpriteMoveHaltWrites|NativeWalkerAxisStep|NativeWalkerFacingFamilies|NativeWalkerSequences|NativeCollisionCandidateOrder|NativeSpriteGroupSchedule|NativeSpriteUpdateGates)$",
                                     "-count=1", "-v"], output / "talos.log", cwd=talos, env=env, allow_failure=True)
        finally:
            xvfb.terminate()
            try:
                xvfb.wait(timeout=5)
            except subprocess.TimeoutExpired:
                xvfb.kill()
                xvfb.wait(timeout=5)
    required_tests = [
        "TestSunDogSpriteMoveHaltWrites", "TestSunDogNativeWalkerAxisStep",
        "TestSunDogNativeWalkerFacingFamilies", "TestSunDogNativeWalkerSequences",
        "TestSunDogNativeCollisionCandidateOrder", "TestSunDogNativeSpriteGroupSchedule",
        "TestSunDogNativeSpriteUpdateGates"]
    passed_tests = {line.split()[2] for line in (output / "talos.log").read_text(encoding="utf-8").splitlines()
                    if line.startswith("--- PASS: ")}
    if status == 0 and not set(required_tests).issubset(passed_tests):
        raise RuntimeError("Go 返回成功，但缺少本次必要的原版切片測試")
    sources = sorted(p for directory in [repo / "cmd", repo / "internal"]
                     for p in directory.rglob("*") if p.is_file() and p.suffix in (".go", ".json"))
    sources.extend([source, Path(__file__).resolve(), repo / "tools/verify_talos_ground_collision.py",
                    repo / "go.mod", repo / "go.sum"])
    report = {"issue": "https://github.com/wicanr2/sundog-remake-zh-hant/issues/4",
              "scope": "native/p-code isolated slices; no full-machine timing",
              "go_version": version, "disk_sha256": sha(disk), "interp_sha256": sha(interp),
              "source_sha256": {str(p.relative_to(repo)): sha(p) for p in sources},
              "required_tests": required_tests, "passed_tests": sorted(passed_tests),
              "talos_source_sha256": {str(p.relative_to(talos)): sha(p)
                  for p in sorted((talos / "internal/m68k").glob("*.go"))},
              "log_sha256": sha(output / "talos.log"), "status": "PASS" if status == 0 else "FAIL"}
    comparison = output / "native-walker-comparison.json"
    if comparison.is_file():
        native = json.loads(comparison.read_text(encoding="utf-8"))
        report.update(updates=native["Updates"], differences=native["Differences"],
                      artifacts={p.name: sha(p) for p in sorted(output.iterdir()) if p.is_file()})
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    if status:
        raise RuntimeError(f"原版人物切片失敗，見 {output / 'talos.log'}")
    print(f"原版人物切片通過：{output / 'receipt.json'}")


if __name__ == "__main__":
    main()
