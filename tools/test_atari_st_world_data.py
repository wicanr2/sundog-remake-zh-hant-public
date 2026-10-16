#!/usr/bin/env python3
"""不依賴原始媒體的 Atari ST 世界資料表解碼器單元測試。

所有測試都用合成的 record／磁碟區段，不讀取任何原版磁碟。
"""

from __future__ import annotations

import unittest

from atari_st_world_data import (
    CITY_RECORD_SIZE,
    MINIMUM_SIGNATURE_BYTES,
    NAME_FIELD_SIZE,
    PCODE_RULES,
    PLANET_RECORD_SIZE,
    SYSTEM_RECORD_SIZE,
    crosscheck,
    crosscheck_planet_layout,
    decode_name,
    decode_planet_fields,
    decode_table,
    pin_pcode_rules,
)


def name_field(name: str, pad_byte: int = 0x00) -> bytes:
    body = name.encode("ascii")
    padding = bytes([pad_byte]) * (NAME_FIELD_SIZE - 1 - len(body))
    return bytes([len(body)]) + body + padding


def record(name: str, tail: bytes) -> bytes:
    return name_field(name) + tail


def table_records(names: list[str], size: int) -> list[dict[str, object]]:
    tail_size = size - NAME_FIELD_SIZE
    disk = b"".join(record(name, bytes(range(tail_size))) for name in names)
    return decode_table(disk, 0, size, len(names), "test")


def planet_record(rate: int, landing: int, layout: int) -> bytes:
    """組一筆合成的行星 record：name 之後是 word +5、+6、+7。"""
    tail = bytes([rate, landing, 0x14, 0x5A, layout >> 8, layout & 0xFF])
    return name_field("Test") + tail


class PlanetFieldTests(unittest.TestCase):
    def test_landing_square_splits_low_byte(self) -> None:
        # 130 mod 20 = 10、130 div 20 = 6，加上基準 10 與 6。
        fields = decode_planet_fields(planet_record(1, 130, 0x0C33))
        self.assertEqual(fields["landingSquare"], {"column": 20, "row": 12})

    def test_orbit_rate_is_the_high_byte(self) -> None:
        self.assertEqual(decode_planet_fields(planet_record(8, 0, 0x0C33))["orbitRate"], 8)

    def test_layout_word_splits_into_three_fields(self) -> None:
        # 0x0c33 = 城市數 3、預設城市 6、區間起點 6（皆為 1-based）。
        fields = decode_planet_fields(planet_record(0, 0, 0x0C33))
        self.assertEqual(fields["cityCount"], 3)
        self.assertEqual(fields["defaultCityIndex"], 5)
        self.assertEqual(fields["firstCityIndex"], 5)
        self.assertEqual(fields["layoutSpareBit"], 0)

    def test_layout_fields_are_independent(self) -> None:
        """預設城市與區間起點是兩個欄位，資料裡相等不代表結構上綁在一起。"""
        layout = 4 | (9 << 3) | (2 << 9)
        fields = decode_planet_fields(planet_record(0, 0, layout))
        self.assertEqual(fields["defaultCityIndex"], 8)
        self.assertEqual(fields["firstCityIndex"], 1)

    def test_spare_bit_is_reported(self) -> None:
        fields = decode_planet_fields(planet_record(0, 0, 0x8C33))
        self.assertEqual(fields["layoutSpareBit"], 1)


