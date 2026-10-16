#!/usr/bin/env python3
"""解碼 Atari ST SunDog 的敵艦資料表。

這張表不在遊戲的資料區，而在**防拷軌**上。找到它的路徑是遊戲自己的 p-code：

1. 主程式 ``SUNDOG:0x01`` 一開機就 ``H_New(SHIPLIB_G6, 0x300)`` 配置 768 words，
   接著兩次 ``MAINLIB:0x49 moveblock``，block 參數是 **負數**（-8 與 -5）。
2. ``moveblock`` 對負的 block 走另一條路：``unitread(4, buf, 0, |block| - 1, 2)``。
   control 位元 2 是「實體定址」——不加磁碟的 track 偏移，直接讀該實體 block。
3. 因此讀的是實體 block 7（一次）與 4、5、6（三次，依序填進 G6 的 word 0、0x100、
   0x200）。實體 block 0–8 就是磁碟的 **track 3**，也就是那條每個 sector 都不照
   標準排列的防拷軌。先讀 block 7 再讀 block 4 正是防拷手法要求的 sector 順序。
4. 於是 ``SHIPLIB_G6 + 0x100 words``（即 G6 的 byte 0x200）落在實體 block 5 的開頭，
   raw 檔案位移 ``(3 * 9 + 5) * 512 = 0x4000``。戰鬥程式碼對敵艦資料的每一次存取都是
   從這個位址起算的。

版面由四段接在一起，**恰好填滿 1,024 bytes**（實體 block 5 與 6），沒有空隙也沒有溢出：

===========  ======  ==============================================================
region       bytes   來源讀取點
===========  ======  ==============================================================
0x000–0x01b      28  7 筆 4-byte 屬性 record；``ixa 0x2`` + ``ldb`` 取 byte 0–3
0x01c–0x05b      64  8 列 × 8 byte；``XLANDFX:0x08`` 以 ``inc 0x10e`` + ``ixa 0x4`` 取用
0x05c–0x0af      84  7 個 12-byte 名稱欄位；``XDOFIGHT:0x03`` 以 ``inc 0x12e`` + ``ixa 0x6`` 取用
0x0b0–0x3ff     848  7 組敵艦外形折線；``XDOFIGHT:0x03`` 對每個型號各給一個位移與長度
===========  ======  ==============================================================

四段長度相加正好是 1,024，而且七組折線的每一段結尾都恰好等於下一段的起點——這是**整體
覆蓋檢查**，不是單點比對。任何一段的邊界記錯，總長就對不上。

屬性 record 的四個 byte 的語意是人讀 p-code 讀出來的，各自的讀取點記在
``docs/atari-st/ship-rules.md``；本工具把數字從磁碟解出來，並檢查它們落在那些讀取點
用得起的範圍內（例如當除數的 byte 不得為 0，當倒數步進的 byte 不得為 0）。

輸出是含 raw offset、來源雜湊與證據等級的 JSON manifest；不把磁碟 bytes 原樣倒出，
只輸出解出來的結構。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import zipfile
from pathlib import Path

from atari_st_fat_inventory import choose_disk, choose_nested


BLOCK_SIZE = 512
SECTORS_PER_TRACK = 9
# 防拷軌是磁碟的 track 3；SUNDOG:0x01 用實體定址讀它的 block 4、5、6。
PROTECTED_TRACK = 3
PROTECTED_TRACK_OFFSET = PROTECTED_TRACK * SECTORS_PER_TRACK * BLOCK_SIZE
# G6 + 0x100 words 對到這三個 block 的第二個。
TABLE_FIRST_BLOCK = 5
TABLE_BLOCK_COUNT = 2
TABLE_OFFSET = PROTECTED_TRACK_OFFSET + TABLE_FIRST_BLOCK * BLOCK_SIZE
TABLE_SIZE = TABLE_BLOCK_COUNT * BLOCK_SIZE

SHIP_COUNT = 7

STAT_RECORD_SIZE = 4
STAT_TABLE_OFFSET = 0x000
STAT_TABLE_SIZE = SHIP_COUNT * STAT_RECORD_SIZE

# XLANDFX:0x08 以 ixa 0x4（4 words = 8 bytes）取一列，再以 ldb 取 index 0..6。
BAR_ROW_SIZE = 8
BAR_ROW_COUNT = 8
BAR_TABLE_OFFSET = STAT_TABLE_OFFSET + STAT_TABLE_SIZE
BAR_TABLE_SIZE = BAR_ROW_COUNT * BAR_ROW_SIZE
BAR_COLUMNS = 7
BAR_MAX = 6

# XDOFIGHT:0x03 以 ixa 0x6（6 words = 12 bytes）取一個名稱欄位。
NAME_FIELD_SIZE = 12
NAME_MAX_LENGTH = NAME_FIELD_SIZE - 1
NAME_TABLE_OFFSET = BAR_TABLE_OFFSET + BAR_TABLE_SIZE
NAME_TABLE_SIZE = SHIP_COUNT * NAME_FIELD_SIZE

OUTLINE_TABLE_OFFSET = NAME_TABLE_OFFSET + NAME_TABLE_SIZE

# XDOFIGHT:0x03 對每個型號呼叫一次 MAINLIB:0x40 draw_poly，兩個常數寫死在呼叫點：
# 折線資料的 word 位移（相對 SHIPLIB_G6）與迴圈的上界。draw_poly 的迴圈是
# `L1 = 0; while L1 < bound: 取 byte L1 與 L1+1`，所以真正讀到的是 bound + 1 個 byte。
OUTLINE_CALL_SITES: tuple[tuple[int, int], ...] = (
    (0x158, 0x67),
    (0x18C, 0x79),
    (0x1C9, 0x9D),
    (0x218, 0x65),
    (0x24B, 0x71),
    (0x284, 0x93),
    (0x2CE, 0x63),
)
# draw_poly 讀的 byte 位址是「G6 的 word 位移 × 2」，而表格本身從 G6 的 byte 0x200 起算。
OUTLINE_WORD_BASE = 0x100

# 屬性 record 的四個 byte，語意由讀取點決定。
STAT_FIELDS: tuple[dict[str, object], ...] = (
    {
        "byte": 0,
        "key": "weaponPower",
        "reader": "SHIPLIB:0x21 update_attack @ 0x0de9",
        "summary": "單發傷害的基數：`(值 + 100) × 比例項 / (距離/3 + rand(25,50))`，再夾限 20–300。比例項由敵艦**當前**血量算出（`min((血量/20 + 4)², 90)`），與距離無關；公式裡真正吃距離的只有分母。見 ship-rules.json 的 combat.damage-scale。",
        "minimum": 0,
    },
    {
        "byte": 1,
        "key": "armour",
        "reader": "XDOFIGHT:0x06 @ 0x0289",
        "summary": "玩家那一發的除數：算好的傷害除以這個值，再夾限 1–80。值越大越耐打。",
        "minimum": 1,
    },
    {
        "byte": 2,
        "key": "accuracy",
        "reader": "SHIPLIB:0x21 update_attack @ 0x0da3",
        "summary": "命中判定：`(值 + 100) / (距離/8) > rand(1,100)` 才算打中。",
        "minimum": 0,
    },
    {
        "byte": 3,
        "key": "fireRate",
        "reader": "SHIPLIB:0x20 set_fire @ 0x0c55",
        "summary": "開火倒數的步進：倒數從 rand(5,10) 起跳，每次扣掉這個值，歸零就開火。",
        "minimum": 1,
    },
)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_disk(archive_path: Path, variant: str) -> tuple[str, bytes]:
    archive_bytes = archive_path.read_bytes()
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as outer:
        _, nested_bytes = choose_nested(outer, variant)
        with zipfile.ZipFile(io.BytesIO(nested_bytes)) as nested:
            disk_name, disk_bytes = choose_disk(nested)
    return disk_name, disk_bytes


def decode_stats(region: bytes) -> list[dict[str, object]]:
    """讀出七筆屬性 record，並檢查每個 byte 落在它的讀取點用得起的範圍。"""
    ships: list[dict[str, object]] = []
    for index in range(SHIP_COUNT):
        start = STAT_TABLE_OFFSET + index * STAT_RECORD_SIZE
        record = region[start : start + STAT_RECORD_SIZE]
        entry: dict[str, object] = {"index": index}
        for field in STAT_FIELDS:
            value = record[int(field["byte"])]
            minimum = int(field["minimum"])
            if value < minimum:
                raise ValueError(
                    f"ship {index} field {field['key']} is {value}, below the minimum {minimum} "
                    f"its reader ({field['reader']}) can use"
                )
            entry[str(field["key"])] = value
        ships.append(entry)
    return ships


def decode_names(region: bytes) -> list[str]:
    """讀出七個長度前綴的名稱欄位；長度、可列印性與填補零都要合法。"""
    names: list[str] = []
    for index in range(SHIP_COUNT):
        start = NAME_TABLE_OFFSET + index * NAME_FIELD_SIZE
        field = region[start : start + NAME_FIELD_SIZE]
        length = field[0]
        if length == 0 or length > NAME_MAX_LENGTH:
            raise ValueError(f"ship {index} name length {length} does not fit a {NAME_FIELD_SIZE}-byte field")
        body = field[1 : 1 + length]
        if any(byte < 0x20 or byte > 0x7E for byte in body):
            raise ValueError(f"ship {index} name contains a byte outside printable ASCII")
        padding = field[1 + length :]
        if any(padding):
            raise ValueError(f"ship {index} name field has non-zero padding {padding.hex()}")
        names.append(body.decode("ascii"))
    return names


def decode_bar_rows(region: bytes) -> list[list[int]]:
    """讀出八列長度值。每列七個值加一個 0 結尾，值域是 1..6。"""
    rows: list[list[int]] = []
    for index in range(BAR_ROW_COUNT):
        start = BAR_TABLE_OFFSET + index * BAR_ROW_SIZE
        row = list(region[start : start + BAR_ROW_SIZE])
        if row[BAR_COLUMNS] != 0:
            raise ValueError(f"bar row {index} does not end in a zero terminator")
        for column, value in enumerate(row[:BAR_COLUMNS]):
            if value < 1 or value > BAR_MAX:
                raise ValueError(f"bar row {index} column {column} is {value}, outside 1..{BAR_MAX}")
        rows.append(row[:BAR_COLUMNS])
    return rows


def decode_outlines(region: bytes) -> list[dict[str, object]]:
    """讀出七組折線，並檢查它們首尾相接、恰好用完剩下的位元組。"""
    outlines: list[dict[str, object]] = []
    expected = OUTLINE_TABLE_OFFSET
    for index, (word_offset, bound) in enumerate(OUTLINE_CALL_SITES):
        start = (word_offset - OUTLINE_WORD_BASE) * 2
        if start != expected:
            raise ValueError(
                f"outline {index} starts at 0x{start:03x} but the previous one ends at 0x{expected:03x}"
            )
        span = bound + 1
        points = [
            [region[start + step], region[start + step + 1]]
            for step in range(0, span - 1, 2)
        ]
        outlines.append({"index": index, "offset": f"0x{start:03x}", "bytes": span, "points": points})
        expected = start + span
    if expected != TABLE_SIZE:
        raise ValueError(f"the outlines end at 0x{expected:03x}, not at the end of the region 0x{TABLE_SIZE:03x}")
    return outlines


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    disk_name, disk = load_disk(archive_path, variant)
    if len(disk) < TABLE_OFFSET + TABLE_SIZE:
        raise ValueError("the disk is too small to hold the protected track")
    region = disk[TABLE_OFFSET : TABLE_OFFSET + TABLE_SIZE]

    stats = decode_stats(region)
    names = decode_names(region)
    bar_rows = decode_bar_rows(region)
    outlines = decode_outlines(region)

    covered = STAT_TABLE_SIZE + BAR_TABLE_SIZE + NAME_TABLE_SIZE + sum(int(o["bytes"]) for o in outlines)
    if covered != TABLE_SIZE:
        raise ValueError(f"the four regions cover {covered} bytes, not the whole {TABLE_SIZE}")

    ships = []
    for entry, name in zip(stats, names):
        ships.append({**entry, "name": name})

    return {
        "schema": 1,
        "tool": "tools/atari_st_enemy_ships.py",
        "inputs": {
            "archive": {"name": archive_path.name, "sha256": sha256(archive_path.read_bytes())},
            "disk": {"name": disk_name, "size": len(disk), "sha256": sha256(disk)},
            "variant": variant,
        },
        "region": {
            "rawOffset": f"0x{TABLE_OFFSET:05x}",
            "size": TABLE_SIZE,
            "track": PROTECTED_TRACK,
            "physicalBlocks": [TABLE_FIRST_BLOCK, TABLE_FIRST_BLOCK + 1],
            "runtimeAddress": "SHIPLIB_G6 + 0x100 words",
            "howItWasFound": (
                "SUNDOG:0x01 配置 0x300 words 給 SHIPLIB_G6，再以負的 block 參數呼叫 "
                "MAINLIB:0x49 moveblock；負值走 unitread(4, buf, 0, |block| - 1, 2) 這條，"
                "control 位元 2 是實體定址，讀到的就是防拷軌 track 3 的實體 block 4、5、6。"
            ),
        },
        "coverage": {
            "statTable": {"offset": f"0x{STAT_TABLE_OFFSET:03x}", "bytes": STAT_TABLE_SIZE},
            "barTable": {"offset": f"0x{BAR_TABLE_OFFSET:03x}", "bytes": BAR_TABLE_SIZE},
            "nameTable": {"offset": f"0x{NAME_TABLE_OFFSET:03x}", "bytes": NAME_TABLE_SIZE},
            "outlineTable": {
                "offset": f"0x{OUTLINE_TABLE_OFFSET:03x}",
                "bytes": sum(int(o["bytes"]) for o in outlines),
            },
            "total": covered,
        },
        "statFields": [
            {
                "byte": field["byte"],
                "key": field["key"],
                "reader": field["reader"],
                "summary": field["summary"],
            }
            for field in STAT_FIELDS
        ],
        "ships": ships,
        "outlines": outlines,
        "barRows": bar_rows,
        "evidence": "proven（自原版 p-code 找到位置，欄位邊界由整體覆蓋檢查確立）",
        "semanticsEvidence": "proven（四個 byte 的語意自 p-code 的讀取點靜態讀出，未經執行對拍）",
        "whatThisProves": (
            "敵艦的名稱、四項屬性與外形折線都在防拷軌 track 3 的實體 block 5 與 6 上，"
            "四段結構首尾相接、恰好填滿 1,024 bytes。"
        ),
        "openQuestions": [
            {
                "topic": "bar rows",
                "status": "strong inference",
                "detail": (
                    "0x01c 起那 8 列只有一個讀取點：XLANDFX:0x08 拿它當垂直線的長度，"
                    "一次畫 6 組各 7 條。用途讀起來是降落畫面的地面輪廓，但沒有第二個證據。"
                ),
            },
            {
                "topic": "enemy hull",
                "status": "proven",
                "detail": (
                    "敵艦血量不在這張表裡：SHIPLIB:0x1f 以 rand(0x78 + 型號 × 20, 0xff) 現場擲出，"
                    "存進 MAINLIB_Gda。"
                ),
            },
        ],
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
