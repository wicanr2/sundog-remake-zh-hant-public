#!/usr/bin/env python3
"""不依賴原始媒體的畫面幾何規則表單元測試。

測的是規則表本身的完整性——簽章夠長、識別碼不重複、宣告的段基底都認得——以及
釘選函式在簽章不唯一或落點對不上時會 fail-closed。語意的正確性不在這裡驗，
那是人讀反組譯的結果，寫在 docs/atari-st/screen-geometry.md。
"""

from __future__ import annotations

import unittest

from atari_st_screen_geometry import (
    MINIMUM_SIGNATURE_BYTES,
    PSYSTEM_STARTUP_RAW,
    RULES,
    SEGMENT_BASES,
    pin_rules,
)


def signature_of(rule: dict) -> bytes:
    return bytes.fromhex(str(rule["signature"]).replace(" ", ""))


class RuleTableTests(unittest.TestCase):
    def test_every_signature_is_long_enough(self) -> None:
        for rule in RULES:
            self.assertGreaterEqual(
                len(signature_of(rule)), MINIMUM_SIGNATURE_BYTES, f"{rule['id']} 的簽章太短"
            )

    def test_rule_ids_are_unique(self) -> None:
        ids = [rule["id"] for rule in RULES]
        self.assertEqual(len(ids), len(set(ids)))

    def test_every_rule_names_a_known_segment(self) -> None:
        for rule in RULES:
            self.assertIn(rule["segment"], SEGMENT_BASES, f"{rule['id']} 的段名不認得")

    def test_all_screens_are_covered(self) -> None:
        screens = {rule["screen"] for rule in RULES}
        self.assertEqual(
            screens,
            {
                "tactical",
                "map-sector",
                "map-system",
                "map-planet",
                "move-on-ship",
                "pilotage",
                "effects",
                "zoom-window",
                "trade",
                "character-status",
                "uniteller",
                "interior",
                "arena",
            },
        )

    def test_every_rule_carries_a_procedure_and_summary(self) -> None:
        for rule in RULES:
            self.assertTrue(str(rule["procedure"]).strip(), f"{rule['id']} 沒有程序名")
            self.assertTrue(str(rule["summary"]).strip(), f"{rule['id']} 沒有語意摘要")


class PinningTests(unittest.TestCase):
    def synthetic_disk(self, rule: dict) -> bytes:
        signature = signature_of(rule)
        base = SEGMENT_BASES[str(rule["segment"])]
        offset = PSYSTEM_STARTUP_RAW + base + int(str(rule["segmentOffset"]), 16)
        return bytes(bytearray(offset) + signature)

    def test_rejects_a_signature_that_appears_twice(self) -> None:
        rule = RULES[0]
        disk = self.synthetic_disk(rule) + bytes(16) + signature_of(rule)
        with self.assertRaises(ValueError) as caught:
            pin_rules(disk)
        self.assertIn("出現 2 次", str(caught.exception))

    def test_rejects_a_signature_at_the_wrong_address(self) -> None:
        rule = RULES[0]
        disk = bytes(64) + signature_of(rule)
        with self.assertRaises(ValueError) as caught:
            pin_rules(disk)
        self.assertIn("段基底與段內位址", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
