#!/usr/bin/env python3
"""把 Atari ST SunDog 的物品圖示條切開，並對回物品表。

輸入是唯讀的外層 Atari ST archive；工具只讀 nested raw ``.st``，輸出是含 raw offset、
來源雜湊與證據等級的 JSON。圖示像素只在明確指定 ``--output-dir`` 時寫成 PNG，
一律不入 repository。

## 這條 496×16 是什麼

image stream 的第 4 筆（raw ``0x11552``）宣告尺寸 496×16。496 = 31 × 16，也就是
**31 格 16×16**。物品表（raw ``0x05e00``，見 ``atari_st_item_data.py``）恰好 32 筆，
第 0 筆是 ``(nothing)``。31 = 32 − 1，圖示 *i* 對應物品碼 *i* + 1。

這個 +1 偏移不是靠數量湊出來的，是七個各自獨立的圖形錨點同時成立：

| 圖示 | 畫的是 | 物品碼 | 表上的名字 |
|---:|---|---:|---|
| 00 | 磁碟片 | 0x01 | ``(freeze/quit)`` |
| 01 | 垃圾桶 | 0x02 | ``(trash)`` |
| 03 | 手錶 | 0x04 | ``(time)`` |
| 04 | 錢袋 | 0x05 | ``(money)`` |
| 07 | 漢堡 | 0x08 | ``burger`` |
| 08 | 啤酒杯 | 0x09 | ``beer`` |
| 22 | 電阻 | 0x17 | ``shunt`` |

尾端九件船艦零件同樣逐一對上電子元件的畫法：``shunt`` 是電阻、``cryofuse`` 是保險絲、
``j-junc module`` 是二極體接面、``photon bridge`` 是橋式整流、``plasma tube`` 是真空管。
偏移若差一格，這些全部會錯開。

## 前七格不是物品

物品碼 0x01–0x07 在表上的名字都帶括號、數值都是 0：``(freeze/quit)``、``(trash)``、
``(next frame)``、``(time)``、``(money)``、``(location)``、``(dead part)``。它們佔著物品
編號的位置，但不是能拿在手上的東西——**同一條圖示條同時放了介面動作與物品**。

## 繪製端與點擊端（p-code）

圖示條不是固定畫在畫面某處的一排；它是 sprite sheet，逐格畫進浮動視窗：

- ``XSTARTUP:0x0f load_database`` 開機時 ``fetch_dpic(G7c, 4)`` 把 image 4（本條）解進
  ``GEMBIND_G536``；``XSTARTUP:0x03 init_gem`` 把 ``G7c`` 記錄填成
  {指標→G536, 總寬 16×31, 高 16, 格數 31, 1, image 4}。整份 p-code 以常數 4 呼叫
  ``fetch_dpic`` 的只有這一處。
- ``GEMBIND:0x20 draw_icon(idx,vrmode,x,y)``：來源 x = ``((idx and 31) − 1) × 16``，
  以 ``copy_opaque`` 貼到 ``(x−7..x+8, y−7..y+8)``——**「圖示 = 物品碼 − 1」直接寫在
  減 1 那條指令裡**，derived 升格 proven。
- ``WINDOWLI`` 維護一張至多 28 格的槽位表（每槽 3 words：y、x、物品碼）。
  ``put_items`` 逐槽 ``draw_icon(code, 6, slot.x+win.x, slot.y+win.y)``；
  ``find_item(x,y)`` 的點擊命中是 **|Δx| ≤ 6 且 |Δy| ≤ 6**。
- 槽位由 ``WINDOWLI:0x1d make_man`` 登記（角色視窗，浮動 zoom 視窗 160×100，原點
  ``cell×8+1``）：垃圾桶 (8,7)；地面模式加 (location)(49,57)、(time)(49,76)、
  (freeze/quit)(48,8)、(money)(49,38)；船上／對話模式在 (27,7) 放 (money) 或
  (next frame)；手槽 (24,29)；四個口袋 (27,57)、(9,60)、(9,77)、(27,77)，
  槽碼讀自 ``MAINLIB_G3f[0..3]``。
- 點下去進哪支程序在 ``XDOUSERM:0x01``：碼 1→``check_quit``、4→``show_time``、
  5→``show_cash``、6→``show_location``、>6→``desc_item``＋``move_item``。

以上每一條都以磁碟位元組簽章釘住（``PCODE_CONSUMERS``），閱讀以次級專案的
p-code 反組譯器輔助，結論錨在原始 bytes。

## 這支工具證明什麼、不證明什麼

- **proven**：raw span 的邊界、解壓後的像素、每一格的雜湊、31 格切法整除、
  圖示 *i* ↔ 物品碼 *i* + 1（``draw_icon`` 的減 1）、繪製端與點擊端的九條簽章。
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from atari_st_item_data import ITEM_TABLE_START, decode_table
from decode_atari_st_images import PALETTE, decode_record, load_disk, write_png


ICON_STRIP_RAW_OFFSET = 0x11552
ICON_SIZE = 16
# 物品表第 0 筆是 (nothing)：它佔一個編號，但沒有圖示。
FIRST_ICON_ITEM_CODE = 1
# 圖示的底色是 palette index 0。用它數「有畫東西的像素」，好認出空格。
BACKGROUND_INDEX = 0

# ── p-code 消費端簽章 ──────────────────────────────────────────────────────
# 繪製端與點擊端是逐條人讀反組譯釘出來的（閱讀以次級專案的 p-code 反組譯器輔助）。
# 每條簽章都必須在 SYSTEM.STARTUP 的磁碟範圍內恰好命中一次，否則這份文件的
# 依據就變了，工具要失敗而不是沿用舊結論。
SYSTEM_STARTUP_RANGE = (0x2E000, 0x2E000 + 106496)

PCODE_CONSUMERS: list[dict[str, str]] = [
    {
        "name": "strip.load",
        "procedure": "XSTARTUP:0x0f load_database",
        "signature": "9b 03 7c 04 73 4d",
        "semantics": "lae G7c; sldc4; fetch_dpic——開機把 image 4（本圖示條）解進 G7c 描述的緩衝。"
        "整份 p-code 以常數 4 呼叫 fetch_dpic 的只有這一處。",
    },
    {
        "name": "strip.tile-record",
        "procedure": "XSTARTUP:0x03 init_gem",
        "signature": "9b 03 85 36 9b 03 7c 72 05 10 1f 8c d9 03 7e 10 d9 03 7f 1f d9 03 80 80 01 d9 03 80 81 04 d9 03 80 82",
        "semantics": "G7c 記錄：指標→G536（像素緩衝）、總寬 16×31、高 16、格數 31、1、image 編號 4。",
    },
    {
        "name": "draw-icon.source-index",
        "procedure": "GEMBIND:0x20 draw_icon(idx,vrmode,x,y)",
        "signature": "24 1f a1 ee 10 8c 68 86 56 86 52 23 20 20 0f a2 00 0f 22 07 a3 22 08 a2 21 07 a3 21 08 a2 91 1c",
        "semantics": "來源 x = ((idx and 31) − 1) × 16，copy_opaque 16×16 到 (x−7..x+8, y−7..y+8)。"
        "「圖示 = 物品碼 − 1」就是這裡的 deci。",
    },
    {
        "name": "put-items.slot-loop",
        "procedure": "WINDOWLI:0x03 put_items",
        "signature": "24 7a 00 b2 f1 12 22 91 0b 24 7a 06 24 79 21 a2 24 78 20 a2 72 20",
        "semantics": "逐槽讀 3-word 槽位（y、x、物品碼）：碼 ≤ 0 畫 clear_slot／set_mark，"
        "否則 draw_icon(code, 6, slot.x+win.x, slot.y+win.y)。",
    },
    {
        "name": "find-item.hit-test",
        "procedure": "WINDOWLI:0x05 find_item(x,y,var code,var slot)",
        "signature": "25 21 79 a3 e0 07 b3 9f 24 21 78 a3 e0 07 b3 9f a1",
        "semantics": "點擊命中 = |Δx| ≤ 6 且 |Δy| ≤ 6（座標先扣掉視窗原點）。",
    },
    {
        "name": "make-man.icon-slots",
        "procedure": "WINDOWLI:0x1d make_zoom:make_man",
        "signature": "02 08 07 91 02 20 00 d2 28 06 80 31 80 39 91 02 04 80 31 80 4c 91 02 01 80 30 08 91 02 "
        "05 80 31 80 26 91 02 00 04 80 26 91 02 85 80 dc a5 02 8a 14 20 01 d2 07 05 1b 07 91 02 "
        "8a 09 20 02 d2 05 03 1b 07 91 02 00 18 1d 91 02",
        "semantics": "角色視窗的圖示槽：垃圾桶 (8,7)；地面模式加 (location)(49,57)、(time)(49,76)、"
        "(freeze/quit)(48,8)、(money)(49,38)；模式 1／2 在 (27,7) 放 (money)／(next frame)；"
        "手槽 (24,29)。",
    },
    {
        "name": "make-man.pockets",
        "procedure": "WINDOWLI:0x1d make_zoom:make_man",
        "signature": "00 1b 80 39 91 0a 01 09 80 3c 91 0a 02 09 80 4d 91 0a 03 1b 80 4d 91 0a",
        "semantics": "四個口袋槽 put_pocket(0..3)：(27,57)、(9,60)、(9,77)、(27,77)，"
        "槽碼讀自 MAINLIB_G3f[0..3]。",
    },
    {
        "name": "user-menu.click-dispatch",
        "procedure": "XDOUSERM:0x01 xdo_user_menu",
        "signature": "27 9a 05 02 d2 04 90 0d 8a 2f 28 01 d2 06 84 11 90 0f 8a 25 28 04 d2 04 90 0b 8a 1d "
        "28 05 d2 04 90 0c 8a 15 28 06 d2 04 90 0e 8a 0d 28 06 b2 f1 08 28 74 07 27 90 0a",
        "semantics": "find_item 命中後按物品碼分派：1→check_quit、4→show_time、5→show_cash、"
        "6→show_location、>6→desc_item＋move_item；屬性槽→show_stats。",
    },
    {
        "name": "make-zoom.window-geometry",
        "procedure": "WINDOWLI:0x15 make_zoom(mode,xcell,ycell)",
        "signature": "21 24 08 8c ed c4 21 e7 02 23 08 8c ed c4 25 0c b3 f1 24 21 e7 01 21 78 80 a0 a2 03 a3 c4 "
        "21 e7 03 21 7a 80 64 a2 03 a3 c4",
        "semantics": "浮動視窗原點 (xcell×8+1, ycell×8+1)；mode < 12 時右下角為原點加 0xa0−3／0x64−3"
        "（160×100 的視窗），否則 77×47 的小窗。",
    },
]


def locate_pcode_consumers(disk: bytes) -> list[dict[str, str]]:
    """把每條簽章釘到磁碟上：SYSTEM.STARTUP 範圍內恰好一次，多或少都失敗。"""

    low, high = SYSTEM_STARTUP_RANGE
    located: list[dict[str, str]] = []
    for rule in PCODE_CONSUMERS:
        needle = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        hits = []
        position = low - 1
        while True:
            position = disk.find(needle, position + 1, high)
            if position < 0:
                break
            hits.append(position)
        if len(hits) != 1:
            raise ValueError(
                f"pcode signature {rule['name']} hit {len(hits)} times in SYSTEM.STARTUP, expected 1"
            )
        located.append({**rule, "rawOffset": f"0x{hits[0]:05x}"})
    return located


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def slice_icons(pixels: bytes, width: int, height: int, size: int = ICON_SIZE) -> list[bytes]:
    """把一條 width×height 的像素帶切成 size×size 的格子。

    高度必須剛好等於格子邊長、寬度必須整除，否則這條就不是等寬圖示條，
    與其猜一個切法不如讓它失敗。
    """

    if size <= 0:
        raise ValueError(f"icon size must be positive, got {size}")
    if height != size:
        raise ValueError(f"icon strip is {height} tall, expected {size}")
    if width % size:
        raise ValueError(f"icon strip width {width} is not a multiple of {size}")
    if len(pixels) != width * height:
        raise ValueError(f"pixel buffer holds {len(pixels)} values, expected {width * height}")
    icons: list[bytes] = []
    for index in range(width // size):
        left = index * size
        cell = bytearray()
        for row in range(size):
            start = row * width + left
            cell.extend(pixels[start : start + size])
        icons.append(bytes(cell))
    return icons


def describe_icons(icons: list[bytes], item_records: list[dict[str, object]]) -> list[dict[str, object]]:
    """把每一格配上它的物品碼與表上的名字。

    物品表要比圖示多出恰好一筆——第 0 筆的 ``(nothing)``。多或少都表示這兩張表
    不是本工具假設的那一對，於是失敗而不是硬配。
    """

    expected = len(icons) + FIRST_ICON_ITEM_CODE
    if len(item_records) != expected:
        raise ValueError(
            f"{len(icons)} icons need {expected} item records to line up, got {len(item_records)}"
        )
    described: list[dict[str, object]] = []
    for index, cell in enumerate(icons):
        code = index + FIRST_ICON_ITEM_CODE
        record = item_records[code]
        name = str(record["name"])
        described.append(
            {
                "index": index,
                "itemCode": f"0x{code:02x}",
                "itemName": name,
                # 括號名是原始表自己的寫法，用它分出「不是物品的那幾格」。
                "isInterfaceEntry": name.startswith("(") and name.endswith(")"),
                "pixelSha256": sha256(cell),
                "inkPixels": sum(1 for value in cell if value != BACKGROUND_INDEX),
            }
        )
    return described


def build_document(archive_path: Path, variant: str) -> tuple[dict[str, object], list[bytes], dict[str, object]]:
    source, disk = load_disk(archive_path, variant)
    record, pixels = decode_record(disk, ICON_STRIP_RAW_OFFSET)
    icons = slice_icons(pixels, int(record["width"]), int(record["height"]))
    items = decode_table(disk, ITEM_TABLE_START, "items")
    described = describe_icons(icons, list(items["records"]))
    consumers = locate_pcode_consumers(disk)
    document = {
        "schema": 2,
        "tool": "tools/atari_st_item_icons.py",
        "inputs": {"archive": archive_path.name, "variant": variant, "source": source},
        "strip": {
            "rawOffset": f"0x{ICON_STRIP_RAW_OFFSET:x}",
            "width": record["width"],
            "height": record["height"],
            "iconSize": ICON_SIZE,
            "iconCount": len(icons),
            "recordSpanSha256": record["recordSpanSha256"],
            "pixelSha256": record["pixelSha256"],
        },
        "itemTable": {
            "rawOffset": items["rawOffset"],
            "recordCount": items["recordCount"],
            "sha256": items["sha256"],
        },
        "palette": [f"#{red:02x}{green:02x}{blue:02x}" for red, green, blue, _alpha in PALETTE],
        "icons": described,
        "pcodeConsumers": consumers,
        "evidence": {
            "proven": "raw span 邊界、解壓像素、每格雜湊、496 = 31 × 16 的整除切法；"
            "圖示 i 對應物品碼 i+1（GEMBIND:0x20 draw_icon 的減 1，加上磁碟片↔(freeze/quit)、"
            "垃圾桶↔(trash)、手錶↔(time)、錢袋↔(money)、漢堡↔burger、啤酒↔beer、電阻↔shunt "
            "七個圖形錨點）；繪製端與點擊端的九條 p-code 簽章（pcodeConsumers）",
        },
        "openQuestions": [
            "物品碼 0x0e 在表上是 (reserved)、數值 0，卻有一格畫好的圖示。它是被拿掉的物品還是備用格，未知。",
        ],
    }
    return document, icons, record


def write_icon_pngs(output_dir: Path, icons: list[bytes], described: list[dict[str, object]]) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    for cell, description in zip(icons, described):
        name = f"icon-{int(description['index']):02d}-{description['itemCode']}.png"
        write_png(output_dir / name, ICON_SIZE, ICON_SIZE, cell)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", default="[a2]", choices=["[a2]", "[a]"])
    parser.add_argument("--output-dir", type=Path, help="把每一格寫成 PNG 的目錄；不給就只輸出 JSON")
    arguments = parser.parse_args()

    document, icons, _record = build_document(arguments.archive, arguments.variant)
    if arguments.output_dir is not None:
        write_icon_pngs(arguments.output_dir, icons, list(document["icons"]))
    print(json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
