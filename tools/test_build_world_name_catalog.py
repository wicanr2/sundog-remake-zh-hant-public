#!/usr/bin/env python3
"""地名 catalog 產生器的單元測試，全部使用合成的世界資料與譯名表。"""

from __future__ import annotations

import unittest

from build_world_name_catalog import build_entries, slug


WORLD = {
    "tables": {
        "systems": {"records": [{"name": "Alpha"}, {"name": "Beta"}]},
        "planets": {"records": [{"name": "Aone"}, {"name": "Bone"}]},
        "cities": {
            "records": [
                {"name": "Ca"},
                {"name": "Cb"},
                {"name": "dummy"},
                {"name": "Hidden"},
            ]
        },
    },
    "grouping": {
        "systemPlanetRuns": [
            {"system": "Alpha", "start": 0, "count": 1},
            {"system": "Beta", "start": 1, "count": 1},
        ],
        "planetCityRuns": [
            {"planet": "Aone", "start": 0, "count": 1},
            {"planet": "Bone", "start": 1, "count": 1},
        ],
        "trailingCities": [{"city": "Hidden", "planet": "Aone"}],
    },
}

NAMES = {
    "displayFormat": "{zh}({source})",
    "systems": {"Alpha": "阿爾法", "Beta": "貝塔"},
    "planets": {"Aone": "艾萬", "Bone": "博恩"},
    "cities": {"Ca": "卡阿", "Cb": "卡貝", "Hidden": "隱地"},
}


class BuildWorldNameCatalogTests(unittest.TestCase):
    def entries_by_key(self, world=WORLD, names=NAMES):
        entries, errors = build_entries(world, names)
        return {entry["key"]: entry for entry in entries}, errors

    def test_slug_matches_go_identity_rules(self) -> None:
        self.assertEqual(slug("Ft. Lyon"), "ft-lyon")
        self.assertEqual(slug("W'orrad"), "w-orrad")
        self.assertEqual(slug("Glory III"), "glory-iii")

    def test_generates_every_level_with_source_in_parentheses(self) -> None:
        entries, errors = self.entries_by_key()
        self.assertEqual(errors, [])
        self.assertEqual(entries["ui.system.alpha"]["translation"], "阿爾法(Alpha)")
        self.assertEqual(entries["ui.planet.aone"]["translation"], "艾萬(Aone)")
        self.assertEqual(entries["ui.city.ca"]["translation"], "卡阿(Ca)")
        self.assertEqual(entries["ui.destination.ca"]["translation"], "艾萬(Aone)／卡阿(Ca)")

    def test_skips_filler_city_records(self) -> None:
        entries, _ = self.entries_by_key()
        self.assertNotIn("ui.city.dummy", entries)
        self.assertNotIn("ui.destination.dummy", entries)

    def test_trailing_city_uses_its_declared_planet(self) -> None:
        entries, errors = self.entries_by_key()
        self.assertEqual(errors, [])
        self.assertEqual(entries["ui.destination.hidden"]["translation"], "艾萬(Aone)／隱地(Hidden)")

    def test_reports_missing_translation(self) -> None:
        names = {**NAMES, "cities": {"Ca": "卡阿", "Hidden": "隱地"}}
        _, errors = build_entries(WORLD, names)
        self.assertTrue(any("Cb" in error for error in errors))

    def test_reports_translation_without_a_source(self) -> None:
        names = {**NAMES, "systems": {**NAMES["systems"], "Gamma": "伽瑪"}}
        _, errors = build_entries(WORLD, names)
        self.assertTrue(any("Gamma" in error for error in errors))

    def test_honours_a_different_display_format(self) -> None:
        names = {**NAMES, "displayFormat": "{zh}"}
        entries, errors = build_entries(WORLD, names)
        self.assertEqual(errors, [])
        by_key = {entry["key"]: entry for entry in entries}
        self.assertEqual(by_key["ui.system.alpha"]["translation"], "阿爾法")


if __name__ == "__main__":
    unittest.main()
