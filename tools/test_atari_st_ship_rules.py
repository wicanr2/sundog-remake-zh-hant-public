#!/usr/bin/env python3
"""不依賴原始媒體的戰鬥／飛行規則定位器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。
"""

from __future__ import annotations

import unittest

from atari_st_ship_rules import RULES, locate, signature_bytes


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
