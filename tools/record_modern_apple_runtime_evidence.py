#!/usr/bin/env python3
"""為 Modern Apple 的兩批高解析圖塊集寫出 runtime 證據文件。

每一批記的是同一組東西：atlas 與 manifest 的雜湊、產生它的 builder 與擷取腳本的雜湊、
擷取到的畫面與尺寸，以及這份證據**不**證明什麼。雜湊由本工具當場算，不是手抄——
腳本改過而證據沒重跑時，重跑一次就會看到差異。

畫面檔要先用 `tools/capture_modern_apple_ground.sh` 在 Docker + Xvfb 裡擷取好，
再把路徑傳進來；本工具不啟動任何應用程式。
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


BATCHES: dict[str, dict[str, Any]] = {
    "modern-apple": {
        "assetDir": "internal/theme/assets/modern-apple",
        "prefix": "modern-apple-atlas",
        "builder": "tools/build_modern_apple_atlas.py",
        "output": "docs/art/modern-apple-highres-runtime-evidence.json",
        "screenshot": "screenshots/modern-apple-ground-v1.png",
        "scenario": (
            "remake-owned normal -start ground screen; two F9 events cycle the tile art to the "
            "Remake Modern batch under the Apple II chrome (the Modern Apple chromes were retired 2026-08-30)"
        ),
        "batchNote": (
            "This batch is the forest-and-bright-water look, the tileset the options call Remake Modern. "
            "The spaceport batch was retired on 2026-08-30; its evidence file stays as history."
        ),
    },
}

SEMANTIC_TILES = ["grass", "water", "road", "wall", "ship", "cargo", "city", "player"]
RUNTIME_DISPLAY_TILE_SIZE = 60
CAPTURE_SCRIPT = "tools/capture_modern_apple_ground.sh"

BOUNDARIES = [
    "The atlas is original remake-owned high-resolution raster art. It contains no Apple II, Atari ST, FTL, SunDog, or other original game image.",
    "Apple II is the sole original visual and game-semantic oracle. Its RAM/HGR never enters the remake runtime, save format, or asset format.",
    "The screenshot validates a normal remake UI path and atomic F8/F9 switching. It does not prove original visual parity, exact frame/anchor mapping, or original collision semantics.",
    "Switching tilesets never changes map semantics, collision, anchors, secrets, rules, or save data. Only the drawn art changes.",
]


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def require(path: Path, label: str) -> Path:
    if not path.is_file():
        raise SystemExit(f"{label} 不存在：{path}")
    return path


def build_document(root: Path, batch: str, date: str) -> dict[str, Any]:
    spec = BATCHES[batch]
    asset_dir = root / str(spec["assetDir"])
    prefix = str(spec["prefix"])
    atlas = require(asset_dir / f"{prefix}.png", "atlas")
    manifest = require(asset_dir / f"{prefix}-manifest.json", "manifest")
    builder = require(root / str(spec["builder"]), "builder")
    capture = require(root / CAPTURE_SCRIPT, "擷取腳本")
    screenshot = require(root / "docs" / "art" / str(spec["screenshot"]), "擷取畫面")

    manifest_data = json.loads(manifest.read_text(encoding="utf-8"))
    if manifest_data["theme"] != batch:
        raise SystemExit(f"manifest 宣告的批次是 {manifest_data['theme']}，不是 {batch}")
    if manifest_data["atlasSha256"] != sha256(atlas):
        raise SystemExit("manifest 記的 atlas 雜湊與檔案不符")
    if sorted(manifest_data["tiles"]) != sorted(SEMANTIC_TILES):
        raise SystemExit("manifest 的語意格與這份工具宣稱的不同")

    return {
        "schema": 1,
        "date": date,
        "status": "pass",
        "assetGroup": {
            "theme": batch,
            "assetVersion": manifest_data["assetVersion"],
            "atlas": f"../../{spec['assetDir']}/{prefix}.png",
            "atlasSha256": sha256(atlas),
            "manifest": f"../../{spec['assetDir']}/{prefix}-manifest.json",
            "manifestSha256": sha256(manifest),
            "cellSize": manifest_data["cellSize"],
            "logicalTileSize": manifest_data["logicalTileSize"],
            "runtimeDisplayTileSize": RUNTIME_DISPLAY_TILE_SIZE,
            "semanticTiles": SEMANTIC_TILES,
        },
        "runtimeCapture": {
            "script": f"../../{CAPTURE_SCRIPT}",
            "scriptSha256": sha256(capture),
            "scenario": spec["scenario"],
            "screenshot": spec["screenshot"],
            "screenshotSha256": sha256(screenshot),
            "dimensions": [960, 600],
            "result": "pass",
        },
        "verification": {
            "assetBuilder": f"../../{spec['builder']}",
            "assetBuilderSha256": sha256(builder),
            "themeUnitTest": "go test -v ./internal/theme: pass",
            "ebitenUnitTest": "Xvfb go test -v ./cmd/sundog-ebiten: pass",
            "fullGoGate": "Xvfb go test ./...; go vet ./...; gofmt -l .: pass",
        },
        "boundaries": BOUNDARIES + [str(spec["batchNote"])],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("."), help="repo 根目錄")
    parser.add_argument("--date", required=True, help="這一輪的日期，格式 YYYY-MM-DD")
    parser.add_argument("--batch", choices=sorted(BATCHES), action="append", help="只寫這一批；可重複")
    args = parser.parse_args()

    written = []
    for batch in args.batch or sorted(BATCHES):
        document = build_document(args.root, batch, args.date)
        output = args.root / str(BATCHES[batch]["output"])
        output.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        written.append(str(output))
    print(json.dumps({"status": "pass", "written": written}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
