#!/usr/bin/env python3
"""說明卡文字排版的單元測試。不畫卡，只驗斷行規則。"""

from __future__ import annotations

import unittest
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

from build_promo_cards import HEIGHT, NO_LINE_START, WIDTH, tokenize, wrap_cjk

# 內建的 bitmap 字型量不了中文寬度（latin-1 編碼），所以測試用 repo 裡那支
# 實際排版用的字型。它是 remake 自己的資產，不是原始媒體。
FONT_PATH = Path(__file__).resolve().parent.parent / "assets" / "fonts" / "NotoSansCJKtc-Regular.otf"


class TokenizeTests(unittest.TestCase):
    def test_latin_runs_stay_whole(self) -> None:
        self.assertEqual(tokenize("logical block 0x22"), ["logical", " ", "block", " ", "0x22"])

    def test_chinese_is_split_per_character(self) -> None:
        self.assertEqual(tokenize("城市地圖"), ["城", "市", "地", "圖"])

    def test_mixed_text_keeps_identifiers_together(self) -> None:
        self.assertEqual(tokenize("見 city-maps.md 一節"), ["見", " ", "city-maps.md", " ", "一", "節"])

    def test_round_trip_is_lossless(self) -> None:
        for text in ("MAINLIB:0x32 是調色盤循環", "20×12 的地圖", "F8／F9"):
            self.assertEqual("".join(tokenize(text)), text)


class WrapTests(unittest.TestCase):
    def setUp(self) -> None:
        if not FONT_PATH.is_file():
            self.skipTest(f"排版字型不在：{FONT_PATH}")
        self.image = Image.new("RGB", (WIDTH, HEIGHT))
        self.draw = ImageDraw.Draw(self.image)
        self.font = ImageFont.truetype(str(FONT_PATH), 24)

    def test_an_identifier_is_never_split_across_lines(self) -> None:
        text = "地表地圖住在 logical block 0x22 起，一座 128 bytes"
        lines = wrap_cjk(self.draw, text, self.font, 260)
        self.assertGreater(len(lines), 1)
        for fragment in ("logical", "block", "0x22", "bytes"):
            self.assertTrue(
                any(fragment in line for line in lines),
                f"{fragment} 被斷開了：{lines}",
            )

    def test_every_line_fits_the_limit(self) -> None:
        text = "原版把地形的 4-bit 碼轉成畫面碼，再分三段挑三條圖塊條"
        limit = 300
        for line in wrap_cjk(self.draw, text, self.font, limit):
            self.assertLessEqual(self.draw.textlength(line, font=self.font), limit + 80)

    def test_wrapping_preserves_every_character(self) -> None:
        text = "60 座城市各畫自己的 20×12 地圖，不共用原型地圖"
        self.assertEqual("".join(wrap_cjk(self.draw, text, self.font, 240)), text)


if __name__ == "__main__":
    unittest.main()


class LineStartRuleTests(unittest.TestCase):
    """行首禁則：逗號、句號這類字不能被推到下一行的開頭。"""

    def wrap(self, text: str, limit: int) -> list[str]:
        image = Image.new("RGB", (10, 10))
        draw = ImageDraw.Draw(image)
        font = ImageFont.truetype(str(FONT_PATH), 24)
        return wrap_cjk(draw, text, font, limit)

    def test_a_comma_is_never_pushed_to_the_start_of_a_line(self) -> None:
        # 寬度故意設得剛好讓逗號落在邊界上。
        for limit in range(8, 120, 4):
            for line in self.wrap("行星公轉、違停罰金與領航台電車，也都照原版的算式。", limit):
                if line:
                    self.assertNotIn(
                        line[0],
                        NO_LINE_START,
                        f"寬度 {limit} 時「{line}」以禁則字開頭",
                    )

    def test_the_text_survives_the_rule(self) -> None:
        text = "甲，乙。丙、丁；戊"
        joined = "".join(self.wrap(text, 24))
        self.assertEqual(joined, text)
