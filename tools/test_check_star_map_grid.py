#!/usr/bin/env python3
"""check_star_map_grid 的判準測試：合成畫面，不需要原版磁碟。"""

import unittest

from PIL import Image

from check_star_map_grid import ORIGIN, PITCH, VIEWPORT, WHITE, find_marks, to_grid

RUNTIME_LEVEL = [v * 0x22 for v in range(8)]


def place(marks, size=(320, 200)):
    """在 320×200 上畫實心方塊當記號；find_marks 只看外接框中心。"""
    im = Image.new("RGB", size, (0, 0, 0))
    px = im.load()
    for cx, cy, half in marks:
        for y in range(cy - half, cy + half + 1):
            for x in range(cx - half, cx + half + 1):
                px[x, y] = WHITE
    return im


class FindMarksTest(unittest.TestCase):
    def test_each_blob_is_reported_once_with_its_centre(self):
        marks = find_marks(place([(120, 40, 3), (200, 90, 2)]))
        self.assertEqual(len(marks), 2)
        self.assertEqual(sorted(m["centre"] for m in marks), [[120.0, 40.0], [200.0, 90.0]])

    def test_anything_outside_the_viewport_is_ignored(self):
        # 視埠左邊界是 96；畫在 40 的記號不該被算進來。
        marks = find_marks(place([(40, 40, 3), (120, 40, 3)]))
        self.assertEqual([m["centre"] for m in marks], [[120.0, 40.0]])

    def test_two_touching_blobs_are_one(self):
        # 八連通：相鄰的兩塊會被併成一個，中心因此落在兩者之間。
        marks = find_marks(place([(120, 40, 2), (125, 40, 2)]))
        self.assertEqual(len(marks), 1)


class GridTest(unittest.TestCase):
    def test_a_mark_on_the_grid_maps_to_integers(self):
        cx = ORIGIN[0] + PITCH[0] * 4
        cy = ORIGIN[1] + PITCH[1] * 3
        self.assertEqual(to_grid([cx, cy]), [4.0, 3.0])

    def test_the_origin_is_grid_zero(self):
        self.assertEqual(to_grid(list(ORIGIN)), [0.0, 0.0])

    def test_a_mark_off_the_grid_is_not_an_integer(self):
        cx = ORIGIN[0] + PITCH[0] * 4 + 5
        grid = to_grid([cx, ORIGIN[1]])
        self.assertNotEqual(grid[0], round(grid[0]))


if __name__ == "__main__":
    unittest.main()
