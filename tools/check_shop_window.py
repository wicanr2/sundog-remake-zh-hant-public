#!/usr/bin/env python3
"""量原版店家視窗的內部幾何，當作可重跑的對拍閘門。

走進城市建築開出來的視窗（`Drahew Depot`、`Uniteller`、交易所…）不是點陣圖，是程式畫的
框加文字，所以**沒有磁碟底圖可以逐像素比**。能比的是版面：框有多寬、裡面幾條選項帶、
每條多高、間距多少。

**判準刻意不含視窗的絕對位置。** 同一個 Depot 在兩次執行裡開在不同座標——2026-09-05
用 laanwj port 量到藍框 x 98–253、選項帶上緣 129／142／155，而 `hatari-live-run.md`
先前用 Hatari 量到 x 74–229、上緣 81／94／107。位置差 (+24, +48)，但寬度 156、帶高 13、
間距 13 三者完全相同。**位置會變、內部幾何不會**，所以只有後者能當閘門。

這也讓它成為兩個獨立 oracle 的交叉驗證：Hatari 與 laanwj port 是兩份互不相干的實作，
在不同局、不同視窗位置下量到同一組內部幾何。

色號取自執行時調色盤（`check_ground_cells.to_runtime_palette`）：框是色號 9 的藍。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

from PIL import Image

from check_ground_cells import to_runtime_palette

# 執行時調色盤的色號 9。
FRAME_BLUE = (34, 68, 238)
# 一條選項帶的上下框各 2 px（實測成對出現）。
BORDER = 2


def frame_box(pixels, size) -> dict | None:
    """回傳藍框的外接矩形。"""
    width, height = size
    xs = [x for y in range(height) for x in range(width) if pixels[x, y] == FRAME_BLUE]
    if not xs:
        return None
    ys = [y for y in range(height) for x in range(width) if pixels[x, y] == FRAME_BLUE]
    return {"x0": min(xs), "x1": max(xs), "y0": min(ys), "y1": max(ys)}


def horizontal_rules(pixels, box: dict, min_run: int) -> list[int]:
    """回傳框內每一條水平藍線的 y。"""
    rules = []
    for y in range(box["y0"], box["y1"] + 1):
        run = sum(1 for x in range(box["x0"], box["x1"] + 1) if pixels[x, y] == FRAME_BLUE)
        if run >= min_run:
            rules.append(y)
    return rules


def bands(rules: list[int]) -> list[dict]:
    """把成對的水平線收成選項帶。

    每條帶的上下各有 BORDER 條線，所以線的序列是 2、2、2…；相鄰兩組之間就是一條帶。
    """
    groups: list[list[int]] = []
    for y in rules:
        if groups and y == groups[-1][-1] + 1:
            groups[-1].append(y)
        else:
            groups.append([y])
    out = []
    for upper, lower in zip(groups, groups[1:]):
        out.append({
            "top": upper[0],
            "bottom": lower[0],
            "height": lower[0] - upper[0],
            "borderTop": len(upper),
            "borderBottom": len(lower),
        })
    return out


def measure(image: Image.Image) -> dict:
    small = to_runtime_palette(image)
    pixels = small.load()
    box = frame_box(pixels, small.size)
    if box is None:
        return {"frame": None, "bands": []}
    width = box["x1"] - box["x0"] + 1
    # 一條帶的橫線至少要橫跨框寬的一半，才不會把文字裡的藍點算進來。
    rules = horizontal_rules(pixels, box, width // 2)
    return {"frame": {**box, "width": width}, "rules": rules, "bands": bands(rules)}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shot", type=pathlib.Path, help="店家視窗的實跑截圖")
    parser.add_argument("--expect-bands", type=int, help="預期的選項帶數")
    parser.add_argument("--expect-width", type=int, help="預期的框寬（像素）")
    parser.add_argument("--expect-band-height", type=int, help="預期的每條帶高度")
    args = parser.parse_args(argv)

    result = measure(Image.open(args.shot))
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))

    failures = []
    if result["frame"] is None:
        failures.append("畫面上找不到色號 9 的藍框")
    else:
        found = result["bands"]
        if args.expect_bands is not None and len(found) != args.expect_bands:
            failures.append(f"選項帶：期望 {args.expect_bands}，實得 {len(found)}")
        if args.expect_width is not None and result["frame"]["width"] != args.expect_width:
            failures.append(f"框寬：期望 {args.expect_width}，實得 {result['frame']['width']}")
        if args.expect_band_height is not None:
            wrong = [b for b in found if b["height"] != args.expect_band_height]
            if wrong:
                failures.append(
                    f"帶高：期望全部 {args.expect_band_height}，"
                    f"有 {len(wrong)} 條不是（{[b['height'] for b in wrong]}）")
        # 帶與帶必須等距——不等距表示這不是一疊選項帶，判準就不成立。
        heights = {b["height"] for b in found}
        if len(heights) > 1:
            failures.append(f"帶高不一致：{sorted(heights)}")
        if len(found) >= 2:
            borders = {b["borderTop"] for b in found} | {b["borderBottom"] for b in found}
            if borders != {BORDER}:
                failures.append(f"框線厚度不是全部 {BORDER}：{sorted(borders)}")

    if failures:
        print("FAIL：" + "；".join(failures))
        return 1
    print(f"PASS：框寬 {result['frame']['width']}、"
          f"{len(result['bands'])} 條選項帶、每條高 {result['bands'][0]['height']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
