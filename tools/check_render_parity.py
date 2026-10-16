#!/usr/bin/env python3
"""比對同一個場景在不同平台建置下的離屏輸出是否一模一樣。

WORKLIST 的 D1／D2 要的是實機驗收，這支工具不是那個。它補的是實機驗收之外、代理做得到
的那一半：**繪圖管線的輸出在不同平台的建置之間有沒有差**。

`cmd/sundog-preview` 走 `internal/render` 直接輸出 PNG，不開視窗、不碰 GPU，所以同一份
原始碼交叉編出來的 Linux 與 Windows binary 可以各跑一次再逐位元比。差異若存在，就是
Go 的浮點、字型 rasterization 或平台相依的行為造成的，與顯示卡或視窗系統無關。

**不是每個場景都能當閘門。** 帶新遊戲狀態的畫面含每局重擲的星圖，同一個平台跑兩次就
不一樣了（同平台 58 個像素、跨平台 5 個——跨平台差異被自己的雜訊蓋過去）。所以判準是
先確認場景在同一平台可重現，再跨平台比；`--same-platform` 就是那道前置閘門。

全等只說明「這兩個建置畫出來的像素相同」。它不能代替在真的 Windows 上跑一次——
驅動、DPI、檔案系統與視窗行為都不在這支工具的視野裡。
"""

from __future__ import annotations

import argparse
import hashlib
import pathlib
import sys

from PIL import Image


def sha256(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def pixel_diff(
    left: pathlib.Path, right: pathlib.Path
) -> tuple[int, int, list[tuple[int, int, tuple, tuple]]]:
    """回傳（相異數, 總數, 前幾個相異點）。尺寸不同就拋例外。"""
    a = Image.open(left).convert("RGBA")
    b = Image.open(right).convert("RGBA")
    if a.size != b.size:
        raise ValueError(f"尺寸不同：{a.size} vs {b.size}")
    pa = a.load()
    pb = b.load()
    diffs: list[tuple[int, int, tuple, tuple]] = []
    count = 0
    for y in range(a.height):
        for x in range(a.width):
            if pa[x, y] != pb[x, y]:
                count += 1
                if len(diffs) < 8:
                    diffs.append((x, y, pa[x, y], pb[x, y]))
    return count, a.width * a.height, diffs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("left", type=pathlib.Path, help="一邊的 PNG（例如 Linux 建置）")
    parser.add_argument("right", type=pathlib.Path, help="另一邊的 PNG（例如 Windows 建置）")
    parser.add_argument(
        "--same-platform",
        nargs=2,
        type=pathlib.Path,
        metavar=("A", "B"),
        help="同一平台跑兩次的輸出；先證明場景可重現，跨平台的結論才有意義",
    )
    args = parser.parse_args(argv)

    if args.same_platform:
        a, b = args.same_platform
        if sha256(a) != sha256(b):
            count, total, diffs = pixel_diff(a, b)
            print(f"FAIL：場景在同一平台就不可重現，{a.name} 與 {b.name} 差 {count}/{total} 像素")
            for x, y, l, r in diffs:
                print(f"  ({x},{y}) {l} vs {r}")
            print("這種場景不能當跨平台閘門——先找出不決定的來源。")
            return 1
        print(f"同平台可重現：{a.name} 與 {b.name} 逐位元相同")

    left_hash = sha256(args.left)
    right_hash = sha256(args.right)
    if left_hash == right_hash:
        print(f"PASS：{args.left.name} 與 {args.right.name} 逐位元相同")
        print(f"  SHA-256 {left_hash}")
        return 0

    count, total, diffs = pixel_diff(args.left, args.right)
    print(f"FAIL：兩個建置的輸出差 {count}/{total} 個像素")
    print(f"  {args.left.name}  {left_hash}")
    print(f"  {args.right.name} {right_hash}")
    for x, y, l, r in diffs:
        print(f"  ({x},{y}) {l} vs {r}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
