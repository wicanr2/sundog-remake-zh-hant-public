#!/usr/bin/env python3
"""物品圖示條切法與對表的單元測試。用合成資料，不讀原始媒體。"""

from __future__ import annotations

import unittest

from atari_st_item_icons import (
    FIRST_ICON_ITEM_CODE,
    PCODE_CONSUMERS,
    SYSTEM_STARTUP_RANGE,
    describe_icons,
    locate_pcode_consumers,
    slice_icons,
)


def strip(icon_count: int, size: int = 16) -> tuple[bytes, int, int]:
    """一條每格填同一個 index 的圖示帶：第 n 格全部填 n mod 16。"""

    width = icon_count * size
    pixels = bytearray()
    for _row in range(size):
        for index in range(icon_count):
            pixels.extend([index % 16] * size)
    return bytes(pixels), width, size


def item_records(count: int) -> list[dict[str, object]]:
    records: list[dict[str, object]] = [{"name": "(nothing)"}]
    records.extend({"name": f"item-{index}"} for index in range(1, count))
    return records


class SliceTests(unittest.TestCase):
    def test_each_cell_holds_its_own_column(self) -> None:
        pixels, width, height = strip(3)
        icons = slice_icons(pixels, width, height)
        self.assertEqual(len(icons), 3)
        for index, cell in enumerate(icons):
            self.assertEqual(set(cell), {index}, f"cell {index} picked up a neighbour")
            self.assertEqual(len(cell), 16 * 16)

    def test_a_strip_taller_than_one_row_of_icons_is_refused(self) -> None:
        # 32 高的帶子可能是兩排圖示，也可能根本不是圖示條。猜一個切法比失敗更糟。
        pixels, width, _height = strip(2)
        with self.assertRaises(ValueError):
            slice_icons(pixels * 2, width, 32)

    def test_a_width_that_does_not_divide_is_refused(self) -> None:
        pixels, width, height = strip(2)
        with self.assertRaises(ValueError):
            slice_icons(pixels, width + 1, height)

    def test_a_short_pixel_buffer_is_refused(self) -> None:
        pixels, width, height = strip(2)
        with self.assertRaises(ValueError):
            slice_icons(pixels[:-1], width, height)


class DescribeTests(unittest.TestCase):
    def test_the_first_icon_is_item_code_one(self) -> None:
        pixels, width, height = strip(4)
        icons = slice_icons(pixels, width, height)
        described = describe_icons(icons, item_records(5))
        self.assertEqual([entry["itemCode"] for entry in described],
                         ["0x01", "0x02", "0x03", "0x04"])
        self.assertEqual(FIRST_ICON_ITEM_CODE, 1)

    def test_a_bracketed_name_is_flagged_as_an_interface_entry(self) -> None:
        pixels, width, height = strip(2)
        icons = slice_icons(pixels, width, height)
        records = item_records(3)
        records[1]["name"] = "(trash)"
        described = describe_icons(icons, records)
        self.assertTrue(described[0]["isInterfaceEntry"])
        self.assertFalse(described[1]["isInterfaceEntry"])

    def test_an_item_table_that_does_not_line_up_is_refused(self) -> None:
        # 對不上就是這兩張表不是同一對，硬配出來的名字會是錯的。
        pixels, width, height = strip(4)
        icons = slice_icons(pixels, width, height)
        with self.assertRaises(ValueError):
            describe_icons(icons, item_records(4))
        with self.assertRaises(ValueError):
            describe_icons(icons, item_records(6))

    def test_ink_pixels_counts_everything_that_is_not_the_background(self) -> None:
        pixels, width, height = strip(2)
        icons = slice_icons(pixels, width, height)
        described = describe_icons(icons, item_records(3))
        # 第 0 格整格是 palette index 0，也就是底色：一個墨點都沒有。
        self.assertEqual(described[0]["inkPixels"], 0)
        self.assertEqual(described[1]["inkPixels"], 16 * 16)


def synthetic_disk() -> bytearray:
    """一張塞好九條簽章的合成磁碟：每條在 SYSTEM.STARTUP 範圍內出現一次。"""

    low, _high = SYSTEM_STARTUP_RANGE
    disk = bytearray(0x5A000)
    position = low + 0x100
    for rule in PCODE_CONSUMERS:
        needle = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        disk[position : position + len(needle)] = needle
        position += len(needle) + 0x40
    return disk


class PcodeConsumerTests(unittest.TestCase):
    def test_each_signature_is_located_once(self) -> None:
        located = locate_pcode_consumers(bytes(synthetic_disk()))
        self.assertEqual(len(located), len(PCODE_CONSUMERS))
        for entry in located:
            offset = int(str(entry["rawOffset"]), 16)
            self.assertGreaterEqual(offset, SYSTEM_STARTUP_RANGE[0])
            self.assertLess(offset, SYSTEM_STARTUP_RANGE[1])

    def test_a_missing_signature_is_refused(self) -> None:
        # 簽章找不到表示磁碟不是這份文件描述的那一張，沿用舊結論比失敗更糟。
        disk = synthetic_disk()
        first = bytes.fromhex(str(PCODE_CONSUMERS[0]["signature"]).replace(" ", ""))
        start = bytes(disk).find(first)
        disk[start] ^= 0xFF
        with self.assertRaises(ValueError):
            locate_pcode_consumers(bytes(disk))

    def test_a_duplicated_signature_is_refused(self) -> None:
        disk = synthetic_disk()
        first = bytes.fromhex(str(PCODE_CONSUMERS[0]["signature"]).replace(" ", ""))
        tail = SYSTEM_STARTUP_RANGE[1] - len(first) - 1
        disk[tail : tail + len(first)] = first
        with self.assertRaises(ValueError):
            locate_pcode_consumers(bytes(disk))

    def test_a_hit_outside_system_startup_does_not_count(self) -> None:
        # 範圍外的命中不算數：簽章釘的是 SYSTEM.STARTUP 的 p-code，不是整張磁碟。
        disk = synthetic_disk()
        first = bytes.fromhex(str(PCODE_CONSUMERS[0]["signature"]).replace(" ", ""))
        disk[0x100 : 0x100 + len(first)] = first
        located = locate_pcode_consumers(bytes(disk))
        self.assertEqual(len(located), len(PCODE_CONSUMERS))


if __name__ == "__main__":
    unittest.main()
