#!/usr/bin/env python3
"""check_ground_cells 的色階轉換測試：用合成圖，不需要磁碟。"""

import json
import pathlib
import tempfile
import unittest
from unittest import mock

from PIL import Image

import check_ground_cells
from check_ground_cells import ST_SIZE, main, to_runtime_palette

PORT_LEVEL = [round(v * 255 / 7) for v in range(8)]
RUNTIME_LEVEL = [v * 0x22 for v in range(8)]


class PaletteTest(unittest.TestCase):
    def test_each_three_bit_level_lands_on_the_comparator_scale(self):
        shot = Image.new("RGB", (8, 1))
        px = shot.load()
        for v in range(8):
            px[v, 0] = (PORT_LEVEL[v],) * 3
        # to_runtime_palette 會縮到 320×200，所以先放大成整數倍再進去
        big = shot.resize((ST_SIZE[0], ST_SIZE[1]), Image.NEAREST)
        out = to_runtime_palette(big).load()
        for v in range(8):
            # 每個色階佔 40 欄，取中間那一欄避開縮放邊界
            x = v * 40 + 20
            self.assertEqual(out[x, 100], (RUNTIME_LEVEL[v],) * 3)

    def test_the_two_scales_are_not_the_same_to_begin_with(self):
        # 若兩邊寫法本來就一樣，這支轉換就沒有存在的必要；釘住它們確實不同。
        self.assertNotEqual(PORT_LEVEL[5], RUNTIME_LEVEL[5])
        self.assertEqual((PORT_LEVEL[5], RUNTIME_LEVEL[5]), (182, 170))

    def test_output_is_always_320x200(self):
        self.assertEqual(to_runtime_palette(Image.new("RGB", (1280, 800))).size, ST_SIZE)
        self.assertEqual(to_runtime_palette(Image.new("RGB", (640, 400))).size, ST_SIZE)


def run_main(original_result, remake_result):
    """用假的分類結果跑一次 main，避開磁碟與圖塊解碼。"""
    with tempfile.TemporaryDirectory() as d:
        root = pathlib.Path(d)
        Image.new("RGB", (320, 200)).save(root / "orig.png")
        # 重製版那一張走 downscale3，尺寸不對它會直接擋下來
        Image.new("RGB", (960, 600)).save(root / "remake.png")
        (root / "disk.st").write_bytes(b"")
        (root / "dir.json").write_text(json.dumps({}))
        results = [original_result, remake_result]
        with mock.patch.object(
            check_ground_cells, "classify_ground_cells", side_effect=results
        ):
            return main([
                str(root / "orig.png"), str(root / "disk.st"),
                "--remake", str(root / "remake.png"),
                "--directory", str(root / "dir.json"),
            ])


CLEAN = {"counts": {"background": 25}, "mixedCells": [[4, 2]], "unknownMixedCells": []}


class SameStateTest(unittest.TestCase):
    def test_identical_classifications_pass(self):
        self.assertEqual(run_main(dict(CLEAN), dict(CLEAN)), 0)

    def test_a_different_cell_count_fails(self):
        other = {**CLEAN, "counts": {"background": 24}}
        self.assertEqual(run_main(dict(CLEAN), other), 1)

    def test_an_unexplained_cell_in_the_original_fails_before_comparing(self):
        bad = {**CLEAN, "unknownMixedCells": [[0, 0]]}
        # 第二個結果不會被取用——原版自己就過不了關
        self.assertEqual(run_main(bad, dict(CLEAN)), 1)


if __name__ == "__main__":
    unittest.main()
