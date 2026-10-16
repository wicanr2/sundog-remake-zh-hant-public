#!/usr/bin/env python3
"""視窗表與選單表解碼的單元測試。用合成資料，不讀原始媒體。"""

from __future__ import annotations

import struct
import unittest

from atari_st_window_menu_tables import (
    MENU_COUNT,
    MENU_DIRECTORY_OFFSET,
    MENU_ITEM_COUNT,
    MENU_ITEMS_OFFSET,
    TABLES_RAW_OFFSET,
    WINDOW_COUNT,
    decode_menus,
    decode_windows,
)


def synthetic_disk() -> bytearray:
    disk = bytearray(0x5A000)
    for index in range(WINDOW_COUNT):
        offset = TABLES_RAW_OFFSET + index * 10
        disk[offset : offset + 10] = struct.pack(">5H", 0, index % 300, index % 300 + 10, 5, 15)
    # 17 個選單把 0..59 切成連續區段：前 16 個各 3 項、最後一個 12 項。
    start = 0
    for menu in range(MENU_COUNT):
        count = 3 if menu < MENU_COUNT - 1 else MENU_ITEM_COUNT - start
        value = (count << 8) | start
        offset = MENU_DIRECTORY_OFFSET + menu * 2
        disk[offset : offset + 2] = struct.pack(">H", value)
        start += count
    for item in range(MENU_ITEM_COUNT):
        offset = MENU_ITEMS_OFFSET + item * 4
        disk[offset : offset + 4] = struct.pack(">2H", (90 << 8) | 2, (50 << 8) | 40)
    return disk


class WindowTests(unittest.TestCase):
    def test_windows_decode(self) -> None:
        windows = decode_windows(bytes(synthetic_disk()))
        self.assertEqual(len(windows), WINDOW_COUNT)
        self.assertEqual(windows[0]["window"], 1)

    def test_an_inside_out_rect_is_refused(self) -> None:
        disk = synthetic_disk()
        disk[TABLES_RAW_OFFSET + 2 : TABLES_RAW_OFFSET + 6] = struct.pack(">2H", 50, 10)
        with self.assertRaises(ValueError):
            decode_windows(bytes(disk))

    def test_a_parent_past_the_table_is_refused(self) -> None:
        disk = synthetic_disk()
        disk[TABLES_RAW_OFFSET : TABLES_RAW_OFFSET + 2] = struct.pack(">H", WINDOW_COUNT + 1)
        with self.assertRaises(ValueError):
            decode_windows(bytes(disk))


class MenuTests(unittest.TestCase):
    def test_menus_decode_and_partition(self) -> None:
        directory, items = decode_menus(bytes(synthetic_disk()))
        self.assertEqual(len(directory), MENU_COUNT)
        self.assertEqual(len(items), MENU_ITEM_COUNT)
        self.assertEqual(sum(entry["count"] for entry in directory), MENU_ITEM_COUNT)

    def test_a_gap_in_the_partition_is_refused(self) -> None:
        # 第二個選單的起始項跳過一格：每個選單拿到的列會整批錯位，要當場失敗。
        disk = synthetic_disk()
        offset = MENU_DIRECTORY_OFFSET + 2
        disk[offset : offset + 2] = struct.pack(">H", (3 << 8) | 4)
        with self.assertRaises(ValueError):
            decode_menus(bytes(disk))

    def test_an_inside_out_item_rect_is_refused(self) -> None:
        disk = synthetic_disk()
        disk[MENU_ITEMS_OFFSET : MENU_ITEMS_OFFSET + 4] = struct.pack(">2H", (2 << 8) | 90, (50 << 8) | 40)
        with self.assertRaises(ValueError):
            decode_menus(bytes(disk))


if __name__ == "__main__":
    unittest.main()
