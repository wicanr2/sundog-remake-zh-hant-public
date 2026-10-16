#!/usr/bin/env python3
"""驗證導航圖上的星系記號落在原版的投影格點上。

原版把星系畫在中央視埠時用的是一個固定的投影：`x = 104 + 12·gx`、`y = 14 + 10·gy`
（`docs/atari-st/screen-geometry.md`）。如果那組常數是對的，畫面上每個記號的中心換算回去
就該是整數——不是「接近整數」，是整數。這支工具就是拿實跑畫面去驗那件事。

它不預設有幾個記號、也不預設位置：把視埠裡的白色連通區塊全部找出來，逐一換算。
記號大小不一（半徑隨星系大小變），所以判準只看中心，不看面積。

**這是格點驗證，不是逐像素對拍。** 它能證明投影常數與畫面一致，不能證明畫面上每個像素
都來自磁碟——那要另外對底圖比（見 `check_title_pixel_parity.py`）。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

from PIL import Image

from check_ground_cells import to_runtime_palette

# 中央視埠（與 compare_appimage_to_original 的動態矩形之一相同）。
VIEWPORT = (96, 6, 258, 118)
# screen-geometry.md 的星圖投影：x = 104 + 12·gx、y = 14 + 10·gy。
ORIGIN = (104, 14)
PITCH = (12, 10)
WHITE = (0xEE, 0xEE, 0xEE)


def find_marks(image: Image.Image, viewport=VIEWPORT) -> list[dict]:
    """回傳視埠裡每個白色連通區塊的中心與大小。"""
    px = image.load()
    x0, y0, x1, y1 = viewport
    seen: set[tuple[int, int]] = set()
    marks: list[dict] = []
    for y in range(y0, y1):
        for x in range(x0, x1):
            if px[x, y] != WHITE or (x, y) in seen:
                continue
            stack = [(x, y)]
            seen.add((x, y))
            points = []
            while stack:
                cx, cy = stack.pop()
                points.append((cx, cy))
                for dx in (-1, 0, 1):
                    for dy in (-1, 0, 1):
                        n = (cx + dx, cy + dy)
                        if (
                            x0 <= n[0] < x1
                            and y0 <= n[1] < y1
                            and n not in seen
                            and px[n] == WHITE
                        ):
                            seen.add(n)
                            stack.append(n)
            xs = [p[0] for p in points]
            ys = [p[1] for p in points]
            marks.append({
                "pixels": len(points),
                "centre": [(min(xs) + max(xs)) / 2, (min(ys) + max(ys)) / 2],
                "size": [max(xs) - min(xs) + 1, max(ys) - min(ys) + 1],
            })
    marks.sort(key=lambda m: -m["pixels"])
    return marks


def to_grid(centre: list[float]) -> list[float]:
    return [(centre[0] - ORIGIN[0]) / PITCH[0], (centre[1] - ORIGIN[1]) / PITCH[1]]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shot", type=pathlib.Path, help="導航圖的實跑截圖")
    parser.add_argument("--expect-count", type=int, help="預期的星系記號數量")
    parser.add_argument(
        "--tolerance",
        type=float,
        default=0.26,
        help="換算後距離整數格點的容許誤差（預設 0.26，約四分之一格）",
    )
    args = parser.parse_args(argv)

    marks = find_marks(to_runtime_palette(Image.open(args.shot)))
    rows = []
    off_grid = []
    for mark in marks:
        grid = to_grid(mark["centre"])
        error = max(abs(v - round(v)) for v in grid)
        row = {**mark, "grid": grid, "error": round(error, 3)}
        rows.append(row)
        if error > args.tolerance:
            off_grid.append(row)

    print(json.dumps({"marks": rows, "offGrid": off_grid}, ensure_ascii=False, indent=2))
    if off_grid:
        print(f"FAIL：{len(off_grid)} 個記號不在格點上")
        return 1
    if args.expect_count is not None and len(marks) != args.expect_count:
        print(f"FAIL：找到 {len(marks)} 個記號，期望 {args.expect_count}")
        return 1
    print(f"PASS：{len(marks)} 個星系記號全部落在整數格點上")
    return 0


if __name__ == "__main__":
    sys.exit(main())
