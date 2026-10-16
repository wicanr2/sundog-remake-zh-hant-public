#!/usr/bin/env python3
"""手冊用語對照表 parser 的單元測試。

只用合成的 Markdown 片段，不讀手冊本身，這樣測的是 parser 的規則而不是當下的內容。
"""

from __future__ import annotations

import unittest

from validate_manual_ui_terms import TABLE_HEADER, parse_table


def build_manual(rows: list[str], trailing: str = "\n一般段落。\n") -> str:
    header = f"{TABLE_HEADER}\n|---|---|---|\n"
    return "前言。\n\n" + header + "\n".join(rows) + "\n" + trailing


class ParseTableTests(unittest.TestCase):
    def test_rows_are_split_into_three_columns(self) -> None:
        manual = build_manual(["| 工程艙 | 工程艙 | `ui.engineering` |",
                               "| 物資 | 殖民地物資 | `ui.supplies` |"])
        self.assertEqual(parse_table(manual), [
            {"manualTerm": "工程艙", "uiLabel": "工程艙", "key": "ui.engineering"},
            {"manualTerm": "物資", "uiLabel": "殖民地物資", "key": "ui.supplies"},
        ])

    def test_backticks_are_stripped_from_the_manual_term(self) -> None:
        manual = build_manual(["| `Uniteller` | Uniteller 銀行 | `ui.uniteller` |"])
        self.assertEqual(parse_table(manual)[0]["manualTerm"], "Uniteller")

    def test_the_table_ends_at_the_first_non_table_line(self) -> None:
        manual = build_manual(["| 工程艙 | 工程艙 | `ui.engineering` |"],
                              trailing="\n### 下一節\n\n| 這是 | 另一張 | 表 |\n")
        self.assertEqual(len(parse_table(manual)), 1)

    def test_a_missing_table_is_an_error(self) -> None:
        with self.assertRaises(ValueError):
            parse_table("手冊裡沒有這張表。\n")

    def test_a_malformed_row_is_an_error(self) -> None:
        manual = build_manual(["| 工程艙 | 工程艙 | ui.engineering |"])
        with self.assertRaises(ValueError):
            parse_table(manual)

    def test_an_empty_table_is_an_error(self) -> None:
        with self.assertRaises(ValueError):
            parse_table(f"前言。\n\n{TABLE_HEADER}\n|---|---|---|\n\n段落。\n")


if __name__ == "__main__":
    unittest.main()