class PlanetLayoutCrosscheckTests(unittest.TestCase):
    def planets(self, layouts: list[int]) -> list[dict[str, object]]:
        disk = b"".join(
            name_field(f"P{index}") + bytes([0, 0, 0x14, 0x5A, layout >> 8, layout & 0xFF])
            for index, layout in enumerate(layouts)
        )
        return decode_table(disk, 0, PLANET_RECORD_SIZE, len(layouts), "planet")

    def test_accepts_matching_runs(self) -> None:
        planets = self.planets([5 | (1 << 3) | (1 << 9), 3 | (6 << 3) | (6 << 9)])
        runs = [
            {"planet": "P0", "start": 0, "count": 5},
            {"planet": "P1", "start": 5, "count": 3},
        ]
        crosscheck_planet_layout(planets, runs)

    def test_rejects_wrong_city_count(self) -> None:
        planets = self.planets([5 | (1 << 3) | (1 << 9)])
        with self.assertRaises(ValueError):
            crosscheck_planet_layout(planets, [{"planet": "P0", "start": 0, "count": 4}])

    def test_rejects_wrong_run_start(self) -> None:
        planets = self.planets([5 | (1 << 3) | (1 << 9)])
        with self.assertRaises(ValueError):
            crosscheck_planet_layout(planets, [{"planet": "P0", "start": 3, "count": 5}])

    def test_rejects_length_mismatch(self) -> None:
        planets = self.planets([5 | (1 << 3) | (1 << 9)])
        with self.assertRaises(ValueError):
            crosscheck_planet_layout(planets, [])


class PcodeRuleTests(unittest.TestCase):
    def test_every_signature_is_long_enough(self) -> None:
        for rule in PCODE_RULES:
            signature = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
            self.assertGreaterEqual(
                len(signature), MINIMUM_SIGNATURE_BYTES, f"{rule['id']} 的簽章太短"
            )

    def test_rule_ids_are_unique(self) -> None:
        ids = [rule["id"] for rule in PCODE_RULES]
        self.assertEqual(len(ids), len(set(ids)))

    def test_rejects_signature_that_appears_twice(self) -> None:
        # 訊息要指名「出現兩次」，否則這條會因為「其他規則找不到」而假綠。
        rule = PCODE_RULES[0]
        signature = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        disk = bytes(0x2E000 + 0x9E00 + 0x298) + signature + bytes(16) + signature
        with self.assertRaises(ValueError) as caught:
            pin_pcode_rules(disk)
        self.assertIn("出現 2 次", str(caught.exception))

    def test_rejects_signature_at_the_wrong_address(self) -> None:
        rule = PCODE_RULES[0]
        signature = bytes.fromhex(str(rule["signature"]).replace(" ", ""))
        with self.assertRaises(ValueError) as caught:
            pin_pcode_rules(bytes(16) + signature)
        self.assertIn("但宣告的段基底與段內位址算出", str(caught.exception))

class DecodeNameTests(unittest.TestCase):
    def test_length_prefixed_name(self) -> None:
        self.assertEqual(decode_name(name_field("Jondd"), 0), "Jondd")

    def test_full_width_name(self) -> None:
        self.assertEqual(decode_name(name_field("KalManDaa"), 0), "KalManDaa")

    def test_rejects_length_over_field(self) -> None:
        field = bytes([NAME_FIELD_SIZE]) + b"toolongxx"
        with self.assertRaises(ValueError):
            decode_name(field, 0)

    def test_rejects_non_zero_padding(self) -> None:
        with self.assertRaises(ValueError):
            decode_name(name_field("Ferr", pad_byte=0x41), 0)

    def test_rejects_non_printable_body(self) -> None:
        field = bytes([2, 0x41, 0x01]) + bytes(7)
        with self.assertRaises(ValueError):
            decode_name(field, 0)

    def test_rejects_wrong_field_size(self) -> None:
        with self.assertRaises(ValueError):
            decode_name(b"\x02AB", 0)


class DecodeTableTests(unittest.TestCase):
    def test_preserves_raw_offset_and_tail(self) -> None:
        disk = bytes(0x100) + record("Ferr", bytes([0xAA] * 6)) + record("Hepa", bytes([0xBB] * 6))
        records = decode_table(disk, 0x100, PLANET_RECORD_SIZE, 2, "planet")
        self.assertEqual([entry["name"] for entry in records], ["Ferr", "Hepa"])
        self.assertEqual(records[0]["rawOffset"], "0x00100")
        self.assertEqual(records[1]["rawOffset"], "0x00110")
        self.assertEqual(records[0]["tailHex"], "aa" * 6)
        self.assertEqual(records[1]["tailHex"], "bb" * 6)

    def test_rejects_truncated_record(self) -> None:
        disk = record("Ferr", bytes(6))[:-2]
        with self.assertRaises(ValueError):
            decode_table(disk, 0, PLANET_RECORD_SIZE, 1, "planet")


