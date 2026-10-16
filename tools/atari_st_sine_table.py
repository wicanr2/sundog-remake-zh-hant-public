#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的四分之一正弦表。

系統內航行是極座標的：船與目的地各有一組（角度、半徑），畫到畫面上要先投影成直角座標。
負責投影的是 ``SHIPLIB:0x0c polar_to_rect``，而它唯一的資料來源是 ``MAINLIB_G726``
起的一張 byte 表。

表的位置是從遊戲自己的載入點讀出來的：``XSTARTUP:0x01`` 以 ``MAINLIB:0x49 moveblock``
把 **logical block 0x46** 讀進 ``MAINLIB_G726``。logical block 要加上磁碟的 track 偏移
（9 個 block）才是實體 block，而這張磁碟的實體 block 0 是檔案的 track 3，所以
raw 檔案位移是 ``(27 + 0x46 + 9) * 512 = 0xd400``。

``polar_to_rect`` 的取用方式決定了表的長度與意義：

```
i = (角度 / 2) mod 0x40        # 256 為一圈 → 半圈 128 → 取模成 64
if i > 0x20: i = 0x40 - i      # 折回第一象限
分量 = 半徑 * 表[i] / 0x80
```

``i`` 的值域因此是 0..0x20，表只需要 **33 個 byte**，代表 0° 到 90° 的正弦乘以 128。

這給了一個難得的**獨立驗證**：解出來的 33 個 byte 必須逐項等於 ``round(128 * sin(k*pi/64))``。
這不是自我一致的檢查，是拿磁碟上的位元組去對一個外部的數學事實；對不上就代表位址、
長度或折疊規則其中之一讀錯了。工具因此 fail-closed。

x 分量走同一張表，只是角度先加四分之一圈（``(角度 + 0x40) / 2``）——也就是餘弦。
兩個分量各自的正負號由角度落在哪一段決定，規則寫在 ``docs/atari-st/ship-rules.md``。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import math
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


BLOCK_SIZE = 512
SECTORS_PER_TRACK = 9
# 這張磁碟的實體 block 0 是檔案的 track 3；logical block 還要再加一個 track 的偏移。
FILE_TRACK_SHIFT = 3 * SECTORS_PER_TRACK
DISK_TRACK_OFFSET = SECTORS_PER_TRACK
# XSTARTUP:0x01 的 moveblock 呼叫點寫死了這個 block 編號。
SINE_TABLE_BLOCK = 0x46
SINE_TABLE_OFFSET = (FILE_TRACK_SHIFT + SINE_TABLE_BLOCK + DISK_TRACK_OFFSET) * BLOCK_SIZE

# polar_to_rect 折疊後的索引值域是 0..0x20，所以表是 33 個 byte。
QUARTER_STEPS = 0x20
SINE_TABLE_LENGTH = QUARTER_STEPS + 1
# 一圈 256 刻度，取用前先除以 2，所以整張表覆蓋的是 0..0x20 對 0°..90°。
TURN_STEPS = 256
# 分量算完要除以這個值，所以表的滿刻度就是它。
SINE_SCALE = 0x80

# ── 同一塊裡的其他表 ────────────────────────────────────────────────────────
# moveblock 一次讀整個 512-byte block 進 MAINLIB_G726..G825，正弦表只是開頭。
# 同塊還有（位移以 G726 為基準的 word 數 × 2）：
#   +0x22  滑鼠斜移表：16 個 word，XDOFIGHT:0x0b slew 以滑鼠距離除以 6 查表
#   +0x62  上箭頭折線：8 個 (x,y) byte 對，XSTARTUP 的 Library 選單以 draw_poly 畫
#   +0x72  下箭頭折線：8 對——上箭頭以 y' = 46 − y 鏡射，工具驗證這件事
#   +0x82  人形折線：12 對，WINDOWLI:0x1a draw_man 畫在角色狀態視窗
MOUSE_SLEW_OFFSET = SINE_TABLE_OFFSET + (0x737 - 0x726) * 2
MOUSE_SLEW_WORDS = 16
ARROW_UP_OFFSET = SINE_TABLE_OFFSET + (0x757 - 0x726) * 2
ARROW_DOWN_OFFSET = SINE_TABLE_OFFSET + (0x75F - 0x726) * 2
ARROW_POINTS = 8
MAN_FIGURE_OFFSET = SINE_TABLE_OFFSET + (0x767 - 0x726) * 2
MAN_FIGURE_POINTS = 12

PCODE_CONSUMERS: list[dict[str, str]] = [
    {
        "name": "man.draw",
        "procedure": "WINDOWLI:0x1a draw_man",
        "signature": "9b 04 09 9a 04 08 ee d7 04 78 68 01 80 40 01 01 73 3e 20 00 d2 09 04 80 6a "
        "01 01 73 3e 8a 07 0c 80 6b 01 01 73 3e 01 72 18 01 17 00 9b 04 87 67 9a 04 08 73 40 00 91 03",
        "semantics": "角色視窗的人形：先畫視窗 0x40 的外框（模式依視窗種類再畫 0x6a 或 0x6b），"
        "再以 draw_poly 畫 G767 的 12 點折線（bytes 0..0x17），最後 put_items 排圖示。",
    },
    {
        "name": "poly.loop",
        "procedure": "MAINLIB:0x40 draw_poly",
        "signature": "63 20 87 80 a8 a3 d7 01 22 87 80 a7 20 87 80 a8 a3 a7 a2 c4 63 20 ed 87 "
        "80 a8 a3 d7 01 21 87 80 a7 20 ed 87 80 a8 a3 a7 a2 c4 20 02 a2 68 8a c9",
        "semantics": "把 byte 表的 (x,y) 對逐一加上視窗原點後送 VDI polyline——"
        "折線表的每一對就是視窗內座標。",
    },
    {
        "name": "mouse.slew",
        "procedure": "XDOFIGHT:0x0b slew",
        "signature": "9b 04 87 37 3c 06 8d d7 01 78 a5 0d 9b 04 87 37 1f 3d 0a 8d a3 d7 01 78 a5 0e",
        "semantics": "戰術畫面的滑鼠斜移：x 距離除以 6、y 用 31 − 距離/10 查同一張 16-word 表，"
        "查出的值就是每拍的移動量。",
    },
]


