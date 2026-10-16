#!/usr/bin/env python3
"""check_city_map_runtime 的判準測試：合成記憶體與地圖，不需要 dump。"""

import json
import pathlib
import tempfile
import unittest

from check_city_map_runtime import (
    COLUMNS,
    FIRST_COLUMN,
    FIRST_ROW,
    GRID_COLUMNS,
    GRID_ROWS,
    ROWS,
    VARIANT,
    compare,
    locate_buffer,
    main,
    tiles_for,
)

PAD = 4096


def make_city(grid_value=1, buildings=()):
    """20×12 的地圖；grid[i][j] 的 i 是欄、j 是列。"""
    return {
        "cityNumber": 1,
        "cityName": "Testville",
        "grid": [[grid_value] * GRID_ROWS for _ in range(GRID_COLUMNS)],
        "buildings": [{"slot": n, "code": c, "row": r, "column": col}
                      for n, (c, r, col) in enumerate(buildings)],
    }


def make_memory(city, *, variant_cells=(), overrides=None, outside=None):
    """把城市畫進一塊 40×24 的緩衝，前後補零當成整份記憶體。"""
    buffer = bytearray(ROWS * COLUMNS)
    build = {(b["row"], b["column"]): b["code"] for b in city["buildings"]}
    for column in range(GRID_COLUMNS):
        for row in range(GRID_ROWS):
            code = build.get((column, row))
            if code is None:
                code = tiles_for(city["grid"][column][row])[0]
                # 碼大於 15 在緩衝裡必然帶偏移，否則 check_exit 會把它正規化成別的碼。
                if code > 15 or (column, row) in variant_cells:
                    code += VARIANT
            buffer[(FIRST_ROW + row) * COLUMNS + FIRST_COLUMN + column] = code
    for (column, row), value in (overrides or {}).items():
        buffer[(FIRST_ROW + row) * COLUMNS + FIRST_COLUMN + column] = value
    if outside:
        (row, column), value = outside
        buffer[row * COLUMNS + column] = value
    return bytes(PAD) + bytes(buffer) + bytes(PAD)


class LocateTest(unittest.TestCase):
    def test_the_buffer_is_found_without_being_told_where_it_is(self):
        memory = make_memory(make_city())
        self.assertIn(PAD, locate_buffer(memory))

    def test_a_non_zero_cell_outside_the_city_rectangle_disqualifies_the_block(self):
        # draw_city_map 只寫中間那塊；矩形外有東西就不是這個緩衝。
        memory = make_memory(make_city(), outside=((0, 0), 3))
        self.assertNotIn(PAD, locate_buffer(memory))

    def test_all_zero_memory_yields_no_candidate(self):
        self.assertEqual(locate_buffer(bytes(PAD * 4)), [])


class CompareTest(unittest.TestCase):
    def test_a_faithfully_drawn_city_matches_every_cell(self):
        city = make_city()
        result = compare(make_memory(city), PAD, city)
        self.assertEqual(result["mismatches"], [])
        self.assertEqual(result["counts"]["exact"], GRID_COLUMNS * GRID_ROWS)

    def test_the_plus_sixteen_visual_variant_is_accepted(self):
        city = make_city()
        result = compare(make_memory(city, variant_cells={(2, 3)}), PAD, city)
        self.assertEqual(result["mismatches"], [])
        self.assertEqual(result["counts"]["variant"], 1)

    def test_terrain_five_must_be_drawn_as_seventeen(self):
        # 恆等會讓這格畫成 5，而 5 在 check_building 是「碼 11」那一支，語意完全不同。
        # 緩衝裡的值因此是 33（17 + 16），由 make_memory 依「碼大於 15 必帶偏移」補上。
        city = make_city(grid_value=5)
        good = compare(make_memory(city), PAD, city)
        self.assertEqual(good["mismatches"], [])
        bad = compare(make_memory(city, overrides={(0, 0): 5}), PAD, city)
        self.assertEqual(len(bad["mismatches"]), 1)

    def test_terrain_eleven_may_be_drawn_as_five_or_eleven(self):
        # 兩種畫法在 check_building 等價（if 碼 = 5 then L1 := 11）。
        city = make_city(grid_value=11)
        for drawn in (5, 11):
            result = compare(make_memory(city, overrides={(0, 0): drawn}), PAD, city)
            self.assertEqual(result["mismatches"], [], f"繪圖碼 {drawn} 該被接受")

    def test_terrain_eleven_drawn_as_something_else_is_a_mismatch(self):
        city = make_city(grid_value=11)
        result = compare(make_memory(city, overrides={(0, 0): 7}), PAD, city)
        self.assertEqual(len(result["mismatches"]), 1)

    def test_a_building_cell_must_hold_its_own_slot_code(self):
        city = make_city(buildings=[(34, 5, 6)])
        self.assertEqual(compare(make_memory(city), PAD, city)["counts"]["building"], 1)
        moved = compare(make_memory(city, overrides={(5, 6): 35}), PAD, city)
        self.assertEqual(len(moved["mismatches"]), 1)

    def test_a_building_cell_does_not_accept_the_variant_offset(self):
        # 建築碼本身已經大於 16；再加變體偏移就會撞進別的槽位。
        city = make_city(buildings=[(34, 5, 6)])
        result = compare(make_memory(city, overrides={(5, 6): 34 + VARIANT}), PAD, city)
        self.assertEqual(len(result["mismatches"]), 1)


class MainTest(unittest.TestCase):
    def run_main(self, memory, city, extra=()):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            (root / "mem.bin").write_bytes(memory)
            (root / "maps.json").write_text(json.dumps({"cities": [city]}))
            return main([str(root / "mem.bin"), "--maps", str(root / "maps.json"), *extra])

    def test_a_matching_dump_passes(self):
        city = make_city(buildings=[(34, 5, 6)])
        self.assertEqual(self.run_main(make_memory(city), city), 0)

    def test_one_wrong_cell_fails(self):
        city = make_city()
        self.assertEqual(self.run_main(make_memory(city, overrides={(1, 1): 9}), city), 1)

    def test_a_dump_taken_outside_a_city_fails_rather_than_matching_something_else(self):
        city = make_city()
        self.assertEqual(self.run_main(bytes(PAD * 4), city), 1)

    def test_an_unknown_city_number_fails(self):
        city = make_city()
        self.assertEqual(self.run_main(make_memory(city), city, ["--city", "99"]), 1)


if __name__ == "__main__":
    unittest.main()
