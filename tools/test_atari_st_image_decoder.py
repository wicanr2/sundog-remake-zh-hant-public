#!/usr/bin/env python3
"""不依賴原始媒體的 Atari ST image decoder 單元測試。"""

from __future__ import annotations

import hashlib
import struct
import unittest

from decode_atari_st_images import (
    decode_record,
    decompress_image,
    describe_image_stream_coverage,
    flip4,
)


class AtariSTImageDecoderTests(unittest.TestCase):
    def test_short_rle(self) -> None:
        pixels, consumed = decompress_image(bytes([0x32]), 4, 1)
        self.assertEqual(pixels, bytes([flip4(2)] * 4))
        self.assertEqual(consumed, 1)

    def test_long_rle(self) -> None:
        pixels, consumed = decompress_image(bytes([0x80 | 0x0D, 0x03]), 4, 1)
        self.assertEqual(pixels, bytes([flip4(0x0D)] * 4))
        self.assertEqual(consumed, 2)

    def test_h4l4_literals(self) -> None:
        pixels, consumed = decompress_image(bytes([0x90 | 0x01, 0x03, 0x23, 0x4F]), 4, 1)
        self.assertEqual(pixels, bytes([flip4(2), flip4(3), flip4(4), flip4(15)]))
        self.assertEqual(consumed, 4)

    def test_previous_line_then_literal(self) -> None:
        # 0x10 + 0x00 produces two pixels in row one. B0/00 copies one pixel
        # from the previous row, then appends one literal pixel.
        pixels, consumed = decompress_image(bytes([0x10, 0xBF, 0x00]), 2, 2)
        self.assertEqual(pixels, bytes([0, 0, 0, flip4(0x0F)]))
        self.assertEqual(consumed, 3)

    def test_record_manifest_hashes_header_payload_and_alignment_padding(self) -> None:
        disk = struct.pack(">HH", 4, 1) + bytes([0x32, 0xA5])
        record, pixels = decode_record(disk, 0)

        self.assertEqual(pixels, bytes([flip4(2)] * 4))
        self.assertEqual(record["recordSpanSize"], 6)
        self.assertEqual(record["recordSpanSha256"], hashlib.sha256(disk).hexdigest())
        self.assertEqual(record["compressedPayloadSha256"], hashlib.sha256(bytes([0x32])).hexdigest())
        self.assertEqual(record["alignmentPaddingBytes"], 1)

    def test_stream_coverage_records_gaps_without_assigning_semantics(self) -> None:
        disk = bytes(10)
        records = [
            ({"rawOffsetDecimal": 0, "spanEndExclusive": "0x4"}, b""),
            ({"rawOffsetDecimal": 6, "spanEndExclusive": "0xa"}, b""),
        ]

        coverage = describe_image_stream_coverage(disk, records, start=0, end=10)

        self.assertEqual(coverage["decodedRecordCount"], 2)
        self.assertEqual(coverage["decodedRecordSpanBytes"], 8)
        self.assertEqual(coverage["gapBytes"], 2)
        self.assertEqual(coverage["gaps"][0]["rawOffset"], "0x4")
        self.assertEqual(coverage["gaps"][0]["kind"], "unclassified-gap")
        self.assertTrue(coverage["gaps"][0]["allBytesZero"])

    def test_stream_coverage_marks_zero_dimension_termination(self) -> None:
        disk = bytes(10)
        records = [({"rawOffsetDecimal": 0, "spanEndExclusive": "0x4"}, b"")]

        coverage = describe_image_stream_coverage(disk, records, start=0, end=10)

        self.assertEqual(coverage["gaps"][0]["kind"], "zero-dimension-termination-or-padding")
        self.assertEqual(coverage["gaps"][0]["zeroDimensionHeader"], {"width": 0, "height": 0})

    def test_stream_coverage_recognizes_secondary_explicit_skip(self) -> None:
        start = 0x150FA
        end = 0x15170
        disk = bytes(end)

        coverage = describe_image_stream_coverage(disk, [], start=start, end=end)

        self.assertEqual(coverage["gaps"][0]["kind"], "secondary-ripper-explicit-skip")


if __name__ == "__main__":
    unittest.main()
