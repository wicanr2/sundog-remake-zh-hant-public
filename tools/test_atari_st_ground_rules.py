#!/usr/bin/env python3
"""不依賴原始媒體的城市地面規則定位器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。真正對磁碟的驗證是
``python3 tools/atari_st_ground_rules.py <archive>``，它會自己 fail-closed。
"""

from __future__ import annotations

import unittest

from atari_st_ground_rules import RULES, locate, signature_bytes


class SignatureTests(unittest.TestCase):
    def test_every_rule_carries_a_usable_signature(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertGreaterEqual(len(signature_bytes(str(rule["signature"]))), 6)

    def test_rule_identifiers_are_unique(self) -> None:
        identifiers = [rule["id"] for rule in RULES]
        self.assertEqual(len(identifiers), len(set(identifiers)))

    def test_every_rule_names_a_procedure_and_a_segment_offset(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertIn(":", str(rule["procedure"]))
                self.assertTrue(str(rule["segmentOffset"]).startswith("0x"))
                self.assertTrue(str(rule["summary"]))

    def test_the_four_questions_this_file_answers_are_all_covered(self) -> None:
        # 這一組的存在理由就是那四項；少掉任何一項這份文件就名不副實。
        prefixes = {str(rule["id"]).split(".")[0] for rule in RULES}
        for topic in ("passability", "encounter", "pockets", "tile"):
            with self.subTest(topic=topic):
                self.assertIn(topic, prefixes)

    def test_pocket_bound_is_pinned_by_two_independent_procedures(self) -> None:
        # 「身上四格」推翻的是專案自己先前的斷言，所以它不能只靠一個寫入點。
        # 只看「上界」那兩條——同前綴的其他規則講的是位元組的內部版面，不是格數。
        bounds = {"pockets.set-membership", "pockets.range-check"}
        procedures = {
            str(rule["procedure"]) for rule in RULES if str(rule["id"]) in bounds
        }
        self.assertEqual(len(procedures), 2)

    def test_pocket_byte_layout_is_pinned_too(self) -> None:
        # 低 5 位是編號、高 3 位是剩餘次數：讀的那一支與寫初值的那一支都要在。
        layout = {
            str(rule["id"])
            for rule in RULES
            if str(rule["id"]).startswith("pockets.")
        }
        self.assertIn("pockets.charges-in-high-bits", layout)
        self.assertIn("pockets.shield-starts-with-seven", layout)


class LocateTests(unittest.TestCase):
    def test_finds_every_occurrence(self) -> None:
        needle = "aabb ccdd"
        payload = signature_bytes(needle)
        stream = b"\x00" + payload + b"\x11\x22" + payload
        self.assertEqual(locate(stream, needle), [1, 1 + len(payload) + 2])

    def test_absent_signature_returns_no_matches(self) -> None:
        self.assertEqual(locate(b"\x00\x01\x02", "aabb ccdd"), [])

    def test_an_empty_signature_is_refused(self) -> None:
        with self.assertRaises(ValueError):
            locate(b"\x00", "")


class SpritePatternTests(unittest.TestCase):
    def _disk(self) -> bytearray:
        from atari_st_ground_rules import SPRITE_PATTERNS_RAW

        disk = bytearray(0x5A000)
        disk[SPRITE_PATTERNS_RAW : SPRITE_PATTERNS_RAW + 8] = bytes(
            [0x00, 0x38, 0x38, 0x38, 0x38, 0x38, 0x38, 0x00]
        )
        return disk

    def test_patterns_decode(self) -> None:
        from atari_st_ground_rules import decode_sprite_patterns

        document = decode_sprite_patterns(bytes(self._disk()))
        self.assertEqual(len(document["patterns"]), 10)

    def test_a_nonzero_padding_byte_is_refused(self) -> None:
        from atari_st_ground_rules import SPRITE_PATTERNS_RAW, decode_sprite_patterns

        disk = self._disk()
        disk[SPRITE_PATTERNS_RAW + 15] = 0x99
        with self.assertRaises(ValueError):
            decode_sprite_patterns(bytes(disk))

    def test_a_wrong_first_pattern_is_refused(self) -> None:
        from atari_st_ground_rules import SPRITE_PATTERNS_RAW, decode_sprite_patterns

        disk = self._disk()
        disk[SPRITE_PATTERNS_RAW + 1] = 0x00
        with self.assertRaises(ValueError):
            decode_sprite_patterns(bytes(disk))
