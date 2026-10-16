#!/usr/bin/env python3
"""oracle 對拍工具的單元測試，使用合成的 dump 與 evidence，不依賴 oracle 原始碼。"""

from __future__ import annotations

import unittest

from crosscheck_dialog_against_oracle import compare, parse_oracle_dump

DUMP = """## Stock phrases
00 06c41 yes|no
01 06cd9 go|are
## Sector 0x07000 (dialog block 0x10)
070a8 [=34] please|go
070b2 [2a] [15], [31][3f]
"""

DOCUMENT = {
    "stockPhrases": {
        "records": [
            {"index": "0x00", "preview": "yes|no"},
            {"index": "0x01", "preview": "go|are"},
        ]
    },
    "dialogBlocks": [
        {
            "strings": [
                {"rawOffset": "0x070a8", "preview": "please|go"},
                {"rawOffset": "0x070b2", "preview": "<0x2a> <0x15>, <0x31><0x3f>"},
            ]
        }
    ],
}


class CrosscheckTests(unittest.TestCase):
    def test_parses_both_oracle_line_shapes(self) -> None:
        stock, strings = parse_oracle_dump(DUMP)
        self.assertEqual(stock[0x00], "yes|no")
        self.assertEqual(strings[0x070A8], "please|go")
        self.assertEqual(strings[0x070B2], "@2a @15, @31@3f")

    def test_matching_corpus_reports_no_mismatch(self) -> None:
        stock, strings = parse_oracle_dump(DUMP)
        checked_stock, checked_strings, mismatches = compare(DOCUMENT, stock, strings)
        self.assertEqual((checked_stock, checked_strings), (2, 2))
        self.assertEqual(mismatches, [])

    def test_reports_a_text_difference(self) -> None:
        stock, strings = parse_oracle_dump(DUMP.replace("yes|no", "yes|nope"))
        _, _, mismatches = compare(DOCUMENT, stock, strings)
        self.assertEqual(len(mismatches), 1)
        self.assertEqual(mismatches[0]["kind"], "stock")

    def test_reports_a_reference_difference(self) -> None:
        stock, strings = parse_oracle_dump(DUMP.replace("[31]", "[32]"))
        _, _, mismatches = compare(DOCUMENT, stock, strings)
        self.assertEqual(len(mismatches), 1)
        self.assertEqual(mismatches[0]["kind"], "string")

    def test_reports_a_string_the_oracle_never_produced(self) -> None:
        stock, strings = parse_oracle_dump(DUMP)
        document = {
            "stockPhrases": DOCUMENT["stockPhrases"],
            "dialogBlocks": [
                {"strings": DOCUMENT["dialogBlocks"][0]["strings"] + [{"rawOffset": "0x07f00", "preview": "hi"}]}
            ],
        }
        _, _, mismatches = compare(document, stock, strings)
        self.assertEqual([m["kind"] for m in mismatches], ["string-absent-from-oracle"])

    def test_ignores_oracle_lines_before_the_dialog_blocks(self) -> None:
        _, strings = parse_oracle_dump(DUMP)
        # The stock phrase payload lives below 0x07000 and must not be read as a
        # block string, or every phrase would be compared twice under two keys.
        self.assertTrue(all(offset >= 0x07000 for offset in strings))


if __name__ == "__main__":
    unittest.main()
