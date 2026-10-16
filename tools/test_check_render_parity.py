#!/usr/bin/env python3
"""check_render_parity 的判準測試：用合成圖，不需要任何平台建置。"""

import pathlib
import tempfile
import unittest

from PIL import Image

from check_render_parity import main, pixel_diff, sha256


def write(path: pathlib.Path, colour, size=(8, 6), tweak=None) -> pathlib.Path:
    im = Image.new("RGBA", size, colour)
    if tweak:
        im.load()[tweak[0], tweak[1]] = tweak[2]
    im.save(path)
    return path


class HashTest(unittest.TestCase):
    def test_identical_content_hashes_the_same(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a = write(root / "a.png", (10, 20, 30, 255))
            b = write(root / "b.png", (10, 20, 30, 255))
            self.assertEqual(sha256(a), sha256(b))


class PixelDiffTest(unittest.TestCase):
    def test_one_changed_pixel_is_counted_and_located(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a = write(root / "a.png", (0, 0, 0, 255))
            b = write(root / "b.png", (0, 0, 0, 255), tweak=(3, 2, (255, 0, 0, 255)))
            count, total, diffs = pixel_diff(a, b)
            self.assertEqual((count, total), (1, 48))
            self.assertEqual(diffs[0][:2], (3, 2))

    def test_size_mismatch_is_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a = write(root / "a.png", (0, 0, 0, 255), size=(8, 6))
            b = write(root / "b.png", (0, 0, 0, 255), size=(4, 4))
            with self.assertRaises(ValueError):
                pixel_diff(a, b)


class MainTest(unittest.TestCase):
    def test_matching_builds_pass(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a = write(root / "linux.png", (1, 2, 3, 255))
            b = write(root / "win.png", (1, 2, 3, 255))
            self.assertEqual(main([str(a), str(b)]), 0)

    def test_differing_builds_fail(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a = write(root / "linux.png", (1, 2, 3, 255))
            b = write(root / "win.png", (1, 2, 3, 255), tweak=(0, 0, (9, 9, 9, 255)))
            self.assertEqual(main([str(a), str(b)]), 1)

    def test_a_scene_that_is_not_reproducible_fails_before_the_cross_check(self):
        # 同平台兩次就不一樣的場景，跨平台結論沒有意義，要在前置閘門就擋下。
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a1 = write(root / "l1.png", (1, 2, 3, 255))
            a2 = write(root / "l2.png", (1, 2, 3, 255), tweak=(1, 1, (4, 4, 4, 255)))
            b = write(root / "win.png", (1, 2, 3, 255))
            self.assertEqual(main([str(a1), str(b), "--same-platform", str(a1), str(a2)]), 1)

    def test_a_reproducible_scene_passes_both_gates(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            a1 = write(root / "l1.png", (1, 2, 3, 255))
            a2 = write(root / "l2.png", (1, 2, 3, 255))
            b = write(root / "win.png", (1, 2, 3, 255))
            self.assertEqual(main([str(a1), str(b), "--same-platform", str(a1), str(a2)]), 0)


if __name__ == "__main__":
    unittest.main()
