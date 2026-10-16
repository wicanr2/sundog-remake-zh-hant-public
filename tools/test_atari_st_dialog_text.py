#!/usr/bin/env python3
"""對話文字解碼器的單元測試，全部使用合成 bytes，不讀原始磁碟。"""

from __future__ import annotations

import unittest

from atari_st_dialog_text import (
    BLOCK_SIZE,
    STOCK_TABLE_ENTRIES,
    decode_dialog_block,
    decode_stock_table,
    decode_tokens,
    preview,
    split_alternatives,
)


def text(value: str, trailing_space_on_last: bool = False) -> bytes:
    raw = bytearray(value.encode("ascii"))
    if trailing_space_on_last and raw:
        raw[-1] |= 0x80
    return bytes(raw)


class DecodeTokensTests(unittest.TestCase):
    def test_lowercase_and_punctuation(self) -> None:
        tokens = decode_tokens(text("hi, pal."), "line")
        self.assertEqual(tokens, [{"kind": "text", "text": "hi, pal."}])

    def test_high_bit_appends_a_space(self) -> None:
        tokens = decode_tokens(text("hi", trailing_space_on_last=True), "line")
        self.assertEqual(tokens, [{"kind": "text", "text": "hi "}])

    def test_line_keeps_uppercase_literal(self) -> None:
        self.assertEqual(preview(decode_tokens(text("Hi"), "line")), "Hi")

    def test_phrase_treats_uppercase_as_alternative_end(self) -> None:
        # "yeS" in a phrase means "yes" followed by an alternative separator.
        tokens = decode_tokens(text("yeS"), "phrase")
        self.assertEqual(tokens, [{"kind": "text", "text": "yes"}, {"kind": "alternative"}])

    def test_explicit_alternative_separator(self) -> None:
        tokens = decode_tokens(bytes(text("a")) + bytes([0x60]) + bytes(text("b")), "phrase")
        self.assertEqual(preview(tokens), "a|b")

    def test_backslash_and_pipe_both_decode_to_capital_i(self) -> None:
        self.assertEqual(preview(decode_tokens(bytes([0x5C, 0x7C]), "line")), "II")

    def test_low_bytes_become_references(self) -> None:
        tokens = decode_tokens(bytes([0x0F]) + text(" ok"), "line")
        self.assertEqual(tokens[0], {"kind": "reference", "index": "0x0f"})
        self.assertEqual(tokens[1], {"kind": "text", "text": " ok"})

    def test_documented_dynamic_reference_is_flagged(self) -> None:
        tokens = decode_tokens(bytes([0x24]), "line")
        self.assertTrue(tokens[0]["dynamic"])
        self.assertIn("money", str(tokens[0]["note"]))

    def test_unrecognised_byte_is_kept_as_unknown(self) -> None:
        tokens = decode_tokens(bytes([0x40]), "line")
        self.assertEqual(tokens, [{"kind": "unknown", "byte": "0x40"}])

    def test_rejects_unknown_kind(self) -> None:
        with self.assertRaises(ValueError):
            decode_tokens(b"a", "prose")


class SplitAlternativesTests(unittest.TestCase):
    def test_splits_and_drops_the_encoding_tail(self) -> None:
        tokens = decode_tokens(text("yeS") + text("nO"), "phrase")
        self.assertEqual(
            split_alternatives(tokens),
            [[{"kind": "text", "text": "yes"}], [{"kind": "text", "text": "no"}]],
        )

    def test_keeps_an_empty_alternative_in_the_middle(self) -> None:
        tokens = decode_tokens(text("aA") + bytes([0x60]) + text("b"), "phrase")
        self.assertEqual(len(split_alternatives(tokens)), 3)


class DecodeStockTableTests(unittest.TestCase):
    def synthetic(self, payloads: list[bytes]) -> bytes:
        table = bytearray(STOCK_TABLE_ENTRIES)
        body = bytearray()
        cursor = STOCK_TABLE_ENTRIES
        for index, payload in enumerate(payloads):
            table[index] = cursor & 0xFF
            body += payload
            cursor += len(payload)
        for index in range(len(payloads), STOCK_TABLE_ENTRIES):
            table[index] = cursor & 0xFF
        return bytes(table) + bytes(body)

    def test_decodes_payloads_in_table_order(self) -> None:
        data = self.synthetic([text("yeS") + text("no"), text("maybe")])
        phrases, payload_end = decode_stock_table(data)
        self.assertEqual(phrases[0]["preview"], "yes|no")
        self.assertEqual(phrases[1]["preview"], "maybe")
        self.assertEqual(payload_end, STOCK_TABLE_ENTRIES + 5 + 5)

    def test_marks_empty_dynamic_entries(self) -> None:
        data = self.synthetic([text("yes")])
        phrases, _ = decode_stock_table(data)
        money = phrases[0x24]
        self.assertTrue(money["empty"])
        self.assertTrue(money["dynamic"])

    def test_rejects_payload_overlapping_the_table(self) -> None:
        data = bytes([0]) * STOCK_TABLE_ENTRIES + b"abc"
        with self.assertRaises(ValueError):
            decode_stock_table(data)


class DecodeDialogBlockTests(unittest.TestCase):
    def synthetic_block(self, phrases: list[bytes], lines: list[bytes], data_start: int = 0x20) -> bytes:
        block = bytearray(BLOCK_SIZE)
        block[0x10] = len(phrases)
        block[0x11] = data_start // 2
        cursor = data_start
        for payload in phrases + lines:
            block[cursor] = len(payload)
            block[cursor + 1 : cursor + 1 + len(payload)] = payload
            cursor += 1 + len(payload)
            cursor += cursor & 1
        return bytes(block)

    def test_splits_block_phrases_from_lines(self) -> None:
        data = self.synthetic_block([text("yeS") + text("no")], [text("Hi ") + bytes([0x24])])
        block = decode_dialog_block(data, 0)
        self.assertEqual(block["blockPhraseCount"], 1)
        self.assertEqual(block["lineCount"], 1)
        self.assertEqual(block["strings"][0]["referenceIndex"], "0x34")
        self.assertEqual(block["strings"][0]["preview"], "yes|no")
        self.assertEqual(block["strings"][1]["preview"], "Hi <0x24>")

    def test_zero_length_string_terminates_the_block(self) -> None:
        data = self.synthetic_block([], [text("hi")])
        block = decode_dialog_block(data, 0)
        self.assertEqual(block["lineCount"], 1)
        self.assertTrue(block["trailingIsZero"])
        self.assertIsNotNone(block["terminatorOffset"])

    def test_rejects_string_running_past_the_block(self) -> None:
        data = bytearray(BLOCK_SIZE)
        data[0x10] = 0
        data[0x11] = 0xFC  # data start 504, only eight bytes left in the block
        data[504] = 0x20   # a 32-byte string cannot fit
        with self.assertRaises(ValueError):
            decode_dialog_block(bytes(data), 0)

    def test_rejects_data_start_pointing_into_the_header(self) -> None:
        data = bytearray(BLOCK_SIZE)
        data[0x11] = 0x04  # data start 8, inside the block's own offset table
        with self.assertRaises(ValueError):
            decode_dialog_block(bytes(data), 0)


if __name__ == "__main__":
    unittest.main()
