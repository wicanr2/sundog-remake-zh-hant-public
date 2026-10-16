"""交易類別表的結構檢查：版面對不上就失敗，而不是靜靜地輸出垃圾。"""

import json
import unittest
from pathlib import Path

import atari_st_trade_classes as tc


DOCUMENT = Path(__file__).resolve().parents[1] / "docs" / "atari-st" / "trade-classes.json"


class TradeClassDocumentTest(unittest.TestCase):
    def setUp(self) -> None:
        self.document = json.loads(DOCUMENT.read_text(encoding="utf-8"))

    def test_one_record_a_planet(self) -> None:
        self.assertEqual(len(self.document["records"]), tc.RECORD_COUNT)
        for index, record in enumerate(self.document["records"]):
            self.assertEqual(record["planetIndex"], index)

    def test_classes_are_three_bits(self) -> None:
        for record in self.document["records"]:
            self.assertEqual(len(record["supply"]), tc.GOODS_COUNT)
            self.assertEqual(len(record["demand"]), tc.GOODS_COUNT)
            for value in record["supply"] + record["demand"]:
                self.assertGreaterEqual(value, 0)
                self.assertLessEqual(value, tc.MAX_CLASS)

    def test_home_goods_are_goods_numbers(self) -> None:
        for record in self.document["records"]:
            for key in ("supplyHomeGoods", "demandHomeGoods"):
                self.assertGreaterEqual(record[key], 0)
                self.assertLessEqual(record[key], tc.GOODS_COUNT)

    def test_the_table_is_not_one_record_repeated(self) -> None:
        rows = {tuple(record["supply"]) for record in self.document["records"]}
        self.assertEqual(len(rows), tc.RECORD_COUNT)

    def test_unpack_is_low_field_first(self) -> None:
        # 0o... 三個位元一格，低位在前：0x01c8 是 [0, 1, 7, 0, 0]。
        self.assertEqual(tc.unpack_classes([0x01C8]), [0, 1, 7, 0, 0])

    def test_every_rule_is_pinned_once(self) -> None:
        self.assertEqual(len(self.document["rules"]), len(tc.RULES))
        for rule in self.document["rules"]:
            self.assertTrue(rule["rawOffset"].startswith("0x"))


if __name__ == "__main__":
    unittest.main()