def decode_block_extras(disk: bytes) -> dict[str, object]:
    slew = [
        int.from_bytes(disk[MOUSE_SLEW_OFFSET + i * 2 : MOUSE_SLEW_OFFSET + i * 2 + 2], "big")
        for i in range(MOUSE_SLEW_WORDS)
    ]
    if any(value > 16 for value in slew) or slew != sorted(slew, reverse=True):
        raise ValueError(f"mouse slew table is not a small descending ramp: {slew}")

    def points(offset: int, count: int) -> list[list[int]]:
        span = disk[offset : offset + count * 2]
        return [[span[i * 2], span[i * 2 + 1]] for i in range(count)]

    up = points(ARROW_UP_OFFSET, ARROW_POINTS)
    down = points(ARROW_DOWN_OFFSET, ARROW_POINTS)
    mirrored = [[x, 46 - y] for x, y in up]
    if down != mirrored:
        raise ValueError("the down arrow is not the up arrow mirrored at y = 46")
    man = points(MAN_FIGURE_OFFSET, MAN_FIGURE_POINTS)
    for x, y in man:
        if x > 30 or y > 60:
            raise ValueError(f"man figure point ({x},{y}) is outside a small window")
    return {
        "mouseSlew": {
            "rawOffset": f"0x{MOUSE_SLEW_OFFSET:05x}",
            "runtimeAddress": "MAINLIB_G737",
            "words": slew,
        },
        "arrowUp": {"rawOffset": f"0x{ARROW_UP_OFFSET:05x}", "runtimeAddress": "MAINLIB_G757", "points": up},
        "arrowDown": {"rawOffset": f"0x{ARROW_DOWN_OFFSET:05x}", "runtimeAddress": "MAINLIB_G75f", "points": down},
        "manFigure": {
            "rawOffset": f"0x{MAN_FIGURE_OFFSET:05x}",
            "runtimeAddress": "MAINLIB_G767",
            "points": man,
        },
    }


def locate_consumers(disk: bytes) -> list[dict[str, str]]:
    low, high = 0x2E000, 0x2E000 + 106496
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
            raise ValueError(f"signature {rule['name']} hit {len(hits)} times, expected 1")
        located.append({**rule, "rawOffset": f"0x{hits[0]:05x}"})
    return located


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def ideal_table() -> list[int]:
    """表應該長成的樣子，由數學算出，與磁碟無關。"""
    return [
        round(SINE_SCALE * math.sin(step * math.pi / (TURN_STEPS / 4)))
        for step in range(SINE_TABLE_LENGTH)
    ]


def decode_sine_table(disk: bytes) -> list[int]:
    if len(disk) < SINE_TABLE_OFFSET + SINE_TABLE_LENGTH:
        raise ValueError("the disk is too small to hold the sine table")
    table = list(disk[SINE_TABLE_OFFSET : SINE_TABLE_OFFSET + SINE_TABLE_LENGTH])
    expected = ideal_table()
    if table != expected:
        first = next(index for index, (a, b) in enumerate(zip(table, expected)) if a != b)
        raise ValueError(
            f"the bytes at 0x{SINE_TABLE_OFFSET:05x} are not a quarter sine table: "
            f"entry {first} is {table[first]}, not {expected[first]}"
        )
    return table


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    table = decode_sine_table(disk)

    return {
        "schema": 2,
        "tool": "tools/atari_st_sine_table.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "region": {
            "rawOffset": f"0x{SINE_TABLE_OFFSET:05x}",
            "logicalBlock": f"0x{SINE_TABLE_BLOCK:02x}",
            "length": SINE_TABLE_LENGTH,
            "runtimeAddress": "MAINLIB_G726",
            "loadedBy": "XSTARTUP:0x01 以 MAINLIB:0x49 moveblock 讀入",
            "readBy": "SHIPLIB:0x0c polar_to_rect",
        },
        "turnSteps": TURN_STEPS,
        "scale": SINE_SCALE,
        "table": table,
        "blockExtras": decode_block_extras(disk),
        "pcodeConsumers": locate_consumers(disk),
        "evidence": "proven（位置自 p-code 的載入點與取用點讀出）",
        "semanticsEvidence": (
            "proven（表的每一項逐一等於 round(128 * sin(k*pi/64))，"
            "這是磁碟位元組對外部數學事實的獨立比對，不是自我一致檢查）"
        ),
        "whatThisProves": (
            "polar_to_rect 用的是一張 33 個 byte、滿刻度 128 的四分之一正弦表，"
            "整圈以 256 刻度計。"
        ),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path, help="外層 Atari ST archive（唯讀）")
    parser.add_argument("--variant", default="[a2]", help="nested archive 變體標記")
    parser.add_argument("--output", type=Path, help="輸出 JSON 路徑；未指定則印到 stdout")
    args = parser.parse_args()

    document = build_document(args.archive, args.variant)
    text = json.dumps(document, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")


if __name__ == "__main__":
    main()
