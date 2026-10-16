"""正式比較閘門的合成測試，不是原版執行或遊戲規則對拍。"""
import copy
import unittest

from verify_talos_fuel_depot import compare_formal


class FormalDepotGateTests(unittest.TestCase):
    def setUp(self):
        self.native = {
            "FuelNative": [{"ResultFuel": 37, "ResultBank": 13, "ResultCash": 17}
                           for _ in range(216)],
            "AuxNative": [{"ResultBank": 13, "ResultCash": 17,
                           "Words": [0xab21, 0xab20], "PriceWords": [4951, 9320]}
                          for _ in range(576)],
        }
        self.remake = {
            "Scope": "synthetic validation gate fixture; not an original receipt",
            "Fuel": [{"Index": i, "Fuel": 37, "Bank": 13, "Cash": 17,
                      "Executed": True, "Unsupported": []} for i in range(216)],
            "Aux": [{"Index": i, "Bank": 13, "Cash": 17, "Executed": True,
                     "Unsupported": [], "Holds": [
                         {"Kind": 1, "SourceCode": 0xab, "Quality": 1, "Price": 4951},
                         {"Kind": 0, "SourceCode": 0xab, "Quality": 1, "Price": 9320}]}
                    for i in range(576)],
        }

    def test_exact_complete_fixture_passes(self):
        result = compare_formal(self.native, self.remake)
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["FuelExecuted"], 216)
        self.assertEqual(result["AuxExecuted"], 576)

    def test_one_value_difference_rejects(self):
        self.remake["Aux"][7]["Holds"][0]["Price"] = 0
        result = compare_formal(self.native, self.remake)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["AuxDifferences"], 1)

    def test_full_source_byte_difference_rejects(self):
        self.remake["Aux"][7]["Holds"][0]["SourceCode"] = 0x2b
        result = compare_formal(self.native, self.remake)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["AuxDifferences"], 1)

    def test_unsupported_input_rejects_even_matching_values(self):
        self.remake["Fuel"][3]["Unsupported"] = ["buy-one-fuel-unit"]
        result = compare_formal(self.native, self.remake)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["UnsupportedFuelCases"], 1)

    def test_missing_row_rejects(self):
        self.remake["Aux"].pop()
        with self.assertRaises(ValueError):
            compare_formal(self.native, self.remake)

    def test_duplicate_index_rejects(self):
        self.remake["Fuel"][1]["Index"] = 0
        with self.assertRaises(ValueError):
            compare_formal(self.native, self.remake)

    def test_unexecuted_supported_row_rejects(self):
        self.remake["Fuel"][0]["Executed"] = False
        with self.assertRaises(ValueError):
            compare_formal(self.native, self.remake)

    def test_unexecuted_unsupported_row_still_rejects(self):
        self.remake["Fuel"][0]["Executed"] = False
        self.remake["Fuel"][0]["Unsupported"] = ["buy-one-fuel-unit"]
        result = compare_formal(self.native, self.remake)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["FuelExecuted"], 215)

    def test_comparison_preserves_receipts(self):
        before = copy.deepcopy((self.native, self.remake))
        compare_formal(self.native, self.remake)
        self.assertEqual(before, (self.native, self.remake))


if __name__ == "__main__":
    unittest.main()
