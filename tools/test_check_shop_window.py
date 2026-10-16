#!/usr/bin/env python3
"""check_shop_window 的判準測試：合成視窗，不需要原版磁碟或截圖。"""

import io
import pathlib
import tempfile
import unittest
from contextlib import redirect_stdout

from PIL import Image

from check_shop_window import BORDER, FRAME_BLUE, main, measure

# to_runtime_palette 會把 port 的色階換成執行時色階，所以合成時要用 port 那一側的值。
RUNTIME_LEVEL = [v * 0x22 for v in range(8)]
PORT_LEVEL = [round(v * 255 / 7) for v in range(8)]
PORT_BLUE = tuple(PORT_LEVEL[RUNTIME_LEVEL.index(c)] for c in FRAME_BLUE)


def make_window(x0=98, y0=76, width=156, bands=3, band_height=13, border=BORDER,
                heights=None) -> Image.Image:
    """畫一個店家視窗：外框加一疊等高的選項帶。"""
    frame = Image.new("RGB", (320, 200), (0, 0, 0))
    px = frame.load()
    # 外框左右兩邊，讓 frame_box 量得到寬度。
    bottom = y0 + 108
    for y in range(y0, bottom):
        for x in (x0, x0 + width - 1):
            px[x, y] = PORT_BLUE
    # 選項帶的水平線。
    each = heights or [band_height] * bands
    y = y0 + 53
    for index in range(len(each) + 1):
        for dy in range(border):
            for x in range(x0, x0 + width):
                px[x, y + dy] = PORT_BLUE
        if index < len(each):
            y += each[index]
    return frame.resize((1280, 800), Image.NEAREST)


def run(image, *args) -> tuple[int, str]:
    with tempfile.TemporaryDirectory() as d:
        path = pathlib.Path(d) / "shot.png"
        image.save(path)
        buffer = io.StringIO()
        with redirect_stdout(buffer):
            code = main([str(path), *args])
        return code, buffer.getvalue()


class GeometryTest(unittest.TestCase):
    def test_a_three_band_window_is_measured(self):
        result = measure(make_window())
        self.assertEqual(result["frame"]["width"], 156)
        self.assertEqual(len(result["bands"]), 3)
        self.assertEqual({b["height"] for b in result["bands"]}, {13})

    def test_the_same_window_moved_measures_the_same(self):
        # 判準的重點：視窗位置每次執行都不同，內部幾何不變。
        here = measure(make_window(x0=98, y0=76))
        there = measure(make_window(x0=74, y0=28))
        self.assertNotEqual(here["frame"]["x0"], there["frame"]["x0"])
        self.assertEqual(here["frame"]["width"], there["frame"]["width"])
        self.assertEqual([b["height"] for b in here["bands"]],
                         [b["height"] for b in there["bands"]])


class GateTest(unittest.TestCase):
    def test_expected_geometry_passes(self):
        code, out = run(make_window(), "--expect-bands", "3",
                        "--expect-width", "156", "--expect-band-height", "13")
        self.assertEqual(code, 0, out)

    def test_wrong_band_count_fails(self):
        code, out = run(make_window(bands=2), "--expect-bands", "3")
        self.assertEqual(code, 1)
        self.assertIn("選項帶", out)

    def test_wrong_width_fails(self):
        code, out = run(make_window(width=140), "--expect-width", "156")
        self.assertEqual(code, 1)
        self.assertIn("框寬", out)

    def test_uneven_bands_fail_even_without_an_expectation(self):
        # 高度不一致就不是一疊選項帶，判準本身不成立——不必先給期望值也要擋下來。
        code, out = run(make_window(heights=[13, 17, 13]))
        self.assertEqual(code, 1)
        self.assertIn("帶高不一致", out)

    def test_a_screen_without_the_frame_fails(self):
        code, out = run(Image.new("RGB", (1280, 800), (0, 0, 0)))
        self.assertEqual(code, 1)
        self.assertIn("藍框", out)


if __name__ == "__main__":
    unittest.main()
