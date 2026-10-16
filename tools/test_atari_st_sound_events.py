#!/usr/bin/env python3
"""不依賴原始媒體的音效事件定位器單元測試。

所有測試都用合成的位元組流，不讀取任何原版磁碟。真正對磁碟的驗證是
``python3 tools/atari_st_sound_events.py <archive>``，它會自己 fail-closed。
"""

from __future__ import annotations

import unittest

from atari_st_sound_events import (
    RULES,
    SOUNDS_WITHOUT_A_CONSTANT_CALLER,
    census_call_sites,
    is_a_global_number_byte,
    locate,
    signature_bytes,
)


class SignatureTests(unittest.TestCase):
    def test_every_rule_carries_a_usable_signature(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertGreaterEqual(len(signature_bytes(str(rule["signature"]))), 4)

    def test_rule_identifiers_are_unique(self) -> None:
        identifiers = [rule["id"] for rule in RULES]
        self.assertEqual(len(identifiers), len(set(identifiers)))

    def test_every_rule_names_a_sound_a_procedure_and_a_call(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertIn(":", str(rule["procedure"]))
                self.assertTrue(str(rule["segmentOffset"]).startswith("0x"))
                self.assertIn(rule["call"], ("fore_sound", "back_sound"))
                self.assertIsInstance(rule["sound"], int)

    def test_sound_numbers_stay_inside_the_bank(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertGreaterEqual(int(str(rule["sound"])), 0)
                self.assertLess(int(str(rule["sound"])), 28)

    def test_the_two_sounds_without_callers_are_not_also_claimed(self) -> None:
        # 一段不能同時「有常數呼叫點」又「沒有常數呼叫點」。
        claimed = {int(str(rule["sound"])) for rule in RULES}
        for index in SOUNDS_WITHOUT_A_CONSTANT_CALLER:
            with self.subTest(sound=index):
                self.assertNotIn(index, claimed)

    def test_every_rule_says_when_it_fires(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule["id"]):
                self.assertTrue(str(rule["summary"]).strip())


class LocateTests(unittest.TestCase):
    def test_finds_every_occurrence(self) -> None:
        needle = "aabb ccdd"
        payload = signature_bytes(needle)
        stream = b"\x00" + payload + b"\x11\x22" + payload
        self.assertEqual(locate(stream, needle), [1, 1 + len(payload) + 2])

    def test_an_empty_signature_is_refused(self) -> None:
        with self.assertRaises(ValueError):
            locate(b"\x00", "")


class CallSiteCensusTests(unittest.TestCase):
    """呼叫點普查同樣用合成位元組流驗，不讀原版磁碟。"""

    def census(self, stream: bytes, rules: list[dict]) -> dict:
        return census_call_sites(stream, rules, (0, len(stream)))

    def test_a_call_inside_a_pinned_signature_is_not_reported(self) -> None:
        stream = bytes.fromhex("0c 73 03")
        rules = [{"rawOffset": "0x0000", "signature": "0c73 03"}]
        result = self.census(stream, rules)
        self.assertEqual(result["foreSound"]["occurrences"], 1)
        self.assertEqual(result["foreSound"]["insideAPinnedSignature"], 1)
        self.assertEqual(result["foreSound"]["outside"], [])

    def test_a_call_outside_every_signature_is_reported_with_context(self) -> None:
        stream = bytes.fromhex("aa bb cc 73 03")
        result = self.census(stream, [])
        outside = result["foreSound"]["outside"]
        self.assertEqual(len(outside), 1)
        self.assertEqual(outside[0]["rawOffset"], "0x00003")
        self.assertTrue(outside[0]["precedingBytes"].endswith("aa bb cc"))

    def test_the_two_entry_points_are_counted_apart(self) -> None:
        result = self.census(bytes.fromhex("01 73 04 05 73 03"), [])
        self.assertEqual(result["backSound"]["occurrences"], 1)
        self.assertEqual(result["foreSound"]["occurrences"], 1)

    def test_the_interposed_idiom_is_counted_with_what_follows_it(self) -> None:
        # 這就是「不能往回讀引數」的證據形狀：段號 0x04 押在慣用法之前。
        result = self.census(bytes.fromhex("04 00 80 23 cb 73 03"), [])
        idiom = result["interposedIdiom"]
        self.assertEqual(idiom["sites"], 1)
        self.assertEqual(idiom["followedByACall"], 1)


class GlobalNumberEncodingTests(unittest.TestCase):
    """全域編號是變長的，所以一個像 opcode 的位元組可能只是編號的第二個位元組。"""

    def test_a_two_byte_global_number_swallows_the_next_byte(self) -> None:
        # 9b 04 87 73：87 的最高位是 1，所以 73 是編號 0x773 的第二個位元組，不是呼叫。
        stream = bytes.fromhex("9b 04 87 73 03")
        self.assertTrue(is_a_global_number_byte(stream, 3))

    def test_a_one_byte_global_number_leaves_the_next_byte_alone(self) -> None:
        # 9b 04 53 是 G53，53 的最高位是 0，所以後面那個位元組是新的指令。
        stream = bytes.fromhex("9b 04 53 73 03")
        self.assertFalse(is_a_global_number_byte(stream, 3))

    def test_a_byte_not_preceded_by_a_global_reference_is_left_alone(self) -> None:
        stream = bytes.fromhex("aa 04 87 73 03")
        self.assertFalse(is_a_global_number_byte(stream, 3))

    def test_the_start_of_the_stream_is_never_a_global_number_byte(self) -> None:
        stream = bytes.fromhex("73 03")
        self.assertFalse(is_a_global_number_byte(stream, 0))
