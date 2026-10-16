#!/usr/bin/env python3
"""介面框量測的單元測試。用合成圖，不讀原始媒體。"""

from __future__ import annotations

import unittest

from PIL import Image, ImageDraw

from atari_st_ui_frame import MINIMUM_BORDER_PIXELS, connected_boxes

YELLOW = (255, 255, 0)
BLACK = (0, 0, 0)


def canvas() -> Image.Image:
    return Image.new("RGB", (320, 200), BLACK)


class ConnectedBoxTests(unittest.TestCase):
    def test_a_rectangle_outline_gives_its_own_bounds(self) -> None:
        image = canvas()
        ImageDraw.Draw(image).rectangle((10, 20, 109, 89), outline=YELLOW)
        boxes = connected_boxes(image, YELLOW)
        self.assertEqual(len(boxes), 1)
        self.assertEqual(
            {k: boxes[0][k] for k in ("x", "y", "width", "height")},
            {"x": 10, "y": 20, "width": 100, "height": 70},
        )

    def test_two_separate_outlines_stay_separate(self) -> None:
        image = canvas()
        draw = ImageDraw.Draw(image)
        draw.rectangle((10, 10, 89, 69), outline=YELLOW)
        draw.rectangle((150, 10, 229, 69), outline=YELLOW)
        boxes = connected_boxes(image, YELLOW)
        self.assertEqual(len(boxes), 2)
        self.assertEqual(sorted(box["x"] for box in boxes), [10, 150])

    def test_a_small_run_of_the_border_colour_is_not_a_box(self) -> None:
        # 文字與圖示也用得到框線的顏色。太小的連通區塊不算一塊。
        image = canvas()
        ImageDraw.Draw(image).line((10, 10, 10 + MINIMUM_BORDER_PIXELS - 2, 10), fill=YELLOW)
        self.assertEqual(connected_boxes(image, YELLOW), [])

    def test_a_colour_that_is_not_there_gives_nothing(self) -> None:
        self.assertEqual(connected_boxes(canvas(), YELLOW), [])


if __name__ == "__main__":
    unittest.main()
