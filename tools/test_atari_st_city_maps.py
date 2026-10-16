#!/usr/bin/env python3
"""不依賴原始媒體的城市地圖解碼器單元測試。

所有測試都在合成的 128-byte 地圖上跑，不讀取任何原版磁碟。合成器本身就是一份規格：
它照著解碼器宣稱的版面把標頭、建築位置與地形格接起來，測試再逐項把某一段弄壞。
"""

from __future__ import annotations

import unittest

from atari_st_city_maps import (
    BUILDING_ABSENT,
    BUILDING_FIRST_CODE,
    BUILDING_OFFSET,
    BUILDING_SLOTS,
    CITY_COUNT,
    CITY_MAPS_PER_BLOCK,
    CITY_MAP_FIRST_BLOCK,
    CITY_MAP_SIZE,
    GRID_COLS,
    GRID_OFFSET,
    GRID_ROWS,
    LOGICAL_BASE_OFFSET,
    BLOCK_SIZE,
    CityMapFormatError,
    city_map_offset,
    decode_buildings,
    decode_grid,
    decode_header,
)


def build_map(header_word: int = 0, buildings: list[int] | None = None,
              cells: list[list[int]] | None = None) -> bytes:
    data = bytearray(CITY_MAP_SIZE)
    # 68000 是大端：word 的高位 byte 在前。
    data[0] = (header_word >> 8) & 0xFF
    data[1] = header_word & 0xFF
    slots = list(buildings or [])
    slots += [BUILDING_ABSENT] * (BUILDING_SLOTS - len(slots))
    data[BUILDING_OFFSET:BUILDING_OFFSET + BUILDING_SLOTS] = bytes(slots)
    if cells:
        for row, values in enumerate(cells):
            start = GRID_OFFSET + row * (GRID_COLS // 2)
            # 一列是三個大端 word，每個 word 由低位往上放四格。
            for word_index in range(GRID_COLS // 4):
                word = 0
                for field in range(4):
                    word |= (values[word_index * 4 + field] & 0xF) << (field * 4)
                data[start + word_index * 2] = (word >> 8) & 0xFF
                data[start + word_index * 2 + 1] = word & 0xFF
    return bytes(data)


class OffsetTests(unittest.TestCase):
    def test_four_cities_share_one_block(self) -> None:
        first = city_map_offset(0)
        self.assertEqual(first, LOGICAL_BASE_OFFSET + CITY_MAP_FIRST_BLOCK * BLOCK_SIZE)
        for index in range(1, CITY_MAPS_PER_BLOCK):
            self.assertEqual(city_map_offset(index), first + index * CITY_MAP_SIZE)
        # 第五座城市換到下一個 block。
        self.assertEqual(city_map_offset(CITY_MAPS_PER_BLOCK), first + BLOCK_SIZE)

    def test_the_sixty_maps_are_contiguous(self) -> None:
        offsets = [city_map_offset(index) for index in range(CITY_COUNT)]
        self.assertEqual(offsets[-1] - offsets[0], (CITY_COUNT - 1) * CITY_MAP_SIZE)

    def test_an_index_outside_the_table_is_rejected(self) -> None:
        with self.assertRaises(CityMapFormatError):
            city_map_offset(CITY_COUNT)


class BuildingTests(unittest.TestCase):
    def test_a_position_byte_splits_into_row_and_column(self) -> None:
        # 值 = 列 × 20 + 行；73 就是第 3 列、第 13 行。
        buildings = decode_buildings(build_map(buildings=[73]))
        self.assertEqual(buildings, [
            {"slot": 0, "code": BUILDING_FIRST_CODE, "row": 13, "column": 3},
        ])

    def test_absent_slots_are_dropped_and_codes_follow_the_slot(self) -> None:
        buildings = decode_buildings(build_map(buildings=[BUILDING_ABSENT, 0, BUILDING_ABSENT, 239]))
        self.assertEqual([b["slot"] for b in buildings], [1, 3])
        self.assertEqual([b["code"] for b in buildings], [BUILDING_FIRST_CODE + 1, BUILDING_FIRST_CODE + 3])
        self.assertEqual(buildings[0], {"slot": 1, "code": BUILDING_FIRST_CODE + 1, "row": 0, "column": 0})
        self.assertEqual(buildings[1]["row"], GRID_ROWS - 1)
        self.assertEqual(buildings[1]["column"], GRID_COLS - 1)

    def test_a_position_outside_the_grid_is_rejected(self) -> None:
        with self.assertRaises(CityMapFormatError):
            decode_buildings(build_map(buildings=[GRID_ROWS * GRID_COLS]))


class GridTests(unittest.TestCase):
    def test_each_row_is_twelve_four_bit_cells(self) -> None:
        row = list(range(GRID_COLS))
        grid = decode_grid(build_map(cells=[row] + [[0] * GRID_COLS] * (GRID_ROWS - 1)))
        self.assertEqual(len(grid), GRID_ROWS)
        self.assertEqual(grid[0], row)
        self.assertTrue(all(len(line) == GRID_COLS for line in grid))

    def test_the_word_low_field_is_the_first_cell(self) -> None:
        """欄位 0 在 word 的最低位，而大端 word 的最低位在第二個 byte。

        逐 byte 讀會把每四格對調成 (2,3,0,1)，讓地圖的列兩兩互換——
        原版實跑對拍抓到的就是這個（PLAN §0.78）。
        """
        data = bytearray(build_map())
        data[GRID_OFFSET] = 0x9A      # word 的高位 byte → 第 3、4 格
        data[GRID_OFFSET + 1] = 0x5C  # word 的低位 byte → 第 1、2 格
        grid = decode_grid(bytes(data))
        self.assertEqual(grid[0][:4], [0xC, 0x5, 0xA, 0x9])


class HeaderTests(unittest.TestCase):
    def test_the_first_word_is_four_fields_with_drawing_biases(self) -> None:
        header = decode_header(build_map(header_word=0x2013))
        self.assertEqual(header["fields"], [3, 1, 0, 2])
        self.assertEqual(header["tileCodes"], {"0": 34, "1": 35, "2": 34})

    def test_the_fields_come_from_the_big_endian_word(self) -> None:
        # 磁碟上的 bytes 13 20 是大端 word 0x1320，欄位由低位往上取。
        header = decode_header(bytes([0x13, 0x20]) + bytes(CITY_MAP_SIZE - 2))
        self.assertEqual(header["fields"], [0, 2, 3, 1])


if __name__ == "__main__":
    unittest.main()
