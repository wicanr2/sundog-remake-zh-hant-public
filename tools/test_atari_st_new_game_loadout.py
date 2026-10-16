#!/usr/bin/env python3
"""不依賴原始媒體的新遊戲配備解碼器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。
"""

from __future__ import annotations

import unittest

from atari_st_new_game_loadout import BASE_PREFIX, decode_write, find_loadout


def fillchar(offset: int, count: int, value: int) -> bytes:
    return BASE_PREFIX + bytes([offset, count, value, 0x70, 0x15])


def store(offset: int, value: int) -> bytes:
    return BASE_PREFIX + bytes([offset, value, 0xC8])


def store_with_high_bits(offset: int, value: int, high: int) -> bytes:
    return BASE_PREFIX + bytes([offset, value, 0x80, high, 0xA2, 0xC8])


# 船體：值是 13 + random() mod 4。
HULL_WRITE = BASE_PREFIX + bytes([0xC0, 0x0D, 0x00, 0x73, 0x0B, 0x04, 0x8F, 0xA2, 0xC8])
# 形狀完全不認識的寫入，用來測試 unrecognised 這條路。
UNKNOWN_WRITE = BASE_PREFIX + bytes([0xC2, 0x0D, 0x99, 0x77, 0x55, 0x33])


def synthetic_stream(*, include_hull: bool = True, include_unknown: bool = False) -> bytes:
    stream = bytearray(b"\xff\xff")
    stream += fillchar(0xB7, 5, 0x17)
    if include_hull:
        stream += HULL_WRITE
    if include_unknown:
        stream += UNKNOWN_WRITE
    stream += fillchar(0xC4, 5, 0x0A)
    stream += fillchar(0xC9, 5, 0x08)
    stream += store_with_high_bits(0xCE, 0x0B, 0xE0)
    stream += store(0xCF, 0x0C)
    stream += b"\x96\x00"
    return bytes(stream)


class DecodeWriteTests(unittest.TestCase):
    def test_fillchar_carries_its_count_and_value(self) -> None:
        write = decode_write(fillchar(0xB7, 5, 0x17), 0)
        self.assertEqual(write["kind"], "fillchar")
        self.assertEqual((write["stateOffset"], write["count"], write["value"]), (0xB7, 5, 0x17))

    def test_store_with_high_bits_keeps_the_whole_byte(self) -> None:
        write = decode_write(store_with_high_bits(0xCE, 0x0B, 0xE0), 0)
        self.assertEqual(write["value"], 0xEB)

    def test_a_write_to_another_base_is_not_decoded(self) -> None:
        self.assertIsNone(decode_write(bytes([0x9B, 0x04, 0x32, 0x80, 0xB7, 0x05, 0x17, 0x70, 0x15]), 0))


class FindLoadoutTests(unittest.TestCase):
    def test_the_hull_write_is_decoded_as_a_range(self) -> None:
        run = find_loadout(synthetic_stream())
        kinds = [write["kind"] for write in run]
        self.assertEqual(kinds, ["fillchar", "randomRange", "fillchar", "fillchar", "store", "store"])
        self.assertEqual((run[1]["value"], run[1]["span"]), (13, 4))

    def test_an_unknown_shape_is_kept_rather_than_skipped(self) -> None:
        run = find_loadout(synthetic_stream(include_hull=False, include_unknown=True))
        self.assertEqual(run[1]["kind"], "unrecognised")
        self.assertIn("bytes", run[1])

    def test_the_run_reads_the_same_items_without_the_hull_write(self) -> None:
        run = find_loadout(synthetic_stream(include_hull=False))
        self.assertEqual([write["value"] for write in run], [0x17, 0x0A, 0x08, 0xEB, 0x0C])

    def test_a_lone_shunt_fill_is_not_a_loadout(self) -> None:
        with self.assertRaises(ValueError):
            find_loadout(b"\xff\xff" + fillchar(0xB7, 5, 0x17) + b"\x96\x00")

    def test_two_matching_runs_are_refused_rather_than_guessed(self) -> None:
        stream = synthetic_stream() + b"\x00" * 32 + synthetic_stream()
        with self.assertRaises(ValueError):
            find_loadout(stream)


if __name__ == "__main__":
    unittest.main()
