#!/usr/bin/env python3
"""驗證實跑出來的標題畫面與磁碟解出的原版影像是同一份像素。

原版有兩條實跑路徑，兩條都不是「當年那台機器」：Hatari 配 EmuTOS（不是 SunDog 當年跑的
TOS），以及公開的 `laanwj/sundog` port（重寫的 SDL 前端）。所以「畫面是原版的」不能靠
執行環境的身分來擔保，要靠畫面自己。這支工具就是那道閘門：把截圖跟
`tools/decode_atari_st_images.py` 從磁碟 raw offset `0xd800` 解出來的標題影像
逐像素比對，全等才算那條路徑可信。

`--source` 選路徑，因為兩邊的幾何與色階都不一樣：Hatari 的截圖是 832×552、ST 畫面在
`(96,58)` 起 2 倍放大、色階是 `(2v)*17`；port 的視窗是 320×200 的 4 倍、色階是 `v*255/7`。

比對前要先把兩邊的顏色收回 Atari ST 的 3 位元色階。同一個色盤索引，Hatari 展開成
`(2v)*17`、解碼工具展開成 `v*255/7`，於是 5 在一邊是 170、在另一邊是 182。
那是兩種 8 位元寫法，不是兩種顏色。

預設只比左下那一塊，因為畫面上還疊著遊戲自己畫的 SUNDOG 標誌與開場對話框；
它們蓋掉的區域無從比對，不是差異。要比別塊就用 `--region`。
"""

from __future__ import annotations

import argparse
import pathlib
import sys

from PIL import Image

from compare_appimage_to_original import COCKPIT_DYNAMIC_REGIONS

ST_SIZE = (320, 200)

# 每條實跑路徑的截圖幾何與色階。origin／scale 描述 ST 畫面在截圖裡的位置與放大倍率；
# quantize 是那一側把 3 位元色階展開成 8 位元的方式。
SOURCES = {
    "hatari": {"origin": (96, 58), "scale": 2, "quantize": "hatari"},
    "port": {"origin": (0, 0), "scale": 4, "quantize": "decoder"},
}

# 未被 SUNDOG 標誌與開場對話框覆蓋的區塊（ST 座標，右下不含）。
DEFAULT_REGION = (0, 65, 140, 199)

# 有些畫面的底圖只畫靜態框架，其餘由即時狀態填。這裡的矩形要排除掉才比得出東西。
# cockpit 那組直接沿用 compare_appimage_to_original 的定義，不另留一份。
DYNAMIC_REGIONS = {"cockpit": COCKPIT_DYNAMIC_REGIONS}


def quantize_hatari(rgb: tuple[int, int, int]) -> tuple[int, int, int]:
    """Hatari 的 ST 色：3 位元值 v 展開成 (2v)*17。"""
    return tuple(int(round(c / 17.0 / 2.0)) for c in rgb)


def quantize_decoder(rgb: tuple[int, int, int]) -> tuple[int, int, int]:
    """decode_atari_st_images.py 的色：3 位元值 v 展開成 v*255/7。"""
    return tuple(int(round(c * 7.0 / 255.0)) for c in rgb)


def st_frame(shot: Image.Image, source: str = "hatari") -> Image.Image:
    """從截圖取出 ST 畫面，還原成 320×200。"""
    spec = SOURCES[source]
    x, y = spec["origin"]
    w, h = ST_SIZE
    box = (x, y, x + w * spec["scale"], y + h * spec["scale"])
    if box[2] > shot.width or box[3] > shot.height:
        raise ValueError(f"截圖 {shot.size} 放不下 ST 畫面 {box}")
    return shot.convert("RGB").crop(box).resize(ST_SIZE, Image.NEAREST)


def compare(
    shot: Image.Image,
    reference: Image.Image,
    region: tuple[int, int, int, int],
    source: str = "hatari",
    exclude: tuple[tuple[int, int, int, int], ...] = (),
) -> tuple[int, int, list[tuple[int, int, tuple, tuple]]]:
    """回傳（相同數, 總數, 前幾個相異點）。`exclude` 裡的矩形不列入計算。"""
    frame = st_frame(shot, source)
    quantize_left = quantize_hatari if SOURCES[source]["quantize"] == "hatari" else quantize_decoder
    ref = reference.convert("RGB")
    if ref.size != ST_SIZE:
        raise ValueError(f"參考影像應為 {ST_SIZE}，得到 {ref.size}")
    a = frame.load()
    b = ref.load()
    x0, y0, x1, y1 = region
    same = total = 0
    diffs: list[tuple[int, int, tuple, tuple]] = []
    for y in range(y0, y1):
        for x in range(x0, x1):
            if any(ex0 <= x < ex1 and ey0 <= y < ey1 for ex0, ey0, ex1, ey1 in exclude):
                continue
            total += 1
            left = quantize_left(a[x, y])
            right = quantize_decoder(b[x, y])
            if left == right:
                same += 1
            elif len(diffs) < 8:
                diffs.append((x, y, left, right))
    return same, total, diffs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("shot", type=pathlib.Path, help="實跑截圖（Hatari 832×552 或 port 1280×800）")
    parser.add_argument(
        "--source",
        choices=sorted(SOURCES),
        default="hatari",
        help="截圖來自哪條實跑路徑（決定幾何與色階）",
    )
    parser.add_argument(
        "reference", type=pathlib.Path, help="磁碟解出的標題影像（320×200 PNG）"
    )
    parser.add_argument(
        "--dynamic",
        choices=sorted(DYNAMIC_REGIONS),
        help="排除該畫面由即時狀態填的矩形（底圖只有靜態框架時要用）",
    )
    parser.add_argument(
        "--region",
        default=",".join(str(v) for v in DEFAULT_REGION),
        help="ST 座標的比對區塊 x0,y0,x1,y1（預設是未被覆蓋的左下角）",
    )
    args = parser.parse_args(argv)

    region = tuple(int(v) for v in args.region.split(","))
    if len(region) != 4:
        parser.error("--region 要四個整數：x0,y0,x1,y1")

    shot = Image.open(args.shot)
    reference = Image.open(args.reference)
    exclude = DYNAMIC_REGIONS[args.dynamic] if args.dynamic else ()
    same, total, diffs = compare(shot, reference, region, args.source, exclude)

    label = f"比對區塊 {region}（來源 {args.source}"
    label += f"，排除 {args.dynamic} 動態區）" if args.dynamic else "）"
    print(f"{label}：相同 {same}/{total}")
    if same == total:
        print("PASS：實跑畫面與磁碟解出的影像逐像素相同")
        return 0
    print(f"FAIL：{total - same} 個像素不同")
    for x, y, left, right in diffs:
        print(f"  ({x},{y}) 實跑={left} 解碼={right}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
