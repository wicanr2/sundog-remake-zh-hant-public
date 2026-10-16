#!/usr/bin/env python3
"""量出原版介面框的分區座標。

原版每個畫面各有一張 320×200 的整版框，框線與按鈕都畫在圖裡。要照著做版面就得先知道
每一塊的座標，這支工具把它量出來，而不是用眼睛估。

作法：框裡的每一塊都由一圈**單一顏色的邊框**圍住（黃、藍）。把同色的連通區塊找出來，
取它的外接矩形，就是那一塊的位置。這比找水平／垂直長線可靠——長線會被文字與圖示打斷。

輸入是 tools/decode_atari_st_images.py 解出來的 PNG；本工具不碰原始磁碟。
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from PIL import Image

# 框線的顏色。原版只有 16 色，這三個是框用的。
BORDER_COLOURS = {
    "yellow": (255, 255, 0),
    "blue": (36, 73, 255),
    "dim-blue": (0, 0, 182),
    "dim-yellow": (146, 109, 0),
}
# 小於這個像素數的連通區塊是文字或圖示的一部分，不是框。
MINIMUM_BORDER_PIXELS = 40


def connected_boxes(image: Image.Image, colour: tuple[int, int, int]) -> list[dict[str, int]]:
    """回傳該顏色每一個連通區塊的外接矩形。"""
    pixels = image.load()
    width, height = image.size
    seen = [[False] * width for _ in range(height)]
    found: list[dict[str, int]] = []
    for start_y in range(height):
        for start_x in range(width):
            if seen[start_y][start_x] or pixels[start_x, start_y] != colour:
                continue
            stack = [(start_x, start_y)]
            seen[start_y][start_x] = True
            xs, ys = [start_x], [start_y]
            while stack:
                x, y = stack.pop()
                for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                    nx, ny = x + dx, y + dy
                    if 0 <= nx < width and 0 <= ny < height and not seen[ny][nx]:
                        if pixels[nx, ny] == colour:
                            seen[ny][nx] = True
                            stack.append((nx, ny))
                            xs.append(nx)
                            ys.append(ny)
            if len(xs) < MINIMUM_BORDER_PIXELS:
                continue
            found.append(
                {
                    "x": min(xs),
                    "y": min(ys),
                    "width": max(xs) - min(xs) + 1,
                    "height": max(ys) - min(ys) + 1,
                    "borderPixels": len(xs),
                }
            )
    return found


def measure(path: Path) -> dict[str, object]:
    image = Image.open(path).convert("RGB")
    if image.size != (320, 200):
        raise ValueError(f"{path} 是 {image.size}，不是原版的 320×200 整版框")
    regions = {
        name: connected_boxes(image, colour) for name, colour in BORDER_COLOURS.items()
    }
    return {
        "schema": 1,
        "tool": "tools/atari_st_ui_frame.py",
        "source": {
            "image": path.name,
            "note": "由 tools/decode_atari_st_images.py 從玩家自己的原版磁碟解出；像素不入庫。",
        },
        "whatThisProves": (
            "每一塊的座標是從框線的連通區塊量出來的，不是估的。"
            "每一塊「是什麼」是人看圖讀出來的，寫在 labels 裡。"
        ),
        "regions": regions,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image", type=Path, help="解出來的 320×200 介面框 PNG")
    parser.add_argument("--output", type=Path, help="輸出 JSON；未指定則印到 stdout")
    args = parser.parse_args()
    document = measure(args.image)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
