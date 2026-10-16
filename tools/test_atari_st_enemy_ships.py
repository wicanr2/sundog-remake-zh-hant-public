#!/usr/bin/env python3
"""不依賴原始媒體的敵艦資料表解碼器單元測試。

所有測試都在合成的 1,024-byte region 上跑，不讀取任何原版磁碟。合成器本身就是一份
規格：它照著解碼器宣稱的版面把四段接起來，測試再逐項把某一段弄壞，確認解碼器擋得住。
"""

from __future__ import annotations

import unittest

from atari_st_enemy_ships import (
    BAR_ROW_COUNT,
    BAR_ROW_SIZE,
    BAR_TABLE_OFFSET,
    NAME_FIELD_SIZE,
    NAME_TABLE_OFFSET,
    OUTLINE_CALL_SITES,
    OUTLINE_TABLE_OFFSET,
    OUTLINE_WORD_BASE,
    SHIP_COUNT,
    STAT_RECORD_SIZE,
    STAT_TABLE_OFFSET,
    TABLE_SIZE,
    decode_bar_rows,
    decode_names,
    decode_outlines,
    decode_stats,
)


NAMES = ["Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf"]


def synthetic_region() -> bytearray:
    region = bytearray(TABLE_SIZE)
    for index in range(SHIP_COUNT):
        start = STAT_TABLE_OFFSET + index * STAT_RECORD_SIZE
        # 除數與倒數步進不得為 0，所以刻意給 index + 1。
        region[start : start + STAT_RECORD_SIZE] = bytes([index * 10, index + 1, index * 20, index + 1])
    for row in range(BAR_ROW_COUNT):
        start = BAR_TABLE_OFFSET + row * BAR_ROW_SIZE
        region[start : start + BAR_ROW_SIZE] = bytes([(row % 6) + 1] * 7 + [0])
    for index, name in enumerate(NAMES):
        start = NAME_TABLE_OFFSET + index * NAME_FIELD_SIZE
        region[start] = len(name)
        region[start + 1 : start + 1 + len(name)] = name.encode("ascii")
    for offset in range(OUTLINE_TABLE_OFFSET, TABLE_SIZE):
        region[offset] = offset % 251
    return region


class LayoutTests(unittest.TestCase):
    def test_the_declared_call_sites_tile_the_region_exactly(self) -> None:
        cursor = OUTLINE_TABLE_OFFSET
        for word_offset, bound in OUTLINE_CALL_SITES:
            self.assertEqual((word_offset - OUTLINE_WORD_BASE) * 2, cursor)
            cursor += bound + 1
        self.assertEqual(cursor, TABLE_SIZE)

    def test_there_are_as_many_outlines_as_ships(self) -> None:
        self.assertEqual(len(OUTLINE_CALL_SITES), SHIP_COUNT)


class StatTests(unittest.TestCase):
    def test_reads_every_ship(self) -> None:
        ships = decode_stats(bytes(synthetic_region()))
        self.assertEqual(len(ships), SHIP_COUNT)
        self.assertEqual(ships[3]["weaponPower"], 30)
        self.assertEqual(ships[3]["armour"], 4)
        self.assertEqual(ships[3]["accuracy"], 60)
        self.assertEqual(ships[3]["fireRate"], 4)

    def test_a_zero_divisor_is_refused(self) -> None:
        region = synthetic_region()
        region[STAT_TABLE_OFFSET + 2 * STAT_RECORD_SIZE + 1] = 0
        with self.assertRaises(ValueError):
            decode_stats(bytes(region))

    def test_a_zero_countdown_step_is_refused(self) -> None:
        region = synthetic_region()
        region[STAT_TABLE_OFFSET + 2 * STAT_RECORD_SIZE + 3] = 0
        with self.assertRaises(ValueError):
            decode_stats(bytes(region))


class NameTests(unittest.TestCase):
    def test_reads_every_name(self) -> None:
        self.assertEqual(decode_names(bytes(synthetic_region())), NAMES)

    def test_an_overlong_length_is_refused(self) -> None:
        region = synthetic_region()
        region[NAME_TABLE_OFFSET] = NAME_FIELD_SIZE
        with self.assertRaises(ValueError):
            decode_names(bytes(region))

    def test_unprintable_text_is_refused(self) -> None:
        region = synthetic_region()
        region[NAME_TABLE_OFFSET + 1] = 0x01
        with self.assertRaises(ValueError):
            decode_names(bytes(region))

    def test_dirty_padding_is_refused(self) -> None:
        region = synthetic_region()
        region[NAME_TABLE_OFFSET + NAME_FIELD_SIZE - 1] = 0x41
        with self.assertRaises(ValueError):
            decode_names(bytes(region))


class BarTests(unittest.TestCase):
    def test_reads_every_row(self) -> None:
        rows = decode_bar_rows(bytes(synthetic_region()))
        self.assertEqual(len(rows), BAR_ROW_COUNT)
        self.assertEqual(rows[0], [1] * 7)

    def test_a_missing_terminator_is_refused(self) -> None:
        region = synthetic_region()
        region[BAR_TABLE_OFFSET + BAR_ROW_SIZE - 1] = 3
        with self.assertRaises(ValueError):
            decode_bar_rows(bytes(region))

    def test_an_out_of_range_length_is_refused(self) -> None:
        region = synthetic_region()
        region[BAR_TABLE_OFFSET] = 9
        with self.assertRaises(ValueError):
            decode_bar_rows(bytes(region))


class OutlineTests(unittest.TestCase):
    def test_reads_every_outline_and_covers_the_region(self) -> None:
        outlines = decode_outlines(bytes(synthetic_region()))
        self.assertEqual(len(outlines), SHIP_COUNT)
        covered = sum(int(outline["bytes"]) for outline in outlines)
        self.assertEqual(OUTLINE_TABLE_OFFSET + covered, TABLE_SIZE)

    def test_every_point_is_a_byte_pair(self) -> None:
        for outline in decode_outlines(bytes(synthetic_region())):
            for point in outline["points"]:
                self.assertEqual(len(point), 2)
                self.assertTrue(all(0 <= value <= 0xFF for value in point))


if __name__ == "__main__":
    unittest.main()
