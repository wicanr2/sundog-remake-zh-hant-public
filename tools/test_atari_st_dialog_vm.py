#!/usr/bin/env python3
"""不依賴原始媒體的對話 VM 定位器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。真正對磁碟的驗證是
``python3 tools/atari_st_dialog_vm.py <archive>``，它會自己 fail-closed。
"""

from __future__ import annotations

import unittest

from atari_st_dialog_vm import RULES, locate, signature_bytes


class SignatureTests(unittest.TestCase):
    def test_every_rule_carries_a_usable_signature(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertGreaterEqual(len(signature_bytes(str(rule["signature"]))), 10)

    def test_rule_identifiers_are_unique(self) -> None:
        identifiers = [rule["id"] for rule in RULES]
        self.assertEqual(len(identifiers), len(set(identifiers)))

    def test_every_rule_names_a_procedure_and_says_what_it_does(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertIn(":", str(rule["procedure"]))
                self.assertTrue(str(rule["segmentOffset"]).startswith("0x"))
                self.assertTrue(str(rule["summary"]).strip())

    def test_the_protection_chain_is_pinned_end_to_end(self) -> None:
        # 光釘住 copy_check 沒有意義：要能指出「誰算校驗和、誰對帳、誰發作」三段。
        ids = {str(rule["id"]) for rule in RULES}
        for identifier in (
            "protection.check-sects",
            "protection.new-frame-hook",
            "protection.copy-check",
            "protection.deferred-quit",
        ):
            with self.subTest(rule=identifier):
                self.assertIn(identifier, ids)

    def test_the_two_roll_primitives_are_pinned(self) -> None:
        ids = {str(rule["id"]) for rule in RULES}
        self.assertIn("dialogue.made-roll", ids)
        self.assertIn("dialogue.made-attr", ids)


class LocateTests(unittest.TestCase):
    def test_finds_every_occurrence(self) -> None:
        needle = "aabb ccdd"
        payload = signature_bytes(needle)
        stream = b"\x00" + payload + b"\x11\x22" + payload
        self.assertEqual(locate(stream, needle), [1, 1 + len(payload) + 2])

    def test_an_empty_signature_is_refused(self) -> None:
        with self.assertRaises(ValueError):
            locate(b"\x00", "")