class CrosscheckTests(unittest.TestCase):
    """以兩星系／三行星／四城市的合成世界驗證分組與 fail-closed 條件。"""

    def setUp(self) -> None:
        self.systems = table_records(["Alpha", "Beta"], SYSTEM_RECORD_SIZE)
        self.planets = table_records(["Aone", "Atwo", "Bone"], PLANET_RECORD_SIZE)
        self.pairs = [
            {"system": "Alpha", "planet": "Aone"},
            {"system": "Alpha", "planet": "Atwo"},
            {"system": "Beta", "planet": "Bone"},
        ]
        self.triples = [
            {"system": "Alpha", "planet": "Aone", "city": "Ca"},
            {"system": "Alpha", "planet": "Aone", "city": "Cb"},
            {"system": "Alpha", "planet": "Atwo", "city": "Cc"},
            {"system": "Beta", "planet": "Bone", "city": "Cd"},
            {"system": "Alpha", "planet": "Aone", "city": "Hidden"},
        ]

    def cities(self, names: list[str]) -> list[dict[str, object]]:
        return table_records(names, CITY_RECORD_SIZE)

    def test_contiguous_runs_and_trailing_city(self) -> None:
        cities = self.cities(["Ca", "Cb", "Cc", "Cd", "dummy", "Hidden"])
        result = crosscheck(self.systems, self.planets, cities, self.pairs, self.triples)
        self.assertEqual(result["contiguousCityCount"], 4)
        self.assertEqual(result["fillerCityCount"], 1)
        self.assertEqual(result["trailingCityCount"], 1)
        self.assertEqual(
            [(run["planet"], run["start"], run["count"]) for run in result["planetCityRuns"]],
            [("Aone", 0, 2), ("Atwo", 2, 1), ("Bone", 3, 1)],
        )
        self.assertEqual(
            [(run["system"], run["start"], run["count"]) for run in result["systemPlanetRuns"]],
            [("Alpha", 0, 2), ("Beta", 2, 1)],
        )
        self.assertEqual(result["trailingCities"][0]["city"], "Hidden")

    def test_rejects_non_contiguous_planet_cities(self) -> None:
        cities = self.cities(["Ca", "Cc", "Cb", "Cd", "dummy", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, self.planets, cities, self.pairs, self.triples)

    def test_rejects_planet_order_mismatch(self) -> None:
        planets = table_records(["Atwo", "Aone", "Bone"], PLANET_RECORD_SIZE)
        cities = self.cities(["Ca", "Cb", "Cc", "Cd", "dummy", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, planets, cities, self.pairs, self.triples)

    def test_rejects_city_absent_from_map(self) -> None:
        cities = self.cities(["Ca", "Cb", "Cc", "Cz", "dummy", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, self.planets, cities, self.pairs, self.triples)

    def test_rejects_filler_inside_contiguous_section(self) -> None:
        cities = self.cities(["Ca", "Cb", "dummy", "Cc", "Cd", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, self.planets, cities, self.pairs, self.triples)

    def test_rejects_missing_filler_record(self) -> None:
        cities = self.cities(["Ca", "Cb", "Cc", "Cd"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, self.planets, cities, self.pairs, self.triples)

    def test_rejects_system_set_mismatch(self) -> None:
        systems = table_records(["Alpha", "Gamma"], SYSTEM_RECORD_SIZE)
        cities = self.cities(["Ca", "Cb", "Cc", "Cd", "dummy", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(systems, self.planets, cities, self.pairs, self.triples)

    def test_rejects_planet_missing_from_pairs(self) -> None:
        pairs = [pair for pair in self.pairs if pair["planet"] != "Bone"]
        cities = self.cities(["Ca", "Cb", "Cc", "Cd", "dummy", "Hidden"])
        with self.assertRaises(ValueError):
            crosscheck(self.systems, self.planets, cities, pairs, self.triples)


if __name__ == "__main__":
    unittest.main()
