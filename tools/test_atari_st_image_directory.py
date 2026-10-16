#!/usr/bin/env python3
"""不依賴原始媒體的影像目錄解碼器單元測試。

所有測試都在合成的磁碟片段上跑，不讀取任何原版磁碟。合成器照著解碼器宣稱的版面把
目錄與影像標頭接起來，測試再逐項把某一段弄壞。
"""

from __future__ import annotations

import struct
import unittest

from atari_st_image_directory import (
    DIRECTORY_ENTRIES,
    ImageDirectoryError,
    decode_directory,
    directory_offset,
    image_stream_offset,
    tile_strips,
)


def build_disk(entries: list[tuple[int, int]], headers: list[tuple[int, int]]) -> bytes:
    """entries 是 (起點 word, 長度 word)；headers 是每一張影像宣告的寬高。"""
    size = image_stream_offset() + 0x20000
    disk = bytearray(size)
    for index, (offset_words, length_words) in enumerate(entries):
        struct.pack_into(">HH", disk, directory_offset() + index * 4, offset_words, length_words)
    for (offset_words, _), (width, height) in zip(entries, headers):
        struct.pack_into(">HH", disk, image_stream_offset() + offset_words * 2, width, height)
    return bytes(disk)


class DirectoryTests(unittest.TestCase):
    def test_entries_are_offset_and_length_in_words(self) -> None:
        disk = build_disk([(0, 100), (100, 50)], [(320, 200), (416, 32)])
        images = decode_directory(disk)
        self.assertEqual(len(images), 2)
        self.assertEqual(images[0]["byteLength"], 200)
        self.assertEqual(images[0]["declaredWidth"], 320)
        self.assertEqual(images[1]["rawOffset"], f"0x{image_stream_offset() + 200:05x}")

    def test_the_tail_of_the_directory_may_be_empty(self) -> None:
        disk = build_disk([(0, 100)], [(320, 200)])
        self.assertEqual(len(decode_directory(disk)), 1)

    def test_a_gap_between_entries_is_rejected(self) -> None:
        disk = build_disk([(0, 100), (101, 50)], [(320, 200), (416, 32)])
        with self.assertRaises(ImageDirectoryError):
            decode_directory(disk)

    def test_an_overlap_between_entries_is_rejected(self) -> None:
        disk = build_disk([(0, 100), (99, 50)], [(320, 200), (416, 32)])
        with self.assertRaises(ImageDirectoryError):
            decode_directory(disk)

    def test_an_implausible_declared_size_is_rejected(self) -> None:
        disk = build_disk([(0, 100)], [(9999, 200)])
        with self.assertRaises(ImageDirectoryError):
            decode_directory(disk)

    def test_a_used_slot_with_no_length_is_rejected(self) -> None:
        disk = bytearray(build_disk([(0, 100)], [(320, 200)]))
        struct.pack_into(">HH", disk, directory_offset() + 4, 100, 0)
        with self.assertRaises(ImageDirectoryError):
            decode_directory(bytes(disk))

    def test_an_empty_directory_is_rejected(self) -> None:
        disk = build_disk([], [])
        with self.assertRaises(ImageDirectoryError):
            decode_directory(disk)

    def test_the_directory_holds_eighty_eight_slots(self) -> None:
        self.assertEqual(DIRECTORY_ENTRIES, 88)


class TileStripTests(unittest.TestCase):
    def test_only_thirty_two_pixel_rows_count_as_strips(self) -> None:
        images = [
            {"index": 0, "rawOffset": "0x0", "declaredWidth": 320, "declaredHeight": 200},
            {"index": 1, "rawOffset": "0x1", "declaredWidth": 416, "declaredHeight": 32},
            {"index": 2, "rawOffset": "0x2", "declaredWidth": 160, "declaredHeight": 32},
            {"index": 3, "rawOffset": "0x3", "declaredWidth": 100, "declaredHeight": 32},
        ]
        strips = tile_strips(images)
        self.assertEqual([(s["index"], s["tileCount"]) for s in strips], [(1, 13), (2, 5)])


if __name__ == "__main__":
    unittest.main()
