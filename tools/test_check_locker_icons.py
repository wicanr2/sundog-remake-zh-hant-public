#!/usr/bin/env python3
"""check_locker_icons 的判準測試：合成圖示與畫面，不需要磁碟。"""

import json
import pathlib
import tempfile
import unittest
from unittest import mock

from PIL import Image

import check_locker_icons
from check_locker_icons import ICON_SIZE, main, name_hits, scan

RUNTIME_LEVEL = [v * 0x22 for v in range(8)]
PORT_LEVEL = [round(v * 255 / 7) for v in range(8)]


def make_icon(seed: int) -> list[tuple[int, int, int]]:
    """16×16 的圖示，用比較器的色階（decoded_icons 的輸出就是這種）。"""
    return [
        tuple(RUNTIME_LEVEL[(seed + dx + dy) % 8] for _ in range(3))
        for dy in range(ICON_SIZE)
        for dx in range(ICON_SIZE)
    ]


def make_shot(placements) -> Image.Image:
    """320×200 的畫面，用 port 的色階；placements 是 (x, y, seed)。"""
    frame = Image.new("RGB", (320, 200), (0, 0, 0))
    px = frame.load()
    for x0, y0, seed in placements:
        for dy in range(ICON_SIZE):
            for dx in range(ICON_SIZE):
                level = (seed + dx + dy) % 8
                px[x0 + dx, y0 + dy] = (PORT_LEVEL[level],) * 3
    return frame.resize((1280, 800), Image.NEAREST)


ICON_TABLE = [
    {"index": 0, "itemName": "(trash)", "itemCode": "0x02"},
    {"index": 1, "itemName": "shunt", "itemCode": "0x17"},
]


class ScanTest(unittest.TestCase):
    def test_finds_every_placement_at_its_exact_position(self):
        icons = [make_icon(0), make_icon(3)]
        shot = make_shot([(40, 60, 0), (100, 120, 3), (150, 120, 3)])
        with mock.patch.object(check_locker_icons, "decoded_icons", return_value=icons):
            hits = scan(shot, b"")
        self.assertEqual(
            sorted((h["x"], h["y"], h["iconIndex"]) for h in hits),
            [(40, 60, 0), (100, 120, 1), (150, 120, 1)],
        )

    def test_an_icon_that_is_not_on_screen_is_not_reported(self):
        icons = [make_icon(0), make_icon(3)]
        shot = make_shot([(40, 60, 0)])
        with mock.patch.object(check_locker_icons, "decoded_icons", return_value=icons):
            hits = scan(shot, b"")
        self.assertEqual([h["iconIndex"] for h in hits], [0])

    def test_one_wrong_pixel_breaks_the_match(self):
        # 判準是逐像素，不是相似——改一格就不該再算命中。
        icons = [make_icon(0)]
        shot = make_shot([(40, 60, 0)])
        big = shot.load()
        for dy in range(4):  # 4×4 個放大後的像素等於 ST 的一格
            for dx in range(4):
                big[(40 + 5) * 4 + dx, (60 + 5) * 4 + dy] = (255, 0, 0)
        with mock.patch.object(check_locker_icons, "decoded_icons", return_value=icons):
            hits = scan(shot, b"")
        self.assertEqual(hits, [])


class ExpectTest(unittest.TestCase):
    def test_names_come_from_the_icon_table(self):
        named = name_hits([{"x": 1, "y": 2, "iconIndex": 1}], ICON_TABLE)
        self.assertEqual(named[0]["itemName"], "shunt")
        self.assertEqual(named[0]["itemCode"], "0x17")

    def test_an_unknown_index_leaves_the_name_empty_rather_than_guessing(self):
        named = name_hits([{"x": 1, "y": 2, "iconIndex": 99}], ICON_TABLE)
        self.assertIsNone(named[0]["itemName"])


def run_main(hits, remake_result, remake_icons="22", mode="icons"):
    """用假的掃描與透明比對結果跑一次 main，避開磁碟與圖示解碼。"""
    with tempfile.TemporaryDirectory() as d:
        root = pathlib.Path(d)
        for name in ("orig.png", "remake.png"):
            Image.new("RGB", (320, 200)).save(root / name)
        (root / "disk.st").write_bytes(b"")
        (root / "icons.json").write_text(json.dumps({"icons": ICON_TABLE}))
        target = "compare_engineering_grid" if mode == "engineering" else "compare_icons"
        with mock.patch.object(check_locker_icons, "scan", return_value=hits), \
             mock.patch.object(check_locker_icons, target, return_value=remake_result):
            return main([
                str(root / "orig.png"), str(root / "disk.st"),
                "--icons", str(root / "icons.json"),
                "--remake", str(root / "remake.png"),
                "--remake-icons", remake_icons,
                "--remake-mode", mode,
            ])


class SameStateTest(unittest.TestCase):
    def test_both_sides_showing_the_icon_passes(self):
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 1}],
            {"missingIcons": [], "iconsFound": {"1": [5, 5]}},
            remake_icons="1",
        ), 0)

    def test_remake_missing_the_icon_fails(self):
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 1}],
            {"missingIcons": [1], "iconsFound": {}},
            remake_icons="1",
        ), 1)

    def test_an_icon_that_is_not_on_the_original_fails_before_looking_at_the_remake(self):
        # 原版畫面上根本沒有那個圖示時，「同狀態」的前提就不成立，不該去問重製版。
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 0}],
            {"missingIcons": [], "iconsFound": {"1": [5, 5]}},
            remake_icons="1",
        ), 1)


class EngineeringModeTest(unittest.TestCase):
    GRID = {
        "status": "exact-source-icons-with-known-status-overlays",
        "matchedSlots": 16,
        "expectedSlots": 16,
        "slots": [{"iconIndex": 1}] * 16,
    }

    def test_a_full_grid_with_the_wanted_icon_passes(self):
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 1}], dict(self.GRID),
            remake_icons="1", mode="engineering",
        ), 0)

    def test_a_partly_matched_grid_fails(self):
        grid = {**self.GRID, "matchedSlots": 15}
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 1}], grid, remake_icons="1", mode="engineering",
        ), 1)

    def test_a_full_grid_without_the_wanted_icon_fails(self):
        # 16 格都命中，但命中的不是我們要的那一格——這仍然不是同狀態。
        grid = {**self.GRID, "slots": [{"iconIndex": 9}] * 16}
        self.assertEqual(run_main(
            [{"x": 1, "y": 2, "iconIndex": 1}], grid, remake_icons="1", mode="engineering",
        ), 1)


if __name__ == "__main__":
    unittest.main()
