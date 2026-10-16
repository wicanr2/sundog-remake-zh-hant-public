#!/usr/bin/env python3
"""把 port oracle 拍到的城市地面逐格對回磁碟上的圖塊。

原版的地面畫面就是 320×200 切成 8×5 格、每格 40 像素，所以判定可以逐格做：這一格整格是
背景圖、整格全黑、內縮 4px 的 32×32 是磁碟上的某個圖塊，還是以上皆非。分格與判定沿用
`compare_appimage_to_original.classify_ground_cells`，不另寫一份——那支工具用它比 remake，
這支用它比原版實跑畫面，兩邊量的是同一件事。

差別只有色階。`laanwj/sundog` port 把 Atari ST 的 3 位元色展開成 `v*255/7`，比較器那邊用的是
`v*0x22`；兩種都對，但擺在一起比就要先收回 3 位元再用同一種展開。

`mixed` 不等於錯：玩家與訊息窗蓋住的格子本來就不會等於任何一個圖塊。要看的是
`unknownMixedCells`——那才是需要解釋的。

給 `--remake` 就多做一件事：把重製版在同一狀態下的地面也判一次，兩份結果逐欄位比。
這是 UX 台帳說的 same-state 升級——先前重製版只能對磁碟的靜態資料比，現在可以對
**原版在同一狀態下實際畫出來的畫面**比，兩邊還是同一把尺。
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

from PIL import Image

from compare_appimage_to_original import classify_ground_cells, downscale3

ST_SIZE = (320, 200)


def to_runtime_palette(shot: Image.Image) -> Image.Image:
    """把 port 的截圖縮回 320×200，並把色階換成比較器用的那一種。"""
    small = shot.convert("RGB").resize(ST_SIZE, Image.NEAREST)
    pixels = small.load()
    for y in range(ST_SIZE[1]):
        for x in range(ST_SIZE[0]):
            r, g, b = pixels[x, y]
            level = tuple(int(round(c * 7.0 / 255.0)) for c in (r, g, b))
            pixels[x, y] = tuple(v * 0x22 for v in level)
    return small


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shot", type=pathlib.Path, help="port oracle 的地面截圖")
    parser.add_argument("disk", type=pathlib.Path, help="原版 raw .st")
    parser.add_argument(
        "--remake",
        type=pathlib.Path,
        help="重製版在同一狀態下的地面截圖（3× 擷取）；給了就兩邊逐欄位比",
    )
    parser.add_argument(
        "--directory",
        type=pathlib.Path,
        default=pathlib.Path(__file__).parent.parent / "docs/atari-st/image-directory.json",
    )
    args = parser.parse_args(argv)

    disk = args.disk.read_bytes()
    directory = json.loads(args.directory.read_text())
    result = classify_ground_cells(to_runtime_palette(Image.open(args.shot)), disk, directory)
    print("原版實跑：")
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    if result["unknownMixedCells"]:
        print("FAIL：原版畫面有無法解釋的格子")
        return 1

    if not args.remake:
        return 0

    remake = classify_ground_cells(downscale3(Image.open(args.remake)), disk, directory)
    print("重製版同狀態：")
    print(json.dumps(remake, ensure_ascii=False, indent=2, sort_keys=True))
    if remake != result:
        print("FAIL：兩邊的格子構成不同")
        for key in sorted(set(result) | set(remake)):
            if result.get(key) != remake.get(key):
                print(f"  {key}：原版 {result.get(key)} vs 重製版 {remake.get(key)}")
        return 1
    print("PASS：原版與重製版在同一狀態下，40 格的分類與覆蓋格位置逐欄位相同")
    return 0


if __name__ == "__main__":
    sys.exit(main())
