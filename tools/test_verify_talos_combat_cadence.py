"""收據差異閘門的合成測試。入口：docs/atari-st/ship-rules.md；不計原版案數。"""
import copy
import json
from pathlib import Path
import tempfile
import unittest

from verify_talos_combat_cadence import compare_fuel_consumers, compare_mixed_energy, compare_fuel_flight_parent


class ContinuousParentGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.output = Path(self.temp.name)
        self.native = []
        for phase in range(3):
            for fuel in (0, 1, 4, 5, 254, 255):
                for energy in ([0, 0, 255], [254, 63, 63], [255, 255, 0]):
                    for kinds in ([0, 0], [1, 0], [0, 1], [1, 1]):
                        for attached in (False, True):
                            self.native.append(dict(Input=dict(Phase=phase, Fuel=fuel, Energy=energy, Kinds=kinds, Attached=attached),
                                Steps=[dict(Phase=(phase + n + 1) % 3, Tick=0, Fuel=0, Weapon=0, Shield=0,
                                            Setting=0, Angle=0, Radius=0, Words=[0, 0], Flags=[0] * 5) for n in range(9)]))
        self.formal = copy.deepcopy(self.native)

    def compare(self):
        for name, rows in (("native", self.native), ("remake", self.formal)):
            (self.output / f"{name}-fuel-flight-parent.json").write_text(json.dumps(rows))
        return compare_fuel_flight_parent(self.output)

    def test_complete_parent_coverage(self):
        result = self.compare()
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["updates"], 3888)

    def test_same_shortened_sequence_is_rejected(self):
        for rows in (self.native, self.formal):
            rows[0]["Steps"].pop()
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_same_duplicate_input_is_rejected(self):
        for rows in (self.native, self.formal):
            rows[1] = copy.deepcopy(rows[0])
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_late_energy_difference_is_rejected(self):
        self.formal[-1]["Steps"][-1]["Fuel"] = 1
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_matching_phase_reset_is_rejected(self):
        for rows in (self.native, self.formal):
            rows[0]["Steps"][2]["Phase"] = 2
        self.assertEqual(self.compare()["status"], "FAIL")


class ConsumerGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.output = Path(self.temp.name)
        self.formal = {"Scope": "synthetic gate input"}
        self.native = {}
        for group, count, filename, key in (
                ("Fuel", 1728, "native-fuel.json", "Native"),
                ("Aux", 320, "native-aux-pod.json", "Native"),
                ("Core", 1152, "native-levels.json", "CoreNative")):
            rows = [{"Input": {"Index": i}, "Fuel": 255} for i in range(count)]
            if group == "Aux":
                for row in rows:
                    row.update(Labels=[[0], [5]], Words=[0xab20, 0xab21], PriceWords=[4951, 9320])
            self.native[filename] = {key: copy.deepcopy(rows)}
            self.native[filename].update({"CoreCases" if group == "Core" else "Cases": count,
                                         "CoreDifferences" if group == "Core" else "Differences": 0})
            self.formal[group] = rows
            if group == "Aux":
                for row in rows:
                    row.pop("Labels")
                    row["LabelEmpty"] = [True, False]
            self.formal[group+"Cases"] = count
            self.formal[group+"Differences"] = 0

    def compare(self):
        for filename, data in self.native.items():
            (self.output / filename).write_text(json.dumps(data))
        (self.output / "remake-fuel-consumers.json").write_text(json.dumps(self.formal))
        return compare_fuel_consumers(self.output)

    def test_complete_matching_rows(self):
        self.assertEqual(self.compare()["status"], "PASS")

    def test_different_value_rejects_zero_counter(self):
        self.formal["Fuel"][3]["Fuel"] = 254
        result = self.compare()
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["groups"]["Fuel"]["differences"], 1)

    def test_full_source_byte_rejects(self):
        self.formal["Aux"][0]["Words"][0] = 0x2b20
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_label_semantics_rejects(self):
        self.formal["Aux"][0]["LabelEmpty"] = [False, False]
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_missing_case_rejects_even_matching_shortened_corpus(self):
        self.formal["Core"].pop()
        self.native["native-levels.json"]["CoreNative"].pop()
        self.formal["CoreCases"] = 1151
        self.native["native-levels.json"]["CoreCases"] = 1151
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_duplicate_case_rejects_even_matching_rows(self):
        for row in (self.formal["Fuel"][1], self.native["native-fuel.json"]["Native"][1]):
            row["Input"] = {"Index": 0}
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_original_difference_rejects(self):
        self.native["native-fuel.json"]["Differences"] = 1
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_missing_receipt_rejects(self):
        self.assertEqual(compare_fuel_consumers(self.output)["status"], "FAIL")


class MixedGateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.output = Path(self.temp.name)
        self.native = {"Cases": 12096, "Native": [
            {"Input": {"Index": i}, "Fuel": 250, "Words": [0xab20, 0xab21], "Draws": 1}
            for i in range(12096)]}
        self.formal = copy.deepcopy(self.native)
        self.formal["Differences"] = 0

    def compare(self):
        for name, data in (("native-fuel-mixed.json", self.native),
                           ("remake-fuel-mixed.json", self.formal)):
            (self.output / name).write_text(json.dumps(data))
        return compare_mixed_energy(self.output)

    def test_complete_matching_rows(self):
        self.assertEqual(self.compare()["status"], "PASS")

    def test_draw_difference_rejects_zero_counter(self):
        self.formal["Native"][1]["Draws"] = 0
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_tank_difference_rejects(self):
        self.formal["Native"][1]["Words"] = [0xab20, 0xab20]
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_matching_subset_rejects(self):
        for data in (self.native, self.formal):
            data["Native"].pop()
            data["Cases"] -= 1
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_duplicate_inputs_rejects(self):
        for data in (self.native, self.formal):
            data["Native"][1]["Input"] = {"Index": 0}
        self.assertEqual(self.compare()["status"], "FAIL")

    def test_missing_receipt_rejects(self):
        self.assertEqual(compare_mixed_energy(self.output)["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()
