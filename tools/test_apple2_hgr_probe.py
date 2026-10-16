#!/usr/bin/env python3
import unittest

try:
    from tools.apple2_hgr_probe import HGR_WIDTH, hgr_row_offset, render_hgr
except ModuleNotFoundError:  # ``cd tools && python3 -m unittest discover``
    from apple2_hgr_probe import HGR_WIDTH, hgr_row_offset, render_hgr


class HGRProbeTests(unittest.TestCase):
    def test_row_interleave(self):
        self.assertEqual(hgr_row_offset(0), 0x0000)
        self.assertEqual(hgr_row_offset(1), 0x0400)
        self.assertEqual(hgr_row_offset(7), 0x1C00)
        self.assertEqual(hgr_row_offset(8), 0x0080)
        self.assertEqual(hgr_row_offset(64), 0x0028)

    def test_bit_placement_ignores_bit_seven(self):
        page = bytearray(8192)
        page[0] = 0b10000001
        pixels = render_hgr(bytes(page))
        self.assertEqual(list(pixels[:7]), [255, 0, 0, 0, 0, 0, 0])
        self.assertEqual(len(pixels), HGR_WIDTH * 192)

    def test_later_row_uses_interleaved_source(self):
        page = bytearray(8192)
        page[0x400] = 0x02
        pixels = render_hgr(bytes(page))
        self.assertEqual(pixels[HGR_WIDTH + 1], 255)


if __name__ == "__main__":
    unittest.main()
