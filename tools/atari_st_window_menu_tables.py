#!/usr/bin/env python3
"""解出原版的視窗表與選單表（raw ``0x6000`` 起的三個 logical block）。

原版整個畫面系統的幾何**是資料不是程式**：`XSTARTUP:0x0f load_database` 以一個
moveblock 把 logical block 0x0c 起的三個 block 載進 `MAINLIB_G773`，裡面依序是——

1. **視窗表**：122 筆、每筆 5 words（父視窗、x0、x1、y0、y1，big-endian）。
   `MAINLIB:0x36 screen_pos` 以「加上父視窗原點」遞迴解出絕對矩形；
   `make_zoom` 動態改寫其中一格當浮動視窗。get_building 給室內邏輯的
   「子視窗編號」（25–43 一帶，父視窗都是 3）就住在這裡。
2. **選單目錄**：每 word 拆（低 8 位 = 起始項、高 8 位 = 項數），17 個選單。
3. **選單項目表**：60 筆、每筆 2 words——**每項一個矩形**（w0 = x1:x0、
   w1 = y1:y0，各是兩個 byte）。`put_menu` 逐項畫盒（`put_box`）或畫字
   （`draw_char`），文字由呼叫端在執行時以 `set_box`／`add_char` 放進去；
   `find_menu` 拿同一批矩形做點擊命中。

三個消費端的位元組簽章一併釘住（``pcodeConsumers``）。表本身要通過結構驗證
才輸出：視窗座標要在 320×200 內且 x0 ≤ x1、選單目錄要把 0–59 切成連續不重疊
的區段，錯一筆就失敗——沿用既有工具的 fail-closed 慣例。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
from pathlib import Path

from atari_st_item_data import load_disk


TABLES_RAW_OFFSET = 0x6000
WINDOW_COUNT = 122
WINDOW_RECORD_WORDS = 5
MENU_DIRECTORY_WORDS = 20
MENU_COUNT = 17
MENU_ITEM_COUNT = 60

MENU_DIRECTORY_OFFSET = TABLES_RAW_OFFSET + WINDOW_COUNT * WINDOW_RECORD_WORDS * 2
MENU_ITEMS_OFFSET = MENU_DIRECTORY_OFFSET + MENU_DIRECTORY_WORDS * 2

SYSTEM_STARTUP_RANGE = (0x2E000, 0x2E000 + 106496)

PCODE_CONSUMERS: list[dict[str, str]] = [
    {
        "name": "tables.load",
        "procedure": "XSTARTUP:0x0f load_database",
        "signature": "01 81 61 02 00 9b 04 87 73 03 0c 01 73 49",
        "semantics": "moveblock 把 logical block 0x0c 起的三個 block（raw 0x6000）載進 "
        "MAINLIB_G773——視窗表、選單目錄與選單項目表一次進來。",
    },
    {
        "name": "menu.new-menu",
        "procedure": "WINDOWLI:0x13 new_menu(id)",
        "signature": "9b 04 89 d5 24 d7 01 6b 23 08 00 c9 6a 22 23 08 08 c9 a2 68 01 69 "
        "86 80 88 21 ee d7 04 6b 23 01 08 01 ca 23 e7 02 9b 04 89 e9 22 d7 02 c5 00 02",
        "semantics": "查目錄（G9d5[id]：低 8 位起始項、高 8 位項數），把每項的兩個 word "
        "從 G9e9 複製進工作陣列 G88。",
    },
    {
        "name": "menu.put-menu-draw",
        "procedure": "WINDOWLI:0x0f put_menu",
        "signature": "86 80 88 22 ee d7 04 01 08 c9 d4 07 22 21 20 91 19 8a 15 86 80 88 "
        "22 ee d7 04 6c 24 79 02 21 24 7b a2 20 24 7a a2 72 21",
        "semantics": "逐項畫：盒項走 put_box、字元項走 draw_char，座標一律加上視窗原點。"
        "文字不在表裡——由呼叫端以 set_box／add_char 在執行時放。",
    },
]


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def decode_windows(disk: bytes) -> list[dict[str, int]]:
    windows: list[dict[str, int]] = []
    for index in range(WINDOW_COUNT):
        offset = TABLES_RAW_OFFSET + index * WINDOW_RECORD_WORDS * 2
        parent, x0, x1, y0, y1 = struct.unpack(">5H", disk[offset : offset + 10])
        if parent > WINDOW_COUNT:
            raise ValueError(f"window {index + 1} names parent {parent}, past the table")
        if not (x0 <= x1 <= 320 and y0 <= y1 <= 200):
            raise ValueError(f"window {index + 1} rect ({x0},{x1},{y0},{y1}) is not a screen rect")
        windows.append({
            "window": index + 1,
            "parent": parent,
            "x0": x0,
            "x1": x1,
            "y0": y0,
            "y1": y1,
        })
    return windows


def decode_menus(disk: bytes) -> tuple[list[dict[str, int]], list[dict[str, int]]]:
    directory: list[dict[str, int]] = []
    for index in range(MENU_DIRECTORY_WORDS):
        value = struct.unpack(">H", disk[MENU_DIRECTORY_OFFSET + index * 2 :][:2])[0]
        if value == 0:
            continue
        directory.append({"menu": index, "start": value & 0xFF, "count": value >> 8})
    if len(directory) != MENU_COUNT:
        raise ValueError(f"{len(directory)} menus in the directory, want {MENU_COUNT}")
    # 目錄必須把 0..59 切成連續不重疊的區段——切法錯了，每個選單拿到的就是別人的列。
    expected_start = 0
    for entry in directory:
        if entry["start"] != expected_start:
            raise ValueError(
                f"menu {entry['menu']} starts at {entry['start']}, want {expected_start}"
            )
        expected_start += entry["count"]
    if expected_start != MENU_ITEM_COUNT:
        raise ValueError(f"menus cover {expected_start} items, want {MENU_ITEM_COUNT}")

    items: list[dict[str, int]] = []
    for index in range(MENU_ITEM_COUNT):
        w0, w1 = struct.unpack(">2H", disk[MENU_ITEMS_OFFSET + index * 4 :][:4])
        x0, x1 = w0 & 0xFF, w0 >> 8
        y0, y1 = w1 & 0xFF, w1 >> 8
        if not (x0 <= x1 and y0 <= y1):
            raise ValueError(f"menu item {index} rect ({x0},{x1},{y0},{y1}) is inside out")
        items.append({"item": index, "x0": x0, "x1": x1, "y0": y0, "y1": y1})
    return directory, items


def locate_consumers(disk: bytes) -> list[dict[str, str]]:
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
            raise ValueError(f"signature {rule['name']} hit {len(hits)} times, expected 1")
        located.append({**rule, "rawOffset": f"0x{hits[0]:05x}"})
    return located


def build_document(archive_path: Path, variant: str) -> dict[str, object]:
    nested_name, disk_name, disk = load_disk(archive_path, variant)
    windows = decode_windows(disk)
    directory, items = decode_menus(disk)
    span = disk[TABLES_RAW_OFFSET : MENU_ITEMS_OFFSET + MENU_ITEM_COUNT * 4]
    return {
        "schema": 1,
        "tool": "tools/atari_st_window_menu_tables.py",
        "inputs": {"archive": archive_path.name, "variant": variant, "nestedArchive": nested_name, "disk": disk_name},
        "tables": {
            "rawOffset": f"0x{TABLES_RAW_OFFSET:05x}",
            "byteLength": len(span),
            "sha256": sha256(span),
        },
        "windowTable": {
            "rawOffset": f"0x{TABLES_RAW_OFFSET:05x}",
            "recordWords": WINDOW_RECORD_WORDS,
            "count": WINDOW_COUNT,
            "windows": windows,
        },
        "menuDirectory": {
            "rawOffset": f"0x{MENU_DIRECTORY_OFFSET:05x}",
            "menus": directory,
        },
        "menuItems": {
            "rawOffset": f"0x{MENU_ITEMS_OFFSET:05x}",
            "count": MENU_ITEM_COUNT,
            "items": items,
        },
        "pcodeConsumers": locate_consumers(disk),
        "evidence": {
            "proven": "表的位置、122 筆視窗記錄與 17 選單 60 項的結構驗證、三個消費端簽章的"
            "唯一命中；視窗 6 的內容區（96–256, 6–117）與 original-layout.json 量出的視埠"
            "互相印證",
            "derived": "欄位語意（父視窗與四邊）讀自 screen_pos／set_window／find_menu 的"
            "存取樣式；選單項目的矩形語意讀自 put_menu 與 find_menu",
        },
        "menuCallers": {
            "0": ["XSTARTUP:0x46 sign_on", "XDOTRADI:0x0f pick_item（倉庫四格）"],
            "1": ["XSTARTUP:0x14 utility:util_menu"],
            "2": ["XSTARTUP:0x16 verified／0x3b check_done／0x42 vquit／0x49 end_game", "XDOTRADI:0x1d do_buying（Buy／Pass）、0x19 goods_info（Exit／Deliver／Accept 列）"],
            "3": ["XSTARTUP:0x08 request_sundog／0x0d show_score／0x16／0x17／0x1b／0x49"],
            "4": ["XPILOTAG:0x02／0x07／0x0a 與 XDOFIGHT:0x0e／0x10 的駕駛台選單"],
            "5": ["XDOUSERM:0x07 init_status／0x0f check_quit（角色卡）"],
            "6": ["XDOUNITE:0x0f do_depot、0x0b do_teller、0x09 do_tube_station、0x0e pay_fine、0x17 engineering、0x05 pick_location", "XMOVEONG:0x2b handle_teleport"],
            "7": ["XDOUNITE:0x0c do_teller:move_cash、0x10 do_depot:select_purchase"],
            "8": ["XDOUNITE:0x05 pick_location、0x10 select_purchase"],
            "9": ["XDOUNITE:0x05 pick_location"],
            "10": ["XDOUNITE:0x05 pick_location"],
            "11": ["XDOTRADI:0x0b do_row（交易所主視窗 Buy／Sell、Exchange／Warehouse、Delivery／Warehouse）"],
            "12": ["XDOTRADI:0x0f pick_item（貨艙兩格）"],
            "15": ["XDOTRADI:0x0b do_row（Exit center）"],
            "16": ["XSLOTS:0x09 get_button"],
        },
        "openQuestions": [
            "選單 13、14 沒有 new_menu 的直接常數呼叫端（do_row 的 style 參數由呼叫端決定，未列舉到）。",
            "122 格視窗有值的約半數；其餘全零格是給 make_zoom 動態用的還是保留，未逐格確認。",
        ],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--variant", default="[a2]", choices=["[a2]", "[a]"])
    arguments = parser.parse_args()
    document = build_document(arguments.archive, arguments.variant)
    print(json.dumps(document, ensure_ascii=False, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
