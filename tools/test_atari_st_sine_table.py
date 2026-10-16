#!/usr/bin/env python3
"""不依賴原始媒體的正弦表解碼器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。
"""

from __future__ import annotations

import math
import unittest

from atari_st_sine_table import (
    SINE_SCALE,
    SINE_TABLE_LENGTH,
    SINE_TABLE_OFFSET,
    TURN_STEPS,
    decode_sine_table,
    ideal_table,
)


def synthetic_disk(table: list[int]) -> bytes:
    disk = bytearray(SINE_TABLE_OFFSET + SINE_TABLE_LENGTH)
    disk[SINE_TABLE_OFFSET : SINE_TABLE_OFFSET + SINE_TABLE_LENGTH] = bytes(table)
    return bytes(disk)


class IdealTableTests(unittest.TestCase):
    def test_covers_a_quarter_turn(self) -> None:
        table = ideal_table()
        self.assertEqual(len(table), SINE_TABLE_LENGTH)
        self.assertEqual(table[0], 0)
        self.assertEqual(table[-1], SINE_SCALE)

    def test_rises_without_ever_falling(self) -> None:
        table = ideal_table()
        for left, right in zip(table, table[1:]):
            self.assertLessEqual(left, right)

    def test_a_quarter_turn_is_a_quarter_of_the_turn_steps(self) -> None:
        self.assertEqual((SINE_TABLE_LENGTH - 1) * 4, TURN_STEPS // 2)
        self.assertAlmostEqual(
            math.sin((SINE_TABLE_LENGTH - 1) * math.pi / (TURN_STEPS / 4)), 1.0, places=9
        )


class DecodeTests(unittest.TestCase):
    def test_accepts_a_correct_table(self) -> None:
        self.assertEqual(decode_sine_table(synthetic_disk(ideal_table())), ideal_table())

    def test_one_wrong_entry_is_refused(self) -> None:
        table = ideal_table()
        table[7] += 1
        with self.assertRaises(ValueError):
            decode_sine_table(synthetic_disk(table))

    def test_an_off_by_one_region_is_refused(self) -> None:
        # 位址讀錯一個 byte 就會讓整張表位移，這是這個檢查主要要擋住的情況。
        shifted = ideal_table()[1:] + [0]
        with self.assertRaises(ValueError):
            decode_sine_table(synthetic_disk(shifted))

    def test_a_truncated_disk_is_refused(self) -> None:
        with self.assertRaises(ValueError):
            decode_sine_table(b"\x00" * (SINE_TABLE_OFFSET + SINE_TABLE_LENGTH - 1))


if __name__ == "__main__":
    unittest.main()


class BlockExtrasTests(unittest.TestCase):
    def _block(self) -> bytearray:
        from atari_st_sine_table import (
            ARROW_DOWN_OFFSET,
            ARROW_UP_OFFSET,
            MAN_FIGURE_OFFSET,
            MOUSE_SLEW_OFFSET,
        )
        disk = bytearray(0x5A000)
        for i, value in enumerate([8, 7, 6, 5, 5, 4, 4, 3, 3, 2, 2, 2, 1, 1, 1, 0]):
            disk[MOUSE_SLEW_OFFSET + i * 2 : MOUSE_SLEW_OFFSET + i * 2 + 2] = value.to_bytes(2, "big")
        up = [(11, 11), (5, 23), (8, 23), (8, 35), (14, 35), (14, 23), (17, 23), (11, 11)]
        for i, (x, y) in enumerate(up):
            disk[ARROW_UP_OFFSET + i * 2] = x
            disk[ARROW_UP_OFFSET + i * 2 + 1] = y
            disk[ARROW_DOWN_OFFSET + i * 2] = x
            disk[ARROW_DOWN_OFFSET + i * 2 + 1] = 46 - y
        for i in range(12):
            disk[MAN_FIGURE_OFFSET + i * 2] = 1 + i
            disk[MAN_FIGURE_OFFSET + i * 2 + 1] = 20 + i
        return disk

    def test_extras_decode(self) -> None:
        from atari_st_sine_table import decode_block_extras

        extras = decode_block_extras(bytes(self._block()))
        self.assertEqual(len(extras["manFigure"]["points"]), 12)
        self.assertEqual(extras["mouseSlew"]["words"][0], 8)

    def test_a_broken_arrow_mirror_is_refused(self) -> None:
        from atari_st_sine_table import ARROW_DOWN_OFFSET, decode_block_extras

        disk = self._block()
        disk[ARROW_DOWN_OFFSET + 1] ^= 0x01
        with self.assertRaises(ValueError):
            decode_block_extras(bytes(disk))

    def test_a_wild_man_point_is_refused(self) -> None:
        from atari_st_sine_table import MAN_FIGURE_OFFSET, decode_block_extras

        disk = self._block()
        disk[MAN_FIGURE_OFFSET] = 200
        with self.assertRaises(ValueError):
            decode_block_extras(bytes(disk))
