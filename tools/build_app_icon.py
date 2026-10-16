#!/usr/bin/env python3
"""產生桌面套件用的應用程式圖示。

圖示取自重製版自己的 Modern Apple atlas 裡的船艦格，放在深色底上。素材是重製版原創，
不含任何原版圖像。產生一次就進版控，封裝流程只要複製，不必在打包時再跑一次影像處理。
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from PIL import Image

CANVAS = 256
INSET = 16
BACKGROUND = (18, 21, 24, 255)


def build(repo: Path, output: Path) -> None:
    manifest_path = repo / "internal/theme/assets/modern-apple/modern-apple-atlas-manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    tile = manifest["tiles"]["ship"]
    with Image.open(repo / "internal/theme/assets/modern-apple/modern-apple-atlas.png") as atlas:
        cell = atlas.convert("RGBA").crop(
            (tile["x"], tile["y"], tile["x"] + tile["width"], tile["y"] + tile["height"])
        )
    canvas = Image.new("RGBA", (CANVAS, CANVAS), BACKGROUND)
    size = CANVAS - INSET * 2
    canvas.alpha_composite(cell.resize((size, size), Image.Resampling.LANCZOS), (INSET, INSET))
    output.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(output)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path("."))
    parser.add_argument("--output", type=Path, default=Path("assets/icons/sundog-remake.png"))
    args = parser.parse_args()
    build(args.repo, args.output)
    print(args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
