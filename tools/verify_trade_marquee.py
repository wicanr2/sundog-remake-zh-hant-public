#!/usr/bin/env python3
"""Docker 內量測庫存字型及正常 F12 畫面。入口：banville-restoration-spec.md。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--library", type=Path, required=True)
    parser.add_argument("--shot", type=Path, required=True)
    parser.add_argument("--locale", choices=["zh-Hant", "en", "zh-Hans", "ja"], default="zh-Hant")
    args = parser.parse_args()
    if not Path("/.dockerenv").is_file():
        parser.error("只能在 Docker 執行")
    repo = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    if output.exists() or output.parent.stat().st_uid != os.getuid():
        parser.error("需要目前使用者擁有的全新輸出路徑")
    library, shot = args.library.resolve(strict=True), args.shot.resolve(strict=True)
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if version.split()[2] != "go1.25.12":
        parser.error("須沿用 Go 1.25.12")
    output.mkdir(mode=0o700)
    source = repo / "tools/trade_marquee_probe.go.txt"
    probe = output / "probe.go"
    probe.write_bytes(source.read_bytes())
    command = ["go", "run", "-buildvcs=false", str(probe), "-library", str(library),
               "-shot", str(shot), "-locale", args.locale]
    result = subprocess.run(command, cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, timeout=120)
    (output / "probe.json").write_text(result.stdout)
    (output / "stderr.log").write_text(result.stderr)
    if result.returncode:
        raise RuntimeError(f"字型量測失敗，見 {output}")
    measured = json.loads(result.stdout)
    if measured["pixel"]["missingInkPixelsAbove32"] or measured["pixel"]["fitInsertedEllipsis"]:
        raise RuntimeError(f"正常截圖出現缺失字形或刪節號，見 {output}")
    sources = [source, Path(__file__).resolve(), repo / "internal/game/exchange.go",
               repo / "cmd/sundog-ebiten/stshop.go", repo / "cmd/sundog-ebiten/main.go",
               repo / "internal/localization/layout.go", repo / "assets/fonts/NotoSansCJKtc-Regular.otf"]
    sources += sorted((repo / "docs/i18n").glob("sundog-ui-*.json"))
    report = {"status": "PASS-font-and-normal-remake-shot", "measurement": measured,
              "command": command, "go": version, "locale": args.locale,
              "librarySHA256": sha(library), "shotSHA256": sha(shot),
              "sources": {str(p.relative_to(repo)): sha(p) for p in sources},
              "originalGUI": "unverified", "pixelEquality": "not-exact; recorded grayscale differences"}
    (output / "receipt.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(f"PASS:{measured['font']['Cases']}文字窗口／{measured['font']['InkCases']}字形；缺失墨點0")


if __name__ == "__main__":
    main()
